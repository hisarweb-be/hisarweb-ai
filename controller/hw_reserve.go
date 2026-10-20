package controller

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// hwReserveAdminView is the administrator's picture of the reserve of the
// current window. It holds the numbers that decide whether customer traffic is
// open, and nothing else: the URL and the token of the bureau measurement are
// never part of it.
type hwReserveAdminView struct {
	WindowStart            int64   `json:"window_start"`
	WindowEnd              int64   `json:"window_end"`
	Mode                   string  `json:"mode"`
	Phase0MeasurementDone  bool    `json:"phase0_measurement_done"`
	CapacityUSD            float64 `json:"capacity_usd"`
	BureauSourceConfigured bool    `json:"bureau_source_configured"`
	BureauUsedUSD          float64 `json:"bureau_used_usd"`
	CustomerUsedUSD        float64 `json:"customer_used_usd"`
	CustomerLimitUSD       float64 `json:"customer_limit_usd"`
	CeilingPaidUSD         float64 `json:"ceiling_paid_usd"`
	CeilingFreeUSD         float64 `json:"ceiling_free_usd"`
	DenyReason             string  `json:"deny_reason"`
	RetryAfterSeconds      int     `json:"retry_after_seconds"`
	MeasuredAt             int64   `json:"measured_at"`
}

// HwAdminReserveStatus reports the reserve of the current window: what the
// bureau and the customers used, the customer limit, where the measurement came
// from and when it was taken. Customer traffic is refused while deny_reason is
// not empty.
func HwAdminReserveStatus(c *gin.Context) {
	snapshot := service.HwBureauReserveSnapshot(c.Request.Context(), time.Now().UTC())
	common.ApiSuccess(c, hwReserveAdminView{
		WindowStart:            snapshot.WindowStart,
		WindowEnd:              snapshot.WindowEnd,
		Mode:                   snapshot.Mode,
		Phase0MeasurementDone:  snapshot.Phase0MeasurementDone,
		CapacityUSD:            snapshot.CapacityUSD,
		BureauSourceConfigured: snapshot.BureauSourceConfigured,
		BureauUsedUSD:          snapshot.BureauUsedUSD,
		CustomerUsedUSD:        snapshot.CustomerUsedUSD,
		CustomerLimitUSD:       snapshot.CustomerLimitUSD,
		CeilingPaidUSD:         service.HwCustomerCeilingUSD(snapshot.CapacityUSD, model.HwPaidGroupName),
		CeilingFreeUSD:         service.HwCustomerCeilingUSD(snapshot.CapacityUSD, model.HwFreeGroupName),
		DenyReason:             snapshot.DenyReason,
		RetryAfterSeconds:      snapshot.RetryAfterSeconds,
		MeasuredAt:             snapshot.MeasuredAt,
	})
}
