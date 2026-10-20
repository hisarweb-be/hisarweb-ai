package service

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// hwFreeTrialQuotaEnv names the free trial amount in US dollars. Part D will
// replace this with the TRY amount converted against the daily rate; until
// then the operator sets one dollar value, which part A caps at its hard
// maximum anyway.
const hwFreeTrialQuotaEnv = "HW_FREE_TRIAL_QUOTA"

// hwFreeTrialDefaultDollars is the free trial default: three dollars, well
// below part A's ten-dollar cap, so a forgotten setting stays on the safe side.
const hwFreeTrialDefaultDollars = 3

// Bounds of an accepted X-HW-Device token: long enough to be a stable signal,
// short enough to stay a header value and nothing else.
const (
	hwDeviceTokenMinLen = 16
	hwDeviceTokenMaxLen = 128
)

var (
	// ErrHwInvalidDeviceToken rejects an X-HW-Device header that is neither a
	// hex string nor base64, or is too short to be a stable device signal.
	ErrHwInvalidDeviceToken = errors.New("invalid device header")
	hwHexTokenRe            = regexp.MustCompile(`^[0-9a-fA-F]+$`)
)

// HwFreeTrialQuota converts the configured free trial dollars to quota units
// and clamps the result to part A's hard cap, so no setting can hand out more
// than the cap allows.
func HwFreeTrialQuota() int {
	dollars := float64(hwFreeTrialDefaultDollars)
	if raw := strings.TrimSpace(os.Getenv(hwFreeTrialQuotaEnv)); raw != "" {
		if parsed, err := strconv.ParseFloat(raw, 64); err == nil && parsed >= 0 {
			dollars = parsed
		}
	}
	quota := common.QuotaFromFloat(dollars * float64(common.QuotaPerUnit))
	if quota > model.HwMaxFreeTrialQuota() {
		quota = model.HwMaxFreeTrialQuota()
	}
	return quota
}

// HwNormalizeDeviceToken validates the X-HW-Device header: at most 128
// characters, at least 16, and either hexadecimal or standard base64. Hex is
// lowercased because its notation is case-insensitive; base64 stays as given
// because it is case-sensitive. An absent header yields an empty token and no
// error: old clients simply register without a device signal.
func HwNormalizeDeviceToken(header string) (string, error) {
	token := strings.TrimSpace(header)
	if token == "" {
		return "", nil
	}
	if len(token) < hwDeviceTokenMinLen || len(token) > hwDeviceTokenMaxLen {
		return "", ErrHwInvalidDeviceToken
	}
	if hwHexTokenRe.MatchString(token) {
		return strings.ToLower(token), nil
	}
	if _, err := base64.StdEncoding.DecodeString(token); err == nil {
		return token, nil
	}
	return "", ErrHwInvalidDeviceToken
}

// HwCompleteTelegramPhoneStep records the Telegram identity of an account as
// the weak Telegram signal, asks the bot to prompt for the phone number and
// then tries the free trial credit. Without a number shared through the bot the
// grant answers "unverified": a Telegram login alone proves an account, not a
// phone number, and the same SIM can start a new Telegram account. The grant
// still passes the frozen flag, the confirmed-email gate and every part A
// check; the return is (granted, reason) with the same reason strings as part A
// plus "frozen" and "email_unverified". Nothing but the outcome is logged.
func HwCompleteTelegramPhoneStep(userID int, telegramID string) (granted bool, reason string, err error) {
	if err := model.HwRecordTelegramPhoneStep(userID, telegramID); err != nil {
		return false, "", err
	}
	// The Telegram login proves the account, not a phone number, so the credit
	// still waits for the number shared through the bot. The bot may only write
	// to a chat the customer opened itself, so this prompt reaches the customers
	// who started the bot before; for everyone else the same prompt answers the
	// /start that opens the chat. A failed prompt costs the prompt only, never
	// the login, and the customer's language is not known here: the site default
	// is Turkish.
	texts := hwTelegramTextsFor("tr")
	hwAnswerTelegramChat(context.Background(), telegramID, texts.Prompt, texts.Button)
	user, err := model.GetUserById(userID, false)
	if err != nil {
		return false, "", err
	}
	return hwMaybeGrantFreeTrialCredit(user)
}

// HwGrantFreeCreditIfPhoneStepSkipped is the local smoke-test path: only when
// HW_SKIP_PHONE_STEP is active outside production, a fresh registration gets a
// per-user dev phone signal and the free trial credit attempt. In production
// this returns phone_step_required and does nothing.
func HwGrantFreeCreditIfPhoneStepSkipped(userID int) (granted bool, reason string, err error) {
	if !common.HwPhoneStepSkipped() {
		return false, "phone_step_required", nil
	}
	if err := model.HwRecordDevPhoneStep(userID); err != nil {
		return false, "", err
	}
	user, err := model.GetUserById(userID, false)
	if err != nil {
		return false, "", err
	}
	return hwMaybeGrantFreeTrialCredit(user)
}

// hwMaybeGrantFreeTrialCredit runs the gates that sit in front of part A's
// grant: a frozen account gets nothing (the review hold), and without a
// confirmed email address nothing either. Logging carries amounts and user
// IDs only, never a signal value.
func hwMaybeGrantFreeTrialCredit(user *model.User) (granted bool, reason string, err error) {
	if user.Role != common.RoleCommonUser {
		return false, "not_common_user", nil
	}
	frozen, err := model.HwAccountFrozen(user.Id)
	if err != nil {
		return false, "", err
	}
	if frozen {
		return false, "frozen", nil
	}
	if user.Email == "" {
		return false, "email_unverified", nil
	}
	granted, reason, err = model.HwGrantFreeTrialCredit(user.Id, HwFreeTrialQuota(), common.GetTimestamp())
	if err != nil {
		return false, "", err
	}
	if granted {
		common.SysLog(common.LogText("hw: free trial credit granted to user %d", user.Id))
	}
	return granted, reason, nil
}
