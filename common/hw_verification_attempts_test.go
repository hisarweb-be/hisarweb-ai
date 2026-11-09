package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A code survives a few wrong guesses, then it is burned: even the right code
// no longer works and the user must request a new one.
func TestVerificationCodeBurnsAfterTooManyWrongGuesses(t *testing.T) {
	key, purpose := "limit@example.com", EmailVerificationPurpose
	RegisterVerificationCodeWithKey(key, "123456", purpose)
	t.Cleanup(func() { DeleteKey(key, purpose) })

	for i := 0; i < HwVerificationMaxAttempts; i++ {
		assert.False(t, VerifyCodeWithKey(key, "000000", purpose))
	}
	assert.False(t, VerifyCodeWithKey(key, "123456", purpose), "burned code must stay invalid")

	RegisterVerificationCodeWithKey(key, "654321", purpose)
	assert.True(t, VerifyCodeWithKey(key, "654321", purpose), "a fresh code resets the counter")
}

func TestVerificationCodeSurvivesWrongGuessesBelowLimit(t *testing.T) {
	key, purpose := "below@example.com", PasswordResetPurpose
	RegisterVerificationCodeWithKey(key, "abcdef", purpose)
	t.Cleanup(func() { DeleteKey(key, purpose) })

	for i := 0; i < HwVerificationMaxAttempts-1; i++ {
		assert.False(t, VerifyCodeWithKey(key, "zzzzzz", purpose))
	}
	assert.True(t, VerifyCodeWithKey(key, "abcdef", purpose))
	// a success clears the counter
	for i := 0; i < HwVerificationMaxAttempts-1; i++ {
		assert.False(t, VerifyCodeWithKey(key, "zzzzzz", purpose))
	}
	assert.True(t, VerifyCodeWithKey(key, "abcdef", purpose))
}

func TestVerificationWrongGuessesWithoutCodeDoNotGrowState(t *testing.T) {
	for i := 0; i < 20; i++ {
		assert.False(t, VerifyCodeWithKey("nocode@example.com", "111111", EmailVerificationPurpose))
	}
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	_, tracked := verificationAttempts[EmailVerificationPurpose+"nocode@example.com"]
	assert.False(t, tracked)
}
