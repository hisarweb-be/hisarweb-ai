package common

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"time"
)

// Verification and reset codes are never kept in clear text: the map (and Redis,
// when it is enabled) holds a salted SHA-256 digest bound to purpose and key, so
// a heap dump or a Redis read does not hand out a usable code (ASVS V2.7.2 and
// V8.3.4 in spirit). With Redis enabled the digest also lives there with the
// code lifetime as expiry, so a restart or a second node still accepts the code.

func hwVerificationDigest(purpose, key, code string) string {
	sum := sha256.Sum256([]byte("hw-verification\x00" + purpose + "\x00" + key + "\x00" + code))
	return hex.EncodeToString(sum[:])
}

func hwVerificationRedisKey(purpose, key string) string {
	sum := sha256.Sum256([]byte("hw-verification-key\x00" + purpose + "\x00" + key))
	return "hw:vc:" + hex.EncodeToString(sum[:])
}

func hwVerificationDigestsEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func hwVerificationRemote() bool {
	return RedisEnabled && RDB != nil
}

func hwVerificationRemoteStore(purpose, key, digest string) {
	if !hwVerificationRemote() {
		return
	}
	ttl := time.Duration(VerificationValidMinutes) * time.Minute
	if err := RedisSet(hwVerificationRedisKey(purpose, key), digest, ttl); err != nil {
		SysError(LogText("hw: verification code not stored in Redis, memory copy only: %v", err))
	}
}

// hwVerificationRemoteLoad returns the stored digest. ok is false when Redis is
// off or does not know the code, so the caller falls back to its memory map.
func hwVerificationRemoteLoad(purpose, key string) (string, bool) {
	if !hwVerificationRemote() {
		return "", false
	}
	digest, err := RedisGet(hwVerificationRedisKey(purpose, key))
	if err != nil {
		return "", false
	}
	return digest, true
}

func hwVerificationRemoteDelete(purpose, key string) {
	if hwVerificationRemote() {
		_ = RedisDel(hwVerificationRedisKey(purpose, key))
	}
}
