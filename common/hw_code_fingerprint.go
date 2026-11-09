package common

import (
	"crypto/sha256"
	"encoding/hex"
)

// HwCodeFingerprint is what a log line may say about a redemption or
// verification code: the first 8 hex characters of its SHA-256. It lets an
// operator match two lines without the log holding a usable code (ASVS V7.1.1).
func HwCodeFingerprint(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:4])
}
