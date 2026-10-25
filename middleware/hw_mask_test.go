package middleware

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gin-gonic/gin"
)

// hwMaskRoute registers one handler behind the provider mask only, so the
// mask's own rewriting is observable without the relay stack.
func hwMaskRoute(t *testing.T, method, path string, handler gin.HandlerFunc) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Handle(method, path, HwMask(), handler)
	return engine
}

func hwMaskDo(t *testing.T, engine *gin.Engine, method, path, contentType string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(string(body)))
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return recorder
}

// Supplier-specific fields must disappear at every depth of a JSON body, not
// only at the top level.
func TestHwMaskDropsNestedSupplierFields(t *testing.T) {
	engine := hwMaskRoute(t, http.MethodPost, "/v1/chat/completions", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.String(http.StatusOK, `{"id":"chatcmpl-1","object":"chat.completion","model":"some-model",`+
			`"choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"native_finish_reason":"stop"}],`+
			`"usage":{"prompt_tokens":1,"total_tokens":2,"is_byok":true,`+
			`"cost_details":{"openrouter_cost":0.01,"x-or-fee":1}},`+
			`"system_fingerprint":"fp-lumia","nested":{"deep":[{"provider":"openrouter"}]}}`)
	})

	recorder := hwMaskDo(t, engine, http.MethodPost, "/v1/chat/completions", "", nil)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	body := recorder.Body.String()
	for _, banned := range []string{"native_finish_reason", "is_byok", "cost_details", "openrouter_cost", "x-or-fee", "system_fingerprint", "\"provider\"", "openrouter", "lumia"} {
		assert.NotContains(t, body, banned, "the customer body must not carry %q:\n%s", banned, body)
	}
	assert.Contains(t, body, `"prompt_tokens":1`, "the usage facts themselves stay:\n%s", body)
	assert.Contains(t, body, `"content":"hi"`, "the answer text stays:\n%s", body)
}

// Model output is the customer's own text. The last-resort scan must never
// rewrite it, including a Responses API delta and a word that merely contains
// the header-name pattern "x-or-".
func TestHwMaskNeverRewritesModelOutput(t *testing.T) {
	engine := hwMaskRoute(t, http.MethodPost, "/v1/responses", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.String(http.StatusOK,
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"I love my Lumia phone and box-or-x\"}\n\n"+
				"data: {\"type\":\"response.output_text.done\",\"text\":\"also box-or-x here\"}\n\n"+
				"data: [DONE]\n\n")
	})

	recorder := hwMaskDo(t, engine, http.MethodPost, "/v1/responses", "", nil)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	body := recorder.Body.String()
	assert.Contains(t, body, "I love my Lumia phone and box-or-x", "delta output is model text and stays verbatim:\n%s", body)
	assert.Contains(t, body, "also box-or-x here", "text output stays verbatim:\n%s", body)
	assert.Contains(t, body, "data: [DONE]", "the terminal marker stays:\n%s", body)
}

// A stored-responses id without supplier text must survive the mask verbatim,
// so the customer can send it back as previous_response_id and retrieve the
// stored response. A supplier generation id is still replaced.
func TestHwMaskKeepsUsableResponseIds(t *testing.T) {
	engine := hwMaskRoute(t, http.MethodPost, "/v1/responses", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.String(http.StatusOK,
			"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_8f3a2b91c6\",\"object\":\"response\",\"model\":\"m\"}}\n\n"+
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_8f3a2b91c6\",\"object\":\"response\",\"model\":\"m\",\"output\":[]}}\n\n"+
				"data: [DONE]\n\n")
	})
	recorder := hwMaskDo(t, engine, http.MethodPost, "/v1/responses", "", nil)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	body := recorder.Body.String()
	assert.Contains(t, body, `"id":"resp_8f3a2b91c6"`, "a plain responses id stays usable as previous_response_id:\n%s", body)
	assert.NotContains(t, body, "resp_hw-", "no unmotivated rewrite of a plain responses id:\n%s", body)

	engine = hwMaskRoute(t, http.MethodPost, "/v1/responses", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.String(http.StatusOK, `{"object":"response","id":"gen-9c2f71a0","status":"completed",`+
			`"model":"m","output":[]}`)
	})
	recorder = hwMaskDo(t, engine, http.MethodPost, "/v1/responses", "", nil)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	body = recorder.Body.String()
	assert.NotContains(t, body, "gen-9c2f71a0", "a supplier generation id never reaches the customer:\n%s", body)
	assert.Contains(t, body, `"id":"resp_hw-`, "the supplier id is rewritten to our own prefix:\n%s", body)
}

