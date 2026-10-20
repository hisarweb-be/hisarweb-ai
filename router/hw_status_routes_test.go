// The public status route, proven over HTTP: it answers without any login, it
// carries every own model name with a Turkish and an English text, the running
// outage and the recent ones, and neither the body nor a header names a
// supplier.
//
// The underlying rows are planted directly, because the measurement of the
// minute and the incident chain are pinned in model/hw_status_test.go and
// service/hw_status_test.go.

package router

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hwStatusRouteEnv is a bare status engine on its own SQLite file, so the
// planted rows are the only rows.
func hwStatusRouteEnv(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedis := common.RedisEnabled
	previousMemoryCache := common.MemoryCacheEnabled
	previousMaster := common.IsMasterNode
	previousSQLitePath := common.SQLitePath
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()

	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	common.IsMasterNode = false
	common.SQLitePath = filepath.Join(t.TempDir(), "hw-status.db")
	t.Setenv("SQL_DSN", "")
	t.Setenv("LOG_SQL_DSN", "")
	require.NoError(t, model.InitDB())
	require.NoError(t, model.InitLogDB())
	sqlDB, err := model.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, model.DB.AutoMigrate(
		&model.Log{}, &model.Option{},
		&model.HwStatusSample{}, &model.HwIncident{}, &model.HwCreditExtension{},
		// The public quality and price answers are exercised over the same
		// engine, so their tables exist here as well; they stay empty.
		&model.HwQualityRun{}, &model.HwQualityScore{}, &model.HwPriceSnapshot{},
	))
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedis
		common.MemoryCacheEnabled = previousMemoryCache
		common.IsMasterNode = previousMaster
		common.SQLitePath = previousSQLitePath
		common.SetDatabaseTypes(previousMainType, previousLogType)
		_ = sqlDB.Close()
	})

	engine := gin.New()
	SetHwRouter(engine.Group("/api"))
	return engine
}

// hwStatusRouteBody is the part of the answer the test reads.
type hwStatusRouteBody struct {
	Success bool `json:"success"`
	Data    struct {
		CheckedAt        int64  `json:"checked_at"`
		AvailabilityDays int    `json:"availability_days"`
		Overall          string `json:"overall"`
		Models           []struct {
			Model               string  `json:"model"`
			Status              string  `json:"status"`
			AvailabilityPercent float64 `json:"availability_percent"`
			MessageTR           string  `json:"message_tr"`
			MessageEN           string  `json:"message_en"`
		} `json:"models"`
		Incident *struct {
			StartedAt int64  `json:"started_at"`
			EndedAt   int64  `json:"ended_at"`
			Duration  int64  `json:"duration"`
			MessageTR string `json:"message_tr"`
			MessageEN string `json:"message_en"`
		} `json:"incident"`
		Recent []struct {
			StartedAt int64  `json:"started_at"`
			EndedAt   int64  `json:"ended_at"`
			Duration  int64  `json:"duration"`
			MessageTR string `json:"message_tr"`
			MessageEN string `json:"message_en"`
		} `json:"recent"`
	} `json:"data"`
}

// hwStatusRouteServe runs one GET without any credentials.
func hwStatusRouteServe(t *testing.T, engine *gin.Engine, path string, header map[string]string) (*http.Response, string) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, bytes.NewReader(nil))
	for name, value := range header {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return recorder.Result(), recorder.Body.String()
}

func TestHwStatusRouteIsPublicAndNamesNoSupplier(t *testing.T) {
	engine := hwStatusRouteEnv(t)
	now := time.Now().Unix()
	minute := now - now%60
	// The last half hour is healthy with one bad minute behind it, and one
	// outage closed ten minutes ago.
	for at := minute - 30*60; at < minute; at += 60 {
		for _, name := range service.HwSoldModelNames() {
			require.NoError(t, model.DB.Create(&model.HwStatusSample{
				Minute: at, Model: name, MeasuredAt: at, Ok: true,
			}).Error)
		}
	}
	require.NoError(t, model.DB.Create(&model.HwIncident{
		StartedAt: minute - 20*60, EndedAt: minute - 10*60, Duration: 10 * 60,
		Kind: model.HwStatusIncidentRelay, Handled: true,
	}).Error)

	// No Authorization header, no token: the answer is public.
	response, body := hwStatusRouteServe(t, engine, "/api/hw/status", nil)
	require.Equal(t, http.StatusOK, response.StatusCode, body)

	var payload hwStatusRouteBody
	require.NoError(t, common.Unmarshal([]byte(body), &payload), body)
	require.True(t, payload.Success, body)
	assert.Equal(t, model.HwStatusAvailabilityDays, payload.Data.AvailabilityDays)
	assert.Equal(t, service.HwStatusOperational, payload.Data.Overall)

	// Every own model is listed, with a Turkish and an English text.
	names := service.HwSoldModelNames()
	require.Len(t, payload.Data.Models, len(names))
	seen := make(map[string]bool, len(names))
	for _, entry := range payload.Data.Models {
		seen[entry.Model] = true
		assert.Equal(t, service.HwStatusOperational, entry.Status)
		assert.Equal(t, 100.0, entry.AvailabilityPercent)
		assert.NotEmpty(t, entry.MessageTR)
		assert.NotEmpty(t, entry.MessageEN)
		// The supplier of a model is never part of the public name or text.
		assert.NotContains(t, strings.ToLower(entry.Model+" "+entry.MessageTR+" "+entry.MessageEN), "lumia")
		assert.NotContains(t, strings.ToLower(entry.Model+" "+entry.MessageTR+" "+entry.MessageEN), "openrouter")
	}
	for _, name := range names {
		assert.True(t, seen[name], "own model %s is missing from the status answer", name)
	}

	// The closed outage is in the recent list, not in the running incident.
	assert.Nil(t, payload.Data.Incident)
	require.Len(t, payload.Data.Recent, 1)
	assert.Equal(t, int64(10*60), payload.Data.Recent[0].Duration)
	assert.NotEmpty(t, payload.Data.Recent[0].MessageTR)
	assert.NotEmpty(t, payload.Data.Recent[0].MessageEN)

	lower := strings.ToLower(body)
	assert.NotContains(t, lower, "lumia")
	assert.NotContains(t, lower, "openrouter")
	assert.NotContains(t, lower, "channel")
	assert.NotContains(t, lower, "upstream")
	assert.Empty(t, response.Header.Get("X-OpenRouter-Id"))
}

