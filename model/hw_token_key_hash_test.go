package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func hwTokenHashSetup(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&Token{}))
	require.NoError(t, DB.Exec("DELETE FROM tokens").Error)
	t.Setenv("SESSION_SECRET", "unit-test-session-secret")
	t.Setenv(HwTokenKeyHashOnlyEnv, "")
	old := common.CryptoSecret
	common.CryptoSecret = "unit-test-crypto-secret"
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.CryptoSecret = old; common.RedisEnabled = oldRedis })
}

// A row written before this change: clear key, no hash.
func hwLegacyToken(t *testing.T, key string) *Token {
	t.Helper()
	token := &Token{UserId: 1, Name: "legacy", Key: key, Status: 1, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, DB.Create(token).Error)
	require.Empty(t, token.KeyHash)
	return token
}

func TestHwTokenLookupFallsBackToClearTextWhileHashIsEmpty(t *testing.T) {
	hwTokenHashSetup(t)
	legacy := hwLegacyToken(t, "legacykey0123456789")

	got, err := GetTokenByKey("legacykey0123456789", true)
	require.NoError(t, err)
	assert.Equal(t, legacy.Id, got.Id)

	_, err = GetTokenByKey("legacykey0123456780", true)
	assert.True(t, errors.Is(err, gorm.ErrRecordNotFound))
}

func TestHwTokenLookupByHash(t *testing.T) {
	hwTokenHashSetup(t)
	legacy := hwLegacyToken(t, "hashedkey0123456789")
	n, err := HwBackfillTokenKeyHashes(DB)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	// Prove the hash path answers: the clear text can no longer match.
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", legacy.Id).Update(commonKeyCol[1:len(commonKeyCol)-1], "other-clear-text").Error)
	got, err := GetTokenByKey("hashedkey0123456789", true)
	require.NoError(t, err)
	assert.Equal(t, legacy.Id, got.Id)
	assert.Equal(t, HwTokenKeyHash("hashedkey0123456789"), got.KeyHash)
}

func TestHwTokenBackfillIsIdempotentAndBatched(t *testing.T) {
	hwTokenHashSetup(t)
	total := hwTokenBackfillBatch + 7
	for i := 0; i < total; i++ {
		hwLegacyToken(t, "bulkkey"+common.GetRandomString(20))
	}
	n, err := HwBackfillTokenKeyHashes(DB)
	require.NoError(t, err)
	assert.Equal(t, total, n)
	var empty int64
	require.NoError(t, DB.Model(&Token{}).Where("key_hash = '' OR key_hash IS NULL").Count(&empty).Error)
	assert.Zero(t, empty)

	n, err = HwBackfillTokenKeyHashes(DB)
	require.NoError(t, err)
	assert.Zero(t, n, "a second run changes nothing")
}

func TestHwTokenBackfillNeedsAStableSecret(t *testing.T) {
	hwTokenHashSetup(t)
	t.Setenv("SESSION_SECRET", "")
	t.Setenv("CRYPTO_SECRET", "")
	hwLegacyToken(t, "nosecretkey0123456")
	n, err := HwBackfillTokenKeyHashes(DB)
	require.NoError(t, err)
	assert.Zero(t, n, "a random per-start secret would make every hash useless after a restart")
	got, err := GetTokenByKey("nosecretkey0123456", true)
	require.NoError(t, err)
	assert.Equal(t, "nosecretkey0123456", got.Key)
}

func TestHwTokenInsertKeepsClearTextAndAddsHashWhenFlagIsOff(t *testing.T) {
	hwTokenHashSetup(t)
	token := &Token{UserId: 1, Name: "new", Key: "plainnewkey0123456", Status: 1, ExpiredTime: -1}
	require.NoError(t, HwInsertToken(token))

	var row Token
	require.NoError(t, DB.First(&row, token.Id).Error)
	assert.Equal(t, "plainnewkey0123456", row.Key, "flag off is the old behaviour")
	assert.Equal(t, HwTokenKeyHash("plainnewkey0123456"), row.KeyHash)
	assert.Equal(t, "plainnewkey0123456", row.GetFullKey())
}

func TestHwTokenInsertHashOnlyStoresNoClearKey(t *testing.T) {
	hwTokenHashSetup(t)
	t.Setenv(HwTokenKeyHashOnlyEnv, "true")
	token := &Token{UserId: 1, Name: "new", Key: "secretnewkey0123456", Status: 1, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, HwInsertToken(token))

	var row Token
	require.NoError(t, DB.First(&row, token.Id).Error)
	assert.NotContains(t, row.Key, "secretnewkey0123456")
	assert.True(t, HwTokenKeyIsHashed(row.Key))
	assert.Equal(t, HwTokenKeyHash("secretnewkey0123456"), row.KeyHash)
	assert.Empty(t, row.GetFullKey(), "a hash-only key cannot be shown again")
	assert.Equal(t, HwMaskedHashedKey, row.GetMaskedKey())

	got, err := ValidateUserToken("secretnewkey0123456")
	require.NoError(t, err)
	assert.Equal(t, token.Id, got.Id)

	// The flag back off: the row keeps authenticating, new rows are clear again.
	t.Setenv(HwTokenKeyHashOnlyEnv, "false")
	got, err = ValidateUserToken("secretnewkey0123456")
	require.NoError(t, err)
	assert.Equal(t, token.Id, got.Id)
	again := &Token{UserId: 1, Name: "again", Key: "afterflagoff0123456", Status: 1, ExpiredTime: -1}
	require.NoError(t, HwInsertToken(again))
	var row2 Token
	require.NoError(t, DB.First(&row2, again.Id).Error)
	assert.Equal(t, "afterflagoff0123456", row2.Key)
}

func TestHwTokenFlagNeedsAStableSecret(t *testing.T) {
	hwTokenHashSetup(t)
	t.Setenv(HwTokenKeyHashOnlyEnv, "true")
	t.Setenv("SESSION_SECRET", "")
	t.Setenv("CRYPTO_SECRET", "")
	assert.False(t, HwTokenKeyHashOnly())
}

func TestHwTokenStoredPlaceholderIsNotACredential(t *testing.T) {
	hwTokenHashSetup(t)
	t.Setenv(HwTokenKeyHashOnlyEnv, "true")
	token := &Token{UserId: 1, Name: "new", Key: "passthehash0123456", Status: 1, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, HwInsertToken(token))
	var row Token
	require.NoError(t, DB.First(&row, token.Id).Error)

	// Whoever reads the database must not be able to use what they read.
	_, err := ValidateUserToken(row.Key)
	assert.ErrorIs(t, err, ErrTokenInvalid)
	_, err = ValidateUserToken(row.KeyHash)
	assert.ErrorIs(t, err, ErrTokenInvalid)
}

func TestHwTokenBackfillOfAPlaceholderRowTakesTheHashFromIt(t *testing.T) {
	hwTokenHashSetup(t)
	hash := HwTokenKeyHash("somekey0123456789")
	row := &Token{UserId: 1, Name: "p", Key: hwTokenPlaceholder(hash), Status: 1, ExpiredTime: -1}
	require.NoError(t, DB.Create(row).Error)
	n, err := HwBackfillTokenKeyHashes(DB)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	var got Token
	require.NoError(t, DB.First(&got, row.Id).Error)
	assert.Equal(t, hash, got.KeyHash)
}
