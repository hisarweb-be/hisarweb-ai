package service

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hwPhoneStepTestSalt keys the signal hashes during these tests. It is a test
// constant, not a secret: the production salt comes only from the environment.
const hwPhoneStepTestSalt = "service-test-signal-salt-0123456789abcdef"

// hwPhoneStepTestSetup starts from an empty abuse and credit ledger for one
// test. The shared service test database is created once in TestMain; the
// hw tables are migrated here because the service tests never assume them.
func hwPhoneStepTestSetup(t *testing.T) {
	t.Helper()
	t.Setenv("HW_SIGNAL_SALT", hwPhoneStepTestSalt)
	require.NoError(t, model.DB.AutoMigrate(
		&model.HwCreditPackage{}, &model.HwUserSignal{}, &model.HwFirstPurchaseClaim{},
		&model.HwFreeCreditClaim{}, &model.HwAccountFlag{}, &model.HwSignupIp{},
	))
	for _, table := range []string{
		"hw_credit_packages", "hw_user_signals", "hw_first_purchase_claims",
		"hw_free_credit_claims", "hw_account_flags", "hw_signup_ips", "users",
	} {
		require.NoError(t, model.DB.Exec("DELETE FROM "+table).Error)
	}
	t.Cleanup(func() {
		for _, table := range []string{
			"hw_credit_packages", "hw_user_signals", "hw_first_purchase_claims",
			"hw_free_credit_claims", "hw_account_flags", "hw_signup_ips", "users",
		} {
			model.DB.Exec("DELETE FROM " + table)
		}
	})
}

// hwPhoneStepTestUser creates a fresh common user; email is set when given.
func hwPhoneStepTestUser(t *testing.T, email string) model.User {
	t.Helper()
	user := model.User{
		Username:    "hw-step-" + common.GetRandomString(8),
		Password:    "unused-password-hash",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
		Email:       email,
		AffCode:     "hwaff" + common.GetRandomString(8),
	}
	require.NoError(t, model.DB.Create(&user).Error)
	return user
}

func hwPhoneStepUserQuota(t *testing.T, userID int) int {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Where("id = ?", userID).First(&user).Error)
	return user.Quota
}

func hwPhoneStepFreePackages(t *testing.T, userID int) []model.HwCreditPackage {
	t.Helper()
	var packages []model.HwCreditPackage
	require.NoError(t, model.DB.Where("user_id = ?", userID).Find(&packages).Error)
	return packages
}

