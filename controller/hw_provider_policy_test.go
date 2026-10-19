package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const hwTestModel = "claude-sonnet-5-5"

// T1: every Lumia alias times out, so the route makes exactly one extra Lumia
// attempt and then reaches the emergency supplier for the paying group.
func TestHwSupplierOutageRetriesLumiaOnceThenFallsBack(t *testing.T) {
	env := hwNewEnv(t, "paid", 1_000_000)
	lumia := env.fake("lumia")
	openrouter := env.fake("openrouter")
	lumia.setMode(hwFakeTimeout)
	env.addChannel("lumia-primary", "paid", "lumia", 0, lumia)
	env.addChannel("emergency", "paid", "openrouter", 1, openrouter)

	recorder := env.post("/v1/chat/completions", hwChatRequest(hwTestModel, false))

	require.Equal(t, 200, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), "hello from the supplier")
	assert.Equal(t, 2, lumia.callCount(), "one Lumia attempt plus exactly one Lumia retry")
	assert.Equal(t, 1, openrouter.callCount(), "exactly one emergency attempt")
	assert.Equal(t, []string{"lumia", "lumia", "openrouter"}, env.calls.snapshot(),
		"the primary supplier must be tried first, then exactly one retry, then the emergency supplier")

	other := env.lastConsumeLogOther(t)
	assert.Equal(t, "openrouter", other["provider"])
	assert.EqualValues(t, 3, other["provider_attempts"])
	assert.Equal(t, true, other["provider_fallback"])
	assert.NotEmpty(t, other["provider_channel_id"])
}

// T3: a free account never reaches the emergency supplier, not even when the
// emergency channel is wrongly listed in the free group, and unknown groups
// behave the same as free.
func TestHwFreeAndUnknownGroupsNeverReachEmergencySupplier(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		group string
	}{
		{name: "free", group: "free"},
		{name: "unknown", group: "internal-ops"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			env := hwNewEnv(t, testCase.group, 1_000_000)
			lumia := env.fake("lumia")
			openrouter := env.fake("openrouter")
			lumia.setMode(hwFakeTimeout)
			env.addChannel("lumia-primary", testCase.group, "lumia", 0, lumia)
			env.addChannel("wrongly-tagged-emergency", testCase.group, "openrouter", 1, openrouter)

			recorder := env.post("/v1/chat/completions", hwChatRequest(hwTestModel, false))

			require.Equal(t, 503, recorder.Code, recorder.Body.String())
			assert.Contains(t, recorder.Body.String(), "service_busy")
			assert.Equal(t, 0, openrouter.callCount(), "a non-paying group must never call the emergency supplier")
			assert.Equal(t, 2, lumia.callCount())
			assert.NotContains(t, strings.ToLower(recorder.Body.String()), "openrouter")
		})
	}
}

// T3b: the paid rule looks at the account group as well. A free account whose
// token stands on the paid group through a usable-group grant must not reach
// the emergency supplier either.
func TestHwFreeAccountWithPaidTokenNeverReachesEmergencySupplier(t *testing.T) {
	env := hwNewEnv(t, "free", 1_000_000)
	hwGrantTokenGroup(t, "free", "paid")
	env.token.Group = "paid"
	require.NoError(t, env.db.Save(env.token).Error)

	lumia := env.fake("lumia")
	openrouter := env.fake("openrouter")
	lumia.setMode(hwFakeTimeout)
	env.addChannel("lumia-primary", "paid", "lumia", 0, lumia)
	env.addChannel("emergency", "paid", "openrouter", 1, openrouter)

	recorder := env.post("/v1/chat/completions", hwChatRequest(hwTestModel, false))

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), "service_busy")
	assert.Equal(t, 0, openrouter.callCount(), "a free account must never reach the emergency supplier through a paid token")
	assert.Equal(t, 2, lumia.callCount(), "the primary supplier keeps its own outage retry")
}

