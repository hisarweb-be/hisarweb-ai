package middleware

import (
	"errors"
	"math"
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
			`"system_fingerprint":"fp-lumia","lumia_cost_usd":0.01,"nested":{"deep":[{"provider":"openrouter"}]}}`)
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

// hwMaskWriterFor builds a bare mask writer for the pure stream helpers.
func hwMaskWriterFor(anthropic bool, acceptLanguage string) *hwMaskWriter {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	if acceptLanguage != "" {
		c.Request.Header.Set("Accept-Language", acceptLanguage)
	}
	return &hwMaskWriter{ctx: c, anthropic: anthropic}
}

// TestHwErrorEnvelopeMessage proves that only the gateway's own error wording
// is reused: a supplier error code gives no message, and a numeric code is
// still read as text.
func TestHwErrorEnvelopeMessage(t *testing.T) {
	cases := []struct {
		name        string
		envelope    map[string]any
		wantMessage string
		wantCode    string
	}{
		{"own code keeps its message", map[string]any{"error": map[string]any{"code": "insufficient_user_quota", "message": "user quota is not enough"}}, "user quota is not enough", "insufficient_user_quota"},
		{"supplier code loses its message but keeps the code", map[string]any{"error": map[string]any{"code": "insufficient_quota", "message": "OpenRouter billing limit"}}, "", "insufficient_quota"},
		{"no error object", map[string]any{"message": "x"}, "", ""},
		{"error is not an object", map[string]any{"error": "boom"}, "", ""},
		{"numeric code is no own code", map[string]any{"error": map[string]any{"code": float64(429), "message": "slow down"}}, "", "429"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			message, code := hwErrorEnvelopeMessage(tc.envelope)
			assert.Equal(t, tc.wantMessage, message)
			assert.Equal(t, tc.wantCode, code)
		})
	}
}

func TestHwStringValue(t *testing.T) {
	assert.Equal(t, "abc", hwStringValue("abc"))
	assert.Equal(t, "429", hwStringValue(float64(429)))
	assert.Equal(t, "", hwStringValue(1.5), "a fractional number is no code")
	assert.Equal(t, "", hwStringValue(nil))
	assert.Equal(t, "", hwStringValue(true))
}

// TestHwMaskStreamErrorEvent proves the terminal error of a stream reaches the
// customer in the customer's own protocol, with our wording only.
func TestHwMaskStreamErrorEvent(t *testing.T) {
	t.Run("openai: own error text and code are kept, stream is closed", func(t *testing.T) {
		w := hwMaskWriterFor(false, "")
		out := string(w.streamErrorEvent([]byte(`{"error":{"code":"insufficient_user_quota","message":"user quota is not enough"}}`)))
		assert.True(t, strings.HasPrefix(out, "data: "), out)
		assert.True(t, strings.HasSuffix(out, "\n\ndata: [DONE]\n\n"), out)
		assert.Contains(t, out, `"message":"user quota is not enough"`)
		assert.Contains(t, out, `"code":"insufficient_user_quota"`)
		assert.Contains(t, out, `"type":"service_error"`)
	})
	t.Run("openai: supplier wording becomes the generic message", func(t *testing.T) {
		w := hwMaskWriterFor(false, "")
		out := string(w.streamErrorEvent([]byte(`{"error":{"code":"insufficient_quota","message":"OpenRouter billing limit reached for lumia"}}`)))
		assert.NotContains(t, out, "billing limit")
		assert.NotContains(t, strings.ToLower(out), "lumia")
		assert.Contains(t, out, service.HwGenericErrorMessage(w.ctx, http.StatusBadGateway))
		assert.Contains(t, out, `"code":"insufficient_quota"`, "the code is not wording and stays")
	})
	t.Run("anthropic: error event without a done marker", func(t *testing.T) {
		w := hwMaskWriterFor(true, "")
		out := string(w.streamErrorEvent([]byte(`{"error":{"code":"insufficient_user_quota","message":"user quota is not enough"}}`)))
		assert.True(t, strings.HasPrefix(out, "event: error\ndata: "), out)
		assert.True(t, strings.HasSuffix(out, "\n\n"), out)
		assert.NotContains(t, out, "[DONE]")
		assert.Contains(t, out, `"type":"api_error"`)
		assert.Contains(t, out, `"message":"user quota is not enough"`)
	})
	t.Run("a payload that is not json is passed on unchanged", func(t *testing.T) {
		w := hwMaskWriterFor(false, "")
		assert.Equal(t, "not json", string(w.streamErrorEvent([]byte("not json"))))
	})
	t.Run("the generic message follows the language of the request", func(t *testing.T) {
		w := hwMaskWriterFor(false, "tr-TR,tr;q=0.9")
		out := string(w.streamErrorEvent([]byte(`{"error":{"code":"insufficient_quota","message":"x"}}`)))
		assert.Contains(t, out, "Hizmet şu anda kullanılamıyor")
	})
	t.Run("a message with a supplier word is cleaned by the safety net", func(t *testing.T) {
		w := hwMaskWriterFor(false, "")
		out := string(w.streamErrorEvent([]byte(`{"error":{"code":"service_busy","message":"OpenRouter is busy"}}`)))
		assert.NotContains(t, strings.ToLower(out), "openrouter")
	})
}

