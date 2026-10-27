package service

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHwChannelProvider proves that the emergency supplier is recognised even
// when its channel carries no exact tag: a prefixed tag, the OpenRouter channel
// type or a base URL on the supplier's host all count. Missing one of those
// would let a free group relay through the emergency supplier.
func TestHwChannelProvider(t *testing.T) {
	testCases := []struct {
		name      string
		tag       string
		baseURL   string
		channelTy int
		want      string
	}{
		{name: "lumia tag", tag: "lumia", channelTy: constant.ChannelTypeOpenAI, want: HwProviderLumia},
		{name: "lumia tag in capitals", tag: " Lumia ", channelTy: constant.ChannelTypeOpenAI, want: HwProviderLumia},
		{name: "openrouter tag", tag: "openrouter", channelTy: constant.ChannelTypeOpenAI, want: HwProviderOpenRouter},
		{name: "openrouter tag in capitals", tag: "OpenRouter", channelTy: constant.ChannelTypeOpenAI, want: HwProviderOpenRouter},
		{name: "prefixed openrouter tag", tag: "openrouter-eu", channelTy: constant.ChannelTypeOpenAI, want: HwProviderOpenRouter},
		{name: "openrouter channel type", channelTy: constant.ChannelTypeOpenRouter, want: HwProviderOpenRouter},
		{name: "openrouter host", channelTy: constant.ChannelTypeOpenAI, baseURL: "https://openrouter.ai/api/v1", want: HwProviderOpenRouter},
		{name: "openrouter host with capitals", channelTy: constant.ChannelTypeOpenAI, baseURL: "HTTPS://OpenRouter.AI/api/v1", want: HwProviderOpenRouter},
		{name: "openrouter subdomain", channelTy: constant.ChannelTypeOpenAI, baseURL: "https://eu.openrouter.ai/api/v1", want: HwProviderOpenRouter},
		{name: "lookalike host", channelTy: constant.ChannelTypeOpenAI, baseURL: "https://openrouter.ai.example.com/v1", want: HwProviderInternal},
		{name: "untagged ordinary channel", channelTy: constant.ChannelTypeOpenAI, baseURL: "https://api.example.com/v1", want: HwProviderInternal},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			channel := &model.Channel{Type: testCase.channelTy}
			if testCase.tag != "" {
				channel.SetTag(testCase.tag)
			}
			if testCase.baseURL != "" {
				baseURL := testCase.baseURL
				channel.BaseURL = &baseURL
			}
			assert.Equal(t, testCase.want, HwChannelProvider(channel))
		})
	}

	assert.Empty(t, HwChannelProvider(nil))
}

// TestHwEmergencyChannelBlockedForFreeGroup proves that a channel recognised as
// the emergency supplier is refused for every non-paying group, whatever made
// it recognisable.
func TestHwEmergencyChannelBlockedForFreeGroup(t *testing.T) {
	tagless := &model.Channel{Type: constant.ChannelTypeOpenRouter}
	byHost := &model.Channel{Type: constant.ChannelTypeOpenAI}
	baseURL := "https://openrouter.ai/api/v1"
	byHost.BaseURL = &baseURL

	for _, channel := range []*model.Channel{tagless, byHost} {
		assert.False(t, HwChannelAllowedForGroup(channel, "free", "free"))
		assert.False(t, HwChannelAllowedForGroup(channel, "paid", "free"))
		assert.True(t, HwChannelAllowedForGroup(channel, "paid", "paid"))
	}
}

