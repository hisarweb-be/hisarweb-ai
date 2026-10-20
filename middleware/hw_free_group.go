package middleware

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
)

// hwFreeModelsEnv names the comma-separated extra models a free trial account
// may call besides the Haiku class. The default stays empty.
const hwFreeModelsEnv = "HW_FREE_MODELS"

const (
	hwFreeGroupMinuteSeconds = 60
	hwFreeGroupDaySeconds    = 24 * 60 * 60
	// The limiter drops keys that stayed idle longer than its expiration; one
	// hour above the longest window keeps a day counter alive between two
	// requests of the same account.
	hwFreeGroupLimiterExpiration = 25 * time.Hour
)

var (
	hwFreeGroupLimiter     common.InMemoryRateLimiter
	hwFreeGroupLimiterOnce sync.Once
)

// hwRequestGroup is the group the request runs as: the token group when the
// request is authenticated by an API token, otherwise the user group.
func hwRequestGroup(c *gin.Context) string {
	if group := common.GetContextKeyString(c, constant.ContextKeyTokenGroup); group != "" {
		return group
	}
	return common.GetContextKeyString(c, constant.ContextKeyUserGroup)
}

// HwFreeGroupRateLimit caps the request rate of a free trial account per minute
// and per rolling day. The limits are fixed and on by default (see
// setting.HwFreeGroupPerMinuteLimit / HwFreeGroupPerDayLimit), unlike the
// operator switch of the general model rate limit: a trial account must never
// be able to use the trial credit as an unpaid relay, whatever the operator
// forgot to enable. Requests of every other group pass unchanged.
//
// The counters live in this process, like the memory handler of the general
// model rate limit, so a multi-node deployment counts per node. The house runs
// one node; a Redis-backed counter would be the upgrade if that changes.
func HwFreeGroupRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !strings.EqualFold(hwRequestGroup(c), setting.HwFreeGroupName) {
			c.Next()
			return
		}
		userID := c.GetInt("id")
		if userID <= 0 {
			c.Next()
			return
		}
		hwFreeGroupLimiterOnce.Do(func() {
			hwFreeGroupLimiter.Init(hwFreeGroupLimiterExpiration)
		})
		subject := strconv.Itoa(userID)
		minuteLimit := setting.HwFreeGroupPerMinuteLimit()
		if !hwFreeGroupLimiter.Request("HWFreeGroup:minute:"+subject, minuteLimit, hwFreeGroupMinuteSeconds) {
			abortWithOpenAiMessage(c, http.StatusTooManyRequests, i18n.T(c, i18n.MsgRateLimitReached, map[string]any{"Minutes": 1, "Max": minuteLimit}))
			return
		}
		dayLimit := setting.HwFreeGroupPerDayLimit()
		if !hwFreeGroupLimiter.Request("HWFreeGroup:day:"+subject, dayLimit, hwFreeGroupDaySeconds) {
			abortWithOpenAiMessage(c, http.StatusTooManyRequests, i18n.T(c, i18n.MsgRateLimitReached, map[string]any{"Minutes": 24 * 60, "Max": dayLimit}))
			return
		}
		c.Next()
	}
}

// HwFreeGroupModelAllowed reports whether a request of the given group may call
// the model. A free trial account is limited to the Haiku class, the only model
// family the trial credit is priced for; every other group is unrestricted
// here. The operator can add exact model names to the allow-list with
// HW_FREE_MODELS. An empty model name is refused, because nothing was chosen to
// check.
func HwFreeGroupModelAllowed(group, model string) bool {
	if !strings.EqualFold(strings.TrimSpace(group), setting.HwFreeGroupName) {
		return true
	}
	name := strings.ToLower(strings.TrimSpace(model))
	if name == "" {
		return false
	}
	if strings.Contains(name, "haiku") {
		return true
	}
	for allowed := range strings.SplitSeq(strings.ToLower(os.Getenv(hwFreeModelsEnv)), ",") {
		if strings.TrimSpace(allowed) == name {
			return true
		}
	}
	return false
}