// Errors this gateway generated itself keep their own readable wording; only
// upstream errors are replaced by the generic text. The quota case shows the
// split: our code survives, the supplier's variant does not.
func TestHwMaskKeepsOwnErrorText(t *testing.T) {
	engine := hwMaskRoute(t, http.MethodPost, "/v1/chat/completions", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.String(http.StatusForbidden, `{"error":{"message":"user quota is not enough, please recharge",`+
			`"type":"insufficient_user_quota","code":"insufficient_user_quota"}}`)
	})
	recorder := hwMaskDo(t, engine, http.MethodPost, "/v1/chat/completions", "", nil)
	require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), "user quota is not enough, please recharge",
		"the gateway's own quota error keeps its readable wording:\n%s", recorder.Body.String())

	engine = hwMaskRoute(t, http.MethodPost, "/v1/chat/completions", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.String(http.StatusTooManyRequests, `{"error":{"message":"OpenRouter billing limit reached for lumia",`+
			`"type":"insufficient_quota","code":"insufficient_quota"}}`)
	})
	recorder = hwMaskDo(t, engine, http.MethodPost, "/v1/chat/completions", "", nil)
	require.Equal(t, http.StatusTooManyRequests, recorder.Code, recorder.Body.String())
	body := recorder.Body.String()
	assert.NotContains(t, body, "billing limit reached", "upstream wording is replaced:\n%s", body)
	assert.NotContains(t, body, "lumia", "no supplier name in the replacement:\n%s", body)
}

// Own /api routes may start a session and redirect; relay routes may not,
// because there a Set-Cookie or Location would come from the supplier side.
func TestHwMaskAllowsSessionHeadersOnApiRoutesOnly(t *testing.T) {
	setHeaders := func(c *gin.Context) {
		c.SetCookie("session", "opaque", 3600, "/", "", false, true)
		c.Header("Location", "/api/user/setup")
		c.String(http.StatusOK, "ok")
	}

	engine := hwMaskRoute(t, http.MethodPost, "/api/user/login", setHeaders)
	recorder := hwMaskDo(t, engine, http.MethodPost, "/api/user/login", "", nil)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.NotEmpty(t, recorder.Result().Header.Values("Set-Cookie"), "our own session cookie survives on /api:\n%s", recorder.Result().Header)
	assert.Equal(t, "/api/user/setup", recorder.Header().Get("Location"), "our own redirect target survives on /api:\n%s", recorder.Result().Header)

	engine = hwMaskRoute(t, http.MethodPost, "/v1/chat/completions", setHeaders)
	recorder = hwMaskDo(t, engine, http.MethodPost, "/v1/chat/completions", "", nil)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Empty(t, recorder.Result().Header.Values("Set-Cookie"), "a relay route never hands out cookies:\n%s", recorder.Result().Header)
	assert.Empty(t, recorder.Header().Get("Location"), "a relay route never redirects:\n%s", recorder.Result().Header)
}

// A panic raised inside the masking itself must never leak a half-masked
// supplier answer: the buffered case turns into a neutral 500 JSON body, the
// streaming case into our own error event. The panic detail stays server-side.
func TestHwMaskRecoversFilterPanic(t *testing.T) {
	hwFilterPanicHook = func() { panic("boom inside the mask") }
	defer func() { hwFilterPanicHook = nil }()

	engine := hwMaskRoute(t, http.MethodPost, "/v1/chat/completions", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.String(http.StatusOK, `{"id":"gen-1","object":"chat.completion","model":"m","choices":[]}`)
	})
	recorder := hwMaskDo(t, engine, http.MethodPost, "/v1/chat/completions", "", nil)
	require.Equal(t, http.StatusInternalServerError, recorder.Code, recorder.Body.String())
	body := recorder.Body.String()
	assert.NotContains(t, body, "gen-1", "no half-masked supplier id leaks:\n%s", body)
	assert.NotContains(t, body, "boom", "the panic detail stays server-side:\n%s", body)
	assert.Contains(t, body, `"error"`, "the customer gets a proper JSON error:\n%s", body)

	engine = hwMaskRoute(t, http.MethodPost, "/v1/chat/completions", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.String(http.StatusOK, "data: {\"id\":\"gen-2\",\"object\":\"chat.completion.chunk\",\"model\":\"m\",\"choices\":[]}\n\n")
	})
	recorder = hwMaskDo(t, engine, http.MethodPost, "/v1/chat/completions", "", nil)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	body = recorder.Body.String()
	assert.NotContains(t, body, "gen-2", "no half-masked supplier id leaks mid-stream:\n%s", body)
	assert.NotContains(t, body, "boom", "the panic detail stays server-side:\n%s", body)
	assert.Contains(t, body, `"error"`, "the stream ends with our own error event:\n%s", body)
}

