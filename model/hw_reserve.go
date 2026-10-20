package model

import (
	"time"

	"github.com/QuantumNous/new-api/common"
)

// HisarWeb AI bureau reserve: we resell unused room of one shared subscription,
// and the bureau's own traffic always goes first. The unit of the reserve is a
// five-hour window that starts at 00, 05, 10, 15 and 20 UTC; the customer share
// of a window is derived from what the bureau used in it, measured against the
// cost of the subscription capacity in that window.
//
// Only the window and the log aggregation live here. The rule itself, the
// source of the bureau usage and the gate sit in service/ and middleware/.
const (
	// HwReserveWindowSeconds is the length of a reserve window. The windows of
	// a day start at 00, 05, 10, 15 and 20 UTC; because five of them do not fit
	// in a day, the last one runs from 20:00 to midnight and is an hour shorter.
	HwReserveWindowSeconds int64 = 5 * 3600

	// HwReserveLastWindowSeconds is the short last window of a day.
	HwReserveLastWindowSeconds int64 = 4 * 3600

	// HwWindowCapacityUSDOption is the cost of the shared capacity per window,
	// in US dollars. Empty or zero means "unknown", and the gate then refuses
	// customer traffic instead of guessing.
	HwWindowCapacityUSDOption = "HwWindowCapacityUSD"

	// HwPhase0MeasurementDoneOption switches the rule from the pre-measurement
	// one-third share to "the rest minus a margin". It stays false until the
	// operator has actually measured the bureau usage.
	HwPhase0MeasurementDoneOption = "HwPhase0MeasurementDone"

	// HwCustomerWindowCeilingUSDOption is the per-customer ceiling of one
	// window. Empty or zero derives it from the capacity.
	HwCustomerWindowCeilingUSDOption = "HwCustomerWindowCeilingUSD"
)

// Environment names of the reserve configuration. They are documented in
// .env.example without values; the URL and token of the bureau usage endpoint
// are never logged.
const (
	HwWindowCapacityUSDEnv = "HW_WINDOW_CAPACITY_USD"
	HwBureauUsageURLEnv    = "HW_BUREAU_USAGE_URL"
	HwBureauUsageTokenEnv  = "HW_BUREAU_USAGE_TOKEN"
	HwReserveModeEnv       = "HW_RESERVE_MODE"
)

// HwReserveWindow returns the reserve window that contains now, as Unix
// seconds: [windowStart, windowEnd). Windows restart every day at 00 UTC and
// start at 00, 05, 10, 15 and 20; the last one ends at the next midnight. The
// computation is done in UTC and does not depend on the server timezone.
func HwReserveWindow(now time.Time) (windowStart int64, windowEnd int64) {
	utc := now.UTC()
	dayStart := time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC).Unix()
	slot := int64(utc.Hour()) / 5
	windowStart = dayStart + slot*HwReserveWindowSeconds
	if slot == 4 {
		return windowStart, dayStart + 24*3600
	}
	return windowStart, windowStart + HwReserveWindowSeconds
}

// Group names of the two customer kinds, published so the reserve rule in
// service/ does not repeat the strings.
const (
	HwFreeGroupName = hwGroupFree
	HwPaidGroupName = hwGroupPaid
)

// HwCustomerGroupNames lists the groups whose traffic the reserve gate limits.
// Every other group (default, operator groups, internal bureau accounts) is
// left alone.
func HwCustomerGroupNames() []string {
	return []string{HwFreeGroupName, HwPaidGroupName}
}

// HwIsCustomerGroup reports whether group belongs to a paying or trial
// customer and therefore falls under the reserve gate.
func HwIsCustomerGroup(group string) bool {
	return group == HwFreeGroupName || group == HwPaidGroupName
}

// HwCustomerWindowQuota sums the consumed quota of all customer traffic in one
// window. It reuses the consume rows of the log table, so there is no second
// counter that could drift from the billing records. The query goes through
// GORM and quotes the group column per dialect, so it also works on MySQL,
// PostgreSQL and ClickHouse.
func HwCustomerWindowQuota(windowStart int64, windowEnd int64) (int64, error) {
	var row struct {
		Total int64
	}
	err := LOG_DB.Table("logs").
		Select("COALESCE(SUM(quota), 0) AS total").
		Where("type = ?", LogTypeConsume).
		Where("created_at >= ? AND created_at < ?", windowStart, windowEnd).
		Where(logGroupCol+" IN ?", HwCustomerGroupNames()).
		Scan(&row).Error
	return row.Total, err
}

// HwUserWindowQuota sums the consumed quota of one customer in one window, for
// the per-customer ceiling.
func HwUserWindowQuota(userID int, windowStart int64, windowEnd int64) (int64, error) {
	var row struct {
		Total int64
	}
	err := LOG_DB.Table("logs").
		Select("COALESCE(SUM(quota), 0) AS total").
		Where("type = ?", LogTypeConsume).
		Where("user_id = ?", userID).
		Where("created_at >= ? AND created_at < ?", windowStart, windowEnd).
		Where(logGroupCol+" IN ?", HwCustomerGroupNames()).
		Scan(&row).Error
	return row.Total, err
}

// HwQuotaToUSD converts a quota amount to its dollar value at the configured
// rate. Callers must check HwQuotaRateValid first: without a rate the reserve
// cannot compare usage with a dollar capacity, and the gate refuses instead of
// treating every amount as zero.
func HwQuotaToUSD(quota int64) float64 {
	if common.QuotaPerUnit <= 0 {
		return 0
	}
	return float64(quota) / common.QuotaPerUnit
}

// HwQuotaRateValid reports whether the configured quota rate can convert
// between usage and dollars.
func HwQuotaRateValid() bool {
	return common.QuotaPerUnit > 0
}
