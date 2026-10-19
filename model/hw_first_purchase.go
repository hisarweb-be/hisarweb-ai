package model

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Reasons HwClaimFirstPurchaseDiscount refuses a claim with.
const (
	HwFirstPurchaseAlreadyClaimed = "already_claimed"
	HwFirstPurchaseSharedPhone    = "shared_phone"
	HwFirstPurchaseSharedCard     = "shared_card"
)

// ErrHwInvalidFirstPurchaseClaim rejects a claim without a user or an order.
var ErrHwInvalidFirstPurchaseClaim = errors.New("invalid first purchase claim")

// HwFirstPurchaseClaim is the single first-purchase discount a user may take. It
// is written when the order is created, not when the payment arrives, so two
// orders cannot both be priced as a first purchase. The card fingerprint and the
// phone hash are copied from the user's signals as our own keyed hashes; the raw
// values are never stored here.
type HwFirstPurchaseClaim struct {
	Id              int    `json:"id" gorm:"primaryKey"`
	UserId          int    `json:"user_id" gorm:"not null;uniqueIndex"`
	CardFingerprint string `json:"card_fingerprint" gorm:"type:varchar(64);index"`
	PhoneHash       string `json:"phone_hash" gorm:"type:varchar(64);index"`
	OrderRef        string `json:"order_ref" gorm:"type:varchar(128);not null"`
	CreatedAt       int64  `json:"created_at" gorm:"not null;default:0"`
}

func (HwFirstPurchaseClaim) TableName() string {
	return "hw_first_purchase_claims"
}

// HwClaimFirstPurchaseDiscount claims the first-purchase discount of one order
// for a user and reports whether the order may use it.
//
// The claim is taken atomically on the unique user_id index, so concurrent
// order creations for the same user produce one eligible order and one refusal.
// It is refused when the user already claimed the discount for another order, or
// when another user with the same non-empty card fingerprint or phone hash (from
// hw_user_signals, the whole history) already claimed it. Repeating the call for
// the same order reference returns the same answer.
//
// The check across users reads before it writes; two orders of two accounts that
// share a card or phone and are created in the same instant can both pass. The
// second payment is caught when the claim of the first one exists, so the window
// is one in-flight order, not a repeatable hole.
//
// Passing a transaction makes the claim part of the caller's order transaction.
func HwClaimFirstPurchaseDiscount(tx *gorm.DB, userID int, orderRef string, now int64) (bool, string, error) {
	if userID <= 0 || orderRef == "" || now <= 0 {
		return false, "", ErrHwInvalidFirstPurchaseClaim
	}
	if tx == nil {
		tx = DB
	}
	hashes, err := hwNewestSignalHashes(tx, userID, []string{HwSignalKindPhone, HwSignalKindCard})
	if err != nil {
		return false, "", err
	}
	checks := []struct {
		kind   string
		column string
		hash   string
	}{
		{HwSignalKindPhone, "phone_hash", hashes[HwSignalKindPhone]},
		{HwSignalKindCard, "card_fingerprint", hashes[HwSignalKindCard]},
	}
	for _, check := range checks {
		if check.hash == "" {
			continue
		}
		var count int64
		err := tx.Model(&HwFirstPurchaseClaim{}).
			Where("user_id <> ? AND "+check.column+" = ?", userID, check.hash).
			Count(&count).Error
		if err != nil {
			return false, "", err
		}
		if count > 0 {
			return false, "shared_" + check.kind, nil
		}
	}

	claim := HwFirstPurchaseClaim{
		UserId:          userID,
		CardFingerprint: hashes[HwSignalKindCard],
		PhoneHash:       hashes[HwSignalKindPhone],
		OrderRef:        orderRef,
		CreatedAt:       now,
	}
	result := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoNothing: true,
	}).Create(&claim)
	if result.Error != nil {
		return false, "", result.Error
	}
	if result.RowsAffected == 1 {
		return true, "", nil
	}

	var existing HwFirstPurchaseClaim
	if err := tx.Where("user_id = ?", userID).First(&existing).Error; err != nil {
		return false, "", err
	}
	if existing.OrderRef == orderRef {
		return true, "", nil
	}
	return false, HwFirstPurchaseAlreadyClaimed, nil
}

// HwReleaseFirstPurchaseClaim gives the claim back when the order it was made for
// is not paid (expired or failed), so the customer keeps the discount for a later
// order. Only a claim of that same order reference is removed; releasing twice or
// releasing something that is not there changes nothing.
func HwReleaseFirstPurchaseClaim(tx *gorm.DB, userID int, orderRef string) error {
	if userID <= 0 || orderRef == "" {
		return ErrHwInvalidFirstPurchaseClaim
	}
	if tx == nil {
		tx = DB
	}
	return tx.Where("user_id = ? AND order_ref = ?", userID, orderRef).Delete(&HwFirstPurchaseClaim{}).Error
}

// hwNewestSignalHashes returns the most recent hash per requested kind for one
// user. Kinds without a signal are absent from the map.
func hwNewestSignalHashes(tx *gorm.DB, userID int, kinds []string) (map[string]string, error) {
	var signals []HwUserSignal
	if err := tx.Where("user_id = ? AND kind IN ? AND hash <> ?", userID, kinds, "").Find(&signals).Error; err != nil {
		return nil, err
	}
	newest := make(map[string]HwUserSignal, len(kinds))
	for _, signal := range signals {
		previous, ok := newest[signal.Kind]
		if !ok || signal.LastSeen > previous.LastSeen || (signal.LastSeen == previous.LastSeen && signal.Id > previous.Id) {
			newest[signal.Kind] = signal
		}
	}
	hashes := make(map[string]string, len(newest))
	for kind, signal := range newest {
		hashes[kind] = signal.Hash
	}
	return hashes, nil
}
