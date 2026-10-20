package router

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// hwRouteTestEnv is a gateway built from the real relay router, so a test proves
// the route registrations themselves carry the supplier mask.
type hwRouteTestEnv struct {
	engine  *gin.Engine
	db      *gorm.DB
	token   *model.Token
	session *service.AuthBundle
	// upstream answers every relay request and deliberately leaks supplier
	// traces in headers and body.
	upstream *httptest.Server
}

func newHwRouteTestEnv(t *testing.T) *hwRouteTestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())

	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedis := common.RedisEnabled
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousMemoryCache := common.MemoryCacheEnabled
	previousMaster := common.IsMasterNode
	previousSQLitePath := common.SQLitePath
	previousSessionSecret := common.SessionSecret

	common.MemoryCacheEnabled = false
	common.RedisEnabled = false
	common.IsMasterNode = false
	common.SQLitePath = filepath.Join(t.TempDir(), "hw-routes.db")
	common.SessionSecret = "hw-route-test-session-secret"
	t.Setenv("SQL_DSN", "")
	t.Setenv("LOG_SQL_DSN", "")

	require.NoError(t, model.InitDB())
	require.NoError(t, model.InitLogDB())
	database := model.DB
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{},
		&model.Ability{}, &model.Log{}, &model.AuditLog{}, &model.Option{}, &model.UserSession{},
		&model.UserSubscription{}))

	previousModelRatios := ratio_setting.ModelRatio2JSONString()
	t.Cleanup(func() {
		sqlDB.Close()
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedis
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.MemoryCacheEnabled = previousMemoryCache
		common.IsMasterNode = previousMaster
		common.SQLitePath = previousSQLitePath
		common.SessionSecret = previousSessionSecret
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousModelRatios))
	})
	modelRatios := ratio_setting.GetModelRatioCopy()
	for _, name := range []string{"gpt-4o-mini", "gemini-2.0-flash"} {
		modelRatios[name] = 1
	}
	encoded, err := common.Marshal(modelRatios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(encoded)))

	env := &hwRouteTestEnv{db: database, engine: gin.New()}
	env.startUpstream(t)

	user := &model.User{
		Username:    "hw-route-user",
		Password:    "unused",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		Quota:       1 << 30,
		AuthVersion: 1,
	}
	require.NoError(t, database.Create(user).Error)
	env.token = &model.Token{
		UserId:         user.Id,
		Key:            "hwrouteskey",
		Status:         common.TokenStatusEnabled,
		ExpiredTime:    -1,
		UnlimitedQuota: true,
	}
	require.NoError(t, database.Create(env.token).Error)
	env.session, err = service.CreateLoginSession(user.Id, "password", "127.0.0.1", "hw-route-test")
	require.NoError(t, err)

	env.addChannel(t, "hw-openai", constant.ChannelTypeOpenAI, "gpt-4o-mini")
	env.addChannel(t, "hw-gemini", constant.ChannelTypeGemini, "gemini-2.0-flash")

	SetRelayRouter(env.engine)
	return env
}

// startUpstream runs one double that answers both wire formats and leaks a
// supplier name in three headers and in the body.
func (e *hwRouteTestEnv) startUpstream(t *testing.T) {
	t.Helper()
	e.upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-OpenRouter-Id", "gen-lumia-id")
		w.Header().Set("X-Powered-By", "lumia")
		w.Header().Set("Server", "openrouter-edge")
		w.Header().Set("X-Provider-Slug", "lumia")
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, ":generateContent") {
			_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"hello from the supplier"}]}}],` +
				`"modelVersion":"lumia-model","usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"gen-lumia-id","object":"chat.completion","created":1700000000,` +
			`"model":"lumia-model","provider":"lumia","choices":[{"index":0,` +
			`"message":{"role":"assistant","content":"hello from the supplier"},"finish_reason":"stop"}],` +
			`"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))
	}))
	t.Cleanup(e.upstream.Close)
}

// addChannel registers an untagged channel, so the request follows the ordinary
// distribution path and the test measures the mask, not the supplier routing.
func (e *hwRouteTestEnv) addChannel(t *testing.T, name string, channelType int, modelName string) {
	t.Helper()
	baseURL := e.upstream.URL
	priority := int64(1)
	weight := uint(1)
	channel := &model.Channel{
		Name:     name,
		Type:     channelType,
		Key:      "sk-fake-route",
		Status:   common.ChannelStatusEnabled,
		BaseURL:  &baseURL,
		Group:    "default",
		Models:   modelName,
		Priority: &priority,
		Weight:   &weight,
		AutoBan:  common.GetPointer(0),
	}
	require.NoError(t, e.db.Create(channel).Error)
	require.NoError(t, e.db.Create(&model.Ability{
		Group:     "default",
		Model:     modelName,
		ChannelId: channel.Id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
}

// serve sends one request through the real router and returns the recorder.
func (e *hwRouteTestEnv) serve(t *testing.T, path string, payload map[string]any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := common.Marshal(payload)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	e.engine.ServeHTTP(recorder, request)
	return recorder
}

func hwAssertNoSupplierTrace(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	assert.NotEqual(t, http.StatusNotFound, recorder.Code)
	assert.Empty(t, recorder.Header().Get("X-OpenRouter-Id"))
	assert.Empty(t, recorder.Header().Get("X-Provider-Slug"))
	body := strings.ToLower(recorder.Body.String())
	assert.NotContains(t, body, "lumia")
	assert.NotContains(t, body, "openrouter")
	assert.NotContains(t, body, "gen-")
	assert.Contains(t, body, "hello from the supplier")
}

// TestHwMaskCoversPlaygroundAndGeminiRoutes proves that the playground route and
// the Gemini route, which used to relay without the mask, no longer hand the
// supplier headers or supplier words to the customer.
func TestHwMaskCoversPlaygroundAndGeminiRoutes(t *testing.T) {
	env := newHwRouteTestEnv(t)

	t.Run("playground chat completions", func(t *testing.T) {
		recorder := env.serve(t, "/pg/chat/completions", map[string]any{
			"model":    "gpt-4o-mini",
			"messages": []map[string]any{{"role": "user", "content": "hi"}},
		}, map[string]string{
			"Authorization": "Bearer " + env.session.AccessToken,
		})
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		hwAssertNoSupplierTrace(t, recorder)
	})

	t.Run("gemini generate content", func(t *testing.T) {
		recorder := env.serve(t, "/v1beta/models/gemini-2.0-flash:generateContent", map[string]any{
			"contents": []map[string]any{{"parts": []map[string]any{{"text": "hi"}}}},
		}, map[string]string{
			"Authorization": "Bearer " + env.token.Key,
		})
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		hwAssertNoSupplierTrace(t, recorder)
	})
}
