package service

import "strings"

// Model family names returned by HwModelFamily for the families that are
// allowed to fall back to one another.
const (
	HwFamilyClaudeOpus   = "claude-opus"
	HwFamilyClaudeSonnet = "claude-sonnet"
	HwFamilyClaudeHaiku  = "claude-haiku"
	HwFamilyGPT          = "gpt"
)

// HwModelFamily maps a model name onto the family that supplier fallback may
// move within. Two names in the same family are interchangeable for the
// customer; two different families never are, so a fallback can never answer a
// Sonnet request with an Opus, GPT or unrelated model.
//
// Vendor prefixes ("openrouter/anthropic/..."), provider suffixes
// ("...:free") and reasoning modifiers ("...@high") are stripped before the
// family is matched, because those decorate the same underlying model.
func HwModelFamily(customerModel string) string {
	name := strings.ToLower(strings.TrimSpace(customerModel))
	if name == "" {
		return ""
	}
	if idx := strings.IndexByte(name, '@'); idx >= 0 {
		name = strings.TrimSpace(name[:idx])
	}
	if idx := strings.LastIndexByte(name, '/'); idx >= 0 {
		name = strings.TrimSpace(name[idx+1:])
	}
	if idx := strings.IndexByte(name, ':'); idx >= 0 {
		name = strings.TrimSpace(name[:idx])
	}
	if name == "" {
		return ""
	}
	switch {
	case strings.HasPrefix(name, HwFamilyClaudeOpus):
		return HwFamilyClaudeOpus
	case strings.HasPrefix(name, HwFamilyClaudeSonnet):
		return HwFamilyClaudeSonnet
	case strings.HasPrefix(name, HwFamilyClaudeHaiku):
		return HwFamilyClaudeHaiku
	case strings.HasPrefix(name, "gpt-"):
		return HwFamilyGPT
	}
	return name
}

// hwModelVendor is the supplier segment of a model name: "claude" for
// claude-sonnet-5-5, "gpt" for gpt-6-luna. One key serves every name of a
// supplier, so the vendor is the widest fallback that can still answer a
// request; it is only used when no channel of the model's own family is left.
func hwModelVendor(customerModel string) string {
	name := strings.ToLower(strings.TrimSpace(customerModel))
	if idx := strings.IndexByte(name, '@'); idx >= 0 {
		name = strings.TrimSpace(name[:idx])
	}
	if idx := strings.LastIndexByte(name, '/'); idx >= 0 {
		name = strings.TrimSpace(name[idx+1:])
	}
	vendor, _, _ := strings.Cut(name, "-")
	return strings.TrimSpace(vendor)
}
