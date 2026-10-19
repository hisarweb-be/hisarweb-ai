package common

import "strings"

// HwProviderTraceTokens identify a supplier or proxy trace in text that is about
// to be shown to a customer. Matching is case-insensitive.
var HwProviderTraceTokens = []string{"lumia", "openrouter", "x-or-", "litellm"}

// HwProviderPlaceholder is the neutral word that replaces a supplier trace.
const HwProviderPlaceholder = "provider"

// HwHasProviderTrace reports whether text contains a supplier or proxy trace.
func HwHasProviderTrace(text string) bool {
	lower := strings.ToLower(text)
	for _, token := range HwProviderTraceTokens {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

// HwReplaceProviderWords rewrites every supplier trace in text to the neutral
// placeholder. It is the last-resort net, not the primary masking: structural
// masking removes the fields that carry these names in the first place.
func HwReplaceProviderWords(text string) string {
	if text == "" || !HwHasProviderTrace(text) {
		return text
	}
	data := []byte(text)
	out := make([]byte, 0, len(data)+8)
	for index := 0; index < len(data); {
		matched := false
		for _, token := range HwProviderTraceTokens {
			if hwEqualFoldAt(data, index, token) {
				out = append(out, HwProviderPlaceholder...)
				index += len(token)
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		out = append(out, data[index])
		index++
	}
	return string(out)
}

func hwEqualFoldAt(data []byte, start int, token string) bool {
	if start+len(token) > len(data) {
		return false
	}
	for offset := 0; offset < len(token); offset++ {
		if data[start+offset]|0x20 != token[offset]|0x20 {
			return false
		}
	}
	return true
}
