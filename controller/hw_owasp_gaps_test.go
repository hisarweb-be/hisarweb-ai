package controller

import (
	"net/http"
	"net/http/httptest"
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

// G6: a TOTP code works once; a replay inside the window is refused and counted.
func TestTOTPCodeCannotBeReplayedInsideWindow(t *testing.T) {
	user, _ := hwResetSetup(t, true)
	twoFA, err := model.GetTwoFAByUserId(user.Id)
	require.NoError(t, err)
	code, err := totp.GenerateCode(hwResetSecret, time.Now())
	require.NoError(t, err)

	ok, err := twoFA.ValidateTOTPAndUpdateUsage(code)
	require.NoError(t, err)
	assert.True(t, ok, "first use is accepted")

	fresh, err := model.GetTwoFAByUserId(user.Id)
	require.NoError(t, err)
	ok, err = fresh.ValidateTOTPAndUpdateUsage(code)
	require.NoError(t, err)
	assert.False(t, ok, "same code again is refused")
	assert.Equal(t, 1, fresh.FailedAttempts, "a replay counts as a failure")

	// a wrong code still fails, and an older step than the stored one is refused
	older, err := totp.GenerateCode(hwResetSecret, time.Now().Add(-30*time.Second))
	require.NoError(t, err)
	another, err := model.GetTwoFAByUserId(user.Id)
	require.NoError(t, err)
	ok, _ = another.ValidateTOTPAndUpdateUsage(older)
	assert.False(t, ok)
	ok, _ = another.ValidateTOTPAndUpdateUsage("000000")
	assert.False(t, ok)
}

func TestTOTPNextStepIsAcceptedAfterUse(t *testing.T) {
	user, _ := hwResetSetup(t, true)
	twoFA, err := model.GetTwoFAByUserId(user.Id)
	require.NoError(t, err)
	// store a step in the past: the current code is newer and must pass
	require.NoError(t, model.DB.Model(&model.TwoFA{}).Where("id = ?", twoFA.Id).Update("last_used_step", time.Now().Unix()/30-5).Error)
	twoFA, err = model.GetTwoFAByUserId(user.Id)
	require.NoError(t, err)
	code, err := totp.GenerateCode(hwResetSecret, time.Now())
	require.NoError(t, err)
	ok, err := twoFA.ValidateTOTPAndUpdateUsage(code)
	require.NoError(t, err)
	assert.True(t, ok)
}

func hwLoginCall(username, password string) (*httptest.ResponseRecorder, map[string]any) {
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/login", strings.NewReader(`{"username":"`+username+`","password":"`+password+`"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	Login(c)
	var out map[string]any
	_ = common.Unmarshal(response.Body.Bytes(), &out)
	return response, out
}

// G4: repeated failures for one account are delayed, the right password is
// refused during the delay without being checked, and success resets it.
func TestLoginIsThrottledPerAccount(t *testing.T) {
	user, _ := setupSecurityEnrollmentTest(t)
	previous := common.PasswordLoginEnabled
	common.PasswordLoginEnabled = true
	t.Cleanup(func() { common.PasswordLoginEnabled = previous; common.LoginThrottleSuccess(user.Username) })
	common.LoginThrottleSuccess(user.Username)

	for i := 0; i < 5; i++ {
		response, out := hwLoginCall(user.Username, "wrong-password-x")
		assert.NotEqual(t, http.StatusTooManyRequests, response.Code)
		assert.Equal(t, false, out["success"])
	}
	response, _ := hwLoginCall(user.Username, "wrong-password-x")
	assert.NotEqual(t, http.StatusTooManyRequests, response.Code, "the sixth failure itself still gets the normal answer")

	response, out := hwLoginCall(user.Username, "enrollment-password")
	assert.Equal(t, http.StatusTooManyRequests, response.Code)
	assert.Equal(t, "LOGIN_THROTTLED", out["code"])
	assert.NotEmpty(t, response.Header().Get("Retry-After"))

	// another account is not affected
	response, _ = hwLoginCall("someone-else", "wrong-password-x")
	assert.NotEqual(t, http.StatusTooManyRequests, response.Code)

	// delay over: the right password logs in and clears the counter
	common.LoginThrottleSuccess(user.Username)
	response, out = hwLoginCall(user.Username, "enrollment-password")
	assert.Equal(t, true, out["success"], response.Body.String())
}
