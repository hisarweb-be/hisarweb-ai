package common

import (
	"os"
	"path/filepath"
	"strconv"
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

	// Only an APP_ENV that explicitly names a development environment may run
	// without the signal salt: an empty or misspelled value is a real deployment
	// that lost its setting, and it must refuse every keyed abuse hash.
	t.Run("every environment but an explicit development one refuses a missing salt", func(t *testing.T) {
		for _, appEnv := range []string{"", "production", "prod", " PROD ", "staging", "preview", "dev-local", "developmentx"} {
			for _, salt := range []string{"", "short", strings.Repeat("x", hwSignalSaltMinBytes-1)} {
				t.Run(appEnv+"/salt="+strconv.Itoa(len(salt)), func(t *testing.T) {
					t.Setenv("APP_ENV", appEnv)
					t.Setenv("HW_SKIP_PHONE_STEP", "")
					t.Setenv("HW_SIGNAL_SALT", salt)
					err := HwValidateEnvironment()
					require.ErrorIs(t, err, ErrHwSignalSaltRequired)
					// The refusal names the variables, never their values.
					if salt != "" {
						assert.NotContains(t, err.Error(), salt)
					}
				})
			}
		}
	})

	t.Run("a sufficient salt is accepted everywhere", func(t *testing.T) {
		for _, appEnv := range []string{"production", "staging", ""} {
			t.Run(appEnv, func(t *testing.T) {
				t.Setenv("APP_ENV", appEnv)
				t.Setenv("HW_SKIP_PHONE_STEP", "")
				t.Setenv("HW_SIGNAL_SALT", strings.Repeat("s", hwSignalSaltMinBytes))
				assert.NoError(t, HwValidateEnvironment())
			})
		}
	})

	t.Run("a development environment does not need the signal salt", func(t *testing.T) {
		for _, appEnv := range []string{"development", "dev", "test", "local", " DEVELOPMENT ", "Dev"} {
			t.Run(appEnv, func(t *testing.T) {
				t.Setenv("APP_ENV", appEnv)
				t.Setenv("HW_SKIP_PHONE_STEP", "")
				t.Setenv("HW_SIGNAL_SALT", "")
				assert.NoError(t, HwValidateEnvironment())
			})
		}
	})
}

// TestHwProductionFailsClosed pins the default of the production switch: only an
// explicit development or test value turns the production guards off. A server
// that forgot APP_ENV therefore still gets every guard instead of silently
// losing the setup token, the secure-cookie requirement and the refusal of the
// default administrator password.
func TestHwProductionFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name  string
		value string
		want  bool
	}{
		{name: "unset means production", value: "", want: true},
		{name: "explicit production", value: "production", want: true},
		{name: "production with padding and case", value: " Production ", want: true},
		{name: "unknown value means production", value: "staging", want: true},
		{name: "development", value: "development", want: false},
		{name: "development with padding and case", value: " Development ", want: false},
		{name: "test", value: "test", want: false},
		{name: "test in capitals", value: "TEST", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("APP_ENV", test.value)
			assert.Equal(t, test.want, HwIsProduction())
		})
	}
}

// TestHwDeployComposePinsProductionAppEnv reads the production compose of this
// repository and requires the app service to pin APP_ENV itself. The variable
// decides whether the production guards are on, so leaving it to the deployment
// platform means one forgotten value disables them; a fixed value cannot be
// forgotten and cannot be interpolated to an empty string.
func TestHwDeployComposePinsProductionAppEnv(t *testing.T) {
	compose, err := os.ReadFile(filepath.Join("..", "deploy", "docker-compose.yml"))
	require.NoError(t, err)
	text := string(compose)

	appStart := strings.Index(text, "\n  app:")
	postgresStart := strings.Index(text, "\n  postgres:")
	require.Positive(t, appStart, "the compose has an app service")
	require.Greater(t, postgresStart, appStart, "postgres follows the app service")
	appBlock := text[appStart:postgresStart]

	require.Contains(t, appBlock, "\n    environment:")
	assert.Contains(t, appBlock, "\n      APP_ENV: \"production\"\n", "the app service pins APP_ENV to production")
	assert.NotContains(t, appBlock, "APP_ENV: ${", "the value is fixed, never interpolated")
	assert.Equal(t, 1, strings.Count(text, "APP_ENV:"), "only the app service sets APP_ENV")
}

// TestHwDeployComposeKeepsBatchUpdateOff requires the production compose to pin
// batch updates off. With them on, consumption waits in memory while the credit
// package expiry run reads users.quota, so that consumption is deducted twice:
// once by the expiry and once when the buffer flushes. A fixed value cannot be
// switched on from the deployment platform.
func TestHwDeployComposeKeepsBatchUpdateOff(t *testing.T) {
	compose, err := os.ReadFile(filepath.Join("..", "deploy", "docker-compose.yml"))
	require.NoError(t, err)
	text := string(compose)

	assert.Contains(t, text, "\n      BATCH_UPDATE_ENABLED: \"false\"\n", "the app service pins batch updates off")
	assert.Equal(t, 1, strings.Count(text, "BATCH_UPDATE_ENABLED:"), "no other line sets or interpolates the flag")
}

// TestHwDeployComposePassesQualityEnv requires the production compose to pass
// the four quality-meter variables to the app container. The meter refuses to
// start without them; a compose that drops them turns a configured deployment
// into a meter that silently never runs, which is how the first measurement
// went missing.
func TestHwDeployComposePassesQualityEnv(t *testing.T) {
	compose, err := os.ReadFile(filepath.Join("..", "deploy", "docker-compose.yml"))
	require.NoError(t, err)
	text := string(compose)

	appStart := strings.Index(text, "\n  app:")
	postgresStart := strings.Index(text, "\n  postgres:")
	require.Positive(t, appStart, "the compose has an app service")
	require.Greater(t, postgresStart, appStart, "postgres follows the app service")
	appBlock := text[appStart:postgresStart]

	// The names are repeated here on purpose: the constants live in package
	// service, and importing that package from common would invert the
	// dependency direction between them.
	for _, name := range []string{
		"HW_QUALITY_BASE_URL",
		"HW_QUALITY_API_KEY",
		"HW_QUALITY_JUDGE_MODEL",
		"HW_QUALITY_OUT_DIR",
	} {
		assert.Contains(t, appBlock, "\n      "+name+": ${"+name+":-}\n",
			"the app service passes %s through from the deployment environment", name)
	}
}
