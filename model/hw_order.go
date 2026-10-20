package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// HwOrderCurrency is the only currency this top-up flow sells in. Every amount
// on an order is recorded in the smallest unit of it (kuruş for TRY).
const HwOrderCurrency = "TRY"

// Order statuses. Only paid writes credit; everything else closes the order.
// refunded is a post-payment status set by the admin refund route: the credit
// of the order was taken back, so the order is treated like a paid one by every
// close path.
const (
	HwOrderStatusCreated   = "created"
	HwOrderStatusPending   = "pending"
	HwOrderStatusPaid      = "paid"
	HwOrderStatusCancelled = "cancelled"
	HwOrderStatusFailed    = "failed"
	HwOrderStatusExpired   = "expired"
	HwOrderStatusRefunded  = "refunded"
)

// Discount kinds recorded on an order.
const (
	HwDiscountFirstPurchase = "first_purchase"
	HwDiscountReturning     = "returning"
)

// Option keys of the top-up flow. The price rate is delivered by the daily
// pricing job (part D); until it is set, order creation is refused.
const (
	HwTryPerUsdOption         = "HwTryPerUsd"
	HwMinTopUpTryOption       = "HwMinTopUpTry"
	HwMaxTopUpTryOption       = "HwMaxTopUpTry"
	HwMaxOpenOrdersOption     = "HwMaxOpenOrders"
	HwCheckoutReturnURLOption = "HwCheckoutReturnURL"
)

const (
	// hwOrderPendingTTLSeconds is how long an unpaid order may stay open. The
	// reconciliation round in the service layer closes it afterwards and
	// releases a first-purchase claim once the provider order is cancelled.
	hwOrderPendingTTLSeconds = 24 * 3600
)

// ErrHwInvalidOrder rejects an order write that breaks the order invariants.
var ErrHwInvalidOrder = errors.New("invalid top-up order")

// HwOrder is one top-up order. The amounts come from our own database and
// pricing (HwComputePurchase and the TRY/USD rate), never from the client
// request or from the payment webhook.
//
// RevolutOrderId is a pointer so that "not created yet" is NULL: a unique index
// on a non-nullable text column would let only one order exist without an id on
// MySQL.
type HwOrder struct {
	Id                int     `json:"id" gorm:"primaryKey"`
	UserId            int     `json:"user_id" gorm:"not null;index"`
	MerchantRef       string  `json:"merchant_ref" gorm:"type:varchar(64);not null;uniqueIndex"`
	RevolutOrderId    *string `json:"revolut_order_id" gorm:"type:varchar(128);uniqueIndex"`
	ListQuota         int     `json:"list_quota" gorm:"not null"`
	ListValueTryMinor int64   `json:"list_value_try_minor" gorm:"not null"`
	PayableTryMinor   int64   `json:"payable_try_minor" gorm:"not null"`
	Currency          string  `json:"currency" gorm:"type:varchar(3);not null"`
	DiscountKind      string  `json:"discount_kind" gorm:"type:varchar(32);not null"`
	Status            string  `json:"status" gorm:"type:varchar(16);not null;index"`
	CreatedAt         int64   `json:"created_at" gorm:"not null;default:0"`
	PaidAt            int64   `json:"paid_at" gorm:"not null;default:0"`
	// AdminNote records why an order was settled outside the normal rules (a
	// late payment without its discount, a provider report that disagreed) or
	// what a refund left owed. It is operational text for the admin only; the
	// user-facing API never returns it.
	AdminNote string `json:"admin_note" gorm:"type:varchar(512);not null;default:''"`
}

func (HwOrder) TableName() string {
	return "hw_orders"
}

func hwOrderIsOpen(status string) bool {
	return status == HwOrderStatusCreated || status == HwOrderStatusPending
}

func hwOrderCloseStatus(status string) bool {
	switch status {
	case HwOrderStatusCancelled, HwOrderStatusFailed, HwOrderStatusExpired:
		return true
	default:
		return false
	}
}

// HwCreateOrder stores a new order before the payment provider is called, so an
// arriving webhook always finds it. The reference is ours and unique.
func HwCreateOrder(order *HwOrder) error {
	if order == nil || order.UserId <= 0 || strings.TrimSpace(order.MerchantRef) == "" ||
		order.ListQuota <= 0 || order.ListValueTryMinor <= 0 || order.PayableTryMinor <= 0 ||
		order.Currency != HwOrderCurrency || order.CreatedAt <= 0 || !hwOrderIsOpen(order.Status) {
		return ErrHwInvalidOrder
	}
	if order.DiscountKind != HwDiscountFirstPurchase && order.DiscountKind != HwDiscountReturning {
		return ErrHwInvalidOrder
	}
	order.PaidAt = 0
	return DB.Create(order).Error
}

