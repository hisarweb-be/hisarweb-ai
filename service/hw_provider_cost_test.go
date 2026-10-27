package service

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hwCostChannel builds a supplier channel whose other_info carries the cost
// metadata for one model: input and output USD per 1M tokens.
func hwCostChannel(id int, provider, modelName string, in, out float64) *model.Channel {
	channel := hwPolicyChannel(id, provider, "")
	channel.SetOtherInfo(map[string]any{
		HwChannelCostKey: map[string]any{modelName: map[string]any{"in": in, "out": out}},
	})
	return channel
}

func TestHwChannelProviderRecognisesPortixAndKie(t *testing.T) {
	testCases := []struct {
		name    string
		tag     string
		baseURL string
		want    string
	}{
		{name: "portix tag", tag: "portix", want: HwProviderPortix},
		{name: "portix tag in capitals", tag: " Portix ", want: HwProviderPortix},
		{name: "prefixed portix tag", tag: "portix-claude", want: HwProviderPortix},
		{name: "portix host", baseURL: "https://www.portixapi.com", want: HwProviderPortix},
		{name: "kie tag", tag: "kie", want: HwProviderKie},
		{name: "prefixed kie tag", tag: "kie-gpt", want: HwProviderKie},
		{name: "kie host", baseURL: "https://api.kie.ai", want: HwProviderKie},
		{name: "kie host with capitals", baseURL: "HTTPS://API.KIE.AI/v1", want: HwProviderKie},
		{name: "lookalike portix host", baseURL: "https://portixapi.com.example.com", want: HwProviderInternal},
		{name: "lookalike kie host", baseURL: "https://notkie.ai", want: HwProviderInternal},
		{name: "tag that only starts with the letters", tag: "kiev-office", want: HwProviderInternal},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			channel := &model.Channel{Type: constant.ChannelTypeOpenAI}
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
}

// Portix and Kie are reachable for the paying group only, whatever made them
// recognisable, exactly like the emergency supplier.
func TestHwPortixAndKieAreForThePayingGroupOnly(t *testing.T) {
	tagged := []*model.Channel{hwPolicyChannel(1, HwProviderPortix, ""), hwPolicyChannel(2, HwProviderKie, "")}
	hosted := &model.Channel{Id: 3, Type: constant.ChannelTypeOpenAI}
	baseURL := "https://api.kie.ai"
	hosted.BaseURL = &baseURL
	tagged = append(tagged, hosted)

	for _, channel := range tagged {
		assert.True(t, HwChannelAllowedForGroup(channel, HwPaidGroup, HwPaidGroup))
		assert.False(t, HwChannelAllowedForGroup(channel, "free", "free"))
		assert.False(t, HwChannelAllowedForGroup(channel, "default", HwPaidGroup))
		assert.False(t, HwChannelAllowedForGroup(channel, HwPaidGroup, "free"), "free account on a paid token")
		assert.False(t, HwChannelAllowedForGroup(channel, "", ""))
	}
	assert.True(t, HwChannelAllowedForGroup(hwPolicyChannel(4, HwProviderLumia, ""), "free", "free"), "Lumia stays open to every group")
}

func TestHwChannelCostReadsMetadataPerModel(t *testing.T) {
	const customer = "claude-sonnet-5-5"
	assert.Equal(t, 12.0, hwChannelCost(hwCostChannel(1, HwProviderPortix, customer, 2, 10), customer))
	assert.True(t, math.IsInf(hwChannelCost(hwCostChannel(2, HwProviderPortix, "claude-opus-5-5", 4, 20), customer), 1), "metadata of another model")
	assert.True(t, math.IsInf(hwChannelCost(hwPolicyChannel(3, HwProviderPortix, ""), customer), 1), "no metadata")
	assert.True(t, math.IsInf(hwChannelCost(nil, customer), 1))

	mapped := hwCostChannel(4, HwProviderKie, "upstream-sonnet", 1, 5)
	mapping := `{"claude-sonnet-5-5":"upstream-sonnet"}`
	mapped.ModelMapping = &mapping
	assert.Equal(t, 6.0, hwChannelCost(mapped, customer), "the upstream name after the model mapping")

	for name, info := range map[string]map[string]any{
		"negative":      {HwChannelCostKey: map[string]any{customer: map[string]any{"in": -1.0, "out": 5.0}}},
		"missing out":   {HwChannelCostKey: map[string]any{customer: map[string]any{"in": 1.0}}},
		"text price":    {HwChannelCostKey: map[string]any{customer: map[string]any{"in": "1", "out": "5"}}},
		"not an object": {HwChannelCostKey: "cheap"},
	} {
		channel := hwPolicyChannel(5, HwProviderKie, "")
		channel.SetOtherInfo(info)
		assert.True(t, math.IsInf(hwChannelCost(channel, customer), 1), name)
	}
}

// The route puts the cheapest alternative first for the requested model, keeps
// Lumia ahead of all of them and ends with the channels of unknown price.
func TestHwStartRouteOrdersAlternatesCheapestFirst(t *testing.T) {
	setupChannelSelectAutoGroupsTest(t)
	const modelName = "claude-sonnet-5-5"
	hwRouteDBChannel(t, 5101, HwPaidGroup, modelName, HwProviderLumia, constant.ChannelTypeOpenAI, "")
	hwRouteDBChannel(t, 5102, HwPaidGroup, modelName, HwProviderOpenRouter, constant.ChannelTypeOpenAI, "")
	hwRouteDBChannel(t, 5103, HwPaidGroup, modelName, HwProviderPortix, constant.ChannelTypeOpenAI, "")
	hwRouteDBChannel(t, 5104, HwPaidGroup, modelName, HwProviderKie, constant.ChannelTypeOpenAI, "")
	costs := func(in, out float64) string {
		info := hwCostChannel(0, HwProviderKie, modelName, in, out).OtherInfo
		return info
	}
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 5103).Update("other_info", costs(2, 10)).Error)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 5104).Update("other_info", costs(1, 5)).Error)
	model.InitChannelCache()

	c := hwClientErrorContext("")
	common.SetContextKey(c, constant.ContextKeyUserGroup, HwPaidGroup)
	state := hwStartRoute(c, HwPaidGroup, modelName, nil)
	require.NotNil(t, state)
	assert.Equal(t, []int{5101}, hwRouteIDs(state.lumia))
	assert.Equal(t, []int{5104, 5103, 5102}, hwRouteIDs(state.alternates), "kie 6, portix 12, openrouter unknown")

	assert.Equal(t, 5101, state.next().Id, "Lumia first")
	assert.Equal(t, 5104, state.next().Id)
	assert.Equal(t, HwProviderKie, state.provider)
	assert.Equal(t, 5103, state.next().Id)
	assert.Equal(t, HwProviderPortix, state.provider)
	assert.Equal(t, 5102, state.next().Id)
	assert.Equal(t, HwProviderOpenRouter, state.provider)
	assert.Nil(t, state.next(), "route exhausted")
	assert.True(t, state.fallback)
}

// A free group never sees Portix or Kie, even when one is wrongly placed in it.
func TestHwStartRouteKeepsPortixAndKieOutOfFreeGroups(t *testing.T) {
	setupChannelSelectAutoGroupsTest(t)
	const modelName = "claude-haiku-5-5"
	hwRouteDBChannel(t, 5201, "free", modelName, HwProviderLumia, constant.ChannelTypeOpenAI, "")
	hwRouteDBChannel(t, 5202, "free", modelName, HwProviderPortix, constant.ChannelTypeOpenAI, "")
	hwRouteDBChannel(t, 5203, "free", modelName, HwProviderKie, constant.ChannelTypeOpenAI, "")
	model.InitChannelCache()

	c := hwClientErrorContext("")
	state := hwStartRoute(c, "free", modelName, nil)
	require.NotNil(t, state)
	assert.Equal(t, []int{5201}, hwRouteIDs(state.lumia))
	assert.Empty(t, state.alternates)
	assert.False(t, state.claim(hwPolicyChannel(5202, HwProviderPortix, "")))
	assert.False(t, state.claim(hwPolicyChannel(5203, HwProviderKie, "")))
}
