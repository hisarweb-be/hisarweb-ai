package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Portix and Kie are masked like every other supplier. "kie" is short, so it
// only counts as a whole word: ordinary words that contain it stay intact.
func TestHwReplaceProviderWordsMasksPortixAndKie(t *testing.T) {
	t.Run("portix and kie.ai are substring traces", func(t *testing.T) {
		assert.Equal(t, "served by provider", HwReplaceProviderTextWords("served by Portix"))
		assert.Equal(t, "see https://api.provider/v1", HwReplaceProviderTextWords("see https://api.kie.ai/v1"))
		assert.Equal(t, "x-provider-cost", HwReplaceProviderWords("x-portix-cost"))
		assert.True(t, HwHasProviderTextTrace("www.PortixAPI.com"))
		assert.True(t, HwHasProviderTextTrace("kieai"))
	})
	t.Run("kie as a whole word", func(t *testing.T) {
		assert.Equal(t, "provider failed, then provider", HwReplaceProviderTextWords("Kie failed, then KIE"))
		assert.Equal(t, "provider_cost", HwReplaceProviderTextWords("kie_cost"))
		assert.Equal(t, "x-provider-id", HwReplaceProviderWords("x-kie-id"))
		assert.True(t, HwHasProviderTextTrace("route via (kie)"))
	})
	t.Run("words that merely contain kie stay untouched", func(t *testing.T) {
		for _, text := range []string{"cookie", "Kiel", "kiev", "skies", "kieł", "kie2", "a Cookie jar", "pokie"} {
			assert.False(t, HwHasProviderTextTrace(text), text)
			assert.Equal(t, text, HwReplaceProviderTextWords(text), text)
		}
	})
}
