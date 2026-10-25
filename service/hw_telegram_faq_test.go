package service

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHwTelegramFAQ(t *testing.T) {
	t.Run("a stranger gets the fixed identity answer, never a model name", func(t *testing.T) {
		api, secret := hwTelegramTestSetup(t)
		body := hwTelegramTextUpdate(t, 9101, 710001, "tr", "Sen kimsin?")
		outcome := HwHandleTelegramWebhook(context.Background(), body, secret, time.Now())
		assert.Equal(t, http.StatusOK, outcome.HTTPStatus())
		sent := api.messages()
		require.Len(t, sent, 1)
		assert.Equal(t, HwAssistantIdentity["tr"], sent[0].Text)
		for _, sold := range hwSoldModels {
			assert.NotContains(t, sent[0].Text, sold.Name)
		}
	})

	t.Run("the help command lists every question in the language", func(t *testing.T) {
		api, secret := hwTelegramTestSetup(t)
		body := hwTelegramTextUpdate(t, 9102, 710002, "nl", "/help")
		HwHandleTelegramWebhook(context.Background(), body, secret, time.Now())
		sent := api.messages()
		require.Len(t, sent, 1)
		for _, entry := range hwAssistantFAQs {
			assert.Contains(t, sent[0].Text, entry.Question["nl"])
		}
		assert.True(t, strings.HasSuffix(sent[0].Text, "?"))
	})

	t.Run("a linked customer without a fixed answer is told so", func(t *testing.T) {
		api, secret := hwTelegramTestSetup(t)
		hwPhoneStepTestTelegramUser(t, "faq-linked@example.com", "710003")
		body := hwTelegramTextUpdate(t, 9103, 710003, "en", "my invoice has a strange line")
		HwHandleTelegramWebhook(context.Background(), body, secret, time.Now())
		sent := api.messages()
		require.Len(t, sent, 1)
		assert.Equal(t, hwTelegramNoAnswer("en"), sent[0].Text)
	})

	t.Run("a stranger without a fixed answer stays unanswered", func(t *testing.T) {
		api, secret := hwTelegramTestSetup(t)
		body := hwTelegramTextUpdate(t, 9104, 710004, "en", "random words here")
		HwHandleTelegramWebhook(context.Background(), body, secret, time.Now())
		assert.Empty(t, api.messages())
	})
}
