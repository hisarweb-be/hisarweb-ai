// One measurement pass of the status meter.
//
// The pass measures, per model, the share of failed paid customer requests of
// the last full minute from the rows the relay already writes: a failed request leaves an
// error row in the paid group and a served request leaves a consume row. It
// records one sample per own model, opens or closes an outage from the sample
// chain and applies the credit extension of every closed but unhandled outage.
//
// The measurement never reads a channel or a supplier: it only counts rows and
// stores a reason class. The minute ticker and panic guard live in the service
// layer, which also knows the catalog of own model names.
package model

import (
	"context"
	"sync"
	"time"
)

// HwStatusProbeOutcome is what one mini call of the status probe tells about a
// sold model. It has three states and not two, because "the route refused us"
// says nothing about the model: a quota or key answer (429, 401, 403) must
// neither open a false outage nor count as a good minute.
type HwStatusProbeOutcome int

const (
	// HwStatusProbeAnswered means the upstream answered the mini call.
	HwStatusProbeAnswered HwStatusProbeOutcome = iota
	// HwStatusProbeNoAnswer means the model itself did not answer: a timeout,
	// a network error or an error answer. That is a bad minute.
	HwStatusProbeNoAnswer
	// HwStatusProbeUnavailable means the route refused the call itself (quota
	// exhausted, key rejected). The model was not measured, so the minute is no
	// evidence either way.
	HwStatusProbeUnavailable
)

// hwStatusProbeIntervalSeconds is how long an outcome of the mini call is
// reused before the upstream is asked again. Without it every quiet minute
// probed every sold model: eight models meant some 11 500 calls a day with real
// supplier cost and real quota use, and an exhausted quota turned the whole
// offer into probe_failed. Five minutes keeps the three-minute outage chain
// intact, because every minute still gets a sample; only the call is reused.
const hwStatusProbeIntervalSeconds = 5 * 60

// hwStatusProbeTimeout bounds one mini call of the status probe. The pass runs
// every minute and probes the sold models one after another only when they saw
// no customer traffic, so a hanging upstream costs at most this per model.
const hwStatusProbeTimeout = 8 * time.Second

var (
	hwStatusProbeMu    sync.RWMutex
	hwStatusProbe      func(ctx context.Context, modelName string) HwStatusProbeOutcome
	hwStatusProbeToken int
	// hwStatusProbeLast is the newest outcome per model, so a quiet minute that
	// skips the call still gets a verdict. It is job memory, not a table: it
	// describes this process only and it is dropped when the probe is replaced.
	hwStatusProbeLast map[string]hwStatusProbeResult
)

// hwStatusProbeResult is one remembered outcome of the mini call.
type hwStatusProbeResult struct {
	ok     bool
	reason string
	minute int64
}

// SetHwStatusProbe installs the mini call that decides a quiet minute, or
// clears it with nil. The service layer installs it once at startup, because
// only that layer knows the customer route and its key; the model layer keeps
// the measurement free of supplier knowledge. Without a probe the route check
// decides, so an environment without a key never reports a false outage.
//
// probeTokenId is the token the mini call itself calls with, or 0 when the
// token could not be looked up. The mini call writes its own rows in the log
// table the measurement reads, so its traffic must be left out of the count:
// otherwise every failed probe makes the next minute look like a customer
// failure, the chain never reaches three bad minutes and an outage of a quiet
// upstream is never opened.
func SetHwStatusProbe(probe func(ctx context.Context, modelName string) HwStatusProbeOutcome, probeTokenId int) {
	hwStatusProbeMu.Lock()
	defer hwStatusProbeMu.Unlock()
	hwStatusProbe = probe
	hwStatusProbeLast = nil
	if probe == nil {
		hwStatusProbeToken = 0
		return
	}
	hwStatusProbeToken = probeTokenId
}

// hwStatusProbeCurrent returns the installed probe, or nil.
func hwStatusProbeCurrent() func(ctx context.Context, modelName string) HwStatusProbeOutcome {
	hwStatusProbeMu.RLock()
	defer hwStatusProbeMu.RUnlock()
	return hwStatusProbe
}

// hwStatusProbeTokenId returns the token id of the installed probe, or 0 when
// no probe is installed or its token is unknown. A zero excludes nothing.
func hwStatusProbeTokenId() int {
	hwStatusProbeMu.RLock()
	defer hwStatusProbeMu.RUnlock()
	return hwStatusProbeToken
}

// HwMeasureStatusOnce measures the minute that ends at at for the given own
// model names and records it. It is exported so a test can run the pass without
// the ticker.
//
// The measured window is the last full minute: from the start of the previous
// minute to the start of this one. A repeated pass for the same minute records
// nothing new (the per-minute sample limit), so a restarted process is safe.
func HwMeasureStatusOnce(at time.Time, models []string) error {
	minute := hwStatusMinuteNow(at)
	from := minute - hwStatusMinuteSeconds
	counts, err := HwCountPaidRequests(from, minute)
	if err != nil {
		return err
	}
	var routed map[string]bool
	probe := hwStatusProbeCurrent()
	for _, name := range models {
		verdict := HwEvaluateStatusMinute(counts[name].Failed, counts[name].Measured)
		ok, reason := verdict.Ok, verdict.Reason
		if verdict.Idle {
			// Without traffic the evidence is the upstream itself: a mini call
			// per sold model. Only that call shows whether an upstream answers,
			// and without it a 503 stayed invisible on the status page (the
			// outage of 24-26 Oct 2026).
			if probe != nil {
				ok, reason = hwProbeStatusModel(probe, name, minute)
			} else {
				// No probe configured: the route is the only evidence. A sold
				// model with an enabled channel in the paid group can be served.
				if routed == nil {
					routed = hwRoutedModels()
				}
				ok, reason = routed[name], ""
				if !ok {
					reason = HwStatusReasonNoRoute
				}
			}
		}
		sample := HwStatusSample{
			Minute:     minute,
			Model:      name,
			MeasuredAt: at.Unix(),
			Ok:         ok,
			Reason:     reason,
		}
		if err := HwRecordStatusSample(&sample); err != nil && err != ErrHwStatusDuplicateSample {
			return err
		}
	}
	if err := HwReconcileStatusIncidents(minute); err != nil {
		return err
	}
	_, err = HwExtendPendingIncidents(at.Unix())
	return err
}

