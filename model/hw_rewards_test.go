package model

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hwMarkVerified gives a user a unique device signal, which counts as verified
// for the affiliate reward and the free trial credit.
func hwMarkVerified(t *testing.T, userID int) {
	t.Helper()
	require.NoError(t, UpsertHwUserSignals(userID, HwUserSignals{DeviceToken: "device-" + common.GetRandomString(8)}))
}

func createHwTestInvitee(t *testing.T, quota int, inviterID int) User {
	t.Helper()
	user := createHwTestUser(t, quota)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("inviter_id", inviterID).Error)
	user.InviterId = inviterID
	return user
}

func TestHwGrantAffiliateReward(t *testing.T) {
	now := int64(1_700_000_000)

	t.Run("gives the inviter five percent of a paid purchase", func(t *testing.T) {
		hwCreditTestSetup(t)
		inviter := createHwTestUser(t, 0)
		buyer := createHwTestInvitee(t, 0, inviter.Id)
		hwMarkVerified(t, buyer.Id)
		hwMarkVerified(t, inviter.Id)
		_, err := GrantCreditPackage(nil, buyer.Id, 1000, HwSourcePurchase, "ord-1", now)
		require.NoError(t, err)

		granted, reason, err := HwGrantAffiliateReward(buyer.Id, 1000, "ord-1", now)
		require.NoError(t, err)
		require.True(t, granted, "reason: %s", reason)
		assert.Empty(t, reason)
		assert.Equal(t, 50, getUserQuotaFromDB(t, inviter.Id))

		var pkg HwCreditPackage
		require.NoError(t, DB.Where("user_id = ? AND source = ?", inviter.Id, HwSourceAffiliate).First(&pkg).Error)
		assert.Equal(t, 50, pkg.GrantedQuota)
		assert.Equal(t, "aff:ord-1", pkg.OrderRef)
		assert.Equal(t, now+hwCreditPackageValiditySeconds, pkg.ExpiresAt)

		granted, reason, err = HwGrantAffiliateReward(buyer.Id, 1000, "ord-1", now)
		require.NoError(t, err)
		assert.False(t, granted)
		assert.Equal(t, HwAffiliateAlreadyGranted, reason)
		assert.Equal(t, 50, getUserQuotaFromDB(t, inviter.Id))
	})

	t.Run("rejects a shared device", func(t *testing.T) {
		hwCreditTestSetup(t)
		inviter := createHwTestUser(t, 0)
		buyer := createHwTestInvitee(t, 0, inviter.Id)
		_, err := GrantCreditPackage(nil, buyer.Id, 1000, HwSourcePurchase, "ord-2", now)
		require.NoError(t, err)
		require.NoError(t, UpsertHwUserSignals(buyer.Id, HwUserSignals{DeviceToken: "device-shared"}))
		require.NoError(t, UpsertHwUserSignals(inviter.Id, HwUserSignals{DeviceToken: "device-shared"}))

		granted, reason, err := HwGrantAffiliateReward(buyer.Id, 1000, "ord-2", now)
		require.NoError(t, err)
		assert.False(t, granted)
		assert.Equal(t, "shared_device", reason)
		assert.Equal(t, 0, getUserQuotaFromDB(t, inviter.Id))
	})

	t.Run("rejects a shared ip address", func(t *testing.T) {
		hwCreditTestSetup(t)
		inviter := createHwTestUser(t, 0)
		buyer := createHwTestInvitee(t, 0, inviter.Id)
		_, err := GrantCreditPackage(nil, buyer.Id, 1000, HwSourcePurchase, "ord-3", now)
		require.NoError(t, err)
		hwMarkVerified(t, buyer.Id)
		hwMarkVerified(t, inviter.Id)
		require.NoError(t, UpsertHwUserSignals(buyer.Id, HwUserSignals{Ip: "iphash-shared"}))
		require.NoError(t, UpsertHwUserSignals(inviter.Id, HwUserSignals{Ip: "iphash-shared"}))

		granted, reason, err := HwGrantAffiliateReward(buyer.Id, 1000, "ord-3", now)
		require.NoError(t, err)
		assert.False(t, granted)
		assert.Equal(t, "shared_ip", reason)
	})

	t.Run("rejects a shared card", func(t *testing.T) {
		hwCreditTestSetup(t)
		inviter := createHwTestUser(t, 0)
		buyer := createHwTestInvitee(t, 0, inviter.Id)
		_, err := GrantCreditPackage(nil, buyer.Id, 1000, HwSourcePurchase, "ord-4", now)
		require.NoError(t, err)
		hwMarkVerified(t, buyer.Id)
		hwMarkVerified(t, inviter.Id)
		require.NoError(t, UpsertHwUserSignals(buyer.Id, HwUserSignals{CardFingerprint: "card-shared"}))
		require.NoError(t, UpsertHwUserSignals(inviter.Id, HwUserSignals{CardFingerprint: "card-shared"}))

		granted, reason, err := HwGrantAffiliateReward(buyer.Id, 1000, "ord-4", now)
		require.NoError(t, err)
		assert.False(t, granted)
		assert.Equal(t, "shared_card", reason)
	})

	t.Run("never rewards free credit", func(t *testing.T) {
		hwCreditTestSetup(t)
		inviter := createHwTestUser(t, 0)
		buyer := createHwTestInvitee(t, 0, inviter.Id)
		hwMarkVerified(t, buyer.Id)
		granted, _, err := HwGrantFreeTrialCredit(buyer.Id, 100, now)
		require.NoError(t, err)
		require.True(t, granted)

		granted, reason, err := HwGrantAffiliateReward(buyer.Id, 100, "free:"+strconv.Itoa(buyer.Id), now)
		require.NoError(t, err)
		assert.False(t, granted)
		assert.Equal(t, HwAffiliateNotPaidPurchase, reason)
		assert.Equal(t, 0, getUserQuotaFromDB(t, inviter.Id))
	})

	t.Run("a buyer without inviter earns nothing", func(t *testing.T) {
		hwCreditTestSetup(t)
		buyer := createHwTestUser(t, 0)
		_, err := GrantCreditPackage(nil, buyer.Id, 1000, HwSourcePurchase, "ord-5", now)
		require.NoError(t, err)

		granted, reason, err := HwGrantAffiliateReward(buyer.Id, 1000, "ord-5", now)
		require.NoError(t, err)
		assert.False(t, granted)
		assert.Equal(t, HwAffiliateNoInviter, reason)
	})

	t.Run("needs a verified signal on both sides", func(t *testing.T) {
		hwCreditTestSetup(t)
		inviter := createHwTestUser(t, 0)
		buyer := createHwTestInvitee(t, 0, inviter.Id)
		hwMarkVerified(t, buyer.Id)
		_, err := GrantCreditPackage(nil, buyer.Id, 1000, HwSourcePurchase, "ord-7", now)
		require.NoError(t, err)

		granted, reason, err := HwGrantAffiliateReward(buyer.Id, 1000, "ord-7", now)
		require.NoError(t, err)
		assert.False(t, granted)
		assert.Equal(t, HwAffiliateUnverifiedSignals, reason)
		assert.Equal(t, 0, getUserQuotaFromDB(t, inviter.Id))

		// One device record is not enough when only an IP address was seen.
		require.NoError(t, UpsertHwUserSignals(inviter.Id, HwUserSignals{Ip: "iphash-inviter"}))
		granted, reason, err = HwGrantAffiliateReward(buyer.Id, 1000, "ord-7", now)
		require.NoError(t, err)
		assert.False(t, granted)
		assert.Equal(t, HwAffiliateUnverifiedSignals, reason)

		hwMarkVerified(t, inviter.Id)
		granted, reason, err = HwGrantAffiliateReward(buyer.Id, 1000, "ord-7", now)
		require.NoError(t, err)
		assert.True(t, granted, "reason: %s", reason)
		assert.Equal(t, 50, getUserQuotaFromDB(t, inviter.Id))
	})

	t.Run("a disabled inviter earns nothing", func(t *testing.T) {
		hwCreditTestSetup(t)
		inviter := createHwTestUser(t, 0)
		require.NoError(t, DB.Model(&User{}).Where("id = ?", inviter.Id).Update("status", 2).Error)
		buyer := createHwTestInvitee(t, 0, inviter.Id)
		_, err := GrantCreditPackage(nil, buyer.Id, 1000, HwSourcePurchase, "ord-6", now)
		require.NoError(t, err)

		granted, reason, err := HwGrantAffiliateReward(buyer.Id, 1000, "ord-6", now)
		require.NoError(t, err)
		assert.False(t, granted)
		assert.Equal(t, HwAffiliateInviterInactive, reason)
	})
}

