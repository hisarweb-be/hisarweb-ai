package router

// Public HisarWeb AI routes: the TL price comparator and the quality meter.
// Both are reachable without a login (the page is public) and sit behind the
// critical rate limiter in addition to the global API limit. Starting a
// quality measurement is an admin action.

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetHwRouter(apiRouter *gin.RouterGroup) {
	hwRouter := apiRouter.Group("/hw")
	hwRouter.GET("/pricing/compare", middleware.CriticalRateLimit(), controller.HwPricingCompare)
	hwRouter.GET("/quality/latest", middleware.CriticalRateLimit(), controller.HwQualityLatest)
	hwRouter.POST("/quality/run", middleware.AdminAuth(), controller.HwQualityRun)
}
