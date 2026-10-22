package model

import (
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var hwTestPhoneSeq atomic.Int64

// hwUniqueTestPhone returns a distinct valid Turkish mobile number, so a test can
// give every user its own phone signal.
func hwUniqueTestPhone() string {
	return "0555" + fmt.Sprintf("%07d", hwTestPhoneSeq.Add(1)%10_000_000)
}

// hwMarkPhoneVerified gives a user a unique phone signal, which the
// first-purchase claim and the free trial credit require.
func hwMarkPhoneVerified(t *testing.T, userID int) {
	t.Helper()
	require.NoError(t, UpsertHwUserSignals(userID, HwUserSignals{Phone: hwUniqueTestPhone()}))
}

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
		{name: "first purchase above the cap", list: "1500", firstPurchase: true, payable: "1000.00", firstValue: "1000.00", remainderValue: "500.00", discount: "500.00"},
		{name: "later purchase pays the full value", list: "1000", firstPurchase: false, payable: "1000.00", firstValue: "0.00", remainderValue: "1000.00", discount: "0.00"},
		{name: "rounds half up", list: "1.25", firstPurchase: true, payable: "0.63", firstValue: "1.25", remainderValue: "0.00", discount: "0.62"},
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
		hwMarkPhoneVerified(t, user.Id)

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
		hwMarkPhoneVerified(t, user.Id)

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
		hwMarkPhoneVerified(t, user.Id)
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
		hwMarkPhoneVerified(t, first.Id)
		hwMarkPhoneVerified(t, second.Id)
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

	t.Run("refuses an account without a phone number", func(t *testing.T) {
		hwCreditTestSetup(t)
		user := createHwTestUser(t, 0)
		// A card signal alone may not unlock the discount: without a phone hash
		// the cross-user check on phone numbers would be skipped entirely.
		require.NoError(t, UpsertHwUserSignals(user.Id, HwUserSignals{CardFingerprint: "fp-no-phone"}))

		eligible, reason, err := HwClaimFirstPurchaseDiscount(nil, user.Id, "ord-1", now)
		require.NoError(t, err)
		assert.False(t, eligible)
		assert.Equal(t, HwFirstPurchaseUnverified, reason)

		var count int64
		require.NoError(t, DB.Model(&HwFirstPurchaseClaim{}).Where("user_id = ?", user.Id).Count(&count).Error)
		assert.Zero(t, count, "an unverified claim is not recorded")
	})

	t.Run("two users without shared signals both claim", func(t *testing.T) {
		hwCreditTestSetup(t)
		first := createHwTestUser(t, 0)
		second := createHwTestUser(t, 0)
		hwMarkPhoneVerified(t, first.Id)
		hwMarkPhoneVerified(t, second.Id)

		eligible, _, err := HwClaimFirstPurchaseDiscount(nil, first.Id, "ord-a", now)
		require.NoError(t, err)
		require.True(t, eligible)
		eligible, reason, err := HwClaimFirstPurchaseDiscount(nil, second.Id, "ord-b", now)
		require.NoError(t, err)
		assert.True(t, eligible, "reason: %s", reason)
	})

	t.Run("racing orders of one user produce one eligible claim", func(t *testing.T) {
		hwCreditTestSetup(t)
		user := createHwTestUser(t, 0)
		hwMarkPhoneVerified(t, user.Id)

		// SQLite in this suite runs on a single connection, so these goroutines
		// queue instead of writing concurrently. The case still pins the
		// contract: the unique user_id index lets exactly one racing order claim
		// the discount; every other order is answered already_claimed.
		var wg sync.WaitGroup
		eligible := make([]bool, 5)
		reasons := make([]string, 5)
		errs := make([]error, 5)
		for i := range 5 {
			wg.Go(func() {
				eligible[i], reasons[i], errs[i] = HwClaimFirstPurchaseDiscount(nil, user.Id, "ord-race-"+strconv.Itoa(i), now)
			})
		}
		wg.Wait()

		claims := 0
		for i := range eligible {
			require.NoError(t, errs[i])
			if eligible[i] {
				claims++
			} else {
				assert.Equal(t, HwFirstPurchaseAlreadyClaimed, reasons[i])
			}
		}
		assert.Equal(t, 1, claims)

		var rows int64
		require.NoError(t, DB.Model(&HwFirstPurchaseClaim{}).Where("user_id = ?", user.Id).Count(&rows).Error)
		assert.Equal(t, int64(1), rows, "one claim row survives the race")
	})
}

