package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func hwCandidateIDs(channels []*Channel) []int {
	ids := make([]int, 0, len(channels))
	for _, channel := range channels {
		ids = append(ids, channel.Id)
	}
	return ids
}

// The supplier routing policy needs the whole candidate set in a stable order:
// highest priority first, ties by id, disabled and foreign-group channels left
// out, and a request name the routing normalizes still finds its channels. Both
// the ability-table path and the memory-cache path have to agree.
func TestHwChannelsForGroupModelOrdersAndFiltersOnBothCachePaths(t *testing.T) {
	truncateTables(t)
	originalMemoryCache := common.MemoryCacheEnabled
	t.Cleanup(func() { common.MemoryCacheEnabled = originalMemoryCache; InitChannelCache() })

	priority := func(value int64) *int64 { return &value }
	weight := uint(1)
	for _, channel := range []Channel{
		{Id: 920003, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "low", Models: "hw-model", Group: "default", Priority: priority(0), Weight: &weight},
		{Id: 920002, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "high-b", Models: "hw-model", Group: "default", Priority: priority(10), Weight: &weight},
		{Id: 920001, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "high-a", Models: "hw-model", Group: "default", Priority: priority(10), Weight: &weight},
		{Id: 920004, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusManuallyDisabled, Name: "disabled", Models: "hw-model", Group: "default", Priority: priority(99), Weight: &weight},
		{Id: 920005, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "other-group", Models: "hw-model", Group: "vip", Priority: priority(99), Weight: &weight},
		{Id: 920006, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "normalized", Models: "gemini-2.5-flash-lite-thinking-*", Group: "default", Priority: priority(0), Weight: &weight},
	} {
		channel := channel
		require.NoError(t, channel.Insert())
	}

	for _, memoryCache := range []bool{false, true} {
		common.MemoryCacheEnabled = memoryCache
		if memoryCache {
			InitChannelCache()
		}
		name := map[bool]string{false: "ability table", true: "memory cache"}[memoryCache]
		assert.Equal(t, []int{920001, 920002, 920003}, hwCandidateIDs(HwChannelsForGroupModel("default", "hw-model")), name)
		assert.Equal(t, []int{920005}, hwCandidateIDs(HwChannelsForGroupModel("vip", "hw-model")), name)
		assert.Empty(t, HwChannelsForGroupModel("default", "unknown-model"), name)
		assert.Empty(t, HwChannelsForGroupModel("nobody", "hw-model"), name)
		assert.Equal(t, []int{920006}, hwCandidateIDs(HwChannelsForGroupModel("default", "gemini-2.5-flash-lite-thinking-512")), name+": normalized model name")
	}
}
