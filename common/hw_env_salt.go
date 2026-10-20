package common

import (
	"os"
	"slices"
)

// hwSignalSaltInsufficient reports whether this process lacks a usable
// HW_SIGNAL_SALT. The salt keys every abuse hash (phone, device, address,
// card), so only an APP_ENV that explicitly names a development environment
// (development, dev, test, local) may leave it out. An empty, unknown or
// misspelled value is a real deployment that lost its setting: it must carry
// the salt rather than hash customer signals with a guessable key.
func hwSignalSaltInsufficient() bool {
	if slices.Contains(hwSkipPhoneStepAppEnvs, hwAppEnv()) {
		return false
	}
	return len(os.Getenv(hwSignalSaltEnv)) < hwSignalSaltMinBytes
}
