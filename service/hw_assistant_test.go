package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

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

// The guard only catches what a reply says about itself; a setup answer that
// names Claude Code, Codex, a sold model or a terminal command passes.
func TestHwAssistantGuardReplacesLeaks(t *testing.T) {
	cases := []struct {
		reply string
		leaks bool
	}{
		{"I am Claude Sonnet, made by Anthropic.", true},
		{"I'm an AI language model and cannot help.", true},
		{"As an AI, I have no opinions.", true},
		{"I am Claude Code, Anthropic's official CLI.", true},
		{"I was trained by OpenAI.", true},
		{"I am ChatGPT.", true},
		{"I run on Gemini.", true},
		{"I work in a development environment with your files.", true},
		{"Ik ben Claude, gemaakt door Anthropic.", true},
		{"Ik ben een taalmodel.", true},
		{"Ik ben ChatGPT.", true},
		{"Ik kan terminal commando's uitvoeren in een ontwikkelomgeving.", true},
		{"Ben Claude, size yardımcı olurum.", true},
		{"Ben bir dil modeliyim.", true},
		{"Ben bir yapay zeka asistanıyım, Claude Haiku ile çalışıyorum.", true},
		{"Ben claude-haiku-4-5-20251001 ve AI geliştirme ortamında çalışırım.", true},
		{"Anthropic tarafından geliştirildim.", true},
		{"Install Claude Code with npm install -g @anthropic-ai/claude-code and set ANTHROPIC_BASE_URL=https://api.hisarweb.be. Which step are you on?", false},
		{"Set ANTHROPIC_MODEL=claude-sonnet-5-5 and type claude in the terminal. I can walk you through it, shall we?", false},
		{"Codex CLI: put model = \"gpt-6-luna\" in ~/.codex/config.toml. Which step are you on?", false},
		{"Zet ANTHROPIC_BASE_URL=https://api.hisarweb.be en typ claude in je terminal. Ik help je graag met Claude Code, zullen we beginnen?", false},
		{"Claude Code'u kurmak için terminalde npm install -g @anthropic-ai/claude-code yaz. Ben de adım adım yardımcı olurum, hangi işletim sistemini kullanıyorsun?", false},
		{"gpt-6-luna en claude-haiku-5-5 zijn onze goedkoopste modellen. Welke wil je proberen?", false},
		{"Fiyatlar sayfasına bakın.", false},
	}
	for _, tc := range cases {
		want := tc.reply
		if tc.leaks {
			want = HwAssistantIdentity["tr"]
		}
		assert.Equal(t, want, HwAssistantGuardReply(tc.reply, "tr"), tc.reply)
	}
	for lang, text := range HwAssistantIdentity {
		assert.False(t, hwAssistantLeaks(text), lang)
		assert.NotContains(t, text, "—", lang)
		assert.NotContains(t, text, "Claude", lang)
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
	assert.Contains(t, answer, "HISARWEB_API_KEY", "a Codex question gets the Codex config.toml steps")
	assert.Contains(t, answer, "https://api.hisarweb.be/v1")
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

	// In the owner's panel the identity speaks as Altuner, by chip id and in
	// the fallback; every other entry is the same as for customers.
	answer, ok = HwAssistantAnswerByID("identity", "tr", true)
	require.True(t, ok)
	assert.Equal(t, HwAltunerIdentity["tr"], answer, "the panel identity speaks as Altuner")
	assert.Equal(t, HwAltunerIdentity["tr"], HwAssistantFallback(HwAssistantChannelAdmin, "nl", "Merhaba sen kimsin"))
	assert.Equal(t, HwAssistantIdentity["tr"], HwAssistantFallback(HwAssistantChannelWidget, "nl", "Merhaba sen kimsin"))
}

func TestHwAssistantAnswerByIDNeedsAKnownID(t *testing.T) {
	answer, ok := HwAssistantAnswerByID("identity", "nl", false)
	require.True(t, ok)
	assert.Equal(t, HwAssistantIdentity["nl"], answer)
	answer, ok = HwAssistantAnswerByID("codex-cli", "en", true)
	require.True(t, ok)
	assert.Contains(t, answer, "HISARWEB_API_KEY")
	for _, free := range []string{"codex", "api key", "kimsin", "", "Sen kimsin?"} {
		_, ok := HwAssistantAnswerByID(free, "tr", false)
		assert.False(t, ok, "free text is no chip id: %q", free)
	}
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

func TestHwAssistantLearnedSection(t *testing.T) {
	rows := []model.HwAssistantFeedback{
		{Question: "How do I get an API key for my app", Answer: "Create a token in the panel.", Rating: 1},
		{Question: "How do I top up my balance with a card", Answer: "Wrong answer about cards.", Rating: -1},
		{Question: "How do I top up my balance with a card", Answer: "Old answer.", Rating: -1, Correction: "Use the Wallet page and pick Top up."},
	}
	cases := []struct {
		name, question string
		contains       []string
		excludes       []string
	}{
		{"similar question picks the approved row", "where can I get an api key?", []string{"Approved answers", "Create a token in the panel."}, []string{"rated wrong"}},
		{"wrong row lands under the rated wrong heading", "top up balance card", []string{"Answers that were rated wrong", "Wrong answer about cards."}, nil},
		{"correction wins over answer", "top up balance card", []string{"Use the Wallet page and pick Top up."}, []string{"Old answer."}},
		{"unrelated question gives nothing", "what is the weather tomorrow", nil, []string{"Q:"}},
		{"one shared word is not enough", "my app crashes", nil, []string{"Q:"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			section := HwAssistantLearnedSection(tc.question, rows)
			for _, want := range tc.contains {
				assert.Contains(t, section, want)
			}
			for _, unwanted := range tc.excludes {
				assert.NotContains(t, section, unwanted)
			}
		})
	}
}

// hwAssistantFeedbackDB gives the test its own feedback table and restores
// the shared database afterwards.
func hwAssistantFeedbackDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.HwAssistantFeedback{}))
	previous := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previous })
	return db
}

