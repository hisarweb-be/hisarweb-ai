package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hwAbuseSweepTestSetup starts from empty abuse tables and empty signals. The
// users table is emptied too: the sweep buckets every account by its creation
// hour, so users left over from earlier tests would fill the same buckets.
func hwAbuseSweepTestSetup(t *testing.T) {
	t.Helper()
	hwCreditTestSetup(t)
	require.NoError(t, DB.AutoMigrate(&HwAccountFlag{}, &HwSignupIp{}))
	for _, table := range []string{"hw_account_flags", "hw_signup_ips", "users"} {
		require.NoError(t, DB.Exec("DELETE FROM "+table).Error)
	}
	t.Cleanup(func() {
		for _, table := range []string{"hw_account_flags", "hw_signup_ips", "users"} {
			DB.Exec("DELETE FROM " + table)
		}
	})
}

// hwSweepTestUser creates a user whose registration hour is set explicitly, so
// the wave buckets of the sweep can be aimed at.
func hwSweepTestUser(t *testing.T, createdAt int64) User {
	t.Helper()
	user := createHwTestUser(t, 0)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("created_at", createdAt).Error)
	return user
}

func hwFlagOf(t *testing.T, userID int) HwAccountFlag {
	t.Helper()
	var flag HwAccountFlag
	require.NoError(t, DB.Where("user_id = ?", userID).First(&flag).Error)
	return flag
}

func TestHwDailyAbuseSweepSharedSignals(t *testing.T) {
	hwAbuseSweepTestSetup(t)
	now := int64(1_700_000_000)

	t.Run("freezes both accounts on a shared phone and leaves paid credit alone", func(t *testing.T) {
		shared := hwSweepTestUser(t, now-3600)
		other := hwSweepTestUser(t, now-3600)
		clean := hwSweepTestUser(t, now-3600)
		require.NoError(t, UpsertHwUserSignals(shared.Id, HwUserSignals{Phone: "+905551112233"}))
		require.NoError(t, UpsertHwUserSignals(other.Id, HwUserSignals{Phone: "0555 111 22 33"}))
		require.NoError(t, UpsertHwUserSignals(clean.Id, HwUserSignals{Phone: "+905559998877"}))
		// Paid credit on one of the shared accounts: freezing must not touch it.
		granted, err := GrantCreditPackage(nil, shared.Id, 40, HwSourcePurchase, "sweep-paid", now)
		require.NoError(t, err)
		require.True(t, granted)

		report, err := HwDailyAbuseSweep(now)
		require.NoError(t, err)
		assert.Equal(t, 2, report.SharedPhone, "both accounts with the shared phone freeze")
		assert.Equal(t, 2, report.FrozenTotal)

		frozen, err := HwAccountFrozen(shared.Id)
		require.NoError(t, err)
		assert.True(t, frozen)
		flag := hwFlagOf(t, shared.Id)
		assert.Equal(t, HwFreezeReasonSharedPhone, flag.Reason)
		frozen, err = HwAccountFrozen(other.Id)
		require.NoError(t, err)
		assert.True(t, frozen, "the other spelling of the same number matches through the hash")
		frozen, err = HwAccountFrozen(clean.Id)
		require.NoError(t, err)
		assert.False(t, frozen)

		var quota int
		require.NoError(t, DB.Model(&User{}).Where("id = ?", shared.Id).Pluck("quota", &quota).Error)
		assert.Equal(t, 40, quota, "freezing leaves the paid balance untouched")
		var packages int64
		require.NoError(t, DB.Model(&HwCreditPackage{}).Where("user_id = ? AND source = ?", shared.Id, HwSourcePurchase).Count(&packages).Error)
		assert.Equal(t, int64(1), packages, "the paid package itself is untouched")
	})

	t.Run("a repeated sweep freezes nothing new and keeps the first reason", func(t *testing.T) {
		report, err := HwDailyAbuseSweep(now)
		require.NoError(t, err)
		assert.Equal(t, 0, report.FrozenTotal, "already frozen accounts are not counted again")
	})

	t.Run("administrators are never frozen", func(t *testing.T) {
		admin := User{
			Username: "hw-admin-" + common.GetRandomString(6), Password: "x",
			Role: common.RoleAdminUser, Status: common.UserStatusEnabled,
			Group: "default", AuthVersion: 1, AffCode: "aff" + common.GetRandomString(6),
		}
		require.NoError(t, DB.Create(&admin).Error)
		require.NoError(t, DB.Model(&User{}).Where("id = ?", admin.Id).Update("created_at", now-3600).Error)
		// The admin shares the phone of the frozen pair above.
		require.NoError(t, UpsertHwUserSignals(admin.Id, HwUserSignals{Phone: "+905551112233"}))
		report, err := HwDailyAbuseSweep(now)
		require.NoError(t, err)
		assert.Equal(t, 0, report.SharedPhone)
		frozen, err := HwAccountFrozen(admin.Id)
		require.NoError(t, err)
		assert.False(t, frozen)
	})
}