// TestHwRouteHasNextAgreesWithNext proves that an exhausted route never
// promises a retry it cannot deliver. hasNext only promises the extra outage
// attempt on a Lumia channel, which is the single condition next accepts, so a
// failed emergency attempt ends the loop with its own upstream error instead of
// being replaced by a service-busy error from a next() that returned nil.
func TestHwRouteHasNextAgreesWithNext(t *testing.T) {
	lumia := &model.Channel{Id: 11, Type: constant.ChannelTypeOpenAI}
	lumia.SetTag(HwProviderLumia)
	openRouter := &model.Channel{Id: 12, Type: constant.ChannelTypeOpenAI}
	openRouter.SetTag(HwProviderOpenRouter)

	testCases := []struct {
		name        string
		lastChannel *model.Channel
		openRouter  []*model.Channel
		wantNext    bool
	}{
		{
			// The reported case: the emergency supplier just failed, so there is
			// no attempt left and the last upstream error must survive.
			name:        "failed emergency attempt",
			lastChannel: openRouter,
		},
		{
			name:        "failed emergency attempt with an unused emergency channel",
			lastChannel: openRouter,
			openRouter:  []*model.Channel{{Id: 13, Type: constant.ChannelTypeOpenRouter}},
			wantNext:    true,
		},
		{
			// The outage retry still exists for the primary supplier.
			name:        "failed primary attempt",
			lastChannel: lumia,
			wantNext:    true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			previousPause := common.HwLumiaRetryPauseMs
			common.HwLumiaRetryPauseMs = 0
			t.Cleanup(func() { common.HwLumiaRetryPauseMs = previousPause })

			state := &HwRouteState{
				active:      true,
				group:       HwPaidGroup,
				userGroup:   HwPaidGroup,
				model:       "claude-sonnet-4-5",
				family:      HwModelFamily("claude-sonnet-4-5"),
				alternates:  testCase.openRouter,
				tried:       map[int]bool{testCase.lastChannel.Id: true},
				outage:      true,
				lastChannel: testCase.lastChannel,
			}
			state.recordAttempt(testCase.lastChannel, HwChannelProvider(testCase.lastChannel))

			require.Equal(t, testCase.wantNext, state.hasNext())

			channel := state.next()
			if !testCase.wantNext {
				require.Nil(t, channel, "hasNext promised a retry that next cannot deliver")
				return
			}
			require.NotNil(t, channel)
		})
	}

	// A nil last channel has no outage retry either: the route is exhausted.
	state := &HwRouteState{active: true, tried: map[int]bool{}, outage: true}
	assert.False(t, state.hasNext())
	assert.Nil(t, state.next())
}

// The task settlement log keeps the supplier-side alias out of the customer
// projection; the admin block of the same row keeps it for diagnosis.
func TestHwTaskBillingAliasIsAdminOnly(t *testing.T) {
	task := &model.Task{}
	task.Properties.OriginModelName = "own-model"
	task.Properties.UpstreamModelName = "supplier-alias-9"

	snapshot := taskBillingOther(task).Snapshot()
	assert.NotContains(t, snapshot, "upstream_model_name", "the alias must not sit in the customer-visible part")
	assert.Equal(t, true, snapshot["is_model_mapped"])
	adminInfo, ok := snapshot["admin_info"].(map[string]any)
	require.True(t, ok, "the admin block must carry the alias: %v", snapshot)
	assert.Equal(t, "supplier-alias-9", adminInfo["upstream_model_name"])
}

func hwPolicyChannel(id int, provider, modelMapping string) *model.Channel {
	channel := &model.Channel{Id: id, Type: constant.ChannelTypeOpenAI}
	channel.SetTag(provider)
	if modelMapping != "" {
		channel.ModelMapping = &modelMapping
	}
	return channel
}

func hwPolicyRoute(lumia, openRouter []*model.Channel) *HwRouteState {
	return &HwRouteState{
		active:     true,
		group:      HwPaidGroup,
		userGroup:  HwPaidGroup,
		model:      "claude-sonnet-4-5",
		family:     HwModelFamily("claude-sonnet-4-5"),
		lumia:      lumia,
		alternates: openRouter,
		tried:      map[int]bool{},
	}
}

func hwPolicyError(status int, code types.ErrorCode, opts ...types.NewAPIErrorOptions) *types.NewAPIError {
	return types.NewErrorWithStatusCode(errors.New("upstream failed"), code, status, opts...)
}

