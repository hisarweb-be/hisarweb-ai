package middleware

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The supplier purchase price must never reach a customer: lumia_cost_usd is
// removed from a plain chat completion and from every chunk of a chat stream,
// including a final usage chunk that carries nothing else worth parsing.
func TestHwMaskStripsLumiaCostFromChat(t *testing.T) {
	t.Run("non-stream", func(t *testing.T) {
		engine := hwMaskRoute(t, http.MethodPost, "/v1/chat/completions", func(c *gin.Context) {
			c.Header("Content-Type", "application/json")
			c.String(http.StatusOK, `{"id":"chatcmpl-1","object":"chat.completion","model":"m",`+
				`"choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],`+
				`"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2,"lumia_cost_usd":0.0123},"lumia_cost_usd":0.0123}`)
		})
		recorder := hwMaskDo(t, engine, http.MethodPost, "/v1/chat/completions", "", nil)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		body := recorder.Body.String()
		assert.NotContains(t, body, "lumia_cost_usd", body)
		assert.NotContains(t, body, "0.0123", body)
		assert.Contains(t, body, `"total_tokens":2`, body)
		assert.Contains(t, body, `"content":"hi"`, body)
	})

	t.Run("stream", func(t *testing.T) {
		engine := hwMaskRoute(t, http.MethodPost, "/v1/chat/completions", func(c *gin.Context) {
			c.Header("Content-Type", "text/event-stream")
			c.String(http.StatusOK,
				"data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"}}]}\n\n"+
					"data: {\"lumia_cost_usd\":0.0123}\n\n"+
					"data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2},\"lumia_cost_usd\":0.0123}\n\n"+
					"data: [DONE]\n\n")
		})
		recorder := hwMaskDo(t, engine, http.MethodPost, "/v1/chat/completions", "", nil)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		body := recorder.Body.String()
		assert.NotContains(t, body, "lumia_cost_usd", body)
		assert.NotContains(t, body, "0.0123", body)
		assert.Contains(t, body, `"total_tokens":2`, body)
		assert.Contains(t, body, "data: [DONE]", body)
	})
}
