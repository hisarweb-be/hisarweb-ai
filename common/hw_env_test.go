package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hwTestSignalSalt is a 32-byte stand-in so the production cases do not fail on
// the salt rule by accident.
const hwTestSignalSalt = "0123456789abcdef0123456789abcdef"

func TestHwValidateEnvironment(t *testing.T) {
	t.Run("the skip flag is refused outside a development environment", func(t *testing.T) {
		for _, appEnv := range []string{"production", " Production ", "prod", "", "staging", "dev-local"} {
			t.Run(appEnv, func(t *testing.T) {
				t.Setenv("APP_ENV", appEnv)
				t.Setenv("HW_SIGNAL_SALT", hwTestSignalSalt)
				t.Setenv("HW_SKIP_PHONE_STEP", "1")
				require.ErrorIs(t, HwValidateEnvironment(), ErrHwPhoneStepSkipOutsideDevelopment)
			})
		}
	})

	t.Run("the skip flag is allowed in each development environment", func(t *testing.T) {
		for _, appEnv := range []string{"development", "dev", "test", "local", " DEVELOPMENT ", "Dev", "Test", "Local"} {
			t.Run(appEnv, func(t *testing.T) {
				t.Setenv("APP_ENV", appEnv)
				t.Setenv("HW_SIGNAL_SALT", hwTestSignalSalt)
				t.Setenv("HW_SKIP_PHONE_STEP", "1")
				assert.NoError(t, HwValidateEnvironment())
				assert.True(t, HwPhoneStepSkipped())
			})
		}
	})

	t.Run("false, 0 and an empty value switch the skip off", func(t *testing.T) {
		for _, value := range []string{"false", "FALSE", "0", "", " false "} {
			t.Run(value, func(t *testing.T) {
				t.Setenv("APP_ENV", "production")
				t.Setenv("HW_SIGNAL_SALT", hwTestSignalSalt)
				t.Setenv("HW_SKIP_PHONE_STEP", value)
				assert.NoError(t, HwValidateEnvironment())
				assert.False(t, HwPhoneStepSkipped())
			})
		}
	})

	t.Run("the skip is inactive in production even when the flag is set", func(t *testing.T) {
		t.Setenv("HW_SKIP_PHONE_STEP", "1")
		t.Setenv("APP_ENV", "production")
		t.Setenv("HW_SIGNAL_SALT", hwTestSignalSalt)
		assert.False(t, HwPhoneStepSkipped(), "the flag alone must never skip the phone step in production")
		assert.True(t, HwAppEnvIsProduction())
	})

	t.Run("production refuses a missing or short signal salt", func(t *testing.T) {
		for _, appEnv := range []string{"production", "prod", " PROD "} {
			for _, salt := range []string{"", "short", strings.Repeat("x", hwSignalSaltMinBytes-1)} {
				t.Setenv("APP_ENV", appEnv)
				t.Setenv("HW_SKIP_PHONE_STEP", "")
				t.Setenv("HW_SIGNAL_SALT", salt)
				err := HwValidateEnvironment()
				require.ErrorIs(t, err, ErrHwSignalSaltRequiredInProduction)
				// The refusal names the variables, never their values.
				if salt != "" {
					assert.NotContains(t, err.Error(), salt)
				}
			}
		}
	})

	t.Run("production accepts a salt of at least 32 bytes", func(t *testing.T) {
		t.Setenv("APP_ENV", "production")
		t.Setenv("HW_SKIP_PHONE_STEP", "")
		t.Setenv("HW_SIGNAL_SALT", strings.Repeat("s", hwSignalSaltMinBytes))
		assert.NoError(t, HwValidateEnvironment())
	})

	t.Run("a development environment does not need the signal salt", func(t *testing.T) {
		t.Setenv("APP_ENV", "development")
		t.Setenv("HW_SKIP_PHONE_STEP", "")
		t.Setenv("HW_SIGNAL_SALT", "")
		assert.NoError(t, HwValidateEnvironment())
	})
}