// T3c: a channel that is the emergency supplier without carrying the exact tag
// (here: the OpenRouter channel type, no tag) must not serve a free group. The
// group has no tagged supplier for the model, so before the fix the request was
// relayed to the emergency channel untouched.
func TestHwUntaggedEmergencyChannelNeverServesFreeGroup(t *testing.T) {
	env := hwNewEnv(t, "free", 1_000_000)
	openrouter := env.fake("openrouter")
	env.addUntaggedChannel("tagless-emergency", "free", constant.ChannelTypeOpenRouter, 0, openrouter)

	recorder := env.post("/v1/chat/completions", hwChatRequest(hwTestModel, false))

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), "service_busy")
	assert.Equal(t, 0, openrouter.callCount(), "an untagged emergency channel must never answer a free request")
	assert.NotContains(t, strings.ToLower(recorder.Body.String()), "openrouter")
}

// T9: the mask never depends on request headers. A customer that sends an
// Upgrade header, or a Connection header that says upgrade, on a plain
// /v1/chat/completions request still gets the filtered answer instead of the
// raw supplier response.
func TestHwUpgradeRequestHeadersNeverBypassTheMask(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		headers map[string]string
	}{
		{name: "upgrade header", headers: map[string]string{"Upgrade": "x"}},
		{name: "connection upgrade", headers: map[string]string{"Connection": "upgrade"}},
		{name: "both", headers: map[string]string{"Upgrade": "websocket", "Connection": "Upgrade"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			env := hwNewEnv(t, "paid", 1_000_000)
			lumia := env.fake("lumia")
			env.addChannel("lumia-primary", "paid", "lumia", 0, lumia)

			recorder := env.postWithHeaders("/v1/chat/completions", hwChatRequest(hwTestModel, false), testCase.headers)

			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			body := recorder.Body.String()
			assert.Contains(t, body, "hello from the supplier", "the answer itself must still reach the customer")
			assert.Empty(t, recorder.Header().Get("X-OpenRouter-Id"), "no supplier response header may reach the customer")
			assert.Empty(t, recorder.Header().Get("X-Provider-Slug"))
			assert.NotContains(t, body, "gen-", "the supplier response id must be rewritten")
			assert.NotContains(t, strings.ToLower(body), "lumia")
			hwAssertCleanResponse(t, recorder, false)
		})
	}
}

func (e *hwEnv) lastConsumeLogOther(t *testing.T) map[string]any {
	t.Helper()
	var stored model.Log
	require.NoError(t, e.db.Where("type = ?", model.LogTypeConsume).Order("id desc").First(&stored).Error)
	outer, err := common.StrToMap(stored.Other)
	require.NoError(t, err)
	adminInfo, ok := outer["admin_info"].(map[string]any)
	require.True(t, ok, "consume log without admin_info: %v", outer)
	return adminInfo
}

// T4: a failing alias of the same family falls through to the next alias, and a
// channel of another model family is never used, not even when it is the only
// candidate.
func TestHwAliasFallbackStaysInsideTheModelFamily(t *testing.T) {
	env := hwNewEnv(t, "paid", 1_000_000)
	aliasOne := env.fake("lumia-one")
	aliasTwo := env.fake("lumia-two")
	openrouter := env.fake("openrouter")
	aliasOne.setMode(hwFakeStatus500)
	env.addChannel("lumia-alias-1", "paid", "lumia", 0, aliasOne)
	env.addChannel("lumia-alias-2", "paid", "lumia", 0, aliasTwo)
	env.addChannel("emergency", "paid", "openrouter", 1, openrouter)

	recorder := env.post("/v1/chat/completions", hwChatRequest(hwTestModel, false))

	require.Equal(t, 200, recorder.Code, recorder.Body.String())
	assert.Equal(t, []string{"lumia-one", "lumia-two"}, env.calls.snapshot())
	assert.Equal(t, 0, openrouter.callCount(), "a same-family alias answered, so the emergency supplier is not needed")
	other := env.lastConsumeLogOther(t)
	assert.Equal(t, "lumia", other["provider"])
	assert.EqualValues(t, 2, other["provider_attempts"])
	assert.Equal(t, true, other["provider_fallback"])
}

