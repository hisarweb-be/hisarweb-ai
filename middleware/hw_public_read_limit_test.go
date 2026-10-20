// The public read routes carry their own budget. These tests pin the two
// properties that matters: the budget is roomier than the critical one but
// still ends, and it never spends the critical budget of the same address.

package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func hwPublicReadRequest(engine *gin.Engine, path string, remoteAddr string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.RemoteAddr = remoteAddr
	engine.ServeHTTP(recorder, request)
	return recorder
}

// hwPublicReadUseMemoryLimiter selects the in-memory sliding window, as a
// server without Redis does.
func hwPublicReadUseMemoryLimiter(t *testing.T) {
	t.Helper()
	previousRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedis })
}

func TestHwPublicReadRateLimitAllowsSixtyAndRejectsTheSixtyFirst(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hwPublicReadUseMemoryLimiter(t)
	engine := gin.New()
	require.NoError(t, engine.SetTrustedProxies(nil))
	engine.GET("/public", HwPublicReadRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// One address, the way a mobile carrier presents itself. The numbers are
	// spelled out on purpose: sixty a minute is the promise to the visitor,
	// not an implementation detail.
	remoteAddr := "198.51.100.10:41000"
	for attempt := 1; attempt <= 60; attempt++ {
		recorder := hwPublicReadRequest(engine, "/public", remoteAddr)
		require.Equal(t, http.StatusOK, recorder.Code, "request %d must pass", attempt)
	}

	rejected := hwPublicReadRequest(engine, "/public", remoteAddr)
	require.Equal(t, http.StatusTooManyRequests, rejected.Code)
	assert.Equal(t, strconv.Itoa(60), rejected.Header().Get("Retry-After"))

	// Another address still has the full budget.
	assert.Equal(t, http.StatusOK,
		hwPublicReadRequest(engine, "/public", "198.51.100.11:41000").Code)
}

func TestHwPublicReadRateLimitKeepsTheCriticalBudgetSeparate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hwPublicReadUseMemoryLimiter(t)

	previousEnable := common.CriticalRateLimitEnable
	previousNum := common.CriticalRateLimitNum
	common.CriticalRateLimitEnable = true
	common.CriticalRateLimitNum = 20
	t.Cleanup(func() {
		common.CriticalRateLimitEnable = previousEnable
		common.CriticalRateLimitNum = previousNum
	})

	engine := gin.New()
	require.NoError(t, engine.SetTrustedProxies(nil))
	engine.GET("/public", HwPublicReadRateLimit(), func(c *gin.Context) { c.Status(http.StatusOK) })
	engine.GET("/critical", CriticalRateLimit(), func(c *gin.Context) { c.Status(http.StatusOK) })

	remoteAddr := "198.51.100.20:41000"
	for attempt := 1; attempt <= 60; attempt++ {
		require.Equal(t, http.StatusOK, hwPublicReadRequest(engine, "/public", remoteAddr).Code, attempt)
	}

	// Spending the whole public budget must leave the critical budget intact.
	for attempt := 1; attempt <= 20; attempt++ {
		require.Equal(t, http.StatusOK, hwPublicReadRequest(engine, "/critical", remoteAddr).Code, "critical request %d", attempt)
	}
	assert.Equal(t, http.StatusTooManyRequests, hwPublicReadRequest(engine, "/critical", remoteAddr).Code)
}

func TestHwPublicReadCacheMarksOnlySuccessfulAnswers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/ok", HwPublicReadCache(), func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"success": true}) })
	engine.GET("/broken", HwPublicReadCache(), func(c *gin.Context) {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false})
	})
	engine.GET("/own", HwPublicReadCache(), func(c *gin.Context) {
		c.Header("Cache-Control", "private, max-age=5")
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	ok := hwPublicReadRequest(engine, "/ok", "198.51.100.30:41000")
	require.Equal(t, http.StatusOK, ok.Code)
	assert.Equal(t, "public, max-age=30", ok.Header().Get("Cache-Control"))
	assert.Contains(t, ok.Body.String(), `"success":true`, "the answer itself must be untouched")

	broken := hwPublicReadRequest(engine, "/broken", "198.51.100.30:41000")
	require.Equal(t, http.StatusInternalServerError, broken.Code)
	assert.Empty(t, broken.Header().Get("Cache-Control"), "a failure must never be cached")

	own := hwPublicReadRequest(engine, "/own", "198.51.100.30:41000")
	require.Equal(t, http.StatusOK, own.Code)
	assert.Equal(t, "private, max-age=5", own.Header().Get("Cache-Control"))
}
