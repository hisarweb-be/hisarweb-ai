package model

// Relay keys stored as a keyed hash (ASVS V2.10.4, V6.4.1; gap G2 part B).
//
// The tokens table gets one extra column, key_hash: HMAC-SHA256 of the key with
// a server pepper derived from CRYPTO_SECRET (or SESSION_SECRET, the same
// fallback the rest of the server uses). Nothing is removed: the clear-text
// column stays and is only emptied in a later, separate, irreversible step.
//
//   - Lookup (hwFirstTokenByKey): by key_hash first, then by the clear text, so
//     a row that has no hash yet (not backfilled, or written by an older
//     binary) still authenticates.
//   - Backfill (HwBackfillTokenKeyHashes): fills key_hash for every row that
//     has none; part of HwMigrateModels, idempotent, in batches.
//   - New keys: with HW_TOKEN_KEY_HASH_ONLY=true a key is shown once, in the
//     create response, and the key column then holds "hw$<hash>" instead of the
//     key. With the flag off (default) new keys are stored in clear as before,
//     with their hash next to them. Switching the flag off again is the old
//     behaviour; hash-only rows keep working because lookup always uses the hash.
//
// A presented key that starts with "hw$" is never looked up: the stored value of
// a hash-only row is not a credential.

import (
	"errors"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const (
	// HwTokenKeyHashOnlyEnv turns clear-text storage of new relay keys off.
	HwTokenKeyHashOnlyEnv = "HW_TOKEN_KEY_HASH_ONLY"

	hwTokenHashedPrefix    = "hw$"
	hwTokenHashPepperLabel = "hw-token-key-v1:"
	hwTokenBackfillBatch   = 500
	// HwMaskedHashedKey is what list responses show for a key that cannot be shown.
	HwMaskedHashedKey = "****************"
)

// HwTokenHashingAvailable is true when a configured, restart-stable secret
// exists. Without one CryptoSecret is random per start and every hash would be
// useless after a restart, so no hash is written or used.
func HwTokenHashingAvailable() bool {
	return os.Getenv("CRYPTO_SECRET") != "" || os.Getenv("SESSION_SECRET") != ""
}

// HwTokenKeyHashOnly reports the storage flag. It also needs a stable secret.
func HwTokenKeyHashOnly() bool {
	return common.GetEnvOrDefaultBool(HwTokenKeyHashOnlyEnv, false) && HwTokenHashingAvailable()
}

// HwTokenKeyHash is the hex keyed hash of a relay key.
func HwTokenKeyHash(key string) string {
	return common.GenerateHMACWithKey([]byte(hwTokenHashPepperLabel+common.CryptoSecret), key)
}

// HwTokenKeyIsHashed reports whether a stored key value is the placeholder of a
// hash-only row (the clear key no longer exists anywhere).
func HwTokenKeyIsHashed(stored string) bool {
	return strings.HasPrefix(stored, hwTokenHashedPrefix)
}

func hwTokenPlaceholder(hash string) string { return hwTokenHashedPrefix + hash }

// hwBeforeTokenInsert fills key_hash for a row about to be created.
func hwBeforeTokenInsert(token *Token) {
	if token.KeyHash != "" || token.Key == "" || !HwTokenHashingAvailable() {
		return
	}
	if HwTokenKeyIsHashed(token.Key) {
		token.KeyHash = strings.TrimPrefix(token.Key, hwTokenHashedPrefix)
		return
	}
	token.KeyHash = HwTokenKeyHash(token.Key)
}

// HwInsertToken creates a token the way a customer-created key is stored. With
// the hash-only flag the clear key is replaced by its placeholder before the
// insert; the caller keeps the clear key to show it once.
func HwInsertToken(token *Token) error {
	if HwTokenKeyHashOnly() && token.Key != "" && !HwTokenKeyIsHashed(token.Key) {
		token.KeyHash = HwTokenKeyHash(token.Key)
		token.Key = hwTokenPlaceholder(token.KeyHash)
	}
	return token.Insert()
}

// hwCacheGetTokenByKey is cacheGetTokenByKey for a presented key: the cache
// entry of a clear-stored row is keyed on the key, that of a hash-only row on
// its placeholder.
func hwCacheGetTokenByKey(key string) (*Token, error) {
	if HwTokenKeyIsHashed(key) {
		return nil, gorm.ErrRecordNotFound
	}
	token, err := cacheGetTokenByKey(key)
	if err == nil || !HwTokenHashingAvailable() {
		return token, err
	}
	return cacheGetTokenByKey(hwTokenPlaceholder(HwTokenKeyHash(key)))
}

// hwFirstTokenByKey loads the row of a presented key: by hash, then by the clear
// text.
func hwFirstTokenByKey(token *Token, key string) error {
	if HwTokenKeyIsHashed(key) {
		return gorm.ErrRecordNotFound
	}
	if HwTokenHashingAvailable() {
		err := DB.Where("key_hash = ?", HwTokenKeyHash(key)).First(token).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		*token = Token{}
	}
	return DB.Where(commonKeyCol+" = ?", key).First(token).Error
}

// HwBackfillTokenKeyHashes gives every row without a hash its hash and returns
// how many it filled. Rows whose key is already a placeholder get the hash from
// it. Idempotent; a failure leaves the remaining rows to the clear-text fallback.
func HwBackfillTokenKeyHashes(db *gorm.DB) (int, error) {
	if !HwTokenHashingAvailable() || !db.Migrator().HasColumn(&Token{}, "key_hash") {
		return 0, nil
	}
	filled := 0
	lastId := 0
	for {
		var rows []Token
		err := db.Unscoped().Select("id", commonKeyCol).
			Where("id > ? AND (key_hash IS NULL OR key_hash = '') AND "+commonKeyCol+" <> ''", lastId).
			Order("id asc").Limit(hwTokenBackfillBatch).Find(&rows).Error
		if err != nil {
			return filled, err
		}
		if len(rows) == 0 {
			return filled, nil
		}
		for _, row := range rows {
			lastId = row.Id
			hash := strings.TrimPrefix(row.Key, hwTokenHashedPrefix)
			if !HwTokenKeyIsHashed(row.Key) {
				hash = HwTokenKeyHash(row.Key)
			}
			if err := db.Unscoped().Model(&Token{}).Where("id = ?", row.Id).Update("key_hash", hash).Error; err != nil {
				return filled, err
			}
			filled++
		}
	}
}
