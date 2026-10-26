package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"gorm.io/gorm"
)

// The Telegram bot is the phone step of the free trial credit. The login widget
// proves a Telegram account, not a phone number: it returns the account id, the
// name, the user name, the photo and the signature, and a person can retire an
// account and start another one with the same SIM. The bot's share-contact
// button does give the number itself, and Telegram confirms on the contact that
// it belongs to the sender (contact.user_id == message.from.id), so that number
// is the one signal this fork trusts.
const (
	hwTelegramBotTokenEnv      = "TELEGRAM_BOT_TOKEN"
	hwTelegramWebhookSecretEnv = "TELEGRAM_WEBHOOK_SECRET"
	hwTelegramAPIBaseEnv       = "HW_TELEGRAM_API_BASE"
	hwTelegramDefaultAPIBase   = "https://api.telegram.org"
	hwTelegramRequestTimeout   = 10 * time.Second
	hwTelegramMaxBodyBytes     = 1 << 20
	// hwTelegramWebhookPath is the webhook route under the server address.
	hwTelegramWebhookPath = "/api/hw/telegram/webhook"
	// hwTelegramWebhookSecretLabel derives the webhook secret from the bot
	// token when TELEGRAM_WEBHOOK_SECRET is not set.
	hwTelegramWebhookSecretLabel = "hisarweb-telegram-webhook"
)

// HwTelegramSecretHeader is the header Telegram sends the registered webhook
// secret in (setWebhook's secret_token).
const HwTelegramSecretHeader = "X-Telegram-Bot-Api-Secret-Token"

var (
	// ErrHwTelegramUnavailable says the bot token is not configured, so nothing
	// can be sent. It is not an error of the update being handled.
	ErrHwTelegramUnavailable = errors.New("telegram bot token is not configured")
	// ErrHwTelegramDeliveryFailed says a message could not be delivered. The
	// cause is deliberately not propagated: the request URL carries the bot
	// token, and an error text that quotes the URL would leak it into a log.
	ErrHwTelegramDeliveryFailed = errors.New("telegram message delivery failed")
	// ErrHwTelegramWebhookAddress says the server address is not https, which
	// Telegram requires of a webhook.
	ErrHwTelegramWebhookAddress = errors.New("telegram webhook needs an https server address")
)

// hwTelegramClient delivers bot messages. It has a fixed timeout so a slow
// Telegram API can never hold a login or a webhook answer.
var hwTelegramClient = &http.Client{Timeout: hwTelegramRequestTimeout}

// hwTelegramTexts holds the prompt that asks for the number and the two answers
// of the contact step, in the language of the Telegram account. They name no
// supplier and never quote the number that was shared.
type hwTelegramTexts struct {
	Prompt   string
	Button   string
	Accepted string
	Rejected string
	// LinkInvalid, LinkTaken and LinkNeeded answer the one-time link that the
	// dashboard hands out, so a customer who opens the bot without a working
	// link is told what to do instead of being ignored.
	LinkInvalid string
	LinkTaken   string
	LinkNeeded  string
}

// hwTelegramTextsFor picks Turkish for a Turkish Telegram account and English
// for every other language.
func hwTelegramTextsFor(languageCode string) hwTelegramTexts {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(languageCode)), "tr") {
		return hwTelegramTexts{
			Prompt:      "Ücretsiz deneme krediniz için kendi telefon numaranızı paylaşın.",
			Button:      "Telefon numaramı paylaş",
			Accepted:    "Telefon numaranız kaydedildi.",
			Rejected:    "Bu telefon numarası kabul edilmiyor.",
			LinkInvalid: "Bu bağlantının süresi dolmuş veya zaten kullanılmış. Panelinizden yeni bir bağlantı isteyin.",
			LinkTaken:   "Bu Telegram hesabı zaten başka bir hesaba bağlı.",
			LinkNeeded:  "Panelinizi https://api.hisarweb.be adresinde açın ve Telegram'ı bağla düğmesine basın.",
		}
	}
	return hwTelegramTexts{
		Prompt:      "Share your own phone number to claim your free trial credit.",
		Button:      "Share my phone number",
		Accepted:    "Your phone number has been recorded.",
		Rejected:    "This phone number cannot be accepted.",
		LinkInvalid: "This link has expired or has already been used. Ask for a new one in your dashboard.",
		LinkTaken:   "This Telegram account is already linked to another account.",
		LinkNeeded:  "Open your dashboard at https://api.hisarweb.be and press the Link Telegram button.",
	}
}

