package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

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
	return engine
}

func hwAssistantPost(engine *gin.Engine, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/ask", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)
	return recorder
}

func TestHwAssistantAskCountsAndReordersFAQ(t *testing.T) {
	engine := hwAssistantTestRouter(t)
	for range 3 {
		reply := hwAssistantPost(engine, `{"question":"How do I use the API?","lang":"en"}`)
		assert.Contains(t, reply.Body.String(), `"matched":true`)
	}
	reply := hwAssistantPost(engine, `{"question":"qwerty zxcv","lang":"en"}`)
	assert.Contains(t, reply.Body.String(), `"matched":false`)
	hwAssistantPost(engine, `{"question":"mail me at a@b.c please","lang":"en"}`)
	rows, err := model.HwAssistantHitsByPrefix(model.DB, "q:", 10)
	require.NoError(t, err)
	require.Len(t, rows, 1, "a question with an address is not stored")
	assert.Equal(t, "qwerty zxcv", rows[0].Key)

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/faq?lang=en", nil))
	assert.Regexp(t, `"faq":\[\{"id":"api"`, recorder.Body.String())
	assert.False(t, strings.Contains(recorder.Body.String(), "—"))
}

func TestHwAssistantAskRejectsEmptyAndLong(t *testing.T) {
	engine := hwAssistantTestRouter(t)
	assert.Contains(t, hwAssistantPost(engine, `{"question":"  "}`).Body.String(), `"success":false`)
	assert.Contains(t, hwAssistantPost(engine, `{"question":"`+strings.Repeat("a", 400)+`"}`).Body.String(), `"success":false`)
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
