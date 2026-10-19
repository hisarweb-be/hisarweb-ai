package model

import (
	"errors"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// hwFirstPurchaseDiscountCapTRY is the part of the list price (in TRY) that
// receives the first-purchase discount. The part above it in the same purchase
// is discounted at the standard rate.
var hwFirstPurchaseDiscountCapTRY = decimal.NewFromInt(1000)

// Discount rates in percent, and the payable share that follows from them.
const (
	HwFirstPurchasePercent = 50
	HwStandardPercent      = 15

	hwFirstPurchasePayablePercent = 100 - HwFirstPurchasePercent
	hwStandardPayablePercent      = 100 - HwStandardPercent
)

// ErrHwInvalidPurchaseValue rejects a purchase without a positive list value.
var ErrHwInvalidPurchaseValue = errors.New("purchase list value must be positive")

// HwPurchaseQuote is the price breakdown of one purchase. The customer always
// receives the full list value as credit and pays PayableTRY.
type HwPurchaseQuote struct {
	// ListValueTRY is the official list value of the purchase, rounded to cents.
	ListValueTRY decimal.Decimal
	// PayableTRY is what the customer pays.
	PayableTRY decimal.Decimal
	// DiscountTRY is the discount given on this purchase.
	DiscountTRY decimal.Decimal
	// FirstPurchaseValueTRY is the value portion discounted as first purchase.
	FirstPurchaseValueTRY decimal.Decimal
	// RemainderValueTRY is the value portion discounted at the standard rate.
	RemainderValueTRY decimal.Decimal
}

// HwComputePurchase prices a purchase. On the first paid purchase the value up
// to HwFirstPurchaseDiscountCapTRY gets 50 % off and everything above it 15 %;
// every later purchase gets 15 % off. Amounts are rounded to cents, half away
// from zero.
func HwComputePurchase(listValueTRY decimal.Decimal, firstPurchase bool) (HwPurchaseQuote, error) {
	if !listValueTRY.IsPositive() {
		return HwPurchaseQuote{}, ErrHwInvalidPurchaseValue
	}
	listValue := listValueTRY.Round(2)
	quote := HwPurchaseQuote{ListValueTRY: listValue}
	if firstPurchase {
		quote.FirstPurchaseValueTRY = decimal.Min(listValue, hwFirstPurchaseDiscountCapTRY)
		quote.RemainderValueTRY = listValue.Sub(quote.FirstPurchaseValueTRY)
	} else {
		quote.FirstPurchaseValueTRY = decimal.Zero
		quote.RemainderValueTRY = listValue
	}
	payable := quote.FirstPurchaseValueTRY.Mul(decimal.NewFromInt(hwFirstPurchasePayablePercent)).
		Add(quote.RemainderValueTRY.Mul(decimal.NewFromInt(hwStandardPayablePercent))).
		Div(decimal.NewFromInt(100))
	quote.PayableTRY = payable.Round(2)
	quote.DiscountTRY = listValue.Sub(quote.PayableTRY)
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