func TestHwCompleteTelegramPhoneStep(t *testing.T) {
	hwPhoneStepTestSetup(t)

	t.Run("grants once and refuses the same telegram identity on a second account", func(t *testing.T) {
		first := hwPhoneStepTestUser(t, "first@example.com")
		granted, reason, err := HwCompleteTelegramPhoneStep(first.Id, "12345678")
		require.NoError(t, err)
		assert.True(t, granted, "the first account with a telegram step gets the free credit (reason %q)", reason)
		assert.Equal(t, 1, len(hwPhoneStepFreePackages(t, first.Id)))
		assert.Greater(t, hwPhoneStepUserQuota(t, first.Id), 0)
		var reloaded model.User
		require.NoError(t, model.DB.Where("id = ?", first.Id).First(&reloaded).Error)
		assert.Equal(t, "free", reloaded.Group, "the granted account moves to the free group")

		second := hwPhoneStepTestUser(t, "second@example.com")
		granted, reason, err = HwCompleteTelegramPhoneStep(second.Id, "12345678")
		require.NoError(t, err)
		assert.False(t, granted, "a second account with the same telegram identity is refused")
		assert.Equal(t, "shared_phone", reason)
		assert.Empty(t, hwPhoneStepFreePackages(t, second.Id))
		assert.Equal(t, 0, hwPhoneStepUserQuota(t, second.Id))

		// A different telegram identity still gets its own credit.
		third := hwPhoneStepTestUser(t, "third@example.com")
		granted, _, err = HwCompleteTelegramPhoneStep(third.Id, "87654321")
		require.NoError(t, err)
		assert.True(t, granted)
	})

	t.Run("without a confirmed email nothing is granted", func(t *testing.T) {
		user := hwPhoneStepTestUser(t, "")
		granted, reason, err := HwCompleteTelegramPhoneStep(user.Id, "11122233")
		require.NoError(t, err)
		assert.False(t, granted)
		assert.Equal(t, "email_unverified", reason)
		assert.Empty(t, hwPhoneStepFreePackages(t, user.Id))
		// The step itself is recorded: the phone signal exists even without a
		// grant, so a later confirmed email can still claim once.
		var signals []model.HwUserSignal
		require.NoError(t, model.DB.Where("user_id = ? AND kind = ?", user.Id, "phone").Find(&signals).Error)
		assert.Len(t, signals, 1)
	})

	t.Run("a frozen account gets nothing", func(t *testing.T) {
		user := hwPhoneStepTestUser(t, "frozen@example.com")
		frozen, err := model.FreezeHwAccounts([]int{user.Id}, model.HwFreezeReasonSharedDevice, common.GetTimestamp())
		require.NoError(t, err)
		require.Equal(t, 1, frozen)
		granted, reason, err := HwCompleteTelegramPhoneStep(user.Id, "33344455")
		require.NoError(t, err)
		assert.False(t, granted)
		assert.Equal(t, "frozen", reason)
		assert.Empty(t, hwPhoneStepFreePackages(t, user.Id))
	})

	t.Run("the stored phone signal is a keyed hash, never the telegram identity", func(t *testing.T) {
		user := hwPhoneStepTestUser(t, "hash@example.com")
		_, _, err := HwCompleteTelegramPhoneStep(user.Id, "99887766")
		require.NoError(t, err)
		var signals []model.HwUserSignal
		require.NoError(t, model.DB.Where("user_id = ?", user.Id).Find(&signals).Error)
		require.Len(t, signals, 1)
		assert.Equal(t, "phone", signals[0].Kind)
		assert.Len(t, signals[0].Hash, 64)
		assert.NotContains(t, signals[0].Hash, "99887766")
		var claims []model.HwFreeCreditClaim
		require.NoError(t, model.DB.Where("user_id = ?", user.Id).Find(&claims).Error)
		require.Len(t, claims, 1)
		assert.Len(t, claims[0].PhoneHash, 64)
		assert.NotContains(t, claims[0].PhoneHash, "99887766")
	})

	t.Run("the free amount follows the environment and part A's cap", func(t *testing.T) {
		assert.Equal(t, common.QuotaFromFloat(3*float64(common.QuotaPerUnit)), HwFreeTrialQuota())
		t.Setenv("HW_FREE_TRIAL_QUOTA", "2")
		assert.Equal(t, common.QuotaFromFloat(2*float64(common.QuotaPerUnit)), HwFreeTrialQuota())
		t.Setenv("HW_FREE_TRIAL_QUOTA", "9999")
		assert.Equal(t, model.HwMaxFreeTrialQuota(), HwFreeTrialQuota(), "an absurd setting is clamped to the cap")
	})
}

func TestHwGrantFreeCreditIfPhoneStepSkipped(t *testing.T) {
	hwPhoneStepTestSetup(t)

	t.Run("production refuses the shortcut", func(t *testing.T) {
		t.Setenv("APP_ENV", "production")
		t.Setenv("HW_SKIP_PHONE_STEP", "1")
		user := hwPhoneStepTestUser(t, "prod@example.com")
		granted, reason, err := HwGrantFreeCreditIfPhoneStepSkipped(user.Id)
		require.NoError(t, err)
		assert.False(t, granted)
		assert.Equal(t, "phone_step_required", reason)
		var signals []model.HwUserSignal
		require.NoError(t, model.DB.Where("user_id = ?", user.Id).Find(&signals).Error)
		assert.Empty(t, signals, "production never writes a dev phone signal")
		assert.Empty(t, hwPhoneStepFreePackages(t, user.Id))
	})

	t.Run("outside production the dev step grants once per user", func(t *testing.T) {
		t.Setenv("APP_ENV", "development")
		t.Setenv("HW_SKIP_PHONE_STEP", "1")
		user := hwPhoneStepTestUser(t, "dev@example.com")
		granted, reason, err := HwGrantFreeCreditIfPhoneStepSkipped(user.Id)
		require.NoError(t, err)
		assert.True(t, granted, "the smoke-test path grants (reason %q)", reason)
		granted, reason, err = HwGrantFreeCreditIfPhoneStepSkipped(user.Id)
		require.NoError(t, err)
		assert.False(t, granted)
		assert.Equal(t, "already_granted", reason)
	})

	t.Run("without the flag the grant waits for the phone step", func(t *testing.T) {
		t.Setenv("APP_ENV", "development")
		t.Setenv("HW_SKIP_PHONE_STEP", "")
		user := hwPhoneStepTestUser(t, "nostep@example.com")
		granted, reason, err := HwGrantFreeCreditIfPhoneStepSkipped(user.Id)
		require.NoError(t, err)
		assert.False(t, granted)
		assert.Equal(t, "phone_step_required", reason)
		assert.Empty(t, hwPhoneStepFreePackages(t, user.Id))
	})

	t.Run("the same device token on two accounts refuses the second", func(t *testing.T) {
		t.Setenv("APP_ENV", "development")
		t.Setenv("HW_SKIP_PHONE_STEP", "1")
		deviceToken := strings.Repeat("ab", 32)
		first := hwPhoneStepTestUser(t, "device-a@example.com")
		require.NoError(t, model.UpsertHwUserSignals(first.Id, model.HwUserSignals{DeviceToken: deviceToken}))
		granted, reason, err := HwGrantFreeCreditIfPhoneStepSkipped(first.Id)
		require.NoError(t, err)
		assert.True(t, granted, "the first account with this device gets the credit (reason %q)", reason)

		second := hwPhoneStepTestUser(t, "device-b@example.com")
		require.NoError(t, model.UpsertHwUserSignals(second.Id, model.HwUserSignals{DeviceToken: deviceToken}))
		granted, reason, err = HwGrantFreeCreditIfPhoneStepSkipped(second.Id)
		require.NoError(t, err)
		assert.False(t, granted, "a second account from the same device is refused")
		assert.Equal(t, "shared_device", reason)
		assert.Empty(t, hwPhoneStepFreePackages(t, second.Id))
	})
}

