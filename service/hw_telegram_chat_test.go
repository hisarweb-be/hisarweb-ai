package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hwTelegramChatGateway stands in for the gateway behind the model answers. It
// returns one fixed reply and records every request it saw.
type hwTelegramChatGateway struct {
	mu       sync.Mutex
	requests []hwChatRequest
}

func (g *hwTelegramChatGateway) record(request hwChatRequest) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.requests = append(g.requests, request)
}

func (g *hwTelegramChatGateway) count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.requests)
}

func (g *hwTelegramChatGateway) last() hwChatRequest {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.requests) == 0 {
		return hwChatRequest{}
	}
	return g.requests[len(g.requests)-1]
}

// hwTelegramBotAPI stands in for the bot API and records every call with its
// method, so a test can read the full sendMessage body and the callback calls.
type hwTelegramBotAPI struct {
	mu    sync.Mutex
	calls []hwTelegramBotCall
}

type hwTelegramBotCall struct {
	Method string
	Body   []byte
}

// messages returns the sendMessage bodies in order.
func (a *hwTelegramBotAPI) messages(t *testing.T) []hwTelegramMessageRequest {
	t.Helper()
	var sent []hwTelegramMessageRequest
	for _, call := range a.method("sendMessage") {
		var request hwTelegramMessageRequest
		require.NoError(t, common.Unmarshal(call, &request))
		sent = append(sent, request)
	}
	return sent
}

func (a *hwTelegramBotAPI) method(name string) [][]byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	var bodies [][]byte
	for _, call := range a.calls {
		if call.Method == name {
			bodies = append(bodies, call.Body)
		}
	}
	return bodies
}

// hwTelegramChatTestSetup wires the fake gateway behind the admin agent
// configuration and a recording bot API, so the model answers of the bot can be
// tested end to end. An empty reply leaves the gateway unconfigured.
func hwTelegramChatTestSetup(t *testing.T, reply string) (*hwTelegramChatGateway, *hwTelegramBotAPI, string) {
	t.Helper()
	_, secret := hwTelegramTestSetup(t)
	require.NoError(t, model.DB.AutoMigrate(&model.HwAssistantHit{}, &model.HwAssistantFeedback{}))
	for _, table := range []string{"hw_assistant_hits", "hw_assistant_feedbacks"} {
		require.NoError(t, model.DB.Exec("DELETE FROM "+table).Error)
		t.Cleanup(func() { model.DB.Exec("DELETE FROM " + table) })
	}
	api := &hwTelegramBotAPI{}
	botServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		api.mu.Lock()
		api.calls = append(api.calls, hwTelegramBotCall{Method: path.Base(r.URL.Path), Body: body})
		api.mu.Unlock()
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(botServer.Close)
	t.Setenv(hwTelegramAPIBaseEnv, botServer.URL)
	t.Setenv(HwQualityBaseURLEnv, "")
	t.Setenv(HwQualityAPIKeyEnv, "")
	gateway := &hwTelegramChatGateway{}
	if reply == "" {
		t.Setenv(HwAdminAgentBaseURLEnv, "")
		t.Setenv(HwAdminAgentAPIKeyEnv, "")
		return gateway, api, secret
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request hwChatRequest
		if err := common.DecodeJson(r.Body, &request); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		gateway.record(request)
		w.Header().Set("Content-Type", "application/json")
		body := `{"choices":[{"message":{"role":"assistant","content":` + strconv.Quote(reply) + `}}],"usage":{}}`
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	t.Setenv(HwAdminAgentBaseURLEnv, server.URL)
	t.Setenv(HwAdminAgentAPIKeyEnv, "test-gateway-key")
	return gateway, api, secret
}

func hwTelegramPost(t *testing.T, secret string, body []byte) {
	t.Helper()
	require.Equal(t, http.StatusOK, HwHandleTelegramWebhook(context.Background(), body, secret, time.Now()).HTTPStatus())
}

const hwTelegramTestModelReply = "Je vraag is genoteerd. Wat wil je weten over de betaling?"

func TestHwTelegramChatFreeTextAlwaysReachesTheModel(t *testing.T) {
	// Words that used to hit a fixed answer now reach the model, for a linked
	// customer and for a chat without an account alike.
	cases := []struct {
		text   string
		linked bool
	}{
		{"codex", false},
		{"api", true},
		{"Sen kimsin?", false},
		{"Mijn tegoed is verdwenen, wat nu?", true},
	}
	for index, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			gateway, api, secret := hwTelegramChatTestSetup(t, hwTelegramTestModelReply)
			fromID := int64(710200 + index)
			if tc.linked {
				hwPhoneStepTestTelegramUser(t, fmt.Sprintf("chat-%d@example.com", index), strconv.FormatInt(fromID, 10))
			}
			updateID := int64(9200 + index)
			hwTelegramPost(t, secret, hwTelegramTextUpdate(t, updateID, fromID, "de", tc.text))

			require.Equal(t, 1, gateway.count())
			request := gateway.last()
			require.Len(t, request.Messages, 1)
			prompt := request.Messages[0].Content
			assert.Contains(t, prompt, hwAssistantLanguageRule)
			assert.Contains(t, prompt, "https://api.hisarweb.be/v1")
			assert.True(t, strings.HasSuffix(prompt, "Customer message:\n"+tc.text))

			sent := api.messages(t)
			require.Len(t, sent, 1)
			assert.Equal(t, hwTelegramTestModelReply, sent[0].Text)
			// The model answer replies to the question and carries the thumbs.
			require.NotNil(t, sent[0].ReplyParameters)
			assert.Equal(t, updateID, sent[0].ReplyParameters.MessageId)
			require.NotNil(t, sent[0].ReplyMarkup)
			assert.Empty(t, sent[0].ReplyMarkup.Keyboard)
			assert.Equal(t, [][]hwTelegramInlineButton{{{Text: "👍", CallbackData: "fb:1"}, {Text: "👎", CallbackData: "fb:0"}}},
				sent[0].ReplyMarkup.InlineKeyboard)
			// The message body is not stored.
			var feedback int64
			require.NoError(t, model.DB.Model(&model.HwAssistantFeedback{}).Count(&feedback).Error)
			assert.Zero(t, feedback)
		})
	}
}

