package service

// The Telegram bot answers the same fixed questions as the chat on the site
// (hw_assistant_faq.go): the answer comes from the code, costs no model call and
// is the same for a customer and for someone who only opened the bot. The
// answer follows the language of the question, not of the Telegram account. A
// question without a fixed answer goes to hw_telegram_chat.go. The text of a
// message is never stored; only the id of a matched question is counted.

import (
	"context"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/model"
)

// hwTelegramAssistantLang maps the Telegram language of the sender to a language
// the fixed answers exist in: Turkish and Dutch, else English.
func hwTelegramAssistantLang(languageCode string) string {
	code := strings.ToLower(strings.TrimSpace(languageCode))
	switch {
	case strings.HasPrefix(code, "tr"):
		return "tr"
	case strings.HasPrefix(code, "nl"):
		return "nl"
	}
	return "en"
}

// hwTelegramIsHelp reports whether the text asks for the list of questions.
func hwTelegramIsHelp(text string) bool {
	command := strings.ToLower(strings.Fields(text + " x")[0])
	if at := strings.Index(command, "@"); at >= 0 {
		command = command[:at]
	}
	return command == "/help" || command == "/faq" || command == "/menu"
}

// hwTelegramHelpText lists the questions the bot can answer without a model.
func hwTelegramHelpText(lang string) string {
	lines := make([]string, 0, len(hwAssistantFAQs))
	for _, entry := range hwAssistantFAQs {
		lines = append(lines, "• "+entry.Question[lang])
	}
	list := strings.Join(lines, "\n")
	return hwAssistantPick(lang,
		"Şu soruları hemen yanıtlayabilirim:\n"+list+"\nHangisini merak ediyorsunuz?",
		"I can answer these right away:\n"+list+"\nWhich one do you want to know?",
		"Deze vragen kan ik meteen beantwoorden:\n"+list+"\nWelke wil je weten?")
}

// hwTelegramNoAnswer is the reply of a linked customer whose question has no
// fixed answer.
func hwTelegramNoAnswer(lang string) string {
	return hwAssistantPick(lang,
		"Bu soruya hazır bir yanıtım yok. /help yazarak sorabileceğiniz soruları görebilirsiniz. Hangi konuda yardım istiyorsunuz?",
		"I have no ready answer for this yet. Send /help to see the questions I can answer. What do you need help with?",
		"Hier heb ik nog geen kant-en-klaar antwoord op. Stuur /help voor de vragen die ik kan beantwoorden. Waarmee kan ik je helpen?")
}

// hwTelegramFAQReply answers a plain text message from a fixed answer. It
// returns whether it answered. Commands other than /help are left to the
// existing flow.
func hwTelegramFAQReply(ctx context.Context, message *hwTelegramMessage, now int64) bool {
	text := strings.TrimSpace(message.Text)
	if message.Contact != nil || text == "" || strings.HasPrefix(text, "/start") {
		return false
	}
	lang := hwTelegramAssistantLang(message.From.LanguageCode)
	chatID := strconv.FormatInt(message.Chat.Id, 10)
	if hwTelegramIsHelp(text) {
		hwAnswerTelegramChat(ctx, chatID, hwTelegramHelpText(lang), "")
		return true
	}
	if strings.HasPrefix(text, "/") {
		return false
	}
	id, answer, ok := HwAssistantAnswerFor(text, lang)
	if !ok {
		return false
	}
	_ = model.HwAssistantRecordHit(model.DB, "faq:"+id, now)
	hwAnswerTelegramChat(ctx, chatID, answer, "")
	return true
}
