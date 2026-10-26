// The status meter and the automatic credit extension, pinned at the storage
// layer where the decisions are made: the minute evaluation, the incident
// chain (three bad minutes), the "more than fifteen minutes" rule, the
// once-per-incident extension in a transaction, the free trial credit staying
// untouched, and the derived fill respecting the extension.

package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// hwStatusTestSetup starts from an empty meter: samples, incidents and
// extensions are cleared and migrated.
func hwStatusTestSetup(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&HwStatusSample{}, &HwIncident{}, &HwCreditExtension{}, &HwCreditPackage{}))
	require.NoError(t, DB.Exec("DELETE FROM hw_status_samples").Error)
	require.NoError(t, DB.Exec("DELETE FROM hw_incidents").Error)
	require.NoError(t, DB.Exec("DELETE FROM hw_credit_extensions").Error)
	t.Cleanup(func() {
		DB.Exec("DELETE FROM hw_status_samples")
		DB.Exec("DELETE FROM hw_incidents")
		DB.Exec("DELETE FROM hw_credit_extensions")
	})
}

// hwStatusTestSample records one sample of one model for one minute.
func hwStatusTestSample(t *testing.T, minute int64, ok bool, reason string) {
	t.Helper()
	require.NoError(t, HwRecordStatusSample(&HwStatusSample{
		Minute: minute, Model: "claude-sonnet-5-5", MeasuredAt: minute, Ok: ok, Reason: reason,
	}))
}

// hwStatusTestIncident creates a closed incident of the given length and marks
// it unhandled, as the reconciliation leaves it.
func hwStatusTestIncident(t *testing.T, startedAt int64, duration int64) HwIncident {
	t.Helper()
	incident := HwIncident{
		StartedAt: startedAt,
		EndedAt:   startedAt + duration,
		Duration:  duration,
		Kind:      HwStatusIncidentRelay,
	}
	require.NoError(t, DB.Create(&incident).Error)
	return incident
}

func TestHwStatusThreeBadMinutesOpenAnIncident(t *testing.T) {
	hwStatusTestSetup(t)
	base := int64(1_800_000_000)

	// Two bad minutes are not an outage yet.
	hwStatusTestSample(t, base, false, HwStatusReasonRelayFailures)
	hwStatusTestSample(t, base+60, false, HwStatusReasonRelayFailures)
	require.NoError(t, HwReconcileStatusIncidents(base+60))
	var count int64
	require.NoError(t, DB.Model(&HwIncident{}).Count(&count).Error)
	assert.Zero(t, count, "two bad minutes must not open an incident")

	// The third bad minute opens it at the first bad minute of the run.
	hwStatusTestSample(t, base+120, false, HwStatusReasonRelayFailures)
	require.NoError(t, HwReconcileStatusIncidents(base+120))
	var open HwIncident
	require.NoError(t, DB.Where("ended_at = 0").First(&open).Error)
	assert.Equal(t, base, open.StartedAt)
	assert.Zero(t, open.EndedAt)
	assert.False(t, open.Handled)

	// One good minute does not close it; three good minutes do.
	hwStatusTestSample(t, base+180, true, "")
	hwStatusTestSample(t, base+240, true, "")
	require.NoError(t, HwReconcileStatusIncidents(base+240))
	require.NoError(t, DB.First(&open, open.Id).Error)
	assert.Zero(t, open.EndedAt, "two good minutes must not close the incident")

	hwStatusTestSample(t, base+300, true, "")
	require.NoError(t, HwReconcileStatusIncidents(base+300))
	require.NoError(t, DB.First(&open, open.Id).Error)
	assert.Equal(t, base+180, open.EndedAt)
	assert.Equal(t, int64(180), open.Duration)

	// A single bad minute after recovery opens nothing new: an outage starts at
	// three consecutive bad minutes, not at one.
	hwStatusTestSample(t, base+360, false, HwStatusReasonRelayFailures)
	require.NoError(t, HwReconcileStatusIncidents(base+360))
	require.NoError(t, DB.Model(&HwIncident{}).Where("ended_at = 0").Count(&count).Error)
	assert.Zero(t, count, "one bad minute must not open a new incident")
}

