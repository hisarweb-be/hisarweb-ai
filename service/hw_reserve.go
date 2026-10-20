package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// HisarWeb AI bureau reserve. We sell the unused room of one shared
// subscription; the bureau itself always goes first. This file holds the rule
// and the measurement of the bureau usage. The gate that applies it to customer
// requests sits in middleware/hw_reserve.go, the window and the log
// aggregation in model/hw_reserve.go.
//
// Every failure mode falls back to the strictest rule, never to "everything
// allowed": an unknown capacity or an unreachable measurement refuses customer
// requests with "busy" instead of opening the window.
const (
	// hwReserveMeasuredMarginShare is the part of the capacity kept free after
	// the phase-0 measurement. It absorbs the requests that were already running
	// when the window was measured.
	hwReserveMeasuredMarginShare = 0.2

	// hwReserveCeilingCapacityShare derives the default per-customer ceiling from
	// the window capacity: 1/20 of the window.
	hwReserveCeilingCapacityShare = 20

	// hwReserveFallbackCeilingUSD is the per-customer ceiling while the capacity
	// is unknown. It still bounds a single customer, even though the gate refuses
	// customer traffic in that state.
	hwReserveFallbackCeilingUSD = 5

	// hwReserveFreeCeilingShare makes the free trial group's window ceiling lower
	// than the paid one.
	hwReserveFreeCeilingShare = 0.5

	hwBureauUsageTimeout  = 3 * time.Second
	hwBureauUsageCacheTTL = 60 * time.Second
	hwBureauUsageMaxBody  = 1 << 20
)

// Reserve modes. HW_RESERVE_MODE=third pins the strict pre-measurement rule, so
// an operator can run the reserve without the bureau measurement; every other
// value (and an unknown one) follows the phase-0 flag, which cannot lift the
// one-third rule before the measurement is done.
const (
	HwReserveModeAuto  = "auto"
	HwReserveModeThird = "third"
)

// Reasons a snapshot refuses customer traffic with. They are fixed words that
// carry no supplier or customer detail and are safe in logs and the admin view.
const (
	HwReserveDenyCapacityUnknown   = "capacity_unknown"
	HwReserveDenySourceUnavailable = "bureau_source_unavailable"
	HwReserveDenyLimitReached      = "limit_reached"
	HwReserveDenyQuotaRateInvalid  = "quota_rate_invalid"
	HwReserveDenyUsageUnavailable  = "usage_unavailable"
	HwReserveDenyCeilingReached    = "ceiling_reached"
)

// HwBureauUsageSource measures what the bureau itself consumed in one window.
// The production implementation reads an HTTP JSON endpoint; tests replace it
// with a fake.
type HwBureauUsageSource interface {
	UsedUSD(ctx context.Context, windowStart int64, windowEnd int64) (float64, error)
}

// hwBureauUsageHTTP reads the bureau usage from the configured endpoint. It
// caches one window for a minute, so a burst of customer requests does not
// become a burst of measurements. The URL and token never leave this struct.
type hwBureauUsageHTTP struct {
	url    string
	token  string
	client *http.Client

	mu          sync.Mutex
	cacheWindow int64
	cacheValue  float64
	cacheErr    error
	cacheAt     time.Time
}

func (source *hwBureauUsageHTTP) UsedUSD(ctx context.Context, windowStart int64, windowEnd int64) (float64, error) {
	source.mu.Lock()
	if source.cacheWindow == windowStart && time.Since(source.cacheAt) < hwBureauUsageCacheTTL {
		value, err := source.cacheValue, source.cacheErr
		source.mu.Unlock()
		return value, err
	}
	source.mu.Unlock()

	value, err := source.measure(ctx, windowStart)

	source.mu.Lock()
	source.cacheWindow = windowStart
	source.cacheValue = value
	source.cacheErr = err
	source.cacheAt = time.Now()
	source.mu.Unlock()
	return value, err
}

