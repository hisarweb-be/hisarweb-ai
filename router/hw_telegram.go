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
}
