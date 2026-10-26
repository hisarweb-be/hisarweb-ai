package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

// SetHwTelegramRoutes registers the account link routes of the bot. Its
// webhook is registered by SetHwWebhookRoutes, outside the global rate limit.
func SetHwTelegramRoutes(apiRouter *gin.RouterGroup) {
	// The deep link that makes the bot link a Telegram account. It is handed
	// out behind the dashboard login and creates a one-time token, so it is
	// rate limited like the other account-security actions.
	apiRouter.POST("/hw/telegram/link", middleware.UserAuth(), middleware.CriticalRateLimit(), middleware.UserCriticalRateLimit("account-security"), middleware.DisableCache(), controller.HwTelegramLinkStart)
	// How far the customer is in the bot conversation. The link dialog reads
	// it every few seconds while it is open; a read of the caller's own flags
	// needs no critical rate limit.
	apiRouter.GET("/hw/telegram/link", middleware.UserAuth(), middleware.DisableCache(), controller.HwTelegramLinkStatus)
}
