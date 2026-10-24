package controller

// What /api/status tells a stranger about this deployment. The answer is public
// and it used to carry the machine the gateway was built on (a localhost
// address, a localhost passkey domain, the upstream project as the passkey
// name), a list of Chinese chat-app deep links and two legal pages announced as
// absent. Everything here is read from the answer a visitor actually gets.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// hwStatusIdentityFixture gives a test an empty options table and the shipped
// code defaults, so the answer starts out exactly like a fresh deployment.
func hwStatusIdentityFixture(t *testing.T) {
	t.Helper()
	originalDB := model.DB
	originalMaster := common.IsMasterNode
	originalSystemName := common.SystemName
	originalServerAddress := system_setting.ServerAddress
	originalPasskey := *system_setting.GetPasskeySettings()
	originalLegal := *system_setting.GetLegalSettings()
	originalChats := setting.Chats2JsonString()
	originalOptions := common.OptionMap

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.Channel{}, &model.Ability{}))
	model.DB = db
	common.IsMasterNode = true
	common.OptionMap = map[string]string{}

	t.Cleanup(func() {
		model.DB = originalDB
		common.IsMasterNode = originalMaster
		common.SystemName = originalSystemName
		system_setting.ServerAddress = originalServerAddress
		*system_setting.GetPasskeySettings() = originalPasskey
		*system_setting.GetLegalSettings() = originalLegal
		require.NoError(t, setting.UpdateChatsByJsonString(originalChats))
		common.OptionMap = originalOptions
		sqlDB, err := db.DB()
		if err == nil {
			require.NoError(t, sqlDB.Close())
		}
	})
}

// hwStatusIdentityServe answers one /api/status request and decodes its data.
func hwStatusIdentityServe(t *testing.T) (map[string]any, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/status", nil)

	GetStatus(context)

	var payload struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	body := recorder.Body.String()
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload), body)
	require.True(t, payload.Success, body)
	return payload.Data, body
}

// TestHwStatusCarriesOurOwnPublicIdentity protects the answer every visitor and
// every browser tab reads: our address, our passkey name and domain, no chat
// app list, both legal pages present, and nothing of the machine underneath.
func TestHwStatusCarriesOurOwnPublicIdentity(t *testing.T) {
	hwStatusIdentityFixture(t)
	require.NoError(t, service.HwSeedDefaultOptions())

	data, body := hwStatusIdentityServe(t)

	assert.Equal(t, "https://api.hisarweb.be", data["server_address"])
	assert.Equal(t, "HisarWeb AI", data["passkey_display_name"])
	assert.Equal(t, "api.hisarweb.be", data["passkey_rp_id"])
	assert.Equal(t, []any{}, data["chats"], "no chat app is offered")
	assert.Equal(t, true, data["user_agreement_enabled"])
	assert.Equal(t, true, data["privacy_policy_enabled"])

	lower := strings.ToLower(body)
	assert.NotContains(t, lower, "localhost")
	assert.NotContains(t, lower, "new api")
	assert.NotContains(t, lower, "new_api")
	assert.NotContains(t, lower, "oneapi")
}

// TestHwStatusTellsWhetherCardPaymentIsOpen lets the top-up page show a calm
// "opening soon" state instead of an order that fails: card payment counts as
// open only with both the merchant key and the webhook secret in place.
func TestHwStatusTellsWhetherCardPaymentIsOpen(t *testing.T) {
	hwStatusIdentityFixture(t)
	t.Setenv(service.HwRevolutAPIBaseEnv, "")

	t.Setenv(service.HwRevolutSecretKeyEnv, "")
	t.Setenv(service.HwRevolutWebhookSecretEnv, "")
	data, _ := hwStatusIdentityServe(t)
	assert.Equal(t, false, data["hw_payment_available"])

	t.Setenv(service.HwRevolutSecretKeyEnv, "sk_test_key")
	data, _ = hwStatusIdentityServe(t)
	assert.Equal(t, false, data["hw_payment_available"], "without the webhook secret no order is ever confirmed")

	t.Setenv(service.HwRevolutWebhookSecretEnv, "wsk_test_secret")
	data, _ = hwStatusIdentityServe(t)
	assert.Equal(t, true, data["hw_payment_available"])
}

