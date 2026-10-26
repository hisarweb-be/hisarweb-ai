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

// hwStatusProbeTimeout bounds one mini call of the status probe. The pass runs
// every minute and probes the sold models one after another only when they saw
// no customer traffic, so a hanging upstream costs at most this per model.
const hwStatusProbeTimeout = 8 * time.Second

var (
	hwStatusProbeMu    sync.RWMutex
	hwStatusProbe      func(ctx context.Context, modelName string) bool
	hwStatusProbeToken int
)

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
func SetHwStatusProbe(probe func(ctx context.Context, modelName string) bool, probeTokenId int) {
	hwStatusProbeMu.Lock()
	defer hwStatusProbeMu.Unlock()
	hwStatusProbe = probe
	if probe == nil {
		hwStatusProbeToken = 0
		return
	}
	hwStatusProbeToken = probeTokenId
}

// hwStatusProbeCurrent returns the installed probe, or nil.
func hwStatusProbeCurrent() func(ctx context.Context, modelName string) bool {
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
				ok, reason = hwProbeStatusModel(probe, name)
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

// hwProbeStatusModel runs the mini call for one model and turns its answer into
// the verdict of a quiet minute: only an answer counts as good.
func hwProbeStatusModel(probe func(ctx context.Context, modelName string) bool, name string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), hwStatusProbeTimeout)
	defer cancel()
	if probe(ctx, name) {
		return true, ""
	}
	return false, HwStatusReasonProbeFailed
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
