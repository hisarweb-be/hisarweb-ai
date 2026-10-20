package model

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// HwSignupIpDailyLimit is the number of new accounts one IP address may create
// per UTC day. The fourth registration from the same address within a day is
// refused; the next UTC day the counter starts over.
const HwSignupIpDailyLimit = 3

// HwSignupIp counts successful new-account reservations per hashed IP address
// and UTC day. The address itself is never stored: the row holds the keyed
// hash of the address (HMAC with HW_SIGNAL_SALT), exactly like the ip signals
// of hw_user_signals, so the daily sweep can link a counter to the same
// address hash. The table is restart-proof because it lives in the database,
// not in memory or Redis.
type HwSignupIp struct {
	Id        int    `json:"id" gorm:"primaryKey"`
	IpHash    string `json:"ip_hash" gorm:"type:varchar(64);not null;uniqueIndex:idx_hw_signup_ips_identity,priority:1"`
	Day       int    `json:"day" gorm:"not null;uniqueIndex:idx_hw_signup_ips_identity,priority:2"`
	Count     int    `json:"count" gorm:"not null;default:0"`
	UpdatedAt int64  `json:"updated_at" gorm:"not null;default:0"`
}

func (HwSignupIp) TableName() string {
	return "hw_signup_ips"
}

// hwSignupIpDayKey reduces a unix timestamp to its UTC day as YYYYMMDD, so the
// "per day" window follows the UTC day boundary used by the rest of the house
// (daily limits close at midnight UTC).
func hwSignupIpDayKey(now int64) int {
	utc := time.Unix(now, 0).UTC()
	return utc.Year()*10000 + int(utc.Month())*100 + utc.Day()
}

// HwReserveSignupSlot reserves one new-account slot for an IP address on its
// UTC day and reports whether the reservation fits the limit. The counter is
// incremented atomically: the conditional update only raises a counter that is
// still below the limit, so a refused fourth registration does not consume a
// slot of the day and cannot lock the address out for legitimate attempts.
//
// The raw address never reaches the table; hashing it can fail when
// HW_SIGNAL_SALT is not configured, and the error is returned to the caller,
// which decides whether registration continues without the per-IP limit.
//
// The counter follows the address that hwHashRawSignal derives: an IPv4 address
// as written, and an IPv6 address by its /64 prefix, because one customer
// normally controls a whole /64 and rotating the lower bits must not buy extra
// accounts. The address must also be the one the connection really came from:
// gin reports a forwarded X-Forwarded-For / X-Real-IP only when the direct peer
// is a trusted proxy, so a deployment behind a proxy has to list that proxy in
// TRUSTED_PROXIES (common.ResolveTrustedProxies). With every peer trusted, a
// client could send a fresh address per request and walk past this limit.
func HwReserveSignupSlot(clientIP string, now int64, limit int) (bool, error) {
	if limit <= 0 {
		return true, nil
	}
	ipHash, err := hwHashRawSignal(HwSignalKindIp, clientIP)
	if err != nil {
		return false, err
	}
	if ipHash == "" {
		return false, ErrHwInvalidUserSignal
	}
	day := hwSignupIpDayKey(now)
	// A reservation attempt whose account creation later fails keeps its slot:
	// counting attempts is the conservative side for an anti-abuse limit, and
	// undoing a reservation would let a burst of failing signups hammer the
	// endpoint without ever hitting the cap.
	row := HwSignupIp{IpHash: ipHash, Day: day, Count: 1, UpdatedAt: now}
	result := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "ip_hash"}, {Name: "day"}},
		DoNothing: true,
	}).Create(&row)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 1 {
		return true, nil
	}
	result = DB.Model(&HwSignupIp{}).
		Where("ip_hash = ? AND day = ? AND count < ?", ipHash, day, limit).
		Updates(map[string]any{"count": gorm.Expr("count + 1"), "updated_at": now})
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 1 {
		return true, nil
	}
	var count int64
	if err := DB.Model(&HwSignupIp{}).Where("ip_hash = ? AND day = ?", ipHash, day).Count(&count).Error; err != nil {
		return false, err
	}
	if count == 0 {
		// The row vanished between the insert and the update (nothing deletes
		// rows today). Take the insert path once more instead of refusing.
		result = DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "ip_hash"}, {Name: "day"}},
			DoNothing: true,
		}).Create(&HwSignupIp{IpHash: ipHash, Day: day, Count: 1, UpdatedAt: now})
		if result.Error != nil {
			return false, result.Error
		}
		return result.RowsAffected == 1, nil
	}
	return false, nil
}

// HwDeleteOldSignupIpRows removes counters older than the given number of UTC
// days, so the table stays small. The daily abuse sweep calls this.
func HwDeleteOldSignupIpRows(now int64, keepDays int) error {
	if keepDays <= 0 {
		keepDays = 2
	}
	cutoffDay := hwSignupIpDayKey(now - int64(keepDays)*24*3600)
	return DB.Where("day < ?", cutoffDay).Delete(&HwSignupIp{}).Error
}
