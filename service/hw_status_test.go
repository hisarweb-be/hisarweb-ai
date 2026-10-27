package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The public status answer is customer-facing: the overall state is the worst
// model state, an unfinished outage is the running incident, finished ones go
// to the recent list, and the texts never quote a supplier or an error.
func TestHwBuildStatusAnswerAggregatesTheWorstStateAndSplitsIncidents(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	snapshot := model.HwStatusSnapshot{
		Models: []model.HwStatusModelState{
			{Model: "own-a", GoodMinutes: 9990, TotalMinutes: 10000},
			{Model: "own-b", BadRecent: true, GoodMinutes: 1, TotalMinutes: 3},
			{Model: "own-c", InOpenIncident: true, BadRecent: true},
		},
		Incidents: []model.HwIncident{
			{StartedAt: 100, EndedAt: 0, Duration: 0},
			{StartedAt: 50, EndedAt: 80, Duration: 30},
			{StartedAt: 10, EndedAt: 0, Duration: 0},
		},
	}

	answer := HwBuildStatusAnswer(now, snapshot)

	assert.Equal(t, now.Unix(), answer.CheckedAt)
	assert.Equal(t, model.HwStatusAvailabilityDays, answer.AvailabilityDays)
	assert.Equal(t, HwStatusOutage, answer.Overall, "an outage outranks a degraded model")
	require.Len(t, answer.Models, 3)
	assert.Equal(t, HwStatusOperational, answer.Models[0].Status)
	assert.Equal(t, 99.9, answer.Models[0].AvailabilityPercent)
	assert.Equal(t, HwStatusDegraded, answer.Models[1].Status)
	assert.Equal(t, 33.33, answer.Models[1].AvailabilityPercent)
	assert.Equal(t, HwStatusOutage, answer.Models[2].Status)
	assert.Zero(t, answer.Models[2].AvailabilityPercent, "no samples is not claimed healthy")

	require.NotNil(t, answer.Incident)
	assert.Equal(t, int64(100), answer.Incident.StartedAt, "the first unfinished incident is the running one")
	assert.Contains(t, answer.Incident.MessageEN, "ongoing")
	require.Len(t, answer.Recent, 1)
	assert.Equal(t, int64(30), answer.Recent[0].Duration)
	assert.Contains(t, answer.Recent[0].MessageEN, "ended")
}

func TestHwBuildStatusAnswerOverallStaysOperationalOrDegraded(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)

	empty := HwBuildStatusAnswer(now, model.HwStatusSnapshot{})
	assert.Equal(t, HwStatusOperational, empty.Overall)
	assert.NotNil(t, empty.Models, "an empty list encodes as [] and not null")
	assert.NotNil(t, empty.Recent)
	assert.Nil(t, empty.Incident)

	degraded := HwBuildStatusAnswer(now, model.HwStatusSnapshot{Models: []model.HwStatusModelState{
		{Model: "own-a"}, {Model: "own-b", BadRecent: true},
	}})
	assert.Equal(t, HwStatusDegraded, degraded.Overall)
}

// Every state has a Turkish and an English text, and none of them can leak
// where the traffic goes.
func TestHwStatusTextsAreFixedAndSupplierFree(t *testing.T) {
	for _, state := range []string{HwStatusOperational, HwStatusDegraded, HwStatusOutage, "unknown"} {
		textTR, textEN := hwStatusModelText(state)
		assert.NotEmpty(t, textTR, state)
		assert.NotEmpty(t, textEN, state)
	}
	operationalTR, _ := hwStatusModelText("unknown")
	expectedTR, _ := hwStatusModelText(HwStatusOperational)
	assert.Equal(t, expectedTR, operationalTR, "an unknown state falls back to operational")

	for _, ended := range []bool{false, true} {
		textTR, textEN := hwStatusIncidentText(ended)
		for _, text := range []string{textTR, textEN} {
			lower := strings.ToLower(text)
			assert.NotEmpty(t, text)
			assert.NotContains(t, lower, "lumia")
			assert.NotContains(t, lower, "openrouter")
		}
	}
	ongoingTR, _ := hwStatusIncidentText(false)
	endedTR, _ := hwStatusIncidentText(true)
	assert.NotEqual(t, ongoingTR, endedTR)
}

