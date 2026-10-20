package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHwValidateEnvironment(t *testing.T) {
	t.Run("the skip flag is refused in production", func(t *testing.T) {
		t.Setenv("HW_SKIP_PHONE_STEP", "1")
		t.Setenv("APP_ENV", "production")
		err := HwValidateEnvironment()
		require.ErrorIs(t, err, ErrHwPhoneStepSkipInProduction)
	})

	t.Run("the skip flag is allowed outside production", func(t *testing.T) {
		t.Setenv("HW_SKIP_PHONE_STEP", "1")
		t.Setenv("APP_ENV", "development")
		assert.NoError(t, HwValidateEnvironment())
	})

	t.Run("production without the skip flag is fine", func(t *testing.T) {
		t.Setenv("HW_SKIP_PHONE_STEP", "")
		t.Setenv("APP_ENV", "production")
		assert.NoError(t, HwValidateEnvironment())
	})

	t.Run("production written with case or spaces still counts", func(t *testing.T) {
		t.Setenv("HW_SKIP_PHONE_STEP", "true")
		t.Setenv("APP_ENV", " Production ")
		err := HwValidateEnvironment()
		require.ErrorIs(t, err, ErrHwPhoneStepSkipInProduction)
		// The refusal names the variables, never their values.
		assert.NotContains(t, err.Error(), "true")
	})

	t.Run("the skip is inactive in production even when the flag is set", func(t *testing.T) {
		t.Setenv("HW_SKIP_PHONE_STEP", "1")
		t.Setenv("APP_ENV", "production")
		assert.False(t, HwPhoneStepSkipped(), "the flag alone must never skip the phone step in production")
	})

	t.Run("the skip is active outside production", func(t *testing.T) {
		t.Setenv("HW_SKIP_PHONE_STEP", "1")
		t.Setenv("APP_ENV", "development")
		assert.True(t, HwPhoneStepSkipped())
	})

	t.Run("without the flag nothing is skipped", func(t *testing.T) {
		t.Setenv("HW_SKIP_PHONE_STEP", "")
		t.Setenv("APP_ENV", "development")
		assert.False(t, HwPhoneStepSkipped())
	})
}
