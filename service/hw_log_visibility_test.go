package service

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The customer reads the log content column, so a supplier name must leave it
// while the untouched text stays available to the operators in the same row.
func TestHwPublicErrorTextMovesSupplierDetailToTheAdminPart(t *testing.T) {
	other := model.NewLogOther()
	text := "upstream https://api.lumia.example/v1 failed via OpenRouter and LiteLLM"

	public := HwPublicErrorText(other, text)

	for _, name := range []string{"lumia", "openrouter", "litellm"} {
		assert.NotContains(t, strings.ToLower(public), name)
	}
	assert.Contains(t, public, "provider")
	adminInfo, ok := other.Snapshot()["admin_info"].(map[string]any)
	require.True(t, ok, "the original text is kept for the admin")
	assert.Equal(t, text, adminInfo["upstream_error"])
}

func TestHwPublicErrorTextLeavesCleanAndEmptyTextAlone(t *testing.T) {
	other := model.NewLogOther()
	assert.Equal(t, "", HwPublicErrorText(other, ""))
	assert.Equal(t, "rate limit reached", HwPublicErrorText(other, "rate limit reached"))
	assert.Empty(t, other.Snapshot(), "nothing is recorded when nothing was replaced")

	assert.Equal(t, "provider", HwPublicErrorText(nil, "Lumia"), "a missing log part must not panic")
}
