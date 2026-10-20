package service

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
		&model.HwTelegramUpdate{}, &model.HwCreditExtension{},
	))
	for _, table := range []string{
		"hw_credit_packages", "hw_user_signals", "hw_first_purchase_claims",
		"hw_free_credit_claims", "hw_account_flags", "hw_signup_ips",
		"hw_telegram_updates", "users",
	} {
		require.NoError(t, model.DB.Exec("DELETE FROM "+table).Error)
	}
	t.Cleanup(func() {
		for _, table := range []string{
			"hw_credit_packages", "hw_user_signals", "hw_first_purchase_claims",
			"hw_free_credit_claims", "hw_account_flags", "hw_signup_ips",
			"hw_telegram_updates", "users",
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

// hwSetTryPerUsdOption publishes (or clears) the daily rate the free trial and
// every lira amount of the site are derived from.
func hwSetTryPerUsdOption(t *testing.T, value string) {
	t.Helper()
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	previous, hadPrevious := common.OptionMap[model.HwTryPerUsdOption]
	common.OptionMap[model.HwTryPerUsdOption] = value
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		if hadPrevious {
			common.OptionMap[model.HwTryPerUsdOption] = previous
			return
		}
		delete(common.OptionMap, model.HwTryPerUsdOption)
	})
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

// hwPhoneStepTestTelegramUser creates a user whose account is linked to the
// given Telegram identity, as the Telegram login leaves it.
func hwPhoneStepTestTelegramUser(t *testing.T, email, telegramID string) model.User {
	t.Helper()
	user := hwPhoneStepTestUser(t, email)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", user.Id).Update("telegram_id", telegramID).Error)
	user.TelegramId = telegramID
	return user
}

// hwPhoneStepSignals returns the signals of a user of one kind.
func hwPhoneStepSignals(t *testing.T, userID int, kind string) []model.HwUserSignal {
	t.Helper()
	var signals []model.HwUserSignal
	require.NoError(t, model.DB.Where("user_id = ? AND kind = ?", userID, kind).Find(&signals).Error)
	return signals
}

func TestHwCompleteTelegramPhoneStep(t *testing.T) {
	hwPhoneStepTestSetup(t)
	// No bot token: the prompt after the login fails silently, as it does for a
	// customer who has not started the bot.
	t.Setenv(hwTelegramBotTokenEnv, "")

	t.Run("a telegram identity alone earns nothing", func(t *testing.T) {
		user := hwPhoneStepTestUser(t, "only-telegram@example.com")
		granted, reason, err := HwCompleteTelegramPhoneStep(user.Id, "12345678")
		require.NoError(t, err)
		assert.False(t, granted, "the login widget proves an account, not a phone number")
		assert.Equal(t, "unverified", reason)
		assert.Empty(t, hwPhoneStepFreePackages(t, user.Id))
		assert.Equal(t, 0, hwPhoneStepUserQuota(t, user.Id))
		assert.Len(t, hwPhoneStepSignals(t, user.Id, "telegram"), 1)
		assert.Empty(t, hwPhoneStepSignals(t, user.Id, "phone"), "the identity is not a phone signal")
	})

	t.Run("after the number was shared the credit follows once", func(t *testing.T) {
		first := hwPhoneStepTestUser(t, "first@example.com")
		require.NoError(t, model.HwRecordPhoneSignal(first.Id, "0555 000 11 22"))
		granted, reason, err := HwCompleteTelegramPhoneStep(first.Id, "12345678")
		require.NoError(t, err)
		assert.True(t, granted, "the account with a shared number gets the credit (reason %q)", reason)
		assert.Equal(t, 1, len(hwPhoneStepFreePackages(t, first.Id)))
		assert.Greater(t, hwPhoneStepUserQuota(t, first.Id), 0)
		var reloaded model.User
		require.NoError(t, model.DB.Where("id = ?", first.Id).First(&reloaded).Error)
		assert.Equal(t, "free", reloaded.Group, "the granted account moves to the free group")

		// A second account that shares the same number earns nothing, whatever
		// its Telegram identity is.
		second := hwPhoneStepTestUser(t, "second@example.com")
		require.NoError(t, model.HwRecordPhoneSignal(second.Id, "+90 555 000 11 22"))
		granted, reason, err = HwCompleteTelegramPhoneStep(second.Id, "87654321")
		require.NoError(t, err)
		assert.False(t, granted)
		assert.Equal(t, "shared_phone", reason)
		assert.Empty(t, hwPhoneStepFreePackages(t, second.Id))

		// A different number still gets its own credit.
		third := hwPhoneStepTestUser(t, "third@example.com")
		require.NoError(t, model.HwRecordPhoneSignal(third.Id, "0555 000 33 44"))
		granted, _, err = HwCompleteTelegramPhoneStep(third.Id, "11122233")
		require.NoError(t, err)
		assert.True(t, granted)
	})

	t.Run("without a confirmed email nothing is granted", func(t *testing.T) {
		user := hwPhoneStepTestUser(t, "")
		require.NoError(t, model.HwRecordPhoneSignal(user.Id, "0555 000 55 66"))
		granted, reason, err := HwCompleteTelegramPhoneStep(user.Id, "12121212")
		require.NoError(t, err)
		assert.False(t, granted)
		assert.Equal(t, "email_unverified", reason)
		assert.Empty(t, hwPhoneStepFreePackages(t, user.Id))
		// The steps themselves are recorded: the signals exist even without a
		// grant, so a later confirmed email can still claim once.
		assert.Len(t, hwPhoneStepSignals(t, user.Id, "phone"), 1)
		assert.Len(t, hwPhoneStepSignals(t, user.Id, "telegram"), 1)
	})

	t.Run("a frozen account gets nothing", func(t *testing.T) {
		user := hwPhoneStepTestUser(t, "frozen@example.com")
		require.NoError(t, model.HwRecordPhoneSignal(user.Id, "0555 000 77 88"))
		frozen, err := model.FreezeHwAccounts([]int{user.Id}, model.HwFreezeReasonSharedDevice, common.GetTimestamp())
		require.NoError(t, err)
		require.Equal(t, 1, frozen)
		granted, reason, err := HwCompleteTelegramPhoneStep(user.Id, "33344455")
		require.NoError(t, err)
		assert.False(t, granted)
		assert.Equal(t, "frozen", reason)
		assert.Empty(t, hwPhoneStepFreePackages(t, user.Id))
	})

	t.Run("the stored telegram signal is a keyed hash, never the identity", func(t *testing.T) {
		user := hwPhoneStepTestUser(t, "hash@example.com")
		_, _, err := HwCompleteTelegramPhoneStep(user.Id, "99887766")
		require.NoError(t, err)
		signals := hwPhoneStepSignals(t, user.Id, "telegram")
		require.Len(t, signals, 1)
		assert.Len(t, signals[0].Hash, 64)
		assert.NotContains(t, signals[0].Hash, "99887766")
		var claims []model.HwFreeCreditClaim
		require.NoError(t, model.DB.Where("user_id = ?", user.Id).Find(&claims).Error)
		assert.Empty(t, claims, "an identity alone reserves no phone claim")
	})

	t.Run("the free amount is 100 lira at the daily rate and part A's cap holds", func(t *testing.T) {
		hwSetTryPerUsdOption(t, "40")
		assert.Equal(t, common.QuotaFromFloat(2.5*float64(common.QuotaPerUnit)), HwFreeTrialQuota(),
			"100 TL at 40 TL/USD is 2.5 dollars of credit")

		hwSetTryPerUsdOption(t, "")
		assert.Equal(t, common.QuotaFromFloat(3*float64(common.QuotaPerUnit)), HwFreeTrialQuota(),
			"without a rate the dollar fallback applies")

		t.Setenv("HW_FREE_TRIAL_QUOTA", "2")
		assert.Equal(t, common.QuotaFromFloat(2*float64(common.QuotaPerUnit)), HwFreeTrialQuota(),
			"the environment override is the fallback amount")
		t.Setenv("HW_FREE_TRIAL_QUOTA", "9999")
		assert.Equal(t, model.HwMaxFreeTrialQuota(), HwFreeTrialQuota(), "an absurd fallback is clamped to the cap")

		hwSetTryPerUsdOption(t, "40")
		assert.Equal(t, common.QuotaFromFloat(2.5*float64(common.QuotaPerUnit)), HwFreeTrialQuota(),
			"a published rate governs the amount; the fallback is not used")

		hwSetTryPerUsdOption(t, "0.5")
		assert.Equal(t, model.HwMaxFreeTrialQuota(), HwFreeTrialQuota(),
			"a rate that turns 100 TL into more than the cap stops at the cap")
	})
}

// A Telegram login must not wait for the bot: a chat that never answers costs
// the customer the whole delivery timeout. The prompt goes out in the
// background, and only while the account has no number yet.
func TestHwCompleteTelegramPhoneStepDoesNotWaitForThePrompt(t *testing.T) {
	t.Run("the login returns while the bot is still silent", func(t *testing.T) {
		hwPhoneStepTestSetup(t)
		started := make(chan struct{})
		release := make(chan struct{})
		var releaseOnce, startedOnce sync.Once
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			startedOnce.Do(func() { close(started) })
			<-release
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		t.Cleanup(func() {
			releaseOnce.Do(func() { close(release) })
			server.Close()
		})
		t.Setenv(hwTelegramAPIBaseEnv, server.URL)
		t.Setenv(hwTelegramBotTokenEnv, "test-bot-token")
		user := hwPhoneStepTestUser(t, "no-wait@example.com")

		returned := make(chan struct{})
		go func() {
			defer close(returned)
			_, _, _ = HwCompleteTelegramPhoneStep(user.Id, "800001")
		}()
		select {
		case <-returned:
		case <-time.After(3 * time.Second):
			t.Fatal("the Telegram login waited for the bot answer")
		}
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("the login did not ask the bot for the share-contact button")
		}
		releaseOnce.Do(func() { close(release) })
	})

	t.Run("an account that already shared its number is not asked again", func(t *testing.T) {
		hwPhoneStepTestSetup(t)
		var sends atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sends.Add(1)
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		t.Cleanup(server.Close)
		t.Setenv(hwTelegramAPIBaseEnv, server.URL)
		t.Setenv(hwTelegramBotTokenEnv, "test-bot-token")
		user := hwPhoneStepTestUser(t, "already-shared@example.com")
		require.NoError(t, model.HwRecordPhoneSignal(user.Id, "0555 987 65 43"))

		_, _, err := HwCompleteTelegramPhoneStep(user.Id, "800002")
		require.NoError(t, err)
		// A prompt that should not be sent would reach the double within
		// milliseconds; the window is generous so it cannot hide a send.
		time.Sleep(200 * time.Millisecond)
		assert.Zero(t, sends.Load(), "a number the account already has needs no second prompt")
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

	t.Run("a copied device token refuses no second account", func(t *testing.T) {
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
		assert.True(t, granted, "the browser chooses the token, so a copy refuses nothing (reason %q)", reason)
		assert.Len(t, hwPhoneStepFreePackages(t, second.Id), 1)
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
		sharedPhone = "+90 555 000 99 88"
		firstEmail  = "hmsignal@example.com"
		secondEmail = "hmsignal2@example.com"
	)
	t.Setenv(hwTelegramBotTokenEnv, "")
	now := common.GetTimestamp()

	first := hwPhoneStepTestUser(t, firstEmail)
	require.NoError(t, model.UpsertHwUserSignals(first.Id, model.HwUserSignals{DeviceToken: deviceToken, Ip: clientIp}))
	require.NoError(t, model.HwRecordPhoneSignal(first.Id, sharedPhone))
	granted, reason, err := HwCompleteTelegramPhoneStep(first.Id, telegramID)
	require.NoError(t, err)
	require.True(t, granted, "first account gets the credit, reason %q", reason)

	second := hwPhoneStepTestUser(t, secondEmail)
	require.NoError(t, model.UpsertHwUserSignals(second.Id, model.HwUserSignals{DeviceToken: deviceToken, Ip: clientIp}))
	require.NoError(t, model.HwRecordPhoneSignal(second.Id, sharedPhone))
	granted, _, err = HwCompleteTelegramPhoneStep(second.Id, "telegram-identity-990011")
	require.NoError(t, err)
	assert.False(t, granted, "the second account on the same phone number is refused")

	_, err = model.HwDailyAbuseSweep(now)
	require.NoError(t, err)
	for range model.HwSignupIpDailyLimit + 1 {
		_, err = model.HwReserveSignupSlot(clientIp, now, model.HwSignupIpDailyLimit)
		require.NoError(t, err)
	}

	logs := buffer.String()
	require.NotEmpty(t, logs, "the abuse paths do write their outcome")
	for _, raw := range []string{deviceToken, clientIp, telegramID, sharedPhone, "+905550009988", "5550009988", firstEmail, secondEmail} {
		assert.NotContains(t, logs, raw)
	}
	assert.NotRegexp(t, regexp.MustCompile(`[0-9a-f]{64}`), logs, "no keyed hash reaches a log line")
}

// hwTelegramSent is one message the fake bot API received.
type hwTelegramSent struct {
	ChatId int64
	Text   string
	// Button is the label of the share-contact button, empty when the message
	// carried no keyboard.
	Button string
}

// hwFakeTelegramAPI stands in for the bot API. It records what the service
// sends, so a test can read the answer a customer would get.
type hwFakeTelegramAPI struct {
	mu   sync.Mutex
	sent []hwTelegramSent
}

func (f *hwFakeTelegramAPI) record(sent hwTelegramSent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, sent)
}

func (f *hwFakeTelegramAPI) messages() []hwTelegramSent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]hwTelegramSent(nil), f.sent...)
}

