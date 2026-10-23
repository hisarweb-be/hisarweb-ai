package model

import (
	"errors"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// HwFirstPurchaseDiscountCapTRY is the part of the list price (in TRY) that
// receives the first-purchase discount. The part above it in the same purchase
// is discounted at the standard rate. Another currency uses the same value in
// dollars, converted at the rate of its own order (service.hwDiscountCap).
var HwFirstPurchaseDiscountCapTRY = decimal.NewFromInt(1000)

// Discount rates in percent, and the payable share that follows from them.
// Later purchases get no purchase discount: the token prices themselves are
// already far below the list price (service.HwListPriceDiscountPercent).
const (
	HwFirstPurchasePercent = 50
	HwStandardPercent      = 0

	hwFirstPurchasePayablePercent = 100 - HwFirstPurchasePercent
	hwStandardPayablePercent      = 100 - HwStandardPercent
)

// ErrHwInvalidPurchaseValue rejects a purchase without a positive list value.
var ErrHwInvalidPurchaseValue = errors.New("purchase list value must be positive")

// HwPurchaseQuote is the price breakdown of one purchase, in the currency of
// the purchase. The customer always receives the full list value as credit and
// pays Payable.
type HwPurchaseQuote struct {
	// ListValue is the official list value of the purchase, rounded to cents.
	ListValue decimal.Decimal
	// Payable is what the customer pays.
	Payable decimal.Decimal
	// Discount is the discount given on this purchase.
	Discount decimal.Decimal
	// FirstPurchaseValue is the value portion discounted as first purchase.
	FirstPurchaseValue decimal.Decimal
	// RemainderValue is the value portion discounted at the standard rate.
	RemainderValue decimal.Decimal
}

// HwComputePurchase prices a purchase in one currency. On the first paid
// purchase the value up to discountCap (in that same currency) gets 50 % off and
// everything above it the standard rate; every later purchase gets the standard
// rate. Amounts are rounded to cents, half away from zero.
func HwComputePurchase(listValue decimal.Decimal, firstPurchase bool, discountCap decimal.Decimal) (HwPurchaseQuote, error) {
	if !listValue.IsPositive() {
		return HwPurchaseQuote{}, ErrHwInvalidPurchaseValue
	}
	listValue = listValue.Round(2)
	quote := HwPurchaseQuote{ListValue: listValue}
	if firstPurchase {
		quote.FirstPurchaseValue = decimal.Min(listValue, discountCap)
		quote.RemainderValue = listValue.Sub(quote.FirstPurchaseValue)
	} else {
		quote.FirstPurchaseValue = decimal.Zero
		quote.RemainderValue = listValue
	}
	payable := quote.FirstPurchaseValue.Mul(decimal.NewFromInt(hwFirstPurchasePayablePercent)).
		Add(quote.RemainderValue.Mul(decimal.NewFromInt(hwStandardPayablePercent))).
		Div(decimal.NewFromInt(100))
	quote.Payable = payable.Round(2)
	quote.Discount = listValue.Sub(quote.Payable)
	return quote, nil
}

// HwIsFirstPaidPurchase reports whether the user has no completed paid purchase
// yet. A purchase-sourced credit package is the record of a completed purchase.
//
// It is no longer the source of the first-purchase decision: the order flow calls
// HwClaimFirstPurchaseDiscount, which records the claim at order creation and
// therefore also catches a second account on the same card or phone. This
// function remains for reporting and back-office checks, where the question is
// what was actually paid.
func HwIsFirstPaidPurchase(tx *gorm.DB, userID int) (bool, error) {
	if userID <= 0 {
		return false, ErrHwInvalidCreditGrant
	}
	if tx == nil {
		tx = DB
	}
	var count int64
	if err := tx.Model(&HwCreditPackage{}).
		Where("user_id = ? AND source = ?", userID, HwSourcePurchase).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count == 0, nil
}
