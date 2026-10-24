package service

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Gouden regel (Altuner, 24 okt 2026): always the best quality for the best
// price. No sold model may be billed above the newest model of its own family:
// an older Haiku, Sonnet, Opus, Fable, GPT-6 tier or Gemini tier is never more
// expensive than its newer successor.

var (
	hwClaudeModelPattern = regexp.MustCompile(`^claude-(haiku|sonnet|opus|fable)-(\d+(?:-\d+)*)$`)
	hwTierModelPattern   = regexp.MustCompile(`^(gpt|gemini)-(\d+(?:\.\d+)*)-([a-z]+)$`)
)

// hwValueFamily splits a sold model name into its family (the tier that is
// interchangeable for the customer) and its version numbers.
func hwValueFamily(name string) (string, []int, error) {
	var family, version string
	if m := hwClaudeModelPattern.FindStringSubmatch(name); m != nil {
		family, version = "claude-"+m[1], strings.ReplaceAll(m[2], "-", ".")
	} else if m := hwTierModelPattern.FindStringSubmatch(name); m != nil {
		family, version = m[1]+"-"+m[3], m[2]
	} else {
		return "", nil, fmt.Errorf("model %q has no known family; extend hwValueFamily", name)
	}
	parts := strings.Split(version, ".")
	numbers := make([]int, len(parts))
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return "", nil, err
		}
		numbers[i] = n
	}
	return family, numbers, nil
}

func hwVersionNewer(a, b []int) bool {
	for i := 0; i < len(a) || i < len(b); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			return x > y
		}
	}
	return false
}

// hwAssertNoOlderModelCostsMore fails for every model whose input or output
// price lies above the newest model of its family.
func hwAssertNoOlderModelCostsMore(t *testing.T, prices map[string]model.HwModelPrice) {
	t.Helper()
	type member struct {
		name    string
		version []int
	}
	newest := map[string]member{}
	familyOf := map[string]string{}
	for _, sold := range hwSoldModels {
		family, version, err := hwValueFamily(sold.Name)
		require.NoError(t, err)
		familyOf[sold.Name] = family
		if current, ok := newest[family]; !ok || hwVersionNewer(version, current.version) {
			newest[family] = member{name: sold.Name, version: version}
		}
	}
	for _, sold := range hwSoldModels {
		top := newest[familyOf[sold.Name]].name
		if top == sold.Name {
			continue
		}
		price, ok := prices[sold.Name]
		require.True(t, ok, "no price for %s", sold.Name)
		topPrice, ok := prices[top]
		require.True(t, ok, "no price for %s", top)
		assert.LessOrEqual(t, price.InputMicroUsd, topPrice.InputMicroUsd,
			"%s is billed above the newer %s (input)", sold.Name, top)
		assert.LessOrEqual(t, price.OutputMicroUsd, topPrice.OutputMicroUsd,
			"%s is billed above the newer %s (output)", sold.Name, top)
		assert.LessOrEqual(t, hwModelRatioFromInputPrice(price.InputMicroUsd), hwModelRatioFromInputPrice(topPrice.InputMicroUsd),
			"%s has a higher model ratio than the newer %s", sold.Name, top)
	}
}

// TestHwSoldModelsNeverCostMoreThanTheNewestOfTheirFamily checks the catalog
// prices that seed the billing ratios.
func TestHwSoldModelsNeverCostMoreThanTheNewestOfTheirFamily(t *testing.T) {
	hwAssertNoOlderModelCostsMore(t, hwSoldModelFallbackPrices())

	// The test only means something while a family has an older member.
	family, _, err := hwValueFamily("claude-haiku-4-5")
	require.NoError(t, err)
	members := 0
	for _, sold := range hwSoldModels {
		if f, _, _ := hwValueFamily(sold.Name); f == family {
			members++
		}
	}
	assert.GreaterOrEqual(t, members, 2, "the haiku family should hold an older and a newer model")
}

// TestHwDailyPriceListKeepsOlderModelsAtTheSuccessorPrice checks the daily
// refresh path: even when the open price list still prices the older model at
// its own (higher) list price, the snapshot bills it at its successor's price.
func TestHwDailyPriceListKeepsOlderModelsAtTheSuccessorPrice(t *testing.T) {
	// Official list prices of 24 Oct 2026, including the older, pricier models.
	list := map[string]hwPriceListEntry{}
	for name, price := range map[string][2]string{
		"claude-opus-5-5":   {"4e-06", "2e-05"},
		"claude-sonnet-5-5": {"2e-06", "1e-05"},
		"claude-sonnet-5":   {"2e-06", "1e-05"},
		"claude-haiku-5-5":  {"1e-07", "5e-07"},
		"claude-haiku-4-5":  {"1e-06", "5e-06"},
		"claude-fable-5-1":  {"1e-05", "5e-05"},
		"gpt-6-astra":       {"1e-05", "5e-05"},
		"gpt-6.1-sol":       {"2e-06", "1e-05"},
		"gpt-6-luna":        {"1e-07", "5e-07"},
		"gemini-3.8-flash":  {"7.5e-07", "3.75e-06"},
	} {
		list[name] = hwPriceListEntry{InputCostPerToken: []byte(price[0]), OutputCostPerToken: []byte(price[1]), Mode: "chat"}
	}
	prices, err := hwExtractSoldModelPrices(list)
	require.NoError(t, err)
	require.Len(t, prices, len(hwSoldModels))
	hwAssertNoOlderModelCostsMore(t, prices)
	assert.Equal(t, prices["claude-haiku-5-5"], prices["claude-haiku-4-5"])
}

func TestHwValueFamily(t *testing.T) {
	cases := map[string]string{
		"claude-haiku-4-5":  "claude-haiku",
		"claude-haiku-5-5":  "claude-haiku",
		"claude-sonnet-5":   "claude-sonnet",
		"claude-fable-5-1":  "claude-fable",
		"gpt-6-astra":       "gpt-astra",
		"gpt-6.1-sol":       "gpt-sol",
		"gemini-3.8-flash":  "gemini-flash",
		"claude-opus-5-5":   "claude-opus",
		"claude-sonnet-5-5": "claude-sonnet",
	}
	for name, want := range cases {
		family, _, err := hwValueFamily(name)
		require.NoError(t, err, name)
		assert.Equal(t, want, family, name)
	}
	_, a, _ := hwValueFamily("claude-sonnet-5-5")
	_, b, _ := hwValueFamily("claude-sonnet-5")
	assert.True(t, hwVersionNewer(a, b))
	assert.False(t, hwVersionNewer(b, a))
	_, _, err := hwValueFamily("mystery-model")
	assert.Error(t, err)
}
