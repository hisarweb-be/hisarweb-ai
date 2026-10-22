package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hwQualityTestSetup brings the quality tables into the shared test database
// and clears them.
func hwQualityTestSetup(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.AutoMigrate(&model.HwQualityRun{}, &model.HwQualityScore{}))
	require.NoError(t, model.DB.Exec("DELETE FROM hw_quality_scores").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM hw_quality_runs").Error)
	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM hw_quality_scores")
		model.DB.Exec("DELETE FROM hw_quality_runs")
	})
}

// TestHwQualityTasksetCoversLanguagesAndKinds guards the embedded set: at
// least ten tasks, Turkish and English, all four kinds, unique ids.
func TestHwQualityTasksetCoversLanguagesAndKinds(t *testing.T) {
	tasks, err := HwQualityTasks()
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(tasks), 10)
	kinds := map[string]bool{}
	languages := map[string]bool{}
	for _, task := range tasks {
		kinds[task.Kind] = true
		languages[task.Language] = true
		assert.NotEmpty(t, task.Criteria)
		assert.NotEmpty(t, task.Prompt)
	}
	for _, kind := range []string{"math", "code", "summary", "translation"} {
		assert.True(t, kinds[kind], "task set must cover kind %s", kind)
	}
	assert.True(t, languages["tr"])
	assert.True(t, languages["en"])
}

// hwQualityFakeGateway doubles the customer route: it answers model calls with
// a fixed answer and token counts, and judge calls with a fixed score. Every
// judge request body is kept so the test can prove the judge is blind.
type hwQualityFakeGateway struct {
	server      *httptest.Server
	mu          sync.Mutex
	judgeBodies []string
	answerCount int
}

func newHwQualityFakeGateway(t *testing.T, judgeScore int, answer string) *hwQualityFakeGateway {
	t.Helper()
	gateway := &hwQualityFakeGateway{}
	gateway.server = hwStartTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		// The handler runs on its own goroutine, so failures are recorded and
		// answered as an error instead of failing the test directly.
		if !assert.Equal(t, "/v1/chat/completions", r.URL.Path) ||
			!assert.Equal(t, "Bearer hw-test-key-not-a-real-secret", r.Header.Get("Authorization")) {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read failed", http.StatusBadRequest)
			return
		}
		var request struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if !assert.NoError(t, common.Unmarshal(body, &request)) {
			http.Error(w, "unreadable body", http.StatusBadRequest)
			return
		}
		if request.Model == "hw-test-judge" {
			gateway.mu.Lock()
			gateway.judgeBodies = append(gateway.judgeBodies, string(body))
			gateway.mu.Unlock()
			// The judge answers through the customer route too: a normal chat
			// completion whose content is the JSON verdict.
			verdict, err := common.Marshal(fmt.Sprintf(`{"score": %d}`, judgeScore))
			if err != nil {
				http.Error(w, "marshal failed", http.StatusInternalServerError)
				return
			}
			writeJSON(w, `{"choices":[{"message":{"role":"assistant","content":`+string(verdict)+`}}],`+
				`"usage":{"prompt_tokens":80,"completion_tokens":8}}`)
			return
		}
		gateway.mu.Lock()
		gateway.answerCount++
		count := gateway.answerCount
		gateway.mu.Unlock()
		quoted, err := common.Marshal(answer + fmt.Sprintf("-%d", count))
		if err != nil {
			http.Error(w, "marshal failed", http.StatusInternalServerError)
			return
		}
		writeJSON(w, `{"choices":[{"message":{"role":"assistant","content":`+
			string(quoted)+`}}],`+
			`"usage":{"prompt_tokens":100,"completion_tokens":50}}`)
	})
	return gateway
}

func writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

