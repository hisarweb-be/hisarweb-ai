package model

import (
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// HwChannelsForGroupModel lists every enabled channel that can serve group and
// model, highest priority first and then by channel id so the caller gets a
// deterministic order instead of a weighted random pick.
//
// It mirrors the lookup of GetRandomSatisfiedChannel (exact model name first,
// then the routing-normalized name) but returns the whole candidate set: the
// supplier routing policy has to know which alternative channels exist before
// it decides to retry, since a weighted sample cannot answer that question.
func HwChannelsForGroupModel(group, modelName string) []*Channel {
	channels := hwChannelsForGroupModelExact(group, modelName)
	if len(channels) == 0 {
		if normalized := ratio_setting.RoutingMatchModelName(modelName); normalized != "" && normalized != modelName {
			channels = hwChannelsForGroupModelExact(group, normalized)
		}
	}
	sort.SliceStable(channels, func(i, j int) bool {
		left, right := channels[i], channels[j]
		leftPriority, rightPriority := left.GetPriority(), right.GetPriority()
		if leftPriority != rightPriority {
			return leftPriority > rightPriority
		}
		return left.Id < right.Id
	})
	return channels
}

// hwChannelsForGroupModelExact reads the candidate ids for one exact model name
// and turns them into enabled channels. The cached id list is authoritative
// when the memory cache is on (same source as GetRandomSatisfiedChannel); the
// fallback reads the ability table so the policy also works with the cache off.
func hwChannelsForGroupModelExact(group, modelName string) []*Channel {
	ids := make([]int, 0, 4)
	if common.MemoryCacheEnabled {
		channelSyncLock.RLock()
		ids = append(ids, group2model2channels[group][modelName]...)
		channelSyncLock.RUnlock()
	} else {
		var abilities []Ability
		query := DB.Where(commonGroupCol+" = ? and model = ? and enabled = ?", group, modelName, true)
		if err := query.Find(&abilities).Error; err != nil {
			return nil
		}
		for _, ability := range abilities {
			ids = append(ids, ability.ChannelId)
		}
	}

	channels := make([]*Channel, 0, len(ids))
	seen := make(map[int]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		channel, err := CacheGetChannel(id)
		if err != nil || channel == nil || channel.Status != common.ChannelStatusEnabled {
			continue
		}
		channels = append(channels, channel)
	}
	return channels
}