// Values above the parse budget leave as text instead of being rewritten
// structurally, but the safety net still scrubs them: an oversized model
// answer must not become the one hole a supplier name slips through.
func TestHwMaskAppliesNetToOversizedValues(t *testing.T) {
	blob := strings.Repeat("a", hwMaxPendingPayload+64)
	engine := hwMaskRoute(t, http.MethodPost, "/v1/responses", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.String(http.StatusOK, "data: {\"type\":\"response.output_text.delta\",\"delta\":\""+blob+"openrouter tail")
	})
	recorder := hwMaskDo(t, engine, http.MethodPost, "/v1/responses", "", nil)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	body := recorder.Body.String()
	assert.NotContains(t, body, "openrouter", "the oversized value passes the safety net:\n%s", body[:64])
	assert.Contains(t, body, blob, "the oversized answer itself still reaches the customer")
	assert.Contains(t, body, "provider tail", "the supplier name became the placeholder")
}

// A chunk that carries nothing the mask rewrites skips JSON parsing and goes
// out byte for byte: spacing and key order survive. A chunk that names a
// supplier is still parsed and rewritten.
func TestHwMaskSkipsParsingForCleanChunks(t *testing.T) {
	verbatim := "data: {  \"zeta\" : 1 , \"alpha\" : [ 2 ] }\n\n"
	engine := hwMaskRoute(t, http.MethodPost, "/v1/chat/completions", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.String(http.StatusOK, verbatim)
	})
	recorder := hwMaskDo(t, engine, http.MethodPost, "/v1/chat/completions", "", nil)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Equal(t, verbatim, recorder.Body.String(), "a clean chunk passes through unparsed")

	engine = hwMaskRoute(t, http.MethodPost, "/v1/chat/completions", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.String(http.StatusOK, "data: {\"model\":\"openrouter/x\",\"alpha\":1}\n\n")
	})
	recorder = hwMaskDo(t, engine, http.MethodPost, "/v1/chat/completions", "", nil)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.NotContains(t, recorder.Body.String(), "openrouter", "a chunk naming the supplier is still rewritten")
}

// The "x-or-" token is a header-name pattern. A header that carries it is
// dropped, while body text with the same letters is left alone.
func TestHwMaskAppliesHeaderPatternToHeaderNamesOnly(t *testing.T) {
	engine := hwMaskRoute(t, http.MethodPost, "/v1/chat/completions", func(c *gin.Context) {
		c.Header("X-OpenRouter-Id", "gen-1")
		c.Header("Content-Type", "text/plain")
		c.String(http.StatusOK, "the box-or-club met at noon")
	})

	recorder := hwMaskDo(t, engine, http.MethodPost, "/v1/chat/completions", "", nil)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.NotContains(t, recorder.Result().Header, "X-OpenRouter-Id")
	assert.Equal(t, "the box-or-club met at noon", recorder.Body.String())
}

// The task id is the handle a customer polls with. It is issued by this gateway,
// so the mask leaves it alone, while a supplier-issued id on the same kind of
// object still goes.
func TestHwMaskKeepsOwnTaskIds(t *testing.T) {
	engine := hwMaskRoute(t, http.MethodGet, "/v1/videos/x", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.String(http.StatusOK, `{"id":"task_own_1","object":"video","status":"completed"}`)
	})
	recorder := hwMaskDo(t, engine, http.MethodGet, "/v1/videos/x", "", nil)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), `"id":"task_own_1"`)

	engine = hwMaskRoute(t, http.MethodGet, "/v1/videos/y", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		c.String(http.StatusOK, `{"id":"gen-77aa","object":"chat.completion","model":"m","choices":[]}`)
	})
	recorder = hwMaskDo(t, engine, http.MethodGet, "/v1/videos/y", "", nil)
	assert.NotContains(t, recorder.Body.String(), "gen-77aa")
}