// hwIncidentCardTestSetup gives the incident card a clean database and a board
// that records every card it receives.
func hwIncidentCardTestSetup(t *testing.T, status int) (HwBureauCardConfig, *[]hwBureauIssueRequest) {
	t.Helper()
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}, &model.HwStatusSample{}, &model.HwIncident{}))
	clean := func() {
		model.DB.Exec("DELETE FROM hw_status_samples")
		model.DB.Exec("DELETE FROM hw_incidents")
		model.DB.Where("key = ?", HwOptionIncidentCardStartedAt).Delete(&model.Option{})
		common.OptionMapRWMutex.Lock()
		delete(common.OptionMap, HwOptionIncidentCardStartedAt)
		common.OptionMapRWMutex.Unlock()
		hwIncidentCardRetryAt = time.Time{}
	}
	clean()
	t.Cleanup(clean)
	cards := &[]hwBureauIssueRequest{}
	board := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var card hwBureauIssueRequest
		require.NoError(t, common.DecodeJson(r.Body, &card))
		*cards = append(*cards, card)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"id":"x","identifier":"HIS-1"}`))
	}))
	t.Cleanup(board.Close)
	return HwBureauCardConfig{BaseURL: board.URL, CompanyID: "c1"}, cards
}

func hwIncidentCardTestOutage(t *testing.T, startedAt int64, failing []string) {
	t.Helper()
	for _, name := range failing {
		require.NoError(t, model.DB.Create(&model.HwStatusSample{
			Minute: startedAt, Model: name, MeasuredAt: startedAt, Reason: model.HwStatusReasonProbeFailed,
		}).Error)
	}
	require.NoError(t, model.DB.Create(&model.HwIncident{StartedAt: startedAt, Kind: model.HwStatusIncidentRelay}).Error)
}

// A new outage files exactly one card, with the role that matches what broke:
// a few models down is a model or supplier route (Modelverkenner), every sold
// model down is the gateway itself (Platformbeheerder). The minute job calls
// this every minute, so the second call files nothing.
func TestHwIncidentCardFilesOneCardPerOutageForTheRightRole(t *testing.T) {
	config, cards := hwIncidentCardTestSetup(t, http.StatusCreated)
	now := time.Unix(1_800_000_000, 0)

	hwIncidentCardTestOutage(t, now.Unix()-180, []string{"claude-opus-5-5"})
	require.NoError(t, hwCardNewIncident(context.Background(), config, now))
	require.NoError(t, hwCardNewIncident(context.Background(), config, now.Add(time.Minute)))
	require.Len(t, *cards, 1, "one card per outage, not one per minute")
	assert.Equal(t, HwBureauAgentModelverkenner, (*cards)[0].AssigneeAgentID)
	assert.Contains(t, (*cards)[0].Description, "claude-opus-5-5")

	require.NoError(t, model.DB.Model(&model.HwIncident{}).Where("ended_at = 0").Update("ended_at", now.Unix()).Error)
	later := now.Add(time.Hour)
	hwIncidentCardTestOutage(t, later.Unix()-180, HwSoldModelNames())
	require.NoError(t, hwCardNewIncident(context.Background(), config, later))
	require.Len(t, *cards, 2)
	assert.Equal(t, HwBureauAgentPlatformbeheerder, (*cards)[1].AssigneeAgentID)

	// A restart forgets nothing: the carded outage is kept as an option.
	hwIncidentCardRetryAt = time.Time{}
	require.NoError(t, hwCardNewIncident(context.Background(), config, later.Add(time.Minute)))
	assert.Len(t, *cards, 2)
}

// Without a board nothing is posted, and a refusing board neither fails the
// minute job forever nor gets asked again every minute.
func TestHwIncidentCardSkipsWithoutBoardAndBacksOffAfterAFailure(t *testing.T) {
	config, cards := hwIncidentCardTestSetup(t, http.StatusInternalServerError)
	now := time.Unix(1_800_000_000, 0)
	hwIncidentCardTestOutage(t, now.Unix()-180, []string{"grok-4"})

	require.NoError(t, hwCardNewIncident(context.Background(), HwBureauCardConfig{}, now))
	assert.Empty(t, *cards)

	assert.Error(t, hwCardNewIncident(context.Background(), config, now))
	require.NoError(t, hwCardNewIncident(context.Background(), config, now.Add(time.Minute)))
	assert.Len(t, *cards, 1, "a failed card is retried after the back-off, not every minute")
	assert.Error(t, hwCardNewIncident(context.Background(), config, now.Add(hwIncidentCardRetryAfter)))
	assert.Len(t, *cards, 2)
}
