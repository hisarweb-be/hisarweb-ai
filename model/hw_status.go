// Public service status and the automatic credit extension after an outage.
//
//   - hw_status_samples holds one row per own model per measured minute: the
//     minute, the model's own name, whether it was good and, when it was bad, a
//     reason class. The class never carries a supplier name or an internal
//     error text.
//   - hw_incidents is one outage: the first bad minute, the first good minute
//     after it, the length, the kind and the single "handled" mark that makes
//     the credit extension run exactly once.
//   - hw_credit_extensions records one extension per outage and package. The
//     derived package fill reads it through HwCreditPackage.ExtensionSeconds,
//     so an extended package expires later without rewriting its own row.
//
// The public answer is built from these rows only: no channel, no supplier and
// no upstream error is read to produce it.
package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// HwStatusIncidentAfterBadSeconds is the run of bad minutes that opens an
	// outage; HwStatusRecoveryAfterGoodSeconds is the run of good minutes that
	// closes it again.
	HwStatusIncidentAfterBadSeconds  = 3 * 60
	HwStatusRecoveryAfterGoodSeconds = 3 * 60

	// HwStatusMinuteSampleLimit bounds the samples of one model in one minute,
	// so a repeated measurement cannot grow the table without end.
	HwStatusMinuteSampleLimit = 4

	// HwStatusIncidentRecentLimit bounds the incident list of the public answer.
	HwStatusIncidentRecentLimit = 10

	// HwStatusAvailabilityDays is the window of the availability figure.
	HwStatusAvailabilityDays = 30

	// HwStatusExtensionThresholdSeconds is the outage length from which running
	// paid packages are extended. An outage of exactly this length does not
	// extend: the rule is "more than 15 minutes".
	HwStatusExtensionThresholdSeconds = 15 * 60

	// hwStatusSampleRetentionSeconds keeps raw samples for the availability
	// window plus one day of margin; incidents are kept on their own.
	hwStatusSampleRetentionSeconds = (HwStatusAvailabilityDays + 1) * 24 * 3600

	// hwStatusBadShareMicro (parts per million, so 50 000 is five percent) is
	// the failed share of paid customer requests above which a minute is bad.
	hwStatusBadShareMicro = 50_000

	// hwStatusMinMeasuredRequests keeps a handful of failures in a quiet minute
	// from marking the whole minute bad; below this count the share says little.
	hwStatusMinMeasuredRequests = 10

	// hwStatusMinAllFailedRequests is the least number of failures that makes a
	// minute bad when nothing was served. One customer retrying a broken request
	// is not an outage of the estate.
	hwStatusMinAllFailedRequests = 3

	hwStatusMinuteSeconds = 60
)

// Reason classes of a bad minute. They are stored, so they stay stable and name
// neither a supplier nor an error text.
const (
	HwStatusReasonRelayFailures = "relay_failures"
	HwStatusReasonNoRoute       = "no_route"
	// HwStatusReasonMeasurement is the retired verdict of a minute without
	// traffic. Until 23 Oct 2026 such a minute counted as bad, so every quiet
	// night opened a false outage of the whole estate; the sweep in
	// HwPurgeFalseIdleOutages removes those rows. It is never written again.
	HwStatusReasonMeasurement = "measurement"
)

// Incident kinds, fixed in code so the answer does not depend on stored text.
const (
	HwStatusIncidentRelay = "relay"
)

// ErrHwStatusDuplicateSample reports a sample beyond the per-minute limit. It
// is a normal outcome of a repeated measurement, not a failure.
var ErrHwStatusDuplicateSample = errors.New("status sample already recorded for this minute and model")

// HwStatusSample is one minute measurement of one own model.
type HwStatusSample struct {
	Id int `json:"id" gorm:"primaryKey"`
	// Minute is the unix minute (second 0) the sample belongs to.
	Minute int64  `json:"minute" gorm:"not null;index:idx_hw_status_samples_minute"`
	Model  string `json:"model" gorm:"type:varchar(128);not null;default:''"`
	// MeasuredAt is the moment the measurement ran.
	MeasuredAt int64  `json:"measured_at" gorm:"not null"`
	Ok         bool   `json:"ok" gorm:"not null"`
	Reason     string `json:"reason" gorm:"type:varchar(32);not null;default:''"`
}

