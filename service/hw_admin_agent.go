package service

// The admin agent: a chat in the admin panel where the owner reports an error
// or a hard word. It runs through the gateway's own customer route (so it
// uses, and is metered like, our own product) with the best model for the
// best price: gpt-6-luna, 0.05/0.25 dollar per million tokens, the fastest of
// the sold models in the 24-hour measurements. The agent answers in a fixed
// JSON shape: a reply, an optional text fix that an admin applies with one
// click, and an optional escalation card for the bureau.

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
	// HwAdminAgentDefaultModel is the cheapest model that answers the owner in
	// Dutch and Turkish, follows the JSON instruction and replies fastest:
	// the 24-hour gateway measurements put gpt-6-luna around 7 seconds against
	// 11 for claude-haiku-5-5, at the same lowest price tier.
	HwAdminAgentDefaultModel = "gpt-6-luna"

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
	model := HwAdminAgentResolveModel(pick(HwAdminAgentModelEnv))
	return HwAdminAgentConfig{
		BaseURL: pick(HwAdminAgentBaseURLEnv, HwQualityBaseURLEnv),
		APIKey:  pick(HwAdminAgentAPIKeyEnv, HwQualityAPIKeyEnv),
		Model:   model,
	}
}

// HwAdminAgentResolveModel makes sure the agent never runs on a retired or
// unknown model: only a model of the sold catalog is accepted, anything else
// (an empty value, a retired name such as claude-haiku-4-5, a typo) falls back
// to the default. A stale environment value can therefore not bring an old,
// dearer model back.
func HwAdminAgentResolveModel(configured string) string {
	name := strings.TrimSpace(configured)
	for _, sold := range hwSoldModels {
		if sold.Name == name {
			return name
		}
	}
	return HwAdminAgentDefaultModel
}

// HwAdminAgentAttachment is a pasted screenshot (an image data URL) or a small
// text file that travels with a message.
type HwAdminAgentAttachment struct {
	Name string `json:"name"`
	Mime string `json:"mime"`
	// Data is a data URL for an image, or the plain text of a text file.
	Data string `json:"data"`
}

const (
	HwAssistantMaxAttachments    = 3
	HwAssistantMaxImageDataBytes = 3 << 20
	HwAssistantMaxTextRunes      = 12000
)

var hwAssistantImageMimes = map[string]bool{"image/png": true, "image/jpeg": true, "image/webp": true, "image/gif": true}

// HwAdminAgentTurn is one chat message of the conversation.
type HwAdminAgentTurn struct {
	Role        string                   `json:"role"`
	Content     string                   `json:"content"`
	Attachments []HwAdminAgentAttachment `json:"attachments,omitempty"`
}

// hwAssistantCleanAttachments checks the attachments of one turn: images must be
// a base64 data URL of a known image type and small, text must be short.
func hwAssistantCleanAttachments(items []HwAdminAgentAttachment) ([]HwAdminAgentAttachment, error) {
	if len(items) > HwAssistantMaxAttachments {
		return nil, errors.New("too many attachments")
	}
	clean := make([]HwAdminAgentAttachment, 0, len(items))
	for _, item := range items {
		item.Name = strings.TrimSpace(item.Name)
		if utf8.RuneCountInString(item.Name) > 120 {
			item.Name = string([]rune(item.Name)[:120])
		}
		if hwAssistantImageMimes[item.Mime] {
			prefix := "data:" + item.Mime + ";base64,"
			if !strings.HasPrefix(item.Data, prefix) || len(item.Data) > HwAssistantMaxImageDataBytes {
				return nil, errors.New("invalid or too large image")
			}
		} else if strings.HasPrefix(item.Mime, "text/") {
			if strings.TrimSpace(item.Data) == "" || utf8.RuneCountInString(item.Data) > HwAssistantMaxTextRunes {
				return nil, errors.New("text attachment is empty or too long")
			}
		} else {
			return nil, errors.New("unsupported attachment type")
		}
		clean = append(clean, item)
	}
	return clean, nil
}

// hwAssistantMessage turns one clean turn into a chat message; images become
// content parts, text files are appended to the text.
func hwAssistantMessage(role, content string, attachments []HwAdminAgentAttachment) hwChatMessage {
	var parts []hwChatPart
	for _, item := range attachments {
		if hwAssistantImageMimes[item.Mime] {
			parts = append(parts, hwChatPart{Type: "image_url", ImageURL: &hwChatImageURL{URL: item.Data}})
		} else {
			content += "\n\nAttached file " + item.Name + ":\n" + item.Data
		}
	}
	if len(parts) == 0 {
		return hwChatMessage{Role: role, Content: content}
	}
	return hwChatMessage{Role: role, Content: content, Parts: append([]hwChatPart{{Type: "text", Text: content}}, parts...)}
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

const hwAdminAgentInstruction = `You are the chat employee of HisarWeb AI, fed through the Claude API, and you work only on api.hisarweb.be, a Turkish AI gateway. If asked who or what you are, say you are the chatbot employee of HisarWeb AI that runs on the Claude API. Never name a model, a model version, a supplier or a development tool, and you are not a coding assistant: you cannot run commands. Write like Altuner of HisarWeb: short active sentences, no em-dash, no emoji, no bold, no invented numbers, and end with one concrete question or action.
You are the assistant of the owner of api.hisarweb.be, a Turkish AI gateway. The owner reports an error or a hard word on the admin panel or the customer pages. Customers are Turkish; the customer interface language is Turkish. Reply in the language the owner writes in (Dutch by default), plain and short.

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
		attachments, err := hwAssistantCleanAttachments(turn.Attachments)
		if err != nil {
			return nil, err
		}
		if (content == "" && len(attachments) == 0) || utf8.RuneCountInString(content) > HwAdminAgentMaxMessageRunes {
			return nil, errors.New("message is empty or too long")
		}
		if content == "" {
			content = "(see attachment)"
		}
		clean = append(clean, HwAdminAgentTurn{Role: turn.Role, Content: content, Attachments: attachments})
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
		messages = append(messages, hwAssistantMessage(turn.Role, content, turn.Attachments))
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
	answer := HwAdminAgentParse(text)
	answer.Reply = HwAssistantGuardReply(answer.Reply, "nl")
	return answer, nil
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