// hwProbeStatusModel turns one outcome of the mini call into the verdict of a
// quiet minute: only an answer counts as good, a model that did not answer is a
// bad minute, and a route refusal is no evidence at all (HwStatusReasonProbe-
// Unavailable), so it can open neither a false outage nor a false green.
//
// The call itself runs at most once per hwStatusProbeIntervalSeconds per model.
// In between, the last outcome is reused, so every minute still gets a sample
// and the three-minute chain keeps working while the cost stays bounded.
func hwProbeStatusModel(probe func(ctx context.Context, modelName string) HwStatusProbeOutcome, name string, minute int64) (bool, string) {
	if cached, ok := hwStatusProbeReuse(name, minute); ok {
		return cached.ok, cached.reason
	}
	ctx, cancel := context.WithTimeout(context.Background(), hwStatusProbeTimeout)
	defer cancel()
	outcome := probe(ctx, name)
	result := hwStatusProbeResult{ok: true, minute: minute}
	switch outcome {
	case HwStatusProbeAnswered:
	case HwStatusProbeNoAnswer:
		result.ok, result.reason = false, HwStatusReasonProbeFailed
	case HwStatusProbeUnavailable:
		result.ok, result.reason = false, HwStatusReasonProbeUnavailable
	}
	hwStatusProbeRemember(name, result)
	return result.ok, result.reason
}

// hwStatusProbeReuse returns the remembered outcome of a model while it is
// younger than the probe interval.
func hwStatusProbeReuse(name string, minute int64) (hwStatusProbeResult, bool) {
	hwStatusProbeMu.RLock()
	defer hwStatusProbeMu.RUnlock()
	result, seen := hwStatusProbeLast[name]
	if !seen || result.minute > minute || minute-result.minute >= hwStatusProbeIntervalSeconds {
		return hwStatusProbeResult{}, false
	}
	return result, true
}

// hwStatusProbeRemember stores the newest outcome of a model.
func hwStatusProbeRemember(name string, result hwStatusProbeResult) {
	hwStatusProbeMu.Lock()
	defer hwStatusProbeMu.Unlock()
	if hwStatusProbeLast == nil {
		hwStatusProbeLast = make(map[string]hwStatusProbeResult)
	}
	hwStatusProbeLast[name] = result
}

// hwRoutedModels is the set of model names with an enabled channel in the paid
// group.
func hwRoutedModels() map[string]bool {
	routed := make(map[string]bool)
	for _, name := range GetGroupEnabledModels(HwPaidGroupName) {
		routed[name] = true
	}
	return routed
}

// HwPurgeFalseIdleOutages removes what the retired idle rule left behind: the
// samples that counted a quiet minute as bad, and every outage that has no run
// of three consecutive really bad minutes inside it (the rule that opens an
// outage). Credit extensions that were already granted stay with the customer.
// Only outages inside the sample retention are judged: an older outage has lost
// its samples to the cleanup, not its truth. Running it twice changes nothing;
// it returns the number of outages removed.
func HwPurgeFalseIdleOutages() (int, error) {
	if err := DB.Where("reason = ?", HwStatusReasonMeasurement).Delete(&HwStatusSample{}).Error; err != nil {
		return 0, err
	}
	var incidents []HwIncident
	since := time.Now().Unix() - hwStatusSampleRetentionSeconds
	if err := DB.Where("started_at >= ?", since).Find(&incidents).Error; err != nil {
		return 0, err
	}
	removed := 0
	for _, incident := range incidents {
		var badMinutes []int64
		query := DB.Model(&HwStatusSample{}).
			Where("ok = ? AND minute >= ?", commonFalseVal, incident.StartedAt)
		if incident.EndedAt > 0 {
			query = query.Where("minute < ?", incident.EndedAt)
		}
		if err := query.Distinct().Order("minute").Pluck("minute", &badMinutes).Error; err != nil {
			return removed, err
		}
		if hwHasBadRun(badMinutes) {
			continue
		}
		if err := DB.Delete(&HwIncident{}, incident.Id).Error; err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

// hwHasBadRun reports whether the sorted bad minutes hold a run that is long
// enough to open an outage.
func hwHasBadRun(sortedMinutes []int64) bool {
	need := HwStatusIncidentAfterBadSeconds / hwStatusMinuteSeconds
	run := 0
	for i, minute := range sortedMinutes {
		if i > 0 && minute == sortedMinutes[i-1]+hwStatusMinuteSeconds {
			run++
		} else {
			run = 1
		}
		if run >= need {
			return true
		}
	}
	return false
}
