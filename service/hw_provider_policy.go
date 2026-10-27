package service

import (
	"errors"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

// Supplier routing. Customer traffic is served by the Lumia supplier first; the
// alternative suppliers (Portix, Kie, OpenRouter) are reachable only for the
// paying group and come after Lumia, cheapest first per model. The policy is
// intentionally a separate state machine: the existing relay loop and channel
// selection keep their behaviour and only ask this file who goes next.
const (
	// HwProviderLumia is the tag of the primary supplier channel.
	HwProviderLumia = "lumia"
	// HwProviderOpenRouter is the tag of the emergency supplier channel.
	HwProviderOpenRouter = "openrouter"
	// HwProviderPortix is the tag of the Portix reseller channel. Like the
	// emergency supplier it is an alternative route for the paying group only.
	HwProviderPortix = "portix"
	// HwProviderKie is the tag of the Kie reseller channel (paying group only).
	HwProviderKie = "kie"
	// HwProviderInternal marks channels without a supplier tag. They are not
	// part of supplier routing and are never chosen or blocked by this policy.
	HwProviderInternal = "internal"
	// HwPaidGroup is the only group allowed to reach the alternative suppliers.
	HwPaidGroup = "paid"
	// HwChannelCostKey is the other_info key that holds the supplier cost per
	// model of a channel: {"<model>": {"in": <USD per 1M>, "out": <USD per 1M>}}.
	// The model is the upstream name after the channel's model mapping, or the
	// customer model name when the channel does not rename it.
	HwChannelCostKey = "hw_cost_per_m"

	hwRouteContextKey = "hw_provider_route"
	hwMaxUpstreamIDs  = 5
)

// HwRouteState is the per-request supplier route. It lives in the gin context.
// Selection, retry and logging run on the request goroutine; the response mask
// additionally appends facts (stream markers, upstream ids, mask alerts) from
// the stream writer goroutines of relay/helper's stream scanner. Those writers
// run under the scanner's writeMutex and join the request goroutine through
// its wg.Wait before the log is written, so access stays race-free without
// this state carrying a lock of its own.
type HwRouteState struct {
	active     bool
	group      string
	userGroup  string
	model      string
	family     string
	lumia      []*model.Channel
	alternates []*model.Channel
	tried      map[int]bool

	outage          bool
	lumiaRetried    bool
	lastChannel     *model.Channel
	finished        bool
	blockedByFamily int
	attempts        int
	provider        string
	channelID       int
	fallback        bool
	streamWrites    int
	streamCut       bool
	abandonedPrompt int
	maskAlerts      int
	upstreamIDs     []string
}

// HwChannelProvider maps a channel onto its supplier role. The tag is the
// primary signal, but an alternative supplier must also be recognised without an
// exact tag: a channel of the OpenRouter type, a tag that merely prefixes the
// supplier name (openrouter-eu) and a base URL that points at the supplier host
// all count. Missing one of those would let a free account reach an alternative
// supplier, so the check is deliberately broad.
func HwChannelProvider(channel *model.Channel) string {
	if channel == nil {
		return ""
	}
	tag := strings.ToLower(strings.TrimSpace(channel.GetTag()))
	if tag == HwProviderLumia {
		return HwProviderLumia
	}
	for _, provider := range hwAlternateProviders {
		if tag == provider || strings.HasPrefix(tag, provider+"-") {
			return provider
		}
	}
	if channel.Type == constant.ChannelTypeOpenRouter {
		return HwProviderOpenRouter
	}
	baseURL := channel.GetBaseURL()
	for _, provider := range hwAlternateProviders {
		if hwChannelHostsProvider(baseURL, provider) {
			return provider
		}
	}
	return HwProviderInternal
}

// hwAlternateProviders are the suppliers that come after Lumia. All of them are
// reachable for the paying group only.
var hwAlternateProviders = []string{HwProviderOpenRouter, HwProviderPortix, HwProviderKie}

// hwProviderDomains maps an alternative supplier onto the domains whose hosts
// (and subdomains) count as that supplier.
var hwProviderDomains = map[string][]string{
	HwProviderOpenRouter: {"openrouter.ai"},
	HwProviderPortix:     {"portixapi.com"},
	HwProviderKie:        {"kie.ai"},
}

// hwChannelHostsProvider reports whether a channel base URL points at the host
// of an alternative supplier.
func hwChannelHostsProvider(baseURL, provider string) bool {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return false
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	for _, domain := range hwProviderDomains[provider] {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

// hwIsAlternateProvider reports whether provider is a supplier behind Lumia.
func hwIsAlternateProvider(provider string) bool {
	return provider == HwProviderOpenRouter || provider == HwProviderPortix || provider == HwProviderKie
}

// hwChannelCost returns the supplier cost of serving customerModel on channel:
// input plus output price per 1M tokens in USD. A channel without cost metadata
// for the model returns +Inf, so it sorts behind every channel with a known
// price instead of being preferred on a guess.
func hwChannelCost(channel *model.Channel, customerModel string) float64 {
	if channel == nil {
		return math.Inf(1)
	}
	costs, ok := channel.GetOtherInfo()[HwChannelCostKey].(map[string]any)
	if !ok {
		return math.Inf(1)
	}
	names := []string{customerModel}
	if mappingJSON := channel.GetModelMapping(); mappingJSON != "" && mappingJSON != "{}" {
		mapping := make(map[string]string)
		if err := common.UnmarshalJsonStr(mappingJSON, &mapping); err == nil {
			if mapped := strings.TrimSpace(mapping[customerModel]); mapped != "" {
				names = []string{mapped, customerModel}
			}
		}
	}
	for _, name := range names {
		entry, ok := costs[name].(map[string]any)
		if !ok {
			continue
		}
		input, inOK := entry["in"].(float64)
		output, outOK := entry["out"].(float64)
		if !inOK || !outOK || input < 0 || output < 0 || math.IsNaN(input) || math.IsNaN(output) {
			continue
		}
		return input + output
	}
	return math.Inf(1)
}

// HwChannelAllowedForGroup enforces the hard rule that the alternative suppliers
// (OpenRouter, Portix, Kie) are only reachable for the paying group. Both the group the token uses and the
// group of the account must be paying: a free account whose token stands on the
// paid group must not reach an alternative supplier either. It is checked while
// building the route and again whenever a channel is picked, so a wrongly
// tagged channel in a free group can never be used even if the route was built
// from stale data.
func HwChannelAllowedForGroup(channel *model.Channel, usingGroup, userGroup string) bool {
	if !hwIsAlternateProvider(HwChannelProvider(channel)) {
		return true
	}
	return usingGroup == HwPaidGroup && userGroup == HwPaidGroup
}

// hwChannelServesFamily reports whether channel serves customerModel inside the
// same model family. The channel's own model mapping decides which upstream
// model the customer name becomes on that channel.
func hwChannelServesFamily(channel *model.Channel, customerModel, family string) bool {
	if channel == nil || family == "" {
		return false
	}
	upstreamModel := customerModel
	if mappingJSON := channel.GetModelMapping(); mappingJSON != "" && mappingJSON != "{}" {
		mapping := make(map[string]string)
		if err := common.UnmarshalJsonStr(mappingJSON, &mapping); err == nil {
			if mapped, ok := mapping[customerModel]; ok && strings.TrimSpace(mapped) != "" {
				upstreamModel = mapped
			}
		}
	}
	return HwModelFamily(upstreamModel) == family
}

// HwRoute returns the supplier route of this request, or nil when supplier
// routing does not apply to it.
func HwRoute(c *gin.Context) *HwRouteState {
	if c == nil {
		return nil
	}
	value, ok := c.Get(hwRouteContextKey)
	if !ok {
		return nil
	}
	state, _ := value.(*HwRouteState)
	if state == nil || !state.active {
		return nil
	}
	return state
}

// HwCustomerModel returns the model name the customer asked for.
func HwCustomerModel(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if state := HwRoute(c); state != nil && state.model != "" {
		return state.model
	}
	return c.GetString("original_model")
}

// hwStartRoute builds the route once. It returns nil when the group+model has no
// supplier-tagged channel at all, so every deployment without supplier tags
// keeps the stock selection untouched.
func hwStartRoute(c *gin.Context, group, modelName string, filters []dto.ChannelFilter) *HwRouteState {
	if c == nil || modelName == "" {
		return nil
	}
	if group == "" {
		group = common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	}
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	if group == "" {
		group = userGroup
	}

	candidates := model.HwChannelsForGroupModel(group, modelName)
	if len(candidates) == 0 {
		return nil
	}
	tagged := 0
	for _, channel := range candidates {
		if HwChannelProvider(channel) != HwProviderInternal {
			tagged++
		}
	}
	if tagged == 0 {
		return nil
	}

	state := &HwRouteState{
		active:    true,
		group:     group,
		userGroup: userGroup,
		model:     modelName,
		family:    HwModelFamily(modelName),
		tried:     make(map[int]bool, len(candidates)),
	}
	for _, channel := range candidates {
		provider := HwChannelProvider(channel)
		if provider == HwProviderInternal {
			continue
		}
		if !HwChannelAllowedForGroup(channel, group, userGroup) {
			// Wrongly tagged alternative channel in a non-paying group: filtered
			// out here and again at pick time.
			continue
		}
		if !hwChannelServesFamily(channel, modelName, state.family) {
			state.blockedByFamily++
			continue
		}
		if ok, _ := model.ChannelSatisfiesFilters(channel, modelName, filters); !ok {
			continue
		}
		if hwIsAlternateProvider(provider) {
			state.alternates = append(state.alternates, channel)
		} else {
			state.lumia = append(state.lumia, channel)
		}
	}
	// Cheapest alternative first for this model; the stable sort keeps the
	// priority and id order of the candidates for equal or unknown prices.
	sort.SliceStable(state.alternates, func(i, j int) bool {
		return hwChannelCost(state.alternates[i], modelName) < hwChannelCost(state.alternates[j], modelName)
	})
	c.Set(hwRouteContextKey, state)
	return state
}

// claim marks channel as this attempt's channel when it belongs to the route.
// It reports false when the route has to pick a channel itself, which happens
// when the stock selection landed on an untagged or foreign-family channel.
func (s *HwRouteState) claim(channel *model.Channel) bool {
	if s == nil || channel == nil {
		return false
	}
	provider := HwChannelProvider(channel)
	if provider != HwProviderLumia && !hwIsAlternateProvider(provider) {
		return false
	}
	if !HwChannelAllowedForGroup(channel, s.group, s.userGroup) {
		return false
	}
	if !hwChannelServesFamily(channel, s.model, s.family) {
		return false
	}
	s.tried[channel.Id] = true
	s.recordAttempt(channel, provider)
	return true
}

// recordAttempt updates the facts the admin log and audit trail report.
func (s *HwRouteState) recordAttempt(channel *model.Channel, provider string) {
	s.attempts++
	if s.attempts > 1 {
		s.fallback = true
	}
	s.provider = provider
	s.channelID = channel.Id
	s.lastChannel = channel
}

// hasNext reports whether another upstream attempt remains without consuming it.
func (s *HwRouteState) hasNext() bool {
	if s == nil || !s.active || s.finished {
		return false
	}
	if s.streamCut {
		// The customer already received data; switching supplier now would
		// splice two different answers together.
		return false
	}
	if next := s.peekLumia(); next != nil {
		return true
	}
	if s.outageRetryPending() {
		return true
	}
	return s.peekAlternate() != nil
}

// outageRetryPending reports whether the single extra Lumia attempt after an
// outage still remains. next is the only consumer that can deliver it, so
// hasNext asks the same question: a route whose last attempt failed on another
// supplier has no retry left and must end with that attempt's own error.
func (s *HwRouteState) outageRetryPending() bool {
	return s.outage && !s.lumiaRetried && s.lastChannel != nil &&
		HwChannelProvider(s.lastChannel) == HwProviderLumia
}

func (s *HwRouteState) peekLumia() *model.Channel {
	for _, channel := range s.lumia {
		if !s.tried[channel.Id] && HwChannelAllowedForGroup(channel, s.group, s.userGroup) {
			return channel
		}
	}
	return nil
}

func (s *HwRouteState) peekAlternate() *model.Channel {
	for _, channel := range s.alternates {
		if !s.tried[channel.Id] && HwChannelAllowedForGroup(channel, s.group, s.userGroup) {
			return channel
		}
	}
	return nil
}

// next returns the next channel to try, or nil when the route is exhausted.
//
// Order: every Lumia alias of the requested family once, then exactly one extra
// Lumia attempt after an outage (after the fixed pause), then the alternative
// suppliers for the paying group (Portix, Kie, OpenRouter), the cheapest for
// this model first, then nothing at all. Another family is never part of the
// route.
func (s *HwRouteState) next() *model.Channel {
	if s == nil || s.finished {
		return nil
	}
	if channel := s.peekLumia(); channel != nil {
		s.tried[channel.Id] = true
		s.recordAttempt(channel, HwProviderLumia)
		return channel
	}
	if s.outageRetryPending() {
		s.lumiaRetried = true
		hwPauseBeforeLumiaRetry()
		s.recordAttempt(s.lastChannel, HwProviderLumia)
		return s.lastChannel
	}
	if channel := s.peekAlternate(); channel != nil {
		s.tried[channel.Id] = true
		s.recordAttempt(channel, HwChannelProvider(channel))
		return channel
	}
	return nil
}

// hwPauseBeforeLumiaRetry waits the configured pause, but never longer than the
// caller is prepared to wait for a broken supplier.
func hwPauseBeforeLumiaRetry() {
	if common.HwLumiaRetryPauseMs <= 0 {
		return
	}
	time.Sleep(time.Duration(common.HwLumiaRetryPauseMs) * time.Millisecond)
}

// HwFirstChannel validates the channel the distributor already picked and
// replaces it when supplier rules forbid it. Returning (nil, nil) keeps the
// picked channel untouched.
func HwFirstChannel(c *gin.Context, current *model.Channel, group, modelName string) (*model.Channel, *types.NewAPIError) {
	if c == nil || current == nil {
		return nil, nil
	}
	state := hwStartRoute(c, group, modelName, GetChannelConstraints(c).Filters)
	if state == nil {
		return nil, nil
	}
	if state.claim(current) {
		return nil, nil
	}
	channel := state.next()
	if channel == nil {
		state.finished = true
		return nil, HwServiceBusyError(c)
	}
	return channel, nil
}

// HwSelectChannel serves the retry path of the relay loop. The stock selection
// runs once (when the route is not built yet) so auto-group handling and the
// cache stay authoritative; after that the route decides.
func HwSelectChannel(c *gin.Context, retryParam *RetryParam, stock func() (*model.Channel, string, error)) (*model.Channel, string, error) {
	modelName := ""
	if retryParam != nil {
		modelName = retryParam.ModelName
	}
	state := HwRoute(c)
	if state == nil {
		channel, group, err := stock()
		if err != nil || channel == nil {
			return channel, group, err
		}
		state = hwStartRoute(c, group, modelName, GetChannelConstraints(c).Filters)
		if state == nil {
			return channel, group, nil
		}
		if state.claim(channel) {
			return channel, group, nil
		}
	}
	channel := state.next()
	if channel == nil {
		state.finished = true
		return nil, state.group, HwServiceBusyError(c)
	}
	return channel, state.group, nil
}

// HwRetryLoopExtends tells the relay loop that the supplier route still has an
// attempt left, independently of the configured RetryTimes.
func HwRetryLoopExtends(c *gin.Context) bool {
	state := HwRoute(c)
	if state == nil {
		return false
	}
	return state.hasNext()
}

// HwDecideRetry replaces the generic retry decision for supplier-routed
// requests. It also records the failure class that drives the outage retry.
func HwDecideRetry(c *gin.Context, apiErr *types.NewAPIError) (PolicyDecision, bool) {
	state := HwRoute(c)
	if state == nil {
		return PolicyDecision{}, false
	}
	if apiErr == nil {
		return hwPolicyDecision("stop", "hw_no_error"), true
	}
	if types.IsSkipRetryError(apiErr) || apiErr.GetErrorCode() == HwErrorCodeServiceBusy {
		state.finished = true
		return hwPolicyDecision("stop", "hw_terminal_error"), true
	}
	if hwIsOutageError(apiErr) {
		state.outage = true
	}
	if state.streamWrites > 0 {
		// The customer already received data; switching supplier now would
		// splice two different answers together.
		state.streamCut = true
	}
	if !state.hasNext() {
		state.finished = true
		return hwPolicyDecision("stop", "hw_route_exhausted"), true
	}
	return hwPolicyDecision("retry", "hw_supplier_route"), true
}

func hwPolicyDecision(action, reason string) PolicyDecision {
	return PolicyDecision{Action: action, Reason: reason, Source: "supplier_policy"}
}

// hwIsOutageError classifies supplier failures that justify the extra Lumia
// attempt: timeouts, connection failures, 5xx and 429.
func hwIsOutageError(apiErr *types.NewAPIError) bool {
	if apiErr == nil {
		return false
	}
	if apiErr.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if apiErr.StatusCode >= http.StatusInternalServerError && apiErr.StatusCode <= 599 {
		return true
	}
	switch apiErr.GetErrorCode() {
	case types.ErrorCodeDoRequestFailed,
		types.ErrorCodeReadResponseBodyFailed,
		types.ErrorCodeBadResponse,
		types.ErrorCodeBadResponseStatusCode,
		HwErrorCodeStreamInterrupted:
		return true
	}
	return false
}

// hwSupplierStreamCut classifies a supplier stream that ended without its
// protocol terminal event. visible reports that the customer already received
// output, which decides between an error event and a retry.
func hwSupplierStreamCut(c *gin.Context, info *relaycommon.RelayInfo) (visible, cut bool) {
	state := HwRoute(c)
	if state == nil || info == nil || !info.IsStream || info.StreamStatus == nil {
		return false, false
	}
	outcome := info.StreamStatus.OutcomeSnapshot()
	if !outcome.ExpectsTerminal || outcome.Response != relaycommon.ResponseOutcomeUnknown {
		return false, false
	}
	switch outcome.EndReason {
	case relaycommon.StreamEndReasonEOF,
		relaycommon.StreamEndReasonScannerErr,
		relaycommon.StreamEndReasonTimeout:
	default:
		return false, false
	}
	return info.ReceivedResponseCount > 0 || state.streamWrites > 0, true
}

// HwStreamCutShort detects a supplier stream that ended before its protocol
// terminal event. With chunks already sent the customer gets a clean error
// event and no retry; without chunks the normal supplier order continues.
func HwStreamCutShort(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	visible, cut := hwSupplierStreamCut(c, info)
	if !cut {
		return nil
	}
	if visible {
		if state := HwRoute(c); state != nil {
			state.streamCut = true
		}
		return types.NewErrorWithStatusCode(
			errors.New("supplier stream ended before completion"),
			HwErrorCodeStreamInterrupted,
			http.StatusBadGateway,
			types.ErrOptionWithSkipRetry(),
		)
	}
	return types.NewErrorWithStatusCode(
		errors.New("supplier stream ended before its first chunk"),
		HwErrorCodeStreamInterrupted,
		http.StatusBadGateway,
	)
}

// HwStreamCutBeforeOutput reports whether this attempt is a supplier stream that
// ended before its terminal event while the customer had not seen any output
// yet. The route retries such an attempt, so it must not be settled: settling it
// charges the prompt estimate of an answer nobody saw and turns the settlement
// of the retry into a no-op, which leaves the completion tokens unbilled.
func HwStreamCutBeforeOutput(c *gin.Context, info *relaycommon.RelayInfo) bool {
	visible, cut := hwSupplierStreamCut(c, info)
	return cut && !visible
}

// HwNoteStreamWrite records that a stream chunk reached the customer. It is
// called by the masking writer, which is the only component that knows what was
// actually written.
func HwNoteStreamWrite(c *gin.Context) {
	if state := HwRoute(c); state != nil {
		state.streamWrites++
	}
}

// HwNoteAbandonedAttempt keeps the supplier prompt count of an attempt whose
// stream ended before the customer saw output. That attempt is retried and not
// settled, but the supplier may still invoice its prompt; the count lands in the
// admin-only part of the request's single log so the invoice can be reconciled.
func HwNoteAbandonedAttempt(c *gin.Context, promptTokens int) {
	state := HwRoute(c)
	if state == nil || promptTokens <= 0 {
		return
	}
	state.abandonedPrompt += promptTokens
}

// HwNoteMaskAlert records that the last-resort scan had to rewrite provider
// traces. Only the count is logged; the content never is.
func HwNoteMaskAlert(c *gin.Context) {
	if state := HwRoute(c); state != nil {
		state.maskAlerts++
	}
}

// HwNoteUpstreamIdentity keeps the upstream response ids for the admin log, so
// the customer-visible id can be our own.
func HwNoteUpstreamIdentity(c *gin.Context, id string) {
	state := HwRoute(c)
	if state == nil {
		return
	}
	identity := strings.TrimSpace(id)
	if identity == "" {
		return
	}
	for _, existing := range state.upstreamIDs {
		if existing == identity {
			return
		}
	}
	if len(state.upstreamIDs) < hwMaxUpstreamIDs {
		state.upstreamIDs = append(state.upstreamIDs, identity)
	}
}

// HwAppendRelayLogAdminInfo writes the supplier facts into the admin-only part
// of the request log. The customer projection of a log line never contains them.
func HwAppendRelayLogAdminInfo(c *gin.Context, other *model.LogOther) {
	state := HwRoute(c)
	if state == nil || other == nil || state.channelID == 0 {
		return
	}
	other.SetAdmin("provider", state.provider)
	other.SetAdmin("provider_channel_id", state.channelID)
	other.SetAdmin("provider_attempts", state.attempts)
	other.SetAdmin("provider_fallback", state.fallback)
	if state.blockedByFamily > 0 {
		other.SetAdmin("provider_family_blocked", state.blockedByFamily)
	}
	if len(state.upstreamIDs) > 0 {
		other.SetAdmin("provider_upstream_ids", state.upstreamIDs)
	}
	if state.streamCut {
		other.SetAdmin("provider_stream_interrupted", true)
	}
	if state.abandonedPrompt > 0 {
		other.SetAdmin("provider_abandoned_prompt_tokens", state.abandonedPrompt)
	}
	if state.maskAlerts > 0 {
		other.SetAdmin("provider_mask_alerts", state.maskAlerts)
	}
}

// HwTerminalError replaces an exhausted supplier route's last upstream error// HwTerminalError replaces an exhausted supplier route's last upstream error
// with the fork's own service_busy error, so an outage never surfaces upstream
// wording or a supplier-flavoured status. Client-side failures (4xx) keep their
// own class; masking replaces their text instead.
func HwTerminalError(c *gin.Context, apiErr *types.NewAPIError, decision PolicyDecision) *types.NewAPIError {
	state := HwRoute(c)
	if state == nil || apiErr == nil || decision.Action == "retry" {
		return nil
	}
	if HwIsOwnErrorCode(string(apiErr.GetErrorCode())) || types.IsSkipRetryError(apiErr) {
		return nil
	}
	if !hwIsOutageError(apiErr) {
		return nil
	}
	return HwServiceBusyError(c)
}