// Only transient supplier failures earn the extra Lumia attempt; a 4xx from the
// supplier is the customer's own problem and must not be retried as an outage.
func TestHwIsOutageErrorClassifiesTransientFailures(t *testing.T) {
	assert.False(t, hwIsOutageError(nil))
	assert.True(t, hwIsOutageError(hwPolicyError(http.StatusTooManyRequests, "other")))
	assert.True(t, hwIsOutageError(hwPolicyError(http.StatusBadGateway, "other")))
	assert.True(t, hwIsOutageError(hwPolicyError(599, "other")))
	assert.False(t, hwIsOutageError(hwPolicyError(600, "other")))
	assert.False(t, hwIsOutageError(hwPolicyError(http.StatusBadRequest, "invalid_request")))
	for _, code := range []types.ErrorCode{
		types.ErrorCodeDoRequestFailed,
		types.ErrorCodeReadResponseBodyFailed,
		types.ErrorCodeBadResponse,
		types.ErrorCodeBadResponseStatusCode,
		HwErrorCodeStreamInterrupted,
	} {
		assert.True(t, hwIsOutageError(hwPolicyError(http.StatusBadRequest, code)), string(code))
	}
}

// HwDecideRetry owns the retry decision of a supplier route: it ends the route
// on terminal errors, remembers an outage for the extra Lumia attempt and never
// switches supplier once the customer received data.
func TestHwDecideRetryFollowsTheSupplierRoute(t *testing.T) {
	previousPause := common.HwLumiaRetryPauseMs
	common.HwLumiaRetryPauseMs = 0
	t.Cleanup(func() { common.HwLumiaRetryPauseMs = previousPause })

	lumia := hwPolicyChannel(1, HwProviderLumia, "")
	spare := hwPolicyChannel(2, HwProviderLumia, "")
	openRouter := hwPolicyChannel(3, HwProviderOpenRouter, "")
	serverError := hwPolicyError(http.StatusInternalServerError, types.ErrorCodeBadResponseStatusCode)

	t.Run("no route leaves the generic decision alone", func(t *testing.T) {
		decision, handled := HwDecideRetry(hwClientErrorContext(""), serverError)
		assert.False(t, handled)
		assert.Equal(t, PolicyDecision{}, decision)
	})

	t.Run("no error stops", func(t *testing.T) {
		c := hwClientErrorContext("")
		c.Set(hwRouteContextKey, hwPolicyRoute([]*model.Channel{lumia}, nil))
		decision, handled := HwDecideRetry(c, nil)
		require.True(t, handled)
		assert.Equal(t, "stop", decision.Action)
		assert.Equal(t, "hw_no_error", decision.Reason)
	})

	t.Run("skip-retry and service-busy errors are terminal", func(t *testing.T) {
		for _, apiErr := range []*types.NewAPIError{
			hwPolicyError(http.StatusBadRequest, "invalid_request", types.ErrOptionWithSkipRetry()),
			hwPolicyError(http.StatusServiceUnavailable, HwErrorCodeServiceBusy),
		} {
			c := hwClientErrorContext("")
			state := hwPolicyRoute([]*model.Channel{lumia, spare}, nil)
			c.Set(hwRouteContextKey, state)
			decision, handled := HwDecideRetry(c, apiErr)
			require.True(t, handled)
			assert.Equal(t, "hw_terminal_error", decision.Reason)
			assert.True(t, state.finished)
			assert.Nil(t, state.next(), "a finished route offers nothing more")
		}
	})

	t.Run("an untried alias keeps the route going", func(t *testing.T) {
		c := hwClientErrorContext("")
		state := hwPolicyRoute([]*model.Channel{lumia, spare}, nil)
		state.tried[lumia.Id] = true
		c.Set(hwRouteContextKey, state)
		decision, handled := HwDecideRetry(c, hwPolicyError(http.StatusBadRequest, "invalid_request"))
		require.True(t, handled)
		assert.Equal(t, "retry", decision.Action)
		assert.Equal(t, "hw_supplier_route", decision.Reason)
		assert.Equal(t, "supplier_policy", decision.Source)
		assert.False(t, state.outage, "a 400 is not an outage")
	})

	t.Run("an outage on the only Lumia alias earns one retry, then the route ends", func(t *testing.T) {
		c := hwClientErrorContext("")
		state := hwPolicyRoute([]*model.Channel{lumia}, nil)
		c.Set(hwRouteContextKey, state)
		require.Same(t, lumia, state.next())

		decision, handled := HwDecideRetry(c, serverError)
		require.True(t, handled)
		assert.Equal(t, "hw_supplier_route", decision.Reason)
		assert.True(t, state.outage)
		assert.True(t, HwRetryLoopExtends(c))

		require.Same(t, lumia, state.next(), "the single extra Lumia attempt")
		assert.True(t, state.lumiaRetried)
		decision, _ = HwDecideRetry(c, serverError)
		assert.Equal(t, "hw_route_exhausted", decision.Reason)
		assert.True(t, state.finished)
		assert.False(t, HwRetryLoopExtends(c))
	})

	t.Run("an outage moves a paid request on to the emergency supplier", func(t *testing.T) {
		c := hwClientErrorContext("")
		state := hwPolicyRoute([]*model.Channel{lumia}, []*model.Channel{openRouter})
		state.lumiaRetried = true
		c.Set(hwRouteContextKey, state)
		require.Same(t, lumia, state.next())

		decision, _ := HwDecideRetry(c, serverError)
		assert.Equal(t, "hw_supplier_route", decision.Reason)
		require.Same(t, openRouter, state.next())
		assert.True(t, state.fallback)
		assert.Equal(t, HwProviderOpenRouter, state.provider)
	})

	t.Run("a stream that already wrote data never switches supplier", func(t *testing.T) {
		c := hwClientErrorContext("")
		state := hwPolicyRoute([]*model.Channel{lumia, spare}, []*model.Channel{openRouter})
		state.tried[lumia.Id] = true
		state.streamWrites = 1
		c.Set(hwRouteContextKey, state)
		decision, _ := HwDecideRetry(c, serverError)
		assert.Equal(t, "hw_route_exhausted", decision.Reason)
		assert.True(t, state.streamCut)
		assert.True(t, state.finished)
	})
}