// The customer sees the token facts of the protocol it speaks. Everything the
// relay uses to account for a request — the upstream billing snapshot, the
// usage-semantics tags, the split cache-write counters and the supplier cost —
// stays in our layer, and the field set is exactly the published one.
func TestHwMaskKeepsOnlyCustomerProtocolUsage(t *testing.T) {
	const claudeBody = `{"id":"msg_1","type":"message","role":"assistant","model":"openrouter/x",` +
		`"content":[],"stop_reason":"end_turn","usage":{"input_tokens":11,"output_tokens":5,` +
		`"cache_creation_input_tokens":3,"cache_read_input_tokens":2,` +
		`"cache_creation":{"ephemeral_5m_input_tokens":3,"ephemeral_1h_input_tokens":0},` +
		`"server_tool_use":{"web_search_requests":1,"code_execution_requests":2},` +
		`"claude_cache_creation_5_m_tokens":3,"claude_cache_creation_1_h_tokens":0,` +
		`"usage_semantic":"anthropic","usage_source":"claude_messages","cost":0.001,` +
		`"billing_usage":{"source":"claude_messages","semantic":"anthropic",` +
		`"openai_usage":{"prompt_tokens":11,"completion_tokens":5,"total_tokens":16}}}}`
	const chatBody = `{"id":"chatcmpl-1","object":"chat.completion","model":"some-model","choices":[],` +
		`"usage":{"prompt_tokens":11,"completion_tokens":5,"total_tokens":16,` +
		`"prompt_tokens_details":{"cached_tokens":2,"text_tokens":11,"image_tokens":0},` +
		`"completion_tokens_details":{"reasoning_tokens":0,"audio_tokens":0},` +
		`"billing_usage":{"source":"oai_chat","semantic":"openai","openai_usage":{"prompt_tokens":11}},` +
		`"input_tokens":11,"output_tokens":5,"prompt_cache_hit_tokens":2,` +
		`"usage_semantic":"openai","usage_source":"oai_chat","claude_cache_creation_5_m_tokens":0,"cost":0.001}}`

	for _, tc := range []struct {
		name, path, body, usageKey string
		wantUsage, wantNestedKey   []string
		nestedObject               string
		wantNested                 []string
	}{
		{
			name: "claude messages", path: "/v1/messages", body: claudeBody, usageKey: "usage",
			wantUsage:    []string{"cache_creation", "cache_creation_input_tokens", "cache_read_input_tokens", "input_tokens", "output_tokens", "server_tool_use"},
			nestedObject: "cache_creation", wantNested: []string{"ephemeral_1h_input_tokens", "ephemeral_5m_input_tokens"},
		},
		{
			name: "chat completions", path: "/v1/chat/completions", body: chatBody, usageKey: "usage",
			wantUsage:    []string{"completion_tokens", "completion_tokens_details", "prompt_tokens", "prompt_tokens_details", "total_tokens"},
			nestedObject: "prompt_tokens_details", wantNested: []string{"cached_tokens"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := hwMaskRoute(t, http.MethodPost, tc.path, func(c *gin.Context) {
				c.Header("Content-Type", "application/json")
				c.String(http.StatusOK, tc.body)
			})
			recorder := hwMaskDo(t, engine, http.MethodPost, tc.path, "", nil)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			body := recorder.Body.String()
			for _, banned := range []string{"billing_usage", "openai_usage", "usage_semantic", "usage_source",
				"claude_cache_creation_5_m_tokens", "claude_cache_creation_1_h_tokens", "cost", "text_tokens"} {
				assert.NotContains(t, body, banned, "the customer body must not carry %q:\n%s", banned, body)
			}
			var decoded map[string]any
			require.NoError(t, common.UnmarshalJsonStr(body, &decoded))
			usage, ok := decoded[tc.usageKey].(map[string]any)
			require.True(t, ok, "usage object survives:\n%s", body)
			assert.Equal(t, tc.wantUsage, hwSortedObjectKeys(usage))
			nested, ok := usage[tc.nestedObject].(map[string]any)
			require.True(t, ok, "%s stays:\n%s", tc.nestedObject, body)
			assert.Equal(t, tc.wantNested, hwSortedObjectKeys(nested))
		})
	}
}

