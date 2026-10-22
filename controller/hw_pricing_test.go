package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupHwPricingTestDB gives the pricing endpoints a fresh database with the
// HisarWeb pricing tables and mounts them on a test engine.
func setupHwPricingTestDB(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.HwPriceSnapshot{}, &model.HwQualityRun{}, &model.HwQualityScore{}))
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})

	engine := gin.New()
	hw := engine.Group("/api/hw")
	hw.GET("/pricing/compare", HwPricingCompare)
	hw.GET("/quality/latest", HwQualityLatest)
	return engine
}

// hwSnapshotFixture stores one current snapshot with known prices:
// claude-sonnet-5-5 at $2 per million input and $10 per million output tokens,
// USD/TRY 49.5523 of 2026-10-17.
func hwSnapshotFixture(t *testing.T, stale bool) {
	t.Helper()
	modelsJSON, err := common.Marshal(map[string]model.HwModelPrice{
		"claude-sonnet-5-5": {InputMicroUsd: 2_000_000, OutputMicroUsd: 10_000_000},
		"claude-haiku-4-5":  {InputMicroUsd: 1_000_000, OutputMicroUsd: 5_000_000},
	})
	require.NoError(t, err)
	staleAt := int64(0)
	if stale {
		staleAt = 1_770_000_000
	}
	snapshot := model.HwPriceSnapshot{
		PriceSourceUrl: "https://example.test/prices.json",
		FxSourceUrl:    "https://example.test/rates.xml",
		FetchedAt:      1_771_000_000,
		FxDate:         "2026-10-17",
		TryPerUsd:      "49.5523",
		Models:         string(modelsJSON),
		ListedCount:    4000,
		StaleAt:        staleAt,
	}
	require.NoError(t, model.HwSavePriceSnapshot(&snapshot))
}

func hwGetCompare(t *testing.T, engine *gin.Engine, query string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/hw/pricing/compare?"+query, nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	body := map[string]any{}
	if recorder.Body.Len() > 0 {
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	}
	return recorder, body
}

// TestHwPricingCompareAnswersOfficialAndPurchasePrices pins the numbers of the
// comparator for one monthly volume: 1M input and 0.5M output tokens cost $7
// officially, TRY 416.24 with the rate and KDV. Our price is half of that, and
// the first purchase gets the ledger's 50 % on top; later purchases pay our
// price. The
// expectations were computed independently with exact decimal math.
func TestHwPricingCompareAnswersOfficialAndPurchasePrices(t *testing.T) {
	engine := setupHwPricingTestDB(t)
	hwSnapshotFixture(t, false)

	recorder, body := hwGetCompare(t, engine, "model=claude-sonnet-5-5&input_tokens=1000000&output_tokens=500000")
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, true, body["success"])
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "claude-sonnet-5-5", data["model"])
	assert.Equal(t, "7", data["official_cost_usd"])
	assert.Equal(t, "416.24", data["official_cost_try"])
	assert.Equal(t, "104.06", data["first_purchase_try"])
	assert.Equal(t, "208.12", data["repeat_purchase_try"])
	assert.Equal(t, "312.18", data["savings_first_try"])
	assert.Equal(t, "208.12", data["savings_repeat_try"])
	assert.Equal(t, "49.5523", data["try_per_usd"])
	assert.Equal(t, true, data["prices_include_kdv"])

	// Sources and dates are always part of the answer.
	assert.Equal(t, "https://example.test/prices.json", data["price_source"])
	assert.Equal(t, time.Unix(1_771_000_000, 0).UTC().Format(time.RFC3339), data["price_fetched_at"])
	assert.Equal(t, "https://example.test/rates.xml", data["fx_source"])
	assert.Equal(t, "2026-10-17", data["fx_date"])
	assert.Equal(t, false, data["stale"])

	// A second volume that crosses the first-purchase value cap: 1B output
	// tokens at $10 per million is $10,000 officially.
	recorder, body = hwGetCompare(t, engine, "model=claude-sonnet-5-5&input_tokens=0&output_tokens=1000000000")
	require.Equal(t, http.StatusOK, recorder.Code)
	data, ok = body["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "594627.6", data["official_cost_try"])
	assert.Equal(t, "296813.8", data["first_purchase_try"])
	assert.Equal(t, "297313.8", data["repeat_purchase_try"])
	assert.Equal(t, "297813.8", data["savings_first_try"])
	assert.Equal(t, "297313.8", data["savings_repeat_try"])
}

