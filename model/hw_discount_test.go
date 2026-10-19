package model

import (
	"strconv"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHwComputePurchase(t *testing.T) {
	cases := []struct {
		name           string
		list           string
		firstPurchase  bool
		payable        string
		firstValue     string
		remainderValue string
		discount       string
	}{
		{name: "first purchase below the cap", list: "600", firstPurchase: true, payable: "300.00", firstValue: "600.00", remainderValue: "0.00", discount: "300.00"},
		{name: "first purchase exactly at the cap", list: "1000", firstPurchase: true, payable: "500.00", firstValue: "1000.00", remainderValue: "0.00", discount: "500.00"},
		{name: "first purchase above the cap", list: "1500", firstPurchase: true, payable: "925.00", firstValue: "1000.00", remainderValue: "500.00", discount: "575.00"},
		{name: "later purchase gets the standard rate", list: "1000", firstPurchase: false, payable: "850.00", firstValue: "0.00", remainderValue: "1000.00", discount: "150.00"},
		{name: "rounds half up", list: "1.3", firstPurchase: false, payable: "1.11", firstValue: "0.00", remainderValue: "1.30", discount: "0.19"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			quote, err := HwComputePurchase(decimal.RequireFromString(tc.list), tc.firstPurchase)
			require.NoError(t, err)
			assert.Equal(t, tc.payable, quote.PayableTRY.StringFixed(2))
			assert.Equal(t, tc.firstValue, quote.FirstPurchaseValueTRY.StringFixed(2))
			assert.Equal(t, tc.remainderValue, quote.RemainderValueTRY.StringFixed(2))
			assert.Equal(t, tc.discount, quote.DiscountTRY.StringFixed(2))
		})
	}
}

func TestHwIsFirstPaidPurchase(t *testing.T) {
	hwCreditTestSetup(t)
	now := int64(1_700_000_000)
	user := createHwTestUser(t, 0)

	first, err := HwIsFirstPaidPurchase(nil, user.Id)
	require.NoError(t, err)
	assert.True(t, first, "a user without packages has not bought yet")

	// Free or affiliate credit is not a purchase.
	_, err = GrantCreditPackage(nil, user.Id, 100, HwSourceFree, "free:"+strconv.Itoa(user.Id), now)
	require.NoError(t, err)
	first, err = HwIsFirstPaidPurchase(nil, user.Id)
	require.NoError(t, err)
	assert.True(t, first)

	_, err = GrantCreditPackage(nil, user.Id, 1000, HwSourcePurchase, "ord-first", now)
	require.NoError(t, err)
	first, err = HwIsFirstPaidPurchase(nil, user.Id)
	require.NoError(t, err)
	assert.False(t, first, "a purchase-sourced package counts as a completed purchase")
}

func TestHwComputePurchaseRejectsNonPositiveValue(t *testing.T) {
	_, err := HwComputePurchase(decimal.Zero, true)
	require.ErrorIs(t, err, ErrHwInvalidPurchaseValue)
	_, err = HwComputePurchase(decimal.RequireFromString("-1"), false)
	require.ErrorIs(t, err, ErrHwInvalidPurchaseValue)
}