func TestHwStatusIncidentShorterThanThresholdExtendsNothing(t *testing.T) {
	hwStatusTestSetup(t)
	hwCreditTestSetup(t)
	now := int64(1_800_000_000)
	user := createHwTestUser(t, 0)
	_, err := GrantCreditPackage(nil, user.Id, 100, HwSourcePurchase, "status-14", now-3600)
	require.NoError(t, err)
	pkg := hwTestPackageByOrderRef(t, "status-14")

	// Fourteen minutes: more than nothing, but not "more than fifteen".
	incident := hwStatusTestIncident(t, now, 14*60)
	extended, err := HwExtendCreditPackagesAfterIncident(incident.Id, now+14*60)
	require.NoError(t, err)
	assert.Zero(t, extended)

	var rows int64
	require.NoError(t, DB.Model(&HwCreditExtension{}).Count(&rows).Error)
	assert.Zero(t, rows)

	// The incident is marked handled all the same: the decision was taken.
	var stored HwIncident
	require.NoError(t, DB.First(&stored, incident.Id).Error)
	assert.True(t, stored.Handled)

	views, err := ListUserCreditPackages(user.Id)
	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, pkg.ExpiresAt, views[0].ExpiresAt)
}

func TestHwStatusIncidentOverThresholdExtendsPaidOnlyOnce(t *testing.T) {
	hwStatusTestSetup(t)
	hwCreditTestSetup(t)
	now := int64(1_800_000_000)
	user := createHwTestUser(t, 0)
	_, err := GrantCreditPackage(nil, user.Id, 100, HwSourcePurchase, "status-16", now-3600)
	require.NoError(t, err)
	paid := hwTestPackageByOrderRef(t, "status-16")
	_, err = GrantCreditPackage(nil, user.Id, 20, HwSourceFree, "status-free", now-3600)
	require.NoError(t, err)
	free := hwTestPackageByOrderRef(t, "status-free")

	// Sixteen minutes and a half round up to seventeen whole minutes.
	incident := hwStatusTestIncident(t, now, 16*60+30)
	extended, err := HwExtendCreditPackagesAfterIncident(incident.Id, now+16*60+30)
	require.NoError(t, err)
	assert.Equal(t, 1, extended, "only the paid package is extended")

	var extension HwCreditExtension
	require.NoError(t, DB.Where("incident_id = ?", incident.Id).First(&extension).Error)
	assert.Equal(t, paid.Id, extension.PackageId)
	assert.Equal(t, int64(17), extension.Minutes)

	// Second run for the same incident: nothing changes.
	extended, err = HwExtendCreditPackagesAfterIncident(incident.Id, now+3600)
	require.NoError(t, err)
	assert.Zero(t, extended)
	var rows int64
	require.NoError(t, DB.Model(&HwCreditExtension{}).Count(&rows).Error)
	assert.Equal(t, int64(1), rows)

	// The sweep finds nothing left to do either.
	swept, err := HwExtendPendingIncidents(now + 7200)
	require.NoError(t, err)
	assert.Zero(t, swept)

	// The derived fill shows the extension on the paid package only.
	views, err := ListUserCreditPackages(user.Id)
	require.NoError(t, err)
	require.Len(t, views, 2)
	var paidView, freeView HwPackageView
	for _, view := range views {
		switch view.Id {
		case paid.Id:
			paidView = view
		case free.Id:
			freeView = view
		}
	}
	assert.Equal(t, paid.ExpiresAt+17*60, paidView.ExpiresAt)
	assert.Equal(t, free.ExpiresAt, freeView.ExpiresAt)
}

