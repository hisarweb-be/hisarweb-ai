package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func hwAssistantTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.HwAssistantHit{}, &model.HwAssistantFeedback{}))
	model.DB = db
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	engine := gin.New()
	engine.GET("/faq", HwAssistantFAQ)
	engine.POST("/ask", HwAssistantAsk)
	engine.POST("/feedback", HwAssistantFeedback)
	engine.POST("/chat", HwAssistantChat)
	return engine
}

func hwAssistantPost(engine *gin.Engine, body string) *httptest.ResponseRecorder {
	return hwAssistantPostTo(engine, "/ask", body)
}

func hwAssistantPostTo(engine *gin.Engine, path, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)
	return recorder
}

func TestHwAssistantAskCountsAndReordersFAQ(t *testing.T) {
	engine := hwAssistantTestRouter(t)
	for range 3 {
		reply := hwAssistantPost(engine, `{"id":"api","lang":"en"}`)
		assert.Contains(t, reply.Body.String(), `"matched":true`)
	}
	assert.Contains(t, hwAssistantPost(engine, `{"id":"no-such-chip","lang":"en"}`).Body.String(), `"matched":false`)
	rows, err := model.HwAssistantHitsByPrefix(model.DB, "faq:", 10)
	require.NoError(t, err)
	require.Len(t, rows, 1, "an unknown id is not counted")
	assert.Equal(t, "api", rows[0].Key)

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/faq?lang=en", nil))
	assert.Regexp(t, `"faq":\[\{"id":"api"`, recorder.Body.String())
	assert.False(t, strings.Contains(recorder.Body.String(), "—"))
}

// Free text never gets a keyword answer from the ask endpoint any more: the
// old matcher answered "codex" or "api" with a fixed text that missed the
// question; such text now goes to the model through /chat.
func TestHwAssistantAskAnswersChipIDsOnly(t *testing.T) {
	engine := hwAssistantTestRouter(t)
	for _, body := range []string{
		`{"question":"codex","lang":"tr"}`,
		`{"question":"How do I use the API?","lang":"en"}`,
		`{"question":"kimsin","lang":"tr"}`,
		`{"id":"codex nasil kurulur","lang":"tr"}`,
		`{"id":"","lang":"tr"}`,
	} {
		reply := hwAssistantPost(engine, body).Body.String()
		assert.Contains(t, reply, `"matched":false`, body)
		assert.NotContains(t, reply, `"answer"`, body)
	}
	assert.Contains(t, hwAssistantPost(engine, `not json`).Body.String(), `"success":false`)
}

// hwAssistantFakeChat replaces the model call and records what it was given.
func hwAssistantFakeChat(t *testing.T, reply string) *[][]service.HwAdminAgentTurn {
	t.Helper()
	calls := &[][]service.HwAdminAgentTurn{}
	previous := hwAssistantChatCall
	hwAssistantChatCall = func(_ context.Context, _ service.HwAdminAgentConfig, channel, _ string, turns []service.HwAdminAgentTurn) (string, bool) {
		assert.Equal(t, service.HwAssistantChannelWidget, channel)
		*calls = append(*calls, turns)
		return reply, true
	}
	t.Cleanup(func() { hwAssistantChatCall = previous })
	return calls
}

type hwAssistantChatReply struct {
	Success bool `json:"success"`
	Data    struct {
		Reply   string `json:"reply"`
		Limited bool   `json:"limited"`
	} `json:"data"`
}

func hwAssistantChat(t *testing.T, engine *gin.Engine, body string) hwAssistantChatReply {
	t.Helper()
	var reply hwAssistantChatReply
	recorder := hwAssistantPostTo(engine, "/chat", body)
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &reply), recorder.Body.String())
	return reply
}

func TestHwAssistantChatAnswersFreeTextThroughTheModel(t *testing.T) {
	engine := hwAssistantTestRouter(t)
	calls := hwAssistantFakeChat(t, "Zet ANTHROPIC_BASE_URL. Lukt het?")

	reply := hwAssistantChat(t, engine, `{"lang":"nl","messages":[{"role":"user","content":"Hoe installeer ik Claude Code?","attachments":[{"name":"a.txt","mime":"text/plain","data":"geheim"}]}]}`)
	require.True(t, reply.Success)
	assert.Equal(t, "Zet ANTHROPIC_BASE_URL. Lukt het?", reply.Data.Reply)
	assert.False(t, reply.Data.Limited)
	require.Len(t, *calls, 1)
	require.Len(t, (*calls)[0], 1)
	assert.Empty(t, (*calls)[0][0].Attachments, "the public widget sends text only")

	for _, body := range []string{`not json`, `{"messages":[]}`, `{"messages":[{"role":"system","content":"x"}]}`, `{"messages":[{"role":"user","content":"a"},{"role":"assistant","content":"b"}]}`} {
		assert.False(t, hwAssistantChat(t, engine, body).Success, body)
	}
	assert.Len(t, *calls, 1, "an invalid conversation never reaches the model")
}

