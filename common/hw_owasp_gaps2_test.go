package common

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// G5: the burn costs a real Argon2id run, and an oversized input is skipped.
func TestHwBurnPasswordHashCostsAboutOneVerification(t *testing.T) {
	start := time.Now()
	HwBurnPasswordHash("a wrong password")
	assert.Greater(t, time.Since(start), time.Millisecond, "an Argon2id run is never instant")

	start = time.Now()
	HwBurnPasswordHash(strings.Repeat("x", MaxAccountPasswordLength*4+1))
	assert.Less(t, time.Since(start), 5*time.Millisecond, "oversized input is skipped")
}

// G11: a log line carries a fingerprint, never the code.
func TestHwCodeFingerprintHidesTheCode(t *testing.T) {
	code := "REDEEM-1234-SECRET"
	fp := HwCodeFingerprint(code)
	assert.Len(t, fp, 8)
	assert.NotContains(t, fp, "SECRET")
	assert.Equal(t, fp, HwCodeFingerprint(code))
	assert.NotEqual(t, fp, HwCodeFingerprint(code+"x"))
}

// G13: the memory map holds a digest, the code still verifies once registered.
func TestVerificationCodesAreStoredAsDigest(t *testing.T) {
	RegisterVerificationCodeWithKey("a@example.com", "123456", EmailVerificationPurpose)
	t.Cleanup(func() { DeleteKey("a@example.com", EmailVerificationPurpose) })

	verificationMutex.Lock()
	stored := verificationMap[EmailVerificationPurpose+"a@example.com"].code
	verificationMutex.Unlock()
	assert.NotEqual(t, "123456", stored)
	assert.NotContains(t, stored, "123456")

	assert.True(t, VerifyCodeWithKey("a@example.com", "123456", EmailVerificationPurpose))
	assert.False(t, VerifyCodeWithKey("a@example.com", "123457", EmailVerificationPurpose))
	assert.False(t, VerifyCodeWithKey("a@example.com", "123456", PasswordResetPurpose), "purpose is part of the digest")
	assert.False(t, VerifyCodeWithKey("b@example.com", "123456", EmailVerificationPurpose))

	DeleteKey("a@example.com", EmailVerificationPurpose)
	assert.False(t, VerifyCodeWithKey("a@example.com", "123456", EmailVerificationPurpose))
}

func TestVerificationCodeExpires(t *testing.T) {
	previous := VerificationValidMinutes
	t.Cleanup(func() {
		VerificationValidMinutes = previous
		DeleteKey("e@example.com", PasswordResetPurpose)
	})
	RegisterVerificationCodeWithKey("e@example.com", "tok", PasswordResetPurpose)
	require.True(t, VerifyCodeWithKey("e@example.com", "tok", PasswordResetPurpose))
	VerificationValidMinutes = 0
	assert.False(t, VerifyCodeWithKey("e@example.com", "tok", PasswordResetPurpose))
}

// G7: off by default, enforced when the variable is set.
func TestHwSessionIdleExpired(t *testing.T) {
	now := int64(1_000_000)
	assert.False(t, HwSessionIdleExpired(now-10_000_000, now), "default keeps the current behaviour")

	t.Setenv(hwSessionIdleEnv, "3600")
	assert.False(t, HwSessionIdleExpired(now-3600, now), "exactly at the limit is still fine")
	assert.True(t, HwSessionIdleExpired(now-3601, now))
	assert.False(t, HwSessionIdleExpired(0, now), "an unknown last activity is not idle")
	assert.False(t, HwSessionIdleExpired(now, now))
}
