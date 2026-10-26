package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHwAdminAgentParseReadsJsonAndFencedJson(t *testing.T) {
	plain := HwAdminAgentParse(`{"reply":"Beter: Kaydet","text_fix":{"lang":"tr","shown":"Sakla","proposed":"Kaydet"},"escalate":null}`)
	assert.Equal(t, "Beter: Kaydet", plain.Reply)
	require.NotNil(t, plain.TextFix)
	assert.Equal(t, "Kaydet", plain.TextFix.Proposed)
	assert.Nil(t, plain.Escalate)

	fenced := HwAdminAgentParse("```json\n{\"reply\":\"Dit is een fout\",\"escalate\":{\"title\":\"Betaling mislukt\",\"summary\":\"500 TRY\"}}\n```")
	assert.Equal(t, "Dit is een fout", fenced.Reply)
	require.NotNil(t, fenced.Escalate)
	assert.Equal(t, "Betaling mislukt", fenced.Escalate.Title)
}

func TestHwAdminAgentParseKeepsAReplyWithoutActionsWhenTheModelIgnoresJson(t *testing.T) {
	answer := HwAdminAgentParse("Gewoon een antwoord zonder JSON.")
	assert.Equal(t, "Gewoon een antwoord zonder JSON.", answer.Reply)
	assert.Nil(t, answer.TextFix)
	assert.Nil(t, answer.Escalate)

	hollow := HwAdminAgentParse(`{"reply":"ok","text_fix":{"lang":"tr","shown":"","proposed":"x"},"escalate":{"title":" ","summary":"s"}}`)
	assert.Nil(t, hollow.TextFix, "a fix without the shown text cannot be applied")
	assert.Nil(t, hollow.Escalate, "a card without a title is dropped")
}

func TestHwAdminAgentValidateBoundsTheConversation(t *testing.T) {
	_, err := HwAdminAgentValidate(nil)
	assert.Error(t, err)
	_, err = HwAdminAgentValidate([]HwAdminAgentTurn{{Role: "system", Content: "x"}})
	assert.Error(t, err, "only user and assistant turns are accepted")
	_, err = HwAdminAgentValidate([]HwAdminAgentTurn{{Role: "user", Content: strings.Repeat("a", HwAdminAgentMaxMessageRunes+1)}})
	assert.Error(t, err)
	_, err = HwAdminAgentValidate([]HwAdminAgentTurn{{Role: "user", Content: "a"}, {Role: "assistant", Content: "b"}})
	assert.Error(t, err, "the last turn must come from the owner")

	long := make([]HwAdminAgentTurn, 0, 30)
	for index := 0; index < 30; index++ {
		long = append(long, HwAdminAgentTurn{Role: "user", Content: "vraag"})
	}
	clean, err := HwAdminAgentValidate(long)
	require.NoError(t, err)
	assert.Len(t, clean, HwAdminAgentMaxTurns, "only the latest turns are sent")
}

func TestHwAdminAgentAskRefusesWithoutGateway(t *testing.T) {
	_, err := HwAdminAgentAsk(context.Background(), HwAdminAgentConfig{}, []HwAdminAgentTurn{{Role: "user", Content: "hallo"}})
	assert.Error(t, err)
}

func TestHwAdminAgentAskSendsTheInstructionInTheFirstUserMessage(t *testing.T) {
	var request hwChatRequest
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = common.DecodeJson(r.Body, &request)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"reply\":\"ok\"}"}}]}`))
	}))
	t.Cleanup(gateway.Close)
	answer, err := HwAdminAgentAsk(context.Background(),
		HwAdminAgentConfig{BaseURL: gateway.URL, APIKey: "k", Model: HwAdminAgentDefaultModel},
		[]HwAdminAgentTurn{{Role: "user", Content: "dit woord is moeilijk"}})
	require.NoError(t, err)
	assert.Equal(t, "ok", answer.Reply)
	require.Len(t, request.Messages, 1)
	assert.Equal(t, "user", request.Messages[0].Role)
	assert.Contains(t, request.Messages[0].Content, "dit woord is moeilijk")
	assert.Contains(t, request.Messages[0].Content, "text_fix")
	assert.Equal(t, hwAdminAgentMaxTokens, request.MaxTokens)
}

func TestHwBureauCreateCardPostsAnIssue(t *testing.T) {
	var path, auth string
	var body hwBureauIssueRequest
	board := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		_ = common.DecodeJson(r.Body, &body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"abc","identifier":"HIS-4242"}`))
	}))
	t.Cleanup(board.Close)

	card, err := HwBureauCreateCard(context.Background(), HwBureauCardConfig{BaseURL: board.URL, CompanyID: "c1", Token: "t"}, "Betaling mislukt", "500 TRY geeft een fout", "/console/topup")
	require.NoError(t, err)
	assert.Equal(t, "HIS-4242", card)
	assert.Equal(t, "/api/companies/c1/issues", path)
	assert.Equal(t, "Bearer t", auth)
	assert.Equal(t, "Admin-agent: Betaling mislukt", body.Title)
	assert.Contains(t, body.Description, "/console/topup")
	assert.Contains(t, body.Description, "500 TRY geeft een fout")
}

func TestHwBureauCreateCardRefusesWhenNotConnectedOrIncomplete(t *testing.T) {
	_, err := HwBureauCreateCard(context.Background(), HwBureauCardConfig{}, "t", "s", "")
	assert.ErrorContains(t, err, "not connected")
	_, err = HwBureauCreateCard(context.Background(), HwBureauCardConfig{BaseURL: "http://x", CompanyID: "c"}, " ", "s", "")
	assert.Error(t, err)

	board := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
	t.Cleanup(board.Close)
	_, err = HwBureauCreateCard(context.Background(), HwBureauCardConfig{BaseURL: board.URL, CompanyID: "c"}, "t", "s", "")
	assert.ErrorContains(t, err, "500")
}

func TestHwBureauCardConfigFromEnvTrimsAndLeavesTheClientToTheDefault(t *testing.T) {
	t.Setenv(HwBureauURLEnv, "  http://10.88.0.1:3100/ ")
	t.Setenv(HwBureauCompanyEnv, " company-1 ")
	t.Setenv(HwBureauTokenEnv, "  ")
	config := HwBureauCardConfigFromEnv()
	assert.Equal(t, "http://10.88.0.1:3100/", config.BaseURL)
	assert.Equal(t, "company-1", config.CompanyID)
	assert.Empty(t, config.Token, "a blank token means header-less calls")
	assert.Nil(t, config.Client)

	t.Setenv(HwBureauURLEnv, "")
	_, err := HwBureauCreateCard(context.Background(), HwBureauCardConfigFromEnv(), "t", "s", "")
	assert.EqualError(t, err, "the bureau board is not connected")
}
