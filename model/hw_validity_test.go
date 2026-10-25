package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const hwValidityTestDay = int64(24 * 3600)

func hwValidityTestExpiry(t *testing.T, orderRef string) int64 {
	t.Helper()
	return hwTestPackageByOrderRef(t, orderRef).ExpiresAt
}

func TestHwPurchaseResetsTheValidityOfTheWholeBalance(t *testing.T) {
	hwCreditTestSetup(t)
	now := int64(1_700_000_000)
	user := createHwTestUser(t, 0)

	_, err := GrantCreditPackage(nil, user.Id, 100, HwSourcePurchase, "first", now)
	require.NoError(t, err)
	_, err = GrantCreditPackage(nil, user.Id, 10, HwSourceFree, "free", now)
	require.NoError(t, err)
	assert.Equal(t, now+14*hwValidityTestDay, hwValidityTestExpiry(t, "free"))

	later := now + 20*hwValidityTestDay
	_, err = GrantCreditPackage(nil, user.Id, 50, HwSourcePurchase, "second", later)
	require.NoError(t, err)

	want := later + 30*hwValidityTestDay
	assert.Equal(t, want, hwValidityTestExpiry(t, "first"), "the older paid package moves to 30 days from the new top-up")
	assert.Equal(t, want, hwValidityTestExpiry(t, "second"))
	assert.Equal(t, now+14*hwValidityTestDay, hwValidityTestExpiry(t, "free"),
		"the free package ran out a week before the top-up and stays expired, it is not revived")
}

func TestHwPurchaseNeverShortensAndSkipsOtherSources(t *testing.T) {
	hwCreditTestSetup(t)
	now := int64(1_700_000_000)
	user := createHwTestUser(t, 0)

	_, err := GrantCreditPackage(nil, user.Id, 100, HwSourceAdmin, "channel-grant", now)
	require.NoError(t, err)
	channelExpiry := hwValidityTestExpiry(t, "channel-grant")
	require.Greater(t, channelExpiry, now+30*hwValidityTestDay, "a channel grant lasts longer than 30 days")

	_, err = GrantCreditPackage(nil, user.Id, 100, HwSourceBonus, "bonus-only", now)
	require.NoError(t, err)
	assert.Equal(t, now+30*hwValidityTestDay, hwValidityTestExpiry(t, "bonus-only"))

	_, err = GrantCreditPackage(nil, user.Id, 100, HwSourcePurchase, "buy", now+hwValidityTestDay)
	require.NoError(t, err)
	assert.Equal(t, channelExpiry, hwValidityTestExpiry(t, "channel-grant"), "a longer expiry is never cut")
	assert.Equal(t, now+31*hwValidityTestDay, hwValidityTestExpiry(t, "bonus-only"))
}

func TestHwPurchaseOfAnotherUserLeavesTheValidityAlone(t *testing.T) {
	hwCreditTestSetup(t)
	now := int64(1_700_000_000)
	one := createHwTestUser(t, 0)
	other := createHwTestUser(t, 0)
	_, err := GrantCreditPackage(nil, one.Id, 100, HwSourcePurchase, "one", now)
	require.NoError(t, err)
	_, err = GrantCreditPackage(nil, other.Id, 100, HwSourcePurchase, "other", now+10*hwValidityTestDay)
	require.NoError(t, err)
	assert.Equal(t, now+30*hwValidityTestDay, hwValidityTestExpiry(t, "one"))
}

