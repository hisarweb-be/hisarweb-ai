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

		userID, _, err := HwClaimTelegramLinkToken(raw, now)
		require.NoError(t, err)
		assert.Equal(t, 42, userID)

		_, _, err = HwClaimTelegramLinkToken(raw, now)
		assert.ErrorIs(t, err, ErrHwTelegramLinkInvalid)
	})

	t.Run("an expired token is refused", func(t *testing.T) {
		raw, expiresAt, err := HwCreateTelegramLinkToken(43, now)
		require.NoError(t, err)

		_, _, err = HwClaimTelegramLinkToken(raw, expiresAt)
		assert.ErrorIs(t, err, ErrHwTelegramLinkInvalid)
	})

	t.Run("a malformed token is refused without a lookup", func(t *testing.T) {
		for _, raw := range []string{"", "  ", "kort maar met spaties", "abc/def", "abc+def=="} {
			_, _, err := HwClaimTelegramLinkToken(raw, now)
			assert.ErrorIs(t, err, ErrHwTelegramLinkInvalid, raw)
		}
	})

	t.Run("a new link replaces the earlier one", func(t *testing.T) {
		first, _, err := HwCreateTelegramLinkToken(44, now)
		require.NoError(t, err)
		second, _, err := HwCreateTelegramLinkToken(44, now)
		require.NoError(t, err)

		_, _, err = HwClaimTelegramLinkToken(first, now)
		assert.ErrorIs(t, err, ErrHwTelegramLinkInvalid, "the older link stops working")

		userID, _, err := HwClaimTelegramLinkToken(second, now)
		require.NoError(t, err)
		assert.Equal(t, 44, userID)

		var tokens int64
		require.NoError(t, DB.Model(&HwTelegramLinkToken{}).Where("user_id = ?", 44).Count(&tokens).Error)
		assert.Zero(t, tokens, "no link row survives its use")
	})

	t.Run("a spent token can be given back while the link did not happen", func(t *testing.T) {
		raw, expiresAt, err := HwCreateTelegramLinkToken(45, now)
		require.NoError(t, err)

		userID, token, err := HwClaimTelegramLinkToken(raw, now)
		require.NoError(t, err)
		assert.Equal(t, 45, userID)
		assert.Equal(t, HwTelegramLinkToken{TokenHash: hwTelegramLinkTokenHash(raw), UserId: 45, ExpiresAt: expiresAt, CreatedAt: now}, token)
		_, _, err = HwClaimTelegramLinkToken(raw, now)
		assert.ErrorIs(t, err, ErrHwTelegramLinkInvalid, "a claimed token cannot be claimed twice")

		require.NoError(t, HwRestoreTelegramLinkToken(token, now))
		userID, _, err = HwClaimTelegramLinkToken(raw, now)
		require.NoError(t, err)
		assert.Equal(t, 45, userID)
	})

	t.Run("a given back token does not replace a newer link or outlive its expiry", func(t *testing.T) {
		raw, expiresAt, err := HwCreateTelegramLinkToken(46, now)
		require.NoError(t, err)
		_, token, err := HwClaimTelegramLinkToken(raw, now)
		require.NoError(t, err)

		newer, _, err := HwCreateTelegramLinkToken(46, now+60)
		require.NoError(t, err)
		require.NoError(t, HwRestoreTelegramLinkToken(token, now+60))
		_, _, err = HwClaimTelegramLinkToken(raw, now+60)
		assert.ErrorIs(t, err, ErrHwTelegramLinkInvalid, "the dashboard replaced this link, so giving it back revives nothing")
		_, _, err = HwClaimTelegramLinkToken(newer, now+60)
		require.NoError(t, err)

		// A token that expired while the attempt ran stays dead when it is given
		// back: a link nobody can use is not worth restoring.
		raw, _, err = HwCreateTelegramLinkToken(47, now)
		require.NoError(t, err)
		_, token, err = HwClaimTelegramLinkToken(raw, now)
		require.NoError(t, err)
		assert.ErrorIs(t, HwRestoreTelegramLinkToken(token, expiresAt), ErrHwTelegramLinkInvalid)
	})
}

// TestHwBindTelegramAccountKeepsOneOwner pins single ownership at the write:
// the check before it cannot see a link that another request makes at the same
// moment, so the write itself must refuse a second owner.
func TestHwBindTelegramAccountKeepsOneOwner(t *testing.T) {
	truncateTables(t)
	first := User{Username: "hw-tg-bind-one", Password: "password", AffCode: "hw-tg-bind-one"}
	second := User{Username: "hw-tg-bind-two", Password: "password", AffCode: "hw-tg-bind-two"}
	require.NoError(t, DB.Create(&first).Error)
	require.NoError(t, DB.Create(&second).Error)

	require.NoError(t, HwBindTelegramAccount(first.Id, "hw-tg-race"))
	assert.ErrorIs(t, HwBindTelegramAccount(second.Id, "hw-tg-race"), ErrExternalIdentityAlreadyClaimed)
	assert.ErrorIs(t, HwBindTelegramAccount(first.Id, "hw-tg-other"), ErrExternalIdentityAlreadyClaimed)

	var holders []User
	require.NoError(t, DB.Where("telegram_id <> ''").Find(&holders).Error)
	require.Len(t, holders, 1)
	assert.Equal(t, first.Id, holders[0].Id)
	assert.Equal(t, "hw-tg-race", holders[0].TelegramId)
}

// TestHwTelegramLinkTokenExpiryIsIndexed proves the expiry cleanup that runs
// on every link start can use an index instead of scanning the whole table.
func TestHwTelegramLinkTokenExpiryIsIndexed(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&HwTelegramLinkToken{}))
	assert.True(t, DB.Migrator().HasIndex(&HwTelegramLinkToken{}, "ExpiresAt"))
}