func TestHwNormalizeDeviceToken(t *testing.T) {
	t.Run("hex is accepted and lowercased", func(t *testing.T) {
		token, err := HwNormalizeDeviceToken("ABCDEF0123456789")
		require.NoError(t, err)
		assert.Equal(t, "abcdef0123456789", token)
	})
	t.Run("base64 is accepted as given", func(t *testing.T) {
		token, err := HwNormalizeDeviceToken("c2lnbmFsLXRva2VuLTEyMw==")
		require.NoError(t, err)
		assert.Equal(t, "c2lnbmFsLXRva2VuLTEyMw==", token)
	})
	t.Run("too short, too long and garbage are refused", func(t *testing.T) {
		for _, header := range []string{"short", strings.Repeat("a", 129), "not hex or base64!!", "zzzz=="} {
			_, err := HwNormalizeDeviceToken(header)
			assert.ErrorIs(t, err, ErrHwInvalidDeviceToken, "header of length %d", len(header))
		}
	})
	t.Run("an absent header is no signal and no error", func(t *testing.T) {
		token, err := HwNormalizeDeviceToken("  ")
		require.NoError(t, err)
		assert.Empty(t, token)
	})
}

// TestHwAbusePathsKeepSignalValuesOutOfLogs runs the abuse paths that log and
// checks the whole log buffer: a device token, an address, a Telegram identity,
// an email address and every keyed hash stay out of it, while the paths still
// produce their count and user-id lines.
func TestHwAbusePathsKeepSignalValuesOutOfLogs(t *testing.T) {
	hwPhoneStepTestSetup(t)
	var buffer bytes.Buffer
	originalOut, originalErr := gin.DefaultWriter, gin.DefaultErrorWriter
	gin.DefaultWriter, gin.DefaultErrorWriter = &buffer, &buffer
	t.Cleanup(func() {
		gin.DefaultWriter, gin.DefaultErrorWriter = originalOut, originalErr
	})

	const (
		deviceToken = "abcdef0123456789abcdef0123456789"
		clientIp    = "203.0.113.77"
		telegramID  = "telegram-identity-778899"
		firstEmail  = "hmsignal@example.com"
		secondEmail = "hmsignal2@example.com"
	)
	now := common.GetTimestamp()

	first := hwPhoneStepTestUser(t, firstEmail)
	require.NoError(t, model.UpsertHwUserSignals(first.Id, model.HwUserSignals{DeviceToken: deviceToken, Ip: clientIp}))
	granted, reason, err := HwCompleteTelegramPhoneStep(first.Id, telegramID)
	require.NoError(t, err)
	require.True(t, granted, "first account gets the credit, reason %q", reason)

	second := hwPhoneStepTestUser(t, secondEmail)
	require.NoError(t, model.UpsertHwUserSignals(second.Id, model.HwUserSignals{DeviceToken: deviceToken, Ip: clientIp}))
	granted, _, err = HwCompleteTelegramPhoneStep(second.Id, telegramID)
	require.NoError(t, err)
	assert.False(t, granted, "the second account on the same Telegram identity is refused")

	_, err = model.HwDailyAbuseSweep(now)
	require.NoError(t, err)
	for range model.HwSignupIpDailyLimit + 1 {
		_, err = model.HwReserveSignupSlot(clientIp, now, model.HwSignupIpDailyLimit)
		require.NoError(t, err)
	}

	logs := buffer.String()
	require.NotEmpty(t, logs, "the abuse paths do write their outcome")
	for _, raw := range []string{deviceToken, clientIp, telegramID, firstEmail, secondEmail} {
		assert.NotContains(t, logs, raw)
	}
	assert.NotRegexp(t, regexp.MustCompile(`[0-9a-f]{64}`), logs, "no keyed hash reaches a log line")
}