// A route claims only a channel that belongs to it: tagged, allowed for the
// group and serving the requested family through its own model mapping.
func TestHwRouteClaimOnlyAcceptsChannelsOfTheRoute(t *testing.T) {
	var nilState *HwRouteState
	assert.False(t, nilState.claim(hwPolicyChannel(1, HwProviderLumia, "")))
	assert.False(t, hwPolicyRoute(nil, nil).claim(nil))

	state := hwPolicyRoute(nil, nil)
	assert.False(t, state.claim(&model.Channel{Id: 2, Type: constant.ChannelTypeOpenAI}), "untagged channel")
	assert.False(t, state.claim(hwPolicyChannel(3, HwProviderLumia, `{"claude-sonnet-4-5":"gpt-4o"}`)), "mapped to another family")

	free := hwPolicyRoute(nil, nil)
	free.group, free.userGroup = "free", "free"
	assert.False(t, free.claim(hwPolicyChannel(4, HwProviderOpenRouter, "")), "emergency supplier for a free group")

	require.True(t, state.claim(hwPolicyChannel(5, HwProviderLumia, `{"claude-sonnet-4-5":"claude-sonnet-4-5-20250929"}`)))
	require.True(t, state.claim(hwPolicyChannel(6, HwProviderOpenRouter, "")))
	assert.Equal(t, 2, state.attempts)
	assert.True(t, state.fallback, "the second attempt counts as a fallback")
	assert.Equal(t, 6, state.channelID)
	assert.Equal(t, HwProviderOpenRouter, state.provider)
	assert.True(t, state.tried[5] && state.tried[6])
}

// The retry-path selector keeps routing on the prebuilt route and ends an
// exhausted route in the service-busy error instead of leaking to a foreign
// family.
func TestHwSelectChannelFollowsTheRoute(t *testing.T) {
	lumia := hwPolicyChannel(1, HwProviderLumia, "")
	retry := &RetryParam{ModelName: "claude-sonnet-4-5"}

	t.Run("first channel needs a context and a pick", func(t *testing.T) {
		replacement, apiErr := HwFirstChannel(nil, lumia, HwPaidGroup, "claude-sonnet-4-5")
		assert.Nil(t, replacement)
		assert.Nil(t, apiErr)

		replacement, apiErr = HwFirstChannel(hwClientErrorContext(""), nil, HwPaidGroup, "claude-sonnet-4-5")
		assert.Nil(t, replacement)
		assert.Nil(t, apiErr)
	})

	t.Run("prebuilt route", func(t *testing.T) {
		c := hwClientErrorContext("")
		state := hwPolicyRoute([]*model.Channel{lumia}, nil)
		c.Set(hwRouteContextKey, state)

		channel, group, err := HwSelectChannel(c, retry, func() (*model.Channel, string, error) {
			t.Fatal("the stock selection must not run once the route exists")
			return nil, "", nil
		})
		require.NoError(t, err)
		assert.Same(t, lumia, channel)
		assert.Equal(t, HwPaidGroup, group)

		channel, _, err = HwSelectChannel(c, retry, nil)
		assert.Nil(t, channel)
		require.Error(t, err)
		assert.True(t, state.finished)
		var apiErr *types.NewAPIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, HwErrorCodeServiceBusy, apiErr.GetErrorCode())
	})

	t.Run("stock selection errors pass through", func(t *testing.T) {
		boom := errors.New("no channel")
		channel, group, err := HwSelectChannel(hwClientErrorContext(""), retry, func() (*model.Channel, string, error) {
			return nil, "stock-group", boom
		})
		assert.Nil(t, channel)
		assert.Equal(t, "stock-group", group)
		assert.ErrorIs(t, err, boom)
	})
}

