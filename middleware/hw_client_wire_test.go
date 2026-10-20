package middleware

// The client reads two own strings on every answer: the request-id response
// header and the type of a gateway-local error. Both are customer-visible, so
// neither may name the upstream project this gateway is forked from.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHwClientWireNamesAreNeutral proves the header a client logs and the type
// it branches on are our own words and carry no project name.
func TestHwClientWireNamesAreNeutral(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(RequestId())
	engine.GET("/probe", func(c *gin.Context) {
		abortWithOpenAiMessage(c, http.StatusForbidden, "model_not_found")
	})

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/probe", nil))
	response := recorder.Result()

	require.Equal(t, http.StatusForbidden, response.StatusCode, recorder.Body.String())
	requestId := response.Header.Get(common.RequestIdKey)
	require.NotEmpty(t, requestId, "every answer carries the request id under our own header")
	assert.Empty(t, response.Header.Get("X-Oneapi-Request-Id"), "the old header name is gone")
	assert.Contains(t, recorder.Body.String(), requestId, "the client can quote the id it was given")

	var payload struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload), recorder.Body.String())
	assert.Equal(t, "api_error", payload.Error.Type)
	assert.Equal(t, string(types.ErrorTypeNewAPIError), payload.Error.Type)
	assert.NotContains(t, strings.ToLower(recorder.Body.String()), "oneapi")
}