// The extension of an outage over many packages must not hold the incident row
// lock for one statement per package: the rows are written in bounded batches,
// every package is extended exactly once, and a repeated run adds nothing.
func TestHwStatusExtensionOfManyPackagesWritesInBatches(t *testing.T) {
	hwStatusTestSetup(t)
	hwCreditTestSetup(t)
	now := int64(1_800_000_000)
	user := createHwTestUser(t, 0)

	const packageCount = 1500
	packages := make([]HwCreditPackage, 0, packageCount)
	for i := range packageCount {
		packages = append(packages, HwCreditPackage{
			UserId:       user.Id,
			Source:       HwSourcePurchase,
			GrantedQuota: 100,
			CreatedAt:    now - 3600,
			ExpiresAt:    now + 24*3600,
			OrderRef:     fmt.Sprintf("status-bulk-%04d", i),
		})
	}
	require.NoError(t, DB.CreateInBatches(packages, 500).Error)

	inserts := 0
	callbackName := "test:hw_status_extension_inserts"
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == (HwCreditExtension{}).TableName() {
			inserts++
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Create().Remove(callbackName) })

	incident := hwStatusTestIncident(t, now, 16*60)
	extended, err := HwExtendCreditPackagesAfterIncident(incident.Id, now+16*60)
	require.NoError(t, err)
	assert.Equal(t, packageCount, extended, "every running package is extended")
	assert.LessOrEqual(t, inserts, 10, "the extensions are written in bounded batches, not one statement per package")

	var rows int64
	require.NoError(t, DB.Model(&HwCreditExtension{}).Where("incident_id = ?", incident.Id).Count(&rows).Error)
	assert.EqualValues(t, packageCount, rows)
	var minutes []int64
	require.NoError(t, DB.Model(&HwCreditExtension{}).Where("incident_id = ?", incident.Id).Distinct().Pluck("minutes", &minutes).Error)
	assert.Equal(t, []int64{16}, minutes, "sixteen minutes of outage extend every package by sixteen")

	// A second run adds nothing: the incident is handled and the unique index
	// refuses a duplicate row.
	extended, err = HwExtendCreditPackagesAfterIncident(incident.Id, now+3600)
	require.NoError(t, err)
	assert.Zero(t, extended)
	require.NoError(t, DB.Model(&HwCreditExtension{}).Where("incident_id = ?", incident.Id).Count(&rows).Error)
	assert.EqualValues(t, packageCount, rows)
}

func TestHwStatusExtensionKeepsExtendedPackageFromRetirement(t *testing.T) {
	hwStatusTestSetup(t)
	hwCreditTestSetup(t)
	start := int64(1_800_000_000)
	user := createHwTestUser(t, 0)
	// A package that would expire ten minutes after the outage ended.
	_, err := GrantCreditPackage(nil, user.Id, 50, HwSourcePurchase, "status-retire", start-3600)
	require.NoError(t, err)
	pkg := hwTestPackageByOrderRef(t, "status-retire")
	require.NoError(t, DB.Model(&HwCreditPackage{}).Where("id = ?", pkg.Id).
		Update("expires_at", start+20*60).Error)

	incident := hwStatusTestIncident(t, start, 16*60)
	_, err = HwExtendCreditPackagesAfterIncident(incident.Id, start+16*60)
	require.NoError(t, err)

	// Before the extension runs out the package stays active...
	retired, err := RetireExpiredPackages(start + 20*60)
	require.NoError(t, err)
	assert.Zero(t, retired)
	assert.Equal(t, 50, getUserQuotaFromDB(t, user.Id))

	// ...and after it runs out it retires as usual.
	retired, err = RetireExpiredPackages(start + 20*60 + 17*60 + 1)
	require.NoError(t, err)
	assert.Equal(t, 1, retired)
	assert.Zero(t, getUserQuotaFromDB(t, user.Id))
}