// hwTelegramUpdate is the part of a bot update this webhook reads. The message
// body is never logged or stored.
type hwTelegramUpdate struct {
	UpdateId int64              `json:"update_id"`
	Message  *hwTelegramMessage `json:"message"`
}

// hwTelegramMessage is one message of the update.
type hwTelegramMessage struct {
	From *struct {
		Id           int64  `json:"id"`
		LanguageCode string `json:"language_code"`
	} `json:"from"`
	Chat struct {
		Id int64 `json:"id"`
	} `json:"chat"`
	Text    string `json:"text"`
	Contact *struct {
		PhoneNumber string `json:"phone_number"`
		UserId      *int64 `json:"user_id"`
	} `json:"contact"`
}

// HwHandleTelegramWebhook verifies and processes one bot update. Without a
// webhook secret every update is refused with 503: an unauthenticated
// caller could otherwise hand any account a phone number it does not own. The
// secret header is compared in constant time.
//
// Only a message that shares the sender's own number is processed: a forwarded
// contact carries another user_id or none, and the linked account of the sender
// must exist. The handler is idempotent on update_id, so a repeat of a finished
// delivery is acknowledged without doing the work twice, while a repeat of a
// delivery that is still running is answered with 503 so Telegram keeps trying.
func HwHandleTelegramWebhook(ctx context.Context, rawBody []byte, secretHeader string, now time.Time) HwWebhookOutcome {
	secret := hwTelegramWebhookSecret()
	if secret == "" {
		common.SysError(common.LogText("telegram webhook refused: neither %s nor a bot token is configured", hwTelegramWebhookSecretEnv))
		return HwWebhookUnavailable
	}
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(secretHeader)), []byte(secret)) != 1 {
		common.SysLog(common.LogText("telegram webhook rejected: the secret header did not match"))
		return HwWebhookRejected
	}
	var update hwTelegramUpdate
	if err := common.Unmarshal(rawBody, &update); err != nil {
		common.SysLog(common.LogText("telegram webhook rejected: the body is not an update"))
		return HwWebhookBadRequest
	}
	claim, err := model.HwClaimTelegramUpdate(update.UpdateId, now.Unix())
	if err != nil {
		if errors.Is(err, model.ErrHwInvalidTelegramUpdate) {
			common.SysLog(common.LogText("telegram webhook rejected: the update has no usable id"))
			return HwWebhookBadRequest
		}
		common.SysError(common.LogText("telegram webhook could not claim its update"))
		return HwWebhookUnavailable
	}
	switch claim {
	case model.HwTelegramUpdateFinished:
		// Telegram repeated a delivery we already handled.
		return HwWebhookAccepted
	case model.HwTelegramUpdateRunning:
		// Another delivery of this update is still working. Answering 200 would
		// stop Telegram from retrying, and that delivery may yet fail without
		// having written anything, so the repeat asks for a retry instead.
		common.SysLog(common.LogText("telegram webhook deferred a delivery that is still running"))
		return HwWebhookUnavailable
	}
	outcome := hwProcessTelegramUpdate(ctx, update, now.Unix())
	if outcome == HwWebhookUnavailable {
		// Give the claim back so the retry of a failed delivery can do the work.
		if err := model.HwReleaseTelegramUpdate(update.UpdateId, now.Unix()); err != nil {
			common.SysError(common.LogText("telegram webhook could not release its failed update"))
		}
		return outcome
	}
	if err := model.HwFinishTelegramUpdate(update.UpdateId, now.Unix(), now.Unix()); err != nil {
		// The work is done and must not run again: a claim left unfinished only
		// makes a later delivery of this update wait for its stale time first.
		common.SysError(common.LogText("telegram webhook could not finish its update"))
	}
	return outcome
}