func (HwStatusSample) TableName() string {
	return "hw_status_samples"
}

// HwIncident is one outage of the gateway's own model route.
type HwIncident struct {
	Id int `json:"id" gorm:"primaryKey"`
	// StartedAt is the first bad minute, EndedAt the first good minute after the
	// outage (0 while it still runs). Duration is EndedAt - StartedAt.
	StartedAt int64  `json:"started_at" gorm:"not null;index"`
	EndedAt   int64  `json:"ended_at" gorm:"not null;default:0;index"`
	Duration  int64  `json:"duration" gorm:"not null;default:0"`
	Kind      string `json:"kind" gorm:"type:varchar(16);not null;default:'relay'"`
	// Handled marks that the credit extension of this incident was evaluated.
	// It is written in the same transaction as the extensions, so a second run
	// changes nothing.
	Handled bool `json:"handled" gorm:"not null;default:false;index"`
}

func (HwIncident) TableName() string {
	return "hw_incidents"
}

// HwCreditExtension is one extension of one paid package by one outage. The
// unique index is the second guarantee against granting the same minutes twice.
type HwCreditExtension struct {
	Id         int   `json:"id" gorm:"primaryKey"`
	PackageId  int   `json:"package_id" gorm:"not null;uniqueIndex:idx_hw_credit_extensions_package_incident,priority:1;index"`
	IncidentId int   `json:"incident_id" gorm:"not null;uniqueIndex:idx_hw_credit_extensions_package_incident,priority:2;index"`
	Minutes    int64 `json:"minutes" gorm:"not null"`
	CreatedAt  int64 `json:"created_at" gorm:"not null"`
}

func (HwCreditExtension) TableName() string {
	return "hw_credit_extensions"
}

// HwRecordStatusSample stores one minute measurement. A sample beyond the limit
// of the same minute and model returns ErrHwStatusDuplicateSample, so a repeated
// measurement is harmless.
func HwRecordStatusSample(sample *HwStatusSample) error {
	if sample == nil || sample.Minute <= 0 || sample.Model == "" {
		return errors.New("invalid status sample")
	}
	var count int64
	if err := DB.Model(&HwStatusSample{}).
		Where("minute = ? AND model = ?", sample.Minute, sample.Model).
		Count(&count).Error; err != nil {
		return err
	}
	if count >= HwStatusMinuteSampleLimit {
		return ErrHwStatusDuplicateSample
	}
	return DB.Create(sample).Error
}

// HwStatusVerdict is the outcome of one health measurement of one minute.
// Idle means the minute had no paid traffic at all: the traffic says nothing
// and the route check of each model decides (HwMeasureStatusOnce).
type HwStatusVerdict struct {
	Ok     bool
	Idle   bool
	Reason string
}

// HwEvaluateStatusMinute derives the verdict of a minute from the failed share
// of paid customer requests. A minute without traffic is idle, never bad: no
// customer asking is not evidence of an outage (that rule kept /durum red for
// 38 hours in Oct 2026).
func HwEvaluateStatusMinute(failedRequests int64, measuredRequests int64) HwStatusVerdict {
	if measuredRequests <= 0 && failedRequests <= 0 {
		return HwStatusVerdict{Idle: true}
	}
	if failedRequests >= measuredRequests {
		if failedRequests < hwStatusMinAllFailedRequests {
			return HwStatusVerdict{Ok: true}
		}
		return HwStatusVerdict{Reason: HwStatusReasonRelayFailures}
	}
	if failedRequests >= hwStatusMinMeasuredRequests &&
		failedRequests*1_000_000 >= measuredRequests*hwStatusBadShareMicro {
		return HwStatusVerdict{Reason: HwStatusReasonRelayFailures}
	}
	return HwStatusVerdict{Ok: true}
}

// HwMinuteVerdicts folds the samples of a span into one verdict per minute: a
// minute is good only when every sample in it is good, and a minute without a
// sample has no verdict at all (it is neither good nor bad).
func HwMinuteVerdicts(samples []HwStatusSample) map[int64]bool {
	verdicts := make(map[int64]bool, len(samples))
	for _, sample := range samples {
		ok, seen := verdicts[sample.Minute]
		if !seen {
			verdicts[sample.Minute] = sample.Ok
			continue
		}
		if !sample.Ok {
			verdicts[sample.Minute] = false
		} else {
			verdicts[sample.Minute] = ok
		}
	}
	return verdicts
}

