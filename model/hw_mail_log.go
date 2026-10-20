// HisarWeb AI customer mail log: the memory of what was mailed, so a restart or
// a second round never mails a customer twice.
//
// The row is written before the mail leaves the process. The unique key
// (user, package key, kind) is what makes a round idempotent; a send that fails
// keeps its row on purpose, because the mail server may already have accepted
// the message, and a second attempt could deliver it twice. The failure is
// logged when it happens; the next threshold crossing gets a new key.
package model

import (
	"gorm.io/gorm/clause"
)

// Mail kinds of the customer mail job. They are part of the idempotency key, so
// the strings must stay stable once a customer has been mailed.
const (
	HwMailKindExpiry     = "expiry"
	HwMailKindLowBalance = "low_balance"
)

// HwMailKeyUnpackaged is the package key of a low balance that is not held by
// any package: balance from an older ledger or a manual adjustment. It spends
// after every package is empty, so it is the last thing a customer spends.
const HwMailKeyUnpackaged = "unpackaged"

// Option keys of the low-balance threshold. The threshold is the configured
// percentage of the last purchased package, raised to the configured minimum in
// whole lira when that is larger.
const (
	HwLowBalancePercentOption = "HwLowBalancePercent"
	HwLowBalanceMinTryOption  = "HwLowBalanceMinTry"
)

// HwMailLog is one mail that was claimed for sending. It carries no content and
// no address: only which customer, which package and which kind, so the table
// can be kept without storing personal data beyond the user id.
type HwMailLog struct {
	Id         int    `json:"id" gorm:"primaryKey"`
	UserId     int    `json:"user_id" gorm:"not null;uniqueIndex:idx_hw_mail_log_key,priority:1"`
	PackageKey string `json:"package_key" gorm:"type:varchar(64);not null;uniqueIndex:idx_hw_mail_log_key,priority:2"`
	Kind       string `json:"kind" gorm:"type:varchar(32);not null;uniqueIndex:idx_hw_mail_log_key,priority:3"`
	CreatedAt  int64  `json:"created_at" gorm:"not null;default:0"`
}

func (HwMailLog) TableName() string {
	return "hw_mail_log"
}

// HwClaimMail reserves the mail of this (user, package, kind) for sending. It
// reports false when the mail was claimed before, also by an earlier process,
// so the caller skips it. The caller must claim before sending: that order is
// what keeps a crash between the two from mailing twice.
func HwClaimMail(userID int, packageKey string, kind string, now int64) (bool, error) {
	if userID <= 0 || packageKey == "" || now <= 0 {
		return false, ErrHwInvalidCreditGrant
	}
	if kind != HwMailKindExpiry && kind != HwMailKindLowBalance {
		return false, ErrHwInvalidCreditGrant
	}
	entry := HwMailLog{UserId: userID, PackageKey: packageKey, Kind: kind, CreatedAt: now}
	// The unique index decides, not a read-then-write check: two rounds racing
	// on the same mail must not both send it. ON CONFLICT DO NOTHING is emitted
	// by all three supported dialects and yields zero affected rows on a repeat.
	result := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "package_key"}, {Name: "kind"}},
		DoNothing: true,
	}).Create(&entry)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// HwActivePackageUserIDs lists the users that still hold an active credit
// package, in id order. Only they can receive a package mail, and only their
// balance can be compared with a threshold derived from a package.
func HwActivePackageUserIDs() ([]int, error) {
	var userIDs []int
	err := DB.Model(&HwCreditPackage{}).
		Where("retired_at = 0").
		Distinct().
		Order("user_id").
		Pluck("user_id", &userIDs).Error
	return userIDs, err
}

// HwLastPurchasedQuota returns the granted quota of the user's most recent paid
// package, also when it was retired already: the size of the last purchase is
// the reference for the low-balance threshold, not what is left of it.
func HwLastPurchasedQuota(userID int) (int, bool, error) {
	if userID <= 0 {
		return 0, false, ErrHwInvalidCreditGrant
	}
	var pkgs []HwCreditPackage
	err := DB.Select("id", "granted_quota").
		Where("user_id = ? AND source = ?", userID, HwSourcePurchase).
		Order("created_at DESC, id DESC").
		Limit(1).
		Find(&pkgs).Error
	if err != nil {
		return 0, false, err
	}
	if len(pkgs) == 0 {
		return 0, false, nil
	}
	return pkgs[0].GrantedQuota, true, nil
}

// HwLastOrderLanguage returns the language the customer chose on the most recent
// top-up order, or an empty string when there is no order. It is the first
// source of the mail language: the choice made at the checkout is the closest
// to what the customer reads.
func HwLastOrderLanguage(userID int) (string, error) {
	if userID <= 0 {
		return "", ErrHwInvalidOrder
	}
	var orders []HwOrder
	err := DB.Select("id", "language").
		Where("user_id = ?", userID).
		Order("created_at DESC, id DESC").
		Limit(1).
		Find(&orders).Error
	if err != nil {
		return "", err
	}
	if len(orders) == 0 {
		return "", nil
	}
	return orders[0].Language, nil
}