func TestHwCustomerModelPrefersTheRouteModel(t *testing.T) {
	assert.Empty(t, HwCustomerModel(nil))

	c := hwClientErrorContext("")
	c.Set("original_model", "from-request")
	assert.Equal(t, "from-request", HwCustomerModel(c))

	c.Set(hwRouteContextKey, hwPolicyRoute(nil, nil))
	assert.Equal(t, "claude-sonnet-4-5", HwCustomerModel(c))
}

func hwStreamCutInfo(endReason relaycommon.StreamEndReason, received int) *relaycommon.RelayInfo {
	status := relaycommon.NewStreamStatus()
	status.RequireTerminal()
	status.SetEndReason(endReason, nil)
	return &relaycommon.RelayInfo{IsStream: true, StreamStatus: status, ReceivedResponseCount: received}
}

// A supplier stream that stops without its terminal event is an interruption.
// Once the customer saw output it is final (skip retry, so two answers are never
// spliced); before any output the route may continue with the next supplier.
func TestHwStreamCutShortSeparatesVisibleFromEarlyCuts(t *testing.T) {
	newContext := func() (*gin.Context, *HwRouteState) {
		c := hwClientErrorContext("")
		state := hwPolicyRoute([]*model.Channel{hwPolicyChannel(1, HwProviderLumia, "")}, nil)
		c.Set(hwRouteContextKey, state)
		return c, state
	}

	t.Run("cut after output is final", func(t *testing.T) {
		c, state := newContext()
		info := hwStreamCutInfo(relaycommon.StreamEndReasonEOF, 3)
		apiErr := HwStreamCutShort(c, info)
		require.NotNil(t, apiErr)
		assert.Equal(t, HwErrorCodeStreamInterrupted, apiErr.GetErrorCode())
		assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
		assert.True(t, types.IsSkipRetryError(apiErr))
		assert.True(t, state.streamCut)
		assert.False(t, HwStreamCutBeforeOutput(c, info))
	})

	t.Run("output the writer saw counts as visible", func(t *testing.T) {
		c, state := newContext()
		HwNoteStreamWrite(c)
		apiErr := HwStreamCutShort(c, hwStreamCutInfo(relaycommon.StreamEndReasonScannerErr, 0))
		require.NotNil(t, apiErr)
		assert.True(t, types.IsSkipRetryError(apiErr))
		assert.Equal(t, 1, state.streamWrites)
	})

	t.Run("cut before the first chunk may retry", func(t *testing.T) {
		c, state := newContext()
		info := hwStreamCutInfo(relaycommon.StreamEndReasonTimeout, 0)
		apiErr := HwStreamCutShort(c, info)
		require.NotNil(t, apiErr)
		assert.Equal(t, HwErrorCodeStreamInterrupted, apiErr.GetErrorCode())
		assert.False(t, types.IsSkipRetryError(apiErr))
		assert.False(t, state.streamCut)
		assert.True(t, HwStreamCutBeforeOutput(c, info))
		assert.True(t, hwIsOutageError(apiErr), "the early cut earns the outage retry")
	})

	t.Run("not a supplier cut", func(t *testing.T) {
		c, _ := newContext()
		done := hwStreamCutInfo(relaycommon.StreamEndReasonDone, 2)
		clientGone := hwStreamCutInfo(relaycommon.StreamEndReasonClientGone, 2)
		completed := hwStreamCutInfo(relaycommon.StreamEndReasonEOF, 2)
		completed.StreamStatus.MarkCompleted()
		noTerminal := &relaycommon.RelayInfo{IsStream: true, StreamStatus: relaycommon.NewStreamStatus()}
		noTerminal.StreamStatus.SetEndReason(relaycommon.StreamEndReasonEOF, nil)
		nonStream := hwStreamCutInfo(relaycommon.StreamEndReasonEOF, 2)
		nonStream.IsStream = false

		for name, info := range map[string]*relaycommon.RelayInfo{
			"done": done, "client gone": clientGone, "completed": completed,
			"terminal not expected": noTerminal, "not a stream": nonStream, "nil info": nil,
		} {
			assert.Nil(t, HwStreamCutShort(c, info), name)
			assert.False(t, HwStreamCutBeforeOutput(c, info), name)
		}
		assert.Nil(t, HwStreamCutShort(hwClientErrorContext(""), hwStreamCutInfo(relaycommon.StreamEndReasonEOF, 2)), "no route")
	})
}