func TestHwAssistantChatStopsAtTheDayLimit(t *testing.T) {
	engine := hwAssistantTestRouter(t)
	calls := hwAssistantFakeChat(t, "antwoord")
	body := `{"lang":"tr","messages":[{"role":"user","content":"Claude Code nasıl kurulur?"}]}`
	for index := 0; index < hwAssistantChatDayLimit; index++ {
		require.False(t, hwAssistantChat(t, engine, body).Data.Limited, index)
	}
	reply := hwAssistantChat(t, engine, body)
	require.True(t, reply.Success)
	assert.True(t, reply.Data.Limited)
	assert.Equal(t, service.HwAssistantLimitedReply("tr", "Claude Code nasıl kurulur?"), reply.Data.Reply)
	assert.Len(t, *calls, hwAssistantChatDayLimit, "over the limit no model call is made")
}

func TestHwAssistantChatStopsAtTheGlobalLimit(t *testing.T) {
	engine := hwAssistantTestRouter(t)
	calls := hwAssistantFakeChat(t, "answer")
	day := time.Unix(common.GetTimestamp(), 0).UTC().Format("2006-01-02")
	require.NoError(t, model.DB.Create(&model.HwAssistantHit{Key: "hw:wchat:all:" + day, Hits: hwAssistantChatDayLimitAll}).Error)
	reply := hwAssistantChat(t, engine, `{"lang":"en","messages":[{"role":"user","content":"How do I set up Codex?"}]}`)
	assert.True(t, reply.Data.Limited)
	assert.Empty(t, *calls)
}

// TestHwAssistantOpenQuestions proves only an admin sees which questions had no
// fixed answer, most asked first, without the internal key prefix, and that the
// answered (faq) counters never leak into that list.
func TestHwAssistantOpenQuestions(t *testing.T) {
	hwAssistantTestRouter(t)
	for _, hit := range []struct {
		key  string
		hits int
	}{{"q:how to refund", 1}, {"q:which models", 3}, {"faq:price", 9}} {
		for range hit.hits {
			require.NoError(t, model.HwAssistantRecordHit(model.DB, hit.key, 1000))
		}
	}
	serve := func(role int) (int, string) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Set("role", role)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/hw/assistant/open", nil)
		HwAssistantOpenQuestions(c)
		return recorder.Code, recorder.Body.String()
	}

	code, _ := serve(common.RoleCommonUser)
	assert.Equal(t, http.StatusForbidden, code)

	code, body := serve(common.RoleAdminUser)
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, body, "which models")
	assert.Contains(t, body, "how to refund")
	assert.NotContains(t, body, "price", "an answered question is not an open one")
	assert.NotContains(t, body, "q:", "the prefix is internal")
	assert.Less(t, strings.Index(body, "which models"), strings.Index(body, "how to refund"), "most asked first")
}

func TestHwAssistantFeedbackStoresAndSkipsPersonal(t *testing.T) {
	engine := hwAssistantTestRouter(t)
	post := func(body string) string {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/feedback", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(recorder, request)
		return recorder.Body.String()
	}
	assert.Contains(t, post(`{"question":"how do I get a key","answer":"Create a token.","rating":1,"lang":"en","correction":"ignored"}`), `"stored":true`)
	assert.Contains(t, post(`{"question":"mail me at a@b.nl","answer":"ok","rating":-1,"lang":"en"}`), `"stored":false`)
	assert.Contains(t, post(`{"question":"q","answer":"a","rating":0}`), `"success":false`)
	assert.Contains(t, post(`{"question":"","answer":"a","rating":1}`), `"success":false`)

	rows, err := model.HwAssistantRecentFeedback(model.DB, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "widget", rows[0].Channel)
	assert.Equal(t, 1, rows[0].Rating)
	assert.Empty(t, rows[0].Correction, "the public endpoint ignores a correction")
}