// hwStatusRunOf reports whether every minute in [from, to] step one minute is
// present in verdicts with the wanted value.
func hwStatusRunOf(verdicts map[int64]bool, from int64, to int64, wanted bool) bool {
	minutes := 0
	for minute := from; minute <= to; minute += hwStatusMinuteSeconds {
		ok, seen := verdicts[minute]
		if !seen || ok != wanted {
			return false
		}
		minutes++
	}
	return minutes > 0
}

// HwReconcileStatusIncidents opens or closes the outage that the newest three
// minute verdicts describe. It runs after every measured minute, so an outage is
// recorded once and gains its end at the first good minute of a run of three.
//
// An incident opens when the last three minutes all have samples and all are
// bad; it closes when the last three minutes all have samples and all are good.
// A minute without samples breaks both runs: no traffic is not evidence.
func HwReconcileStatusIncidents(now int64) error {
	if now <= 0 {
		return errors.New("invalid status reconciliation time")
	}
	var samples []HwStatusSample
	if err := DB.Where("minute > ? AND minute <= ?", now-HwStatusIncidentAfterBadSeconds, now).
		Find(&samples).Error; err != nil {
		return err
	}
	verdicts := HwMinuteVerdicts(samples)
	from := now - HwStatusIncidentAfterBadSeconds + hwStatusMinuteSeconds
	allBad := hwStatusRunOf(verdicts, from, now, false)
	allGood := hwStatusRunOf(verdicts, from, now, true)

	var open HwIncident
	err := DB.Where("ended_at = 0").Order("id DESC").First(&open).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		if !allBad {
			return nil
		}
		incident := HwIncident{StartedAt: from, Kind: HwStatusIncidentRelay}
		return DB.Create(&incident).Error
	case err != nil:
		return err
	}
	if !allGood {
		return nil
	}
	return DB.Model(&HwIncident{}).
		Where("id = ? AND ended_at = 0", open.Id).
		Updates(map[string]any{
			"ended_at": from,
			"duration": from - open.StartedAt,
		}).Error
}

// HwExtendPendingIncidents extends the credit of every closed incident that has
// not been handled yet. It is the safety sweep behind HwReconcileStatusIncidents:
// an incident that closed while the process restarted still reaches its credit
// extension on the next run. Running it twice changes nothing.
func HwExtendPendingIncidents(now int64) (int, error) {
	var incidents []HwIncident
	if err := DB.Where("ended_at > 0 AND handled = ?", commonFalseVal).
		Order("id ASC").Find(&incidents).Error; err != nil {
		return 0, err
	}
	extended := 0
	for _, incident := range incidents {
		count, err := HwExtendCreditPackagesAfterIncident(incident.Id, now)
		if err != nil {
			return extended, err
		}
		extended += count
	}
	return extended, nil
}

// HwExtendCreditPackagesAfterIncident extends every paid package that was
// running during the outage by the outage length, rounded up to whole minutes.
//
// It runs exactly once per incident: the incident row is locked and marked
// handled inside the same transaction that inserts the extensions, and the
// unique index on (package_id, incident_id) refuses a second row. An incident
// of HwStatusExtensionThresholdSeconds or shorter is marked handled without
// extending anything. Free trial credit is never extended.
func HwExtendCreditPackagesAfterIncident(incidentID int, now int64) (int, error) {
	if incidentID <= 0 || now <= 0 {
		return 0, errors.New("invalid credit extension request")
	}
	extended := 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		var incident HwIncident
		if err := lockForUpdate(tx).First(&incident, incidentID).Error; err != nil {
			return err
		}
		if incident.Handled || incident.EndedAt == 0 {
			return nil
		}
		minutes := (incident.Duration + hwStatusMinuteSeconds - 1) / hwStatusMinuteSeconds
		if incident.Duration <= HwStatusExtensionThresholdSeconds {
			minutes = 0
		}
		if minutes > 0 {
			// Every package that was running when the outage started is extended,
			// whatever its source except the free trial credit. A package bought
			// during the outage did not lose time at the start of it.
			var packages []HwCreditPackage
			if err := tx.Where("retired_at = 0 AND created_at <= ?", incident.StartedAt).
				Find(&packages).Error; err != nil {
				return err
			}
			if err := hwAttachCreditExtensions(tx, packages); err != nil {
				return err
			}
			for i := range packages {
				pkg := packages[i]
				if pkg.Source == HwSourceFree || pkg.HwEffectiveExpiresAt() <= incident.StartedAt {
					continue
				}
				result := tx.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "package_id"}, {Name: "incident_id"}},
					DoNothing: true,
				}).Create(&HwCreditExtension{
					PackageId:  pkg.Id,
					IncidentId: incident.Id,
					Minutes:    minutes,
					CreatedAt:  now,
				})
				if result.Error != nil {
					return result.Error
				}
				extended += int(result.RowsAffected)
			}
		}
		return tx.Model(&HwIncident{}).
			Where("id = ? AND handled = ?", incident.Id, commonFalseVal).
			Update("handled", true).Error
	})
	if err != nil {
		return 0, err
	}
	return extended, nil
}