func TestHwUserValidityStatuses(t *testing.T) {
	hwCreditTestSetup(t)
	now := int64(1_700_000_000)
	user := createHwTestUser(t, 0)

	none, err := HwUserValidity(user.Id, now)
	require.NoError(t, err)
	assert.Equal(t, HwValiditySummary{Status: HwValidityNone}, none)

	_, err = GrantCreditPackage(nil, user.Id, 100, HwSourcePurchase, "buy", now)
	require.NoError(t, err)
	active, err := HwUserValidity(user.Id, now+10*hwValidityTestDay)
	require.NoError(t, err)
	assert.Equal(t, HwValidityActive, active.Status)
	assert.Equal(t, now+30*hwValidityTestDay, active.ValidUntil)
	assert.Equal(t, 20, active.DaysRemaining)

	lastDay, err := HwUserValidity(user.Id, now+30*hwValidityTestDay-3600)
	require.NoError(t, err)
	assert.Equal(t, 1, lastDay.DaysRemaining, "the last started day still counts as a day")

	// The expiry job retires the package; the customer sees what was lost.
	end := now + 31*hwValidityTestDay
	retired, err := RetireExpiredPackages(end)
	require.NoError(t, err)
	require.Equal(t, 1, retired)
	expired, err := HwUserValidity(user.Id, end)
	require.NoError(t, err)
	assert.Equal(t, HwValidityExpired, expired.Status)
	assert.Equal(t, now+30*hwValidityTestDay, expired.ExpiredAt)
	assert.Equal(t, 100, expired.ExpiredQuota)
	assert.Zero(t, expired.DaysRemaining)

	// A new top-up makes the account active again.
	_, err = GrantCreditPackage(nil, user.Id, 40, HwSourcePurchase, "again", end)
	require.NoError(t, err)
	again, err := HwUserValidity(user.Id, end)
	require.NoError(t, err)
	assert.Equal(t, HwValidityActive, again.Status)
	assert.Equal(t, 30, again.DaysRemaining)
}

func TestHwUserValidityIgnoresRefundedPackages(t *testing.T) {
	hwCreditTestSetup(t)
	now := int64(1_700_000_000)
	user := createHwTestUser(t, 0)
	_, err := GrantCreditPackage(nil, user.Id, 100, HwSourcePurchase, "refunded", now)
	require.NoError(t, err)
	_, err = HwRevokeOrderCredit(nil, "refunded", now+hwValidityTestDay)
	require.NoError(t, err)
	summary, err := HwUserValidity(user.Id, now+2*hwValidityTestDay)
	require.NoError(t, err)
	assert.Equal(t, HwValidityNone, summary.Status, "a refund is not an expiry")
}

func TestHwMigrateCreditValidityRunsOnce(t *testing.T) {
	hwCreditTestSetup(t)
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Where(commonKeyCol+" = ?", hwValidityMigratedOption).Delete(&Option{}).Error)
	t.Cleanup(func() { DB.Where(commonKeyCol+" = ?", hwValidityMigratedOption).Delete(&Option{}) })
	created := int64(1_700_000_000)
	rollout := created + 25*hwValidityTestDay
	user := createHwTestUser(t, 0)

	_, err := GrantCreditPackage(nil, user.Id, 100, HwSourcePurchase, "soon", created)
	require.NoError(t, err)
	_, err = GrantCreditPackage(nil, user.Id, 10, HwSourceFree, "free", created+20*hwValidityTestDay)
	require.NoError(t, err)
	_, err = GrantCreditPackage(nil, user.Id, 10, HwSourceFree, "gone", created-30*hwValidityTestDay)
	require.NoError(t, err)

	moved, err := HwMigrateCreditValidity(DB, rollout)
	require.NoError(t, err)
	assert.EqualValues(t, 2, moved, "only packages that still run are moved; the lapsed one is not revived")
	assert.Equal(t, rollout+30*hwValidityTestDay, hwValidityTestExpiry(t, "soon"))
	assert.Equal(t, rollout+30*hwValidityTestDay, hwValidityTestExpiry(t, "free"))
	assert.Equal(t, created-30*hwValidityTestDay+14*hwValidityTestDay, hwValidityTestExpiry(t, "gone"))

	moved, err = HwMigrateCreditValidity(DB, rollout+5*hwValidityTestDay)
	require.NoError(t, err)
	assert.Zero(t, moved, "the marker stops a second run")
	assert.Equal(t, rollout+30*hwValidityTestDay, hwValidityTestExpiry(t, "soon"))
}
