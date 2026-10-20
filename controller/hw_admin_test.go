package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
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

// hwAdminTestSetup gives the unfreeze handler its own database with the flag
// table and one frozen common user.
func hwAdminTestSetup(t *testing.T) (frozenUser model.User, admin model.User) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.HwAccountFlag{}, &model.User{}))

	frozenUser = model.User{
		Username: "hw-frozen-" + common.GetRandomString(6), Password: "x",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
		Group: "default", AuthVersion: 1, AffCode: "aff" + common.GetRandomString(6),
	}
	require.NoError(t, db.Create(&frozenUser).Error)
	admin = model.User{
		Username: "hw-admin-" + common.GetRandomString(6), Password: "x",
		Role: common.RoleAdminUser, Status: common.UserStatusEnabled,
		Group: "default", AuthVersion: 1, AffCode: "aff" + common.GetRandomString(6),
	}
	require.NoError(t, db.Create(&admin).Error)
	frozen, err := model.FreezeHwAccounts([]int{frozenUser.Id}, model.HwFreezeReasonSharedPhone, common.GetTimestamp())
	require.NoError(t, err)
	require.Equal(t, 1, frozen)
	return frozenUser, admin
}

func hwAdminUnfreezeRequest(t *testing.T, role int, adminID int, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("role", role)
	c.Set("id", adminID)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/hw/admin/unfreeze", strings.NewReader(body))
	HwAdminUnfreeze(c)
	return recorder
}

func TestHwAdminUnfreeze(t *testing.T) {
	frozenUser, admin := hwAdminTestSetup(t)

	t.Run("a non-admin is refused with 403", func(t *testing.T) {
		recorder := hwAdminUnfreezeRequest(t, common.RoleCommonUser, admin.Id,
			`{"user_id":`+strconv.Itoa(frozenUser.Id)+`}`)
		assert.Equal(t, http.StatusForbidden, recorder.Code)
		isFrozen, err := model.HwAccountFrozen(frozenUser.Id)
		require.NoError(t, err)
		assert.True(t, isFrozen, "the refusal never thaws anything")
	})

	t.Run("an admin thaws the account", func(t *testing.T) {
		recorder := hwAdminUnfreezeRequest(t, common.RoleAdminUser, admin.Id,
			`{"user_id":`+strconv.Itoa(frozenUser.Id)+`}`)
		assert.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		isFrozen, err := model.HwAccountFrozen(frozenUser.Id)
		require.NoError(t, err)
		assert.False(t, isFrozen)
		var flag model.HwAccountFlag
		require.NoError(t, model.DB.Where("user_id = ?", frozenUser.Id).First(&flag).Error)
		assert.Equal(t, admin.Id, flag.UnfrozenBy)
	})

	t.Run("an invalid body is refused", func(t *testing.T) {
		recorder := hwAdminUnfreezeRequest(t, common.RoleAdminUser, admin.Id, `{"user_id":0}`)
		assert.Equal(t, http.StatusOK, recorder.Code, "API errors answer 200 with success=false")
		assert.Contains(t, recorder.Body.String(), `"success":false`)
	})
}