// PurgeOldStatusSamples drops the samples outside the availability window. It
// runs hourly from the measurement job and changes nothing when called twice.
func PurgeOldStatusSamples(now int64) error {
	if now <= 0 {
		return errors.New("invalid status purge time")
	}
	return DB.Where("minute < ?", now-hwStatusSampleRetentionSeconds).
		Delete(&HwStatusSample{}).Error
}

// HwStatusModelState is the public state of one own model.
type HwStatusModelState struct {
	Model string `json:"model"`
	// BadRecent is true when one of the newest three minutes of this model was
	// bad, which makes the model degraded even without an outage.
	BadRecent bool `json:"bad_recent"`
	// InOpenIncident is true when the model had a bad minute since the running
	// outage started.
	InOpenIncident bool `json:"in_open_incident"`
	// GoodMinutes and TotalMinutes cover the availability window.
	GoodMinutes  int64 `json:"good_minutes"`
	TotalMinutes int64 `json:"total_minutes"`
}

// HwStatusSnapshot is what the public status answer needs.
type HwStatusSnapshot struct {
	Models    []HwStatusModelState
	Incidents []HwIncident
}

// hwStatusAvailabilityRow is one aggregate row of the availability query.
type hwStatusAvailabilityRow struct {
	Model string
	Total int64
	Good  int64
}

// HwCountPaidRequests counts the paid customer requests and the failed ones in
// one window. The consume rows are the measured requests and the error rows of
// the same paid group are the failures, so the share comes from the rows the
// relay already writes. The window is bounded in plain comparisons instead of
// dialect-specific time functions, because the log table can live on any of the
// supported databases.
func HwCountPaidRequests(from int64, to int64) (failed int64, measured int64, err error) {
	if from <= 0 || to <= from {
		return 0, 0, errors.New("invalid status measurement window")
	}
	err = LOG_DB.Model(&Log{}).
		Where("type = ?", LogTypeConsume).
		Where("created_at >= ? AND created_at < ?", from, to).
		Where(logGroupCol+" = ?", HwPaidGroupName).
		Count(&measured).Error
	if err != nil {
		return 0, 0, err
	}
	err = LOG_DB.Model(&Log{}).
		Where("type = ?", LogTypeError).
		Where("created_at >= ? AND created_at < ?", from, to).
		Where(logGroupCol+" = ?", HwPaidGroupName).
		Count(&failed).Error
	if err != nil {
		return 0, 0, err
	}
	return failed, measured, nil
}

