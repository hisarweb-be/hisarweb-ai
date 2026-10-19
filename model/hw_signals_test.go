package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hwTestSignalSalt is a clearly fake salt of the required minimum length.
const hwTestSignalSalt = "test-only-signal-salt-0123456789abcdef"

func TestHwHashSignalValueNeedsAServerSecret(t *testing.T) {
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "server-crypto-secret"
	t.Cleanup(func() {
		common.CryptoSecret = previousSecret
	})

	t.Setenv(hwSignalSaltEnv, "")
	_, err := HwHashSignalValue("10.0.0.1")
	require.ErrorIs(t, err, ErrHwSignalSaltMissing, "there is no fallback to the server crypto secret")
	_, err = HwSignalSalt()
	require.ErrorIs(t, err, ErrHwSignalSaltMissing)

	t.Setenv(hwSignalSaltEnv, hwTestSignalSalt)
	hash, err := HwHashSignalValue("10.0.0.1")
	require.NoError(t, err)
	assert.Len(t, hash, 64)
	assert.NotContains(t, hash, "10.0.0.1")

	trimmed, err := HwHashSignalValue(" 10.0.0.1 ")
	require.NoError(t, err)
	assert.Equal(t, hash, trimmed, "normalisation keeps matching signals comparable")

	t.Setenv(hwSignalSaltEnv, hwTestSignalSalt+"-other")
	otherSalt, err := HwHashSignalValue("10.0.0.1")
	require.NoError(t, err)
	assert.NotEqual(t, hash, otherSalt)
}

