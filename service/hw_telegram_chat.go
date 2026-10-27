package service

// The Telegram bot answers every free-typed message through the model with the
// shared instruction of every assistant channel (hw_assistant_knowledge.go), in
// the language of the message. A linked customer and a chat that is linked to
// no account both get answers, each within its own day budget. A message that
// replies to an earlier bot answer brings that answer along as context. The
// message body is never stored; only a thumb the customer presses stores the
// question and the answer it rates, as learning material.

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const (
	// hwTelegramChatMaxQuestionRunes bounds the question that travels to the
	// model.
	hwTelegramChatMaxQuestionRunes = 800
	// hwTelegramChatDayLimit is how many model answers one linked customer may
	// get per day.
	hwTelegramChatDayLimit = 20
	// hwTelegramChatGuestDayLimit is how many model answers a chat that is
	// linked to no account may get per day.
	hwTelegramChatGuestDayLimit = 10
	// hwTelegramEarlierMessage stands in for the customer message that a
	// replied-to bot answer once answered: Telegram does not send it along, and
	// the conversation must open with a customer turn.
	hwTelegramEarlierMessage = "(earlier message in this chat)"
	// hwTelegramFeedbackUp and hwTelegramFeedbackDown are the callback data of
	// the thumbs under a model answer.
	hwTelegramFeedbackUp   = "fb:1"
	hwTelegramFeedbackDown = "fb:0"
)

// hwTelegramDayLimit is the answer of a chat that used all model answers of the
// day. The fixed questions keep working.
func hwTelegramDayLimit(lang string) string {
	return hwAssistantPick(lang,
		"Bugünlük soru hakkınız doldu. Sabit sorular /help ile çalışmaya devam eder; yarın tekrar yazabilirsiniz.",
		"You have used today's questions. The fixed questions keep working through /help; please write again tomorrow.",
		"Je vragen van vandaag zijn op. De vaste vragen blijven via /help werken; schrijf morgen opnieuw.")
}

// hwTelegramTooLong answers a message that is too long for one model answer.
func hwTelegramTooLong(lang string) string {
	return hwAssistantPick(lang,
		"Mesajınız çok uzun. Lütfen soruyu kısaltıp tekrar gönderin.",
		"Your message is too long. Please shorten the question and send it again.",
		"Je bericht is te lang. Kort de vraag in en stuur hem opnieuw.")
}

// hwTelegramFeedbackThanks acknowledges a thumb.
func hwTelegramFeedbackThanks(lang string) string {
	return hwAssistantPick(lang,
		"Teşekkürler, geri bildiriminiz kaydedildi.",
		"Thanks, your feedback has been noted.",
		"Bedankt, je feedback is genoteerd.")
}

