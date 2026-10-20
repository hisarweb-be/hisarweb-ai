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

func TestHwNormalizePhone(t *testing.T) {
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
			normalised, err := hwNormalizePhone(tc.given)
			require.NoError(t, err)
			assert.Equal(t, tc.want, normalised)
		})
	}

	// A foreign number is refused by default: only a Turkish number may become
	// the signal that earns the free trial credit.
	for _, given := range []string{"", "12345", "+1 555 123 4567", "+49 30 1234567", "0555 123 45 6a", "00905551234567"} {
		t.Run("rejects "+given, func(t *testing.T) {
			_, err := hwNormalizePhone(given)
			require.ErrorIs(t, err, ErrHwInvalidPhoneNumber)
		})
	}

	// The operator may add a country code; the plus then says which one.
	t.Run("an added country code is accepted and the default stays first", func(t *testing.T) {
		t.Setenv(hwPhoneCountryCodesEnv, "90, +49")
		normalised, err := hwNormalizePhone("+49 30 1234567")
		require.NoError(t, err)
		assert.Equal(t, "+49301234567", normalised)
		// A national spelling is still completed with the default code.
		normalised, err = hwNormalizePhone("555 123 45 67")
		require.NoError(t, err)
		assert.Equal(t, "+905551234567", normalised)
	})
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

func TestHwMigrationDropsTheLegacyUserSignalsTable(t *testing.T) {
	hwCreditTestSetup(t)
	require.NoError(t, DB.Migrator().DropTable(&HwUserSignal{}))
	// The pre-ledger shape: one column per signal kind instead of kind/hash
	// history rows, with a row of leftover data.
	require.NoError(t, DB.Exec(`CREATE TABLE hw_user_signals (
		id integer primary key autoincrement,
		user_id integer not null,
		device_token text,
		ip_hash text,
		phone_hash text
	)`).Error)
	require.NoError(t, DB.Exec("INSERT INTO hw_user_signals (user_id, device_token) VALUES (1, 'legacy-token')").Error)
	require.True(t, DB.Migrator().HasColumn(&HwUserSignal{}, "device_token"), "the old shape is in place")

	// The drop the migration performs before AutoMigrate, then the rebuild.
	require.NoError(t, hwDropLegacyUserSignalsTable(DB))
	require.NoError(t, DB.AutoMigrate(&HwUserSignal{}))

	assert.False(t, DB.Migrator().HasColumn(&HwUserSignal{}, "device_token"), "the legacy column does not survive")
	assert.True(t, DB.Migrator().HasColumn(&HwUserSignal{}, "kind"))
	var count int64
	require.NoError(t, DB.Model(&HwUserSignal{}).Count(&count).Error)
	assert.Zero(t, count, "the leftover row is gone with the old shape")

	user := createHwTestUser(t, 0)
	require.NoError(t, UpsertHwUserSignals(user.Id, HwUserSignals{DeviceToken: "dev-rebuilt"}))
	require.NoError(t, DB.Model(&HwUserSignal{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.Equal(t, int64(1), count, "the rebuilt table takes new history rows")

	// A second run over the current shape changes nothing.
	require.NoError(t, hwDropLegacyUserSignalsTable(DB))
	require.NoError(t, DB.Model(&HwUserSignal{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestHwSignalCaseHandling(t *testing.T) {
	hwCreditTestSetup(t)
	ipFirst := createHwTestUser(t, 0)
	ipSecond := createHwTestUser(t, 0)

	// IP address notation is case-insensitive by convention: the same address in
	// another spelling must keep matching.
	require.NoError(t, UpsertHwUserSignals(ipFirst.Id, HwUserSignals{Ip: "2001:DB8::1"}))
	require.NoError(t, UpsertHwUserSignals(ipSecond.Id, HwUserSignals{Ip: "2001:db8::1"}))
	shared, err := HwSharedSignal(ipFirst.Id, ipSecond.Id)
	require.NoError(t, err)
	assert.True(t, shared, "the same address in another case still matches")

	// A device token is an opaque browser value: two tokens that differ only in
	// case are two devices, not one shared device.
	deviceFirst := createHwTestUser(t, 0)
	deviceSecond := createHwTestUser(t, 0)
	require.NoError(t, UpsertHwUserSignals(deviceFirst.Id, HwUserSignals{DeviceToken: "Device-Token-ABC"}))
	require.NoError(t, UpsertHwUserSignals(deviceSecond.Id, HwUserSignals{DeviceToken: "device-token-abc"}))
	shared, err = HwSharedSignal(deviceFirst.Id, deviceSecond.Id)
	require.NoError(t, err)
	assert.False(t, shared, "a device token keeps its case")

	// A card fingerprint keeps the case the payment provider spelled it with.
	require.NoError(t, UpsertHwUserSignals(deviceFirst.Id, HwUserSignals{CardFingerprint: "FP-Provider-X9"}))
	require.NoError(t, UpsertHwUserSignals(deviceSecond.Id, HwUserSignals{CardFingerprint: "fp-provider-x9"}))
	shared, err = HwSharedSignal(deviceFirst.Id, deviceSecond.Id)
	require.NoError(t, err)
	assert.False(t, shared, "a card fingerprint keeps its case")

	// The E.164 phone form is digits only, so case handling cannot disturb it:
	// two spellings of one number keep hashing the same.
	phoneFirst := createHwTestUser(t, 0)
	phoneSecond := createHwTestUser(t, 0)
	require.NoError(t, UpsertHwUserSignals(phoneFirst.Id, HwUserSignals{Phone: "+90 555 123 45 67"}))
	require.NoError(t, UpsertHwUserSignals(phoneSecond.Id, HwUserSignals{Phone: "905551234567"}))
	shared, err = HwSharedSignal(phoneFirst.Id, phoneSecond.Id)
	require.NoError(t, err)
	assert.True(t, shared, "phone normalisation is untouched by the case rules")
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