// hwProcessTelegramUpdate handles one claimed update and answers the customer.
// Log lines carry user ids, outcomes and counts only: no link token, no phone
// number, no chat id and no message body.
func hwProcessTelegramUpdate(ctx context.Context, update hwTelegramUpdate, now int64) HwWebhookOutcome {
	message := update.Message
	if message == nil || message.From == nil {
		return HwWebhookIgnored
	}
	texts := hwTelegramTextsFor(message.From.LanguageCode)
	if payload, ok := hwTelegramLinkPayload(message.Text); ok {
		return hwLinkTelegramAccount(ctx, message, payload, texts, now)
	}
	// A question with a fixed answer is answered for anyone, without a model call.
	if hwTelegramFAQReply(ctx, message, now) {
		return HwWebhookAccepted
	}
	user, err := model.GetUserByTelegramID(strconv.FormatInt(message.From.Id, 10))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Someone who opens the bot without a link gets told where the link
		// lives; every other message of an unknown account stays ignored.
		if strings.HasPrefix(strings.TrimSpace(message.Text), "/start") {
			hwAnswerTelegramChat(ctx, strconv.FormatInt(message.Chat.Id, 10), texts.LinkNeeded, "")
			return HwWebhookAccepted
		}
		common.SysLog(common.LogText("telegram webhook ignored a message from a Telegram account that is linked to no user"))
		return HwWebhookIgnored
	}
	if err != nil {
		common.SysError(common.LogText("telegram webhook could not look up the linked user"))
		return HwWebhookUnavailable
	}
	if message.Contact == nil {
		// A bot may only write to a chat the customer opened, so the request for
		// the number answers the /start that opened it; pushing it after the
		// login would reach only customers who had opened the bot before.
		if strings.HasPrefix(strings.TrimSpace(message.Text), "/start") {
			hwAnswerTelegramChat(ctx, strconv.FormatInt(message.Chat.Id, 10), texts.Prompt, texts.Button)
			return HwWebhookAccepted
		}
		// A linked customer who asks something without a fixed answer is told so
		// instead of being left without a reply.
		if text := strings.TrimSpace(message.Text); text != "" && !strings.HasPrefix(text, "/") {
			hwAnswerTelegramChat(ctx, strconv.FormatInt(message.Chat.Id, 10), hwTelegramNoAnswer(hwTelegramAssistantLang(message.From.LanguageCode)), "")
			return HwWebhookAccepted
		}
		return HwWebhookIgnored
	}
	if message.Contact.UserId == nil || *message.Contact.UserId != message.From.Id {
		// A shared contact that is not the sender's own number proves nothing.
		common.SysLog(common.LogText("telegram webhook refused a contact that is not the sender's own"))
		hwAnswerTelegramChat(ctx, strconv.FormatInt(message.Chat.Id, 10), texts.Rejected, "")
		return HwWebhookAccepted
	}
	// Telegram shares the full international number, usually without the plus;
	// adding it lets a German or Belgian number keep its own country code. A
	// number with a trunk zero is national and stays as given.
	phone := strings.TrimSpace(message.Contact.PhoneNumber)
	if !strings.HasPrefix(phone, "+") && !strings.HasPrefix(phone, "0") {
		phone = "+" + phone
	}
	if err := model.HwRecordPhoneSignal(user.Id, phone); err != nil {
		if errors.Is(err, model.ErrHwInvalidPhoneNumber) {
			common.SysLog(common.LogText("telegram webhook refused an unaccepted phone number for user %d", user.Id))
			hwAnswerTelegramChat(ctx, strconv.FormatInt(message.Chat.Id, 10), texts.Rejected, "")
			return HwWebhookAccepted
		}
		common.SysError(common.LogText("telegram webhook could not store the phone step for user %d", user.Id))
		return HwWebhookUnavailable
	}
	granted, reason, err := hwMaybeGrantFreeTrialCredit(user)
	if err != nil {
		common.SysError(common.LogText("telegram webhook could not run the free trial credit for user %d", user.Id))
		return HwWebhookUnavailable
	}
	common.SysLog(common.LogText("telegram webhook recorded the phone step of user %d (granted %t, reason %s)", user.Id, granted, reason))
	// A number that already earned the credit elsewhere is recorded as a signal
	// but earns nothing here, and the customer is told so plainly.
	if reason == model.HwFreeTrialSharedSignal+model.HwSignalKindPhone {
		hwAnswerTelegramChat(ctx, strconv.FormatInt(message.Chat.Id, 10), texts.Rejected, "")
		return HwWebhookAccepted
	}
	hwAnswerTelegramChat(ctx, strconv.FormatInt(message.Chat.Id, 10), texts.Accepted, "")
	return HwWebhookAccepted
}

