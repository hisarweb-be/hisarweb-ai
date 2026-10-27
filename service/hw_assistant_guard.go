package service

import (
	"regexp"
	"strings"
)

// The upstream supplier adds a hidden prompt of its own, so a model sometimes
// presents itself as an AI, names its own model or supplier, or claims to run
// in a development environment. Such a reply is replaced by the fixed identity
// line. The guard only looks at what the reply says about itself: a setup
// answer that names Claude Code, Codex, a sold model, ANTHROPIC_BASE_URL or a
// terminal command passes untouched. The patterns run on the folded reply
// (lower case, no accents, the Turkish dotless i as i).
var hwAssistantLeakPatterns = []*regexp.Regexp{
	// English: "I am / I'm an AI, a language model, Claude, ChatGPT, GPT, Gemini".
	regexp.MustCompile(`\b(i am|i'm|i’m)\s+(an?\s+)?(ai|artificial intelligence|(large\s+)?language model|llm|claude|chatgpt|gpt|gemini)\b`),
	regexp.MustCompile(`\bas an?\s+(ai|artificial intelligence|(large\s+)?language model)\b`),
	regexp.MustCompile(`\b(i was|i am|i'm|i’m|i have been|i've been|i’ve been)\s+(made|trained|developed|built|created|designed)\s+by\b`),
	regexp.MustCompile(`\btrained by (anthropic|openai|google)\b`),
	regexp.MustCompile(`\b(i|me|my)\b[^.!?\n]*\b(made|developed|built|created) by (anthropic|openai|google)\b`),
	regexp.MustCompile(`\b(anthropic|openai|google) (made|built|created|trained|developed) me\b`),
	regexp.MustCompile(`\b(i run|i'm running|i am running|i'm powered|i am powered|i'm based|i am based)\s+(on|by)\s+(the\s+)?(claude|chatgpt|gpt|gemini|anthropic|openai)\b`),
	regexp.MustCompile(`\bi (am|'m|’m|run|work|operate)\b[^.!?\n]*\b(development|coding) environment\b`),
	regexp.MustCompile(`\bi (can|could) (run|execute) (terminal |shell )?commands\b`),
	// Dutch: "ik ben Claude / ChatGPT / een taalmodel / een AI".
	regexp.MustCompile(`\bik ben\s+(een\s+)?(ai|kunstmatige intelligentie|(groot\s+)?taalmodel|claude|chatgpt|gpt|gemini)\b`),
	regexp.MustCompile(`\bik ben\s+(gemaakt|getraind|ontwikkeld|gebouwd)\s+door\b`),
	regexp.MustCompile(`\bik (draai|werk|loop)\s+(op|met)\s+(de\s+)?(claude|chatgpt|gpt|gemini|anthropic|openai)\b`),
	regexp.MustCompile(`\bik (ben|draai|werk|zit|kan)\b[^.!?\n]*\bontwikkelomgeving`),
	// Turkish: "ben Claude", "bir dil modeliyim", "Anthropic tarafından geliştirildim".
	regexp.MustCompile(`\bben\s+(bir\s+)?(claude|chatgpt|gpt|gemini)[^\s.,!?;]*\s*([.,!?;]|$|ve\b)`),
	regexp.MustCompile(`\b(claude|chatgpt|gemini|gpt)[\w.-]*'y?[iu]m\b`),
	regexp.MustCompile(`\bben\s+(bir\s+)?(yapay zeka|(buyuk\s+)?dil modeli)`),
	regexp.MustCompile(`\b(yapay zekayim|yapay zeka asistaniyim|yapay zeka modeliyim|dil modeliyim)\b`),
	regexp.MustCompile(`\b(claude|chatgpt|gpt|gemini|anthropic|openai)\S*\s+(\S+\s+)?(ile|uzerinde)\s+calisiyorum\b`),
	regexp.MustCompile(`\b(anthropic|openai|google)\s+tarafindan\s+(gelistirildim|egitildim|yapildim|olusturuldum)\b`),
	regexp.MustCompile(`\bgelistirme ortaminda calis(irim|iyorum)\b`),
	regexp.MustCompile(`\bkomut(lari)? calistirabilirim\b`),
}

// hwAssistantLeaks reports whether the reply says something about itself that
// the customer must never see.
func hwAssistantLeaks(reply string) bool {
	folded := HwAssistantFold(reply)
	for _, pattern := range hwAssistantLeakPatterns {
		if pattern.MatchString(folded) {
			return true
		}
	}
	return false
}

// HwAssistantGuardReply returns the reply, or the identity answer when the
// reply presents itself as an AI or names its own model or supplier.
func HwAssistantGuardReply(reply, lang string) string {
	return hwAssistantGuardReplyWithIdentity(reply, lang, HwAssistantIdentity)
}

// hwAssistantGuardReplyWithIdentity is the guard with the caller's identity
// line: the owner's assistant replaces a leak with the Altuner voice, the
// customer channels with the digital-assistant line.
func hwAssistantGuardReplyWithIdentity(reply, lang string, identity map[string]string) string {
	if hwAssistantLeaks(strings.TrimSpace(reply)) {
		return identity[hwAssistantLang(lang)]
	}
	return reply
}