func TestHwNeverSelectsAnotherModelFamily(t *testing.T) {
	env := hwNewEnv(t, "paid", 1_000_000)
	foreign := env.fake("lumia-foreign")
	channel := env.addChannel("lumia-gpt-only", "paid", "lumia", 0, foreign)
	mapping, err := common.Marshal(map[string]string{hwTestModel: "gpt-4o-mini"})
	require.NoError(t, err)
	mappingText := string(mapping)
	channel.ModelMapping = &mappingText
	require.NoError(t, env.db.Save(channel).Error)

	recorder := env.post("/v1/chat/completions", hwChatRequest(hwTestModel, false))

	require.Equal(t, 503, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), "service_busy")
	assert.Equal(t, 0, foreign.callCount(), "a GPT channel must never answer a Claude request")
}

func TestHwHealthyLumiaChannelWins(t *testing.T) {
	env := hwNewEnv(t, "paid", 1_000_000)
	lumia := env.fake("lumia")
	openrouter := env.fake("openrouter")
	env.addChannel("lumia-primary", "paid", "lumia", 0, lumia)
	env.addChannel("emergency", "paid", "openrouter", 1, openrouter)

	recorder := env.post("/v1/chat/completions", hwChatRequest(hwTestModel, false))

	require.Equal(t, 200, recorder.Code, recorder.Body.String())
	assert.Equal(t, []string{"lumia"}, env.calls.snapshot())
	assert.Equal(t, 0, openrouter.callCount())
}