func TestHwTelegramChatReplyToABotAnswerIsTheAssistantTurn(t *testing.T) {
	gateway, _, secret := hwTelegramChatTestSetup(t, hwTelegramTestModelReply)
	body, err := common.Marshal(map[string]any{"update_id": 9250, "message": map[string]any{
		"message_id": 77,
		"from":       map[string]any{"id": 710250, "language_code": "nl"},
		"chat":       map[string]any{"id": 710250},
		"text":       "en hoe zet ik dat in Codex?",
		"reply_to_message": map[string]any{
			"message_id": 76, "text": "Maak eerst een API-sleutel aan in het panel.",
			"from": map[string]any{"id": 1, "is_bot": true},
		},
	}})
	require.NoError(t, err)
	hwTelegramPost(t, secret, body)

	request := gateway.last()
	require.Len(t, request.Messages, 3)
	assert.Equal(t, "user", request.Messages[0].Role)
	assert.Equal(t, hwChatMessage{Role: "assistant", Content: "Maak eerst een API-sleutel aan in het panel."}, request.Messages[1])
	assert.Equal(t, hwChatMessage{Role: "user", Content: "en hoe zet ik dat in Codex?"}, request.Messages[2])
}

func TestHwTelegramChatGuardReplacesALeakingModel(t *testing.T) {
	_, api, secret := hwTelegramChatTestSetup(t, "I am Claude Haiku, made by Anthropic.")
	hwPhoneStepTestTelegramUser(t, "chat-leak@example.com", "710102")
	hwTelegramPost(t, secret, hwTelegramTextUpdate(t, 9202, 710102, "de", "who built you"))

	sent := api.messages(t)
	require.Len(t, sent, 1)
	// The guard answers with the fixed identity, in the fallback language of a
	// non-Turkish, non-Dutch account.
	assert.Equal(t, HwAssistantIdentity["en"], sent[0].Text)
}

