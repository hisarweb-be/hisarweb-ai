package model

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// HwOrderCurrency is the only currency this top-up flow sells in. Every amount
// on an order is recorded in the smallest unit of it (kuruş for TRY).
const HwOrderCurrency = "TRY"

// Order statuses. Only paid writes credit; everything else closes the order.
const (
	HwOrderStatusCreated   = "created"
	HwOrderStatusPending   = "pending"
	HwOrderStatusPaid      = "paid"
	HwOrderStatusCancelled = "cancelled"
	HwOrderStatusFailed    = "failed"
	HwOrderStatusExpired   = "expired"
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
	// expiry job closes it afterwards and releases a first-purchase claim.
	hwOrderPendingTTLSeconds = 24 * 3600
	hwOrderExpiryJobInterval = 10 * time.Minute
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
}

// HwSettleOrderPayment marks an order paid and issues its credit package in one
// transaction, so the credit and the order status cannot disagree. A second call
// for the same order changes nothing and reports Paid false, which makes a
// duplicated webhook harmless. A payment that arrives after the order was closed
// locally (cancelled, failed or expired) is still credited: the customer paid,
// and refusing the credit would take money without delivering it.
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
		if order.Status == HwOrderStatusPaid || order.Currency != HwOrderCurrency {
			return nil
		}
		granted, err := GrantCreditPackage(tx, order.UserId, order.ListQuota, HwSourcePurchase, order.MerchantRef, now)
		if err != nil {
			return err
		}
		updates := map[string]any{"status": HwOrderStatusPaid, "paid_at": now}
		if id := strings.TrimSpace(revolutOrderID); id != "" && (order.RevolutOrderId == nil || *order.RevolutOrderId == "") {
			updates["revolut_order_id"] = id
		}
		if err := tx.Model(&HwOrder{}).Where("id = ?", order.Id).UpdateColumns(updates).Error; err != nil {
			return err
		}
		settlement.Paid = true
		settlement.Granted = granted
		if order.DiscountKind == HwDiscountFirstPurchase && strings.TrimSpace(cardFingerprint) != "" {
			if err := HwRecordFirstPurchaseCard(tx, order.UserId, order.MerchantRef, cardFingerprint); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				common.SysError(common.LogText("could not record the paying card of first-purchase order %s (class %s); the credit itself is unaffected", order.MerchantRef, hwErrorClass(err)))
			}
		}
		return nil
	})
	return settlement, err
}

// HwMarkOrderNotPaid closes an unpaid order as cancelled, failed or expired and
// releases a first-purchase claim it holds. An order that is already paid is
// left alone. A second call for the same status changes nothing.
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
		if order.Status == HwOrderStatusPaid {
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

// HwExpirePendingOrders closes every unpaid order older than the pending TTL and
// releases the first-purchase claims those orders hold. It returns how many
// orders it closed; a second run changes nothing.
func HwExpirePendingOrders(now int64) (int, error) {
	if now <= 0 {
		return 0, ErrHwInvalidOrder
	}
	var ids []int
	err := DB.Model(&HwOrder{}).
		Where("status IN ? AND created_at <= ?", []string{HwOrderStatusCreated, HwOrderStatusPending}, now-hwOrderPendingTTLSeconds).
		Pluck("id", &ids).Error
	if err != nil {
		return 0, err
	}
	closed := 0
	for _, id := range ids {
		changed, err := hwExpireOrder(id, now)
		if err != nil {
			common.SysError(common.LogText("expiring top-up order %d failed (class %s); continuing with the next order", id, hwErrorClass(err)))
			continue
		}
		if changed {
			closed++
		}
	}
	return closed, nil
}

func hwExpireOrder(orderID int, now int64) (bool, error) {
	changed := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var order HwOrder
		err := lockForUpdate(tx).Where("id = ?", orderID).First(&order).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if !hwOrderIsOpen(order.Status) || order.CreatedAt > now-hwOrderPendingTTLSeconds {
			return nil
		}
		if err := tx.Model(&HwOrder{}).Where("id = ?", order.Id).UpdateColumns(map[string]any{"status": HwOrderStatusExpired}).Error; err != nil {
			return err
		}
		changed = true
		return HwReleaseFirstPurchaseClaim(tx, order.UserId, order.MerchantRef)
	})
	return changed, err
}

// HwRevokeOrderCredit takes back the credit package of one paid order, for a
// refund or a chargeback. Only what the package still holds is subtracted from
// users.quota (never below zero); the package is retired with that remainder, so
// a second call changes nothing. Balance the customer already spent is not
// recovered from other packages.
func HwRevokeOrderCredit(merchantRef string, now int64) error {
	merchantRef = strings.TrimSpace(merchantRef)
	if merchantRef == "" || now <= 0 {
		return ErrHwInvalidOrder
	}
	var pkg HwCreditPackage
	err := DB.Where("source = ? AND order_ref = ?", HwSourcePurchase, merchantRef).First(&pkg).Error
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
			common.SysError(common.LogText("failed to decrease user quota cache after order revocation: %s", err.Error()))
		}
	}
	return nil
}

var hwOrderExpiryOnce sync.Once

// StartHwOrderExpiryJob starts the job that expires unpaid orders. It runs once
// per process and only on the master node; HwExpirePendingOrders re-checks each
// order inside its own transaction, so several masters are harmless.
func StartHwOrderExpiryJob() {
	hwOrderExpiryOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		go func() {
			common.SysLog(common.LogText("top-up order expiry job started: interval=%s", hwOrderExpiryJobInterval))
			runHwOrderExpiryOnceSafely()
			ticker := time.NewTicker(hwOrderExpiryJobInterval)
			defer ticker.Stop()
			for range ticker.C {
				runHwOrderExpiryOnceSafely()
			}
		}()
	})
}

func runHwOrderExpiryOnceSafely() {
	defer func() {
		if r := recover(); r != nil {
			common.SysError(common.LogText("top-up order expiry job panicked: %v", r))
		}
	}()
	closed, err := HwExpirePendingOrders(common.GetTimestamp())
	if err != nil {
		common.SysError(common.LogText("top-up order expiry failed (class %s)", hwErrorClass(err)))
		return
	}
	if closed > 0 {
		common.SysLog(common.LogText("top-up order expiry closed %d unpaid orders", closed))
	}
}
