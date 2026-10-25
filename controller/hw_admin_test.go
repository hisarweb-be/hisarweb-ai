package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
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

// hwAdminTestSetup gives the unfreeze handler its own database with the flag
// table and one frozen common user.
func hwAdminTestSetup(t *testing.T) (frozenUser model.User, admin model.User) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	previousDB := model.DB
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.HwAccountFlag{}, &model.User{}, &model.HwCreditPackage{}, &model.HwCreditExtension{}, &model.HwAdminEscalation{}))

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
	t.Cleanup(func() { model.DB = previousDB })
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

// TestHwAdminAgentEscalate proves the escalation handler against the board and
// the database: a card that exists is reported even when the bookkeeping write
// fails, because a reported failure makes the operator create a second card.
func TestHwAdminAgentEscalate(t *testing.T) {
	_, admin := hwAdminTestSetup(t)
	previousCreateCard := hwAdminAgentCreateCard
	previousResolver := hwAdminAgentCardUpdate
	t.Cleanup(func() {
		hwAdminAgentCreateCard = previousCreateCard
		hwAdminAgentCardUpdate = previousResolver
	})

	boardFail, dropRow, cards := false, false, 0
	hwAdminAgentCreateCard = func(_ context.Context, _ service.HwBureauCardConfig, _, _, _ string) (string, error) {
		if boardFail {
			return "", errors.New("the bureau board could not be reached")
		}
		cards++
		if dropRow {
			// The board answered and the escalation row is gone by the time the
			// card is written back, so the update affects no row.
			require.NoError(t, model.DB.Exec("DELETE FROM hw_admin_escalations").Error)
		}
		return "CARD-" + strconv.Itoa(cards), nil
	}
	// Each escalation writes through the database handle live at request time,
	// as production writes through the one model.DB the process started with.
	hwAdminAgentCardUpdate = func() *gorm.DB { return model.DB }
	escalate := func(body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Set("role", common.RoleAdminUser)
		c.Set("id", admin.Id)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/hw/admin/agent/escalate", strings.NewReader(body))
		HwAdminAgentEscalate(c)
		return recorder
	}
	pendingCount := func(t *testing.T) int64 {
		t.Helper()
		var count int64
		require.NoError(t, model.DB.Model(&model.HwAdminEscalation{}).Where("card_ref = ''").Count(&count).Error)
		return count
	}
	cardOf := func(t *testing.T, id int) string {
		t.Helper()
		var row model.HwAdminEscalation
		require.NoError(t, model.DB.First(&row, id).Error)
		return row.CardRef
	}

	t.Run("a stored card is returned and leaves nothing pending", func(t *testing.T) {
		recorder := escalate(`{"title":"card","summary":"kept"}`)
		require.Equal(t, http.StatusOK, recorder.Code)
		var body struct {
			Success bool `json:"success"`
			Data    struct {
				Card string `json:"card"`
				ID   int    `json:"id"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
		assert.True(t, body.Success)
		assert.Equal(t, "CARD-1", body.Data.Card)
		assert.Equal(t, "CARD-1", cardOf(t, body.Data.ID))
		assert.EqualValues(t, 0, pendingCount(t))
	})

	t.Run("an unreachable board still queues the escalation", func(t *testing.T) {
		boardFail = true
		defer func() { boardFail = false }()
		recorder := escalate(`{"title":"offline","summary":"later"}`)
		assert.Contains(t, recorder.Body.String(), `"queued":true`)
		assert.EqualValues(t, 1, pendingCount(t))
	})

	t.Run("a failed bookkeeping write still reports the card that exists", func(t *testing.T) {
		dropRow = true
		defer func() { dropRow = false }()
		// The board answers, so the card exists; the row is gone before the card
		// is written back, the way a dropped connection or a vanished row behaves.
		recorder := escalate(`{"title":"lost","summary":"the row disappears while the board answers"}`)

		var body struct {
			Success bool `json:"success"`
			Data    struct {
				Card string `json:"card"`
				ID   int    `json:"id"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body), recorder.Body.String())
		assert.True(t, body.Success, "the handler must not report a failure once the card exists")
		assert.Equal(t, "CARD-2", body.Data.Card,
			"the caller gets the card that exists, never an error that invites a second one")
	})
}

func TestHwUpstreamReleases(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamStatus, upstreamCalls := http.StatusOK, 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		w.WriteHeader(upstreamStatus)
		_, _ = w.Write([]byte(`[{"tag_name":"v1.0.0-rc.46","prerelease":true}]`))
	}))
	defer upstream.Close()
	previousURL := hwUpstreamReleasesURL
	hwUpstreamReleasesURL = upstream.URL
	t.Cleanup(func() {
		hwUpstreamReleasesURL = previousURL
		hwUpstreamReleases.expiresAt = time.Time{}
	})
	request := func(role int) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Set("role", role)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/hw/upstream-releases", nil)
		HwUpstreamReleases(c)
		return recorder
	}

	assert.Equal(t, http.StatusForbidden, request(common.RoleCommonUser).Code)
	assert.Equal(t, 0, upstreamCalls)

	first := request(common.RoleAdminUser)
	require.Equal(t, http.StatusOK, first.Code)
	assert.JSONEq(t, `[{"tag_name":"v1.0.0-rc.46","prerelease":true}]`, first.Body.String())
	assert.Equal(t, http.StatusOK, request(common.RoleAdminUser).Code)
	assert.Equal(t, 1, upstreamCalls, "a second check within the hour is served from the cache")

	for _, tc := range []struct{ upstream, want int }{
		{http.StatusForbidden, http.StatusTooManyRequests},
		{http.StatusTooManyRequests, http.StatusTooManyRequests},
		{http.StatusInternalServerError, http.StatusBadGateway},
	} {
		hwUpstreamReleases.expiresAt = time.Time{}
		upstreamStatus = tc.upstream
		assert.Equal(t, tc.want, request(common.RoleAdminUser).Code, "upstream %d", tc.upstream)
	}
}