// The same allowlist applies to every usage object inside an SSE stream: the
// nested message usage of message_start and the final message_delta usage.
func TestHwMaskKeepsOnlyCustomerProtocolUsageInStream(t *testing.T) {
	engine := hwMaskRoute(t, http.MethodPost, "/v1/messages", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.String(http.StatusOK,
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\","+
				"\"usage\":{\"input_tokens\":11,\"output_tokens\":1,\"billing_usage\":{\"openai_usage\":{\"prompt_tokens\":11}}}}}\n\n"+
				"event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":5,"+
				"\"claude_cache_creation_5_m_tokens\":0,\"usage_semantic\":\"anthropic\"}}\n\n"+
				"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	})
	recorder := hwMaskDo(t, engine, http.MethodPost, "/v1/messages", "", nil)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	body := recorder.Body.String()
	assert.NotContains(t, body, "billing_usage")
	assert.NotContains(t, body, "openai_usage")
	assert.NotContains(t, body, "usage_semantic")
	assert.NotContains(t, body, "claude_cache_creation_5_m_tokens")
	assert.Contains(t, body, `"input_tokens":11`)
	assert.Contains(t, body, `"output_tokens":5`)
}

func hwSortedObjectKeys(object map[string]any) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// The supplier counts a hidden prompt overhead that the customer never sent.
// The answer shows the corrected count, with the same rule the settlement bills
// with: reported minus overhead, never below zero or the local estimate.
func TestHwMaskAppliesSupplierPromptOverhead(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMap["HwUpstreamPromptOverhead"] = `{"*":800}`
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		delete(common.OptionMap, "HwUpstreamPromptOverhead")
	})

	for _, tc := range []struct {
		name, path, body, estimate string
		wantBody                   []string
		wantAbsent                 []string
	}{
		{
			name: "claude messages", path: "/v1/messages",
			body:     `{"id":"msg_1","type":"message","role":"assistant","content":[],"usage":{"input_tokens":807,"output_tokens":5}}`,
			estimate: "0", wantBody: []string{`"input_tokens":7`, `"output_tokens":5`},
		},
		{
			name: "short request keeps its local estimate", path: "/v1/messages",
			body:     `{"id":"msg_1","type":"message","role":"assistant","content":[],"usage":{"input_tokens":807,"output_tokens":5}}`,
			estimate: "9", wantBody: []string{`"input_tokens":9`, `"output_tokens":5`},
		},
		{
			name: "chat completions", path: "/v1/chat/completions",
			body:     `{"id":"chatcmpl-1","object":"chat.completion","choices":[],"usage":{"prompt_tokens":807,"completion_tokens":5,"total_tokens":812}}`,
			estimate: "0", wantBody: []string{`"prompt_tokens":7`, `"completion_tokens":5`, `"total_tokens":12`},
		},
		{
			// Without a completion count the reported total cannot be recomputed,
			// and the stale total would contradict prompt + completion.
			name: "chat completions without a completion count", path: "/v1/chat/completions",
			body:     `{"id":"chatcmpl-1","object":"chat.completion","choices":[],"usage":{"prompt_tokens":807,"total_tokens":812}}`,
			estimate: "0", wantBody: []string{`"prompt_tokens":7`}, wantAbsent: []string{"total_tokens"},
		},
		{
			name: "chat completions with an unreadable completion count", path: "/v1/chat/completions",
			body:     `{"id":"chatcmpl-1","object":"chat.completion","choices":[],"usage":{"prompt_tokens":807,"completion_tokens":"5","total_tokens":812}}`,
			estimate: "0", wantBody: []string{`"prompt_tokens":7`}, wantAbsent: []string{"total_tokens"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			engine := gin.New()
			engine.Handle(http.MethodPost, tc.path, func(c *gin.Context) {
				service.HwNoteChannelTag(c, "lumia")
				estimate, err := strconv.Atoi(tc.estimate)
				require.NoError(t, err)
				service.HwNoteLocalPromptEstimate(c, estimate)
				c.Next()
			}, HwMask(), func(c *gin.Context) {
				c.Header("Content-Type", "application/json")
				c.String(http.StatusOK, tc.body)
			})
			recorder := hwMaskDo(t, engine, http.MethodPost, tc.path, "", nil)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			for _, want := range tc.wantBody {
				assert.Contains(t, recorder.Body.String(), want, "the customer sees the corrected count:\n%s", recorder.Body.String())
			}
			for _, absent := range tc.wantAbsent {
				assert.NotContains(t, recorder.Body.String(), absent, "the customer sees no inconsistent count:\n%s", recorder.Body.String())
			}
		})
	}
}
