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
	"time"
)

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
	for _, name := range models {
		verdict := HwEvaluateStatusMinute(counts[name].Failed, counts[name].Measured)
		ok, reason := verdict.Ok, verdict.Reason
		if verdict.Idle {
			// Without traffic the evidence is the route: a sold model with an
			// enabled channel in the paid group can be served.
			if routed == nil {
				routed = hwRoutedModels()
			}
			ok, reason = routed[name], ""
			if !ok {
				reason = HwStatusReasonNoRoute
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
// Running it twice changes nothing; it returns the number of outages removed.
func HwPurgeFalseIdleOutages() (int, error) {
	if err := DB.Where("reason = ?", HwStatusReasonMeasurement).Delete(&HwStatusSample{}).Error; err != nil {
		return 0, err
	}
	var incidents []HwIncident
	if err := DB.Find(&incidents).Error; err != nil {
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