// The route keeps the facts the admin log reports: how often the mask rewrote
// something, which upstream ids were seen (blank and repeats dropped, capped)
// and the prompt tokens of abandoned attempts.
func TestHwRouteNotesFeedTheAdminLog(t *testing.T) {
	c := hwClientErrorContext("")
	state := hwPolicyRoute(nil, nil)
	c.Set(hwRouteContextKey, state)

	HwNoteMaskAlert(c)
	HwNoteMaskAlert(c)
	HwNoteAbandonedAttempt(c, 40)
	HwNoteAbandonedAttempt(c, 0)
	HwNoteAbandonedAttempt(c, 2)
	HwNoteUpstreamIdentity(c, "  ")
	HwNoteUpstreamIdentity(c, " chatcmpl-1 ")
	HwNoteUpstreamIdentity(c, "chatcmpl-1")
	for i := 2; i <= hwMaxUpstreamIDs+3; i++ {
		HwNoteUpstreamIdentity(c, fmt.Sprintf("chatcmpl-%d", i))
	}

	assert.Equal(t, 2, state.maskAlerts)
	assert.Equal(t, 42, state.abandonedPrompt)
	require.Len(t, state.upstreamIDs, hwMaxUpstreamIDs)
	assert.Equal(t, "chatcmpl-1", state.upstreamIDs[0])

	// Without a route nothing panics and nothing is recorded.
	bare := hwClientErrorContext("")
	HwNoteMaskAlert(bare)
	HwNoteStreamWrite(bare)
	HwNoteAbandonedAttempt(bare, 5)
	HwNoteUpstreamIdentity(bare, "x")
	assert.Nil(t, HwRoute(bare))
}

// An exhausted route reports the fork's own service-busy error for supplier
// outages only; client mistakes, own codes and skip-retry errors keep theirs.
func TestHwTerminalErrorHidesSupplierOutagesOnly(t *testing.T) {
	c := hwClientErrorContext("")
	c.Set(hwRouteContextKey, hwPolicyRoute(nil, nil))
	stop := PolicyDecision{Action: "stop"}
	outage := hwPolicyError(http.StatusBadGateway, types.ErrorCodeBadResponseStatusCode)

	replaced := HwTerminalError(c, outage, stop)
	require.NotNil(t, replaced)
	assert.Equal(t, HwErrorCodeServiceBusy, replaced.GetErrorCode())

	assert.Nil(t, HwTerminalError(c, outage, PolicyDecision{Action: "retry"}), "a retrying route keeps the error")
	assert.Nil(t, HwTerminalError(c, nil, stop))
	assert.Nil(t, HwTerminalError(c, hwPolicyError(http.StatusBadRequest, "invalid_request"), stop), "a client error is no outage")
	assert.Nil(t, HwTerminalError(c, hwPolicyError(http.StatusBadGateway, HwErrorCodeStreamInterrupted), stop), "own code survives")
	assert.Nil(t, HwTerminalError(c, hwPolicyError(http.StatusBadGateway, "other", types.ErrOptionWithSkipRetry()), stop))
	assert.Nil(t, HwTerminalError(hwClientErrorContext(""), outage, stop), "no route")
}

