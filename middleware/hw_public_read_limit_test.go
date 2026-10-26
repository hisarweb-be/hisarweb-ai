// The public read routes carry their own budget. These tests pin the two
// properties that matters: the budget is roomier than the critical one but
// still ends, and it never spends the critical budget of the same address.

package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

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

var hwPublicReadClientCounter atomic.Int32

// hwPublicReadFreshClient returns an address nobody used before in this
// process. The in-memory limiter keeps its counters for the life of the
// process, so a fixed address would be spent by an earlier run of the same
// test under -count or -shuffle.
func hwPublicReadFreshClient() (string, string) {
	n := hwPublicReadClientCounter.Add(1)
	ip := fmt.Sprintf("198.19.%d.%d", n/250, n%250+1)
	return ip, ip + ":41000"
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
	_, remoteAddr := hwPublicReadFreshClient()
	for attempt := 1; attempt <= 60; attempt++ {
		recorder := hwPublicReadRequest(engine, "/public", remoteAddr)
		require.Equal(t, http.StatusOK, recorder.Code, "request %d must pass", attempt)
	}

	rejected := hwPublicReadRequest(engine, "/public", remoteAddr)
	require.Equal(t, http.StatusTooManyRequests, rejected.Code)
	assert.Equal(t, strconv.Itoa(60), rejected.Header().Get("Retry-After"))

	// Another address still has the full budget.
	_, otherAddr := hwPublicReadFreshClient()
	assert.Equal(t, http.StatusOK, hwPublicReadRequest(engine, "/public", otherAddr).Code)
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

	_, remoteAddr := hwPublicReadFreshClient()
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

// TestHwPublicReadRateLimitOnRedisSharesTheBudgetAcrossServers proves the
// Redis path keeps the same promise as the memory path (sixty a minute, then a
// rejection with the window as retry hint) in its own namespace, so two
// servers behind one address count together and never touch the critical
// budget.
func TestHwPublicReadRateLimitOnRedisSharesTheBudgetAcrossServers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redisServer, _ := useRateLimitMiniRedis(t)

	engine := gin.New()
	require.NoError(t, engine.SetTrustedProxies(nil))
	engine.GET("/public", HwPublicReadRateLimit(), func(c *gin.Context) { c.Status(http.StatusOK) })

	clientIP, remoteAddr := hwPublicReadFreshClient()
	for attempt := 1; attempt <= 60; attempt++ {
		require.Equal(t, http.StatusOK, hwPublicReadRequest(engine, "/public", remoteAddr).Code, "request %d must pass", attempt)
	}

	rejected := hwPublicReadRequest(engine, "/public", remoteAddr)
	require.Equal(t, http.StatusTooManyRequests, rejected.Code)
	assert.Equal(t, "60", rejected.Header().Get("Retry-After"))

	key := redisIPRateLimitKey("HR", clientIP)
	count, err := redisServer.Get(key)
	require.NoError(t, err)
	assert.Equal(t, "61", count, "the rejected request is counted in the shared window")
	assert.Equal(t, 60*time.Second, redisServer.TTL(key))
	assert.False(t, redisServer.Exists(redisIPRateLimitKey("CT", clientIP)), "the critical namespace stays untouched")

	_, otherAddr := hwPublicReadFreshClient()
	assert.Equal(t, http.StatusOK, hwPublicReadRequest(engine, "/public", otherAddr).Code, "another address keeps its own budget")
}
