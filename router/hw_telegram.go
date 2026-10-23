package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

// SetHwTelegramRoutes registers the bot webhook. The route is anonymous: the
// secret header that the operator registered with setWebhook authenticates the
// caller, and the handler refuses every update without it. No IP rate limit:
// Telegram retries bursts of updates after a timeout and would otherwise be
// dropped exactly when the bot is catching up. The body-size limit stays.
func SetHwTelegramRoutes(apiRouter *gin.RouterGroup) {
	apiRouter.POST("/hw/telegram/webhook", middleware.AnonymousRequestBodyLimit(), controller.HwTelegramWebhook)
	// The deep link that makes the bot link a Telegram account. It is handed
	// out behind the dashboard login and creates a one-time token, so it is
	// rate limited like the other account-security actions.
	apiRouter.POST("/hw/telegram/link", middleware.UserAuth(), middleware.CriticalRateLimit(), middleware.UserCriticalRateLimit("account-security"), middleware.DisableCache(), controller.HwTelegramLinkStart)
	// How far the customer is in the bot conversation. The link dialog reads
	// it every few seconds while it is open; a read of the caller's own flags
	// needs no critical rate limit.
	apiRouter.GET("/hw/telegram/link", middleware.UserAuth(), middleware.DisableCache(), controller.HwTelegramLinkStatus)
}
