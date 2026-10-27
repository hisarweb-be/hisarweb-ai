package controller

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hwSetChannelCost stores the supplier cost metadata of one model on a channel,
// the way the admin API does: other_info.hw_cost_per_m.
func (e *hwEnv) hwSetChannelCost(channel *model.Channel, modelName string, in, out float64) {
	e.t.Helper()
	channel.SetOtherInfo(map[string]any{
		service.HwChannelCostKey: map[string]any{modelName: map[string]any{"in": in, "out": out}},
	})
	require.NoError(e.t, e.db.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("other_info", channel.OtherInfo).Error)
}

// Lumia is down: the route answers from the cheapest alternative for this model
// (Kie, 6 USD per 1M) before the dearer one (Portix, 12 USD) is touched.
func TestHwOutageGoesToTheCheapestAlternativeFirst(t *testing.T) {
	env := hwNewEnv(t, "paid", 1_000_000)
	lumia := env.fake("lumia")
	portix := env.fake("portix")
	kie := env.fake("kie")
	lumia.setMode(hwFakeTimeout)
	env.addChannel("lumia-primary", "paid", "lumia", hwTestModel, 0, lumia)
	// The dearer channel has the better channel priority: cost, not priority,
	// decides among the alternatives.
	env.hwSetChannelCost(env.addChannel("portix", "paid", "portix", hwTestModel, 5, portix), hwTestModel, 2, 10)
	env.hwSetChannelCost(env.addChannel("kie", "paid", "kie", hwTestModel, 1, kie), hwTestModel, 1, 5)

	recorder := env.post("/v1/chat/completions", hwChatRequest(hwTestModel, false))

	require.Equal(t, 200, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), "hello from the supplier")
	assert.Equal(t, []string{"lumia", "lumia", "kie"}, env.calls.snapshot())
	assert.Equal(t, 0, portix.callCount(), "the dearer alternative is not needed")
	hwAssertNoSupplierWords(t, "response", strings.ToLower(recorder.Body.String()))

	other := env.lastConsumeLogOther(t)
	assert.Equal(t, "kie", other["provider"])
	assert.Equal(t, true, other["provider_fallback"])
}

// The cheapest alternative fails too: the next one answers, and the customer
// never sees a supplier name.
func TestHwFailedCheapestAlternativeFallsBackToTheNext(t *testing.T) {
	env := hwNewEnv(t, "paid", 1_000_000)
	lumia := env.fake("lumia")
	portix := env.fake("portix")
	kie := env.fake("kie")
	lumia.setMode(hwFakeTimeout)
	kie.setMode(hwFakeStatus429)
	env.addChannel("lumia-primary", "paid", "lumia", hwTestModel, 0, lumia)
	env.hwSetChannelCost(env.addChannel("portix", "paid", "portix", hwTestModel, 1, portix), hwTestModel, 2, 10)
	env.hwSetChannelCost(env.addChannel("kie", "paid", "kie", hwTestModel, 1, kie), hwTestModel, 1, 5)

	recorder := env.post("/v1/chat/completions", hwChatRequest(hwTestModel, false))

	require.Equal(t, 200, recorder.Code, recorder.Body.String())
	assert.Equal(t, []string{"lumia", "lumia", "kie", "portix"}, env.calls.snapshot())
	assert.Equal(t, "portix", env.lastConsumeLogOther(t)["provider"])
	hwAssertNoSupplierWords(t, "response", strings.ToLower(recorder.Body.String()))
}

// A free account never reaches Portix or Kie, not even through a wrongly
// listed channel, and the customer-visible error stays free of supplier names.
func TestHwFreeGroupNeverReachesPortixOrKie(t *testing.T) {
	env := hwNewEnv(t, "free", 1_000_000)
	lumia := env.fake("lumia")
	portix := env.fake("portix")
	kie := env.fake("kie")
	lumia.setMode(hwFakeTimeout)
	env.addChannel("lumia-primary", "free", "lumia", "claude-haiku-5-5", 0, lumia)
	env.addChannel("wrongly-listed-portix", "free", "portix", "claude-haiku-5-5", 1, portix)
	env.addChannel("wrongly-listed-kie", "free", "kie", "claude-haiku-5-5", 1, kie)

	recorder := env.post("/v1/chat/completions", hwChatRequest("claude-haiku-5-5", false))

	assert.NotEqual(t, 200, recorder.Code)
	assert.Equal(t, 0, portix.callCount())
	assert.Equal(t, 0, kie.callCount())
	body := strings.ToLower(recorder.Body.String())
	assert.NotContains(t, body, "portix")
	assert.NotContains(t, body, "kie.ai")
}