// HwReadStatusSnapshot reads the public status of the named own models: their
// availability over the window, the recent bad minutes and the outages. It
// aggregates in the database, because thirty days of minute samples are too
// many to load per request.
func HwReadStatusSnapshot(now int64, models []string) (HwStatusSnapshot, error) {
	if now <= 0 {
		return HwStatusSnapshot{}, errors.New("invalid status read time")
	}
	snapshot := HwStatusSnapshot{Models: make([]HwStatusModelState, 0, len(models))}
	states := make(map[string]*HwStatusModelState, len(models))
	for _, name := range models {
		states[name] = &HwStatusModelState{Model: name}
	}

	var availability []hwStatusAvailabilityRow
	if err := DB.Model(&HwStatusSample{}).
		Select("model, COUNT(*) AS total, SUM(CASE WHEN ok = ? THEN 1 ELSE 0 END) AS good", commonTrueVal).
		Where("minute >= ?", now-HwStatusAvailabilityDays*24*3600).
		Group("model").
		Scan(&availability).Error; err != nil {
		return HwStatusSnapshot{}, err
	}
	for _, row := range availability {
		if state, ok := states[row.Model]; ok {
			state.TotalMinutes = row.Total
			state.GoodMinutes = row.Good
		}
	}

	var badModels []string
	if err := DB.Model(&HwStatusSample{}).
		Where("minute > ? AND ok = ?", now-HwStatusRecoveryAfterGoodSeconds, commonFalseVal).
		Distinct().
		Pluck("model", &badModels).Error; err != nil {
		return HwStatusSnapshot{}, err
	}
	for _, name := range badModels {
		if state, ok := states[name]; ok {
			state.BadRecent = true
		}
	}

	var open HwIncident
	err := DB.Where("ended_at = 0").Order("id DESC").First(&open).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return HwStatusSnapshot{}, err
	}
	if err == nil {
		var incidentModels []string
		if err := DB.Model(&HwStatusSample{}).
			Where("minute >= ? AND ok = ?", open.StartedAt, commonFalseVal).
			Distinct().
			Pluck("model", &incidentModels).Error; err != nil {
			return HwStatusSnapshot{}, err
		}
		for _, name := range incidentModels {
			if state, ok := states[name]; ok {
				state.InOpenIncident = true
			}
		}
	}

	var incidents []HwIncident
	if err := DB.Order("id DESC").Limit(HwStatusIncidentRecentLimit).Find(&incidents).Error; err != nil {
		return HwStatusSnapshot{}, err
	}
	snapshot.Incidents = incidents

	for _, name := range models {
		snapshot.Models = append(snapshot.Models, *states[name])
	}
	return snapshot, nil
}

// HwStatusCodeOf maps the state of one model onto the public status code.
func HwStatusCodeOf(state HwStatusModelState) string {
	switch {
	case state.InOpenIncident:
		return "outage"
	case state.BadRecent:
		return "degraded"
	}
	return "operational"
}

// HwStatusAvailabilityPercent is the availability over the window as a rounded
// percentage. A model without any sample reports 0 and is not claimed healthy.
func HwStatusAvailabilityPercent(state HwStatusModelState) float64 {
	if state.TotalMinutes <= 0 {
		return 0
	}
	ratio := float64(state.GoodMinutes) / float64(state.TotalMinutes) * 100
	return float64(int64(ratio*100+0.5)) / 100
}

// hwStatusMinuteNow is the current unix minute, kept in one place so the job
// and the tests agree on rounding.
func hwStatusMinuteNow(at time.Time) int64 {
	return at.Unix() - at.Unix()%hwStatusMinuteSeconds
}

// HwStatusMinuteOf returns the unix minute (second 0) of at.
func HwStatusMinuteOf(at time.Time) int64 {
	return hwStatusMinuteNow(at)
}

// hwCreditExtensionSum is one package total of the extension table.
type hwCreditExtensionSum struct {
	PackageId int
	Seconds   int64
}

// hwAttachCreditExtensions loads the outage extensions of the given packages
// and sets ExtensionSeconds on each. It is called wherever a package's expiry
// is used, so the derived fill, the retirement and the package list all see the
// same effective expiry. A package without extensions keeps zero.
func hwAttachCreditExtensions(db *gorm.DB, pkgs []HwCreditPackage) error {
	if len(pkgs) == 0 {
		return nil
	}
	ids := make([]int, 0, len(pkgs))
	for _, pkg := range pkgs {
		ids = append(ids, pkg.Id)
	}
	var sums []hwCreditExtensionSum
	if err := db.Model(&HwCreditExtension{}).
		Select("package_id, COALESCE(SUM(minutes), 0) * ? AS seconds", hwStatusMinuteSeconds).
		Where("package_id IN ?", ids).
		Group("package_id").
		Scan(&sums).Error; err != nil {
		return err
	}
	if len(sums) == 0 {
		return nil
	}
	seconds := make(map[int]int64, len(sums))
	for _, sum := range sums {
		seconds[sum.PackageId] = sum.Seconds
	}
	for i := range pkgs {
		pkgs[i].ExtensionSeconds = seconds[pkgs[i].Id]
	}
	return nil
}