// TestHwQualityRunMeasuresJudgesBlindAndWritesFile runs the whole meter against
// the fake gateway: scores, tokens, and latency per task; a judge that never
// sees the measured model's name; and the JSON measurement file on disk.
func TestHwQualityRunMeasuresJudgesBlindAndWritesFile(t *testing.T) {
	hwQualityTestSetup(t)
	gateway := newHwQualityFakeGateway(t, 7, "hw sample answer")
	outDir := t.TempDir()
	clockStep := int64(0)
	nowFunc := func() time.Time {
		clockStep++
		return time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC).Add(time.Duration(clockStep) * 7 * time.Millisecond)
	}

	run, err := HwRunQualityMeasurement(context.Background(), HwQualityRunOptions{
		BaseURL:    gateway.server.URL,
		APIKey:     "hw-test-key-not-a-real-secret",
		JudgeModel: "hw-test-judge",
		Models:     []string{"claude-sonnet-5-5", "gpt-6.1-sol"},
		OutDir:     outDir,
		Client:     gateway.server.Client(),
		NowFunc:    nowFunc,
	})
	require.NoError(t, err)

	tasks, err := HwQualityTasks()
	require.NoError(t, err)

	var scores []model.HwQualityScore
	require.NoError(t, model.DB.Where("run_id = ?", run.Id).Order("model, task_id").Find(&scores).Error)
	require.Len(t, scores, 2*len(tasks))
	for _, score := range scores {
		assert.Equal(t, 7, score.Score)
		assert.Equal(t, 100, score.TokensIn)
		assert.Equal(t, 50, score.TokensOut)
		assert.Greater(t, score.LatencyMs, int64(0))
		assert.Contains(t, []string{model.HwQualityReferencePending, model.HwQualityReferenceNone}, score.Reference)
		if strings.HasPrefix(score.Model, "claude") {
			assert.Equal(t, model.HwQualityReferencePending, score.Reference)
		} else {
			assert.Equal(t, model.HwQualityReferenceNone, score.Reference)
		}
	}

	// The measurement file exists and carries every score back to disk.
	filePath := filepath.Join(outDir, run.FileName)
	content, err := os.ReadFile(filePath)
	require.NoError(t, err)
	var file struct {
		GeneratedAt    int64  `json:"generated_at"`
		TasksetVersion string `json:"taskset_version"`
		JudgeModel     string `json:"judge_model"`
		SampleNote     string `json:"sample_note"`
		Models         []struct {
			Model     string `json:"model"`
			Reference string `json:"reference"`
			Tasks     []struct {
				TaskId    string `json:"task_id"`
				Score     int    `json:"score"`
				TokensIn  int    `json:"tokens_in"`
				TokensOut int    `json:"tokens_out"`
			} `json:"tasks"`
		} `json:"models"`
	}
	require.NoError(t, common.Unmarshal(content, &file))
	require.Len(t, file.Models, 2)
	for _, measured := range file.Models {
		require.Len(t, measured.Tasks, len(tasks))
		for _, taskResult := range measured.Tasks {
			assert.Equal(t, 7, taskResult.Score)
			assert.Equal(t, 100, taskResult.TokensIn)
			assert.Equal(t, 50, taskResult.TokensOut)
		}
	}
	assert.NotEmpty(t, file.SampleNote)
	assert.NotEmpty(t, file.TasksetVersion)

	// The judge is blind: its requests carry the task and the answer, never
	// the name of the measured model.
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	require.Len(t, gateway.judgeBodies, 2*len(tasks))
	for _, body := range gateway.judgeBodies {
		lower := strings.ToLower(body)
		assert.NotContains(t, lower, "claude-sonnet-5-5")
		assert.NotContains(t, lower, "gpt-6.1-sol")
		assert.Contains(t, body, "hw sample answer")
		assert.Contains(t, body, "Grading criteria")
		// The primary supplier drops customer system messages, so the grading
		// instruction must reach the judge inside the user message.
		assert.NotContains(t, body, `"role":"system"`)
		assert.Contains(t, body, "You are a grader")
	}

	// The public summary aggregates the same run.
	summary, err := HwLatestQualitySummary()
	require.NoError(t, err)
	require.Len(t, summary.Models, 2)
	assert.Equal(t, run.FileName, summary.FileName)
	assert.Equal(t, 7.0, summary.Models[0].AvgScore)
	for _, measured := range summary.Models {
		assert.Len(t, measured.Tasks, len(tasks))
		assert.Greater(t, measured.AvgLatencyMs, int64(0))
	}
	assert.NotEmpty(t, summary.Note)
	assert.NotEmpty(t, summary.NoteEn)
}

// TestHwQualityLatestWithoutRunIsEmpty pins the empty answer: no run, no
// invented values, but the sample note stays.
func TestHwQualityLatestWithoutRunIsEmpty(t *testing.T) {
	hwQualityTestSetup(t)
	summary, err := HwLatestQualitySummary()
	require.NoError(t, err)
	require.NotNil(t, summary.Models)
	assert.Empty(t, summary.Models)
	assert.Equal(t, int64(0), summary.MeasuredAt)
	assert.Empty(t, summary.FileName)
	assert.NotEmpty(t, summary.Note)
}

// TestHwQualityRunRefusesIncompleteConfig keeps the meter from starting with a
// missing endpoint, key, judge, or output directory.
func TestHwQualityRunRefusesIncompleteConfig(t *testing.T) {
	hwQualityTestSetup(t)
	_, err := HwRunQualityMeasurement(context.Background(), HwQualityRunOptions{
		BaseURL: "http://localhost:1",
		APIKey:  "hw-test-key-not-a-real-secret",
	})
	require.Error(t, err)
	var count int64
	require.NoError(t, model.DB.Model(&model.HwQualityRun{}).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}