// hwTelegramChatReply answers one free-typed message through the model.
// budgetKey names whose day budget it spends (a user id, or "c" and the chat
// id for a chat without an account). A model answer goes out as a reply to the
// message with the two thumbs under it; a fixed text goes out plain.
func hwTelegramChatReply(ctx context.Context, message *hwTelegramMessage, budgetKey string, dayLimit int, lang string, now time.Time) {
	question := strings.TrimSpace(message.Text)
	chatID := strconv.FormatInt(message.Chat.Id, 10)
	if utf8.RuneCountInString(question) > hwTelegramChatMaxQuestionRunes {
		hwAnswerTelegramChat(ctx, chatID, hwTelegramTooLong(lang), "")
		return
	}
	config := HwAdminAgentConfigFromEnv()
	// Only a call that can reach the gateway costs money, so only that one
	// spends the budget; without a gateway HwAssistantChat answers from the code.
	if config.BaseURL != "" && config.APIKey != "" {
		count, err := model.HwAssistantBumpCounter(model.DB, "hw:tgchat:"+budgetKey+":"+now.Format("2006-01-02"), now.Unix())
		if err != nil {
			// A counter that cannot be written may not stop the answer; the day
			// limit is a cost guard, not a promise.
			common.SysError(common.LogText("telegram chat could not read its model answer budget"))
		} else if count > dayLimit {
			// Over the budget the fixed questions still answer, as the text says.
			if entry, matched := HwAssistantMatch(question, lang); entry != nil {
				hwAnswerTelegramChat(ctx, chatID, entry.Answer(matched), "")
				return
			}
			hwAnswerTelegramChat(ctx, chatID, hwTelegramDayLimit(lang), "")
			return
		}
	}
	turns := []HwAdminAgentTurn{{Role: "user", Content: question}}
	if replied := message.ReplyToMessage; replied != nil && replied.From != nil && replied.From.IsBot && strings.TrimSpace(replied.Text) != "" {
		turns = []HwAdminAgentTurn{
			{Role: "user", Content: hwTelegramEarlierMessage},
			{Role: "assistant", Content: hwAssistantClipRunes(strings.TrimSpace(replied.Text), HwAdminAgentMaxMessageRunes)},
			{Role: "user", Content: question},
		}
	}
	reply, ok := HwAssistantChat(ctx, config, HwAssistantChannelTelegram, lang, turns)
	if !ok {
		hwAnswerTelegramChat(ctx, chatID, reply, "")
		return
	}
	err := hwTelegramCall(ctx, "sendMessage", hwTelegramMessageRequest{
		ChatId:          message.Chat.Id,
		Text:            reply,
		ReplyParameters: &hwTelegramReplyParameters{MessageId: message.MessageId, AllowSendingWithoutReply: true},
		ReplyMarkup: &hwTelegramKeyboard{InlineKeyboard: [][]hwTelegramInlineButton{{
			{Text: "👍", CallbackData: hwTelegramFeedbackUp},
			{Text: "👎", CallbackData: hwTelegramFeedbackDown},
		}}},
	})
	if err != nil && !errors.Is(err, ErrHwTelegramUnavailable) {
		common.SysError(common.LogText("telegram answer could not be delivered"))
	}
}

// hwTelegramFeedback stores the thumb a customer pressed under a model answer.
// The answer is the bot message that carries the buttons, the question is the
// customer message it replies to. The thumbs are taken away after one press
// and the press is always acknowledged, so the button stops spinning.
func hwTelegramFeedback(ctx context.Context, query *hwTelegramCallbackQuery) HwWebhookOutcome {
	rating := 0
	switch query.Data {
	case hwTelegramFeedbackUp:
		rating = 1
	case hwTelegramFeedbackDown:
		rating = -1
	default:
		return HwWebhookIgnored
	}
	if query.From == nil {
		return HwWebhookIgnored
	}
	lang := hwTelegramAssistantLang(query.From.LanguageCode)
	if message := query.Message; message != nil && message.ReplyToMessage != nil {
		question := strings.TrimSpace(message.ReplyToMessage.Text)
		userID := 0
		if user, err := model.GetUserByTelegramID(strconv.FormatInt(query.From.Id, 10)); err == nil {
			userID = user.Id
		}
		if _, err := HwAssistantRecordFeedback(HwAssistantChannelTelegram, hwAssistantReplyLang(question, lang),
			question, strings.TrimSpace(message.Text), rating, userID, ""); err != nil {
			common.SysError(common.LogText("telegram feedback could not be stored"))
		}
		// Without a reply_markup the edit removes the inline keyboard.
		if err := hwTelegramCall(ctx, "editMessageReplyMarkup", hwTelegramEditMarkupRequest{
			ChatId: message.Chat.Id, MessageId: message.MessageId,
		}); err != nil && !errors.Is(err, ErrHwTelegramUnavailable) {
			common.SysError(common.LogText("telegram feedback buttons could not be removed"))
		}
	}
	if err := hwTelegramCall(ctx, "answerCallbackQuery", hwTelegramCallbackAnswer{
		CallbackQueryId: query.Id, Text: hwTelegramFeedbackThanks(lang),
	}); err != nil && !errors.Is(err, ErrHwTelegramUnavailable) {
		common.SysError(common.LogText("telegram feedback could not be acknowledged"))
	}
	return HwWebhookAccepted
}