// measure performs one request and returns the used dollars of the window.
// The capacity the endpoint reports is validated but not used: the capacity of
// the reserve is an operator setting, and an unknown one keeps the gate closed.
func (source *hwBureauUsageHTTP) measure(ctx context.Context, windowStart int64) (float64, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source.url, nil)
	if err != nil {
		return 0, errors.New("bureau usage request could not be built")
	}
	if source.token != "" {
		request.Header.Set("Authorization", "Bearer "+source.token)
	}
	response, err := source.client.Do(request)
	if err != nil {
		return 0, errors.New("bureau usage endpoint is unreachable")
	}
	defer CloseResponseBodyGracefully(response)
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("bureau usage endpoint answered %d", response.StatusCode)
	}
	var payload struct {
		WindowStart int64   `json:"window_start"`
		UsedUSD     float64 `json:"used_usd"`
		CapacityUSD float64 `json:"capacity_usd"`
	}
	if err := common.DecodeJson(io.LimitReader(response.Body, hwBureauUsageMaxBody), &payload); err != nil {
		return 0, errors.New("bureau usage answer is not valid JSON")
	}
	if payload.WindowStart != 0 && payload.WindowStart != windowStart {
		// A measurement of another window is worse than none: it would open this
		// window on the previous one's numbers.
		return 0, errors.New("bureau usage answer belongs to another window")
	}
	if payload.UsedUSD < 0 || math.IsNaN(payload.UsedUSD) || math.IsInf(payload.UsedUSD, 0) {
		return 0, errors.New("bureau usage answer holds an invalid amount")
	}
	return payload.UsedUSD, nil
}

// hwBureauUsageRegistry holds the production source built from the environment
// and the override a test installs.
var hwBureauUsageRegistry struct {
	mu       sync.Mutex
	override HwBureauUsageSource
	source   *hwBureauUsageHTTP
	url      string
	token    string
}

// HwSetBureauUsageSource replaces the bureau usage source; nil restores the
// environment-driven one. It exists for tests and for a deployment that wires
// the measurement itself.
func HwSetBureauUsageSource(source HwBureauUsageSource) {
	hwBureauUsageRegistry.mu.Lock()
	defer hwBureauUsageRegistry.mu.Unlock()
	hwBureauUsageRegistry.override = source
}

// hwBureauUsageSource returns the source to use and whether one is configured.
func hwBureauUsageSource() (HwBureauUsageSource, bool) {
	url := strings.TrimSpace(os.Getenv(model.HwBureauUsageURLEnv))
	token := strings.TrimSpace(os.Getenv(model.HwBureauUsageTokenEnv))

	hwBureauUsageRegistry.mu.Lock()
	defer hwBureauUsageRegistry.mu.Unlock()
	if hwBureauUsageRegistry.override != nil {
		return hwBureauUsageRegistry.override, true
	}
	if url == "" {
		hwBureauUsageRegistry.source = nil
		return nil, false
	}
	if hwBureauUsageRegistry.source == nil || hwBureauUsageRegistry.url != url || hwBureauUsageRegistry.token != token {
		hwBureauUsageRegistry.source = &hwBureauUsageHTTP{
			url:    url,
			token:  token,
			client: &http.Client{Timeout: hwBureauUsageTimeout},
		}
		hwBureauUsageRegistry.url, hwBureauUsageRegistry.token = url, token
	}
	return hwBureauUsageRegistry.source, true
}

func hwFloatOption(key string, fallback float64) float64 {
	value := hwOption(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return fallback
	}
	return parsed
}

func hwEnvFloat(name string) float64 {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return 0
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0
	}
	return parsed
}

func hwBoolOption(key string, fallback bool) bool {
	value := strings.ToLower(hwOption(key))
	switch value {
	case "true", "1":
		return true
	case "false", "0":
		return false
	}
	return fallback
}

// HwWindowCapacityUSD is the cost of the shared capacity of one window. The
// option wins over the environment; zero means unknown.
func HwWindowCapacityUSD() float64 {
	if value := hwFloatOption(model.HwWindowCapacityUSDOption, 0); value > 0 {
		return value
	}
	if value := hwEnvFloat(model.HwWindowCapacityUSDEnv); value > 0 {
		return value
	}
	return 0
}

// HwReserveModeNow reads HW_RESERVE_MODE. An unknown value is treated as the
// strict mode, never as "open".
func HwReserveModeNow() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(model.HwReserveModeEnv))) {
	case HwReserveModeThird:
		return HwReserveModeThird
	default:
		return HwReserveModeAuto
	}
}

