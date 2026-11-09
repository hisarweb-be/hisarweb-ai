package common

import (
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// hwDummySalt is a fixed, public salt: the burn below only has to cost the same
// as a real Argon2id verification, its result is discarded.
var hwDummySalt = []byte("hw-dummy-salt-16")

// HwBurnPasswordHash spends one Argon2id verification worth of time. Login calls
// it when no account or no password hash exists for the identifier, so the answer
// time does not tell an unknown account from a wrong password (ASVS V2.2.1,
// Authentication Cheat Sheet "response timing").
func HwBurnPasswordHash(password string) {
	if len(password) > MaxAccountPasswordLength*utf8.UTFMax {
		return
	}
	_ = argon2.IDKey([]byte(password), hwDummySalt, accountPasswordTime, accountPasswordMemory, 1, accountPasswordKeyBytes)
}
