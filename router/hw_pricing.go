package router

// Public HisarWeb AI routes: the TL price comparator, the quality meter and the
// service status. They are reachable without a login (the page is public) and
// sit behind the public read limit (60 a minute per address, one budget for all
// three) in addition to the global API limit. The critical limiter is not used
// here: mobile carriers in Turkey share one public address between many
// customers, so a strict per-address budget would lock out a whole carrier.
// The status and quality answers take a short public cache; the comparator
// takes none because its answer depends on the query. Starting a quality
// measurement is an admin action.

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetHwRouter(apiRouter *gin.RouterGroup) {
	hwRouter := apiRouter.Group("/hw")
	hwRouter.GET("/pricing/compare", middleware.HwPublicReadRateLimit(), controller.HwPricingCompare)
	hwRouter.GET("/quality/latest", middleware.HwPublicReadRateLimit(), middleware.HwPublicReadCache(), controller.HwQualityLatest)
	// The public service status. No login; the answer never names a supplier.
	hwRouter.GET("/status", middleware.HwPublicReadRateLimit(), middleware.HwPublicReadCache(), controller.HwStatus)
	hwRouter.POST("/quality/run", middleware.AdminAuth(), controller.HwQualityRun)
	// The bureau reserve of the current window (part B2). Admin only; it reports
	// numbers, never the token or the URL of the measurement.
	hwRouter.GET("/admin/reserve", middleware.AdminAuth(), controller.HwAdminReserveStatus)
}
