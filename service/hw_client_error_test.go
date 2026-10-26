package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func hwClientErrorContext(acceptLanguage string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	if acceptLanguage != "" {
		c.Request.Header.Set("Accept-Language", acceptLanguage)
	}
	return c
}

func TestHwPrefersTurkishReadsOnlyTheFirstLanguage(t *testing.T) {
	for header, want := range map[string]bool{
		"":                     false,
		"tr":                   true,
		"TR-tr":                true,
		" tr ;q=0.9, en;q=0.8": true,
		"en-US,tr;q=0.9":       false,
		"trx":                  false,
		"en":                   false,
		"zh-CN":                false,
	} {
		assert.Equal(t, want, HwPrefersTurkish(hwClientErrorContext(header)), "Accept-Language %q", header)
	}
	assert.False(t, HwPrefersTurkish(nil))
	assert.False(t, HwPrefersTurkish(&gin.Context{}), "a context without a request is not Turkish")
}

func TestHwIsOwnErrorCodeKeepsOnlyGatewayCodes(t *testing.T) {
	for _, code := range []string{
		"service_busy", "stream_interrupted", "account_frozen", "window_limit_reached", "model_retired",
		string(types.ErrorCodeInsufficientUserQuota), string(types.ErrorCodeInvalidRequest), string(types.ErrorCodeAccessDenied),
	} {
		assert.True(t, HwIsOwnErrorCode(code), code)
	}
	for _, code := range []string{"", "insufficient_quota", "upstream_error", "rate_limit_exceeded"} {
		assert.False(t, HwIsOwnErrorCode(code), "supplier wording must still be replaced: %q", code)
	}
}

func TestHwGenericErrorMessageIsNeutralPerStatusAndLanguage(t *testing.T) {
	english, turkish := hwClientErrorContext(""), hwClientErrorContext("tr")
	statuses := []int{
		http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusUnauthorized,
		http.StatusForbidden, http.StatusNotFound, http.StatusRequestEntityTooLarge, http.StatusBadRequest,
		http.StatusUnprocessableEntity, http.StatusTeapot,
	}
	seen := map[string]int{}
	for _, status := range statuses {
		en, tr := HwGenericErrorMessage(english, status), HwGenericErrorMessage(turkish, status)
		assert.NotEmpty(t, en)
		assert.NotEmpty(t, tr)
		assert.NotEqual(t, en, tr, "status %d has a Turkish text", status)
		seen[en]++
	}
	// 401/403, 400/422 and 500/502 share a text; every other class has its own.
	assert.Len(t, seen, 7)
	assert.Equal(t, HwGenericErrorMessage(english, http.StatusInternalServerError), HwGenericErrorMessage(english, http.StatusBadGateway))
	assert.Equal(t, HwGenericErrorMessage(english, http.StatusUnauthorized), HwGenericErrorMessage(english, http.StatusForbidden))
}

func TestHwServiceBusyErrorSkipsRetryAndNamesNoSupplier(t *testing.T) {
	for _, header := range []string{"", "tr"} {
		c := hwClientErrorContext(header)
		apiErr := HwServiceBusyError(c)
		require.NotNil(t, apiErr)
		assert.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
		assert.Equal(t, HwErrorCodeServiceBusy, apiErr.GetErrorCode())
		assert.Equal(t, HwServiceBusyMessage(c), apiErr.Error())
		assert.True(t, types.IsSkipRetryError(apiErr), "an exhausted route must not be retried again")
	}
	assert.NotEqual(t, HwServiceBusyMessage(hwClientErrorContext("")), HwServiceBusyMessage(hwClientErrorContext("tr")))
}

func TestHwRetiredAndFrozenMessagesFollowTheLanguage(t *testing.T) {
	assert.Equal(t, "claude-old artık kullanılamıyor, claude-new kullanın.", HwModelRetiredMessage(hwClientErrorContext("tr"), "claude-old", "claude-new"))
	retired := HwModelRetiredMessage(hwClientErrorContext(""), "claude-old", "claude-new")
	assert.Contains(t, retired, "claude-old")
	assert.Contains(t, retired, "claude-new")

	assert.Contains(t, HwAccountFrozenMessage(hwClientErrorContext("tr")), "Hesabınız")
	assert.Contains(t, HwAccountFrozenMessage(hwClientErrorContext("")), "Paid credit still works")
}
