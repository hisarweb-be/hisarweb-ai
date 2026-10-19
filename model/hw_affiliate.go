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
	HwAffiliateNoInviter       = "no_inviter"
	HwAffiliateNotPaidPurchase = "not_a_paid_purchase"
	HwAffiliateSelfInvite      = "self_invite"
	HwAffiliateBuyerInactive   = "buyer_not_active"
	HwAffiliateInviterInactive = "inviter_not_active"
	HwAffiliateSharedSignal    = "shared_"
	HwAffiliateRewardTooSmall  = "reward_too_small"
	HwAffiliateAlreadyGranted  = "already_granted"
	HwAffiliateValueMismatch   = "purchase_value_mismatch"
	HwAffiliateInvalidInput    = "invalid_input"
)

// HwGrantAffiliateReward gives the inviter of a buyer HwAffiliatePercent of the
// list value of a completed paid purchase as a new credit package. It only
// rewards paid purchases: orderRef must reference a purchase-sourced package of
// the buyer. The grant is idempotent on the purchase reference.
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
