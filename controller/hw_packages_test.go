package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupHwPackagesTestDB gives the package endpoint a fresh database with one
// logged-in user and mounts the route behind a stand-in for UserAuth.
func setupHwPackagesTestDB(t *testing.T, userID int) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.HwCreditPackage{}, &model.HwPriceSnapshot{}, &model.HwCreditExtension{}, &model.HwUserSignal{}))
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})

	engine := gin.New()
	route := engine.Group("/api/user/hw")
	route.Use(func(c *gin.Context) {
		c.Set("id", userID)
		c.Next()
	})
	route.GET("/packages", HwListUserPackages)
	return engine
}

// hwPackageFixture stores one user with a known balance and two packages: a
// purchase of 1000 units expiring in 10 days and an affiliate reward of 500
// units expiring in 20 days.
func hwPackageFixture(t *testing.T, userID int, quota int, withSnapshot bool) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.User{
		Id:       userID,
		Username: "hw-packages-user",
		Quota:    quota,
		AffCode:  "hwpackages",
	}).Error)
	packages := []model.HwCreditPackage{
		{UserId: userID, Source: model.HwSourcePurchase, GrantedQuota: 1000, CreatedAt: 1_770_000_000, ExpiresAt: 1_770_864_000, OrderRef: "hw-pkg-order"},
		{UserId: userID, Source: model.HwSourceAffiliate, GrantedQuota: 500, CreatedAt: 1_770_000_000, ExpiresAt: 1_771_728_000, OrderRef: "hw-pkg-affiliate"},
	}
	require.NoError(t, model.DB.Create(&packages).Error)
	if !withSnapshot {
		return
	}
	modelsJSON, err := common.Marshal(map[string]model.HwModelPrice{
		"claude-sonnet-5-5": {InputMicroUsd: 2_000_000, OutputMicroUsd: 10_000_000},
	})
	require.NoError(t, err)
	require.NoError(t, model.HwSavePriceSnapshot(&model.HwPriceSnapshot{
		PriceSourceUrl: "https://example.test/prices.json",
		FxSourceUrl:    "https://example.test/rates.xml",
		FetchedAt:      1_771_000_000,
		FxDate:         "2026-10-17",
		TryPerUsd:      "49.5523",
		Models:         string(modelsJSON),
	}))
}

func hwGetPackages(t *testing.T, engine *gin.Engine) map[string]any {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/user/hw/packages", nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	body := map[string]any{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	return body
}

// TestHwListUserPackagesSplitsBalancePerPackage proves the endpoint fills the
// balance into the packages (latest expiry first, so the soonest expiring one
// holds what is left) and names the expiry of each. The balance (1200) is lower
// than the granted total (1500), so the purchase package holds 700 and the
// later affiliate package holds its full 500.
func TestHwListUserPackagesSplitsBalancePerPackage(t *testing.T) {
	const userID = 7
	engine := setupHwPackagesTestDB(t, userID)
	hwPackageFixture(t, userID, 1200, true)

	body := hwGetPackages(t, engine)
	require.Equal(t, true, body["success"])
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)

	packages, ok := data["packages"].([]any)
	require.True(t, ok)
	require.Len(t, packages, 2)
	first, ok := packages[0].(map[string]any)
	require.True(t, ok)
	second, ok := packages[1].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, model.HwSourcePurchase, first["source"])
	assert.Equal(t, float64(700), first["remaining_quota"])
	assert.Equal(t, float64(1_770_864_000), first["expires_at"])
	assert.Equal(t, model.HwSourceAffiliate, second["source"])
	assert.Equal(t, float64(500), second["remaining_quota"])
	assert.Equal(t, float64(1_771_728_000), second["expires_at"])
	assert.Equal(t, float64(1200), data["total_remaining_quota"])

	// No ledger ids leave the server: the package rows identify rows, not
	// packages the customer can act on.
	for _, entry := range packages {
		fields := entry.(map[string]any)
		assert.NotContains(t, fields, "id")
		assert.NotContains(t, fields, "user_id")
	}

	// The lira value per package comes from the current rate of the snapshot.
	assert.Equal(t, "49.5523", data["try_per_usd"])
	assert.Equal(t, "2026-10-17", data["fx_date"])
	assert.Equal(t, "https://example.test/rates.xml", data["fx_source"])
	assert.Equal(t, common.QuotaPerUnit, data["quota_per_unit"])
}

// TestHwListUserPackagesReportsPhoneStep proves the dashboard can mark the
// phone step of the start bonus: the flag is false while no number is shared
// and true once the bot stored one, so the card never shows a finished step
// as pending or the other way round.
func TestHwListUserPackagesReportsPhoneStep(t *testing.T) {
	const userID = 9
	t.Setenv("HW_SIGNAL_SALT", strings.Repeat("s", 32))
	engine := setupHwPackagesTestDB(t, userID)
	hwPackageFixture(t, userID, 1200, false)

	before, ok := hwGetPackages(t, engine)["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, before["phone_shared"])

	require.NoError(t, model.HwRecordPhoneSignal(userID, "+905321234567"))

	after, ok := hwGetPackages(t, engine)["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, after["phone_shared"])
}

// TestHwListUserPackagesWithoutRateNamesNoLiraValue keeps the answer honest
// when the daily price fetch has not run yet: the quota is still there, the
// rate is empty, and the page shows no made-up lira amount.
func TestHwListUserPackagesWithoutRateNamesNoLiraValue(t *testing.T) {
	const userID = 8
	engine := setupHwPackagesTestDB(t, userID)
	hwPackageFixture(t, userID, 1200, false)

	data, ok := hwGetPackages(t, engine)["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "", data["try_per_usd"])
	assert.Equal(t, "", data["fx_date"])
	assert.Equal(t, float64(1200), data["total_remaining_quota"])
}