// hwTelegramLinkPayload reads the token out of "/start <token>", the message
// Telegram sends when a customer opens the bot through the dashboard link.
// A bare /start carries no payload and is answered elsewhere.
func hwTelegramLinkPayload(text string) (string, bool) {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) < 2 || fields[0] != "/start" {
		return "", false
	}
	return fields[1], true
}

// hwLinkTelegramAccount claims a link token and ties the sender's Telegram
// account to the user the token was made for, then asks for the number in the
// same chat so the second and third start-bonus step happen in one conversation.
//
// The token is single use, and only a linked account spends it: every way out
// that links nothing gives the token back before the webhook answers, so a
// temporary failure cannot cost the customer a link they were handed for a
// conversation that never happened. A retry of that failed delivery finds the
// same token, and the dashboard link works until it expires or is replaced.
func hwLinkTelegramAccount(ctx context.Context, message *hwTelegramMessage, payload string, texts hwTelegramTexts, now int64) HwWebhookOutcome {
	chatID := strconv.FormatInt(message.Chat.Id, 10)
	telegramID := strconv.FormatInt(message.From.Id, 10)
	userID, token, err := model.HwClaimTelegramLinkToken(payload, now)
	if err != nil {
		if errors.Is(err, model.ErrHwTelegramLinkInvalid) {
			// A customer who is already linked and opens the bot again through an
			// old dashboard link wants the share button, not a dead end.
			if _, lookupErr := model.GetUserByTelegramID(telegramID); lookupErr == nil {
				hwAnswerTelegramChat(ctx, chatID, texts.Prompt, texts.Button)
				return HwWebhookAccepted
			}
			common.SysLog(common.LogText("telegram webhook refused an unusable link token"))
			hwAnswerTelegramChat(ctx, chatID, texts.LinkInvalid, "")
			return HwWebhookAccepted
		}
		common.SysError(common.LogText("telegram webhook could not claim its link token"))
		return HwWebhookUnavailable
	}
	linked := false
	defer func() {
		if linked {
			return
		}
		if err := model.HwRestoreTelegramLinkToken(token, now); err != nil {
			common.SysError(common.LogText("telegram webhook could not give back its unused link token"))
		}
	}()
	// An account that already belongs to this user is linked: answer with the
	// prompt, not with an error.
	owner, lookupErr := model.GetUserByTelegramID(telegramID)
	switch {
	case lookupErr == nil && owner.Id == userID:
		linked = true
		hwAnswerTelegramChat(ctx, chatID, texts.Prompt, texts.Button)
		return HwWebhookAccepted
	case lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound):
		common.SysError(common.LogText("telegram webhook could not look up the linked user"))
		return HwWebhookUnavailable
	}
	if model.IsTelegramIdAlreadyTaken(telegramID) {
		common.SysLog(common.LogText("telegram webhook refused a link for a Telegram account that another user holds"))
		hwAnswerTelegramChat(ctx, chatID, texts.LinkTaken, "")
		return HwWebhookAccepted
	}
	target, err := model.GetUserById(userID, false)
	if err != nil {
		common.SysError(common.LogText("telegram webhook could not read the user of its link token"))
		return HwWebhookUnavailable
	}
	if strings.TrimSpace(target.TelegramId) != "" {
		common.SysLog(common.LogText("telegram webhook refused a link for user %d that already holds a Telegram account", userID))
		hwAnswerTelegramChat(ctx, chatID, texts.LinkTaken, "")
		return HwWebhookAccepted
	}
	if err := model.UpdateUserBindColumn(userID, "telegram_id", telegramID); err != nil {
		common.SysError(common.LogText("telegram webhook could not link a Telegram account to user %d", userID))
		return HwWebhookUnavailable
	}
	linked = true
	common.SysLog(common.LogText("telegram webhook linked a Telegram account to user %d", userID))
	hwAnswerTelegramChat(ctx, chatID, texts.Prompt, texts.Button)
	return HwWebhookAccepted
}