// TestHwStatusAnnouncesBonusAndValidityToVisitors: the landing page is public,
// so the bonus rules and the validity days come from the status answer.
func TestHwStatusAnnouncesBonusAndValidityToVisitors(t *testing.T) {
	hwStatusIdentityFixture(t)
	common.OptionMapRWMutex.Lock()
	common.OptionMap[model.HwBonusTryMinOption] = "500"
	common.OptionMap[model.HwBonusTryAmountOption] = "50"
	common.OptionMapRWMutex.Unlock()

	data, _ := hwStatusIdentityServe(t)
	assert.Equal(t, 30.0, data["hw_credit_validity_days"])
	rules, ok := data["hw_bonus_rules"].([]any)
	require.True(t, ok)
	require.Len(t, rules, 3)
	assert.Equal(t, map[string]any{"currency": "TRY", "min": 500.0, "amount": 50.0}, rules[0])
}

// TestHwStatusListsTheCurrenciesThatCanBeSold: the shop shows the currencies
// that can be charged right now, with their rates per dollar and the top-up
// bounds. Without Stripe only the lira is offered, and a euro rate older than a
// week takes the euro away without touching the others.
func TestHwStatusListsTheCurrenciesThatCanBeSold(t *testing.T) {
	hwStatusIdentityFixture(t)
	t.Setenv(service.HwRevolutAPIBaseEnv, "")
	t.Setenv(service.HwRevolutSecretKeyEnv, "")
	t.Setenv(service.HwRevolutWebhookSecretEnv, "")
	t.Setenv(service.HwStripeSecretKeyEnv, "")
	t.Setenv(service.HwStripeWebhookSecretEnv, "")
	common.OptionMapRWMutex.Lock()
	common.OptionMap[model.HwTryPerUsdOption] = "40"
	common.OptionMap[model.HwEurPerUsdOption] = "0.8"
	common.OptionMap[model.HwEurPerUsdDateOption] = time.Now().UTC().Format("2006-01-02")
	common.OptionMap[model.HwEurPresetBaseOption] = "0.8"
	common.OptionMapRWMutex.Unlock()

	data, _ := hwStatusIdentityServe(t)
	assert.Equal(t, []any{"TRY"}, data["hw_currencies"], "without Stripe only the lira is sold")

	t.Setenv(service.HwStripeSecretKeyEnv, "sk_test_key")
	t.Setenv(service.HwStripeWebhookSecretEnv, "whsec_test_secret")
	data, _ = hwStatusIdentityServe(t)
	assert.Equal(t, []any{"TRY", "EUR", "USD"}, data["hw_currencies"])
	assert.Equal(t, map[string]any{"TRY": "40", "EUR": "0.8", "USD": "1"}, data["hw_rates"])
	topUp, ok := data["hw_topup"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, map[string]any{"min": 100.0, "max": 20000.0, "presets": []any{250.0, 500.0, 1000.0, 2000.0, 5000.0}}, topUp["TRY"])
	assert.Equal(t, map[string]any{"min": 2.0, "max": 400.0, "presets": []any{8.0, 20.0, 40.0, 80.0, 200.0}}, topUp["EUR"])
	assert.Equal(t, map[string]any{"min": 3.0, "max": 500.0, "presets": []any{10.0, 25.0, 50.0, 100.0, 250.0}}, topUp["USD"])

	common.OptionMapRWMutex.Lock()
	common.OptionMap[model.HwEurPerUsdDateOption] = time.Now().UTC().AddDate(0, 0, -9).Format("2006-01-02")
	common.OptionMapRWMutex.Unlock()
	data, _ = hwStatusIdentityServe(t)
	assert.Equal(t, []any{"TRY", "USD"}, data["hw_currencies"])
	assert.NotContains(t, data["hw_rates"], "EUR")
	assert.NotContains(t, data["hw_topup"], "EUR")
}