func TestHwStatusEvaluateMinute(t *testing.T) {
	cases := []struct {
		name     string
		failed   int64
		measured int64
		ok       bool
		reason   string
	}{
		{"no traffic is idle, not bad", 0, 0, false, ""},
		{"all failed", 12, 12, false, HwStatusReasonRelayFailures},
		{"one customer retrying a broken request is no outage", 2, 0, true, ""},
		{"a few failures in a busy minute are fine", 3, 200, true, ""},
		{"few failures in a quiet minute say little", 4, 20, true, ""},
		{"a real failing share", 30, 100, false, HwStatusReasonRelayFailures},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verdict := HwEvaluateStatusMinute(tc.failed, tc.measured)
			assert.Equal(t, tc.ok, verdict.Ok)
			assert.Equal(t, tc.reason, verdict.Reason)
		})
	}
}

// A quiet night must never open an outage: with no paid traffic every minute is
// judged by the route, and a routed model stays good (the 38-hour false outage
// of Oct 2026).
func TestHwStatusQuietNightOpensNoOutage(t *testing.T) {
	hwStatusTestSetup(t)
	require.NoError(t, DB.AutoMigrate(&Ability{}, &Log{}))
	require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
	t.Cleanup(func() { DB.Exec("DELETE FROM abilities") })
	require.NoError(t, DB.Create(&Ability{Group: HwPaidGroupName, Model: "claude-sonnet-5-5", ChannelId: 1, Enabled: true}).Error)

	base := int64(1_800_000_000)
	models := []string{"claude-sonnet-5-5", "claude-unrouted"}
	for i := int64(1); i <= 5; i++ {
		require.NoError(t, HwMeasureStatusOnce(time.Unix(base+i*60, 0), models))
	}
	var good, noRoute int64
	require.NoError(t, DB.Model(&HwStatusSample{}).Where("model = ? AND ok = ?", "claude-sonnet-5-5", true).Count(&good).Error)
	require.NoError(t, DB.Model(&HwStatusSample{}).Where("model = ? AND reason = ?", "claude-unrouted", HwStatusReasonNoRoute).Count(&noRoute).Error)
	assert.EqualValues(t, 5, good, "a routed model is good in a quiet minute")
	assert.EqualValues(t, 5, noRoute, "a model without an enabled route is bad")
}

// A quiet minute is decided by the mini call, not by the route: a model whose
// upstream answers 503 keeps its enabled channel, so the route alone reported it
// operational for days (24-26 Oct 2026). With a probe installed only a 200 is
// good, and a model without a route but with a working upstream is good.
func TestHwStatusQuietMinuteProbesTheUpstream(t *testing.T) {
	hwStatusTestSetup(t)
	require.NoError(t, DB.AutoMigrate(&Ability{}, &Log{}))
	require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
	t.Cleanup(func() { DB.Exec("DELETE FROM abilities") })
	require.NoError(t, DB.Create(&Ability{Group: HwPaidGroupName, Model: "claude-sonnet-5-5", ChannelId: 1, Enabled: true}).Error)
	t.Cleanup(func() { SetHwStatusProbe(nil, 0) })

	base := int64(1_800_000_000)
	models := []string{"claude-sonnet-5-5", "claude-unrouted"}

	// The upstream of the routed model does not answer; the unrouted one does.
	SetHwStatusProbe(func(ctx context.Context, modelName string) bool {
		return modelName != "claude-sonnet-5-5"
	}, 0)
	for i := int64(1); i <= 5; i++ {
		require.NoError(t, HwMeasureStatusOnce(time.Unix(base+i*60, 0), models))
	}
	var failed, good int64
	require.NoError(t, DB.Model(&HwStatusSample{}).Where("model = ? AND ok = ? AND reason = ?", "claude-sonnet-5-5", false, HwStatusReasonProbeFailed).Count(&failed).Error)
	require.NoError(t, DB.Model(&HwStatusSample{}).Where("model = ? AND ok = ?", "claude-unrouted", true).Count(&good).Error)
	assert.EqualValues(t, 5, failed, "a model whose upstream fails is bad, whatever the route says")
	assert.EqualValues(t, 5, good, "a model whose upstream answers is good, routed or not")

	// Without a probe the route decides again, so an environment without a key
	// never reports a false outage.
	SetHwStatusProbe(nil, 0)
	require.NoError(t, HwMeasureStatusOnce(time.Unix(base+6*60, 0), models))
	var probeFree int64
	require.NoError(t, DB.Model(&HwStatusSample{}).Where("minute = ? AND model = ? AND ok = ?", base+6*60, "claude-sonnet-5-5", true).Count(&probeFree).Error)
	assert.EqualValues(t, 1, probeFree, "without a probe the route decides, as before")
}