func TestHwAssistantInstructionEndsWithTheLanguageRule(t *testing.T) {
	hwAssistantFeedbackDB(t)
	for _, channel := range []string{HwAssistantChannelWidget, HwAssistantChannelTelegram, HwAssistantChannelAdmin} {
		instruction := HwAssistantInstruction(channel, "Hoe installeer ik Claude Code op Windows?")
		assert.True(t, strings.HasSuffix(instruction, hwAssistantLanguageRule), channel)
		assert.Contains(t, instruction, "https://api.hisarweb.be/v1", channel)
		assert.Contains(t, instruction, "ANTHROPIC_BASE_URL", channel)
		assert.Contains(t, instruction, "config.toml", channel)
		assert.LessOrEqual(t, len([]rune(instruction)), hwAssistantInstructionMaxRunes+len([]rune(hwAssistantLanguageRule))+2, channel)
	}
}

func TestHwAssistantInstructionCarriesLearnedAnswersForSimilarQuestionsOnly(t *testing.T) {
	db := hwAssistantFeedbackDB(t)
	stored, err := model.HwAssistantStoreFeedback(db, model.HwAssistantFeedback{
		Channel: HwAssistantChannelWidget, Lang: "nl", Rating: 1, CreatedAt: 1,
		Question: "Hoe zet ik Codex CLI op Windows?", Answer: "Zet config.toml in de map codex-uniek-geleerd.",
	})
	require.NoError(t, err)
	require.True(t, stored)

	similar := HwAssistantInstruction(HwAssistantChannelWidget, "Codex CLI op Windows instellen?")
	assert.Contains(t, similar, "codex-uniek-geleerd")
	assert.True(t, strings.HasSuffix(similar, hwAssistantLanguageRule), "the learned section never pushes the rule out")
	unrelated := HwAssistantInstruction(HwAssistantChannelWidget, "Welke betaalmethoden accepteren jullie?")
	assert.NotContains(t, unrelated, "codex-uniek-geleerd")
}

// hwAssistantFakeGateway answers every chat call with status and body and
// keeps the last request.
func hwAssistantFakeGateway(t *testing.T, status int, body string) (HwAdminAgentConfig, *hwChatRequest) {
	t.Helper()
	request := &hwChatRequest{}
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = common.DecodeJson(r.Body, request)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(gateway.Close)
	return HwAdminAgentConfig{BaseURL: gateway.URL, APIKey: "k", Model: HwAdminAgentDefaultModel}, request
}

func TestHwAssistantChatAnswersThroughTheGateway(t *testing.T) {
	hwAssistantFeedbackDB(t)
	config, request := hwAssistantFakeGateway(t, http.StatusOK, `{"choices":[{"message":{"role":"assistant","content":"Zet ANTHROPIC_BASE_URL. Lukt het?"}}]}`)
	turns := make([]HwAdminAgentTurn, 0, 20)
	for index := 0; index < 19; index++ {
		role := "user"
		if index%2 == 1 {
			role = "assistant"
		}
		turns = append(turns, HwAdminAgentTurn{Role: role, Content: "bericht"})
	}
	turns = append(turns, HwAdminAgentTurn{Role: "user", Content: "Hoe installeer ik Claude Code?"})

	reply, ok := HwAssistantChat(context.Background(), config, HwAssistantChannelWidget, "tr", turns)
	require.True(t, ok)
	assert.Equal(t, "Zet ANTHROPIC_BASE_URL. Lukt het?", reply)
	require.NotEmpty(t, request.Messages)
	assert.LessOrEqual(t, len(request.Messages), HwAdminAgentMaxTurns)
	assert.Equal(t, "user", request.Messages[0].Role)
	first := request.Messages[0].Content
	assert.Contains(t, first, "https://api.hisarweb.be/v1")
	assert.Contains(t, first, "ANTHROPIC_BASE_URL")
	assert.Contains(t, first, hwAssistantLanguageRule)
	assert.Equal(t, hwAssistantMaxTokens, request.MaxTokens)
}

func TestHwAssistantChatFallsBackWithoutShowingTheCause(t *testing.T) {
	hwAssistantFeedbackDB(t)
	for _, status := range []int{http.StatusForbidden, http.StatusInternalServerError} {
		config, _ := hwAssistantFakeGateway(t, status, `{"error":{"message":"upstream failed"}}`)
		for lang, question := range map[string]string{"nl": "Waarom werkt mijn sleutel niet in de terminal?", "tr": "Anahtarım terminalde neden çalışmıyor?", "en": "Why does my key not work in the terminal?"} {
			reply, ok := HwAssistantChat(context.Background(), config, HwAssistantChannelWidget, lang, []HwAdminAgentTurn{{Role: "user", Content: question}})
			assert.False(t, ok)
			assert.NotEmpty(t, reply)
			assert.NotContains(t, strings.ToLower(reply), "status")
			assert.NotContains(t, strings.ToLower(reply), "failed")
		}
	}
	reply, ok := HwAssistantChat(context.Background(), HwAdminAgentConfig{}, HwAssistantChannelAdmin, "nl", []HwAdminAgentTurn{{Role: "user", Content: "wie ben jij"}})
	assert.False(t, ok)
	assert.Equal(t, HwAltunerIdentity["nl"], reply, "the owner's fallback for the identity question speaks as Altuner")
}
