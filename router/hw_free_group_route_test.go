package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHwFreeGroupRateLimitCoversTaskAndVideoRoutes proves that the routes that
// start billable work outside the relay families carry the same frozen check as
// a relay call: a task submission, a video generation and a video remix. Each
// request is refused by the guard in front of the handler, so the test does not
// need a task plugin round trip or a supplier.
func TestHwFreeGroupRateLimitCoversTaskAndVideoRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previousDB := model.DB
	previousDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()
	previousSQLitePath := common.SQLitePath
	previousMasterNode := common.IsMasterNode
	previousRedisEnabled := common.RedisEnabled
	common.SQLitePath = t.TempDir() + "/router-hw-free-group.db"
	common.IsMasterNode = false
	common.RedisEnabled = false
	t.Setenv("SQL_DSN", "")
	require.NoError(t, model.InitDB())
	database := model.DB
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.Token{}, &model.HwAccountFlag{}))
	t.Cleanup(func() {
		sqlDB, closeErr := database.DB()
		require.NoError(t, closeErr)
		require.NoError(t, sqlDB.Close())
		model.DB = previousDB
		common.SetDatabaseTypes(previousDatabaseType, previousLogDatabaseType)
		common.SQLitePath = previousSQLitePath
		common.IsMasterNode = previousMasterNode
		common.RedisEnabled = previousRedisEnabled
	})

	require.NoError(t, database.Create(&model.User{
		Id:          91,
		Username:    "hw-free-group-user",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Quota:       100,
		Group:       "free",
		AuthVersion: 1,
	}).Error)
	require.NoError(t, database.Create(&model.Token{
		Id:             1,
		UserId:         91,
		Key:            "hwfreegroup",
		Status:         common.TokenStatusEnabled,
		Name:           "free group",
		ExpiredTime:    -1,
		UnlimitedQuota: true,
	}).Error)

	frozen, err := model.FreezeHwAccounts([]int{91}, model.HwFreezeReasonSharedDevice, common.GetTimestamp())
	require.NoError(t, err)
	require.Equal(t, 1, frozen)

	engine := gin.New()
	SetTaskRouter(engine)
	SetVideoRouter(engine)

	for _, path := range []string{"/v1/tasks/example", "/v1/video/generations", "/v1/videos/vid_1/remix"} {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"any"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer sk-hwfreegroup")
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)

		assert.Equal(t, http.StatusForbidden, recorder.Code, "%s must be refused for a frozen free account", path)
		assert.Contains(t, recorder.Body.String(), "account_frozen", path)
		assert.NotContains(t, recorder.Body.String(), "lumia")
		assert.NotContains(t, recorder.Body.String(), "openrouter")
	}
}