// T8: function calling survives the supplier path unchanged, in both protocols
// and in both streaming and non-streaming mode.
func TestHwFunctionCallingPassesThrough(t *testing.T) {
	env := hwNewEnv(t, "paid", 1_000_000)
	lumia := env.fake("lumia")
	lumia.enableToolCall(true)
	env.addChannel("lumia-primary", "paid", "lumia", 0, lumia)

	t.Run("openai non-stream", func(t *testing.T) {
		recorder := env.post("/v1/chat/completions", hwChatRequest(hwTestModel, false))
		require.Equal(t, 200, recorder.Code, recorder.Body.String())
		var body struct {
			Choices []struct {
				Message struct {
					ToolCalls []struct {
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
		require.Len(t, body.Choices, 1)
		require.Len(t, body.Choices[0].Message.ToolCalls, 1)
		assert.Equal(t, "get_weather", body.Choices[0].Message.ToolCalls[0].Function.Name)
		assert.JSONEq(t, `{"city":"Ankara"}`, body.Choices[0].Message.ToolCalls[0].Function.Arguments)
		assert.Equal(t, "tool_calls", body.Choices[0].FinishReason)
	})

	t.Run("openai stream", func(t *testing.T) {
		recorder := env.post("/v1/chat/completions", hwChatRequest(hwTestModel, true))
		require.Equal(t, 200, recorder.Code, recorder.Body.String())
		assert.Contains(t, recorder.Body.String(), `"tool_calls"`)
		assert.Contains(t, recorder.Body.String(), "get_weather")
		assert.Contains(t, recorder.Body.String(), `{\"city\":\"Ankara\"}`)
		assert.Contains(t, recorder.Body.String(), `"finish_reason":"tool_calls"`)
		assert.Equal(t, 1, strings.Count(recorder.Body.String(), "data: [DONE]"),
			"a stream that completes normally ends with exactly one done marker")
	})

	t.Run("anthropic non-stream", func(t *testing.T) {
		recorder := env.post("/v1/messages", hwClaudeRequest(hwTestModel, false))
		require.Equal(t, 200, recorder.Code, recorder.Body.String())
		var body struct {
			Type    string `json:"type"`
			Content []struct {
				Type  string         `json:"type"`
				ID    string         `json:"id"`
				Name  string         `json:"name"`
				Input map[string]any `json:"input"`
			} `json:"content"`
			StopReason string `json:"stop_reason"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
		require.Len(t, body.Content, 1)
		assert.Equal(t, "tool_use", body.Content[0].Type)
		assert.Equal(t, "get_weather", body.Content[0].Name)
		assert.Equal(t, "Ankara", body.Content[0].Input["city"])
	})

	t.Run("anthropic stream", func(t *testing.T) {
		recorder := env.post("/v1/messages", hwClaudeRequest(hwTestModel, true))
		require.Equal(t, 200, recorder.Code, recorder.Body.String())
		assert.Contains(t, recorder.Body.String(), "tool_use")
		assert.Contains(t, recorder.Body.String(), "get_weather")
		assert.Contains(t, recorder.Body.String(), "Ankara")
	})
}

// T2: the supplier dies mid-stream. The customer already received chunks, so it
// gets a clean error event in its own protocol, the route does not splice in
// another supplier, and the balance only carries the tokens that were consumed.
func TestHwStreamCutMidStreamYieldsOwnErrorEvent(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		path      string
		payload   map[string]any
		wantEvent string
		wantDone  bool
	}{
		{name: "openai", path: "/v1/chat/completions", payload: hwChatRequest(hwTestModel, true), wantEvent: "data: {\"error\":", wantDone: true},
		{name: "anthropic", path: "/v1/messages", payload: hwClaudeRequest(hwTestModel, true), wantEvent: "event: error", wantDone: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			env := hwNewEnv(t, "paid", 1_000_000)
			lumia := env.fake("lumia")
			openrouter := env.fake("openrouter")
			lumia.setMode(hwFakeCutStream)
			env.addChannel("lumia-primary", "paid", "lumia", 0, lumia)
			env.addChannel("emergency", "paid", "openrouter", 1, openrouter)

			before := env.userQuota(t)
			recorder := env.post(testCase.path, testCase.payload)
			after := env.userQuota(t)

			body := recorder.Body.String()
			assert.Equal(t, 200, recorder.Code, body)
			assert.Contains(t, body, testCase.wantEvent, "the customer must get our own error event")
			errorIndex := strings.Index(body, testCase.wantEvent)
			require.GreaterOrEqual(t, errorIndex, 0)
			if testCase.wantDone {
				assert.NotContains(t, body[:errorIndex], "data: [DONE]",
					"an SDK stops at the first done marker, so none may precede the error")
				assert.Equal(t, 1, strings.Count(body, "data: [DONE]"),
					"exactly one done marker closes the stream, after the error")
			} else {
				assert.NotContains(t, body[errorIndex:], "event: message_stop",
					"the error event must close an Anthropic stream")
				assert.Equal(t, errorIndex, strings.LastIndex(body, "event: "),
					"the error event is the last event of the stream")
			}
			assert.Equal(t, 0, openrouter.callCount(), "a stream that already started must not be spliced onto another supplier")

			charged := before - after
			assert.GreaterOrEqual(t, charged, 0)
			assert.LessOrEqual(t, charged, env.estimatedStreamQuota(t), "no more than the request's own estimate may be charged")
			assert.Equal(t, charged, env.consumeQuotaSum(t), "the balance may be charged exactly once, and every charge must be accounted for")
		})
	}
}

// T2b: the supplier dies before the first chunk, so the ordinary supplier order
// still applies and the paying customer gets a complete answer. The failed
// attempt is never settled: the customer is billed once, for the tokens of the
// answer that actually arrived, and used_quota carries the same amount.
func TestHwStreamCutBeforeFirstChunkFallsBack(t *testing.T) {
	t.Run("openai connection cut before the answer", func(t *testing.T) {
		env := hwNewEnv(t, "paid", 1_000_000)
		lumia := env.fake("lumia")
		openrouter := env.fake("openrouter")
		lumia.setMode(hwFakeCutEarly)
		env.addChannel("lumia-primary", "paid", "lumia", 0, lumia)
		env.addChannel("emergency", "paid", "openrouter", 1, openrouter)

		recorder := env.post("/v1/chat/completions", hwChatRequest(hwTestModel, true))

		require.Equal(t, 200, recorder.Code, recorder.Body.String())
		assert.Equal(t, 1, openrouter.callCount(), "without customer-visible output the emergency supplier may answer")
		assert.Contains(t, recorder.Body.String(), `"content":"Hel"`, "the emergency supplier's stream must reach the customer")
		assert.Contains(t, recorder.Body.String(), "data: [DONE]")
	})

	// A native Claude answer that carries status 200 and closes the event stream
	// without a single event is the case that used to be settled before the
	// cut-short check: the prompt estimate was charged, the fallback's settlement
	// became a no-op, and the completion tokens were free.
	t.Run("anthropic native stream without events", func(t *testing.T) {
		env := hwNewEnv(t, "paid", 1_000_000)
		lumia := env.fake("lumia")
		openrouter := env.fake("openrouter")
		lumia.setMode(hwFakeEmptyStream)
		env.addChannel("lumia-primary", "paid", "lumia", 0, lumia)
		env.addChannel("emergency", "paid", "openrouter", 1, openrouter)

		before := env.userQuota(t)
		recorder := env.post("/v1/messages", hwClaudeRequest(hwTestModel, true))
		after := env.userQuota(t)

		require.Equal(t, 200, recorder.Code, recorder.Body.String())
		assert.Equal(t, 1, openrouter.callCount(), "without customer-visible output the emergency supplier may answer")
		assert.Contains(t, recorder.Body.String(), "Hel", "the emergency supplier's stream must reach the customer")
		assert.Equal(t, 1, env.consumeLogCount(t), "one request leaves exactly one consume log")
		charged := before - after
		assert.Positive(t, charged, "the answer that arrived must be billed")
		assert.Equal(t, charged, env.consumeQuotaSum(t), "the balance moves exactly as much as the consume log records")
		assert.Equal(t, charged, env.userUsedQuota(t), "used_quota counts the same charge once")

		// The same request against a healthy supplier must cost the same: the
		// failed attempt may neither add nor remove a charge.
		control := hwNewEnv(t, "paid", 1_000_000)
		control.addChannel("lumia-primary", "paid", "lumia", 0, control.fake("lumia"))
		controlBefore := control.userQuota(t)
		controlRecorder := control.post("/v1/messages", hwClaudeRequest(hwTestModel, true))
		require.Equal(t, 200, controlRecorder.Code, controlRecorder.Body.String())
		assert.Equal(t, charged, controlBefore-control.userQuota(t),
			"a fallback request costs the same as a healthy one")
	})
}

// consumeLogCount is the number of consume logs this user has.
func (e *hwEnv) consumeLogCount(t *testing.T) int {
	t.Helper()
	var count int64
	require.NoError(t, e.db.Model(&model.Log{}).Where("type = ? and user_id = ?", model.LogTypeConsume, e.user.Id).Count(&count).Error)
	return int(count)
}

func (e *hwEnv) userUsedQuota(t *testing.T) int {
	t.Helper()
	var stored model.User
	require.NoError(t, e.db.Select("used_quota").First(&stored, e.user.Id).Error)
	return stored.UsedQuota
}

func (e *hwEnv) userQuota(t *testing.T) int {
	t.Helper()
	var stored model.User
	require.NoError(t, e.db.Select("quota").First(&stored, e.user.Id).Error)
	return stored.Quota
}

func (e *hwEnv) estimatedStreamQuota(t *testing.T) int {
	t.Helper()
	// The pre-consume estimate for the fixture model: prompt tokens of the tiny
	// request plus max_tokens, at model ratio 3 with group ratio 1. It is a
	// generous upper bound for the tokens two chunks can represent.
	return 10_000
}

// consumeQuotaSum is the total the consume log records for this user, which must
// equal the balance movement: a second, unlogged deduction would show up here.
func (e *hwEnv) consumeQuotaSum(t *testing.T) int {
	t.Helper()
	var logs []model.Log
	require.NoError(t, e.db.Where("type = ? and user_id = ?", model.LogTypeConsume, e.user.Id).Find(&logs).Error)
	total := 0
	for _, entry := range logs {
		total += int(entry.Quota)
	}
	return total
}

// hwCallHandler runs one API handler with the identity the authentication
// middleware would have set, so the test sees the route's own projection of the
// stored data instead of a hand-built one.
func hwCallHandler(t *testing.T, env *hwEnv, handler gin.HandlerFunc, method, path string, role int) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(method, path, nil)
	context.Set("id", env.user.Id)
	context.Set("role", role)
	context.Set("username", env.user.Username)
	handler(context)
	return recorder
}

// hwAssertNoSupplierWords fails when customer-visible text names a supplier.
func hwAssertNoSupplierWords(t *testing.T, what, text string) {
	t.Helper()
	lower := strings.ToLower(text)
	for _, word := range []string{"lumia", "openrouter"} {
		assert.NotContainsf(t, lower, word, "%s exposes the supplier word %q:\n%s", what, word, text)
	}
}

// hwAssertCleanResponse checks the customer-visible part of a relayed response:
// body, every header name and value, and every stream chunk on its own.
func hwAssertCleanResponse(t *testing.T, recorder *httptest.ResponseRecorder, stream bool) {
	t.Helper()
	hwAssertNoSupplierWords(t, "response body", recorder.Body.String())
	for name, values := range recorder.Result().Header {
		hwAssertNoSupplierWords(t, "response header "+name, name)
		for _, value := range values {
			hwAssertNoSupplierWords(t, "response header "+name, value)
		}
	}
	if !stream {
		return
	}
	for _, chunk := range strings.Split(recorder.Body.String(), "\n\n") {
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		hwAssertNoSupplierWords(t, "stream chunk "+strconv.Quote(chunk), chunk)
	}
}

// hwDecodeLogItems decodes the items of a log page response.
func hwDecodeLogItems(t *testing.T, body string) []map[string]any {
	t.Helper()
	var page struct {
		Success bool `json:"success"`
		Data    struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal([]byte(body), &page), body)
	require.True(t, page.Success, body)
	return page.Data.Items
}

// hwDecodeLogOther decodes the role-projected metadata of one log item.
func hwDecodeLogOther(t *testing.T, item map[string]any) map[string]any {
	t.Helper()
	raw, ok := item["other"].(string)
	require.True(t, ok, "log item without a metadata string: %v", item)
	if raw == "" {
		return map[string]any{}
	}
	decoded, err := common.StrToMap(raw)
	require.NoError(t, err, raw)
	return decoded
}

// hwProviderAudit returns the newest relay.provider audit row as an
// administrator reads it and as the customer reads it.
func hwProviderAudit(t *testing.T, env *hwEnv) (*model.AuditLog, *model.AuditLog) {
	t.Helper()
	adminRows, _, err := model.GetAuditLogs(model.AuditLogFilter{UserId: env.user.Id, Category: model.AuditCategoryOperation}, 0, 10, common.RoleAdminUser)
	require.NoError(t, err)
	var admin *model.AuditLog
	for _, row := range adminRows {
		if row.Action == "relay.provider" {
			admin = row
			break
		}
	}
	require.NotNil(t, admin, "the admin audit view must contain the relay.provider event")

	selfRows, _, err := model.GetAuditLogs(model.AuditLogFilter{SelfView: true, UserId: env.user.Id}, 0, 10, common.RoleCommonUser)
	require.NoError(t, err)
	var self *model.AuditLog
	for _, row := range selfRows {
		if row.Action == "relay.provider" {
			self = row
			break
		}
	}
	require.NotNil(t, self, "the customer audit view must still contain the event")
	return admin, self
}

// T5: the supplier facts land in the admin log and the audit log, and the
// customer routes never show them.
func TestHwProviderFactsAreAdminOnly(t *testing.T) {
	check := func(t *testing.T, env *hwEnv, provider string, attempts int) {
		t.Helper()

		customerLogs := hwCallHandler(t, env, GetUserLogs, http.MethodGet, "/api/log/self", common.RoleCommonUser)
		require.Equal(t, http.StatusOK, customerLogs.Code, customerLogs.Body.String())
		customerItems := hwDecodeLogItems(t, customerLogs.Body.String())
		require.Len(t, customerItems, 1, "the customer must still see the usage log itself")
		assert.Equal(t, hwTestModel, customerItems[0]["model_name"])
		assert.Empty(t, customerItems[0]["channel_name"], "the channel name is not customer information")
		customerOther := hwDecodeLogOther(t, customerItems[0])
		assert.NotContains(t, customerOther, "admin_info", "/api/log/self must not carry the admin block")
		assert.NotContains(t, customerOther, "provider")
		hwAssertNoSupplierWords(t, "/api/log/self", customerLogs.Body.String())

		adminLogs := hwCallHandler(t, env, GetAllLogs, http.MethodGet, "/api/log", common.RoleAdminUser)
		require.Equal(t, http.StatusOK, adminLogs.Code, adminLogs.Body.String())
		adminItems := hwDecodeLogItems(t, adminLogs.Body.String())
		require.Len(t, adminItems, 1)
		adminOther := hwDecodeLogOther(t, adminItems[0])
		adminInfo, ok := adminOther["admin_info"].(map[string]any)
		require.True(t, ok, "the admin log must carry the admin block: %v", adminOther)
		assert.Equal(t, provider, adminInfo["provider"], "the admin log must name the supplier that actually answered")
		assert.EqualValues(t, attempts, adminInfo["provider_attempts"])
		assert.Equal(t, true, adminInfo["provider_fallback"])

		adminAudit, selfAudit := hwProviderAudit(t, env)
		require.NotNil(t, adminAudit.Other.Provider, "the admin audit view must carry the supplier block")
		assert.Equal(t, provider, adminAudit.Other.Provider.Provider)
		assert.Equal(t, attempts, adminAudit.Other.Provider.Attempts)
		assert.True(t, adminAudit.Other.Provider.Fallback)
		assert.NotZero(t, adminAudit.Other.Provider.ChannelID)

		assert.Nil(t, selfAudit.Other.Provider, "the customer audit view must not carry the supplier block")
		encoded, err := common.Marshal(selfAudit)
		require.NoError(t, err)
		hwAssertNoSupplierWords(t, "/api/audit/self", string(encoded))

		self := hwCallHandler(t, env, GetSelf, http.MethodGet, "/api/user/self", common.RoleCommonUser)
		require.Equal(t, http.StatusOK, self.Code, self.Body.String())
		hwAssertNoSupplierWords(t, "/api/user/self", self.Body.String())
		assert.NotContains(t, self.Body.String(), "admin_info")
	}

	t.Run("emergency fallback", func(t *testing.T) {
		env := hwNewEnv(t, "paid", 1_000_000)
		lumia := env.fake("lumia")
		openrouter := env.fake("openrouter")
		lumia.setMode(hwFakeTimeout)
		env.addChannel("lumia-primary", "paid", "lumia", 0, lumia)
		env.addChannel("emergency", "paid", "openrouter", 1, openrouter)

		recorder := env.post("/v1/chat/completions", hwChatRequest(hwTestModel, false))
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

		check(t, env, "openrouter", 3)
	})

	t.Run("same family alias fallback", func(t *testing.T) {
		env := hwNewEnv(t, "paid", 1_000_000)
		aliasOne := env.fake("lumia-one")
		aliasTwo := env.fake("lumia-two")
		openrouter := env.fake("openrouter")
		aliasOne.setMode(hwFakeStatus500)
		env.addChannel("lumia-alias-1", "paid", "lumia", 0, aliasOne)
		env.addChannel("lumia-alias-2", "paid", "lumia", 0, aliasTwo)
		env.addChannel("emergency", "paid", "openrouter", 1, openrouter)

		recorder := env.post("/v1/chat/completions", hwChatRequest(hwTestModel, false))
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

		check(t, env, "lumia", 2)
	})
}

// T6: nothing a customer sees names a supplier. The structural masking has to
// be enough on its own, and the last-resort word scan has to catch a leak the
// structure does not know about.
func TestHwCustomerNeverSeesTheSupplierName(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		payload map[string]any
		stream  bool
		want    string
	}{
		{name: "chat completions", path: "/v1/chat/completions", payload: hwChatRequest(hwTestModel, false), want: "hello from the supplier"},
		{name: "chat completions stream", path: "/v1/chat/completions", payload: hwChatRequest(hwTestModel, true), stream: true, want: "Hel"},
		{name: "messages", path: "/v1/messages", payload: hwClaudeRequest(hwTestModel, false), want: "hello from the supplier"},
		{name: "messages stream", path: "/v1/messages", payload: hwClaudeRequest(hwTestModel, true), stream: true, want: "Hel"},
	}

	t.Run("structural masking without the word scan", func(t *testing.T) {
		middleware.HwSetProviderNetEnabled(false)
		t.Cleanup(func() { middleware.HwSetProviderNetEnabled(true) })

		env := hwNewEnv(t, "paid", 1_000_000)
		lumia := env.fake("lumia")
		openrouter := env.fake("openrouter")
		lumia.setMode(hwFakeTimeout)
		env.addChannel("lumia-primary", "paid", "lumia", 0, lumia)
		env.addChannel("emergency", "paid", "openrouter", 1, openrouter)

		for _, testCase := range cases {
			t.Run(testCase.name, func(t *testing.T) {
				recorder := env.post(testCase.path, testCase.payload)
				require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
				require.Positive(t, openrouter.callCount())
				assert.Contains(t, recorder.Body.String(), testCase.want)
				hwAssertCleanResponse(t, recorder, testCase.stream)
			})
		}

		// An error path after a real fallback: both suppliers fail.
		openrouter.setMode(hwFakeStatus429)
		recorder := env.post("/v1/chat/completions", hwChatRequest(hwTestModel, false))
		require.GreaterOrEqual(t, recorder.Code, http.StatusBadRequest, recorder.Body.String())
		hwAssertCleanResponse(t, recorder, false)

		self := hwCallHandler(t, env, GetSelf, http.MethodGet, "/api/user/self", common.RoleCommonUser)
		require.Equal(t, http.StatusOK, self.Code, self.Body.String())
		hwAssertNoSupplierWords(t, "/api/user/self", self.Body.String())
	})

	t.Run("the word scan replaces a leak the structure does not know", func(t *testing.T) {
		env := hwNewEnv(t, "paid", 1_000_000)
		lumia := env.fake("lumia")
		openrouter := env.fake("openrouter")
		lumia.setMode(hwFakeTimeout)
		env.addChannel("lumia-primary", "paid", "lumia", 0, lumia)
		env.addChannel("emergency", "paid", "openrouter", 1, openrouter)
		openrouter.enableLeakField(true)

		recorder := env.post("/v1/chat/completions", hwChatRequest(hwTestModel, false))
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		assert.Contains(t, recorder.Body.String(), "served by provider",
			"the last-resort scan must rewrite the supplier word it does not structurally know")
		hwAssertCleanResponse(t, recorder, false)

		// Control: with the scan off the same answer does leak, so the
		// assertion above can actually fail.
		middleware.HwSetProviderNetEnabled(false)
		t.Cleanup(func() { middleware.HwSetProviderNetEnabled(true) })
		recorder = env.post("/v1/chat/completions", hwChatRequest(hwTestModel, false))
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		assert.Contains(t, strings.ToLower(recorder.Body.String()), "served by openrouter",
			"without the scan the deliberately leaked word must reach the customer, otherwise the scan is untested")
	})
}
