package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hwSignupIpTestSetup starts from an empty signup counter table.
func hwSignupIpTestSetup(t *testing.T) {
	t.Helper()
	hwCreditTestSetup(t)
	require.NoError(t, DB.AutoMigrate(&HwSignupIp{}))
	require.NoError(t, DB.Exec("DELETE FROM hw_signup_ips").Error)
	t.Cleanup(func() { DB.Exec("DELETE FROM hw_signup_ips") })
}

func TestHwReserveSignupSlot(t *testing.T) {
	hwSignupIpTestSetup(t)
	now := int64(1_700_000_000)

	t.Run("allows three accounts per ip per day and refuses the fourth", func(t *testing.T) {
		for i := 1; i <= HwSignupIpDailyLimit; i++ {
			allowed, err := HwReserveSignupSlot("203.0.113.9", now, HwSignupIpDailyLimit)
			require.NoError(t, err)
			assert.True(t, allowed, "registration number %d must be allowed", i)
		}
		allowed, err := HwReserveSignupSlot("203.0.113.9", now, HwSignupIpDailyLimit)
		require.NoError(t, err)
		assert.False(t, allowed, "the fourth registration of the day must be refused")

		// A refused attempt does not consume anything: the address stays at the
		// limit, and another address is independent.
		allowed, err = HwReserveSignupSlot("203.0.113.9", now, HwSignupIpDailyLimit)
		require.NoError(t, err)
		assert.False(t, allowed)
		allowed, err = HwReserveSignupSlot("198.51.100.4", now, HwSignupIpDailyLimit)
		require.NoError(t, err)
		assert.True(t, allowed, "another address has its own counter")
	})

	t.Run("the next day the same address is allowed again", func(t *testing.T) {
		nextDay := now + 24*3600
		allowed, err := HwReserveSignupSlot("203.0.113.9", nextDay, HwSignupIpDailyLimit)
		require.NoError(t, err)
		assert.True(t, allowed, "a new UTC day starts a new counter")

		ipHash, err := hwHashRawSignal(HwSignalKindIp, "203.0.113.9")
		require.NoError(t, err)
		var rows []HwSignupIp
		require.NoError(t, DB.Where("ip_hash = ?", ipHash).Find(&rows).Error)
		days := map[int]int{}
		for _, row := range rows {
			days[row.Day] += row.Count
		}
		assert.Equal(t, HwSignupIpDailyLimit, days[hwSignupIpDayKey(now)], "the old day keeps its three")
		assert.Equal(t, 1, days[hwSignupIpDayKey(nextDay)], "the new day counts one")
	})

	t.Run("the stored value is the keyed hash, never the address", func(t *testing.T) {
		marker := "203.0.113.77"
		_, err := HwReserveSignupSlot(marker, now, HwSignupIpDailyLimit)
		require.NoError(t, err)
		ipHash, err := hwHashRawSignal(HwSignalKindIp, marker)
		require.NoError(t, err)

		var row HwSignupIp
		require.NoError(t, DB.Where("day = ? AND ip_hash = ?", hwSignupIpDayKey(now), ipHash).First(&row).Error)
		assert.Equal(t, ipHash, row.IpHash)
		assert.Len(t, row.IpHash, 64)
		assert.NotContains(t, row.IpHash, "203.0.113")

		// Two spellings of the same address share the counter.
		upper, err := HwReserveSignupSlot(strings.ToUpper(marker), now, HwSignupIpDailyLimit)
		require.NoError(t, err)
		assert.True(t, upper)
		var count int64
		require.NoError(t, DB.Model(&HwSignupIp{}).Where("ip_hash = ?", ipHash).Count(&count).Error)
		assert.Equal(t, int64(1), count, "case-insensitive addresses resolve to one row")
	})

	t.Run("without the signal salt the caller decides to continue", func(t *testing.T) {
		t.Setenv(hwSignalSaltEnv, "")
		_, err := HwReserveSignupSlot("203.0.113.9", now, HwSignupIpDailyLimit)
		require.ErrorIs(t, err, ErrHwSignalSaltMissing)
	})

	t.Run("a limit of zero disables the check", func(t *testing.T) {
		allowed, err := HwReserveSignupSlot("203.0.113.9", now, 0)
		require.NoError(t, err)
		assert.True(t, allowed)
	})
}

func TestHwDeleteOldSignupIpRows(t *testing.T) {
	hwSignupIpTestSetup(t)
	now := int64(1_700_000_000)

	for _, day := range []int{20230101, 20230102, 20230103} {
		require.NoError(t, DB.Create(&HwSignupIp{IpHash: "hash-" + strings.Repeat("0", 59) + string(rune('a'+day%26)), Day: day, Count: 1}).Error)
	}
	require.NoError(t, HwDeleteOldSignupIpRows(now, 2))
	var remaining []HwSignupIp
	require.NoError(t, DB.Find(&remaining).Error)
	// now is 2023-11-14, so every 2023-01 row is older than two days and gone.
	assert.Empty(t, remaining, "old counters are removed")
}
