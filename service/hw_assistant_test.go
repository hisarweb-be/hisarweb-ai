package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The agent must run on a sold, current and cheapest model; a retired model in
// the environment must never come back as default or fallback.
func TestHwAdminAgentDefaultModelIsSoldCurrentAndCheapest(t *testing.T) {
	var found *HwSoldModel
	for index := range hwSoldModels {
		if hwSoldModels[index].Name == HwAdminAgentDefaultModel {
			found = &hwSoldModels[index]
		}
	}
	require.NotNil(t, found, "the default model must be in the sold catalog")
	_, retired := HwRetiredModelSuccessor(HwAdminAgentDefaultModel)
	assert.False(t, retired)
	for _, sold := range hwSoldModels {
		assert.LessOrEqual(t, found.OutputMicroUsd, sold.OutputMicroUsd, "%s is cheaper than the default", sold.Name)
	}
}

func TestHwAdminAgentResolveModelRejectsRetiredAndUnknown(t *testing.T) {
	for retired := range hwRetiredModels {
		assert.Equal(t, HwAdminAgentDefaultModel, HwAdminAgentResolveModel(retired), retired)
	}
	assert.Equal(t, HwAdminAgentDefaultModel, HwAdminAgentResolveModel("claude-haiku-4-5-20251001"))
	assert.Equal(t, HwAdminAgentDefaultModel, HwAdminAgentResolveModel(""))
	assert.Equal(t, "claude-sonnet-5-5", HwAdminAgentResolveModel(" claude-sonnet-5-5 "))
	t.Setenv(HwAdminAgentModelEnv, "claude-haiku-4-5")
	assert.Equal(t, HwAdminAgentDefaultModel, HwAdminAgentConfigFromEnv().Model)
}

func TestHwAssistantGuardReplacesLeaks(t *testing.T) {
	for _, leak := range []string{
		"Ben claude-haiku-4-5-20251001 ve AI geliştirme ortamında çalışırım.",
		"I am Claude Sonnet, made by Anthropic.",
		"Ik kan terminal commando's uitvoeren in een ontwikkelomgeving.",
	} {
		assert.Equal(t, HwAssistantIdentity["tr"], HwAssistantGuardReply(leak, "tr"), leak)
	}
	assert.Equal(t, "Fiyatlar sayfasına bakın.", HwAssistantGuardReply("Fiyatlar sayfasına bakın.", "tr"))
	for lang, text := range HwAssistantIdentity {
		assert.NotRegexp(t, hwAssistantLeakPattern, text, lang)
		assert.NotContains(t, text, "—", lang)
	}
}

func TestHwAssistantFAQMatchesWithoutModelAndAccents(t *testing.T) {
	for question, wantID := range map[string]string{
		"Sen kimsin?":                   "identity",
		"SEN KİMSİN":                    "identity",
		"Wie ben je eigenlijk?":         "identity",
		"Hangi modelleri sunuyorsunuz":  "models",
		"fiyatlar ne kadar":             "price",
		"How do I use the API?":         "api",
		"Hangi para birimleriyle ödeme": "currency",
	} {
		id, answer, ok := HwAssistantAnswerFor(question, "tr")
		require.True(t, ok, question)
		assert.Equal(t, wantID, id, question)
		assert.NotEmpty(t, answer)
	}
	_, _, ok := HwAssistantAnswerFor("blablabla qwerty", "tr")
	assert.False(t, ok)
}

func TestHwAssistantAnswerFollowsTheQuestionLanguage(t *testing.T) {
	// A Turkish question is answered in Turkish, whatever the panel or the
	// Telegram account shows; an English question in English.
	_, answer, ok := HwAssistantAnswerFor("sen kimsin", "en")
	require.True(t, ok)
	assert.Equal(t, HwAssistantIdentity["tr"], answer)

	_, answer, ok = HwAssistantAnswerFor("who are you", "tr")
	require.True(t, ok)
	assert.Equal(t, HwAssistantIdentity["en"], answer)

	_, answer, ok = HwAssistantAnswerFor("wat kost het", "tr")
	require.True(t, ok)
	assert.Contains(t, answer, "lijstprijs")

	// A keyword that lives in several languages follows the language of the
	// question's own words; only a bare keyword without any everyday words
	// falls back to the preferred panel language.
	_, answer, _ = HwAssistantAnswerFor("api", "nl")
	assert.Contains(t, answer, "API-sleutel")

	_, answer, ok = HwAssistantAnswerFor("Hoe kan ik dit api op mijn cli installeren", "tr")
	require.True(t, ok, "a Dutch sentence with the shared word api must match the api entry")
	assert.Contains(t, answer, "API-sleutel", "a Dutch sentence is answered in Dutch on a Turkish panel")

	_, answer, ok = HwAssistantAnswerFor("bu apiyi nasil kullanirim", "nl")
	require.True(t, ok)
	assert.Contains(t, answer, "anahtar", "a Turkish sentence is answered in Turkish on a Dutch panel")

	// An English loan word inside a Turkish question does not make the answer
	// English: the owner's own report from 27 Oct. The claude-cli entry wins
	// on the longer keyword, and its Turkish steps still mention the key.
	_, answer, ok = HwAssistantAnswerFor("Nasil api key yukleye bilirim Claude CLI e?", "tr")
	require.True(t, ok, "a question about Claude CLI must match the claude-cli entry")
	assert.Contains(t, answer, "anahtar", "a Turkish question is answered in Turkish even when it contains api key")

	_, answer, ok = HwAssistantAnswerFor("nasil api key yukleye bilirim", "nl")
	require.True(t, ok)
	assert.Contains(t, answer, "anahtar", "the question language beats the panel language and the keyword language")

	// The owner's report of 27 Oct: a Codex CLI question must not be hijacked
	// by the Claude CLI entry, and both CLIs have their own steps.
	_, answer, ok = HwAssistantAnswerFor("En hoe kan ik dit bij ChatGPT codex cli installeren?", "tr")
	require.True(t, ok)
	assert.Contains(t, answer, "OPENAI_BASE_URL", "a Codex question gets the Codex steps")
	assert.NotContains(t, answer, "ANTHROPIC", "the Claude steps do not answer a Codex question")

	_, answer, ok = HwAssistantAnswerFor("Hayir Claude CLI nasil yuklerim bana adim adim anlat", "tr")
	require.True(t, ok)
	assert.Contains(t, answer, "ANTHROPIC_BASE_URL")

	_, answer, ok = HwAssistantAnswerFor("Hoe kan ik dit api bij Claude CLI installeren", "nl")
	require.True(t, ok)
	assert.Contains(t, answer, "ANTHROPIC_BASE_URL")

	// A name question matches the identity entry, and in the owner's panel
	// that answer speaks as Altuner while the public widget stays honest.
	_, answer, ok = HwAssistantAnswerFor("Senin adin ne?", "tr")
	require.True(t, ok)
	assert.Equal(t, HwAssistantIdentity["tr"], answer, "the public identity stays the chat employee")

	_, answer, ok = HwAssistantAnswerForAdmin("Merhaba sen kimsin", "tr")
	require.True(t, ok)
	assert.Equal(t, HwAltunerIdentity["tr"], answer, "the panel identity speaks as Altuner")

	_, answer, ok = HwAssistantAnswerForAdmin("Hoe kan ik dit bij ChatGPT codex cli installeren?", "nl")
	require.True(t, ok)
	assert.Contains(t, answer, "OPENAI_BASE_URL", "the panel FAQ is the same except for the identity")
}

