package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// Customer-visible usage. A customer sees the token facts of the protocol it
// speaks and nothing else. The relay itself keeps more: the upstream billing
// snapshot (billing_usage), the semantics/source tags and the split cache-write
// counters are how we account for a request, never how we answer it. The mask
// writer strips everything outside the allowlist of the protocol on the route,
// so an upstream that adds its own fields cannot leak them.
//
// The allowlists follow the published usage objects:
//   - Anthropic Messages: input_tokens, output_tokens, cache_creation_input_tokens,
//     cache_read_input_tokens, cache_creation{ephemeral_5m_input_tokens,
//     ephemeral_1h_input_tokens}, server_tool_use{web_search_requests,
//     web_fetch_requests}, service_tier.
//   - OpenAI Chat Completions: prompt_tokens, completion_tokens, total_tokens,
//     prompt_tokens_details{cached_tokens, audio_tokens},
//     completion_tokens_details{reasoning_tokens, audio_tokens,
//     accepted_prediction_tokens, rejected_prediction_tokens}.
var (
	hwAnthropicUsageKeys = map[string]bool{
		"input_tokens":                true,
		"output_tokens":               true,
		"cache_creation_input_tokens": true,
		"cache_read_input_tokens":     true,
		"cache_creation":              true,
		"server_tool_use":             true,
		"service_tier":                true,
	}
	hwAnthropicCacheCreationKeys = map[string]bool{
		"ephemeral_5m_input_tokens": true,
		"ephemeral_1h_input_tokens": true,
	}
	hwAnthropicServerToolUseKeys = map[string]bool{
		"web_search_requests": true,
		"web_fetch_requests":  true,
	}
	hwOpenAIChatUsageKeys = map[string]bool{
		"prompt_tokens":             true,
		"completion_tokens":         true,
		"total_tokens":              true,
		"prompt_tokens_details":     true,
		"completion_tokens_details": true,
	}
	hwOpenAIPromptDetailsKeys = map[string]bool{
		"cached_tokens": true,
		"audio_tokens":  true,
	}
	hwOpenAICompletionDetailsKeys = map[string]bool{
		"reasoning_tokens":           true,
		"audio_tokens":               true,
		"accepted_prediction_tokens": true,
		"rejected_prediction_tokens": true,
	}
)

// hwOpenAIChatPath reports whether this request is answered in the OpenAI chat
// completion shape, whose usage allowlist applies.
func hwOpenAIChatPath(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	return strings.HasSuffix(c.Request.URL.Path, "/chat/completions")
}

// hwFilterCustomerUsage keeps only the usage fields the customer's protocol
// publishes. It runs on every decoded "usage" object of a masked answer.
func (w *hwMaskWriter) filterCustomerUsage(usage map[string]any) {
	switch {
	case w.anthropic:
		hwKeepOnly(usage, hwAnthropicUsageKeys)
		hwKeepOnlyNested(usage, "cache_creation", hwAnthropicCacheCreationKeys)
		hwKeepOnlyNested(usage, "server_tool_use", hwAnthropicServerToolUseKeys)
	case w.oaiChat:
		hwKeepOnly(usage, hwOpenAIChatUsageKeys)
		hwKeepOnlyNested(usage, "prompt_tokens_details", hwOpenAIPromptDetailsKeys)
		hwKeepOnlyNested(usage, "completion_tokens_details", hwOpenAICompletionDetailsKeys)
	}
}

func hwKeepOnly(object map[string]any, allowed map[string]bool) {
	for key := range object {
		if !allowed[key] {
			delete(object, key)
		}
	}
}

func hwKeepOnlyNested(object map[string]any, key string, allowed map[string]bool) {
	nested, ok := object[key].(map[string]any)
	if !ok {
		return
	}
	hwKeepOnly(nested, allowed)
}