// hwAnswerTelegramChat sends one answer and a button when one is given. A
// delivery failure is logged as a class only and never fails the handled
// update: the phone signal and the credit are already written.
func hwAnswerTelegramChat(ctx context.Context, chatID string, text string, button string) {
	err := HwTelegramSendMessage(ctx, chatID, text, button)
	if err != nil && !errors.Is(err, ErrHwTelegramUnavailable) {
		common.SysError(common.LogText("telegram answer could not be delivered"))
	}
}

// HwTelegramSendMessage sends one plain text to a Telegram chat. A non-empty
// button label adds the keyboard that carries the share-contact button
// (request_contact), which makes Telegram attach the sender's own number to the
// answer. Neither the token nor the URL reaches a log line, an error text or
// an answer.
func HwTelegramSendMessage(ctx context.Context, chatID string, text string, button string) error {
	if hwTelegramBotToken() == "" {
		return ErrHwTelegramUnavailable
	}
	chat, err := strconv.ParseInt(strings.TrimSpace(chatID), 10, 64)
	if err != nil || strings.TrimSpace(text) == "" {
		return ErrHwTelegramDeliveryFailed
	}
	request := hwTelegramMessageRequest{ChatId: chat, Text: text}
	if button != "" {
		request.ReplyMarkup = &hwTelegramKeyboard{
			Keyboard:        [][]hwTelegramButton{{{Text: button, RequestContact: true}}},
			ResizeKeyboard:  true,
			OneTimeKeyboard: true,
		}
	}
	return hwTelegramCall(ctx, "sendMessage", request)
}

// hwTelegramBotToken is TELEGRAM_BOT_TOKEN, or else the bot token saved in the
// Telegram settings, so the bot works from one admin field as well.
func hwTelegramBotToken() string {
	if token := strings.TrimSpace(os.Getenv(hwTelegramBotTokenEnv)); token != "" {
		return token
	}
	return strings.TrimSpace(common.TelegramBotToken)
}

// HwTelegramBotConfigured says a bot token is set, so the dashboard link the
// customer receives will actually reach this webhook.
func HwTelegramBotConfigured() bool {
	return hwTelegramBotToken() != ""
}

// hwTelegramWebhookSecret is TELEGRAM_WEBHOOK_SECRET, or else a secret derived
// from the bot token. Only the token holder can derive it, and it changes with
// the token, so registering the webhook needs no second value to keep.
func hwTelegramWebhookSecret() string {
	if secret := strings.TrimSpace(os.Getenv(hwTelegramWebhookSecretEnv)); secret != "" {
		return secret
	}
	token := hwTelegramBotToken()
	if token == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write([]byte(hwTelegramWebhookSecretLabel))
	return hex.EncodeToString(mac.Sum(nil))
}

