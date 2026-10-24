package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func hwSetControllerOptions(t *testing.T, values map[string]string) {
	t.Helper()
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	previous := map[string]string{}
	for key, value := range values {
		previous[key] = common.OptionMap[key]
		common.OptionMap[key] = value
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		for key, value := range previous {
			if value == "" {
				delete(common.OptionMap, key)
			} else {
				common.OptionMap[key] = value
			}
		}
	})
}

// TestHwListUserPackagesReportsValidityAndBonusRules proves the dashboard gets
// what the validity card and the bonus badge draw: days left, the date and the
// rules from the settings (not hardcoded in the page).
func TestHwListUserPackagesReportsValidityAndBonusRules(t *testing.T) {
	const userID = 21
	hwSetControllerOptions(t, map[string]string{
		model.HwBonusTryMinOption:    "500",
		model.HwBonusTryAmountOption: "50",
		model.HwBonusUsdMinOption:    "10",
		model.HwBonusUsdAmountOption: "1",
		model.HwBonusEurMinOption:    "0",
		model.HwBonusEurAmountOption: "0",
	})
	engine := setupHwPackagesTestDB(t, userID)
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "hw-validity-user", Quota: 1000, AffCode: "hwvalidity"}).Error)
	now := common.GetTimestamp()
	until := now + 5*24*3600 - 60
	require.NoError(t, model.DB.Create(&model.HwCreditPackage{
		UserId: userID, Source: model.HwSourcePurchase, GrantedQuota: 1000,
		CreatedAt: now, ExpiresAt: until, OrderRef: "hw-validity-order",
	}).Error)

	data, ok := hwGetPackages(t, engine)["data"].(map[string]any)
	require.True(t, ok)
	validity, ok := data["validity"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "active", validity["status"])
	assert.Equal(t, float64(5), validity["days_remaining"])
	assert.Equal(t, float64(until), validity["valid_until"])

	rules, ok := data["bonus_rules"].([]any)
	require.True(t, ok)
	byCurrency := map[string]map[string]any{}
	for _, entry := range rules {
		rule := entry.(map[string]any)
		byCurrency[rule["currency"].(string)] = rule
	}
	assert.Equal(t, float64(500), byCurrency["TRY"]["min"])
	assert.Equal(t, float64(50), byCurrency["TRY"]["amount"])
	assert.Equal(t, float64(10), byCurrency["USD"]["min"])
	assert.Equal(t, float64(0), byCurrency["EUR"]["amount"], "a switched-off currency reports amount 0")
}

func TestHwListUserPackagesReportsExpiredAndNone(t *testing.T) {
	const userID = 22
	engine := setupHwPackagesTestDB(t, userID)
	require.NoError(t, model.DB.Create(&model.User{Id: userID, Username: "hw-none-user", Quota: 0, AffCode: "hwnone"}).Error)

	none := hwGetPackages(t, engine)["data"].(map[string]any)["validity"].(map[string]any)
	assert.Equal(t, "none", none["status"])

	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.HwCreditPackage{
		UserId: userID, Source: model.HwSourcePurchase, GrantedQuota: 700,
		CreatedAt: now - 40*24*3600, ExpiresAt: now - 10*24*3600, OrderRef: "hw-expired-order",
		RetiredAt: now - 9*24*3600, RetiredRemaining: 300,
	}).Error)
	expired := hwGetPackages(t, engine)["data"].(map[string]any)["validity"].(map[string]any)
	assert.Equal(t, "expired", expired["status"])
	assert.Equal(t, float64(300), expired["expired_quota"])
	assert.Equal(t, float64(now-10*24*3600), expired["expired_at"])
}

func TestHwListUserOrdersShowsTheBonusOfOnlyOwnPaidOrders(t *testing.T) {
	const userID = 23
	engine := setupHwPackagesTestDB(t, userID)
	require.NoError(t, model.DB.AutoMigrate(&model.HwOrder{}))
	orders := []model.HwOrder{
		{UserId: userID, MerchantRef: "own-paid", ListQuota: 1, ListValueTryMinor: 50000, PayableTryMinor: 50000, Currency: "TRY", BonusQuota: 10, BonusMinor: 5000, Status: model.HwOrderStatusPaid, CreatedAt: 100, PaidAt: 110, DiscountKind: "none"},
		{UserId: userID, MerchantRef: "own-pending", ListQuota: 1, ListValueTryMinor: 50000, PayableTryMinor: 50000, Currency: "TRY", BonusQuota: 10, BonusMinor: 5000, Status: model.HwOrderStatusPending, CreatedAt: 120, DiscountKind: "none"},
		{UserId: userID + 1, MerchantRef: "other-paid", ListQuota: 1, ListValueTryMinor: 50000, PayableTryMinor: 50000, Currency: "TRY", BonusQuota: 10, BonusMinor: 5000, Status: model.HwOrderStatusPaid, CreatedAt: 130, PaidAt: 140, DiscountKind: "none"},
	}
	require.NoError(t, model.DB.Create(&orders).Error)
	engine.Group("/api/user/hw").Use(func(c *gin.Context) { c.Set("id", userID) }).GET("/orders", HwListUserOrders)

	request := httptest.NewRequest(http.MethodGet, "/api/user/hw/orders", nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	body := map[string]any{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	list := body["data"].(map[string]any)["orders"].([]any)
	require.Len(t, list, 1, "an unpaid order and another user's order stay out")
	first := list[0].(map[string]any)
	assert.Equal(t, "own-paid", first["merchant_ref"])
	assert.Equal(t, float64(5000), first["bonus_minor"])
	assert.NotContains(t, first, "user_id")
	assert.NotContains(t, first, "admin_note")
}
