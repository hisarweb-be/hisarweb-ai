package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// HwAffiliatePercent is the share of a paid purchase's list value that the
// inviter receives as its own credit package.
const HwAffiliatePercent = 5

// Affiliate reward rejection reasons returned by HwGrantAffiliateReward.
const (
	HwAffiliateNoInviter         = "no_inviter"
	HwAffiliateNotPaidPurchase   = "not_a_paid_purchase"
	HwAffiliateSelfInvite        = "self_invite"
	HwAffiliateBuyerInactive     = "buyer_not_active"
	HwAffiliateInviterInactive   = "inviter_not_active"
	HwAffiliateSharedSignal      = "shared_"
	HwAffiliateUnverifiedSignals = "unverified_signals"
	HwAffiliateRewardTooSmall    = "reward_too_small"
	HwAffiliateAlreadyGranted    = "already_granted"
	HwAffiliateValueMismatch     = "purchase_value_mismatch"
	HwAffiliateInvalidInput      = "invalid_input"
)

// HwGrantAffiliateReward gives the inviter of a buyer HwAffiliatePercent of the
// list value of a completed paid purchase as a new credit package. It only
// rewards paid purchases: orderRef must reference a purchase-sourced package of
// the buyer. Both the buyer and the inviter need a verified device or phone
// signal, and no signal kind may be shared between them. The grant is idempotent
// on the purchase reference.
//
// granted is false with a reason when nothing was (or should be) granted; err
// is reserved for storage failures and invalid input.
func HwGrantAffiliateReward(buyerID int, purchaseListQuota int, orderRef string, now int64) (granted bool, reason string, err error) {
	if buyerID <= 0 || orderRef == "" || now <= 0 || purchaseListQuota <= 0 {
		return false, HwAffiliateInvalidInput, ErrHwInvalidCreditGrant
	}
	var buyer User
	if err := DB.Select("id", "inviter_id", "status").Where("id = ?", buyerID).First(&buyer).Error; err != nil {
		return false, "", err
	}
	var purchase HwCreditPackage
	err = DB.Where("user_id = ? AND source = ? AND order_ref = ?", buyerID, HwSourcePurchase, orderRef).
		First(&purchase).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, HwAffiliateNotPaidPurchase, nil
	}
	if err != nil {
		return false, "", err
	}
	// The ledger decides how much was bought; a caller cannot inflate the reward
	// by passing a larger value than the purchase granted.
	if purchase.GrantedQuota != purchaseListQuota {
		return false, HwAffiliateValueMismatch, nil
	}
	if buyer.InviterId <= 0 {
		return false, HwAffiliateNoInviter, nil
	}
	if buyer.InviterId == buyerID {
		return false, HwAffiliateSelfInvite, nil
	}
	if buyer.Status != common.UserStatusEnabled {
		return false, HwAffiliateBuyerInactive, nil
	}
	var inviter User
	err = DB.Select("id", "status").Where("id = ?", buyer.InviterId).First(&inviter).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, HwAffiliateNoInviter, nil
	}
	if err != nil {
		return false, "", err
	}
	if inviter.Status != common.UserStatusEnabled {
		return false, HwAffiliateInviterInactive, nil
	}
	// Both sides need at least one device or phone signal. Without it the shared
	// signal check would compare two empty records and the reward could be farmed
	// with accounts that never verified anything.
	buyerVerified, err := hwUserHasVerifiedSignal(buyerID)
	if err != nil {
		return false, "", err
	}
	if !buyerVerified {
		return false, HwAffiliateUnverifiedSignals, nil
	}
	inviterVerified, err := hwUserHasVerifiedSignal(inviter.Id)
	if err != nil {
		return false, "", err
	}
	if !inviterVerified {
		return false, HwAffiliateUnverifiedSignals, nil
	}
	kinds, err := hwSharedSignalKinds(buyerID, inviter.Id)
	if err != nil {
		return false, "", err
	}
	if len(kinds) > 0 {
		return false, HwAffiliateSharedSignal + kinds[0], nil
	}
	reward := common.QuotaRound(float64(purchaseListQuota) * HwAffiliatePercent / 100)
	if reward <= 0 {
		return false, HwAffiliateRewardTooSmall, nil
	}
	granted, err = GrantCreditPackage(nil, inviter.Id, reward, HwSourceAffiliate, "aff:"+orderRef, now)
	if err != nil {
		return false, "", err
	}
	if !granted {
		return false, HwAffiliateAlreadyGranted, nil
	}
	return true, "", nil
}

// HwRevokeAffiliateReward takes back the affiliate package of one purchase
// ("aff:<orderRef>"), for example when that purchase is refunded or reversed.
//
// Only the part the package still holds, derived from the inviter's balance the
// same way the expiry job derives it, is subtracted from users.quota (never
// below zero); the package is retired with that remainder so it cannot be
// revoked twice. A reference without an affiliate package is not an error, and a
// second call changes nothing.
func HwRevokeAffiliateReward(orderRef string, now int64) error {
	if orderRef == "" || now <= 0 {
		return ErrHwInvalidCreditGrant
	}
	var pkg HwCreditPackage
	err := DB.Where("source = ? AND order_ref = ?", HwSourceAffiliate, "aff:"+orderRef).First(&pkg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if pkg.RetiredAt != 0 {
		return nil
	}
	decrease := 0
	revoked := false
	err = DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Select("id", "quota").Where("id = ?", pkg.UserId).First(&user).Error; err != nil {
			return err
		}
		var active []HwCreditPackage
		if err := tx.Where("user_id = ? AND retired_at = 0", pkg.UserId).Find(&active).Error; err != nil {
			return err
		}
		remaining := 0
		for _, view := range HwPackageRemaining(pkg.UserId, user.Quota, active) {
			if view.Id == pkg.Id {
				remaining = view.RemainingQuota
				break
			}
		}
		result := tx.Model(&HwCreditPackage{}).
			Where("id = ? AND retired_at = 0", pkg.Id).
			UpdateColumns(map[string]any{"retired_at": now, "retired_remaining": remaining})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			// Another call revoked it between the read and this transaction.
			return nil
		}
		decrease = min(remaining, max(user.Quota, 0))
		if decrease == 0 {
			return nil
		}
		if err := hwDecreaseUserQuota(tx, pkg.UserId, decrease); err != nil {
			return err
		}
		revoked = true
		return nil
	})
	if err != nil {
		return err
	}
	if revoked {
		if err := cacheDecrUserQuota(pkg.UserId, int64(decrease)); err != nil {
			common.SysError(common.LogText("failed to decrease user quota cache after affiliate revocation: %s", err.Error()))
		}
	}
	return nil
}
