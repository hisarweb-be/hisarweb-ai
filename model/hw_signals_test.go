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

func TestUpsertHwUserSignalsKeepsExistingValues(t *testing.T) {
	hwCreditTestSetup(t)
	user := createHwTestUser(t, 0)

	require.NoError(t, UpsertHwUserSignals(user.Id, HwUserSignals{DeviceToken: "dev-1", IpHash: "ip-1"}))
	require.NoError(t, UpsertHwUserSignals(user.Id, HwUserSignals{PhoneHash: "phone-1"}))

	var signal HwUserSignal
	require.NoError(t, DB.Where("user_id = ?", user.Id).First(&signal).Error)
	assert.Equal(t, "dev-1", signal.DeviceToken)
	assert.Equal(t, "ip-1", signal.IpHash)
	assert.Equal(t, "phone-1", signal.PhoneHash)
	assert.Empty(t, signal.CardFingerprint)
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
