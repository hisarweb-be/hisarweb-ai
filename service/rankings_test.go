package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rankingTestMeta() map[string]rankingModelMeta {
	return map[string]rankingModelMeta{
		"claude-opus-5-5": {vendor: "Anthropic"},
		"gpt-6-sol":       {vendor: "OpenAI"},
	}
}

// quota_data keeps the usage of internal, test and retired models for good. The
// public leaderboard only ranks what the pricing catalog sells, so those rows
// neither show up as "Unknown" nor take a share, and the ranks and shares of
// the models that stay are computed over the sold models only.
func TestAssembleRankingsOnlyRanksCatalogModels(t *testing.T) {
	config := rankingPeriodConfig{id: "week", duration: 7 * 24 * time.Hour, bucketSize: 24 * 3600, labelLayout: "Jan 2", hasPrevious: true}
	currentTotals := []model.RankingQuotaTotal{
		{ModelName: "claude-haiku-4-5", TotalTokens: 500},
		{ModelName: "claude-opus-5-5", TotalTokens: 300},
		{ModelName: "gpt-6-luna", TotalTokens: 100},
		{ModelName: "gpt-6-sol", TotalTokens: 100},
	}
	currentBuckets := []model.RankingQuotaBucket{
		{ModelName: "claude-haiku-4-5", Bucket: 86400, Tokens: 500},
		{ModelName: "claude-opus-5-5", Bucket: 86400, Tokens: 300},
		{ModelName: "gpt-6-luna", Bucket: 172800, Tokens: 100},
		{ModelName: "gpt-6-sol", Bucket: 172800, Tokens: 100},
	}
	previousTotals := []model.RankingQuotaTotal{
		{ModelName: "gpt-6-luna", TotalTokens: 900},
		{ModelName: "gpt-6-sol", TotalTokens: 200},
		{ModelName: "claude-opus-5-5", TotalTokens: 100},
	}

	answer := assembleRankings(config, currentTotals, currentBuckets, previousTotals, rankingTestMeta())

	require.Len(t, answer.Models, 2)
	assert.Equal(t, "claude-opus-5-5", answer.Models[0].ModelName)
	assert.Equal(t, "gpt-6-sol", answer.Models[1].ModelName)
	assert.InDelta(t, 0.75, answer.Models[0].Share, 0.0001, "300 of the 400 sold tokens, not of 1000")
	assert.InDelta(t, 0.25, answer.Models[1].Share, 0.0001)
	require.NotNil(t, answer.Models[0].PreviousRank)
	assert.Equal(t, 2, *answer.Models[0].PreviousRank, "last period's rank is also counted over the sold models")
	require.NotNil(t, answer.Models[1].PreviousRank)
	assert.Equal(t, 1, *answer.Models[1].PreviousRank)

	require.Len(t, answer.Vendors, 2)
	for _, vendor := range answer.Vendors {
		assert.NotEqual(t, rankingUnknownVendor, vendor.Vendor)
		assert.Equal(t, 1, vendor.ModelsCount)
	}

	require.Len(t, answer.ModelsHistory.Models, 2)
	for _, point := range answer.ModelsHistory.Points {
		assert.Contains(t, []string{"claude-opus-5-5", "gpt-6-sol"}, point.Model)
	}
	for _, point := range answer.VendorShareHistory.Points {
		assert.NotEqual(t, rankingUnknownVendor, point.Vendor)
	}
}

func TestAssembleRankingsWithoutSoldUsageIsEmpty(t *testing.T) {
	config := rankingPeriodConfig{id: "all", bucketSize: 7 * 24 * 3600, labelLayout: "Jan 2"}
	answer := assembleRankings(config,
		[]model.RankingQuotaTotal{{ModelName: "claude-fable-5-1", TotalTokens: 10}},
		[]model.RankingQuotaBucket{{ModelName: "claude-fable-5-1", Bucket: 86400, Tokens: 10}},
		nil, rankingTestMeta())

	assert.Empty(t, answer.Models)
	assert.Empty(t, answer.Vendors)
	assert.Empty(t, answer.ModelsHistory.Points)
	assert.Empty(t, answer.VendorShareHistory.Points)
}

// A model is in the public catalog when it is enabled for everyone ("all") or
// for a group an anonymous visitor may use, the rule /api/pricing applies.
func TestRankingPricingIsPublic(t *testing.T) {
	usable := map[string]string{"default": "Default"}

	assert.True(t, rankingPricingIsPublic([]string{"all"}, usable))
	assert.True(t, rankingPricingIsPublic([]string{"vip", "default"}, usable))
	assert.False(t, rankingPricingIsPublic([]string{"vip", "internal"}, usable), "only groups nobody can pick")
	assert.False(t, rankingPricingIsPublic(nil, usable))
	assert.False(t, rankingPricingIsPublic([]string{"all"}, nil), "no usable group: the pricing page lists nothing")
}
