package common

import (
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPasswordMinimumIsTwelve(t *testing.T) {
	assert.ErrorIs(t, ValidateNewAccountPassword("elevenchars"), ErrAccountPasswordLength)
	assert.NoError(t, ValidateNewAccountPassword("twelve-chars"))
	assert.ErrorIs(t, ValidateNewAccountPassword(strings.Repeat("a", 129)), ErrAccountPasswordLength)
}

func pwnedServer(t *testing.T, body string, status int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// only the 5 character prefix may arrive
		assert.Len(t, strings.TrimPrefix(r.URL.Path, "/range/"), 5)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	prev := PwnedPasswordsRangeURL
	PwnedPasswordsRangeURL = srv.URL + "/range/"
	t.Setenv("APP_ENV", "production")
	t.Cleanup(func() { srv.Close(); PwnedPasswordsRangeURL = prev })
}

func TestBreachedPasswordIsRejected(t *testing.T) {
	// SHA-1("password1234") suffix after the 5 character prefix
	sum := sha1Upper("password1234")
	pwnedServer(t, sum[5:]+":42\r\n0000000000000000000000000000000000A:1\r\n", http.StatusOK)
	assert.ErrorIs(t, ValidateNewAccountPassword("password1234"), ErrAccountPasswordBreached)
	assert.NoError(t, ValidateNewAccountPassword("another-fine-passphrase"))
}

func TestBreachedPasswordPaddingRowIsNotAMatch(t *testing.T) {
	sum := sha1Upper("password1234")
	pwnedServer(t, sum[5:]+":0\r\n", http.StatusOK)
	assert.NoError(t, ValidateNewAccountPassword("password1234"))
}

func TestBreachedPasswordFailsOpen(t *testing.T) {
	pwnedServer(t, "boom", http.StatusInternalServerError)
	assert.NoError(t, ValidateNewAccountPassword("password1234"))
	PwnedPasswordsRangeURL = "http://127.0.0.1:1/range/"
	assert.NoError(t, ValidateNewAccountPassword("password1234"))
}

func TestBreachedPasswordCheckOffOutsideProduction(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	assert.False(t, isBreachedPassword("password1234"))
}

func TestTOTPStepIsReportedAndSkewAccepted(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"
	now := time.Unix(1_700_000_010, 0)
	code, err := totp.GenerateCode(secret, now)
	require.NoError(t, err)
	ok, step := ValidateTOTPCodeStep(secret, code, now)
	assert.True(t, ok)
	assert.Equal(t, now.Unix()/30, step)
	// the previous step's code still passes and reports the earlier step
	old, _ := totp.GenerateCode(secret, now.Add(-30*time.Second))
	ok, step = ValidateTOTPCodeStep(secret, old, now)
	assert.True(t, ok)
	assert.Equal(t, now.Unix()/30-1, step)
	ok, _ = ValidateTOTPCodeStep(secret, "000000x", now)
	assert.False(t, ok)
	ok, _ = ValidateTOTPCodeStep(secret, "12345", now)
	assert.False(t, ok)
}

func TestLoginThrottleGrowsAndResets(t *testing.T) {
	id := "Throttle-User"
	now := time.Now()
	for i := 0; i < loginThrottleFreeFailures; i++ {
		assert.Zero(t, LoginThrottleCheck(id, now))
		LoginThrottleFailure(id, now)
	}
	assert.Zero(t, LoginThrottleCheck(id, now), "free failures are not delayed")
	LoginThrottleFailure(id, now)
	first := LoginThrottleCheck("throttle-user ", now)
	assert.Equal(t, loginThrottleBaseDelay, first, "key is case and space insensitive")
	LoginThrottleFailure(id, now.Add(first))
	assert.Equal(t, 2*loginThrottleBaseDelay, LoginThrottleCheck(id, now.Add(first)))
	for i := 0; i < 30; i++ {
		LoginThrottleFailure(id, now.Add(first))
	}
	assert.Equal(t, loginThrottleMaxDelay, LoginThrottleCheck(id, now.Add(first)), "delay is capped")
	assert.Zero(t, LoginThrottleCheck(id, now.Add(first+loginThrottleMaxDelay)))
	LoginThrottleSuccess(id)
	assert.Zero(t, LoginThrottleCheck(id, now.Add(first)))
	assert.Zero(t, LoginThrottleCheck(id, now.Add(2*time.Hour)))
}

func sha1Upper(s string) string {
	sum := sha1.Sum([]byte(s))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}
