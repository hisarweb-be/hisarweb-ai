package service

import (
	"regexp"
	"strings"
)

// The upstream supplier adds a hidden prompt of its own, so a model sometimes
// calls itself a development tool or names a model that is not the one we run.
// A reply that does so is replaced by the fixed identity answer: the customer
// never sees a made-up persona or a model name. The sold model names are the
// customer-facing catalog and may appear; every other model name may not.
var hwAssistantLeakPattern = regexp.MustCompile(`(?i)\b(claude[- ]?(code|haiku|sonnet|opus|fable)|haiku|sonnet|opus|fable|gemini|gpt[- ]?\d|anthropic)\b|ontwikkelomgeving|development environment|geli[sş]tirme ortam|terminal ?(komut|command|opdracht)`)

// HwAssistantGuardReply returns the reply, or the identity answer when the
// reply leaks a model name or a made-up persona.
func HwAssistantGuardReply(reply, lang string) string {
	checked := reply
	for _, sold := range hwSoldModels {
		checked = strings.ReplaceAll(strings.ToLower(checked), strings.ToLower(sold.Name), " ")
	}
	if hwAssistantLeakPattern.MatchString(checked) {
		return HwAssistantIdentity[hwAssistantLang(lang)]
	}
	return reply
}
