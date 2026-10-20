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
	require.NoError(t, db.AutoMigrate(&model.Option{}))
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
