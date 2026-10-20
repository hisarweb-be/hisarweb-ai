package service

import (
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/gin-gonic/gin"
)

// Supplier prompt overhead. The supplier of the primary channels adds a fixed
// number of prompt tokens to every request without the customer having sent
// them (measured 20 Oct 2026: about 790-800 tokens, independent of the prompt
// length). The customer is billed on the usage the supplier reports, so those
// tokens must be subtracted before the reported prompt becomes quota. The
// correction is configured by the operator, never guessed: one measurement with
// a customer key per model yields the constant.
//
// It applies only to channels tagged as the primary supplier, and only where the
// customer is charged: the settlement below and the customer-visible usage both
// use HwCorrectUpstreamPrompt, so the two cannot disagree.
const (
	hwPromptOverheadWildcard = "*"
)

const (
	// hwChannelTagContextKey carries the tag of the channel that answered the
	// request. It is set when the channel is selected, so the response mask and
	// the settlement read the same supplier.
	hwChannelTagContextKey = "hw_channel_tag"
	// hwPromptEstimateContextKey carries the local estimate of the request's
	// prompt tokens, the floor a corrected count may not fall below.
	hwPromptEstimateContextKey = "hw_local_prompt_estimate"
)

// HwNoteChannelTag records which supplier answered, for the accounting rules
// that only apply to the primary supplier.
func HwNoteChannelTag(c *gin.Context, tag string) {
	if c == nil {
		return
	}
	c.Set(hwChannelTagContextKey, tag)
}

// HwNoteLocalPromptEstimate records the local prompt estimate of the request.
func HwNoteLocalPromptEstimate(c *gin.Context, tokens int) {
	if c == nil {
		return
	}
	c.Set(hwPromptEstimateContextKey, tokens)
}

// HwLocalPromptEstimate returns the recorded local prompt estimate of the
// request, or 0 when the request never reached the pre-consume step.
func HwLocalPromptEstimate(c *gin.Context) int {
	if c == nil {
		return 0
	}
	return c.GetInt(hwPromptEstimateContextKey)
}

// hwPromptOverheadForModel returns the configured overhead of one model.
func hwPromptOverheadForModel(modelName string) int {
	raw := hwOptionValue(model.HwUpstreamPromptOverheadOption)
	if raw == "" {
		return 0
	}
	overheads := make(map[string]int)
	if err := common.UnmarshalJsonStr(raw, &overheads); err != nil {
		hwWarnPromptOverheadOption(raw)
		return 0
	}
	overhead := overheads[modelName]
	if overhead <= 0 {
		overhead = overheads[hwPromptOverheadWildcard]
	}
	if overhead <= 0 {
		return 0
	}
	return overhead
}

// hwWarnPromptOverheadOption reports a malformed option once per value instead
// of once per settled request.
var hwPromptOverheadWarned atomic.Pointer[string]

func hwWarnPromptOverheadOption(raw string) {
	if last := hwPromptOverheadWarned.Load(); last != nil && *last == raw {
		return
	}
	value := raw
	hwPromptOverheadWarned.Store(&value)
	common.SysError(common.LogText("HwUpstreamPromptOverhead is not a JSON object of model name to tokens; no prompt overhead is applied"))
}

// HwCorrectUpstreamPrompt turns a supplier-reported prompt count into the count
// the customer pays for. Without a configured overhead, or on a channel that is
// not the primary supplier, the reported count stands. A corrected count never
// goes below zero and never below the local estimate of the request when that
// estimate is larger: the supplier cannot make us charge for nothing, and our
// own count is the floor.
func HwCorrectUpstreamPrompt(c *gin.Context, modelName string, reported, localEstimate int) (corrected, overhead int) {
	if c == nil || reported <= 0 {
		return reported, 0
	}
	if !hwChannelIsPrimarySupplier(c) {
		return reported, 0
	}
	overhead = hwPromptOverheadForModel(modelName)
	if overhead <= 0 {
		return reported, 0
	}
	corrected = max(reported-overhead, 0)
	if localEstimate > corrected {
		corrected = localEstimate
	}
	return corrected, overhead
}

// hwChannelIsPrimarySupplier reports whether the channel that answered carries
// the primary supplier's tag. An untagged or different channel is untouched.
func hwChannelIsPrimarySupplier(c *gin.Context) bool {
	return c.GetString(hwChannelTagContextKey) == HwProviderLumia
}

// hwPromptOverheadCorrection describes one applied correction for the admin log.
type hwPromptOverheadCorrection struct {
	Applied  bool
	Reported int
	Overhead int
	Billed   int
}

// hwCorrectUsagePromptOverhead applies the supplier overhead to the usage the
// settlement bills on. The returned usage is a copy when a correction applies,
// so the caller's raw usage keeps the supplier's numbers for the admin log.
func hwCorrectUsagePromptOverhead(c *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage) (*dto.Usage, hwPromptOverheadCorrection) {
	if usage == nil || relayInfo == nil {
		return usage, hwPromptOverheadCorrection{}
	}
	corrected, overhead := HwCorrectUpstreamPrompt(c, relayInfo.OriginModelName, usage.PromptTokens, HwLocalPromptEstimate(c))
	if overhead <= 0 || corrected == usage.PromptTokens {
		return usage, hwPromptOverheadCorrection{Reported: usage.PromptTokens}
	}
	adjusted := *usage
	adjusted.PromptTokens = corrected
	adjusted.TotalTokens = corrected + usage.CompletionTokens
	// InputTokens counts the same prompt in the canonical shape (it adds the
	// cache reads and writes for Anthropic). Remove the same tokens there so the
	// two counts stay one story; cache tokens themselves are untouched.
	removed := usage.PromptTokens - corrected
	adjusted.InputTokens = usage.InputTokens
	if removed > 0 && adjusted.InputTokens > 0 {
		adjusted.InputTokens = max(adjusted.InputTokens-removed, 0)
	}
	return &adjusted, hwPromptOverheadCorrection{
		Applied:  true,
		Reported: usage.PromptTokens,
		Overhead: overhead,
		Billed:   corrected,
	}
}

// hwAppendPromptOverheadAdminInfo keeps the supplier's own prompt count in the
// admin-only part of the consume log, so a correction never hides what the
// supplier reported.
func hwAppendPromptOverheadAdminInfo(other *model.LogOther, correction hwPromptOverheadCorrection) {
	if other == nil || !correction.Applied {
		return
	}
	other.SetAdmin("prompt_tokens_upstream", correction.Reported)
	other.SetAdmin("prompt_overhead_tokens", correction.Overhead)
	other.SetAdmin("prompt_tokens_billed", correction.Billed)
}
