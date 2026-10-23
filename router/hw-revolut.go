package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

// SetHwRoutes registers the HisarWeb top-up API: the user-facing order endpoint
// behind the normal login, the anonymous payment webhook (whose signature of the
// raw body is its authentication), and the admin refund route.
func SetHwRoutes(apiRouter *gin.RouterGroup) {
	// No IP rate limit here on purpose: the webhook is authenticated by the
	// HMAC signature over the raw body, which is checked before any expensive
	// work, and the provider retries bursts of events after an outage. The
	// shared critical limiter (20 requests per 20 minutes per IP) would drop
	// exactly those retries. The body-size limit stays.
	apiRouter.POST("/hw/revolut/webhook", middleware.AnonymousRequestBodyLimit(), controller.HwRevolutWebhook)
	apiRouter.POST("/hw/stripe/webhook", middleware.AnonymousRequestBodyLimit(), controller.HwStripeWebhook)

	hwUserRoute := apiRouter.Group("/user/hw")
	hwUserRoute.Use(middleware.DisableCache(), middleware.UserAuth())
	{
		hwUserRoute.POST("/orders", middleware.CriticalRateLimit(), middleware.UserCriticalRateLimit("hw-topup"), controller.HwCreateTopUpOrder)
		// The balance per package with its expiry date, for the top-up page. A
		// read of the caller's own ledger needs no critical rate limit.
		hwUserRoute.GET("/packages", controller.HwListUserPackages)
	}

	hwAdminRoute := apiRouter.Group("/hw/admin")
	hwAdminRoute.Use(middleware.DisableCache(), middleware.AdminAuth())
	{
		hwAdminRoute.POST("/orders/refund", controller.HwAdminRefundOrder)
	}
}
