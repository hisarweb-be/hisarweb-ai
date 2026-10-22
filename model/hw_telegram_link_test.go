package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hwTelegramLinkTestSetup gives the link table a clean start. The shared model
// test database is created in TestMain; this table is migrated here because the
// model tests never assume it.
func hwTelegramLinkTestSetup(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&HwTelegramLinkToken{}))
	require.NoError(t, DB.Exec("DELETE FROM hw_telegram_link_tokens").Error)
	t.Cleanup(func() {
		DB.Exec("DELETE FROM hw_telegram_link_tokens")
	})
}

func TestHwTelegramLinkToken(t *testing.T) {
	hwTelegramLinkTestSetup(t)
	const now int64 = 1_770_000_000

	t.Run("a new token is 32 characters and only its hash is stored", func(t *testing.T) {
		raw, expiresAt, err := HwCreateTelegramLinkToken(41, now)
		require.NoError(t, err)
		assert.Len(t, raw, 32)
		assert.Equal(t, now+HwTelegramLinkTokenLifetimeSeconds, expiresAt)

		var tokens []HwTelegramLinkToken
		require.NoError(t, DB.Find(&tokens).Error)
		require.Len(t, tokens, 1)
		assert.Equal(t, 41, tokens[0].UserId)
		assert.NotEqual(t, raw, tokens[0].TokenHash, "the raw token is never stored")
		assert.Len(t, tokens[0].TokenHash, 64)
	})

	t.Run("a token is spent once", func(t *testing.T) {
		raw, _, err := HwCreateTelegramLinkToken(42, now)
		require.NoError(t, err)

		userID, err := HwConsumeTelegramLinkToken(raw, now)
		require.NoError(t, err)
		assert.Equal(t, 42, userID)

		_, err = HwConsumeTelegramLinkToken(raw, now)
		assert.ErrorIs(t, err, ErrHwTelegramLinkInvalid)
	})

	t.Run("an expired token is refused", func(t *testing.T) {
		raw, expiresAt, err := HwCreateTelegramLinkToken(43, now)
		require.NoError(t, err)

		_, err = HwConsumeTelegramLinkToken(raw, expiresAt)
		assert.ErrorIs(t, err, ErrHwTelegramLinkInvalid)
	})

	t.Run("a malformed token is refused without a lookup", func(t *testing.T) {
		for _, raw := range []string{"", "  ", "kort maar met spaties", "abc/def", "abc+def=="} {
			_, err := HwConsumeTelegramLinkToken(raw, now)
			assert.ErrorIs(t, err, ErrHwTelegramLinkInvalid, raw)
		}
	})

	t.Run("a new link replaces the earlier one", func(t *testing.T) {
		first, _, err := HwCreateTelegramLinkToken(44, now)
		require.NoError(t, err)
		second, _, err := HwCreateTelegramLinkToken(44, now)
		require.NoError(t, err)

		_, err = HwConsumeTelegramLinkToken(first, now)
		assert.ErrorIs(t, err, ErrHwTelegramLinkInvalid, "the older link stops working")

		userID, err := HwConsumeTelegramLinkToken(second, now)
		require.NoError(t, err)
		assert.Equal(t, 44, userID)

		var tokens int64
		require.NoError(t, DB.Model(&HwTelegramLinkToken{}).Where("user_id = ?", 44).Count(&tokens).Error)
		assert.Zero(t, tokens, "no link row survives its use")
	})
}