// The mini call writes its own rows in the log table the measurement reads, so
// its traffic must stay out of the customer count. Left in, a failed probe kept
// the next minute from being idle, the failure never rose above one, the run of
// three bad minutes never happened and an outage of a quiet upstream stayed
// invisible (the 503 of 24-26 Oct 2026 that /api/hw/status reported as
// operational). The counter-check keeps a customer row counting.
func TestHwStatusProbeTrafficStaysOutOfTheCount(t *testing.T) {
	hwStatusTestSetup(t)
	require.NoError(t, LOG_DB.AutoMigrate(&Log{}))
	require.NoError(t, LOG_DB.Exec("DELETE FROM logs").Error)
	t.Cleanup(func() { LOG_DB.Exec("DELETE FROM logs") })
	t.Cleanup(func() { SetHwStatusProbe(nil, 0) })

	const probeTokenId = 7
	base := int64(1_800_000_000)
	models := []string{"claude-sonnet-5-5"}

	// The pass runs at the start of its minute, so the row a probe writes lands
	// in the window of the next pass, exactly as in production.
	probeRowAt := base + 60
	SetHwStatusProbe(func(ctx context.Context, modelName string) bool {
		require.NoError(t, LOG_DB.Create(&Log{
			Type: LogTypeError, CreatedAt: probeRowAt, Group: HwPaidGroupName,
			ModelName: modelName, TokenId: probeTokenId, RequestId: "probe",
		}).Error)
		probeRowAt += 60
		return false
	}, probeTokenId)

	for i := int64(1); i <= 10; i++ {
		require.NoError(t, HwMeasureStatusOnce(time.Unix(base+i*60, 0), models))
		if i == 3 {
			var opened int64
			require.NoError(t, DB.Model(&HwIncident{}).Where("ended_at = 0").Count(&opened).Error)
			assert.EqualValues(t, 1, opened, "three bad minutes of a quiet upstream open the outage")
		}
	}
	var bad int64
	require.NoError(t, DB.Model(&HwStatusSample{}).
		Where("model = ? AND ok = ? AND reason = ?", "claude-sonnet-5-5", false, HwStatusReasonProbeFailed).
		Count(&bad).Error)
	assert.EqualValues(t, 10, bad, "every silent minute with a failing probe is a bad sample")

	// Counter-check: the error rows of a customer token do count. Three of them
	// in one window are customer failures, so that minute is not idle and no
	// probe decides it.
	SetHwStatusProbe(nil, 0)
	for i := 0; i < 3; i++ {
		require.NoError(t, LOG_DB.Create(&Log{
			Type: LogTypeError, CreatedAt: base + 661, Group: HwPaidGroupName,
			ModelName: "claude-sonnet-5-5", TokenId: 99,
			RequestId: fmt.Sprintf("customer-%d", i),
		}).Error)
	}
	require.NoError(t, HwMeasureStatusOnce(time.Unix(base+12*60, 0), models))
	var customerBad int64
	require.NoError(t, DB.Model(&HwStatusSample{}).
		Where("minute = ? AND model = ? AND ok = ? AND reason = ?", base+12*60, "claude-sonnet-5-5", false, HwStatusReasonRelayFailures).
		Count(&customerBad).Error)
	assert.EqualValues(t, 1, customerBad, "an error row of a customer token still counts")
}