// HwCountOpenOrders counts the orders of a user that still wait for payment.
func HwCountOpenOrders(userID int) (int64, error) {
	if userID <= 0 {
		return 0, ErrHwInvalidOrder
	}
	var count int64
	err := DB.Model(&HwOrder{}).
		Where("user_id = ? AND status IN ?", userID, []string{HwOrderStatusCreated, HwOrderStatusPending}).
		Count(&count).Error
	return count, err
}

// HwFindOrderForWebhook finds the order a webhook event belongs to. The event's
// own reference wins; the provider order id is the fallback. found is false when
// neither matches, which the caller answers without revealing anything.
func HwFindOrderForWebhook(merchantRef, revolutOrderID string) (*HwOrder, bool, error) {
	merchantRef = strings.TrimSpace(merchantRef)
	revolutOrderID = strings.TrimSpace(revolutOrderID)
	var order HwOrder
	var err error
	if merchantRef != "" {
		err = DB.Where("merchant_ref = ?", merchantRef).First(&order).Error
	} else if revolutOrderID != "" {
		err = DB.Where("revolut_order_id = ?", revolutOrderID).First(&order).Error
	} else {
		return nil, false, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &order, true, nil
}

// HwAttachRevolutOrder records the provider order id of a freshly created
// checkout and moves the order to pending.
func HwAttachRevolutOrder(orderID int, revolutOrderID string, now int64) error {
	revolutOrderID = strings.TrimSpace(revolutOrderID)
	if orderID <= 0 || revolutOrderID == "" || now <= 0 {
		return ErrHwInvalidOrder
	}
	return DB.Model(&HwOrder{}).
		Where("id = ? AND status = ?", orderID, HwOrderStatusCreated).
		UpdateColumns(map[string]any{"revolut_order_id": revolutOrderID, "status": HwOrderStatusPending}).Error
}

// HwOrderSettlement reports what a completed payment did to the ledger.
type HwOrderSettlement struct {
	// Paid is true when this call moved the order to paid.
	Paid bool
	// Granted is true when this call also issued the credit package.
	Granted bool
	UserId  int
	// ListQuota is the full list value of the purchase in quota units.
	ListQuota int
	// GrantedQuota is the quota this call actually issued; it is ListQuota when
	// the discount was applied and the face value of the paid amount when it
	// was not.
	GrantedQuota int
	// DiscountApplied is true when the first-purchase discount was still held
	// by this order when the payment was settled.
	DiscountApplied bool
	// ForfeitedFreeQuota is the remaining free trial credit this purchase
	// retired because the account was frozen; paid credit is never part of it.
	ForfeitedFreeQuota int
	// AdminNote is the admin-visible reason recorded on the order when the
	// discount was not applied; empty when it was.
	AdminNote string
}

// hwStandardCreditQuota converts what the customer actually paid into quota at
// face value: no discount, never more than the paid amount is worth. It is used
// when a discounted order is paid after its first-purchase claim was released,
// so the same discount can never be spent twice.
func hwStandardCreditQuota(order *HwOrder) (int, error) {
	if order.ListValueTryMinor <= 0 || order.PayableTryMinor <= 0 || order.ListQuota <= 0 {
		return 0, ErrHwInvalidOrder
	}
	credit := decimal.NewFromInt(order.PayableTryMinor).
		Mul(decimal.NewFromInt(int64(order.ListQuota))).
		Div(decimal.NewFromInt(order.ListValueTryMinor))
	quota, err := common.WalletQuotaFromDecimalStrict(credit)
	if err != nil {
		return 0, err
	}
	if quota <= 0 {
		return 0, ErrHwInvalidOrder
	}
	return quota, nil
}

// HwSettleOrderPayment marks an order paid and issues its credit package in one
// transaction, so the credit and the order status cannot disagree. A second call
// for the same order changes nothing and reports Paid false, which makes a
// duplicated webhook harmless. A payment that arrives after the order was closed
// locally (cancelled, failed or expired) is still credited: the customer paid,
// and refusing the credit would take money without delivering it.
//
// A discounted order only keeps its discount when, inside this same transaction,
// the first-purchase claim still belongs to this order and no other discounted
// purchase of the user was paid before it. Otherwise the discount was already
// spent elsewhere and only the face value of the paid amount is credited; the
// order records that decision in its admin note.
//
// cardFingerprint is the provider's card fingerprint of the paying card, not a
// card number. It is recorded on the first-purchase claim of this order and
// hashed like every other signal; a failure to record it does not cost the
// customer the credit.
func HwSettleOrderPayment(merchantRef, revolutOrderID, cardFingerprint string, now int64) (HwOrderSettlement, error) {
	if strings.TrimSpace(merchantRef) == "" || now <= 0 {
		return HwOrderSettlement{}, ErrHwInvalidOrder
	}
	var settlement HwOrderSettlement
	err := DB.Transaction(func(tx *gorm.DB) error {
		var order HwOrder
		if err := lockForUpdate(tx).Where("merchant_ref = ?", merchantRef).First(&order).Error; err != nil {
			return err
		}
		settlement.UserId = order.UserId
		settlement.ListQuota = order.ListQuota
		if order.Status == HwOrderStatusPaid || order.Status == HwOrderStatusRefunded || order.Currency != HwOrderCurrency {
			return nil
		}
		creditQuota := order.ListQuota
		if order.DiscountKind == HwDiscountFirstPurchase {
			claimHeld := false
			var claim HwFirstPurchaseClaim
			err := tx.Where("user_id = ?", order.UserId).First(&claim).Error
			switch {
			case err == nil:
				claimHeld = claim.OrderRef == order.MerchantRef
			case errors.Is(err, gorm.ErrRecordNotFound):
			default:
				return err
			}
			var discountedPurchases int64
			if err := tx.Model(&HwOrder{}).
				Where("user_id = ? AND status = ? AND discount_kind = ? AND id <> ?",
					order.UserId, HwOrderStatusPaid, HwDiscountFirstPurchase, order.Id).
				Count(&discountedPurchases).Error; err != nil {
				return err
			}
			if !claimHeld || discountedPurchases > 0 {
				// The discount of this order was released (or spent on another
				// order) before the payment arrived. Credit only what was paid
				// at face value and record why, so the discount is never given
				// twice for one first purchase.
				quota, err := hwStandardCreditQuota(&order)
				if err != nil {
					return err
				}
				creditQuota = quota
				settlement.AdminNote = "first-purchase discount was no longer held by this order; credited the paid amount at face value"
			} else {
				settlement.DiscountApplied = true
			}
		}
		granted, forfeited, err := GrantCreditPackageInTx(tx, order.UserId, creditQuota, HwSourcePurchase, order.MerchantRef, now)
		if err != nil {
			return err
		}
		settlement.ForfeitedFreeQuota = forfeited
		updates := map[string]any{"status": HwOrderStatusPaid, "paid_at": now, "admin_note": settlement.AdminNote}
		if !settlement.DiscountApplied && order.DiscountKind == HwDiscountFirstPurchase {
			// A paid first-purchase order means the discount was really spent;
			// this one was not, and the kind must say so for later checks.
			updates["discount_kind"] = HwDiscountReturning
		}
		if id := strings.TrimSpace(revolutOrderID); id != "" && (order.RevolutOrderId == nil || *order.RevolutOrderId == "") {
			updates["revolut_order_id"] = id
		}
		if err := tx.Model(&HwOrder{}).Where("id = ?", order.Id).UpdateColumns(updates).Error; err != nil {
			return err
		}
		settlement.Paid = true
		settlement.Granted = granted
		settlement.GrantedQuota = creditQuota
		if settlement.DiscountApplied && strings.TrimSpace(cardFingerprint) != "" {
			if err := HwRecordFirstPurchaseCard(tx, order.UserId, order.MerchantRef, cardFingerprint); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				common.SysError(common.LogText("could not record the paying card of first-purchase order %s (class %s); the credit itself is unaffected", order.MerchantRef, hwErrorClass(err)))
			}
		}
		return nil
	})
	return settlement, err
}

