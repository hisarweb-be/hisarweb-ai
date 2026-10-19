package model

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		require.NoError(t, UpsertHwUserSignals(buyer.Id, HwUserSignals{IpHash: "iphash-shared"}))
		require.NoError(t, UpsertHwUserSignals(inviter.Id, HwUserSignals{IpHash: "iphash-shared"}))

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
		granted, err := HwGrantFreeTrialCredit(buyer.Id, 100, now)
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

	granted, err := HwGrantFreeTrialCredit(user.Id, 100, now)
	require.NoError(t, err)
	require.True(t, granted)
	assert.Equal(t, 100, getUserQuotaFromDB(t, user.Id))

	pkg := hwTestPackageByOrderRef(t, "free:"+strconv.Itoa(user.Id))
	assert.Equal(t, HwSourceFree, pkg.Source)
	assert.Equal(t, 100, pkg.GrantedQuota)
	assert.Equal(t, now+hwFreeCreditValiditySeconds, pkg.ExpiresAt)

	var group string
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Select(commonGroupCol).Find(&group).Error)
	assert.Equal(t, hwGroupFree, group)

	granted, err = HwGrantFreeTrialCredit(user.Id, 100, now)
	require.NoError(t, err)
	assert.False(t, granted)
	assert.Equal(t, 100, getUserQuotaFromDB(t, user.Id))

	// The first paid purchase upgrades the free group to paid.
	_, err = GrantCreditPackage(nil, user.Id, 500, HwSourcePurchase, "ord-paid", now)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Select(commonGroupCol).Find(&group).Error)
	assert.Equal(t, hwGroupPaid, group)
}

func TestHwFreeTrialCreditLeavesOperatorAccountsAlone(t *testing.T) {
	hwCreditTestSetup(t)
	now := int64(1_700_000_000)
	admin := createHwTestUser(t, 0)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", admin.Id).Update("role", common.RoleAdminUser).Error)

	granted, err := HwGrantFreeTrialCredit(admin.Id, 100, now)
	require.NoError(t, err)
	require.True(t, granted)

	var group string
	require.NoError(t, DB.Model(&User{}).Where("id = ?", admin.Id).Select(commonGroupCol).Find(&group).Error)
	assert.Equal(t, hwGroupDefault, group)
}
