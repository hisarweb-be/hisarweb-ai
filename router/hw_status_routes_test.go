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