// HwRegisterTelegramWebhook points the bot at this server's webhook with the
// secret the webhook checks. Telegram only delivers to https, so a server
// address without it is refused before any call.
func HwRegisterTelegramWebhook(ctx context.Context) error {
	if hwTelegramBotToken() == "" {
		return ErrHwTelegramUnavailable
	}
	address := strings.TrimRight(strings.TrimSpace(system_setting.ServerAddress), "/")
	if !strings.HasPrefix(address, "https://") {
		return ErrHwTelegramWebhookAddress
	}
	return hwTelegramCall(ctx, "setWebhook", hwTelegramWebhookRequest{
		Url:            address + hwTelegramWebhookPath,
		SecretToken:    hwTelegramWebhookSecret(),
		AllowedUpdates: []string{"message"},
	})
}

// HwRegisterTelegramWebhookInBackground registers the webhook outside the
// caller, after a token or server address change and at startup. Without a
// token there is nothing to register and nothing is logged.
func HwRegisterTelegramWebhookInBackground() {
	if hwTelegramBotToken() == "" {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				common.SysLog("telegram webhook registration recovered from a panic")
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), hwTelegramRequestTimeout)
		defer cancel()
		if err := HwRegisterTelegramWebhook(ctx); err != nil {
			common.SysError(common.LogText("telegram webhook registration failed: %s", err.Error()))
			return
		}
		common.SysLog("telegram webhook registered")
	}()
}

// hwTelegramCall posts one Bot API method. The API base comes from
// HW_TELEGRAM_API_BASE (a test double in tests). The URL carries the token, so
// no error returned here quotes it.
// hwTelegramAPIBase returns the Bot API base. The bot token travels in the URL
// path, so only https leaves the host; plain http is accepted for a loopback
// address only (a test server or a local Bot API server).
func hwTelegramAPIBase() (string, bool) {
	base := strings.TrimSuffix(strings.TrimSpace(os.Getenv(hwTelegramAPIBaseEnv)), "/")
	if base == "" {
		return hwTelegramDefaultAPIBase, true
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" {
		return "", false
	}
	switch parsed.Scheme {
	case "https":
		return base, true
	case "http":
		host := parsed.Hostname()
		ip := net.ParseIP(host)
		return base, host == "localhost" || (ip != nil && ip.IsLoopback())
	}
	return "", false
}

func hwTelegramCall(ctx context.Context, method string, request any) error {
	body, err := common.Marshal(request)
	if err != nil {
		return ErrHwTelegramDeliveryFailed
	}
	base, ok := hwTelegramAPIBase()
	if !ok {
		return ErrHwTelegramDeliveryFailed
	}
	endpoint := base + "/bot" + hwTelegramBotToken() + "/" + method
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ErrHwTelegramDeliveryFailed
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := hwTelegramClient.Do(httpRequest)
	if err != nil {
		return ErrHwTelegramDeliveryFailed
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, hwTelegramMaxBodyBytes))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return ErrHwTelegramDeliveryFailed
	}
	return nil
}

// hwTelegramWebhookRequest is the setWebhook body.
type hwTelegramWebhookRequest struct {
	Url            string   `json:"url"`
	SecretToken    string   `json:"secret_token"`
	AllowedUpdates []string `json:"allowed_updates"`
}

// hwTelegramMessageRequest is the sendMessage body. The reply markup is only
// present when a button is asked for.
type hwTelegramMessageRequest struct {
	ChatId      int64               `json:"chat_id"`
	Text        string              `json:"text"`
	ReplyMarkup *hwTelegramKeyboard `json:"reply_markup,omitempty"`
}

// hwTelegramKeyboard is the one-button keyboard with the share-contact button.
type hwTelegramKeyboard struct {
	Keyboard        [][]hwTelegramButton `json:"keyboard"`
	ResizeKeyboard  bool                 `json:"resize_keyboard,omitempty"`
	OneTimeKeyboard bool                 `json:"one_time_keyboard,omitempty"`
}

// hwTelegramButton is one keyboard button. request_contact asks Telegram to
// attach the sender's own phone number to the answer.
type hwTelegramButton struct {
	Text           string `json:"text"`
	RequestContact bool   `json:"request_contact,omitempty"`
}