// The sweep removes an outage made only of quiet minutes and keeps a real one.
func TestHwStatusPurgeFalseIdleOutages(t *testing.T) {
	hwStatusTestSetup(t)
	base := int64(1_800_000_000)
	for i := int64(0); i < 5; i++ {
		hwStatusTestSample(t, base+i*60, false, HwStatusReasonMeasurement)
	}
	false1 := hwStatusTestIncident(t, base, 300)
	realStart := base + 3600
	for i := int64(0); i < 3; i++ {
		hwStatusTestSample(t, realStart+i*60, false, HwStatusReasonRelayFailures)
	}
	real1 := hwStatusTestIncident(t, realStart, 180)
	// A lone failure inside a long quiet outage does not make it real.
	lonely := HwIncident{StartedAt: base + 7200, Kind: HwStatusIncidentRelay}
	require.NoError(t, DB.Create(&lonely).Error)
	hwStatusTestSample(t, base+7260, false, HwStatusReasonRelayFailures)
	// An outage older than the sample retention has lost its evidence, not its truth.
	oldStart := time.Now().Unix() - hwStatusSampleRetentionSeconds - 24*3600
	old := hwStatusTestIncident(t, oldStart, 600)

	removed, err := HwPurgeFalseIdleOutages()
	require.NoError(t, err)
	assert.Equal(t, 2, removed)
	var left []HwIncident
	require.NoError(t, DB.Order("id").Find(&left).Error)
	require.Len(t, left, 2)
	assert.Equal(t, real1.Id, left[0].Id)
	assert.Equal(t, old.Id, left[1].Id)
	assert.NotEqual(t, false1.Id, left[0].Id)
	var idle int64
	require.NoError(t, DB.Model(&HwStatusSample{}).Where("reason = ?", HwStatusReasonMeasurement).Count(&idle).Error)
	assert.Zero(t, idle)

	again, err := HwPurgeFalseIdleOutages()
	require.NoError(t, err)
	assert.Zero(t, again)
}

// One broken model must not turn the others red: the verdict is per model (22
// Oct 2026: a 404 on claude-haiku-4-5 marked all ten models bad).
func TestHwStatusOneBrokenModelStaysAlone(t *testing.T) {
	hwStatusTestSetup(t)
	require.NoError(t, DB.AutoMigrate(&Ability{}, &Log{}))
	require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, LOG_DB.Exec("DELETE FROM logs").Error)
	t.Cleanup(func() {
		DB.Exec("DELETE FROM abilities")
		LOG_DB.Exec("DELETE FROM logs")
	})
	require.NoError(t, DB.Create(&Ability{Group: HwPaidGroupName, Model: "claude-quiet", ChannelId: 1, Enabled: true}).Error)

	base := int64(1_800_000_000)
	at := base + 60
	for i := range 3 {
		// Three failed attempts of three different customer requests, as the relay
		// writes them: each attempt carries its own request id.
		require.NoError(t, LOG_DB.Create(&Log{Type: LogTypeError, CreatedAt: base + 5, Group: HwPaidGroupName, ModelName: "claude-broken", RequestId: fmt.Sprintf("broken-%d", i)}).Error)
	}
	require.NoError(t, LOG_DB.Create(&Log{Type: LogTypeConsume, CreatedAt: base + 5, Group: HwPaidGroupName, ModelName: "claude-busy", RequestId: "busy-served"}).Error)
	// Another group's failure does not count for the paid status.
	require.NoError(t, LOG_DB.Create(&Log{Type: LogTypeError, CreatedAt: base + 5, Group: "free", ModelName: "claude-busy", RequestId: "busy-free"}).Error)

	require.NoError(t, HwMeasureStatusOnce(time.Unix(at, 0), []string{"claude-broken", "claude-busy", "claude-quiet"}))
	var samples []HwStatusSample
	require.NoError(t, DB.Order("model").Find(&samples).Error)
	require.Len(t, samples, 3)
	assert.False(t, samples[0].Ok, "the broken model is bad")
	assert.Equal(t, HwStatusReasonRelayFailures, samples[0].Reason)
	assert.True(t, samples[1].Ok, "a served model stays good")
	assert.True(t, samples[2].Ok, "a quiet routed model stays good")
}

