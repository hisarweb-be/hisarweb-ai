package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

// SetHwRoutes registers the HisarWeb top-up API: the user-facing order endpoint
// behind the normal login, and the anonymous payment webhook, whose signature of
// the raw body is its authentication.
func SetHwRoutes(apiRouter *gin.RouterGroup) {
	apiRouter.POST("/hw/revolut/webhook", middleware.CriticalRateLimit(), middleware.AnonymousRequestBodyLimit(), controller.HwRevolutWebhook)

	hwUserRoute := apiRouter.Group("/user/hw")
	hwUserRoute.Use(middleware.DisableCache(), middleware.UserAuth())
	{
		hwUserRoute.POST("/orders", middleware.CriticalRateLimit(), middleware.UserCriticalRateLimit("hw-topup"), controller.HwCreateTopUpOrder)
	}
}