func TestHwRecordFirstPurchaseCard(t *testing.T) {
	now := int64(1_700_000_000)

	t.Run("writes the paid card on the claim, hashed", func(t *testing.T) {
		hwCreditTestSetup(t)
		user := createHwTestUser(t, 0)
		hwMarkPhoneVerified(t, user.Id)
		eligible, reason, err := HwClaimFirstPurchaseDiscount(nil, user.Id, "ord-1", now)
		require.NoError(t, err)
		require.True(t, eligible, "reason: %s", reason)

		require.NoError(t, HwRecordFirstPurchaseCard(nil, user.Id, "ord-1", "fp-paid-with-this-card"))

		var claim HwFirstPurchaseClaim
		require.NoError(t, DB.Where("user_id = ?", user.Id).First(&claim).Error)
		assert.Len(t, claim.CardFingerprint, 64)
		assert.NotContains(t, claim.CardFingerprint, "fp-paid-with-this-card", "the raw fingerprint is never stored")

		// Recording the same card twice is a quiet repeat.
		require.NoError(t, HwRecordFirstPurchaseCard(nil, user.Id, "ord-1", "fp-paid-with-this-card"))
	})

	t.Run("the recorded card refuses another account's claim", func(t *testing.T) {
		hwCreditTestSetup(t)
		first := createHwTestUser(t, 0)
		second := createHwTestUser(t, 0)
		hwMarkPhoneVerified(t, first.Id)
		hwMarkPhoneVerified(t, second.Id)

		eligible, _, err := HwClaimFirstPurchaseDiscount(nil, first.Id, "ord-1", now)
		require.NoError(t, err)
		require.True(t, eligible)
		// The second account only shows the card in its signals when it pays with
		// it; the claim of the first account must carry the fingerprint by then.
		require.NoError(t, HwRecordFirstPurchaseCard(nil, first.Id, "ord-1", "fp-shared-later"))
		require.NoError(t, UpsertHwUserSignals(second.Id, HwUserSignals{CardFingerprint: "fp-shared-later"}))

		eligible, reason, err := HwClaimFirstPurchaseDiscount(nil, second.Id, "ord-2", now)
		require.NoError(t, err)
		assert.False(t, eligible)
		assert.Equal(t, HwFirstPurchaseSharedCard, reason)
	})

	t.Run("rejects input without a claim to write", func(t *testing.T) {
		hwCreditTestSetup(t)
		user := createHwTestUser(t, 0)
		hwMarkPhoneVerified(t, user.Id)

		err := HwRecordFirstPurchaseCard(nil, user.Id, "ord-unknown", "fp-card")
		require.ErrorIs(t, err, gorm.ErrRecordNotFound, "a reference without a claim has nothing to write")

		err = HwRecordFirstPurchaseCard(nil, user.Id, "ord-1", "  ")
		require.ErrorIs(t, err, ErrHwInvalidFirstPurchaseClaim)
	})
}

func TestHwComputePurchaseRejectsNonPositiveValue(t *testing.T) {
	_, err := HwComputePurchase(decimal.Zero, true)
	require.ErrorIs(t, err, ErrHwInvalidPurchaseValue)
	_, err = HwComputePurchase(decimal.RequireFromString("-1"), false)
	require.ErrorIs(t, err, ErrHwInvalidPurchaseValue)
}
