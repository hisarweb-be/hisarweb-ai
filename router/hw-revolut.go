package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

// SetHwWebhookRoutes registers the anonymous provider webhooks on a group
// without any IP rate limit. Each one authenticates itself (the payment
// signature over the raw body, the Telegram secret header) before any
// expensive work, and the providers retry bursts after an outage; a per-IP
// limiter, the global one included, would drop exactly those retries. The
// body-size limit stays.
func SetHwWebhookRoutes(webhookRouter *gin.RouterGroup) {
	webhookRouter.POST("/hw/revolut/webhook", middleware.AnonymousRequestBodyLimit(), controller.HwRevolutWebhook)
	webhookRouter.POST("/hw/stripe/webhook", middleware.AnonymousRequestBodyLimit(), controller.HwStripeWebhook)
	webhookRouter.POST("/hw/telegram/webhook", middleware.AnonymousRequestBodyLimit(), controller.HwTelegramWebhook)
}

// SetHwRoutes registers the HisarWeb top-up API: the user-facing order endpoint
// behind the normal login and the admin refund route.
func SetHwRoutes(apiRouter *gin.RouterGroup) {

	hwUserRoute := apiRouter.Group("/user/hw")
	hwUserRoute.Use(middleware.DisableCache(), middleware.UserAuth())
	{
		hwUserRoute.POST("/orders", middleware.CriticalRateLimit(), middleware.UserCriticalRateLimit("hw-topup"), controller.HwCreateTopUpOrder)
		// The balance per package with its expiry date, for the top-up page. A
		// read of the caller's own ledger needs no critical rate limit.
		hwUserRoute.GET("/packages", controller.HwListUserPackages)
		// The settled top-ups with the bonus each one earned.
		hwUserRoute.GET("/orders", controller.HwListUserOrders)
	}

	hwAdminRoute := apiRouter.Group("/hw/admin")
	hwAdminRoute.Use(middleware.DisableCache(), middleware.AdminAuth())
	{
		hwAdminRoute.POST("/orders/refund", controller.HwAdminRefundOrder)
	}
}
