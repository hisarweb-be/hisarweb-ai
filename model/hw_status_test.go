// The status meter and the automatic credit extension, pinned at the storage
// layer where the decisions are made: the minute evaluation, the incident
// chain (three bad minutes), the "more than fifteen minutes" rule, the
// once-per-incident extension in a transaction, the free trial credit staying
// untouched, and the derived fill respecting the extension.

package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	removed, err := HwPurgeFalseIdleOutages()
	require.NoError(t, err)
	assert.Equal(t, 2, removed)
	var left []HwIncident
	require.NoError(t, DB.Find(&left).Error)
	require.Len(t, left, 1)
	assert.Equal(t, real1.Id, left[0].Id)
	assert.NotEqual(t, false1.Id, left[0].Id)
	var idle int64
	require.NoError(t, DB.Model(&HwStatusSample{}).Where("reason = ?", HwStatusReasonMeasurement).Count(&idle).Error)
	assert.Zero(t, idle)

	again, err := HwPurgeFalseIdleOutages()
	require.NoError(t, err)
	assert.Zero(t, again)
}