func TestHwGrantFreeTrialCredit(t *testing.T) {
	hwCreditTestSetup(t)
	now := int64(1_700_000_000)
	user := createHwTestUser(t, 0)

	hwMarkVerified(t, user.Id)
	granted, reason, err := HwGrantFreeTrialCredit(user.Id, 100, now)
	require.NoError(t, err)
	require.True(t, granted, "reason: %s", reason)
	assert.Equal(t, 100, getUserQuotaFromDB(t, user.Id))

	pkg := hwTestPackageByOrderRef(t, "free:"+strconv.Itoa(user.Id))
	assert.Equal(t, HwSourceFree, pkg.Source)
	assert.Equal(t, 100, pkg.GrantedQuota)
	assert.Equal(t, now+hwFreeCreditValiditySeconds, pkg.ExpiresAt)

	var group string
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Select(commonGroupCol).Find(&group).Error)
	assert.Equal(t, hwGroupFree, group)

	granted, reason, err = HwGrantFreeTrialCredit(user.Id, 100, now)
	require.NoError(t, err)
	assert.False(t, granted)
	assert.Equal(t, HwFreeTrialAlreadyGranted, reason)
	assert.Equal(t, 100, getUserQuotaFromDB(t, user.Id))

	// The first paid purchase upgrades the free group to paid.
	_, err = GrantCreditPackage(nil, user.Id, 500, HwSourcePurchase, "ord-paid", now)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Select(commonGroupCol).Find(&group).Error)
	assert.Equal(t, hwGroupPaid, group)
}

