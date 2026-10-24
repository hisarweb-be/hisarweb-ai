package service

// The admin agent: a chat in the admin panel where the owner reports an error
// or a hard word. It runs through the gateway's own customer route (so it
// uses, and is metered like, our own product) with the best model for the
// best price: Haiku 5.5, 0.10/0.50 dollar per million tokens, newer and
// cheaper than Haiku 4.5 (kennis/beste-model-beste-prijs.md). The agent
// answers in a fixed JSON shape: a reply, an optional text fix that an admin
// applies with one click, and an optional escalation card for the bureau.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
)

const (
	HwAdminAgentBaseURLEnv = "HW_ADMIN_AGENT_BASE_URL"
	HwAdminAgentAPIKeyEnv  = "HW_ADMIN_AGENT_API_KEY"
	HwAdminAgentModelEnv   = "HW_ADMIN_AGENT_MODEL"
	// HwAdminAgentDefaultModel is the cheapest model that writes good Turkish
	// and Dutch and follows a JSON instruction.
	HwAdminAgentDefaultModel = "claude-haiku-5-5"

	HwAdminAgentMaxTurns        = 12
	HwAdminAgentMaxMessageRunes = 2000
	hwAdminAgentMaxTokens       = 700
)

// HwAdminAgentConfig is where the agent calls the gateway.
type HwAdminAgentConfig struct {
	BaseURL string
	APIKey  string
	Model   string
	Client  *http.Client
}

// HwAdminAgentConfigFromEnv reads the configuration. The measurement runner's
// gateway address and key are the fallback: it is the same gateway.
func HwAdminAgentConfigFromEnv() HwAdminAgentConfig {
	pick := func(names ...string) string {
		for _, name := range names {
			if value := strings.TrimSpace(os.Getenv(name)); value != "" {
				return value
			}
		}
		return ""
	}
	model := pick(HwAdminAgentModelEnv)
	if model == "" {
		model = HwAdminAgentDefaultModel
	}
	return HwAdminAgentConfig{
		BaseURL: pick(HwAdminAgentBaseURLEnv, HwQualityBaseURLEnv),
		APIKey:  pick(HwAdminAgentAPIKeyEnv, HwQualityAPIKeyEnv),
		Model:   model,
	}
}

// HwAdminAgentTurn is one chat message of the conversation.
type HwAdminAgentTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// HwAdminAgentTextFix asks the admin to replace a displayed text. Shown is
// the text exactly as the admin sees it; the panel finds the i18n key.
type HwAdminAgentTextFix struct {
	Lang     string `json:"lang"`
	Shown    string `json:"shown"`
	Proposed string `json:"proposed"`
}

// HwAdminAgentEscalation is a larger problem that goes to the bureau.
type HwAdminAgentEscalation struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

// HwAdminAgentAnswer is what the panel receives.
type HwAdminAgentAnswer struct {
	Reply    string                  `json:"reply"`
	TextFix  *HwAdminAgentTextFix    `json:"text_fix,omitempty"`
	Escalate *HwAdminAgentEscalation `json:"escalate,omitempty"`
}

const hwAdminAgentInstruction = `You are the assistant of the owner of api.hisarweb.be, a Turkish AI gateway. The owner reports an error or a hard word on the admin panel or the customer pages. Customers are Turkish; the customer interface language is Turkish. Reply in the language the owner writes in (Dutch by default), plain and short.

Answer with one JSON object and nothing else:
{"reply": "<your answer>", "text_fix": null, "escalate": null}

Fill text_fix only when the owner points at a text that should read better: {"lang": "tr", "shown": "<the text exactly as the owner sees it>", "proposed": "<the better text in the same language>"}. Keep every {{placeholder}} of the original unchanged, add no HTML, and keep it short and everyday: no jargon, no machine-translated phrasing.
Fill escalate only for a real malfunction that a text change does not solve (a wrong number, a broken page, a failed payment): {"title": "<one line>", "summary": "<what happens, where, what was expected>"}.
Never ask for passwords, API keys or other secrets. You cannot change settings, money or accounts yourself.`

