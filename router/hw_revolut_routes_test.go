package router

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHwWebhookRouteKeepsAcceptingBursts proves the webhook route carries no
// per-IP rate limit: a provider that redelivers a burst of correctly signed
// events after an outage must never run into 429 answers. The HMAC signature
// over the raw body is the route's authentication and is checked before any
// expensive work, so a broad counter would only drop legitimate retries.
func TestHwWebhookRouteKeepsAcceptingBursts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB, previousSQLitePath, previousMaster := model.DB, common.SQLitePath, common.IsMasterNode
	previousRedis, previousMemoryCache := common.RedisEnabled, common.MemoryCacheEnabled
	previousCriticalEnabled, previousCriticalNum, previousCriticalDuration := common.CriticalRateLimitEnable, common.CriticalRateLimitNum, common.CriticalRateLimitDuration
	common.IsMasterNode = false
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	// The middleware builds its limiter from these values when the route is
	// registered, so they must hold the production defaults here: otherwise a
	// limiter put back on the route would silently be a no-op and this test could
	// never fail.
	common.CriticalRateLimitEnable = true
	common.CriticalRateLimitNum = 20
	common.CriticalRateLimitDuration = 20 * 60
	common.SQLitePath = filepath.Join(t.TempDir(), "hw-webhook-routes.db")
	t.Setenv("SQL_DSN", "")
	t.Setenv("LOG_SQL_DSN", "")
	require.NoError(t, model.InitDB())
	t.Cleanup(func() {
		sqlDB, err := model.DB.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
		model.DB = previousDB
		common.SQLitePath = previousSQLitePath
		common.IsMasterNode = previousMaster
		common.RedisEnabled = previousRedis
		common.MemoryCacheEnabled = previousMemoryCache
		common.CriticalRateLimitEnable = previousCriticalEnabled
		common.CriticalRateLimitNum = previousCriticalNum
		common.CriticalRateLimitDuration = previousCriticalDuration
	})
	require.NoError(t, model.DB.AutoMigrate(&model.HwOrder{}))

	t.Setenv(service.HwRevolutWebhookSecretEnv, "hw-route-webhook-secret")
	engine := gin.New()
	SetHwWebhookRoutes(engine.Group("/api"))

	body := []byte(`{"event":"ORDER_COMPLETED","order_id":"","merchant_order_ext_ref":"hw-route-unknown"}`)
	answers := map[int]int{}
	for range 100 {
		at := time.Now()
		timestamp := strconv.FormatInt(at.UnixMilli(), 10)
		mac := hmac.New(sha256.New, []byte("hw-route-webhook-secret"))
		mac.Write([]byte("v1." + timestamp + "."))
		mac.Write(body)
		request := httptest.NewRequest(http.MethodPost, "/api/hw/revolut/webhook", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Revolut-Request-Timestamp", timestamp)
		request.Header.Set("Revolut-Signature", "v1="+hex.EncodeToString(mac.Sum(nil)))
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		answers[recorder.Code]++
	}
	assert.Equal(t, 100, answers[http.StatusOK], "every correctly signed event is answered 200, also in a burst: %v", answers)
	assert.NotContains(t, answers, http.StatusTooManyRequests)
}

// TestHwUpstreamPaymentWebhooksStayUnregistered proves the payment webhooks of
// the upstream project exist only when the deployment switches them on, and
// that the Revolut webhook HisarWeb AI does use is untouched by that switch.
// Registration alone is inspected; no handler behind these routes runs.
func TestHwUpstreamPaymentWebhooksStayUnregistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamRoutes := []string{
		"POST /api/stripe/webhook",
		"POST /api/creem/webhook",
		"POST /api/waffo/webhook",
		"POST /api/waffo-pancake/webhook/:env",
		"POST /api/user/epay/notify",
		"GET /api/user/epay/notify",
		"POST /api/subscription/epay/notify",
		"POST /api/subscription/epay/return",
		"POST /api/user/stripe/pay",
		"POST /api/user/creem/pay",
		"POST /api/user/waffo/pay",
	}
	register := func(enabled string) (*gin.Engine, map[string]bool) {
		t.Setenv(common.HwUpstreamPaymentsEnv, enabled)
		engine := gin.New()
		SetApiRouter(engine)
		registered := make(map[string]bool, len(engine.Routes()))
		for _, route := range engine.Routes() {
			registered[route.Method+" "+route.Path] = true
		}
		return engine, registered
	}

	engine, off := register("false")
	for _, route := range upstreamRoutes {
		assert.False(t, off[route], route)
	}
	assert.True(t, off["POST /api/hw/revolut/webhook"], "the Revolut webhook must stay registered")
	assert.True(t, off["POST /api/user/topup"], "redeeming a top-up code is not a payment provider and must stay reachable")

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/stripe/webhook", nil))
	assert.Equal(t, http.StatusNotFound, recorder.Code, "a switched-off webhook must answer like unknown paths")

	_, on := register("true")
	for _, route := range upstreamRoutes {
		assert.True(t, on[route], route)
	}
	assert.True(t, on["POST /api/hw/revolut/webhook"])
}

// TestHwProviderWebhooksSkipTheGlobalRateLimit proves the provider webhooks
// sit outside the global per-IP limit of the /api group: a burst of
// redeliveries gets no 429, while an ordinary /api route behind the same limit
// does. Each webhook authenticates itself, so these unauthenticated calls are
// refused by the handler, never by the limiter.
func TestHwProviderWebhooksSkipTheGlobalRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousRedis := common.RedisEnabled
	previousEnable, previousNum, previousDuration := common.GlobalApiRateLimitEnable, common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration
	common.RedisEnabled = false
	common.GlobalApiRateLimitEnable = true
	common.GlobalApiRateLimitNum = 5
	common.GlobalApiRateLimitDuration = 60
	t.Cleanup(func() {
		common.RedisEnabled = previousRedis
		common.GlobalApiRateLimitEnable = previousEnable
		common.GlobalApiRateLimitNum = previousNum
		common.GlobalApiRateLimitDuration = previousDuration
	})
	engine := gin.New()
	SetApiRouter(engine)

	burst := func(method, path, ip string) map[int]int {
		answers := map[int]int{}
		for range 20 {
			request := httptest.NewRequest(method, path, bytes.NewReader([]byte(`{}`)))
			request.RemoteAddr = ip + ":1234"
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)
			answers[recorder.Code]++
		}
		return answers
	}
	for i, path := range []string{"/api/hw/telegram/webhook", "/api/hw/revolut/webhook", "/api/hw/stripe/webhook"} {
		answers := burst(http.MethodPost, path, "203.0.113."+strconv.Itoa(i+1))
		assert.NotContains(t, answers, http.StatusTooManyRequests, "%s: %v", path, answers)
	}
	control := burst(http.MethodGet, "/api/hw/telegram/link", "203.0.113.9")
	assert.Positive(t, control[http.StatusTooManyRequests], "the global limit must still hold for ordinary routes: %v", control)
}
