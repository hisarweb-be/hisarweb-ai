package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHwModelFamily(t *testing.T) {
	cases := map[string]string{
		"claude-opus-4-1":                        HwFamilyClaudeOpus,
		"claude-opus-5-5-thinking":               HwFamilyClaudeOpus,
		"Claude-Sonnet-5-5":                      HwFamilyClaudeSonnet,
		"openrouter/anthropic/claude-sonnet-4.5": HwFamilyClaudeSonnet,
		"claude-sonnet-4.5:free":                 HwFamilyClaudeSonnet,
		"vendor/claude-haiku-4-5:free":           HwFamilyClaudeHaiku,
		"claude-haiku-4-5@high":                  HwFamilyClaudeHaiku,
		"gpt-4o-mini":                            HwFamilyGPT,
		"o3":                                     "o3",
		"":                                       "",
		"   ":                                    "",
	}
	for in, want := range cases {
		assert.Equal(t, want, HwModelFamily(in), "input %q", in)
	}
	// Different families never compare equal.
	assert.NotEqual(t, HwModelFamily("claude-opus-5-5"), HwModelFamily("claude-sonnet-5-5"))
	assert.NotEqual(t, HwModelFamily("claude-haiku-5-5"), HwModelFamily("gpt-4o"))
	assert.NotEqual(t, HwModelFamily("o3"), HwModelFamily("gpt-4o"))
}
