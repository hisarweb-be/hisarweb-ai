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
	require.NoError(t, db.AutoMigrate(&model.HwAssistantHit{}))
	model.DB = db
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	engine := gin.New()
	engine.GET("/faq", HwAssistantFAQ)
	engine.POST("/ask", HwAssistantAsk)
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