// HwMarkOrderNotPaid closes an unpaid order as cancelled, failed or expired and
// releases a first-purchase claim it holds. An order that is already paid or
// refunded is left alone. A second call for the same status changes nothing.
func HwMarkOrderNotPaid(merchantRef, status string, now int64) (bool, error) {
	merchantRef = strings.TrimSpace(merchantRef)
	if merchantRef == "" || now <= 0 || !hwOrderCloseStatus(status) {
		return false, ErrHwInvalidOrder
	}
	changed := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var order HwOrder
		err := lockForUpdate(tx).Where("merchant_ref = ?", merchantRef).First(&order).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if order.Status == HwOrderStatusPaid || order.Status == HwOrderStatusRefunded {
			return nil
		}
		if order.Status == status {
			// A repeated close event still releases a claim a crash may have left.
			return HwReleaseFirstPurchaseClaim(tx, order.UserId, order.MerchantRef)
		}
		if err := tx.Model(&HwOrder{}).Where("id = ?", order.Id).UpdateColumns(map[string]any{"status": status}).Error; err != nil {
			return err
		}
		changed = true
		return HwReleaseFirstPurchaseClaim(tx, order.UserId, order.MerchantRef)
	})
	return changed, err
}

// HwOpenOrdersPastTTL lists the unpaid orders older than the pending TTL, in id
// order. The caller (the reconciliation round in the service layer) checks each
// of them with the payment provider before closing it, so a paid order is never
// expired away and a claim is only released after the provider order was
// cancelled. Orders that already carry an admin note were flagged by an earlier
// round for an admin decision and are left out until that note is cleared.
func HwOpenOrdersPastTTL(now int64) ([]HwOrder, error) {
	if now <= 0 {
		return nil, ErrHwInvalidOrder
	}
	var orders []HwOrder
	err := DB.Where("status IN ? AND created_at <= ? AND admin_note = ''",
		[]string{HwOrderStatusCreated, HwOrderStatusPending}, now-hwOrderPendingTTLSeconds).
		Order("id").Find(&orders).Error
	if err != nil {
		return nil, err
	}
	return orders, nil
}