// The relay writes one error row per failed attempt under the request id of the
// inbound request, so retries of one broken request must count as one failure:
// five customers whose request was retried three times are five failures, not
// fifteen (the quality meter already counts distinct request ids).
func TestHwStatusFailedAttemptsOfOneRequestCountOnce(t *testing.T) {
	hwStatusTestSetup(t)
	require.NoError(t, LOG_DB.Exec("DELETE FROM logs").Error)
	t.Cleanup(func() { LOG_DB.Exec("DELETE FROM logs") })

	base := int64(1_800_000_000)
	for customer := range 5 {
		for range 3 {
			require.NoError(t, LOG_DB.Create(&Log{
				Type: LogTypeError, CreatedAt: base + 5, Group: HwPaidGroupName,
				ModelName: "claude-broken", RequestId: fmt.Sprintf("customer-%d", customer),
			}).Error)
		}
		// One served request of the same model, so the minute is not all failed.
		require.NoError(t, LOG_DB.Create(&Log{
			Type: LogTypeConsume, CreatedAt: base + 5, Group: HwPaidGroupName,
			ModelName: "claude-broken", RequestId: fmt.Sprintf("served-%d", customer),
		}).Error)
	}
	// An error row without a request id cannot be attributed to a request; the
	// relay always writes one, so it is left out like the quality meter does.
	require.NoError(t, LOG_DB.Create(&Log{Type: LogTypeError, CreatedAt: base + 5, Group: HwPaidGroupName, ModelName: "claude-broken"}).Error)

	counts, err := HwCountPaidRequests(base, base+60)
	require.NoError(t, err)
	assert.Equal(t, int64(5), counts["claude-broken"].Failed, "retries of one request are one failure")
	assert.Equal(t, int64(5), counts["claude-broken"].Measured)
}

// Customer groups always write an error row per failed attempt, also when a
// later attempt of the same request is served (a fallback). Such a request
// reached the customer, so it is no failure of the status meter.
func TestHwStatusServedFallbackIsNoFailure(t *testing.T) {
	hwStatusTestSetup(t)
	require.NoError(t, LOG_DB.Exec("DELETE FROM logs").Error)
	t.Cleanup(func() { LOG_DB.Exec("DELETE FROM logs") })

	base := int64(1_800_000_000)
	rows := []Log{
		{Type: LogTypeError, CreatedAt: base + 5, Group: HwPaidGroupName, ModelName: "claude-x", RequestId: "recovered"},
		{Type: LogTypeConsume, CreatedAt: base + 6, Group: HwPaidGroupName, ModelName: "claude-x", RequestId: "recovered"},
		{Type: LogTypeError, CreatedAt: base + 7, Group: HwPaidGroupName, ModelName: "claude-x", RequestId: "lost"},
	}
	for i := range rows {
		require.NoError(t, LOG_DB.Create(&rows[i]).Error)
	}

	counts, err := HwCountPaidRequests(base, base+60)
	require.NoError(t, err)
	assert.Equal(t, int64(1), counts["claude-x"].Failed, "only the request no attempt served is a failure")
	assert.Equal(t, int64(1), counts["claude-x"].Measured)
}

