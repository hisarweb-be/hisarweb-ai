package service

// /help in the Telegram bot lists the fixed questions of the chat on the site
// (hw_assistant_faq.go). A typed question always goes to the model
// (hw_telegram_chat.go); the fixed answers only serve as its fallback.

import (
	"strings"
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