// HwCustomerLimitUSD is the share of the window customer traffic may use. It is
// always at most one third of the capacity until the phase-0 measurement is
// done; after it, the customers get the rest minus a margin. The result is
// never negative.
func HwCustomerLimitUSD(capacityUSD float64, bureauUsedUSD float64, phase0Done bool, mode string) float64 {
	if capacityUSD <= 0 {
		return 0
	}
	if mode == HwReserveModeThird || !phase0Done {
		return capacityUSD / 3
	}
	limit := capacityUSD - bureauUsedUSD - hwReserveMeasuredMarginShare*capacityUSD
	if limit < 0 {
		return 0
	}
	return limit
}

// HwCustomerCeilingUSD is the per-customer ceiling of one window. The free
// trial group gets half of the paid ceiling. Without a known capacity the
// ceiling rests on a fixed safe amount.
func HwCustomerCeilingUSD(capacityUSD float64, group string) float64 {
	base := hwFloatOption(model.HwCustomerWindowCeilingUSDOption, 0)
	if base <= 0 {
		if capacityUSD > 0 {
			base = capacityUSD / hwReserveCeilingCapacityShare
		} else {
			base = hwReserveFallbackCeilingUSD
		}
	}
	if group == model.HwFreeGroupName {
		return base * hwReserveFreeCeilingShare
	}
	return base
}

// HwReserveSnapshot is one measurement of the window: what the bureau used,
// what the customers used, how much the customers may use and why traffic is
// refused, if it is. It holds no secrets.
type HwReserveSnapshot struct {
	WindowStart            int64   `json:"window_start"`
	WindowEnd              int64   `json:"window_end"`
	Mode                   string  `json:"mode"`
	Phase0MeasurementDone  bool    `json:"phase0_measurement_done"`
	CapacityUSD            float64 `json:"capacity_usd"`
	BureauSourceConfigured bool    `json:"bureau_source_configured"`
	BureauUsedUSD          float64 `json:"bureau_used_usd"`
	CustomerUsedUSD        float64 `json:"customer_used_usd"`
	CustomerLimitUSD       float64 `json:"customer_limit_usd"`
	DenyReason             string  `json:"deny_reason"`
	RetryAfterSeconds      int     `json:"retry_after_seconds"`
	MeasuredAt             int64   `json:"measured_at"`
}

// HwBureauReserveSnapshot measures the current window and applies the rule.
// Any error inside it denies customer traffic with a reason instead of a zero
// usage that would look like an empty window.
func HwBureauReserveSnapshot(ctx context.Context, now time.Time) HwReserveSnapshot {
	windowStart, windowEnd := model.HwReserveWindow(now)
	snapshot := HwReserveSnapshot{
		WindowStart:           windowStart,
		WindowEnd:             windowEnd,
		Mode:                  HwReserveModeNow(),
		Phase0MeasurementDone: hwBoolOption(model.HwPhase0MeasurementDoneOption, false),
		CapacityUSD:           HwWindowCapacityUSD(),
		MeasuredAt:            now.UTC().Unix(),
	}
	snapshot.RetryAfterSeconds = hwReserveRetryAfter(windowEnd, now)

	deny := func(reason string) HwReserveSnapshot {
		if snapshot.DenyReason == "" {
			snapshot.DenyReason = reason
		}
		return snapshot
	}

	if !model.HwQuotaRateValid() {
		return deny(HwReserveDenyQuotaRateInvalid)
	}
	customerQuota, err := model.HwCustomerWindowQuota(windowStart, windowEnd)
	if err != nil {
		common.SysLog(common.LogText("hw reserve: customer usage of the window could not be read"))
		return deny(HwReserveDenyUsageUnavailable)
	}
	snapshot.CustomerUsedUSD = model.HwQuotaToUSD(customerQuota)

	if snapshot.CapacityUSD <= 0 {
		return deny(HwReserveDenyCapacityUnknown)
	}

	source, configured := hwBureauUsageSource()
	snapshot.BureauSourceConfigured = configured
	if snapshot.Mode == HwReserveModeThird {
		// The operator pinned the strict rule: no bureau measurement is needed to
		// apply one third of a known capacity.
		snapshot.CustomerLimitUSD = HwCustomerLimitUSD(snapshot.CapacityUSD, 0, false, snapshot.Mode)
	} else if !configured {
		return deny(HwReserveDenySourceUnavailable)
	} else if !snapshot.Phase0MeasurementDone {
		// The pre-measurement rule does not use the number, but a missing or
		// failing source still means the reserve cannot be verified, so the gate
		// stays closed instead of leaning on an unmeasured capacity. The number
		// itself is kept for the admin view.
		used, err := source.UsedUSD(ctx, windowStart, windowEnd)
		if err != nil {
			common.SysLog(common.LogText("hw reserve: bureau usage could not be measured"))
			return deny(HwReserveDenySourceUnavailable)
		}
		snapshot.BureauUsedUSD = used
		snapshot.CustomerLimitUSD = HwCustomerLimitUSD(snapshot.CapacityUSD, 0, false, snapshot.Mode)
	} else {
		used, err := source.UsedUSD(ctx, windowStart, windowEnd)
		if err != nil {
			common.SysLog(common.LogText("hw reserve: bureau usage could not be measured"))
			return deny(HwReserveDenySourceUnavailable)
		}
		snapshot.BureauUsedUSD = used
		snapshot.CustomerLimitUSD = HwCustomerLimitUSD(snapshot.CapacityUSD, used, true, snapshot.Mode)
	}

	if snapshot.CustomerUsedUSD >= snapshot.CustomerLimitUSD {
		return deny(HwReserveDenyLimitReached)
	}
	return snapshot
}