// hwRouteDBChannel stores an enabled channel with its ability and supplier tag,
// then rebuilds the channel cache so hwStartRoute sees it like production does.
func hwRouteDBChannel(t *testing.T, id int, group, modelName, tag string, channelType int, mapping string) {
	t.Helper()
	createChannelSelectAutoGroupsChannel(t, model.DB, id, group, modelName)
	updates := map[string]any{"type": channelType}
	if tag != "" {
		updates["tag"] = tag
	}
	if mapping != "" {
		updates["model_mapping"] = mapping
	}
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", id).Updates(updates).Error)
}

func hwRouteIDs(channels []*model.Channel) []int {
	ids := make([]int, 0, len(channels))
	for _, channel := range channels {
		ids = append(ids, channel.Id)
	}
	return ids
}

// hwStartRoute decides whether the supplier policy takes over a request at
// all. A deployment without supplier tags must keep the stock selection, a
// free group must never see the emergency supplier, and a channel that maps the
// model into another family must be counted but not offered.
func TestHwStartRouteBuildsTheRouteFromTaggedChannels(t *testing.T) {
	setupChannelSelectAutoGroupsTest(t)
	const modelName = "claude-sonnet-4-5"
	hwRouteDBChannel(t, 3101, HwPaidGroup, modelName, "", constant.ChannelTypeOpenAI, "")
	hwRouteDBChannel(t, 3102, HwPaidGroup, modelName, HwProviderLumia, constant.ChannelTypeOpenAI, "")
	hwRouteDBChannel(t, 3103, HwPaidGroup, modelName, HwProviderOpenRouter, constant.ChannelTypeOpenAI, "")
	hwRouteDBChannel(t, 3104, HwPaidGroup, modelName, HwProviderLumia, constant.ChannelTypeOpenAI, `{"claude-sonnet-4-5":"gpt-4o"}`)
	hwRouteDBChannel(t, 3105, HwPaidGroup, modelName, HwProviderLumia, constant.ChannelTypeTaskPlugin, "")
	hwRouteDBChannel(t, 3201, "plain", modelName, "", constant.ChannelTypeOpenAI, "")
	hwRouteDBChannel(t, 3301, "free", modelName, HwProviderOpenRouter, constant.ChannelTypeOpenAI, "")
	model.InitChannelCache()

	t.Run("nothing to route", func(t *testing.T) {
		assert.Nil(t, hwStartRoute(nil, HwPaidGroup, modelName, nil))
		c := hwClientErrorContext("")
		assert.Nil(t, hwStartRoute(c, HwPaidGroup, "", nil), "no model")
		assert.Nil(t, hwStartRoute(c, HwPaidGroup, "unknown-model", nil), "no candidates")
		assert.Nil(t, HwRoute(c), "no route stored when the policy stays out")
	})

	t.Run("untagged channels keep the stock selection", func(t *testing.T) {
		c := hwClientErrorContext("")
		assert.Nil(t, hwStartRoute(c, "plain", modelName, nil))
		assert.Nil(t, HwRoute(c))
	})

	t.Run("lumia and openrouter are split, family and filters respected", func(t *testing.T) {
		c := hwClientErrorContext("")
		common.SetContextKey(c, constant.ContextKeyUserGroup, HwPaidGroup)
		filters := []dto.ChannelFilter{{Kind: dto.FilterTaskPluginIdentity}}

		state := hwStartRoute(c, HwPaidGroup, modelName, filters)
		require.NotNil(t, state)
		assert.Same(t, state, HwRoute(c))
		assert.True(t, state.active)
		assert.Equal(t, HwPaidGroup, state.group)
		assert.Equal(t, HwPaidGroup, state.userGroup)
		assert.Equal(t, modelName, state.model)
		assert.Equal(t, HwModelFamily(modelName), state.family)
		assert.Equal(t, []int{3102}, hwRouteIDs(state.lumia), "untagged, foreign-family and plugin-typed channels are left out")
		assert.Equal(t, []int{3103}, hwRouteIDs(state.alternates))
		assert.Equal(t, 1, state.blockedByFamily)
	})

	t.Run("an empty group falls back to the context groups", func(t *testing.T) {
		c := hwClientErrorContext("")
		common.SetContextKey(c, constant.ContextKeyUsingGroup, HwPaidGroup)
		common.SetContextKey(c, constant.ContextKeyUserGroup, HwPaidGroup)
		state := hwStartRoute(c, "", modelName, nil)
		require.NotNil(t, state)
		assert.Equal(t, HwPaidGroup, state.group)

		c = hwClientErrorContext("")
		common.SetContextKey(c, constant.ContextKeyUserGroup, HwPaidGroup)
		state = hwStartRoute(c, "", modelName, nil)
		require.NotNil(t, state, "the user group stands in when no group is in use")
		assert.Equal(t, HwPaidGroup, state.group)
	})

	t.Run("the emergency supplier never serves a free group", func(t *testing.T) {
		c := hwClientErrorContext("")
		common.SetContextKey(c, constant.ContextKeyUserGroup, "free")
		state := hwStartRoute(c, "free", modelName, nil)
		require.NotNil(t, state, "a tagged channel still puts the route in charge")
		assert.Empty(t, state.alternates)
		assert.Empty(t, state.lumia)
		assert.Nil(t, state.next(), "nothing is left to try, so the caller ends in service-busy")
	})
}