func TestHwGrantFreeTrialCreditGuards(t *testing.T) {
	now := int64(1_700_000_000)

	t.Run("refuses an account without a device or phone signal", func(t *testing.T) {
		hwCreditTestSetup(t)
		user := createHwTestUser(t, 0)
		require.NoError(t, UpsertHwUserSignals(user.Id, HwUserSignals{Ip: "203.0.113.9"}))

		granted, reason, err := HwGrantFreeTrialCredit(user.Id, 100, now)
		require.NoError(t, err)
		assert.False(t, granted)
		assert.Equal(t, HwFreeTrialUnverified, reason)
		assert.Equal(t, 0, getUserQuotaFromDB(t, user.Id))

		var count int64
		require.NoError(t, DB.Model(&HwCreditPackage{}).Where("user_id = ?", user.Id).Count(&count).Error)
		assert.Zero(t, count)
	})

	t.Run("refuses a device another account already used for the free credit", func(t *testing.T) {
		hwCreditTestSetup(t)
		first := createHwTestUser(t, 0)
		second := createHwTestUser(t, 0)
		require.NoError(t, UpsertHwUserSignals(first.Id, HwUserSignals{DeviceToken: "device-one"}))
		require.NoError(t, UpsertHwUserSignals(second.Id, HwUserSignals{DeviceToken: "device-one"}))

		granted, reason, err := HwGrantFreeTrialCredit(first.Id, 100, now)
		require.NoError(t, err)
		require.True(t, granted, "reason: %s", reason)

		granted, reason, err = HwGrantFreeTrialCredit(second.Id, 100, now)
		require.NoError(t, err)
		assert.False(t, granted)
		assert.Equal(t, HwFreeTrialSharedSignal+"device", reason)
		assert.Equal(t, 0, getUserQuotaFromDB(t, second.Id))
	})

	t.Run("refuses a card another account already used for the free credit", func(t *testing.T) {
		hwCreditTestSetup(t)
		first := createHwTestUser(t, 0)
		second := createHwTestUser(t, 0)
		hwMarkVerified(t, first.Id)
		hwMarkVerified(t, second.Id)
		require.NoError(t, UpsertHwUserSignals(first.Id, HwUserSignals{CardFingerprint: "fp-free"}))
		require.NoError(t, UpsertHwUserSignals(second.Id, HwUserSignals{CardFingerprint: "fp-free"}))

		granted, _, err := HwGrantFreeTrialCredit(first.Id, 100, now)
		require.NoError(t, err)
		require.True(t, granted)

		granted, reason, err := HwGrantFreeTrialCredit(second.Id, 100, now)
		require.NoError(t, err)
		assert.False(t, granted)
		assert.Equal(t, HwFreeTrialSharedSignal+"card", reason)
	})

	t.Run("refuses an amount above the hard cap", func(t *testing.T) {
		hwCreditTestSetup(t)
		user := createHwTestUser(t, 0)
		hwMarkVerified(t, user.Id)
		aboveCap := HwMaxFreeTrialQuota() + 1

		granted, reason, err := HwGrantFreeTrialCredit(user.Id, aboveCap, now)
		require.NoError(t, err)
		assert.False(t, granted)
		assert.Equal(t, HwFreeTrialQuotaTooHigh, reason)
		assert.Equal(t, 0, getUserQuotaFromDB(t, user.Id))

		var count int64
		require.NoError(t, DB.Model(&HwCreditPackage{}).Where("user_id = ?", user.Id).Count(&count).Error)
		assert.Zero(t, count)
	})

	t.Run("the cap itself is allowed", func(t *testing.T) {
		hwCreditTestSetup(t)
		user := createHwTestUser(t, 0)
		hwMarkVerified(t, user.Id)

		granted, reason, err := HwGrantFreeTrialCredit(user.Id, HwMaxFreeTrialQuota(), now)
		require.NoError(t, err)
		assert.True(t, granted, "reason: %s", reason)
		assert.Equal(t, HwMaxFreeTrialQuota(), getUserQuotaFromDB(t, user.Id))
	})
}

func TestHwFreeTrialCreditLeavesOperatorAccountsAlone(t *testing.T) {
	hwCreditTestSetup(t)
	now := int64(1_700_000_000)
	admin := createHwTestUser(t, 0)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", admin.Id).Update("role", common.RoleAdminUser).Error)

	hwMarkVerified(t, admin.Id)
	granted, reason, err := HwGrantFreeTrialCredit(admin.Id, 100, now)
	require.NoError(t, err)
	require.True(t, granted, "reason: %s", reason)

	var group string
	require.NoError(t, DB.Model(&User{}).Where("id = ?", admin.Id).Select(commonGroupCol).Find(&group).Error)
	assert.Equal(t, hwGroupDefault, group)
}
