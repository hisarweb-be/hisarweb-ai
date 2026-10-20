package service

import (
	"bytes"
	"context"
	"crypto/subtle"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

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
}

// hwTelegramTextsFor picks Turkish for a Turkish Telegram account and English
// for every other language.
func hwTelegramTextsFor(languageCode string) hwTelegramTexts {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(languageCode)), "tr") {
		return hwTelegramTexts{
			Prompt:   "Ücretsiz deneme krediniz için kendi telefon numaranızı paylaşın.",
			Button:   "Telefon numaramı paylaş",
			Accepted: "Telefon numaranız kaydedildi.",
			Rejected: "Bu telefon numarası kabul edilmiyor.",
		}
	}
	return hwTelegramTexts{
		Prompt:   "Share your own phone number to claim your free trial credit.",
		Button:   "Share my phone number",
		Accepted: "Your phone number has been recorded.",
		Rejected: "This phone number cannot be accepted.",
	}
}

// hwTelegramUpdate is the part of a bot update this webhook reads. The message
// body is never logged or stored.
type hwTelegramUpdate struct {
	UpdateId int64 `json:"update_id"`
	Message  *struct {
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
	} `json:"message"`
}

// HwHandleTelegramWebhook verifies and processes one bot update. Without
// TELEGRAM_WEBHOOK_SECRET every update is refused with 503: an unauthenticated
// caller could otherwise hand any account a phone number it does not own. The
// secret header is compared in constant time.
//
// Only a message that shares the sender's own number is processed: a forwarded
// contact carries another user_id or none, and the linked account of the sender
// must exist. The handler is idempotent on update_id, so a repeated delivery is
// acknowledged without doing the work twice.
func HwHandleTelegramWebhook(ctx context.Context, rawBody []byte, secretHeader string, now time.Time) HwWebhookOutcome {
	secret := strings.TrimSpace(os.Getenv(hwTelegramWebhookSecretEnv))
	if secret == "" {
		common.SysError(common.LogText("telegram webhook refused: %s is not configured", hwTelegramWebhookSecretEnv))
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
	claimed, err := model.HwClaimTelegramUpdate(update.UpdateId, now.Unix())
	if err != nil {
		if errors.Is(err, model.ErrHwInvalidTelegramUpdate) {
			common.SysLog(common.LogText("telegram webhook rejected: the update has no usable id"))
			return HwWebhookBadRequest
		}
		common.SysError(common.LogText("telegram webhook could not claim its update"))
		return HwWebhookUnavailable
	}
	if !claimed {
		// Telegram repeated a delivery we already handled.
		return HwWebhookAccepted
	}
	outcome := hwProcessTelegramUpdate(ctx, update)
	if outcome == HwWebhookUnavailable {
		// Give the claim back so the retry of a failed delivery can do the work.
		if err := model.HwReleaseTelegramUpdate(update.UpdateId); err != nil {
			common.SysError(common.LogText("telegram webhook could not release its failed update"))
		}
	}
	return outcome
}

// hwProcessTelegramUpdate handles one claimed update and answers the customer.
// Log lines carry user ids, outcomes and counts only: no phone number, no chat
// id and no message body.
func hwProcessTelegramUpdate(ctx context.Context, update hwTelegramUpdate) HwWebhookOutcome {
	message := update.Message
	if message == nil || message.From == nil {
		return HwWebhookIgnored
	}
	user, err := model.GetUserByTelegramID(strconv.FormatInt(message.From.Id, 10))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		common.SysLog(common.LogText("telegram webhook ignored a message from a Telegram account that is linked to no user"))
		return HwWebhookIgnored
	}
	if err != nil {
		common.SysError(common.LogText("telegram webhook could not look up the linked user"))
		return HwWebhookUnavailable
	}
	texts := hwTelegramTextsFor(message.From.LanguageCode)
	if message.Contact == nil {
		// A bot may only write to a chat the customer opened, so the request for
		// the number answers the /start that opened it; pushing it after the
		// login would reach only customers who had opened the bot before.
		if strings.HasPrefix(strings.TrimSpace(message.Text), "/start") {
			hwAnswerTelegramChat(ctx, strconv.FormatInt(message.Chat.Id, 10), texts.Prompt, texts.Button)
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
	if err := model.HwRecordPhoneSignal(user.Id, message.Contact.PhoneNumber); err != nil {
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
// answer. The bot token is read from TELEGRAM_BOT_TOKEN and the API base from
// HW_TELEGRAM_API_BASE (a test double in tests). Neither the token nor the URL
// reaches a log line, an error text or an answer.
func HwTelegramSendMessage(ctx context.Context, chatID string, text string, button string) error {
	token := strings.TrimSpace(os.Getenv(hwTelegramBotTokenEnv))
	if token == "" {
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
	body, err := common.Marshal(request)
	if err != nil {
		return ErrHwTelegramDeliveryFailed
	}
	base := strings.TrimSuffix(strings.TrimSpace(os.Getenv(hwTelegramAPIBaseEnv)), "/")
	if base == "" {
		base = hwTelegramDefaultAPIBase
	}
	url := base + "/bot" + token + "/sendMessage"
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
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