// HwCustomerReserveStatus is the per-customer part of the rule: how much this
// customer already used in the window and where its ceiling lies.
type HwCustomerReserveStatus struct {
	WindowStart       int64   `json:"window_start"`
	WindowEnd         int64   `json:"window_end"`
	Group             string  `json:"group"`
	UsedUSD           float64 `json:"used_usd"`
	CeilingUSD        float64 `json:"ceiling_usd"`
	Limited           bool    `json:"limited"`
	RetryAfterSeconds int     `json:"retry_after_seconds"`
}

// HwCustomerReserveStatusNow measures one customer's usage of the current
// window and compares it with its ceiling. The ceiling is capped by the window
// limit, so a single customer can never use more than the window allows.
func HwCustomerReserveStatusNow(userID int, group string, now time.Time) (HwCustomerReserveStatus, error) {
	windowStart, windowEnd := model.HwReserveWindow(now)
	status := HwCustomerReserveStatus{
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
		Group:       group,
	}
	quota, err := model.HwUserWindowQuota(userID, windowStart, windowEnd)
	if err != nil {
		return status, err
	}
	status.UsedUSD = model.HwQuotaToUSD(quota)
	status.CeilingUSD = HwCustomerCeilingUSD(HwWindowCapacityUSD(), group)
	status.Limited = status.UsedUSD >= status.CeilingUSD
	status.RetryAfterSeconds = hwReserveRetryAfter(windowEnd, now)
	return status, nil
}

// hwReserveRetryAfter is the number of whole seconds until the next window. It
// is never negative.
func hwReserveRetryAfter(windowEnd int64, now time.Time) int {
	seconds := windowEnd - now.UTC().Unix()
	if seconds < 0 {
		return 0
	}
	return int(seconds)
}

// HwReserveMinutes turns a wait in seconds into the minutes a customer is told
// to wait. It is at least one minute, so the message never says "in 0 minutes".
func HwReserveMinutes(retryAfterSeconds int) int {
	if retryAfterSeconds <= 0 {
		return 1
	}
	return (retryAfterSeconds + 59) / 60
}

// HwReserveBusyMessage is the "we are busy" text for a customer whose request
// the reserve refuses. It names no supplier and no internal detail.
func HwReserveBusyMessage(c *gin.Context, retryAfterSeconds int) string {
	minutes := HwReserveMinutes(retryAfterSeconds)
	if HwPrefersTurkish(c) {
		return fmt.Sprintf("Şu anda yoğunuz, lütfen %d dakika sonra tekrar deneyin.", minutes)
	}
	return fmt.Sprintf("We are busy right now, please try again in %d minutes.", minutes)
}

// HwReserveCeilingMessage is the text for a customer that reached its own
// window ceiling.
func HwReserveCeilingMessage(c *gin.Context, retryAfterSeconds int) string {
	minutes := HwReserveMinutes(retryAfterSeconds)
	if HwPrefersTurkish(c) {
		return fmt.Sprintf("Bu dönem için kullanım sınırınıza ulaştınız, lütfen %d dakika sonra tekrar deneyin.", minutes)
	}
	return fmt.Sprintf("You have reached your usage limit for this period, please try again in %d minutes.", minutes)
}
