package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The distributor resolves a retired model to its successor before channel
// selection, so the whole downstream chain serves and prices the successor.
// Without a channel for either name the selection error names the model it
// tried, which is exactly what these tests assert against.
func distributeWithModel(t *testing.T, tokenModelLimit map[string]bool, body string) *httptest.ResponseRecorder {
	t.Helper()
	require.NoError(t, i18n.Init())
	gin.SetMode(gin.TestMode)
	previousCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() { common.MemoryCacheEnabled = previousCacheEnabled })
	require.NoError(t, setting.UpdateModelRedirectsByJSONString(`{"retired-model":"successor-model"}`))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateModelRedirectsByJSONString("{}"))
	})

	router := gin.New()
	router.POST("/v1/chat/completions", RequestId(), func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		if tokenModelLimit != nil {
			common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, true)
			common.SetContextKey(c, constant.ContextKeyTokenModelLimit, tokenModelLimit)
		}
	}, Distribute(), func(c *gin.Context) {
		t.Error("a request without a serving channel must stop before the relay handler")
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept-Language", "en")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestDistributeRedirectsRetiredModelBeforeSelection(t *testing.T) {
	recorder := distributeWithModel(t, nil, `{"model":"retired-model"}`)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "successor-model")
	assert.NotContains(t, recorder.Body.String(), "retired-model",
		"selection must run on the successor, never on the retired name")
}

func TestDistributeLeavesCurrentModelAlone(t *testing.T) {
	recorder := distributeWithModel(t, nil, `{"model":"untouched-model"}`)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "untouched-model")
}

// A token limited to the retired name the customer typed keeps working: the
// limit check accepts the redirected name or the original one.
func TestDistributeTokenLimitAcceptsOriginalNameAcrossRedirect(t *testing.T) {
	recorder := distributeWithModel(t, map[string]bool{"retired-model": true}, `{"model":"retired-model"}`)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code,
		"the token limit must pass on the original name and fail later on the missing channel")
	assert.Contains(t, recorder.Body.String(), "successor-model")

	recorder = distributeWithModel(t, map[string]bool{"other-model": true}, `{"model":"retired-model"}`)
	require.Equal(t, http.StatusForbidden, recorder.Code,
		"a token limited to an unrelated model stays refused")
}
