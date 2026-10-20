package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hwFreeGroupTestRouter serves a request as one user in one group, so the rate
// limiter sees the group and the id that TokenAuth would normally set.
func hwFreeGroupTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	router := gin.New()
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		c.Set("id", hwFreeGroupTestUserID(c))
		c.Set(string(constant.ContextKeyTokenGroup), c.Query("group"))
	}, HwFreeGroupRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return router
}

// hwFreeGroupTestUserID reads the user of the request from the path query, so
// every case of a table test can use its own counter.
func hwFreeGroupTestUserID(c *gin.Context) int {
	id, _ := strconv.Atoi(c.Query("user"))
	return id
}

func TestHwFreeGroupRateLimitCapsFreeAccounts(t *testing.T) {
	t.Setenv("HW_FREE_GROUP_PER_MINUTE", "2")
	t.Setenv("HW_FREE_GROUP_PER_DAY", "10")
	router := hwFreeGroupTestRouter(t)

	call := func(group string, userID int) int {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?user="+strconv.Itoa(userID)+"&group="+group, nil)
		router.ServeHTTP(recorder, request)
		return recorder.Code
	}

	assert.Equal(t, http.StatusOK, call("free", 4101))
	assert.Equal(t, http.StatusOK, call("free", 4101))
	assert.Equal(t, http.StatusTooManyRequests, call("free", 4101), "the third request in a minute is refused")
	assert.Equal(t, http.StatusOK, call("free", 4102), "another account has its own budget")
	for range 3 {
		assert.Equal(t, http.StatusOK, call("paid", 4101), "a paid account is not capped here")
	}
}

func TestHwFreeGroupRateLimitCapsTheDay(t *testing.T) {
	t.Setenv("HW_FREE_GROUP_PER_MINUTE", "100")
	t.Setenv("HW_FREE_GROUP_PER_DAY", "1")
	router := hwFreeGroupTestRouter(t)

	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/v1/chat/completions?user=4201&group=free", nil))
	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/v1/chat/completions?user=4201&group=free", nil))

	assert.Equal(t, http.StatusOK, first.Code)
	assert.Equal(t, http.StatusTooManyRequests, second.Code, "the daily budget ends the day")
}

func TestHwFreeGroupModelAllowed(t *testing.T) {
	t.Setenv("HW_FREE_MODELS", " our-extra-model ")
	cases := []struct {
		name  string
		group string
		model string
		want  bool
	}{
		{name: "free may call haiku", group: "free", model: "claude-haiku-4-5", want: true},
		{name: "free may call haiku in another spelling", group: "free", model: "Claude-Haiku-4-5-Latest", want: true},
		{name: "free may not call sonnet", group: "free", model: "claude-sonnet-5-5", want: false},
		{name: "free may not call opus", group: "free", model: "claude-opus-5", want: false},
		{name: "free may not call an unrelated model", group: "free", model: "gpt-5", want: false},
		{name: "free may not call an empty model", group: "free", model: "", want: false},
		{name: "the allow-list admits an exact extra model", group: "free", model: "our-extra-model", want: true},
		{name: "paid may call sonnet", group: "paid", model: "claude-sonnet-5-5", want: true},
		{name: "default group may call opus", group: "default", model: "claude-opus-5", want: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.want, HwFreeGroupModelAllowed(testCase.group, testCase.model))
		})
	}
}