// hwTelegramTestSetup starts a fake bot API and configures the environment of
// the webhook. The token and the secret are clearly fake values.
func hwTelegramTestSetup(t *testing.T) (*hwFakeTelegramAPI, string) {
	t.Helper()
	hwPhoneStepTestSetup(t)
	api := &hwFakeTelegramAPI{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body hwTelegramMessageRequest
		if err := common.DecodeJson(r.Body, &body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		sent := hwTelegramSent{ChatId: body.ChatId, Text: body.Text}
		if markup := body.ReplyMarkup; markup != nil && len(markup.Keyboard) == 1 && len(markup.Keyboard[0]) == 1 {
			sent.Button = markup.Keyboard[0][0].Text
		}
		api.record(sent)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)
	const secret = "test-webhook-secret"
	t.Setenv(hwTelegramAPIBaseEnv, server.URL)
	t.Setenv(hwTelegramBotTokenEnv, "test-bot-token")
	t.Setenv(hwTelegramWebhookSecretEnv, secret)
	return api, secret
}

func hwInt64Ptr(value int64) *int64 {
	return &value
}

// hwTelegramUpdateBody renders one bot update around the given message.
func hwTelegramUpdateBody(t *testing.T, updateID int64, message map[string]any) []byte {
	t.Helper()
	body, err := common.Marshal(map[string]any{"update_id": updateID, "message": message})
	require.NoError(t, err)
	return body
}

// hwTelegramContactUpdate renders a message that shares a contact. A nil
// contactUserID omits the field, as a contact without a Telegram user does.
func hwTelegramContactUpdate(t *testing.T, updateID, fromID int64, languageCode string, contactUserID *int64, phone string) []byte {
	t.Helper()
	contact := map[string]any{"phone_number": phone}
	if contactUserID != nil {
		contact["user_id"] = *contactUserID
	}
	return hwTelegramUpdateBody(t, updateID, map[string]any{
		"message_id": updateID,
		"from":       map[string]any{"id": fromID, "language_code": languageCode},
		"chat":       map[string]any{"id": fromID},
		"contact":    contact,
	})
}

// hwTelegramTextUpdate renders a text message, as the /start that opens the
// bot chat.
func hwTelegramTextUpdate(t *testing.T, updateID, fromID int64, languageCode, text string) []byte {
	t.Helper()
	return hwTelegramUpdateBody(t, updateID, map[string]any{
		"message_id": updateID,
		"from":       map[string]any{"id": fromID, "language_code": languageCode},
		"chat":       map[string]any{"id": fromID},
		"text":       text,
	})
}

func TestHwTelegramWebhookPhoneStep(t *testing.T) {
	t.Run("the sender's own number is recorded and earns the credit", func(t *testing.T) {
		api, secret := hwTelegramTestSetup(t)
		user := hwPhoneStepTestTelegramUser(t, "own-number@example.com", "700001")
		body := hwTelegramContactUpdate(t, 1, 700001, "tr", hwInt64Ptr(700001), "+90 555 000 12 34")

		outcome := HwHandleTelegramWebhook(context.Background(), body, secret, time.Now())
		assert.Equal(t, http.StatusOK, outcome.HTTPStatus())
		assert.Len(t, hwPhoneStepSignals(t, user.Id, "phone"), 1)
		assert.Len(t, hwPhoneStepFreePackages(t, user.Id), 1)
		assert.Greater(t, hwPhoneStepUserQuota(t, user.Id), 0)

		sent := api.messages()
		require.Len(t, sent, 1)
		assert.Equal(t, int64(700001), sent[0].ChatId)
		assert.Equal(t, hwTelegramTextsFor("tr").Accepted, sent[0].Text)
		assert.NotContains(t, sent[0].Text, "555")
	})

	t.Run("a contact of someone else is refused", func(t *testing.T) {
		api, secret := hwTelegramTestSetup(t)
		user := hwPhoneStepTestTelegramUser(t, "forwarded@example.com", "700002")
		body := hwTelegramContactUpdate(t, 2, 700002, "en", hwInt64Ptr(999999), "+90 555 000 22 33")

		outcome := HwHandleTelegramWebhook(context.Background(), body, secret, time.Now())
		assert.Equal(t, http.StatusOK, outcome.HTTPStatus())
		assert.Empty(t, hwPhoneStepSignals(t, user.Id, "phone"))
		assert.Empty(t, hwPhoneStepFreePackages(t, user.Id))
		sent := api.messages()
		require.Len(t, sent, 1)
		assert.Equal(t, hwTelegramTextsFor("en").Rejected, sent[0].Text)
	})

	t.Run("a contact without a Telegram user id is refused", func(t *testing.T) {
		_, secret := hwTelegramTestSetup(t)
		user := hwPhoneStepTestTelegramUser(t, "no-user-id@example.com", "700003")
		body := hwTelegramContactUpdate(t, 3, 700003, "tr", nil, "+90 555 000 33 44")

		outcome := HwHandleTelegramWebhook(context.Background(), body, secret, time.Now())
		assert.Equal(t, http.StatusOK, outcome.HTTPStatus())
		assert.Empty(t, hwPhoneStepSignals(t, user.Id, "phone"))
		assert.Empty(t, hwPhoneStepFreePackages(t, user.Id))
	})

	t.Run("a wrong or missing secret is refused without processing", func(t *testing.T) {
		for _, testCase := range []struct {
			name   string
			header string
		}{
			{"wrong secret", "not-the-secret"},
			{"missing secret", ""},
		} {
			t.Run(testCase.name, func(t *testing.T) {
				api, secret := hwTelegramTestSetup(t)
				user := hwPhoneStepTestTelegramUser(t, "forged@example.com", "700004")
				body := hwTelegramContactUpdate(t, 4, 700004, "tr", hwInt64Ptr(700004), "+90 555 000 44 55")

				outcome := HwHandleTelegramWebhook(context.Background(), body, testCase.header, time.Now())
				assert.Equal(t, http.StatusUnauthorized, outcome.HTTPStatus())
				assert.NotEmpty(t, secret)
				assert.Empty(t, hwPhoneStepSignals(t, user.Id, "phone"))
				assert.Empty(t, hwPhoneStepFreePackages(t, user.Id))
				assert.Empty(t, api.messages())
				var claimed int64
				require.NoError(t, model.DB.Model(&model.HwTelegramUpdate{}).Count(&claimed).Error)
				assert.Zero(t, claimed, "a refused update is not claimed either")
			})
		}
	})

	t.Run("without a configured secret every update is refused", func(t *testing.T) {
		_, secret := hwTelegramTestSetup(t)
		user := hwPhoneStepTestTelegramUser(t, "unconfigured@example.com", "700005")
		t.Setenv(hwTelegramWebhookSecretEnv, "")
		body := hwTelegramContactUpdate(t, 5, 700005, "tr", hwInt64Ptr(700005), "+90 555 000 55 66")

		outcome := HwHandleTelegramWebhook(context.Background(), body, secret, time.Now())
		assert.Equal(t, http.StatusServiceUnavailable, outcome.HTTPStatus())
		assert.Empty(t, hwPhoneStepSignals(t, user.Id, "phone"))
	})

	t.Run("a second account with the same number earns nothing", func(t *testing.T) {
		api, secret := hwTelegramTestSetup(t)
		first := hwPhoneStepTestTelegramUser(t, "number-first@example.com", "700006")
		second := hwPhoneStepTestTelegramUser(t, "number-second@example.com", "700007")

		firstOutcome := HwHandleTelegramWebhook(context.Background(), hwTelegramContactUpdate(t, 6, 700006, "tr", hwInt64Ptr(700006), "0555 000 66 77"), secret, time.Now())
		require.Equal(t, http.StatusOK, firstOutcome.HTTPStatus())
		outcome := HwHandleTelegramWebhook(context.Background(), hwTelegramContactUpdate(t, 7, 700007, "tr", hwInt64Ptr(700007), "+90 555 000 66 77"), secret, time.Now())
		assert.Equal(t, http.StatusOK, outcome.HTTPStatus())

		assert.Len(t, hwPhoneStepFreePackages(t, first.Id), 1)
		assert.Empty(t, hwPhoneStepFreePackages(t, second.Id), "the second account on one number earns nothing")
		assert.Len(t, hwPhoneStepSignals(t, second.Id, "phone"), 1, "the number is still recorded as a signal")
		sent := api.messages()
		require.Len(t, sent, 2)
		assert.Equal(t, hwTelegramTextsFor("tr").Rejected, sent[1].Text)
	})

	t.Run("a Telegram account that is linked to no user earns nothing", func(t *testing.T) {
		api, secret := hwTelegramTestSetup(t)
		body := hwTelegramContactUpdate(t, 8, 700008, "tr", hwInt64Ptr(700008), "+90 555 000 88 99")

		outcome := HwHandleTelegramWebhook(context.Background(), body, secret, time.Now())
		assert.Equal(t, http.StatusOK, outcome.HTTPStatus())
		var signals int64
		require.NoError(t, model.DB.Model(&model.HwUserSignal{}).Count(&signals).Error)
		assert.Zero(t, signals)
		var packages int64
		require.NoError(t, model.DB.Model(&model.HwCreditPackage{}).Count(&packages).Error)
		assert.Zero(t, packages)
		assert.Empty(t, api.messages())
	})

	t.Run("an invalid or foreign number is refused", func(t *testing.T) {
		for _, phone := range []string{"12345", "+1 555 123 4567", "+90 555 000 11 2a"} {
			t.Run(phone, func(t *testing.T) {
				api, secret := hwTelegramTestSetup(t)
				user := hwPhoneStepTestTelegramUser(t, "bad-number@example.com", "700009")
				body := hwTelegramContactUpdate(t, 9, 700009, "en", hwInt64Ptr(700009), phone)

				outcome := HwHandleTelegramWebhook(context.Background(), body, secret, time.Now())
				assert.Equal(t, http.StatusOK, outcome.HTTPStatus())
				assert.Empty(t, hwPhoneStepSignals(t, user.Id, "phone"))
				assert.Empty(t, hwPhoneStepFreePackages(t, user.Id))
				sent := api.messages()
				require.Len(t, sent, 1)
				assert.Equal(t, hwTelegramTextsFor("en").Rejected, sent[0].Text)
			})
		}
	})

	t.Run("a repeated update id is handled once", func(t *testing.T) {
		api, secret := hwTelegramTestSetup(t)
		user := hwPhoneStepTestTelegramUser(t, "repeat@example.com", "700010")
		body := hwTelegramContactUpdate(t, 10, 700010, "tr", hwInt64Ptr(700010), "+90 555 000 10 10")

		first := HwHandleTelegramWebhook(context.Background(), body, secret, time.Now())
		require.Equal(t, http.StatusOK, first.HTTPStatus())
		outcome := HwHandleTelegramWebhook(context.Background(), body, secret, time.Now())
		assert.Equal(t, http.StatusOK, outcome.HTTPStatus(), "a repeated delivery is acknowledged")

		assert.Len(t, hwPhoneStepSignals(t, user.Id, "phone"), 1)
		assert.Len(t, hwPhoneStepFreePackages(t, user.Id), 1)
		assert.Len(t, api.messages(), 1, "the repeated delivery sends no second answer")
		var claimed int64
		require.NoError(t, model.DB.Model(&model.HwTelegramUpdate{}).Count(&claimed).Error)
		assert.Equal(t, int64(1), claimed)
	})

	t.Run("a start command answers with the share-contact button", func(t *testing.T) {
		api, secret := hwTelegramTestSetup(t)
		hwPhoneStepTestTelegramUser(t, "start@example.com", "700011")
		body := hwTelegramTextUpdate(t, 11, 700011, "tr", "/start")

		outcome := HwHandleTelegramWebhook(context.Background(), body, secret, time.Now())
		assert.Equal(t, http.StatusOK, outcome.HTTPStatus())
		sent := api.messages()
		require.Len(t, sent, 1)
		assert.Equal(t, hwTelegramTextsFor("tr").Prompt, sent[0].Text)
		assert.Equal(t, hwTelegramTextsFor("tr").Button, sent[0].Button, "the prompt carries the share-contact button")
	})

	t.Run("the number stays out of the logs and the answers", func(t *testing.T) {
		hwTelegramTestSetup(t)
		var buffer bytes.Buffer
		originalOut, originalErr := gin.DefaultWriter, gin.DefaultErrorWriter
		gin.DefaultWriter, gin.DefaultErrorWriter = &buffer, &buffer
		t.Cleanup(func() {
			gin.DefaultWriter, gin.DefaultErrorWriter = originalOut, originalErr
		})
		hwPhoneStepTestTelegramUser(t, "quiet@example.com", "700012")
		body := hwTelegramContactUpdate(t, 12, 700012, "tr", hwInt64Ptr(700012), "+90 555 000 12 12")

		outcome := HwHandleTelegramWebhook(context.Background(), body, "test-webhook-secret", time.Now())
		assert.Equal(t, http.StatusOK, outcome.HTTPStatus())

		logs := buffer.String()
		assert.NotContains(t, logs, "5550001212")
		assert.NotContains(t, logs, "+905550001212")
		assert.NotRegexp(t, regexp.MustCompile(`[0-9a-f]{64}`), logs, "no keyed hash reaches a log line")
	})
}
