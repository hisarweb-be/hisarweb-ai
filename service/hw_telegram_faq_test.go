package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHwTelegramFAQ(t *testing.T) {
	t.Run("the help command lists every question in the language", func(t *testing.T) {
		_, api, secret := hwTelegramChatTestSetup(t, "")
		hwTelegramPost(t, secret, hwTelegramTextUpdate(t, 9102, 710002, "nl", "/help"))
		sent := api.messages(t)
		require.Len(t, sent, 1)
		for _, entry := range hwAssistantFAQs {
			assert.Contains(t, sent[0].Text, entry.Question["nl"])
		}
		assert.True(t, strings.HasSuffix(sent[0].Text, "?"))
	})

	// Without a gateway the fixed answers are the fallback: in the language of
	// the question, never with a model name, and without thumbs.
	cases := []struct {
		name   string
		linked bool
		lang   string
		text   string
		want   string
	}{
		{"a stranger gets the fixed identity answer", false, "tr", "Sen kimsin?", HwAssistantIdentity["tr"]},
		{"a Turkish question gets the Turkish answer from an English account", false, "en", "sen kimsin", HwAssistantIdentity["tr"]},
		{"a linked customer without a fixed answer is told the answer is delayed", true, "en", "my invoice has a strange line",
			HwAssistantFallback(HwAssistantChannelTelegram, "en", "my invoice has a strange line")},
		{"a stranger without a fixed answer is told the answer is delayed", false, "en", "random words here",
			HwAssistantFallback(HwAssistantChannelTelegram, "en", "random words here")},
	}
	for index, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, api, secret := hwTelegramChatTestSetup(t, "")
			if tc.linked {
				hwPhoneStepTestTelegramUser(t, "faq-linked@example.com", "710003")
			}
			hwTelegramPost(t, secret, hwTelegramTextUpdate(t, int64(9103+index), 710003, tc.lang, tc.text))
			sent := api.messages(t)
			require.Len(t, sent, 1)
			assert.Equal(t, tc.want, sent[0].Text)
			assert.Nil(t, sent[0].ReplyMarkup)
			for _, sold := range hwSoldModels {
				assert.NotContains(t, sent[0].Text, sold.Name)
			}
		})
	}
}
