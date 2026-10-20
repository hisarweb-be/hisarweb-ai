package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
