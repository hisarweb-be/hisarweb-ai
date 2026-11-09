package common

import (
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// ValidateTOTPCodeStep checks a TOTP code like ValidateTOTPCode (30 s step,
// one step of clock skew either way) and also returns the time step that
// matched, so the caller can refuse a replay of the same step (ASVS V2.8.4).
func ValidateTOTPCodeStep(secret, code string, now time.Time) (bool, int64) {
	cleanCode := strings.ReplaceAll(code, " ", "")
	if len(cleanCode) != 6 {
		return false, 0
	}
	for _, skew := range []int64{0, -1, 1} {
		at := now.Add(time.Duration(skew) * 30 * time.Second)
		ok, err := totp.ValidateCustom(cleanCode, secret, at, totp.ValidateOpts{
			Period:    30,
			Skew:      0,
			Digits:    otp.DigitsSix,
			Algorithm: otp.AlgorithmSHA1,
		})
		if err == nil && ok {
			return true, at.Unix() / 30
		}
	}
	return false, 0
}
