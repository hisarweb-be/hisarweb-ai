package service

import (
	"strings"
	"testing"
	"time"

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
