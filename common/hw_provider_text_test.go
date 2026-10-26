package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestHwReplaceProviderWords proves the last-resort net that hides the
// supplier from the customer: header values lose the header-name pattern as
// well, body text keeps ordinary words that merely contain it.
func TestHwReplaceProviderWords(t *testing.T) {
	t.Run("a header value loses every supplier trace in any case", func(t *testing.T) {
		assert.Equal(t, "via provider and provider, x-provider", HwReplaceProviderWords("via LiteLLM and OpenRouter, x-LUMIA"))
		assert.Equal(t, "providercost", HwReplaceProviderWords("x-or-cost"), "the whole header-name pattern is replaced")
	})
	t.Run("body text keeps the header-name pattern as ordinary prose", func(t *testing.T) {
		assert.Equal(t, "a box-or-bag", HwReplaceProviderTextWords("a box-or-bag"))
		assert.Equal(t, "provider failed, then provider", HwReplaceProviderTextWords("Lumia failed, then OPENROUTER"))
	})
	t.Run("text without a trace and empty text come back unchanged", func(t *testing.T) {
		assert.Equal(t, "", HwReplaceProviderWords(""))
		assert.Equal(t, "hello wörld", HwReplaceProviderWords("hello wörld"))
	})
	t.Run("the trace check agrees with the rewrite", func(t *testing.T) {
		assert.True(t, HwHasProviderTrace("X-OR-Cost"))
		assert.False(t, HwHasProviderTextTrace("X-OR-Cost"))
		assert.True(t, HwHasProviderTextTrace("served by LiteLLM"))
	})
	t.Run("multibyte text around a trace survives", func(t *testing.T) {
		assert.Equal(t, "şu provider çalışmadı", HwReplaceProviderTextWords("şu lumia çalışmadı"))
	})
}