// HwAdminAgentValidate trims and bounds a conversation before it is sent.
func HwAdminAgentValidate(turns []HwAdminAgentTurn) ([]HwAdminAgentTurn, error) {
	if len(turns) == 0 {
		return nil, errors.New("no message")
	}
	if len(turns) > HwAdminAgentMaxTurns {
		turns = turns[len(turns)-HwAdminAgentMaxTurns:]
	}
	clean := make([]HwAdminAgentTurn, 0, len(turns))
	for _, turn := range turns {
		if turn.Role != "user" && turn.Role != "assistant" {
			return nil, errors.New("invalid role")
		}
		content := strings.TrimSpace(turn.Content)
		if content == "" || utf8.RuneCountInString(content) > HwAdminAgentMaxMessageRunes {
			return nil, errors.New("message is empty or too long")
		}
		clean = append(clean, HwAdminAgentTurn{Role: turn.Role, Content: content})
	}
	if clean[len(clean)-1].Role != "user" {
		return nil, errors.New("the last message must come from the user")
	}
	return clean, nil
}

// HwAdminAgentAsk sends the conversation to the gateway and decodes the
// answer. The instruction travels in the first user message, like the judge
// prompt: the primary supplier replaces a customer system message.
func HwAdminAgentAsk(ctx context.Context, config HwAdminAgentConfig, turns []HwAdminAgentTurn) (HwAdminAgentAnswer, error) {
	if config.BaseURL == "" || config.APIKey == "" {
		return HwAdminAgentAnswer{}, errors.New("the admin agent is not connected to the gateway")
	}
	turns, err := HwAdminAgentValidate(turns)
	if err != nil {
		return HwAdminAgentAnswer{}, err
	}
	client := config.Client
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	messages := make([]hwChatMessage, 0, len(turns))
	for index, turn := range turns {
		content := turn.Content
		if index == 0 {
			content = hwAdminAgentInstruction + "\n\nOwner message:\n" + content
		}
		messages = append(messages, hwChatMessage{Role: turn.Role, Content: content})
	}
	if messages[0].Role != "user" {
		messages = append([]hwChatMessage{{Role: "user", Content: hwAdminAgentInstruction}}, messages...)
	}
	text, _, err := hwCallChat(ctx, client, config.BaseURL, config.APIKey, hwChatRequest{
		Model: config.Model, Messages: messages, MaxTokens: hwAdminAgentMaxTokens,
	})
	if err != nil {
		return HwAdminAgentAnswer{}, fmt.Errorf("admin agent call failed: %w", err)
	}
	return HwAdminAgentParse(text), nil
}

// HwAdminAgentParse reads the model's answer. A model that ignores the JSON
// instruction still gets its reply shown, just without any action buttons.
func HwAdminAgentParse(text string) HwAdminAgentAnswer {
	trimmed := strings.TrimSpace(text)
	if start, end := strings.Index(trimmed, "{"), strings.LastIndex(trimmed, "}"); start >= 0 && end > start {
		var answer HwAdminAgentAnswer
		if err := common.UnmarshalJsonStr(trimmed[start:end+1], &answer); err == nil && strings.TrimSpace(answer.Reply) != "" {
			answer.Reply = strings.TrimSpace(answer.Reply)
			if answer.TextFix != nil && (strings.TrimSpace(answer.TextFix.Shown) == "" || strings.TrimSpace(answer.TextFix.Proposed) == "") {
				answer.TextFix = nil
			}
			if answer.Escalate != nil && strings.TrimSpace(answer.Escalate.Title) == "" {
				answer.Escalate = nil
			}
			return answer
		}
	}
	return HwAdminAgentAnswer{Reply: trimmed}
}
