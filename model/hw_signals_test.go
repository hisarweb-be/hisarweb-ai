package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	t.Setenv(hwSignalSaltEnv, "test-only-signal-salt")
	hash, err := HwHashSignalValue("10.0.0.1")
	require.NoError(t, err)
	assert.Len(t, hash, 64)
	assert.NotContains(t, hash, "10.0.0.1")

	trimmed, err := HwHashSignalValue(" 10.0.0.1 ")
	require.NoError(t, err)
	assert.Equal(t, hash, trimmed, "normalisation keeps matching signals comparable")

	t.Setenv(hwSignalSaltEnv, "another-test-only-signal-salt")
	otherSalt, err := HwHashSignalValue("10.0.0.1")
	require.NoError(t, err)
	assert.NotEqual(t, hash, otherSalt)
}

func TestUpsertHwUserSignalsKeepsHistory(t *testing.T) {
	hwCreditTestSetup(t)
	user := createHwTestUser(t, 0)

	require.NoError(t, UpsertHwUserSignals(user.Id, HwUserSignals{DeviceToken: "dev-1", IpHash: "ip-1"}))
	require.NoError(t, UpsertHwUserSignals(user.Id, HwUserSignals{IpHash: "ip-2"}))

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

	require.NoError(t, UpsertHwUserSignals(first.Id, HwUserSignals{PhoneHash: "phone-x"}))
	shared, err = HwSharedSignal(first.Id, second.Id)
	require.NoError(t, err)
	assert.False(t, shared)

	require.NoError(t, UpsertHwUserSignals(second.Id, HwUserSignals{PhoneHash: "phone-x"}))
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
	require.NoError(t, UpsertHwUserSignals(first.Id, HwUserSignals{IpHash: "ip-old"}))
	require.NoError(t, UpsertHwUserSignals(first.Id, HwUserSignals{IpHash: "ip-new"}))
	shared, err := HwSharedSignal(first.Id, second.Id)
	require.NoError(t, err)
	require.False(t, shared, "a changed address alone does not make a match")

	require.NoError(t, UpsertHwUserSignals(second.Id, HwUserSignals{IpHash: "ip-old"}))
	shared, err = HwSharedSignal(first.Id, second.Id)
	require.NoError(t, err)
	assert.True(t, shared, "an address the first user no longer uses still connects the accounts")

	kinds, err := hwSharedSignalKinds(first.Id, second.Id)
	require.NoError(t, err)
	assert.Equal(t, []string{HwSignalKindIp}, kinds)
}

func TestUpsertHwUserSignalsWithoutSaltStoresNothing(t *testing.T) {
	hwCreditTestSetup(t)
	user := createHwTestUser(t, 0)
	t.Setenv(hwSignalSaltEnv, "")

	err := UpsertHwUserSignals(user.Id, HwUserSignals{IpHash: "10.0.0.1"})
	require.ErrorIs(t, err, ErrHwSignalSaltMissing)

	var count int64
	require.NoError(t, DB.Model(&HwUserSignal{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.Zero(t, count, "a raw value is never stored as a fallback")
}
