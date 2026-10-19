package model

import (
	"strconv"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

func TestHwClaimFirstPurchaseDiscount(t *testing.T) {
	now := int64(1_700_000_000)

	t.Run("claims once and answers the same order the same way", func(t *testing.T) {
		hwCreditTestSetup(t)
		user := createHwTestUser(t, 0)

		eligible, reason, err := HwClaimFirstPurchaseDiscount(nil, user.Id, "ord-1", now)
		require.NoError(t, err)
		assert.True(t, eligible, "reason: %s", reason)
		assert.Empty(t, reason)

		eligible, reason, err = HwClaimFirstPurchaseDiscount(nil, user.Id, "ord-1", now)
		require.NoError(t, err)
		assert.True(t, eligible, "the same order keeps its answer")
		assert.Empty(t, reason)

		eligible, reason, err = HwClaimFirstPurchaseDiscount(nil, user.Id, "ord-2", now)
		require.NoError(t, err)
		assert.False(t, eligible)
		assert.Equal(t, HwFirstPurchaseAlreadyClaimed, reason)

		var count int64
		require.NoError(t, DB.Model(&HwFirstPurchaseClaim{}).Where("user_id = ?", user.Id).Count(&count).Error)
		assert.Equal(t, int64(1), count)
	})

	t.Run("runs inside the caller's transaction", func(t *testing.T) {
		hwCreditTestSetup(t)
		user := createHwTestUser(t, 0)

		var eligible bool
		require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
			var reason string
			var err error
			eligible, reason, err = HwClaimFirstPurchaseDiscount(tx, user.Id, "ord-tx", now)
			require.Empty(t, reason)
			return err
		}))
		assert.True(t, eligible)
	})

	t.Run("releasing gives the discount back for another order", func(t *testing.T) {
		hwCreditTestSetup(t)
		user := createHwTestUser(t, 0)
		_, _, err := HwClaimFirstPurchaseDiscount(nil, user.Id, "ord-1", now)
		require.NoError(t, err)

		require.NoError(t, HwReleaseFirstPurchaseClaim(nil, user.Id, "ord-other"))
		eligible, reason, err := HwClaimFirstPurchaseDiscount(nil, user.Id, "ord-2", now)
		require.NoError(t, err)
		assert.False(t, eligible, "another order's release changes nothing (reason: %s)", reason)

		require.NoError(t, HwReleaseFirstPurchaseClaim(nil, user.Id, "ord-1"))
		require.NoError(t, HwReleaseFirstPurchaseClaim(nil, user.Id, "ord-1"), "releasing twice changes nothing")
		eligible, reason, err = HwClaimFirstPurchaseDiscount(nil, user.Id, "ord-2", now)
		require.NoError(t, err)
		assert.True(t, eligible, "reason: %s", reason)
	})

	t.Run("refuses a phone another claim already used", func(t *testing.T) {
		hwCreditTestSetup(t)
		first := createHwTestUser(t, 0)
		second := createHwTestUser(t, 0)
		require.NoError(t, UpsertHwUserSignals(first.Id, HwUserSignals{Phone: "0555 123 45 67"}))
		require.NoError(t, UpsertHwUserSignals(second.Id, HwUserSignals{Phone: "+90 555 123 45 67"}))

		eligible, _, err := HwClaimFirstPurchaseDiscount(nil, first.Id, "ord-a", now)
		require.NoError(t, err)
		require.True(t, eligible)

		eligible, reason, err := HwClaimFirstPurchaseDiscount(nil, second.Id, "ord-b", now)
		require.NoError(t, err)
		assert.False(t, eligible)
		assert.Equal(t, HwFirstPurchaseSharedPhone, reason)

		var count int64
		require.NoError(t, DB.Model(&HwFirstPurchaseClaim{}).Where("user_id = ?", second.Id).Count(&count).Error)
		assert.Zero(t, count, "a refused claim is not recorded")

		require.NoError(t, HwReleaseFirstPurchaseClaim(nil, first.Id, "ord-a"))
		eligible, reason, err = HwClaimFirstPurchaseDiscount(nil, second.Id, "ord-b", now)
		require.NoError(t, err)
		assert.True(t, eligible, "the discount returns when the first claim is released (reason: %s)", reason)
	})

	t.Run("refuses a card another claim already used", func(t *testing.T) {
		hwCreditTestSetup(t)
		first := createHwTestUser(t, 0)
		second := createHwTestUser(t, 0)
		require.NoError(t, UpsertHwUserSignals(first.Id, HwUserSignals{CardFingerprint: "fp-card-1"}))
		require.NoError(t, UpsertHwUserSignals(second.Id, HwUserSignals{CardFingerprint: "fp-card-1"}))

		eligible, _, err := HwClaimFirstPurchaseDiscount(nil, first.Id, "ord-a", now)
		require.NoError(t, err)
		require.True(t, eligible)

		eligible, reason, err := HwClaimFirstPurchaseDiscount(nil, second.Id, "ord-b", now)
		require.NoError(t, err)
		assert.False(t, eligible)
		assert.Equal(t, HwFirstPurchaseSharedCard, reason)
	})

	t.Run("two users without shared signals both claim", func(t *testing.T) {
		hwCreditTestSetup(t)
		first := createHwTestUser(t, 0)
		second := createHwTestUser(t, 0)

		eligible, _, err := HwClaimFirstPurchaseDiscount(nil, first.Id, "ord-a", now)
		require.NoError(t, err)
		require.True(t, eligible)
		eligible, reason, err := HwClaimFirstPurchaseDiscount(nil, second.Id, "ord-b", now)
		require.NoError(t, err)
		assert.True(t, eligible, "reason: %s", reason)
	})
}

func TestHwComputePurchaseRejectsNonPositiveValue(t *testing.T) {
	_, err := HwComputePurchase(decimal.Zero, true)
	require.ErrorIs(t, err, ErrHwInvalidPurchaseValue)
	_, err = HwComputePurchase(decimal.RequireFromString("-1"), false)
	require.ErrorIs(t, err, ErrHwInvalidPurchaseValue)
}
