package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const hwKeyPassword = "enrollment-password"

func hwKeySetup(t *testing.T, withTwoFA bool) (*model.User, *model.Token) {
	t.Helper()
	user, _ := setupSecurityEnrollmentTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Token{}))
	token := &model.Token{UserId: user.Id, Name: "relay", Key: "stepupkey0123456789abcdefgh", Status: 1, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, model.DB.Create(token).Error)
	if withTwoFA {
		require.NoError(t, model.DB.Create(&model.TwoFA{UserId: user.Id, Secret: hwResetSecret, IsEnabled: true}).Error)
	}
	hwStepUpFailures.Lock()
	hwStepUpFailures.byUser = map[int][]time.Time{}
	hwStepUpFailures.Unlock()
	return user, token
}

func hwKeyCall(handler gin.HandlerFunc, userId int, tokenId int, body string) (*httptest.ResponseRecorder, map[string]any) {
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/token/"+strconv.Itoa(tokenId)+"/key", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(tokenId)}}
	c.Set("id", userId)
	handler(c)
	var out map[string]any
	_ = common.Unmarshal(response.Body.Bytes(), &out)
	return response, out
}

func hwKeyRefused(t *testing.T, response *httptest.ResponseRecorder, out map[string]any, token *model.Token, status int, code string) {
	t.Helper()
	assert.Equal(t, status, response.Code, response.Body.String())
	assert.Equal(t, false, out["success"])
	assert.Equal(t, code, out["code"])
	assert.NotContains(t, out, "data")
	assert.NotContains(t, response.Body.String(), token.Key)
}

func TestHwTokenKeyStepUpPasswordHappyPath(t *testing.T) {
	user, token := hwKeySetup(t, false)
	response, out := hwKeyCall(HwGetTokenKey, user.Id, token.Id, `{"password":"`+hwKeyPassword+`"}`)
	require.Equal(t, true, out["success"], response.Body.String())
	data := out["data"].(map[string]any)
	assert.Equal(t, token.GetFullKey(), data["key"])
}

func TestHwTokenKeyWithoutStepUpReturnsNoKey(t *testing.T) {
	user, token := hwKeySetup(t, false)
	for _, body := range []string{``, `{}`, `not json`, `{"password":""}`} {
		response, out := hwKeyCall(HwGetTokenKey, user.Id, token.Id, body)
		hwKeyRefused(t, response, out, token, http.StatusForbidden, "PASSWORD_REQUIRED")
	}
}

func TestHwTokenKeyWrongPasswordRefusedAndLocksAfterRepeats(t *testing.T) {
	user, token := hwKeySetup(t, false)
	for i := 0; i < hwStepUpMaxFailures; i++ {
		response, out := hwKeyCall(HwGetTokenKey, user.Id, token.Id, `{"password":"wrong-password-1"}`)
		hwKeyRefused(t, response, out, token, http.StatusForbidden, "PASSWORD_INVALID")
	}
	// even the right password is refused while locked
	response, out := hwKeyCall(HwGetTokenKey, user.Id, token.Id, `{"password":"`+hwKeyPassword+`"}`)
	hwKeyRefused(t, response, out, token, http.StatusTooManyRequests, "STEP_UP_LOCKED")
}

func TestHwTokenKeyTwoFARequiredInvalidAndValid(t *testing.T) {
	user, token := hwKeySetup(t, true)
	// the password alone is not enough when 2FA is on
	response, out := hwKeyCall(HwGetTokenKey, user.Id, token.Id, `{"password":"`+hwKeyPassword+`"}`)
	hwKeyRefused(t, response, out, token, http.StatusForbidden, "TWO_FA_REQUIRED")
	response, out = hwKeyCall(HwGetTokenKey, user.Id, token.Id, `{"two_fa_code":"000000"}`)
	hwKeyRefused(t, response, out, token, http.StatusForbidden, "TWO_FA_INVALID")
	code, err := totp.GenerateCode(hwResetSecret, time.Now())
	require.NoError(t, err)
	response, out = hwKeyCall(HwGetTokenKey, user.Id, token.Id, `{"two_fa_code":"`+code+`"}`)
	require.Equal(t, true, out["success"], response.Body.String())
	assert.Equal(t, token.GetFullKey(), out["data"].(map[string]any)["key"])
}

func TestHwTokenKeyPasswordlessAccountWithoutTwoFACannotReveal(t *testing.T) {
	user, token := hwKeySetup(t, false)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", user.Id).Update("password", "").Error)
	response, out := hwKeyCall(HwGetTokenKey, user.Id, token.Id, `{"password":"anything-at-all"}`)
	hwKeyRefused(t, response, out, token, http.StatusForbidden, "STEP_UP_UNAVAILABLE")
}

func TestHwTokenKeysBatchNeedsStepUpAndKeepsIds(t *testing.T) {
	user, token := hwKeySetup(t, false)
	ids := `"ids":[` + strconv.Itoa(token.Id) + `]`
	response, out := hwKeyCall(HwGetTokenKeysBatch, user.Id, token.Id, `{`+ids+`}`)
	hwKeyRefused(t, response, out, token, http.StatusForbidden, "PASSWORD_REQUIRED")
	response, out = hwKeyCall(HwGetTokenKeysBatch, user.Id, token.Id, `{`+ids+`,"password":"wrong-password-1"}`)
	hwKeyRefused(t, response, out, token, http.StatusForbidden, "PASSWORD_INVALID")
	response, out = hwKeyCall(HwGetTokenKeysBatch, user.Id, token.Id, `{`+ids+`,"password":"`+hwKeyPassword+`"}`)
	require.Equal(t, true, out["success"], response.Body.String())
	keys := out["data"].(map[string]any)["keys"].(map[string]any)
	assert.Equal(t, token.GetFullKey(), keys[strconv.Itoa(token.Id)])
}

func TestHwTokenListStaysMaskedWithoutStepUp(t *testing.T) {
	user, token := hwKeySetup(t, false)
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/token/?p=1&size=10", nil)
	c.Set("id", user.Id)
	GetAllTokens(c)
	require.Equal(t, http.StatusOK, response.Code)
	assert.NotContains(t, response.Body.String(), token.Key)
	assert.Contains(t, response.Body.String(), token.GetMaskedKey())
}