// TestHwSplitStreamTerminal proves the end-of-stream marker is held back,
// including a marker cut in two by the write boundary, so an error event can
// replace it.
func TestHwSplitStreamTerminal(t *testing.T) {
	cases := []struct {
		name         string
		data         string
		anthropic    bool
		wantBody     string
		wantTerminal string
	}{
		{"empty", "", false, "", ""},
		{"openai marker at the end", "data: {\"a\":1}\n\ndata: [DONE]\n\n", false, "data: {\"a\":1}\n\n", "data: [DONE]\n\n"},
		{"marker followed by more data is no terminal", "data: [DONE]\n\ndata: {\"a\":1}\n\n", false, "data: [DONE]\n\ndata: {\"a\":1}\n\n", ""},
		{"marker cut in two", "data: {\"a\":1}\n\ndata: [DO", false, "data: {\"a\":1}\n\n", "data: [DO"},
		{"no marker", "data: {\"a\":1}\n\n", false, "data: {\"a\":1}\n\n", ""},
		{"anthropic message_stop", "event: content_block_stop\ndata: {}\n\nevent: message_stop\ndata: {}\n\n", true, "event: content_block_stop\ndata: {}\n\n", "event: message_stop\ndata: {}\n\n"},
		{"anthropic without message_stop", "event: ping\ndata: {}\n\n", true, "event: ping\ndata: {}\n\n", ""},
		{"an openai marker is no terminal for anthropic", "data: [DONE]\n\n", true, "data: [DONE]\n\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, terminal := hwSplitStreamTerminal([]byte(tc.data), tc.anthropic)
			assert.Equal(t, tc.wantBody, string(body))
			assert.Equal(t, tc.wantTerminal, string(terminal))
		})
	}
}

// TestHwStreamCarriesError proves a stream write counts as an error only when
// it holds a protocol error event, not when the model merely writes the word.
func TestHwStreamCarriesError(t *testing.T) {
	assert.True(t, hwStreamCarriesError([]byte("event: error\ndata: {}\n\n")))
	assert.True(t, hwStreamCarriesError([]byte("data: {\"error\":{\"message\":\"x\"}}\n\n")))
	assert.False(t, hwStreamCarriesError([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"error\"}}]}\n\n")))
	assert.False(t, hwStreamCarriesError([]byte("data: not json\n\n")))
	assert.False(t, hwStreamCarriesError([]byte("data: [DONE]\n\n")))
	assert.False(t, hwStreamCarriesError(nil))
}

func TestHwLooksLikeSSE(t *testing.T) {
	for _, data := range []string{"data: {}", "  \n event: x", ": ping", "id: 1", "retry: 10", ""} {
		assert.True(t, hwLooksLikeSSE([]byte(data)), data)
	}
	for _, data := range []string{`{"a":1}`, "<html>", "datum: x", "events: x"} {
		assert.False(t, hwLooksLikeSSE([]byte(data)), data)
	}
}

// TestHwFilterHandshake proves the upgrade response of a hijacked connection
// keeps the handshake working but loses every supplier trace.
func TestHwFilterHandshake(t *testing.T) {
	block := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: abc=\r\n" +
		"X-Or-Cost: 0.2\r\n" +
		"X-LiteLLM-Call-Id: 1\r\n" +
		"X-Served-By: served by Lumia\r\n" +
		"\r\n" +
		"\r\n"
	out := string(hwFilterHandshake([]byte(block)))
	lines := strings.Split(strings.TrimSuffix(out, "\r\n\r\n"), "\r\n")
	assert.Equal(t, "HTTP/1.1 101 Switching Protocols", lines[0], "the status line stays")
	assert.Contains(t, lines, "Upgrade: websocket", "protocol headers stay")
	assert.Contains(t, lines, "Connection: Upgrade")
	assert.Contains(t, lines, "Sec-WebSocket-Accept: abc=")
	assert.NotContains(t, strings.ToLower(out), "x-or-cost", "a header named after the supplier is dropped")
	assert.NotContains(t, strings.ToLower(out), "litellm")
	assert.NotContains(t, strings.ToLower(out), "lumia", "a supplier word in a value is replaced")
	assert.Contains(t, out, "X-Served-By: served by provider")
	assert.True(t, strings.HasSuffix(out, "\r\n\r\n"), "the head ends with one empty line")
	assert.False(t, strings.HasSuffix(out, "\r\n\r\n\r\n"), "an empty line in the block does not end up as a second empty line")

	t.Run("a line without a colon is kept", func(t *testing.T) {
		out := string(hwFilterHandshake([]byte("HTTP/1.1 200 OK\r\nstray line\r\n\r\n")))
		assert.Equal(t, "HTTP/1.1 200 OK\r\nstray line\r\n\r\n", out)
	})
}

