package model

import (
	"errors"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Reasons an account can be frozen for, recorded on the flag row. They are
// coarse categories on purpose: a reason never contains a signal value.
const (
	HwFreezeReasonSharedPhone  = "shared_phone"
	HwFreezeReasonSharedDevice = "shared_device"
	HwFreezeReasonSharedCard   = "shared_card"
	HwFreezeReasonSignupWave   = "signup_wave"
)

// HwAccountFlag is the review hold of the daily abuse sweep. Freezing does not
// disable the account and never touches any balance: paid credit stays exactly
// as it is, and the block only applies to API usage that draws on the free
// trial credit (the free group). An administrator thaws the account after the
// review; the unfreeze columns keep who and when.
type HwAccountFlag struct {
	Id         int    `json:"id" gorm:"primaryKey"`
	UserId     int    `json:"user_id" gorm:"not null;uniqueIndex"`
	Frozen     bool   `json:"frozen"`
	Reason     string `json:"reason" gorm:"type:varchar(64);not null;default:''"`
	FrozenAt   int64  `json:"frozen_at" gorm:"not null;default:0"`
	UnfrozenAt int64  `json:"unfrozen_at" gorm:"not null;default:0"`
	UnfrozenBy int    `json:"unfrozen_by" gorm:"not null;default:0"`
}

func (HwAccountFlag) TableName() string {
	return "hw_account_flags"
}

// FreezeHwAccounts freezes the given common-user accounts and returns how many
// were frozen now. Accounts that are already frozen keep their original reason
// and time, so the first finding stays visible for the review. Administrators
// and the root user are never frozen.
func FreezeHwAccounts(userIDs []int, reason string, now int64) (int, error) {
	if len(userIDs) == 0 || reason == "" {
		return 0, ErrHwInvalidUserSignal
	}
	var freezable []int
	if err := DB.Model(&User{}).
		Where("id IN ? AND role = ?", userIDs, common.RoleCommonUser).
		Pluck("id", &freezable).Error; err != nil {
		return 0, err
	}
	frozen := 0
	for _, userID := range freezable {
		result := DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}},
			DoNothing: true,
		}).Create(&HwAccountFlag{UserId: userID, Frozen: true, Reason: reason, FrozenAt: now})
		if result.Error != nil {
			return frozen, result.Error
		}
		if result.RowsAffected == 1 {
			frozen++
			hwInvalidateFrozenCache(userID)
		}
	}
	return frozen, nil
}

// HwUnfreezeAccount thaws one account after review and reports whether a
// frozen flag was lifted. The cache entry is dropped, so the next request sees
// the thawed state at once.
func HwUnfreezeAccount(userID int, adminID int, now int64) (bool, error) {
	if userID <= 0 {
		return false, ErrHwInvalidUserSignal
	}
	result := DB.Model(&HwAccountFlag{}).
		Where("user_id = ? AND frozen = ?", userID, true).
		Updates(map[string]any{"frozen": false, "unfrozen_at": now, "unfrozen_by": adminID})
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected > 0 {
		hwInvalidateFrozenCache(userID)
		return true, nil
	}
	return false, nil
}

// hwFrozenCache holds the frozen state per user for a short window, so the
// relay path does not query the flag table on every request of the free group.
// The window is deliberately short: an unfreeze must take effect quickly.
const hwFrozenCacheTTL = 15 * time.Second

type hwFrozenCacheEntry struct {
	frozen  bool
	expires time.Time
}

var (
	hwFrozenCacheMu sync.Mutex
	hwFrozenCache   = make(map[int]hwFrozenCacheEntry)
)

func hwInvalidateFrozenCache(userID int) {
	hwFrozenCacheMu.Lock()
	delete(hwFrozenCache, userID)
	hwFrozenCacheMu.Unlock()
}

// HwAccountFrozen reports whether the account is currently frozen. The value
// comes from the short-lived cache when it is fresh; a database error is
// returned and never cached as "not frozen".
func HwAccountFrozen(userID int) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	hwFrozenCacheMu.Lock()
	entry, ok := hwFrozenCache[userID]
	hwFrozenCacheMu.Unlock()
	if ok && time.Now().Before(entry.expires) {
		return entry.frozen, nil
	}
	var flag HwAccountFlag
	err := DB.Where("user_id = ? AND frozen = ?", userID, true).First(&flag).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			hwFrozenCacheStore(userID, false)
			return false, nil
		}
		return false, err
	}
	hwFrozenCacheStore(userID, true)
	return true, nil
}

func hwFrozenCacheStore(userID int, frozen bool) {
	hwFrozenCacheMu.Lock()
	hwFrozenCache[userID] = hwFrozenCacheEntry{frozen: frozen, expires: time.Now().Add(hwFrozenCacheTTL)}
	hwFrozenCacheMu.Unlock()
}