func TestHwStatusCodeAndAvailabilityPercent(t *testing.T) {
	cases := []struct {
		name    string
		state   HwStatusModelState
		code    string
		percent float64
	}{
		{"no samples is not claimed healthy", HwStatusModelState{}, "operational", 0},
		{"all good", HwStatusModelState{GoodMinutes: 60, TotalMinutes: 60}, "operational", 100},
		{"recent bad minute is degraded", HwStatusModelState{BadRecent: true, GoodMinutes: 2, TotalMinutes: 3}, "degraded", 66.67},
		{"open incident wins over degraded", HwStatusModelState{InOpenIncident: true, BadRecent: true, GoodMinutes: 1, TotalMinutes: 3}, "outage", 33.33},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.code, HwStatusCodeOf(tc.state))
			assert.InDelta(t, tc.percent, HwStatusAvailabilityPercent(tc.state), 0.001)
		})
	}
}

func TestHwStatusMinuteOfRoundsDownToTheMinute(t *testing.T) {
	at := time.Unix(1_800_000_059, 0)
	assert.Equal(t, at.Unix()-at.Unix()%60, HwStatusMinuteOf(at))
	assert.Equal(t, int64(0), HwStatusMinuteOf(at)%60)
}

func TestHwPurgeOldStatusSamplesKeepsTheWindowAndIsIdempotent(t *testing.T) {
	hwStatusTestSetup(t)
	now := int64(1_800_000_000)
	oldMinute := now - hwStatusSampleRetentionSeconds - 120
	keepMinute := now - hwStatusSampleRetentionSeconds + 120
	hwStatusTestSample(t, oldMinute, true, "")
	hwStatusTestSample(t, keepMinute, true, "")

	require.Error(t, PurgeOldStatusSamples(0))
	for i := 0; i < 2; i++ {
		require.NoError(t, PurgeOldStatusSamples(now))
		var count int64
		require.NoError(t, DB.Model(&HwStatusSample{}).Count(&count).Error)
		assert.Equal(t, int64(1), count)
	}
	var left HwStatusSample
	require.NoError(t, DB.First(&left).Error)
	assert.Equal(t, keepMinute, left.Minute)
}

func TestHwReadStatusSnapshotCountsWindowRecentBadAndOpenIncident(t *testing.T) {
	hwStatusTestSetup(t)
	now := int64(1_800_000_000)
	_, err := HwReadStatusSnapshot(0, []string{"claude-sonnet-5-5"})
	require.Error(t, err)

	// Quiet meter: nothing sampled, nothing claimed.
	snap, err := HwReadStatusSnapshot(now, []string{"claude-sonnet-5-5"})
	require.NoError(t, err)
	require.Len(t, snap.Models, 1)
	assert.Equal(t, HwStatusModelState{Model: "claude-sonnet-5-5"}, snap.Models[0])
	assert.Empty(t, snap.Incidents)

	// Two good minutes long ago, then a bad one inside the recovery window, with
	// an incident that is still open.
	hwStatusTestSample(t, now-3600, true, "")
	hwStatusTestSample(t, now-3540, true, "")
	hwStatusTestSample(t, now-60, false, "relay")
	require.NoError(t, DB.Create(&HwIncident{StartedAt: now - 120, Kind: HwStatusIncidentRelay}).Error)

	snap, err = HwReadStatusSnapshot(now, []string{"claude-sonnet-5-5", "unsampled-model"})
	require.NoError(t, err)
	require.Len(t, snap.Models, 2)
	got := snap.Models[0]
	assert.Equal(t, int64(3), got.TotalMinutes)
	assert.Equal(t, int64(2), got.GoodMinutes)
	assert.True(t, got.BadRecent)
	assert.True(t, got.InOpenIncident)
	assert.Equal(t, "outage", HwStatusCodeOf(got))
	assert.Equal(t, HwStatusModelState{Model: "unsampled-model"}, snap.Models[1])
	assert.Len(t, snap.Incidents, 1)
}