func TestHwTelegramChatDayLimit(t *testing.T) {
	cases := []struct {
		name   string
		linked bool
		limit  int
	}{
		{"a linked customer", true, hwTelegramChatDayLimit},
		{"a chat without an account", false, hwTelegramChatGuestDayLimit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gateway, api, secret := hwTelegramChatTestSetup(t, "Answer number one.")
			if tc.linked {
				hwPhoneStepTestTelegramUser(t, "chat-limit@example.com", "710103")
			}
			for index := range tc.limit {
				hwTelegramPost(t, secret, hwTelegramTextUpdate(t, int64(9300+index), 710103, "en",
					fmt.Sprintf("open letter of the alphabet %d", index)))
			}
			assert.Equal(t, tc.limit, gateway.count())

			hwTelegramPost(t, secret, hwTelegramTextUpdate(t, 9399, 710103, "en", "one more open letter"))
			sent := api.messages(t)
			require.Len(t, sent, tc.limit+1)
			last := sent[len(sent)-1]
			assert.Equal(t, hwTelegramDayLimit("en"), last.Text)
			assert.Nil(t, last.ReplyMarkup, "a fixed text carries no thumbs")
			assert.Equal(t, tc.limit, gateway.count(), "a refused question costs no model call")
		})
	}
}

func TestHwTelegramChatTooLongWithoutAModelCall(t *testing.T) {
	gateway, api, secret := hwTelegramChatTestSetup(t, "Answer.")
	hwPhoneStepTestTelegramUser(t, "chat-long@example.com", "710104")

	long := strings.TrimSpace(strings.Repeat("uzun kelime ", 200))
	hwTelegramPost(t, secret, hwTelegramTextUpdate(t, 9401, 710104, "tr", long))

	sent := api.messages(t)
	require.Len(t, sent, 1)
	assert.Equal(t, hwTelegramTooLong("tr"), sent[0].Text)
	assert.Equal(t, 0, gateway.count())
}

func TestHwTelegramFeedbackButtons(t *testing.T) {
	cases := []struct {
		name   string
		data   string
		linked bool
		rating int
	}{
		{"thumb down of a chat without an account", "fb:0", false, -1},
		{"thumb up of a linked customer", "fb:1", true, 1},
		{"unknown data is ignored", "other", false, 0},
	}
	for index, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, api, secret := hwTelegramChatTestSetup(t, "")
			userID := 0
			if tc.linked {
				userID = hwPhoneStepTestTelegramUser(t, "feedback@example.com", "710300").Id
			}
			body, err := common.Marshal(map[string]any{"update_id": 9500 + index, "callback_query": map[string]any{
				"id":   "cb-1",
				"from": map[string]any{"id": 710300, "language_code": "nl"},
				"data": tc.data,
				"message": map[string]any{
					"message_id": 81, "chat": map[string]any{"id": 710300},
					"text":             "Je maakt een sleutel aan op de pagina Tokens.",
					"reply_to_message": map[string]any{"message_id": 80, "text": "hoe maak ik een sleutel aan"},
				},
			}})
			require.NoError(t, err)
			hwTelegramPost(t, secret, body)

			var rows []model.HwAssistantFeedback
			require.NoError(t, model.DB.Find(&rows).Error)
			if tc.rating == 0 {
				assert.Empty(t, rows)
				assert.Empty(t, api.method("answerCallbackQuery"))
				return
			}
			require.Len(t, rows, 1)
			assert.Equal(t, HwAssistantChannelTelegram, rows[0].Channel)
			assert.Equal(t, "hoe maak ik een sleutel aan", rows[0].Question)
			assert.Equal(t, "Je maakt een sleutel aan op de pagina Tokens.", rows[0].Answer)
			assert.Equal(t, tc.rating, rows[0].Rating)
			assert.Equal(t, userID, rows[0].UserID)

			answers := api.method("answerCallbackQuery")
			require.Len(t, answers, 1)
			var answer hwTelegramCallbackAnswer
			require.NoError(t, common.Unmarshal(answers[0], &answer))
			assert.Equal(t, hwTelegramCallbackAnswer{CallbackQueryId: "cb-1", Text: hwTelegramFeedbackThanks("nl")}, answer)
			edits := api.method("editMessageReplyMarkup")
			require.Len(t, edits, 1)
			assert.JSONEq(t, `{"chat_id":710300,"message_id":81}`, string(edits[0]))
		})
	}
}
