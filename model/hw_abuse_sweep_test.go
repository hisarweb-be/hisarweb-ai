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
	require.NoError(t, DB.AutoMigrate(&HwAccountFlag{}, &HwSignupIp{}, &HwTelegramUpdate{}))
	for _, table := range []string{"hw_account_flags", "hw_signup_ips", "hw_telegram_updates", "users"} {
		require.NoError(t, DB.Exec("DELETE FROM "+table).Error)
	}
	t.Cleanup(func() {
		for _, table := range []string{"hw_account_flags", "hw_signup_ips", "hw_telegram_updates", "users"} {
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

func TestHwDailyAbuseSweepSharedDevice(t *testing.T) {
	now := int64(1_700_000_000)

	t.Run("a device token shared by two accounts freezes neither", func(t *testing.T) {
		hwAbuseSweepTestSetup(t)
		owner := hwSweepTestUser(t, now-3600)
		copier := hwSweepTestUser(t, now-3600)
		require.NoError(t, UpsertHwUserSignals(owner.Id, HwUserSignals{DeviceToken: "device-copied", Phone: "+905551110001"}))
		require.NoError(t, UpsertHwUserSignals(copier.Id, HwUserSignals{DeviceToken: "device-copied", Phone: "+905551110002"}))

		report, err := HwDailyAbuseSweep(now)
		require.NoError(t, err)
		assert.Zero(t, report.FrozenTotal, "a copied device token alone must freeze no one")
		for _, user := range []User{owner, copier} {
			frozen, err := HwAccountFrozen(user.Id)
			require.NoError(t, err)
			assert.False(t, frozen)
		}
	})

	t.Run("a device token with a second shared signal freezes the group", func(t *testing.T) {
		hwAbuseSweepTestSetup(t)
		first := hwSweepTestUser(t, now-3600)
		second := hwSweepTestUser(t, now-3600)
		require.NoError(t, UpsertHwUserSignals(first.Id, HwUserSignals{DeviceToken: "device-pair", Ip: "198.51.100.7", Phone: "+905551110011"}))
		require.NoError(t, UpsertHwUserSignals(second.Id, HwUserSignals{DeviceToken: "device-pair", Ip: "198.51.100.7", Phone: "+905551110012"}))

		report, err := HwDailyAbuseSweep(now)
		require.NoError(t, err)
		assert.Equal(t, 2, report.SharedDevice)
		assert.Equal(t, HwFreezeReasonSharedDevice, hwFlagOf(t, first.Id).Reason)
		assert.Equal(t, HwFreezeReasonSharedDevice, hwFlagOf(t, second.Id).Reason)
	})

	t.Run("five accounts on one device token freeze on the default threshold", func(t *testing.T) {
		hwAbuseSweepTestSetup(t)
		var users []User
		for i := range hwSweepDefaultDeviceUsers {
			user := hwSweepTestUser(t, now-3600)
			require.NoError(t, UpsertHwUserSignals(user.Id, HwUserSignals{
				DeviceToken: "device-bulk",
				Phone:       fmt.Sprintf("+9055511101%02d", i),
			}))
			users = append(users, user)
		}

		report, err := HwDailyAbuseSweep(now)
		require.NoError(t, err)
		assert.Equal(t, hwSweepDefaultDeviceUsers, report.SharedDevice)
		for _, user := range users {
			frozen, err := HwAccountFrozen(user.Id)
			require.NoError(t, err)
			assert.True(t, frozen, "one token on five accounts is a signup cycle, not a copy")
		}
	})

	t.Run("the device threshold is configurable", func(t *testing.T) {
		hwAbuseSweepTestSetup(t)
		t.Setenv(hwSweepDeviceUsersEnv, "3")
		for i := range 3 {
			user := hwSweepTestUser(t, now-3600)
			require.NoError(t, UpsertHwUserSignals(user.Id, HwUserSignals{
				DeviceToken: "device-three",
				Phone:       fmt.Sprintf("+9055511201%02d", i),
			}))
		}

		report, err := HwDailyAbuseSweep(now)
		require.NoError(t, err)
		assert.Equal(t, 3, report.SharedDevice)
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
			require.NoError(t, UpsertHwUserSignals(id, HwUserSignals{Phone: "+9055510000" + padTwo(id)}))
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

// M4: a full registration hour is not evidence against one account. Only the
// accounts of the wave that share a second signal with another account of the
// same hour, or that have no confirmed email address, go to review.
func TestHwDailyAbuseSweepWaveNeedsASecondSignal(t *testing.T) {
	now := int64(1_700_000_000)
	hour := (now - 3600) / 3600 * 3600

	// hwSweepWaveUser creates one account of the wave hour with a confirmed
	// email address and no signals, so only the explicit signal of the case
	// itself can qualify it.
	hwSweepWaveUser := func(t *testing.T, createdAt int64, email string) User {
		t.Helper()
		user := hwSweepTestUser(t, createdAt)
		require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("email", email).Error)
		return user
	}

	t.Run("an hour of confirmed emails with nothing shared freezes nobody", func(t *testing.T) {
		hwAbuseSweepTestSetup(t)
		t.Setenv(hwSweepHourWaveEnv, "3")
		t.Setenv(hwSweepIpDayWaveEnv, "10")
		var ids []int
		for i := 0; i < 3; i++ {
			user := hwSweepWaveUser(t, hour+int64(i)*60, fmt.Sprintf("customer-%d@example.com", i))
			ids = append(ids, user.Id)
		}

		report, err := HwDailyAbuseSweep(now)
		require.NoError(t, err)
		assert.Equal(t, 0, report.SignupWave, "the hour alone never freezes a real signup")
		for _, id := range ids {
			frozen, err := HwAccountFrozen(id)
			require.NoError(t, err)
			assert.False(t, frozen)
		}
	})

	t.Run("an account without a confirmed email in the wave freezes", func(t *testing.T) {
		hwAbuseSweepTestSetup(t)
		t.Setenv(hwSweepHourWaveEnv, "3")
		t.Setenv(hwSweepIpDayWaveEnv, "10")
		confirmed := make([]int, 0, 2)
		for i := 0; i < 2; i++ {
			confirmed = append(confirmed, hwSweepWaveUser(t, hour+int64(i)*60, fmt.Sprintf("known-%d@example.com", i)).Id)
		}
		anonymous := hwSweepTestUser(t, hour+150)

		report, err := HwDailyAbuseSweep(now)
		require.NoError(t, err)
		assert.Equal(t, 1, report.SignupWave)
		frozen, err := HwAccountFrozen(anonymous.Id)
		require.NoError(t, err)
		assert.True(t, frozen, "an unconfirmed email is the second signal")
		for _, id := range confirmed {
			frozen, err := HwAccountFrozen(id)
			require.NoError(t, err)
			assert.False(t, frozen, "a confirmed email without a second signal stays open")
		}
	})

	t.Run("one shared signup address freezes only its accounts", func(t *testing.T) {
		hwAbuseSweepTestSetup(t)
		t.Setenv(hwSweepHourWaveEnv, "3")
		t.Setenv(hwSweepIpDayWaveEnv, "10")
		var shared []int
		for i := 0; i < 2; i++ {
			user := hwSweepWaveUser(t, hour+int64(i)*60, fmt.Sprintf("same-address-%d@example.com", i))
			require.NoError(t, UpsertHwUserSignals(user.Id, HwUserSignals{Ip: "203.0.113.90"}))
			shared = append(shared, user.Id)
		}
		clean := hwSweepWaveUser(t, hour+150, "other-address@example.com")
		require.NoError(t, UpsertHwUserSignals(clean.Id, HwUserSignals{Ip: "203.0.113.91"}))

		report, err := HwDailyAbuseSweep(now)
		require.NoError(t, err)
		assert.Equal(t, 2, report.SignupWave)
		for _, id := range shared {
			frozen, err := HwAccountFrozen(id)
			require.NoError(t, err)
			assert.True(t, frozen)
		}
		frozen, err := HwAccountFrozen(clean.Id)
		require.NoError(t, err)
		assert.False(t, frozen, "another address of the same hour is not part of the finding")
	})

	t.Run("an email pattern shared inside the wave freezes its accounts", func(t *testing.T) {
		hwAbuseSweepTestSetup(t)
		t.Setenv(hwSweepHourWaveEnv, "3")
		t.Setenv(hwSweepIpDayWaveEnv, "10")
		var patterned []int
		for i := 0; i < 2; i++ {
			user := hwSweepWaveUser(t, hour+int64(i)*60, fmt.Sprintf("repeated+%d@example.com", i))
			patterned = append(patterned, user.Id)
		}
		clean := hwSweepWaveUser(t, hour+150, "different@example.com")

		report, err := HwDailyAbuseSweep(now)
		require.NoError(t, err)
		assert.Equal(t, 2, report.SignupWave)
		for _, id := range patterned {
			frozen, err := HwAccountFrozen(id)
			require.NoError(t, err)
			assert.True(t, frozen, "the same local part with a different +tag is one pattern")
		}
		frozen, err := HwAccountFrozen(clean.Id)
		require.NoError(t, err)
		assert.False(t, frozen)
	})

	t.Run("a copied device token stays weak inside a wave", func(t *testing.T) {
		hwAbuseSweepTestSetup(t)
		t.Setenv(hwSweepHourWaveEnv, "3")
		t.Setenv(hwSweepIpDayWaveEnv, "10")
		var copied []int
		for i := 0; i < 2; i++ {
			user := hwSweepWaveUser(t, hour+int64(i)*60, fmt.Sprintf("copied-%d@example.com", i))
			require.NoError(t, UpsertHwUserSignals(user.Id, HwUserSignals{DeviceToken: "wave-copied-device"}))
			copied = append(copied, user.Id)
		}
		clean := hwSweepWaveUser(t, hour+150, "deviceless@example.com")

		report, err := HwDailyAbuseSweep(now)
		require.NoError(t, err)
		assert.Equal(t, 0, report.SignupWave, "a device token below the bulk threshold is no second signal")
		assert.Equal(t, 0, report.SharedDevice)
		for _, id := range append(copied, clean.Id) {
			frozen, err := HwAccountFrozen(id)
			require.NoError(t, err)
			assert.False(t, frozen)
		}

		// The same token on the whole bulk group is the shape of one person
		// cycling through signups, and does freeze.
		var bulk []int
		for i := 0; i < hwSweepDefaultDeviceUsers; i++ {
			user := hwSweepWaveUser(t, hour-7200+int64(i)*60, fmt.Sprintf("bulk-%d@example.com", i))
			require.NoError(t, UpsertHwUserSignals(user.Id, HwUserSignals{DeviceToken: "wave-bulk-device"}))
			bulk = append(bulk, user.Id)
		}
		report, err = HwDailyAbuseSweep(now)
		require.NoError(t, err)
		assert.Equal(t, hwSweepDefaultDeviceUsers, report.SharedDevice)
		for _, id := range bulk {
			frozen, err := HwAccountFrozen(id)
			require.NoError(t, err)
			assert.True(t, frozen)
		}
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

	// M5/L4: one objection must not grant a lifetime of new findings, while a
	// thaw the sweep itself has to respect never gets overruled. A finding is
	// new only when a signal of the account was observed after the thaw: the
	// same old shared number must not refreeze the account every night.
	t.Run("a thawed account freezes again only on a newer signal", func(t *testing.T) {
		user := hwSweepTestUser(t, now-7200)
		require.NoError(t, UpsertHwUserSignals(user.Id, HwUserSignals{Phone: "+905557770011"}))
		hwSweepTestSignalSeen(t, user.Id, now-7200)
		frozen, err := FreezeHwAccounts([]int{user.Id}, HwFreezeReasonSharedPhone, now)
		require.NoError(t, err)
		require.Equal(t, 1, frozen)
		unfrozen, err := HwUnfreezeAccount(user.Id, 42, now+60)
		require.NoError(t, err)
		require.True(t, unfrozen)

		// A finding from before the thaw does not override the administrator,
		// and neither does the old signal the finding rested on.
		again, err := FreezeHwAccounts([]int{user.Id}, HwFreezeReasonSharedDevice, now+30)
		require.NoError(t, err)
		assert.Equal(t, 0, again)
		again, err = FreezeHwAccounts([]int{user.Id}, HwFreezeReasonSharedCard, now+120)
		require.NoError(t, err)
		assert.Equal(t, 0, again, "the old observation does not count as a new finding")
		isFrozen, err := HwAccountFrozen(user.Id)
		require.NoError(t, err)
		assert.False(t, isFrozen, "a thawed account stays thawed on the same old signal")

		// A fresh observation freezes the account again and replaces the reason.
		hwSweepTestSignalSeen(t, user.Id, now+180)
		again, err = FreezeHwAccounts([]int{user.Id}, HwFreezeReasonSharedCard, now+240)
		require.NoError(t, err)
		assert.Equal(t, 1, again)
		isFrozen, err = HwAccountFrozen(user.Id)
		require.NoError(t, err)
		assert.True(t, isFrozen)
		flag := hwFlagOf(t, user.Id)
		assert.True(t, flag.Frozen)
		assert.Equal(t, HwFreezeReasonSharedCard, flag.Reason)
		assert.Equal(t, now+240, flag.FrozenAt)
		assert.Equal(t, 42, flag.UnfrozenBy, "the thaw stays in the review history")
	})

	t.Run("the sweep keeps a thawed account and refreezes it on a new signal", func(t *testing.T) {
		first := hwSweepTestUser(t, now-10800)
		second := hwSweepTestUser(t, now-10800)
		require.NoError(t, UpsertHwUserSignals(first.Id, HwUserSignals{Phone: "+905559990011"}))
		require.NoError(t, UpsertHwUserSignals(second.Id, HwUserSignals{Phone: "+905559990011"}))
		hwSweepTestSignalSeen(t, first.Id, now-10800)
		hwSweepTestSignalSeen(t, second.Id, now-10800)
		report, err := HwDailyAbuseSweep(now)
		require.NoError(t, err)
		require.Equal(t, 2, report.SharedPhone)

		unfrozen, err := HwUnfreezeAccount(first.Id, 42, now+60)
		require.NoError(t, err)
		require.True(t, unfrozen)

		report, err = HwDailyAbuseSweep(now + 120)
		require.NoError(t, err)
		assert.Equal(t, 0, report.SharedPhone, "the same old signal finds nothing new")
		isFrozen, err := HwAccountFrozen(first.Id)
		require.NoError(t, err)
		assert.False(t, isFrozen, "the thaw holds through the next sweep")

		hwSweepTestSignalSeen(t, first.Id, now+180)
		report, err = HwDailyAbuseSweep(now + 240)
		require.NoError(t, err)
		assert.Equal(t, 1, report.SharedPhone, "a new observation freezes the account again")
		isFrozen, err = HwAccountFrozen(first.Id)
		require.NoError(t, err)
		assert.True(t, isFrozen)
	})
}

// The Telegram update log grows with every onboarding message. Telegram retries
// a delivery for a short while only, so the sweep keeps a week of claims and
// drops the rest; a fresh claim must survive.
func TestHwDailyAbuseSweepKeepsOneWeekOfTelegramUpdates(t *testing.T) {
	hwAbuseSweepTestSetup(t)
	now := int64(1_700_000_000)
	require.NoError(t, DB.Create(&HwTelegramUpdate{UpdateId: 11, CreatedAt: now - 8*24*3600}).Error)
	require.NoError(t, DB.Create(&HwTelegramUpdate{UpdateId: 12, CreatedAt: now - 6*24*3600}).Error)
	require.NoError(t, DB.Create(&HwTelegramUpdate{UpdateId: 13, CreatedAt: now - 7*24*3600 + 60}).Error)

	_, err := HwDailyAbuseSweep(now)
	require.NoError(t, err)

	var remaining []int64
	require.NoError(t, DB.Model(&HwTelegramUpdate{}).Order("update_id").Pluck("update_id", &remaining).Error)
	assert.Equal(t, []int64{12, 13}, remaining, "a week of update claims stays, the older one goes")
}

// hwSweepTestSignalSeen pins the observation time of every signal of a user, so
// a test can place a finding before or after a thaw. Signals written through
// UpsertHwUserSignals carry the real clock otherwise.
func hwSweepTestSignalSeen(t *testing.T, userID int, seen int64) {
	t.Helper()
	require.NoError(t, DB.Model(&HwUserSignal{}).
		Where("user_id = ?", userID).
		UpdateColumns(map[string]any{"first_seen": seen, "last_seen": seen}).Error)
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
