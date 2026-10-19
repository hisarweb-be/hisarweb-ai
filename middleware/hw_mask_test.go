package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