// HwFirstChannel runs once for the first attempt: it leaves the stock pick
// alone when the route claims it, swaps it for the route's own first channel
// when the stock pick is foreign, and ends in the service-busy error when the
// route has nothing left to offer.
func TestHwFirstChannelKeepsClaimsAndReplacesForeignPicks(t *testing.T) {
	setupChannelSelectAutoGroupsTest(t)
	const modelName = "claude-sonnet-4-5"
	hwRouteDBChannel(t, 4101, HwPaidGroup, modelName, "", constant.ChannelTypeOpenAI, "")
	hwRouteDBChannel(t, 4102, HwPaidGroup, modelName, HwProviderLumia, constant.ChannelTypeOpenAI, "")
	hwRouteDBChannel(t, 4201, "plain", modelName, "", constant.ChannelTypeOpenAI, "")
	hwRouteDBChannel(t, 4301, "free", modelName, HwProviderOpenRouter, constant.ChannelTypeOpenAI, "")
	model.InitChannelCache()

	channelOf := func(id int) *model.Channel {
		channel, err := model.CacheGetChannel(id)
		require.NoError(t, err)
		return channel
	}

	t.Run("untagged deployment is left alone", func(t *testing.T) {
		replacement, apiErr := HwFirstChannel(hwClientErrorContext(""), channelOf(4201), "plain", modelName)
		assert.Nil(t, replacement)
		assert.Nil(t, apiErr)
	})

	t.Run("a claimed stock pick stays", func(t *testing.T) {
		c := hwClientErrorContext("")
		common.SetContextKey(c, constant.ContextKeyUserGroup, HwPaidGroup)
		replacement, apiErr := HwFirstChannel(c, channelOf(4102), HwPaidGroup, modelName)
		assert.Nil(t, replacement)
		assert.Nil(t, apiErr)
		state := HwRoute(c)
		require.NotNil(t, state)
		assert.Equal(t, 1, state.attempts)
		assert.True(t, state.tried[4102])
	})

	t.Run("an untagged stock pick is replaced by the route", func(t *testing.T) {
		c := hwClientErrorContext("")
		common.SetContextKey(c, constant.ContextKeyUserGroup, HwPaidGroup)
		replacement, apiErr := HwFirstChannel(c, channelOf(4101), HwPaidGroup, modelName)
		assert.Nil(t, apiErr)
		require.NotNil(t, replacement)
		assert.Equal(t, 4102, replacement.Id)
	})

	t.Run("a route without channels ends in service-busy", func(t *testing.T) {
		c := hwClientErrorContext("")
		common.SetContextKey(c, constant.ContextKeyUserGroup, "free")
		replacement, apiErr := HwFirstChannel(c, channelOf(4301), "free", modelName)
		assert.Nil(t, replacement)
		require.NotNil(t, apiErr)
		assert.Equal(t, HwErrorCodeServiceBusy, apiErr.GetErrorCode())
		state := HwRoute(c)
		require.NotNil(t, state)
		assert.True(t, state.finished)
	})
}