// TestHwStatusRouteCarriesThePublicReadBudget proves the budget the public
// reads really use over HTTP: twenty-five polls in quick succession are not
// rejected, the answer is cacheable for half a minute, and the budget still
// ends at sixty-one - a mobile carrier behind one address cannot be locked out
// after twenty visits, but a runaway client is still stopped.
func TestHwStatusRouteCarriesThePublicReadBudget(t *testing.T) {
	engine := hwStatusRouteEnv(t)
	remoteAddr := "198.51.100.40:41000"
	serve := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "/api/hw/status", nil)
		request.RemoteAddr = remoteAddr
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		return recorder
	}

	for attempt := 1; attempt <= 25; attempt++ {
		recorder := serve()
		require.Equal(t, http.StatusOK, recorder.Code, "poll %d must not be rejected", attempt)
		assert.Equal(t, "public, max-age=30", recorder.Header().Get("Cache-Control"))
	}
	for attempt := 26; attempt <= 60; attempt++ {
		require.Equal(t, http.StatusOK, serve().Code, "poll %d must still pass", attempt)
	}

	tooMany := serve()
	require.Equal(t, http.StatusTooManyRequests, tooMany.Code, "the sixty-first poll must be stopped")
	assert.Equal(t, "60", tooMany.Header().Get("Retry-After"))
	assert.Empty(t, tooMany.Header().Get("Cache-Control"), "a rejection must never be cached")
}

// TestHwPublicReadRoutesShareTheRoomierBudget proves the other two public
// reads are on the same roomy budget: twenty-five polls of each are answered,
// not rejected, and the shared budget still closes at sixty-one. The quality
// answer is cacheable; the comparator answer depends on the query and is not.
func TestHwPublicReadRoutesShareTheRoomierBudget(t *testing.T) {
	engine := hwStatusRouteEnv(t)
	remoteAddr := "198.51.100.41:41000"
	get := func(target string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request.RemoteAddr = remoteAddr
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		return recorder
	}

	// Fifty polls over the two routes, all answered: the old critical budget
	// of twenty in twenty minutes would have rejected the twenty-first.
	for attempt := 1; attempt <= 25; attempt++ {
		quality := get("/api/hw/quality/latest")
		require.Equal(t, http.StatusOK, quality.Code, "quality poll %d: %s", attempt, quality.Body.String())
		assert.Equal(t, "public, max-age=30", quality.Header().Get("Cache-Control"))

		compare := get("/api/hw/pricing/compare?model=claude-sonnet-5-5&input_tokens=1000&output_tokens=1000")
		require.Equal(t, http.StatusNotFound, compare.Code, "comparator poll %d: %s", attempt, compare.Body.String())
		assert.Empty(t, compare.Header().Get("Cache-Control"), "the comparator answer must not be cached")
	}

	for attempt := 51; attempt <= 60; attempt++ {
		require.Equal(t, http.StatusOK, get("/api/hw/quality/latest").Code, "poll %d must still pass", attempt)
	}
	assert.Equal(t, http.StatusTooManyRequests, get("/api/hw/status").Code,
		"the sixty-first public read must be stopped, whichever of the three it is")
}

func TestHwStatusRouteShowsRunningOutage(t *testing.T) {
	engine := hwStatusRouteEnv(t)
	now := time.Now().Unix()
	minute := now - now%60
	// The last four minutes are bad: an outage that runs and made the models
	// degraded until it opened.
	for at := minute - 4*60; at < minute; at += 60 {
		for _, name := range service.HwSoldModelNames() {
			require.NoError(t, model.DB.Create(&model.HwStatusSample{
				Minute: at, Model: name, MeasuredAt: at, Ok: false, Reason: model.HwStatusReasonRelayFailures,
			}).Error)
		}
	}
	require.NoError(t, model.DB.Create(&model.HwIncident{
		StartedAt: minute - 4*60, Kind: model.HwStatusIncidentRelay,
	}).Error)

	_, body := hwStatusRouteServe(t, engine, "/api/hw/status", nil)
	var payload hwStatusRouteBody
	require.NoError(t, common.Unmarshal([]byte(body), &payload), body)
	assert.Equal(t, service.HwStatusOutage, payload.Data.Overall)
	require.NotNil(t, payload.Data.Incident)
	assert.Zero(t, payload.Data.Incident.EndedAt)
	assert.NotEmpty(t, payload.Data.Incident.MessageTR)
	assert.NotEmpty(t, payload.Data.Incident.MessageEN)
	for _, entry := range payload.Data.Models {
		assert.Equal(t, service.HwStatusOutage, entry.Status, entry.Model)
	}
	assert.NotContains(t, strings.ToLower(body), "lumia")
	assert.NotContains(t, strings.ToLower(body), "openrouter")
}
