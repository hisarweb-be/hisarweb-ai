package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	reply    string
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

func (g *hwTelegramChatGateway) lastPrompt() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.requests) == 0 {
		return ""
	}
	return g.requests[len(g.requests)-1].Messages[0].Content
}

// hwTelegramChatTestSetup wires the fake gateway behind the admin agent
// configuration, so the model answers of the bot can be tested end to end.
func hwTelegramChatTestSetup(t *testing.T, reply string) (*hwTelegramChatGateway, *hwFakeTelegramAPI, string) {
	t.Helper()
	api, secret := hwTelegramTestSetup(t)
	require.NoError(t, model.DB.AutoMigrate(&model.HwAssistantHit{}))
	t.Cleanup(func() { model.DB.Exec("DELETE FROM hw_assistant_hits") })
	gateway := &hwTelegramChatGateway{reply: reply}
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

func TestHwTelegramChatAnswersInTheLanguageOfTheMessage(t *testing.T) {
	const modelReply = "Je vraag is genoteerd. Wat wil je weten over de betaling?"
	gateway, api, secret := hwTelegramChatTestSetup(t, modelReply)
	hwPhoneStepTestTelegramUser(t, "chat-de@example.com", "710101")
	// A German Telegram account: the fixed texts would be English, but the
	// model answers in the language of the message itself, here Dutch.
	body := hwTelegramTextUpdate(t, 9201, 710101, "de", "Mijn tegoed is verdwenen, wat nu?")
	require.Equal(t, http.StatusOK, HwHandleTelegramWebhook(context.Background(), body, secret, time.Now()).HTTPStatus())

	sent := api.messages()
	require.Len(t, sent, 1)
	assert.Equal(t, modelReply, sent[0].Text)
	assert.Equal(t, 1, gateway.count())
	prompt := gateway.lastPrompt()
	assert.Contains(t, prompt, "Answer in the language of the customer's message")
	assert.Contains(t, prompt, "Mijn tegoed is verdwenen, wat nu?")
	assert.Contains(t, prompt, "https://api.hisarweb.be/v1")
}

func TestHwTelegramChatGuardReplacesALeakingModel(t *testing.T) {
	_, api, secret := hwTelegramChatTestSetup(t, "I am Claude Haiku, made by Anthropic.")
	hwPhoneStepTestTelegramUser(t, "chat-leak@example.com", "710102")

	body := hwTelegramTextUpdate(t, 9202, 710102, "de", "who built you")
	require.Equal(t, http.StatusOK, HwHandleTelegramWebhook(context.Background(), body, secret, time.Now()).HTTPStatus())

	sent := api.messages()
	require.Len(t, sent, 1)
	// The guard answers with the fixed identity, in the fallback language of a
	// non-Turkish, non-Dutch account.
	assert.Equal(t, HwAssistantIdentity["en"], sent[0].Text)
}

func TestHwTelegramChatDayLimit(t *testing.T) {
	gateway, api, secret := hwTelegramChatTestSetup(t, "Answer number one.")
	hwPhoneStepTestTelegramUser(t, "chat-limit@example.com", "710103")
	for index := 0; index < hwTelegramChatDayLimit; index++ {
		body := hwTelegramTextUpdate(t, int64(9300+index), 710103, "en",
			fmt.Sprintf("open letter of the alphabet %d", index))
		require.Equal(t, http.StatusOK, HwHandleTelegramWebhook(context.Background(), body, secret, time.Now()).HTTPStatus())
	}
	assert.Len(t, api.messages(), hwTelegramChatDayLimit)
	assert.Equal(t, hwTelegramChatDayLimit, gateway.count())

	body := hwTelegramTextUpdate(t, 9399, 710103, "en", "one more open letter")
	require.Equal(t, http.StatusOK, HwHandleTelegramWebhook(context.Background(), body, secret, time.Now()).HTTPStatus())

	sent := api.messages()
	require.Len(t, sent, hwTelegramChatDayLimit+1)
	assert.Equal(t, hwTelegramDayLimit("en"), sent[len(sent)-1].Text)
	assert.Equal(t, hwTelegramChatDayLimit, gateway.count(), "a refused question costs no model call")
}

func TestHwTelegramChatTooLongWithoutAModelCall(t *testing.T) {
	gateway, api, secret := hwTelegramChatTestSetup(t, "Answer.")
	hwPhoneStepTestTelegramUser(t, "chat-long@example.com", "710104")

	long := strings.TrimSpace(strings.Repeat("uzun kelime ", 200))
	body := hwTelegramTextUpdate(t, 9401, 710104, "tr", long)
	require.Equal(t, http.StatusOK, HwHandleTelegramWebhook(context.Background(), body, secret, time.Now()).HTTPStatus())

	sent := api.messages()
	require.Len(t, sent, 1)
	assert.Equal(t, hwTelegramTooLong("tr"), sent[0].Text)
	assert.Equal(t, 0, gateway.count())
}