type hwRecordingWriter struct {
	writes [][]byte
	failAt int
}

func (r *hwRecordingWriter) Write(data []byte) (int, error) {
	r.writes = append(r.writes, append([]byte(nil), data...))
	if r.failAt > 0 && len(r.writes) == r.failAt {
		return 0, errors.New("closed")
	}
	return len(data), nil
}

// TestHwHandshakeWriter proves the head is held until it is complete, filtered
// once, and that the traffic after it is never touched or delayed.
func TestHwHandshakeWriter(t *testing.T) {
	t.Run("a head split over two writes is filtered as one block", func(t *testing.T) {
		target := &hwRecordingWriter{}
		writer := &hwHandshakeWriter{target: target}
		first := []byte("HTTP/1.1 101 Switching Protocols\r\nX-Or-Cost: 1\r\nUpg")
		n, err := writer.Write(first)
		require.NoError(t, err)
		assert.Equal(t, len(first), n)
		assert.Empty(t, target.writes, "nothing leaves before the head is complete")

		second := []byte("rade: websocket\r\n\r\nframe")
		n, err = writer.Write(second)
		require.NoError(t, err)
		assert.Equal(t, len(second), n, "the caller is told every byte was taken")
		require.Len(t, target.writes, 2)
		assert.Equal(t, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\n\r\n", string(target.writes[0]))
		assert.Equal(t, "frame", string(target.writes[1]), "bytes after the head pass through")
	})
	t.Run("everything after the handshake is passed through untouched", func(t *testing.T) {
		target := &hwRecordingWriter{}
		writer := &hwHandshakeWriter{target: target}
		_, err := writer.Write([]byte("HTTP/1.1 101 Switching Protocols\r\n\r\n"))
		require.NoError(t, err)
		later := "frame\r\nX-Or-Cost: raw lumia payload\r\n\r\n"
		_, err = writer.Write([]byte(later))
		require.NoError(t, err)
		require.Len(t, target.writes, 2)
		assert.Equal(t, later, string(target.writes[1]), "the filter only reads the first block")
	})
	t.Run("a failing connection is reported", func(t *testing.T) {
		writer := &hwHandshakeWriter{target: &hwRecordingWriter{failAt: 1}}
		_, err := writer.Write([]byte("HTTP/1.1 101 Switching Protocols\r\n\r\n"))
		assert.Error(t, err, "a failed head write surfaces")

		writer = &hwHandshakeWriter{target: &hwRecordingWriter{failAt: 2}}
		_, err = writer.Write([]byte("HTTP/1.1 101 Switching Protocols\r\n\r\nframe"))
		assert.Error(t, err, "a failed write of the bytes after the head surfaces")
	})
}

// TestHwUsageNumber proves a token count is only trusted when it is a plausible
// non-negative number, so a corrupt supplier value never reaches the customer.
func TestHwUsageNumber(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  int
		ok    bool
	}{
		{"whole float", float64(42), 42, true},
		{"zero", float64(0), 0, true},
		{"negative float", float64(-1), 0, false},
		{"largest accepted", float64(math.MaxInt32), math.MaxInt32, true},
		{"too large", float64(math.MaxInt32) + 1, 0, false},
		{"int", 7, 7, true},
		{"negative int", -7, 0, false},
		{"string", "7", 0, false},
		{"nil", nil, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := hwUsageNumber(tc.value)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestHwOpenAIChatPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	contextFor := func(path string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, path, nil)
		return c
	}
	assert.True(t, hwOpenAIChatPath(contextFor("/v1/chat/completions")))
	assert.True(t, hwOpenAIChatPath(contextFor("/pg/chat/completions")))
	assert.False(t, hwOpenAIChatPath(contextFor("/v1/completions")))
	assert.False(t, hwOpenAIChatPath(contextFor("/v1/messages")))
	assert.False(t, hwOpenAIChatPath(contextFor("/v1/chat/models")), "only the completions route is answered in the chat shape")
	assert.False(t, hwOpenAIChatPath(nil))
	assert.False(t, hwOpenAIChatPath(&gin.Context{}), "a context without a request is no chat route")
}
