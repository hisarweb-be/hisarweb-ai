package controller

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// Step-up before a relay key is shown in full (ASVS V2.10.4, V3.5.2; gap G2).
// The caller proves it is the account owner again: a TOTP or backup code when
// 2FA is enabled, otherwise the account password. Same 403 codes as the reset
// flow (TWO_FA_REQUIRED, TWO_FA_INVALID) plus PASSWORD_REQUIRED and
// PASSWORD_INVALID. Nothing about the key is in a refusal.

const (
	hwStepUpMaxBody     = 1 << 20
	hwStepUpMaxFailures = 5
	hwStepUpWindow      = 5 * time.Minute
)

type hwTokenKeyStepUp struct {
	Password  string `json:"password"`
	TwoFACode string `json:"two_fa_code"`
}

// Failed password attempts per user. A password has no lockout of its own
// (TOTP has one in model.TwoFA), and the per-IP bucket alone would let one
// session guess from many addresses.
var hwStepUpFailures = struct {
	sync.Mutex
	byUser map[int][]time.Time
}{byUser: map[int][]time.Time{}}

func hwStepUpLocked(userId int, now time.Time) bool {
	hwStepUpFailures.Lock()
	defer hwStepUpFailures.Unlock()
	recent := hwStepUpFailures.byUser[userId][:0:0]
	for _, at := range hwStepUpFailures.byUser[userId] {
		if now.Sub(at) < hwStepUpWindow {
			recent = append(recent, at)
		}
	}
	if len(recent) == 0 {
		delete(hwStepUpFailures.byUser, userId)
	} else {
		hwStepUpFailures.byUser[userId] = recent
	}
	return len(recent) >= hwStepUpMaxFailures
}

func hwStepUpFailed(userId int, now time.Time) {
	hwStepUpFailures.Lock()
	defer hwStepUpFailures.Unlock()
	hwStepUpFailures.byUser[userId] = append(hwStepUpFailures.byUser[userId], now)
}

func hwStepUpForbidden(c *gin.Context, code, message string) {
	c.JSON(http.StatusForbidden, gin.H{"success": false, "message": message, "code": code})
}

// hwRequireKeyStepUp reads the step-up fields from the JSON body (and puts the
// body back for the wrapped handler). It answers and returns false when the
// proof is missing or wrong.
func hwRequireKeyStepUp(c *gin.Context) bool {
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, hwStepUpMaxBody))
	if err != nil {
		common.ApiError(c, err)
		return false
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	var proof hwTokenKeyStepUp
	if len(bytes.TrimSpace(raw)) > 0 {
		// A body that is not JSON simply carries no proof.
		_ = json.Unmarshal(raw, &proof)
	}
	userId := c.GetInt("id")
	user, err := model.GetUserById(userId, true)
	if err != nil {
		common.ApiError(c, err)
		return false
	}
	twoFA, err := model.GetTwoFAByUserId(userId)
	if err != nil {
		common.ApiError(c, err)
		return false
	}
	if twoFA != nil && twoFA.IsEnabled {
		code := strings.TrimSpace(proof.TwoFACode)
		if code == "" {
			hwStepUpForbidden(c, "TWO_FA_REQUIRED", "Enter your two-factor authentication code to show the key")
			return false
		}
		if verr := service.VerifyTwoFactorCode(twoFA, code); verr != nil {
			if errors.Is(verr, service.ErrVerificationFailed) || errors.Is(verr, service.ErrVerificationLocked) {
				hwStepUpForbidden(c, "TWO_FA_INVALID", "Invalid two-factor authentication code")
				return false
			}
			common.ApiError(c, verr)
			return false
		}
		return true
	}
	if user.Password == "" {
		hwStepUpForbidden(c, "STEP_UP_UNAVAILABLE", "Set a password or enable two-factor authentication to show full keys")
		return false
	}
	if proof.Password == "" {
		hwStepUpForbidden(c, "PASSWORD_REQUIRED", "Enter your password to show the key")
		return false
	}
	now := time.Now()
	if hwStepUpLocked(userId, now) {
		c.JSON(http.StatusTooManyRequests, gin.H{"success": false, "code": "STEP_UP_LOCKED", "message": "Too many failed attempts. Try again in a few minutes."})
		return false
	}
	if !common.ValidatePasswordAndHash(proof.Password, user.Password) {
		hwStepUpFailed(userId, now)
		hwStepUpForbidden(c, "PASSWORD_INVALID", "Incorrect password")
		return false
	}
	return true
}

// HwGetTokenKey replaces GetTokenKey on POST /api/token/:id/key.
func HwGetTokenKey(c *gin.Context) {
	if !hwRequireKeyStepUp(c) {
		return
	}
	GetTokenKey(c)
}

// HwGetTokenKeysBatch replaces GetTokenKeysBatch on POST /api/token/batch/keys.
func HwGetTokenKeysBatch(c *gin.Context) {
	if !hwRequireKeyStepUp(c) {
		return
	}
	GetTokenKeysBatch(c)
}