// The owner's assistant never presents itself as a bot: a leaking reply is
// replaced with the Altuner line, not with the chat-employee identity.
func TestHwAltunerGuardReplacesLeaksAsAltuner(t *testing.T) {
	for _, leak := range []string{
		"I am Claude Haiku, an AI assistant.",
		"Ben bir yapay zeka asistanıyım, Claude Haiku ile çalışıyorum.",
	} {
		assert.Equal(t, HwAltunerIdentity["nl"], hwAssistantGuardReplyWithIdentity(leak, "nl", HwAltunerIdentity), leak)
	}
	for lang, text := range HwAltunerIdentity {
		assert.NotContains(t, strings.ToLower(text), "bot", lang)
		assert.NotRegexp(t, `(?i)\b(yapay zeka|ai assistant|language model|asistan)\b`, text, lang)
		assert.NotContains(t, text, "—", lang)
	}
	assert.Equal(t, "Fiyatlar sayfasına bakın.", hwAssistantGuardReplyWithIdentity("Fiyatlar sayfasına bakın.", "tr", HwAltunerIdentity))
}

func TestHwAssistantFAQAnswersFromCatalogAndStyle(t *testing.T) {
	for _, view := range HwAssistantFAQList("en", nil) {
		assert.NotContains(t, view.Answer, "—", view.ID)
		assert.Equal(t, view.Answer, HwAssistantGuardReply(view.Answer, "en"), view.ID)
		assert.True(t, strings.HasSuffix(strings.TrimSpace(view.Answer), "?"), "ends with a question: "+view.ID)
	}
	_, answer, _ := HwAssistantAnswerFor("which models do you offer", "en")
	for _, sold := range hwSoldModels {
		assert.Contains(t, answer, sold.Name)
	}
	list := HwAssistantFAQList("tr", map[string]int{"api": 9, "price": 3})
	assert.Equal(t, "api", list[0].ID)
	assert.Equal(t, "price", list[1].ID)
}

func TestHwAdminAgentValidateAttachments(t *testing.T) {
	png := HwAdminAgentAttachment{Name: "a.png", Mime: "image/png", Data: "data:image/png;base64,AAAA"}
	turns, err := HwAdminAgentValidate([]HwAdminAgentTurn{{Role: "user", Attachments: []HwAdminAgentAttachment{png}}})
	require.NoError(t, err)
	assert.Equal(t, "(see attachment)", turns[0].Content)
	message := hwAssistantMessage("user", "x", turns[0].Attachments)
	body, err := message.MarshalJSON()
	require.NoError(t, err)
	assert.Contains(t, string(body), `"image_url"`)
	for _, bad := range []HwAdminAgentAttachment{
		{Name: "x", Mime: "image/png", Data: "http://evil/x.png"},
		{Name: "x", Mime: "image/svg+xml", Data: "data:image/svg+xml;base64,AAAA"},
		{Name: "x", Mime: "application/pdf", Data: "x"},
		{Name: "x", Mime: "text/plain", Data: "  "},
	} {
		_, err := HwAdminAgentValidate([]HwAdminAgentTurn{{Role: "user", Content: "hi", Attachments: []HwAdminAgentAttachment{bad}}})
		assert.Error(t, err, bad.Mime)
	}
	_, err = HwAdminAgentValidate([]HwAdminAgentTurn{{Role: "user", Content: "hi", Attachments: []HwAdminAgentAttachment{png, png, png, png}}})
	assert.Error(t, err)
}
