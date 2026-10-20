package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// hwAffiliateRetryMaxAttempts bounds how often the retry queue re-attempts one
// reward. The queue is picked up every reconciliation round (10 minutes), so
// this is roughly an hour of retries before the loss is escalated to the log.
const hwAffiliateRetryMaxAttempts = 6

// HwAffiliateRetry is the retry queue of affiliate rewards whose grant failed
// with a storage error after a payment was already settled. Losing the reward
// silently is not acceptable, so the failed grant is recorded here, unique per
// purchase reference, and the reconciliation round retries it.
type HwAffiliateRetry struct {
	Id        int    `json:"id" gorm:"primaryKey"`
	OrderRef  string `json:"order_ref" gorm:"type:varchar(128);not null;uniqueIndex"`
	BuyerId   int    `json:"buyer_id" gorm:"not null"`
	ListQuota int    `json:"list_quota" gorm:"not null"`
	Attempts  int    `json:"attempts" gorm:"not null;default:0"`
	CreatedAt int64  `json:"created_at" gorm:"not null;default:0"`
}

func (HwAffiliateRetry) TableName() string {
	return "hw_affiliate_retries"
}

// HwEnqueueAffiliateRetry records a reward grant that failed after its payment
// was settled, so the reconciliation round retries it. The unique index on the
// purchase reference makes a second enqueue for the same purchase a quiet
// no-op.
func HwEnqueueAffiliateRetry(buyerID int, listQuota int, orderRef string, now int64) (bool, error) {
	if buyerID <= 0 || listQuota <= 0 || orderRef == "" || now <= 0 {
		return false, ErrHwInvalidCreditGrant
	}
	retry := HwAffiliateRetry{OrderRef: orderRef, BuyerId: buyerID, ListQuota: listQuota, CreatedAt: now}
	result := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "order_ref"}},
		DoNothing: true,
	}).Create(&retry)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// HwRetryPendingAffiliateRewards works the retry queue. A grant that succeeded
// or was refused for a permanent reason removes its row; a storage failure
// counts one attempt, and a row whose attempts run out is dropped with an error
// log so the loss is visible to the operator. Only counts are logged, never
// values: an order reference or a quota never reaches the log.
func HwRetryPendingAffiliateRewards(now int64) (granted int, dropped int, err error) {
	if now <= 0 {
		return 0, 0, ErrHwInvalidCreditGrant
	}
	var rows []HwAffiliateRetry
	if err := DB.Order("id").Find(&rows).Error; err != nil {
		return 0, 0, err
	}
	var firstErr error
	for _, row := range rows {
		ok, reason, grantErr := HwGrantAffiliateReward(row.BuyerId, row.ListQuota, row.OrderRef, now)
		switch {
		case grantErr == nil:
			// granted, or a permanent refusal reason: either way this row is
			// done and must not be retried again.
			if ok {
				granted++
			} else if reason == HwAffiliateNotPaidPurchase {
				// The purchase package is gone (for example a refund took the
				// order back); nothing left to reward.
			}
			if err := DB.Where("id = ?", row.Id).Delete(&HwAffiliateRetry{}).Error; err != nil && firstErr == nil {
				firstErr = err
			}
		case row.Attempts+1 >= hwAffiliateRetryMaxAttempts:
			dropped++
			common.SysError(common.LogText("affiliate reward retry gave up after %d attempts (class %s)", row.Attempts+1, hwErrorClass(grantErr)))
			if err := DB.Where("id = ?", row.Id).Delete(&HwAffiliateRetry{}).Error; err != nil && firstErr == nil {
				firstErr = err
			}
		default:
			if err := DB.Model(&HwAffiliateRetry{}).Where("id = ?", row.Id).
				Update("attempts", row.Attempts+1).Error; err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return granted, dropped, firstErr
}
