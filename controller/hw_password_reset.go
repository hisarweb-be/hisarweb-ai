package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

type HwPasswordResetRequest struct {
	Email    string `json:"email"`
	Token    string `json:"token"`
	Password string `json:"password"`
	// TwoFACode is a TOTP or backup code; required when the account has 2FA enabled.
	TwoFACode string `json:"two_fa_code"`
}

// HwResetPassword completes a mailed reset link. The caller chooses the new
// password; the server never generates or returns one (ASVS V2.5.6, V2.5.7).
// Accounts with 2FA must also present a TOTP or backup code, and a missing or
// wrong code leaves the one-time link usable until it expires.
func HwResetPassword(c *gin.Context) {
	var req HwPasswordResetRequest
	err := json.NewDecoder(c.Request.Body).Decode(&req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	req.Email = model.NormalizeEmail(req.Email)
	if req.Email == "" || req.Token == "" {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if req.Password == "" {
		// Contract change: the old route returned a generated password.
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Choose a new password: send \"password\" with the reset request",
			"code":    "PASSWORD_REQUIRED",
		})
		return
	}
	if !common.VerifyCodeWithKey(req.Email, req.Token, common.PasswordResetPurpose) {
		common.ApiErrorT(c, "Password reset link is invalid or has expired")
		return
	}
	if err = common.ValidateNewAccountPassword(req.Password); err != nil {
		common.ApiError(c, err)
		return
	}
	user, err := model.GetUniqueUserByEmail(req.Email)
	if err != nil {
		if errors.Is(err, model.ErrEmailNotFound) || errors.Is(err, model.ErrEmailAmbiguous) {
			common.ApiErrorT(c, "Password reset link is invalid or has expired")
			return
		}
		common.ApiError(c, err)
		return
	}
	twoFA, err := model.GetTwoFAByUserId(user.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if twoFA != nil && twoFA.IsEnabled {
		code := strings.TrimSpace(req.TwoFACode)
		if code == "" {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "Enter your two-factor authentication code to reset the password",
				"code":    "TWO_FA_REQUIRED",
			})
			return
		}
		if verr := service.VerifyTwoFactorCode(twoFA, code); verr != nil {
			if errors.Is(verr, service.ErrVerificationFailed) || errors.Is(verr, service.ErrVerificationLocked) {
				c.JSON(http.StatusForbidden, gin.H{
					"success": false,
					"message": "Invalid two-factor authentication code",
					"code":    "TWO_FA_INVALID",
				})
				return
			}
			common.ApiError(c, verr)
			return
		}
	}
	if err = model.ResetUserPasswordByEmail(req.Email, req.Password); err != nil {
		common.ApiError(c, err)
		return
	}
	common.DeleteKey(req.Email, common.PasswordResetPurpose)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}
