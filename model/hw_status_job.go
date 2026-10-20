// One measurement pass of the status meter.
//
// The pass measures the share of failed paid customer requests of the last full
// minute from the rows the relay already writes: a failed request leaves an
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
	failed, measured, err := HwCountPaidRequests(from, minute)
	if err != nil {
		return err
	}
	verdict := HwEvaluateStatusMinute(failed, measured)
	for _, name := range models {
		sample := HwStatusSample{
			Minute:     minute,
			Model:      name,
			MeasuredAt: at.Unix(),
			Ok:         verdict.Ok,
			Reason:     verdict.Reason,
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
