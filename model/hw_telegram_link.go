package model

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrHwTelegramLinkInvalid refuses a link token that is empty, malformed,
// expired or already spent. The caller answers the same text for all of them:
// telling a stranger which of the four it was would help only a guesser.
var ErrHwTelegramLinkInvalid = errors.New("telegram link token is invalid or expired")

const (
	// HwTelegramLinkTokenLifetimeSeconds is how long the deep link in the
	// dashboard stays usable. Long enough to switch to the phone and open
	// Telegram, short enough that a token seen over a shoulder is worthless.
	HwTelegramLinkTokenLifetimeSeconds int64 = 15 * 60
	// hwTelegramLinkTokenBytes is the entropy of a token. 24 random bytes give
	// 32 base64 characters, inside the 64 characters Telegram allows in the
	// start payload and inside its [A-Za-z0-9_-] alphabet.
	hwTelegramLinkTokenBytes = 24
	// hwTelegramLinkTokenMaxLength bounds what is hashed, so nobody can make
	// the server hash a megabyte per request.
	hwTelegramLinkTokenMaxLength = 64
)

// HwTelegramLinkToken is one pending "link my Telegram account" link. Only the
// hash of the token is stored: a read of this table cannot be turned into a
// working link. The row is deleted the moment it is used, so a link works once.
type HwTelegramLinkToken struct {
	TokenHash string `json:"token_hash" gorm:"primaryKey;size:64"`
	UserId    int    `json:"user_id" gorm:"index;not null"`
	ExpiresAt int64  `json:"expires_at" gorm:"not null;default:0"`
	CreatedAt int64  `json:"created_at" gorm:"not null;default:0"`
}

func (HwTelegramLinkToken) TableName() string {
	return "hw_telegram_link_tokens"
}

// HwCreateTelegramLinkToken makes a fresh link token for one user and returns
// the raw value, which is never stored. Asking again replaces every earlier
// link of that user, so only the newest link in the dashboard works.
func HwCreateTelegramLinkToken(userID int, now int64) (string, int64, error) {
	if userID <= 0 {
		return "", 0, ErrHwTelegramLinkInvalid
	}
	random := make([]byte, hwTelegramLinkTokenBytes)
	if _, err := rand.Read(random); err != nil {
		return "", 0, err
	}
	raw := base64.RawURLEncoding.EncodeToString(random)
	expiresAt := now + HwTelegramLinkTokenLifetimeSeconds
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&HwTelegramLinkToken{}).Error; err != nil {
			return err
		}
		// The cheapest place to sweep: each new link drops every link that has
		// already expired, of any user.
		if err := tx.Where("expires_at < ?", now).Delete(&HwTelegramLinkToken{}).Error; err != nil {
			return err
		}
		return tx.Create(&HwTelegramLinkToken{
			TokenHash: hwTelegramLinkTokenHash(raw),
			UserId:    userID,
			ExpiresAt: expiresAt,
			CreatedAt: now,
		}).Error
	})
	if err != nil {
		return "", 0, err
	}
	return raw, expiresAt, nil
}

// HwClaimTelegramLinkToken spends a link token and answers the user it was made
// for, together with the row that was deleted so the caller can give it back
// with HwRestoreTelegramLinkToken when the link did not happen. The row is
// deleted with the same condition it was read with, and a delete that did not
// remove exactly one row fails: two deliveries of the same start command cannot
// both link an account.
func HwClaimTelegramLinkToken(raw string, now int64) (int, HwTelegramLinkToken, error) {
	if !hwTelegramLinkTokenWellFormed(raw) {
		return 0, HwTelegramLinkToken{}, ErrHwTelegramLinkInvalid
	}
	hash := hwTelegramLinkTokenHash(raw)
	var token HwTelegramLinkToken
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("token_hash = ? AND expires_at > ?", hash, now).First(&token).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrHwTelegramLinkInvalid
			}
			return err
		}
		result := tx.Where("token_hash = ?", hash).Delete(&HwTelegramLinkToken{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrHwTelegramLinkInvalid
		}
		return nil
	})
	if err != nil {
		return 0, HwTelegramLinkToken{}, err
	}
	return token.UserId, token, nil
}

// HwRestoreTelegramLinkToken puts back a token whose link did not happen, so the
// retry Telegram sends for a 503 still finds it. A link only comes back when it
// has not expired and the user has no newer link, so giving one back can never
// revive a link that a fresh dashboard visit already replaced.
func HwRestoreTelegramLinkToken(token HwTelegramLinkToken, now int64) error {
	if token.TokenHash == "" || token.UserId <= 0 {
		return ErrHwTelegramLinkInvalid
	}
	if token.ExpiresAt <= now {
		return ErrHwTelegramLinkInvalid
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var newer int64
		if err := tx.Model(&HwTelegramLinkToken{}).Where("user_id = ?", token.UserId).Count(&newer).Error; err != nil {
			return err
		}
		if newer > 0 {
			return nil
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&token).Error
	})
}

// hwTelegramLinkTokenWellFormed accepts only the alphabet and length Telegram
// can carry in a start payload, so the server never hashes an arbitrary body.
func hwTelegramLinkTokenWellFormed(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > hwTelegramLinkTokenMaxLength {
		return false
	}
	for i := range len(raw) {
		switch c := raw[i]; {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

func hwTelegramLinkTokenHash(raw string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	return hex.EncodeToString(sum[:])
}

// HwBindTelegramAccount links a Telegram account to a user without a Telegram
// account. It claims the account in the identity claim table, whose unique
// indexes let only one user win two links that race, and writes the user only
// while the user has no account yet. A lost race returns
// ErrExternalIdentityAlreadyClaimed and changes nothing.
func HwBindTelegramAccount(userID int, telegramID string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := ClaimExternalIdentityWithTx(tx, ExternalIdentityProviderTelegram, telegramID, userID); err != nil {
			return err
		}
		result := tx.Model(&User{}).
			Where("id = ? AND (telegram_id = ? OR telegram_id IS NULL)", userID, "").
			Update("telegram_id", telegramID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrExternalIdentityAlreadyClaimed
		}
		return nil
	})
}
