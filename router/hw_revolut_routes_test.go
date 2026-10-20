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
	SetHwRoutes(engine.Group("/api"))

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