func TestHwSignalSaltRefusesAShortSecret(t *testing.T) {
	hwCreditTestSetup(t)
	user := createHwTestUser(t, 0)
	t.Setenv(hwSignalSaltEnv, strings.Repeat("s", hwSignalSaltMinBytes-1))

	_, err := HwSignalSalt()
	require.ErrorIs(t, err, ErrHwSignalSaltTooShort)
	_, err = HwHashSignalValue("10.0.0.1")
	require.ErrorIs(t, err, ErrHwSignalSaltTooShort, "every hash function inherits the minimum length")
	err = UpsertHwUserSignals(user.Id, HwUserSignals{Ip: "10.0.0.1"})
	require.ErrorIs(t, err, ErrHwSignalSaltTooShort)

	var count int64
	require.NoError(t, DB.Model(&HwUserSignal{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.Zero(t, count, "a short salt stores nothing")

	t.Setenv(hwSignalSaltEnv, strings.Repeat("s", hwSignalSaltMinBytes))
	_, err = HwSignalSalt()
	require.NoError(t, err, "exactly the minimum length is accepted")
}

func TestUpsertHwUserSignalsKeepsHistory(t *testing.T) {
	hwCreditTestSetup(t)
	user := createHwTestUser(t, 0)

	require.NoError(t, UpsertHwUserSignals(user.Id, HwUserSignals{DeviceToken: "dev-1", Ip: "ip-1"}))
	require.NoError(t, UpsertHwUserSignals(user.Id, HwUserSignals{Ip: "ip-2"}))

	var signals []HwUserSignal
	require.NoError(t, DB.Where("user_id = ?", user.Id).Find(&signals).Error)
	require.Len(t, signals, 3, "a new value adds a row instead of replacing the old one")
	for _, signal := range signals {
		assert.Len(t, signal.Hash, 64)
		assert.NotContains(t, signal.Hash, "dev-1")
		assert.NotContains(t, signal.Hash, "ip-")
		assert.Positive(t, signal.FirstSeen)
		assert.Equal(t, signal.FirstSeen, signal.LastSeen)
	}
}

func TestHwSharedSignal(t *testing.T) {
	hwCreditTestSetup(t)
	first := createHwTestUser(t, 0)
	second := createHwTestUser(t, 0)

	shared, err := HwSharedSignal(first.Id, second.Id)
	require.NoError(t, err)
	assert.False(t, shared, "users without signals never match")

	require.NoError(t, UpsertHwUserSignals(first.Id, HwUserSignals{Phone: "0555 111 22 33"}))
	shared, err = HwSharedSignal(first.Id, second.Id)
	require.NoError(t, err)
	assert.False(t, shared)

	require.NoError(t, UpsertHwUserSignals(second.Id, HwUserSignals{Phone: "0555 111 22 33"}))
	shared, err = HwSharedSignal(first.Id, second.Id)
	require.NoError(t, err)
	assert.True(t, shared)
}

func TestHwSharedSignalUsesTheWholeHistory(t *testing.T) {
	hwCreditTestSetup(t)
	first := createHwTestUser(t, 0)
	second := createHwTestUser(t, 0)

	// The first user changes address; the old one stays in history and still
	// matches the second user.
	require.NoError(t, UpsertHwUserSignals(first.Id, HwUserSignals{Ip: "ip-old"}))
	require.NoError(t, UpsertHwUserSignals(first.Id, HwUserSignals{Ip: "ip-new"}))
	shared, err := HwSharedSignal(first.Id, second.Id)
	require.NoError(t, err)
	require.False(t, shared, "a changed address alone does not make a match")

	require.NoError(t, UpsertHwUserSignals(second.Id, HwUserSignals{Ip: "ip-old"}))
	shared, err = HwSharedSignal(first.Id, second.Id)
	require.NoError(t, err)
	assert.True(t, shared, "an address the first user no longer uses still connects the accounts")

	kinds, err := hwSharedSignalKinds(first.Id, second.Id)
	require.NoError(t, err)
	assert.Equal(t, []string{HwSignalKindIp}, kinds)
}

func TestUpsertHwUserSignalsAcceptsTheSameValueTwice(t *testing.T) {
	hwCreditTestSetup(t)
	user := createHwTestUser(t, 0)

	require.NoError(t, UpsertHwUserSignals(user.Id, HwUserSignals{DeviceToken: "dev-1"}))
	// A second writer for the same value must not fail on the unique index.
	require.NoError(t, UpsertHwUserSignals(user.Id, HwUserSignals{DeviceToken: "dev-1"}))

	var signals []HwUserSignal
	require.NoError(t, DB.Where("user_id = ?", user.Id).Find(&signals).Error)
	require.Len(t, signals, 1, "one value keeps one row")

	// The row keeps its first sighting and moves its last sighting.
	firstSeen := signals[0].FirstSeen
	require.NoError(t, DB.Model(&HwUserSignal{}).Where("id = ?", signals[0].Id).Update("last_seen", int64(1)).Error)
	require.NoError(t, UpsertHwUserSignals(user.Id, HwUserSignals{DeviceToken: "dev-1"}))
	require.NoError(t, DB.Where("id = ?", signals[0].Id).First(&signals[0]).Error)
	assert.Greater(t, signals[0].LastSeen, int64(1), "last_seen moves on a repeated value")
	assert.Equal(t, firstSeen, signals[0].FirstSeen, "first_seen is not touched")
}

func TestHwNormalizeTrPhone(t *testing.T) {
	cases := []struct{ given, want string }{
		{"0555 123 45 67", "+905551234567"},
		{"+90 555 123 45 67", "+905551234567"},
		{"905551234567", "+905551234567"},
		{"5551234567", "+905551234567"},
		{"555 123 45 67", "+905551234567"},
		{"(0555) 123-45-67", "+905551234567"},
	}
	for _, tc := range cases {
		t.Run(tc.given, func(t *testing.T) {
			normalised, err := hwNormalizeTrPhone(tc.given)
			require.NoError(t, err)
			assert.Equal(t, tc.want, normalised)
		})
	}

	for _, given := range []string{"", "12345", "+1 555 123 4567", "0555 123 45 6a", "00905551234567"} {
		t.Run("rejects "+given, func(t *testing.T) {
			_, err := hwNormalizeTrPhone(given)
			require.ErrorIs(t, err, ErrHwInvalidPhoneNumber)
		})
	}
}

func TestUpsertHwUserSignalsNormalisesThePhone(t *testing.T) {
	hwCreditTestSetup(t)
	first := createHwTestUser(t, 0)
	second := createHwTestUser(t, 0)

	require.NoError(t, UpsertHwUserSignals(first.Id, HwUserSignals{Phone: "0555 123 45 67"}))
	require.NoError(t, UpsertHwUserSignals(second.Id, HwUserSignals{Phone: "+90 555 123 45 67"}))
	shared, err := HwSharedSignal(first.Id, second.Id)
	require.NoError(t, err)
	assert.True(t, shared, "two spellings of one number must hash the same")

	err = UpsertHwUserSignals(second.Id, HwUserSignals{Phone: "12345"})
	require.ErrorIs(t, err, ErrHwInvalidPhoneNumber)

	var count int64
	require.NoError(t, DB.Model(&HwUserSignal{}).Where("user_id = ?", second.Id).Count(&count).Error)
	assert.Equal(t, int64(1), count, "an unrecognisable number stores nothing")
}

func TestUpsertHwUserSignalsNeverStoresRawValues(t *testing.T) {
	hwCreditTestSetup(t)
	user := createHwTestUser(t, 0)
	// A raw device token that happens to have the shape of one of our hashes is
	// still hashed, not accepted as a hash and not rejected.
	looksLikeOurHash := strings.Repeat("ab", 32)

	require.NoError(t, UpsertHwUserSignals(user.Id, HwUserSignals{
		Ip:              "203.0.113.7",
		Phone:           "905551234567",
		CardFingerprint: "fingerprint-from-provider",
		DeviceToken:     looksLikeOurHash,
	}))

	var signals []HwUserSignal
	require.NoError(t, DB.Where("user_id = ?", user.Id).Find(&signals).Error)
	require.Len(t, signals, 4)
	raw := map[string]bool{
		"203.0.113.7":               true,
		"905551234567":              true,
		"fingerprint-from-provider": true,
		looksLikeOurHash:            true,
	}
	for _, signal := range signals {
		assert.Len(t, signal.Hash, 64)
		assert.False(t, raw[signal.Hash], "the raw %s value must not be stored", signal.Kind)
	}
}

func TestUpsertHwUserSignalsWithoutSaltStoresNothing(t *testing.T) {
	hwCreditTestSetup(t)
	user := createHwTestUser(t, 0)
	t.Setenv(hwSignalSaltEnv, "")

	err := UpsertHwUserSignals(user.Id, HwUserSignals{Ip: "10.0.0.1"})
	require.ErrorIs(t, err, ErrHwSignalSaltMissing)

	var count int64
	require.NoError(t, DB.Model(&HwUserSignal{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.Zero(t, count, "a raw value is never stored as a fallback")
}
