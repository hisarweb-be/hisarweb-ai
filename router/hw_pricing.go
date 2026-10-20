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
	// The public service status. No login; the answer never names a supplier.
	hwRouter.GET("/status", middleware.CriticalRateLimit(), controller.HwStatus)
	hwRouter.POST("/quality/run", middleware.AdminAuth(), controller.HwQualityRun)
	// The bureau reserve of the current window (part B2). Admin only; it reports
	// numbers, never the token or the URL of the measurement.
	hwRouter.GET("/admin/reserve", middleware.AdminAuth(), controller.HwAdminReserveStatus)
}