// TestHwPricingCompareMarksStaleSnapshot pins the stale flag of a snapshot
// whose refresh failed.
func TestHwPricingCompareMarksStaleSnapshot(t *testing.T) {
	engine := setupHwPricingTestDB(t)
	hwSnapshotFixture(t, true)

	recorder, body := hwGetCompare(t, engine, "model=claude-haiku-4-5&input_tokens=1000&output_tokens=1000")
	require.Equal(t, http.StatusOK, recorder.Code)
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, data["stale"])
	assert.Equal(t, "2026-10-17", data["fx_date"])
}

// TestHwPricingCompareRejectsUnknownAndInvalidInput pins the error paths: an
// unknown or unsold model is a 404, malformed token counts and a missing model
// are a 400, and no answer ever names an upstream provider.
func TestHwPricingCompareRejectsUnknownAndInvalidInput(t *testing.T) {
	engine := setupHwPricingTestDB(t)
	hwSnapshotFixture(t, false)

	unknown := []string{
		"model=gpt-9-turbo&input_tokens=1000&output_tokens=1000",
		"model=claude-opus-5-5&input_tokens=1000&output_tokens=1000", // sold, but not in this snapshot
	}
	for _, query := range unknown {
		recorder, body := hwGetCompare(t, engine, query)
		assert.Equal(t, http.StatusNotFound, recorder.Code, query)
		assert.Equal(t, false, body["success"], query)
	}

	invalid := []string{
		"model=&input_tokens=1000&output_tokens=1000",
		"model=claude-sonnet-5-5&input_tokens=-5&output_tokens=1000",
		"model=claude-sonnet-5-5&input_tokens=abc&output_tokens=1000",
		"model=claude-sonnet-5-5&input_tokens=1000&output_tokens=1.5",
		"model=claude-sonnet-5-5&input_tokens=1000&output_tokens=100000000001",
		"model=claude-sonnet-5-5&input_tokens=0&output_tokens=0",
	}
	for _, query := range invalid {
		recorder, body := hwGetCompare(t, engine, query)
		assert.Equal(t, http.StatusBadRequest, recorder.Code, query)
		assert.Equal(t, false, body["success"], query)
	}

	// No provider name in any answer, case-insensitive.
	for _, query := range append(unknown, invalid...) {
		recorder, _ := hwGetCompare(t, engine, query)
		lower := strings.ToLower(recorder.Body.String())
		assert.NotContains(t, lower, "lumia", query)
		assert.NotContains(t, lower, "openrouter", query)
	}

	// The successful answer carries none either.
	recorder, _ := hwGetCompare(t, engine, "model=claude-sonnet-5-5&input_tokens=1000&output_tokens=1000")
	lower := strings.ToLower(recorder.Body.String())
	assert.NotContains(t, lower, "lumia")
	assert.NotContains(t, lower, "openrouter")
}

// TestHwPricingCompareWithoutSnapshotIsHonest pins the empty state: without a
// snapshot the comparator answers a plain 404 instead of a made-up price.
func TestHwPricingCompareWithoutSnapshotIsHonest(t *testing.T) {
	engine := setupHwPricingTestDB(t)
	recorder, body := hwGetCompare(t, engine, "model=claude-sonnet-5-5&input_tokens=1000&output_tokens=1000")
	assert.Equal(t, http.StatusNotFound, recorder.Code)
	assert.Equal(t, false, body["success"])
}

// TestHwQualityLatestIsEmptyWithoutMeasurement pins the public quality answer
// without any run: an empty list with the sample note, nothing invented.
func TestHwQualityLatestIsEmptyWithoutMeasurement(t *testing.T) {
	engine := setupHwPricingTestDB(t)
	req := httptest.NewRequest(http.MethodGet, "/api/hw/quality/latest", nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Models []any  `json:"models"`
			Note   string `json:"note"`
			NoteEn string `json:"note_en"`
			File   string `json:"file"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	assert.True(t, body.Success)
	require.NotNil(t, body.Data.Models)
	assert.Empty(t, body.Data.Models)
	assert.Empty(t, body.Data.File)
	assert.NotEmpty(t, body.Data.Note)
	assert.NotEmpty(t, body.Data.NoteEn)

	lower := strings.ToLower(recorder.Body.String())
	assert.NotContains(t, lower, "lumia")
	assert.NotContains(t, lower, "openrouter")
}
