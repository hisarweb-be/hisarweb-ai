package service

// The Telegram bot answers a question without a fixed answer through the
// gateway itself, in the language of the message: a customer may write in any
// language and the answer comes back in that language. A question with a fixed
// answer (hw_telegram_faq.go) still costs no model call. Only a linked account
// gets model answers, the message body is never stored, a per-day counter
// bounds the cost, and a gateway that does not answer falls back to the honest
// fixed text of hw_telegram_faq.go.

import (
	"context"
	"fmt"
	"net/http"
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
	// hwTelegramChatMaxTokens keeps one answer short and cheap.
	hwTelegramChatMaxTokens = 400
	// hwTelegramChatDayLimit is how many model answers one linked customer may
	// get per day. A question with a fixed answer is answered from the code and
	// never counts.
	hwTelegramChatDayLimit = 20
)

// hwTelegramChatInstruction is the stable part of the customer prompt. The
// facts and the question are appended by hwTelegramChatPrompt so the numbers
// come from the code and cannot drift from the real offer.
const hwTelegramChatInstruction = `You are the chatbot employee of HisarWeb AI and you serve the customers of api.hisarweb.be through Telegram. If asked who or what you are, say you are the chatbot employee of HisarWeb AI that runs on the Claude API. Never name a model, a model version, a supplier or a development tool, and you are not a coding assistant: you cannot run commands.
Answer in the language of the customer's message, whatever language that is, and stay in that language for the whole answer. Write short active sentences, no em-dash, no emoji, no bold, no invented numbers: use only the facts below, say plainly when you do not know, and end with one concrete question or action. You are a support chat, not a general chat: keep the talk on api.hisarweb.be and its models, prices, payment and account.
Never ask for passwords, API keys or other secrets. You cannot change settings, money or accounts yourself; for that, point at the panel at https://api.hisarweb.be.`

// hwTelegramChatFacts are the numbers the model may state: the shared facts of
// every assistant channel (hw_assistant_knowledge.go).
func hwTelegramChatFacts() string {
	return HwAssistantFacts()
}

// hwTelegramChatPrompt builds the one message of the call.
func hwTelegramChatPrompt(question string) string {
	return hwTelegramChatInstruction + "\n\n" + hwTelegramChatFacts() + "\n\nCustomer message:\n" + question
}

// hwTelegramDayLimit is the answer of a customer who used all model answers of
// the day. The fixed questions keep working.
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

// hwTelegramModelAnswer answers one question through the gateway, in the
// language of the question. An empty string means no answer came; the caller
// then sends the honest fixed text instead. lang is only the fallback language
// of the fixed texts (tr, nl or en) and of the leak guard, not the answer
// language: the model answers in the language of the message itself.
func hwTelegramModelAnswer(ctx context.Context, userID int, question, lang string, now time.Time) string {
	question = strings.TrimSpace(question)
	if utf8.RuneCountInString(question) > hwTelegramChatMaxQuestionRunes {
		return hwTelegramTooLong(lang)
	}
	config := HwAdminAgentConfigFromEnv()
	if config.BaseURL == "" || config.APIKey == "" {
		return ""
	}
	key := fmt.Sprintf("hw:tgchat:%d:%s", userID, now.Format("2006-01-02"))
	count, err := model.HwAssistantBumpCounter(model.DB, key, now.Unix())
	if err != nil {
		// A counter that cannot be written may not stop the answer; the day
		// limit is a cost guard, not a promise.
		common.SysError(common.LogText("telegram chat could not read its model answer budget"))
	} else if count > hwTelegramChatDayLimit {
		return hwTelegramDayLimit(lang)
	}
	client := config.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	text, _, err := hwCallChat(ctx, client, config.BaseURL, config.APIKey, hwChatRequest{
		Model:     config.Model,
		Messages:  []hwChatMessage{{Role: "user", Content: hwTelegramChatPrompt(question)}},
		MaxTokens: hwTelegramChatMaxTokens,
	})
	if err != nil {
		common.SysError(common.LogText("telegram chat model answer failed"))
		return ""
	}
	reply := strings.TrimSpace(text)
	if reply == "" {
		return ""
	}
	return HwAssistantGuardReply(reply, lang)
}

// hwTelegramChatReply answers a question of a linked customer that no fixed
// answer covers: through the model in the language of the message, and when
// that yields nothing with the honest fixed text.
func hwTelegramChatReply(ctx context.Context, userID int, chatID, question, lang string, now time.Time) {
	answer := hwTelegramModelAnswer(ctx, userID, question, lang, now)
	if answer == "" {
		answer = hwTelegramNoAnswer(lang)
	}
	hwAnswerTelegramChat(ctx, chatID, answer, "")
}
