package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func hwOnceSetup(t *testing.T, flag string) *model.User {
	t.Helper()
	configureTokenAutoGroupsTest(t, "5", `["default","vip"]`)
	user, _ := hwKeySetup(t, false)
	t.Setenv("SESSION_SECRET", "unit-test-session-secret")
	t.Setenv(model.HwTokenKeyHashOnlyEnv, flag)
	old := common.CryptoSecret
	common.CryptoSecret = "unit-test-crypto-secret"
	t.Cleanup(func() { common.CryptoSecret = old })
	return user
}

func hwOnceCreate(t *testing.T, user *model.User, name string) (map[string]any, model.Token) {
	t.Helper()
	request := map[string]any{"name": name, "expired_time": -1, "remain_quota": 0, "unlimited_quota": true, "group": "default"}
	ctx, recorder := newTokenAutoGroupsAuthenticatedContext(t, http.MethodPost, "/api/token/", request, user.Id)
	AddToken(ctx)
	var out map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &out))
	require.Equal(t, true, out["success"], recorder.Body.String())
	var row model.Token
	require.NoError(t, model.DB.Where("name = ?", name).First(&row).Error)
	return out, row
}

func TestHwTokenCreateShowsKeyOnceAndStoresOnlyHash(t *testing.T) {
	user := hwOnceSetup(t, "true")
	out, row := hwOnceCreate(t, user, "once-key")

	data, ok := out["data"].(map[string]any)
	require.True(t, ok, "the create response carries the key")
	key, _ := data["key"].(string)
	require.NotEmpty(t, key)
	assert.NotContains(t, row.Key, key, "the clear key is not stored")
	assert.Equal(t, model.HwTokenKeyHash(key), row.KeyHash)

	// It authenticates.
	got, err := model.ValidateUserToken(key)
	require.NoError(t, err)
	assert.Equal(t, row.Id, got.Id)

	// Showing it again is a 409, after the step-up proof, and nothing leaks.
	response, body := hwKeyCall(HwGetTokenKey, user.Id, row.Id, `{"password":"`+hwKeyPassword+`"}`)
	assert.Equal(t, http.StatusConflict, response.Code, response.Body.String())
	assert.Equal(t, "KEY_NOT_RECOVERABLE", body["code"])
	assert.NotContains(t, response.Body.String(), key)
}

func TestHwTokenCreateWithFlagOffKeepsTheOldBodyAndStorage(t *testing.T) {
	user := hwOnceSetup(t, "")
	out, row := hwOnceCreate(t, user, "old-way")
	assert.NotContains(t, out, "data", "the body is what it always was")
	assert.False(t, model.HwTokenKeyIsHashed(row.Key))
	assert.Len(t, row.Key, 48)
	assert.Equal(t, model.HwTokenKeyHash(row.Key), row.KeyHash)
}
