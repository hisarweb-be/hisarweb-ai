package model

// HisarWeb AI credit validity: the balance stays valid 30 days after the last
// paid top-up. Every purchase pushes the expiry of all active packages out to
// 30 days from that moment (never earlier than they already were, and never for
// a package that has already expired), so the customer has one date to watch.
// This file holds the extension, the summary the dashboard card reads, and the
// one-off migration that gave the customers of the old rules a fresh 30 days.

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	// HwValidityActive: the account holds package credit that is still valid.
	HwValidityActive = "active"
	// HwValidityExpired: the last package credit ran out of time and was retired
	// by the expiry job; ExpiredQuota says how much was taken off the balance.
	HwValidityExpired = "expired"
	// HwValidityNone: there never was a package with credit to expire.
	HwValidityNone = "none"

	// hwValidityMigratedOption marks the one-off rollout migration as done.
	hwValidityMigratedOption = "HwCreditValidityMigratedAt"
)

// hwResetCreditValidity pushes every active, not yet expired package of the user
// out to now + 30 days. It is called in the transaction of a paid purchase. A
// package whose expiry is already later (a channel grant) stays as it is, and a
// package that has already passed its expiry is left for the expiry job: paying
// does not bring expired credit back.
func hwResetCreditValidity(tx *gorm.DB, userID int, now int64) error {
	if tx == nil || userID <= 0 || now <= 0 {
		return ErrHwInvalidCreditGrant
	}
	until := now + hwCreditPackageValiditySeconds
	return tx.Model(&HwCreditPackage{}).
		Where("user_id = ? AND retired_at = 0 AND expires_at > ? AND expires_at < ?", userID, now, until).
		UpdateColumns(map[string]any{"expires_at": until}).Error
}

// HwValiditySummary is what the dashboard card shows.
type HwValiditySummary struct {
	Status string `json:"status"`
	// ValidUntil is the unix time at which the soonest expiring credit lapses;
	// zero unless Status is active.
	ValidUntil int64 `json:"valid_until"`
	// DaysRemaining counts whole days up to ValidUntil, rounded up, so the last
	// day still says 1; zero unless Status is active.
	DaysRemaining int `json:"days_remaining"`
	// ExpiredAt and ExpiredQuota describe the latest expiry that took credit off
	// the balance; zero unless Status is expired.
	ExpiredAt    int64 `json:"expired_at"`
	ExpiredQuota int   `json:"expired_quota"`
}

// HwUserValidity reads the validity of one user from the database.
func HwUserValidity(userID int, now int64) (HwValiditySummary, error) {
	views, err := ListUserCreditPackages(userID)
	if err != nil {
		return HwValiditySummary{}, err
	}
	summary := HwValiditySummary{Status: HwValidityNone}
	for _, view := range views {
		if view.RemainingQuota <= 0 {
			continue
		}
		if summary.Status != HwValidityActive || view.ExpiresAt < summary.ValidUntil {
			summary.Status = HwValidityActive
			summary.ValidUntil = view.ExpiresAt
		}
	}
	if summary.Status == HwValidityActive {
		summary.DaysRemaining = hwDaysUntil(summary.ValidUntil, now)
		return summary, nil
	}
	// Refunds also retire a package, but before its expiry; only a package that
	// was retired at or after its expiry lapsed.
	var last HwCreditPackage
	err = DB.Where("user_id = ? AND retired_at > 0 AND retired_remaining > 0 AND retired_at >= expires_at", userID).
		Order("retired_at DESC, id DESC").First(&last).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return summary, nil
	}
	if err != nil {
		return HwValiditySummary{}, err
	}
	var batch []HwCreditPackage
	if err := DB.Where("user_id = ? AND retired_at = ? AND retired_remaining > 0 AND retired_at >= expires_at", userID, last.RetiredAt).
		Find(&batch).Error; err != nil {
		return HwValiditySummary{}, err
	}
	summary.Status = HwValidityExpired
	summary.ExpiredAt = last.ExpiresAt
	for _, pkg := range batch {
		summary.ExpiredQuota += pkg.RetiredRemaining
		summary.ExpiredAt = max(summary.ExpiredAt, pkg.ExpiresAt)
	}
	return summary, nil
}

func hwDaysUntil(until, now int64) int {
	if until <= now {
		return 0
	}
	const day = int64(24 * time.Hour / time.Second)
	return int((until - now + day - 1) / day)
}

// HwMigrateCreditValidity gives every active package at least 30 days from the
// rollout, once. Customers of the old rules had a mix of 14 and 30 day packages
// that all counted from different moments; this puts them on the same footing.
// The marker is stored in the options table, so a restart does not extend the
// packages again. Balance that belongs to no package never expires and is not
// touched. It reports how many packages were moved.
func HwMigrateCreditValidity(db *gorm.DB, now int64) (int64, error) {
	if db == nil || now <= 0 {
		return 0, ErrHwInvalidCreditGrant
	}
	var marker Option
	err := db.Where(commonKeyCol+" = ?", hwValidityMigratedOption).First(&marker).Error
	if err == nil && strings.TrimSpace(marker.Value) != "" {
		return 0, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	var moved int64
	err = db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&HwCreditPackage{}).
			Where("retired_at = 0 AND expires_at > ? AND expires_at < ?", now, now+hwCreditPackageValiditySeconds).
			UpdateColumns(map[string]any{"expires_at": now + hwCreditPackageValiditySeconds})
		if result.Error != nil {
			return result.Error
		}
		moved = result.RowsAffected
		option := Option{Key: hwValidityMigratedOption}
		if err := tx.FirstOrCreate(&option, Option{Key: hwValidityMigratedOption}).Error; err != nil {
			return err
		}
		return tx.Model(&Option{}).Where(commonKeyCol+" = ?", hwValidityMigratedOption).
			Update("value", time.Unix(now, 0).UTC().Format(time.RFC3339)).Error
	})
	return moved, err
}