// HwNoteOrder records an admin-visible note on an unpaid order and leaves the
// order open, so neither the reconciliation round nor a close path touches it
// until an admin has looked at it.
func HwNoteOrder(merchantRef, note string) error {
	merchantRef = strings.TrimSpace(merchantRef)
	if merchantRef == "" || strings.TrimSpace(note) == "" {
		return ErrHwInvalidOrder
	}
	return DB.Model(&HwOrder{}).
		Where("merchant_ref = ? AND status IN ?", merchantRef, []string{HwOrderStatusCreated, HwOrderStatusPending}).
		Update("admin_note", note).Error
}

// HwOrderRevocation reports what taking back an order's credit did.
type HwOrderRevocation struct {
	// Retired is true when this call retired the purchase package.
	Retired bool
	// GrantedQuota is what the purchase package originally issued.
	GrantedQuota int
	// RemainingQuota is what the package still held and what was subtracted
	// from the balance; the difference to GrantedQuota was already spent.
	RemainingQuota int
}

// HwRevokeOrderCredit takes back the credit package of one paid order, for the
// admin refund route. Only what the package still holds is subtracted from
// users.quota (never below zero); the package is retired with that remainder, so
// a second call changes nothing and reports Retired false. Balance the customer
// already spent is not recovered from other packages; the caller records it as
// the owed part of the refund.
func HwRevokeOrderCredit(merchantRef string, now int64) (HwOrderRevocation, error) {
	merchantRef = strings.TrimSpace(merchantRef)
	if merchantRef == "" || now <= 0 {
		return HwOrderRevocation{}, ErrHwInvalidOrder
	}
	var pkg HwCreditPackage
	err := DB.Where("source = ? AND order_ref = ?", HwSourcePurchase, merchantRef).First(&pkg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return HwOrderRevocation{}, nil
	}
	if err != nil {
		return HwOrderRevocation{}, err
	}
	revocation := HwOrderRevocation{GrantedQuota: pkg.GrantedQuota}
	if pkg.RetiredAt != 0 {
		revocation.RemainingQuota = pkg.RetiredRemaining
		return revocation, nil
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
			return nil
		}
		revocation.Retired = true
		revocation.RemainingQuota = remaining
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
		return HwOrderRevocation{}, err
	}
	if revoked {
		if err := cacheDecrUserQuota(pkg.UserId, int64(decrease)); err != nil {
			common.SysError(common.LogText("failed to decrease user quota cache after order revocation: %s", err.Error()))
		}
	}
	return revocation, nil
}

// HwMarkOrderRefunded moves a paid order to refunded and stores the admin note of
// the refund. Only a paid order can be refunded; a second call changes nothing.
func HwMarkOrderRefunded(merchantRef, note string, now int64) (bool, error) {
	merchantRef = strings.TrimSpace(merchantRef)
	if merchantRef == "" || now <= 0 {
		return false, ErrHwInvalidOrder
	}
	changed := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var order HwOrder
		err := lockForUpdate(tx).Where("merchant_ref = ?", merchantRef).First(&order).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if order.Status != HwOrderStatusPaid {
			return nil
		}
		if err := tx.Model(&HwOrder{}).Where("id = ?", order.Id).
			UpdateColumns(map[string]any{"status": HwOrderStatusRefunded, "admin_note": note, "paid_at": order.PaidAt}).Error; err != nil {
			return err
		}
		changed = true
		return nil
	})
	return changed, err
}

// The expiry job itself lives in the service layer (service.StartHwOrderExpiryJob):
// closing an unpaid order must be reconciled with the payment provider first,
// and the provider client is not visible from the model layer.
