package service

import (
	"errors"
	"net/http"
	"net/url"
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
// emergency supplier (OpenRouter) is reachable only for the paying group. The
// policy is intentionally a separate state machine: the existing relay loop and
// channel selection keep their behaviour and only ask this file who goes next.
const (
	// HwProviderLumia is the tag of the primary supplier channel.
	HwProviderLumia = "lumia"
	// HwProviderOpenRouter is the tag of the emergency supplier channel.
	HwProviderOpenRouter = "openrouter"
	// HwProviderInternal marks channels without a supplier tag. They are not
	// part of supplier routing and are never chosen or blocked by this policy.
	HwProviderInternal = "internal"
	// HwPaidGroup is the only group allowed to reach the emergency supplier.
	HwPaidGroup = "paid"

	hwRouteContextKey = "hw_provider_route"
	hwMaxUpstreamIDs  = 5
)

// HwRouteState is the per-request supplier route. It lives in the gin context
// and is only touched by the request goroutine: selection, retry, logging and
// masking all run there, so it needs no lock and must never be shared.
type HwRouteState struct {
	active     bool
	group      string
	userGroup  string
	model      string
	family     string
	lumia      []*model.Channel
	openRouter []*model.Channel
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
	maskAlerts      int
	upstreamIDs     []string
}

// HwChannelProvider maps a channel onto its supplier role. The tag is the
// primary signal, but an emergency supplier must also be recognised without an
// exact tag: a channel of the OpenRouter type, a tag that merely prefixes the
// supplier name (openrouter-eu) and a base URL that points at the supplier host
// all count. Missing one of those would let a free account reach the emergency
// supplier, so the check is deliberately broad.
func HwChannelProvider(channel *model.Channel) string {
	if channel == nil {
		return ""
	}
	tag := strings.ToLower(strings.TrimSpace(channel.GetTag()))
	switch {
	case tag == HwProviderLumia:
		return HwProviderLumia
	case tag == HwProviderOpenRouter, strings.HasPrefix(tag, HwProviderOpenRouter+"-"):
		return HwProviderOpenRouter
	}
	if channel.Type == constant.ChannelTypeOpenRouter || hwChannelHostsOpenRouter(channel.GetBaseURL()) {
		return HwProviderOpenRouter
	}
	return HwProviderInternal
}

// hwChannelHostsOpenRouter reports whether a channel base URL points at the
// emergency supplier's host.
func hwChannelHostsOpenRouter(baseURL string) bool {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return false
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "openrouter.ai" || strings.HasSuffix(host, ".openrouter.ai")
}

// HwChannelAllowedForGroup enforces the hard rule that the emergency supplier
// is only reachable for the paying group. Both the group the token uses and the
// group of the account must be paying: a free account whose token stands on the
// paid group must not reach the emergency supplier either. It is checked while
// building the route and again whenever a channel is picked, so a wrongly
// tagged channel in a free group can never be used even if the route was built
// from stale data.
func HwChannelAllowedForGroup(channel *model.Channel, usingGroup, userGroup string) bool {
	if HwChannelProvider(channel) != HwProviderOpenRouter {
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
			// Wrongly tagged emergency channel in a non-paying group: filtered
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
		if provider == HwProviderOpenRouter {
			state.openRouter = append(state.openRouter, channel)
		} else {
			state.lumia = append(state.lumia, channel)
		}
	}
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
	if provider != HwProviderLumia && provider != HwProviderOpenRouter {
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
	if s.outage && !s.lumiaRetried && s.lastChannel != nil {
		return true
	}
	return s.peekOpenRouter() != nil
}

func (s *HwRouteState) peekLumia() *model.Channel {
	for _, channel := range s.lumia {
		if !s.tried[channel.Id] && HwChannelAllowedForGroup(channel, s.group, s.userGroup) {
			return channel
		}
	}
	return nil
}

func (s *HwRouteState) peekOpenRouter() *model.Channel {
	for _, channel := range s.openRouter {
		if !s.tried[channel.Id] && HwChannelAllowedForGroup(channel, s.group, s.userGroup) {
			return channel
		}
	}
	return nil
}

// next returns the next channel to try, or nil when the route is exhausted.
//
// Order: every Lumia alias of the requested family once, then exactly one extra
// Lumia attempt after an outage (after the fixed pause), then the emergency
// supplier for the paying group, then nothing at all. Another family is never
// part of the route.
func (s *HwRouteState) next() *model.Channel {
	if s == nil || s.finished {
		return nil
	}
	if channel := s.peekLumia(); channel != nil {
		s.tried[channel.Id] = true
		s.recordAttempt(channel, HwProviderLumia)
		return channel
	}
	if s.outage && !s.lumiaRetried && s.lastChannel != nil && HwChannelProvider(s.lastChannel) == HwProviderLumia {
		s.lumiaRetried = true
		hwPauseBeforeLumiaRetry()
		s.recordAttempt(s.lastChannel, HwProviderLumia)
		return s.lastChannel
	}
	if channel := s.peekOpenRouter(); channel != nil {
		s.tried[channel.Id] = true
		s.recordAttempt(channel, HwProviderOpenRouter)
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