func TestHwDailyAbuseSweepWaves(t *testing.T) {
	hwAbuseSweepTestSetup(t)
	now := int64(1_700_000_000)
	hour := (now - 3600) / 3600 * 3600

	t.Run("a full registration hour freezes as a wave", func(t *testing.T) {
		t.Setenv(hwSweepHourWaveEnv, "3")
		waveStart := hour
		quietStart := hour - 7200
		waveUsers := make([]int, 0, 3)
		for i := 0; i < 3; i++ {
			user := hwSweepTestUser(t, waveStart+int64(i)*60)
			waveUsers = append(waveUsers, user.Id)
		}
		quiet := hwSweepTestUser(t, quietStart+60)
		// Keep every account below the per-IP wave and off shared signals.
		for _, id := range append(waveUsers, quiet.Id) {
			require.NoError(t, UpsertHwUserSignals(id, HwUserSignals{Phone: "+90555100" + padTwo(id)}))
		}

		report, err := HwDailyAbuseSweep(now)
		require.NoError(t, err)
		assert.Equal(t, 3, report.SignupWave, "the three same-hour signups freeze as a wave")
		for _, id := range waveUsers {
			flag := hwFlagOf(t, id)
			assert.Equal(t, HwFreezeReasonSignupWave, flag.Reason)
		}
		frozen, err := HwAccountFrozen(quiet.Id)
		require.NoError(t, err)
		assert.False(t, frozen, "an account from a quiet hour stays open")
	})

	t.Run("one address with many new accounts freezes as a wave", func(t *testing.T) {
		t.Setenv(hwSweepHourWaveEnv, "30")
		t.Setenv(hwSweepIpDayWaveEnv, "3")
		ipUsers := make([]int, 0, 3)
		for i := 0; i < 3; i++ {
			user := hwSweepTestUser(t, hour+int64(i)*120)
			ipUsers = append(ipUsers, user.Id)
			require.NoError(t, UpsertHwUserSignals(user.Id, HwUserSignals{Ip: "203.0.113.50"}))
		}
		report, err := HwDailyAbuseSweep(now)
		require.NoError(t, err)
		assert.Equal(t, 3, report.SignupWave, "three accounts from one address freeze as a wave")
		for _, id := range ipUsers {
			frozen, err := HwAccountFrozen(id)
			require.NoError(t, err)
			assert.True(t, frozen, "account %d from the flagged address freezes", id)
		}
	})

	t.Run("an old first_seen address outside the window does not freeze", func(t *testing.T) {
		t.Setenv(hwSweepHourWaveEnv, "30")
		t.Setenv(hwSweepIpDayWaveEnv, "3")
		stale := hwSweepTestUser(t, hour+300)
		require.NoError(t, UpsertHwUserSignals(stale.Id, HwUserSignals{Ip: "198.51.100.7"}))
		// Age the signal row beyond the sweep window.
		require.NoError(t, DB.Model(&HwUserSignal{}).
			Where("user_id = ? AND kind = ?", stale.Id, HwSignalKindIp).
			Update("first_seen", now-48*3600).Error)
		report, err := HwDailyAbuseSweep(now)
		require.NoError(t, err)
		assert.Equal(t, 0, report.SignupWave)
		frozen, err := HwAccountFrozen(stale.Id)
		require.NoError(t, err)
		assert.False(t, frozen)
	})
}

func TestHwUnfreezeAccount(t *testing.T) {
	hwAbuseSweepTestSetup(t)
	now := int64(1_700_000_000)

	t.Run("an admin unfreezes and the gate opens at once", func(t *testing.T) {
		user := hwSweepTestUser(t, now-3600)
		frozen, err := FreezeHwAccounts([]int{user.Id}, HwFreezeReasonSharedPhone, now)
		require.NoError(t, err)
		require.Equal(t, 1, frozen)
		isFrozen, err := HwAccountFrozen(user.Id)
		require.NoError(t, err)
		require.True(t, isFrozen)

		unfrozen, err := HwUnfreezeAccount(user.Id, 42, now+60)
		require.NoError(t, err)
		assert.True(t, unfrozen)
		isFrozen, err = HwAccountFrozen(user.Id)
		require.NoError(t, err)
		assert.False(t, isFrozen, "the cache drops with the flag, the gate opens at once")
		flag := hwFlagOf(t, user.Id)
		assert.False(t, flag.Frozen)
		assert.Equal(t, 42, flag.UnfrozenBy)
		assert.Equal(t, now+60, flag.UnfrozenAt)

		// Unfreezing twice reports that nothing was lifted.
		unfrozen, err = HwUnfreezeAccount(user.Id, 42, now+61)
		require.NoError(t, err)
		assert.False(t, unfrozen)
	})
}

// padTwo formats a small id as a two-digit phone suffix in tests.
func padTwo(id int) string {
	return fmt.Sprintf("%02d", id%100)
}

func TestHwDailyAbuseSweepLogsCountsOnly(t *testing.T) {
	hwAbuseSweepTestSetup(t)
	now := int64(1_700_000_000)
	capture := hwCaptureServerLog(t)

	shared := hwSweepTestUser(t, now-3600)
	other := hwSweepTestUser(t, now-3600)
	require.NoError(t, UpsertHwUserSignals(shared.Id, HwUserSignals{Phone: "+905554443322", Ip: "203.0.113.90"}))
	require.NoError(t, UpsertHwUserSignals(other.Id, HwUserSignals{Phone: "+905554443322", Ip: "203.0.113.90"}))

	report, err := HwDailyAbuseSweep(now)
	require.NoError(t, err)
	require.Equal(t, 2, report.FrozenTotal)

	logged := capture.Out.String() + capture.Err.String()
	assert.Contains(t, logged, "hw: abuse sweep froze 2 accounts")
	assert.NotContains(t, logged, "905554443322", "no phone number in the log")
	assert.NotContains(t, logged, "203.0.113.90", "no address in the log")
	assert.NotContains(t, logged, shared.Username, "no account name in the log")
	// Not even a raw hash value of a signal may appear.
	var hashes []string
	require.NoError(t, DB.Model(&HwUserSignal{}).Where("user_id IN ?", []int{shared.Id, other.Id}).Pluck("hash", &hashes).Error)
	for _, hash := range hashes {
		assert.NotContains(t, logged, hash)
	}
}
