package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"

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
				openRouter:  testCase.openRouter,
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
