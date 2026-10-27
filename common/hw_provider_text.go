package common

import "strings"

// HwProviderTraceTokens identify a supplier or proxy trace in a header name.
// Matching is case-insensitive.
var HwProviderTraceTokens = []string{"lumia", "openrouter", "x-or-", "litellm", "portix", "kie.ai", "kieai"}

// HwProviderTextTokens are the traces scanned in body text, header values and
// log text. They omit "x-or-": that token is a header-name pattern, and
// applying it to prose would rewrite ordinary words such as "box-or-".
var HwProviderTextTokens = []string{"lumia", "openrouter", "litellm", "portix", "kie.ai", "kieai"}

// HwProviderWordTokens are supplier names too short to match as a substring
// ("cookie", "Kiel"). They count as a trace only as a whole word: the
// characters on both sides must not be a letter or a digit.
var HwProviderWordTokens = []string{"kie"}

// HwProviderPlaceholder is the neutral word that replaces a supplier trace.
const HwProviderPlaceholder = "provider"

// HwHasProviderTrace reports whether a header name contains a supplier or proxy
// trace. Body text must use HwHasProviderTextTrace instead.
func HwHasProviderTrace(text string) bool {
	return hwHasProviderTrace(text, HwProviderTraceTokens)
}

// HwHasProviderTextTrace reports whether body text contains a supplier or proxy
// trace, without the header-name-only "x-or-" pattern.
func HwHasProviderTextTrace(text string) bool {
	return hwHasProviderTrace(text, HwProviderTextTokens)
}

func hwHasProviderTrace(text string, tokens []string) bool {
	lower := strings.ToLower(text)
	for _, token := range tokens {
		if strings.Contains(lower, token) {
			return true
		}
	}
	data := []byte(lower)
	for index := range data {
		if hwWordTokenAt(data, index) != "" {
			return true
		}
	}
	return false
}

// hwWordTokenAt returns the whole-word supplier name that starts at start, or
// an empty string when none does.
func hwWordTokenAt(data []byte, start int) string {
	for _, token := range HwProviderWordTokens {
		if !hwEqualFoldAt(data, start, token) {
			continue
		}
		end := start + len(token)
		if start > 0 && hwIsWordByte(data[start-1]) {
			continue
		}
		if end < len(data) && hwIsWordByte(data[end]) {
			continue
		}
		return token
	}
	return ""
}

// hwIsWordByte reports whether b continues a word. Bytes of multi-byte
// characters count as word bytes, so "kieł" is not mistaken for "kie".
func hwIsWordByte(b byte) bool {
	return b >= 0x80 || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

// HwReplaceProviderWords rewrites every supplier trace in a header value to the
// neutral placeholder. It is the last-resort net, not the primary masking:
// structural masking removes the fields that carry these names in the first
// place.
func HwReplaceProviderWords(text string) string {
	return hwReplaceProviderWords(text, HwProviderTraceTokens)
}

// HwReplaceProviderTextWords rewrites every supplier trace in body text to the
// neutral placeholder, without the header-name-only "x-or-" pattern.
func HwReplaceProviderTextWords(text string) string {
	return hwReplaceProviderWords(text, HwProviderTextTokens)
}

func hwReplaceProviderWords(text string, tokens []string) string {
	if text == "" || !hwHasProviderTrace(text, tokens) {
		return text
	}
	data := []byte(text)
	out := make([]byte, 0, len(data)+8)
	for index := 0; index < len(data); {
		matched := false
		for _, token := range tokens {
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
		if word := hwWordTokenAt(data, index); word != "" {
			out = append(out, HwProviderPlaceholder...)
			index += len(word)
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
