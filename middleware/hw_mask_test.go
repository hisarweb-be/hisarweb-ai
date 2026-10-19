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
