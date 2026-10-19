package service

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
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
