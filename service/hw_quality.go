package service

// Quality meter for HisarWeb AI. A fixed task set (embedded, versioned) is run
// once per model through the customer route of the gateway itself; a blind
// judge — a second model call that sees the task, the criteria, and the answer,
// but never the name of the model that produced the answer — scores each reply
// from 0 to 10. The pass is stored in the database and written to a JSON
// measurement file, so every score shown on the site traces back to a file.
//
// Nothing here claims equality with the original first-party models. A
// claude-family model is marked "pending" (the comparison runs later through
// the owner's subscription); everything else is marked "none" (no original is
// reachable through our providers).

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"

	_ "embed"
)

// Environment variables of the measurement. The base URL points at the
// gateway's own customer route; the key is a normal customer API key.
const (
	HwQualityBaseURLEnv    = "HW_QUALITY_BASE_URL"
	HwQualityAPIKeyEnv     = "HW_QUALITY_API_KEY"
	HwQualityJudgeModelEnv = "HW_QUALITY_JUDGE_MODEL"
	HwQualityOutDirEnv     = "HW_QUALITY_OUT_DIR"
)

const (
	hwQualityCallTimeout  = 120 * time.Second
	hwQualityDefaultLimit = 2048
)

//go:embed hw_quality_taskset.json
var hwQualityTasksetJSON []byte

// HwQualityTask is one task of the fixed set.
type HwQualityTask struct {
	Id       string `json:"id"`
	Language string `json:"language"` // "tr" or "en"
	Kind     string `json:"kind"`     // math, code, summary, translation
	Prompt   string `json:"prompt"`
	// Expected is the required answer when the task has a single correct one;
	// open tasks are judged on their criteria alone.
	Expected  string   `json:"expected,omitempty"`
	Criteria  []string `json:"criteria"`
	MaxTokens int      `json:"max_tokens,omitempty"`
}

type hwQualityTaskset struct {
	TasksetVersion string          `json:"taskset_version"`
	Tasks          []HwQualityTask `json:"tasks"`
}

// HwQualityTasks decodes and sanity-checks the embedded task set.
func HwQualityTasks() ([]HwQualityTask, error) {
	var taskset hwQualityTaskset
	if err := common.Unmarshal(hwQualityTasksetJSON, &taskset); err != nil {
		return nil, fmt.Errorf("embedded task set is not valid JSON: %w", err)
	}
	if len(taskset.Tasks) < 10 {
		return nil, errors.New("embedded task set has fewer than 10 tasks")
	}
	seen := make(map[string]bool, len(taskset.Tasks))
	for _, task := range taskset.Tasks {
		if task.Id == "" || task.Prompt == "" || len(task.Criteria) == 0 {
			return nil, fmt.Errorf("task %q is incomplete", task.Id)
		}
		if seen[task.Id] {
			return nil, fmt.Errorf("task id %q appears twice", task.Id)
		}
		seen[task.Id] = true
	}
	return taskset.Tasks, nil
}

// HwQualityTasksetVersion returns the version string of the embedded set.
func HwQualityTasksetVersion() string {
	var taskset hwQualityTaskset
	if err := common.Unmarshal(hwQualityTasksetJSON, &taskset); err != nil {
		return ""
	}
	return taskset.TasksetVersion
}

// HwQualityRunOptions configures one measurement pass.
type HwQualityRunOptions struct {
	// BaseURL is the gateway's own base URL; the runner calls the customer
	// route {BaseURL}/v1/chat/completions.
	BaseURL string
	APIKey  string
	// JudgeModel is the model that scores the answers blind.
	JudgeModel string
	// Models are the own model names to measure; empty means the catalog.
	Models []string
	// OutDir is the directory the JSON measurement file is written to.
	OutDir  string
	Client  *http.Client
	NowFunc func() time.Time
}

// HwQualityRunOptionsFromEnv reads the measurement configuration from the
// environment. Missing values stay empty and make the run refuse to start.
func HwQualityRunOptionsFromEnv() HwQualityRunOptions {
	return HwQualityRunOptions{
		BaseURL:    strings.TrimSpace(os.Getenv(HwQualityBaseURLEnv)),
		APIKey:     os.Getenv(HwQualityAPIKeyEnv),
		JudgeModel: strings.TrimSpace(os.Getenv(HwQualityJudgeModelEnv)),
		OutDir:     strings.TrimSpace(os.Getenv(HwQualityOutDirEnv)),
	}
}

// hwChatMessage is one message of an OpenAI-compatible chat request.
type hwChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	// Parts carries images next to the text (OpenAI content parts). When set it
	// replaces Content in the request body.
	Parts []hwChatPart `json:"-"`
}

// hwChatPart is one text or image part of a message.
type hwChatPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *hwChatImageURL `json:"image_url,omitempty"`
}

type hwChatImageURL struct {
	URL string `json:"url"`
}

// MarshalJSON writes the content as a string, or as parts when images came along.
func (m hwChatMessage) MarshalJSON() ([]byte, error) {
	if len(m.Parts) == 0 {
		return common.Marshal(struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{m.Role, m.Content})
	}
	return common.Marshal(struct {
		Role    string       `json:"role"`
		Content []hwChatPart `json:"content"`
	}{m.Role, m.Parts})
}

type hwChatRequest struct {
	Model     string          `json:"model"`
	Messages  []hwChatMessage `json:"messages"`
	MaxTokens int             `json:"max_tokens"`
	Stream    bool            `json:"stream"`
}

type hwChatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type hwChatResponse struct {
	Choices []struct {
		Message hwChatMessage `json:"message"`
	} `json:"choices"`
	Usage hwChatUsage `json:"usage"`
}

// hwCallChat makes one chat completion call through the customer route and
// returns the answer text with the token counts the route reported.
func hwCallChat(ctx context.Context, client *http.Client, baseURL string, apiKey string, request hwChatRequest) (string, hwChatUsage, error) {
	body, err := common.Marshal(request)
	if err != nil {
		return "", hwChatUsage{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", hwChatUsage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := client.Do(req)
	if err != nil {
		return "", hwChatUsage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", hwChatUsage{}, fmt.Errorf("customer route returned status %d", resp.StatusCode)
	}
	var answer hwChatResponse
	if err := common.DecodeJson(resp.Body, &answer); err != nil {
		return "", hwChatUsage{}, fmt.Errorf("customer route returned an unreadable body: %w", err)
	}
	if len(answer.Choices) == 0 {
		return "", hwChatUsage{}, errors.New("customer route returned no answer")
	}
	return answer.Choices[0].Message.Content, answer.Usage, nil
}

// hwJudgePrompt builds the blind judge request. It carries the task, the
// criteria, and the answer — and deliberately nothing that identifies the
// model being judged. The grading instruction travels in the user message:
// the primary supplier replaces a customer system message with its own, and a
// judge that never sees the instruction solves the task instead of scoring it.
func hwJudgePrompt(task HwQualityTask, answer string) []hwChatMessage {
	expected := ""
	if task.Expected != "" {
		expected = "Expected answer: " + task.Expected + "\n\n"
	}
	criteria := strings.Join(task.Criteria, "\n- ")
	return []hwChatMessage{
		{
			Role: "user",
			Content: "You are a grader. Do not solve the task yourself. You grade a model answer " +
				"to a task. You see only the task, the grading criteria, and the answer. Reply " +
				"with a JSON object {\"score\": N} and nothing else, where N is an integer from " +
				"0 (completely wrong) to 10 (perfect). Judge strictly on the criteria.\n\n" +
				"Task (" + task.Kind + ", " + task.Language + "):\n" + task.Prompt +
				"\n\nGrading criteria:\n- " + criteria + "\n\n" + expected + "Answer to grade:\n" + answer,
		},
	}
}

// hwJudgeVerdict is the JSON object the judge model must reply with.
type hwJudgeVerdict struct {
	Score int `json:"score"`
}

// HwQualityReference marks how the model relates to the original first-party
// model. A claude-family model is pending a later comparison; anything else has
// no reachable original, and neither state ever claims equality.
func HwQualityReference(modelName string) string {
	if strings.HasPrefix(modelName, "claude") {
		return model.HwQualityReferencePending
	}
	return model.HwQualityReferenceNone
}

// hwQualityTaskResult is one measured task in the JSON measurement file.
type hwQualityTaskResult struct {
	TaskId    string `json:"task_id"`
	Language  string `json:"language"`
	Kind      string `json:"kind"`
	Score     int    `json:"score"`
	TokensIn  int    `json:"tokens_in"`
	TokensOut int    `json:"tokens_out"`
	LatencyMs int64  `json:"latency_ms"`
}

// hwQualityModelResult is one measured model in the JSON measurement file.
type hwQualityModelResult struct {
	Model     string                `json:"model"`
	Reference string                `json:"reference"`
	Tasks     []hwQualityTaskResult `json:"tasks"`
}

// hwQualityFile is the complete JSON measurement file.
type hwQualityFile struct {
	GeneratedAt    int64                  `json:"generated_at"`
	TasksetVersion string                 `json:"taskset_version"`
	JudgeModel     string                 `json:"judge_model"`
	SampleNote     string                 `json:"sample_note"`
	Models         []hwQualityModelResult `json:"models"`
}

// ErrHwQualityRunBusy refuses a measurement pass while another one runs: a
// second pass would call every model again and double the gateway spend.
var ErrHwQualityRunBusy = errors.New("a quality measurement is already running")

// hwQualityRunMu lets one measurement pass run at a time.
var hwQualityRunMu sync.Mutex

// HwRunQualityMeasurement runs the fixed task set over the configured models,
// judges every answer blind, writes the JSON measurement file, and stores the
// pass. The run refuses to start with an incomplete configuration and aborts
// on the first failed call — a half-measured run would put made-up gaps into
// the results, so nothing is persisted and no file is written then.
func HwRunQualityMeasurement(ctx context.Context, opts HwQualityRunOptions) (*model.HwQualityRun, error) {
	if !hwQualityRunMu.TryLock() {
		return nil, ErrHwQualityRunBusy
	}
	defer hwQualityRunMu.Unlock()
	if opts.BaseURL == "" || opts.APIKey == "" || opts.JudgeModel == "" || opts.OutDir == "" {
		return nil, errors.New("quality measurement needs a base URL, an API key, a judge model, and an output directory")
	}
	models := opts.Models
	if len(models) == 0 {
		models = HwSoldModelNames()
	}
	for _, name := range models {
		if _, ok := hwFindSoldModel(name); !ok {
			return nil, fmt.Errorf("model %q is not sold here", name)
		}
	}
	tasks, err := HwQualityTasks()
	if err != nil {
		return nil, err
	}
	nowFunc := opts.NowFunc
	if nowFunc == nil {
		nowFunc = time.Now
	}
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: hwQualityCallTimeout}
	}

	startedAt := nowFunc()
	scores := make([]model.HwQualityScore, 0, len(models)*len(tasks))
	fileModels := make([]hwQualityModelResult, 0, len(models))
	for _, modelName := range models {
		result := hwQualityModelResult{Model: modelName, Reference: HwQualityReference(modelName)}
		for _, task := range tasks {
			limit := task.MaxTokens
			if limit <= 0 {
				limit = hwQualityDefaultLimit
			}
			callCtx, cancel := context.WithTimeout(ctx, hwQualityCallTimeout)
			started := nowFunc()
			answer, usage, err := hwCallChat(callCtx, client, opts.BaseURL, opts.APIKey, hwChatRequest{
				Model:     modelName,
				Messages:  []hwChatMessage{{Role: "user", Content: task.Prompt}},
				MaxTokens: limit,
				Stream:    false,
			})
			latency := nowFunc().Sub(started).Milliseconds()
			if err != nil {
				cancel()
				return nil, fmt.Errorf("model %q task %q failed: %w", modelName, task.Id, err)
			}
			verdictRaw, _, err := hwCallChat(callCtx, client, opts.BaseURL, opts.APIKey, hwChatRequest{
				Model:     opts.JudgeModel,
				Messages:  hwJudgePrompt(task, answer),
				MaxTokens: 64,
				Stream:    false,
			})
			cancel()
			if err != nil {
				return nil, fmt.Errorf("judge for model %q task %q failed: %w", modelName, task.Id, err)
			}
			var verdict hwJudgeVerdict
			if err := common.Unmarshal([]byte(strings.TrimSpace(verdictRaw)), &verdict); err != nil {
				return nil, fmt.Errorf("judge reply for model %q task %q is not a JSON score", modelName, task.Id)
			}
			if verdict.Score < 0 || verdict.Score > 10 {
				return nil, fmt.Errorf("judge reply for model %q task %q is out of range", modelName, task.Id)
			}
			judgedAt := nowFunc()
			scores = append(scores, model.HwQualityScore{
				Model:     modelName,
				TaskId:    task.Id,
				Score:     verdict.Score,
				TokensIn:  usage.PromptTokens,
				TokensOut: usage.CompletionTokens,
				LatencyMs: latency,
				JudgedAt:  judgedAt.Unix(),
				Reference: HwQualityReference(modelName),
			})
			result.Tasks = append(result.Tasks, hwQualityTaskResult{
				TaskId:    task.Id,
				Language:  task.Language,
				Kind:      task.Kind,
				Score:     verdict.Score,
				TokensIn:  usage.PromptTokens,
				TokensOut: usage.CompletionTokens,
				LatencyMs: latency,
			})
		}
		fileModels = append(fileModels, result)
	}

	finishedAt := nowFunc()
	run := model.HwQualityRun{
		CreatedAt:  startedAt.Unix(),
		FinishedAt: finishedAt.Unix(),
		FileName:   fmt.Sprintf("hw-quality-%s.json", finishedAt.UTC().Format("20060102T150405Z")),
		TaskCount:  len(tasks),
		TasksetVer: HwQualityTasksetVersion(),
		JudgeModel: opts.JudgeModel,
	}
	fileContent, err := common.Marshal(hwQualityFile{
		GeneratedAt:    finishedAt.Unix(),
		TasksetVersion: run.TasksetVer,
		JudgeModel:     opts.JudgeModel,
		SampleNote:     HwQualitySampleNoteTr,
		Models:         fileModels,
	})
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
		return nil, fmt.Errorf("quality output directory is not writable: %w", err)
	}
	if err := os.WriteFile(filepath.Join(opts.OutDir, run.FileName), fileContent, 0o600); err != nil {
		return nil, fmt.Errorf("quality measurement file could not be written: %w", err)
	}
	if err := model.HwSaveQualityRun(&run, scores); err != nil {
		return nil, err
	}
	return &run, nil
}

// The sample note is part of every public quality answer: the measurements are
// a sample over a small fixed task set, not a guarantee.
const (
	HwQualitySampleNoteTr = "Bu ölçümler sabit ve sınırlı bir görev setiyle yapılan bir örneklemdir, garanti değildir."
	HwQualitySampleNoteEn = "These measurements are a sample over a small fixed task set, not a guarantee."
)

// HwQualityTaskSummary is one task of one model in the public summary.
type HwQualityTaskSummary struct {
	TaskId    string `json:"task_id"`
	Language  string `json:"language"`
	Kind      string `json:"kind"`
	Score     int    `json:"score"`
	TokensIn  int    `json:"tokens_in"`
	TokensOut int    `json:"tokens_out"`
	LatencyMs int64  `json:"latency_ms"`
}

// HwQualityModelSummary is the public per-model summary of the latest run.
type HwQualityModelSummary struct {
	Model        string                 `json:"model"`
	Reference    string                 `json:"reference"`
	AvgScore     float64                `json:"avg_score"`
	AvgLatencyMs int64                  `json:"avg_latency_ms"`
	Tasks        []HwQualityTaskSummary `json:"tasks"`
}

// HwQualityLatestSummary is the public answer of the latest measurement.
type HwQualityLatestSummary struct {
	MeasuredAt     int64                   `json:"measured_at"`
	FileName       string                  `json:"file"`
	TasksetVersion string                  `json:"taskset_version"`
	TaskCount      int                     `json:"task_count"`
	JudgeModel     string                  `json:"judge_model"`
	Models         []HwQualityModelSummary `json:"models"`
	Note           string                  `json:"note"`
	NoteEn         string                  `json:"note_en"`
	// Traffic is the observed quality of the real customer traffic. It is
	// filled by the public endpoint and stays nil for the judged run alone.
	Traffic *HwQualityTraffic `json:"traffic"`
}

// HwLatestQualitySummary aggregates the newest run per model. Without any run
// it returns an empty model list with the sample note — never a made-up value.
func HwLatestQualitySummary() (*HwQualityLatestSummary, error) {
	summary := &HwQualityLatestSummary{
		Models: []HwQualityModelSummary{},
		Note:   HwQualitySampleNoteTr,
		NoteEn: HwQualitySampleNoteEn,
	}
	run, scores, err := model.HwLatestQualityRun()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return summary, nil
		}
		return nil, err
	}
	taskMeta := make(map[string]HwQualityTask)
	if tasks, taskErr := HwQualityTasks(); taskErr == nil {
		for _, task := range tasks {
			taskMeta[task.Id] = task
		}
	}
	byModel := make(map[string]*HwQualityModelSummary)
	order := make([]string, 0)
	for _, score := range scores {
		entry, ok := byModel[score.Model]
		if !ok {
			entry = &HwQualityModelSummary{Model: score.Model, Reference: score.Reference, Tasks: []HwQualityTaskSummary{}}
			byModel[score.Model] = entry
			order = append(order, score.Model)
		}
		task := HwQualityTaskSummary{
			TaskId:    score.TaskId,
			Score:     score.Score,
			TokensIn:  score.TokensIn,
			TokensOut: score.TokensOut,
			LatencyMs: score.LatencyMs,
		}
		if meta, ok := taskMeta[score.TaskId]; ok {
			task.Language = meta.Language
			task.Kind = meta.Kind
		}
		entry.Tasks = append(entry.Tasks, task)
		entry.AvgScore += float64(score.Score)
		entry.AvgLatencyMs += score.LatencyMs
	}
	for _, name := range order {
		entry := byModel[name]
		if count := len(entry.Tasks); count > 0 {
			entry.AvgScore = math.Round(entry.AvgScore/float64(count)*100) / 100
			entry.AvgLatencyMs = entry.AvgLatencyMs / int64(count)
		}
		summary.Models = append(summary.Models, *entry)
	}
	summary.MeasuredAt = run.FinishedAt
	summary.FileName = run.FileName
	summary.TasksetVersion = run.TasksetVer
	summary.TaskCount = run.TaskCount
	summary.JudgeModel = run.JudgeModel
	return summary, nil
}

// Observed quality of the real customer traffic. Where the judged run above
// needs paid test calls, these figures come for free from the rows the relay
// writes for every customer request anyway: the share of requests that got an
// answer, the median and 95th percentile of the response time, and the output
// speed. They cover the last seven and thirty days and are rebuilt at most
// every ten minutes, so they stay current without a job and without letting a
// public read scan the log table on every visit.
const (
	// HwQualityTrafficMinRequests is the request count of a window below which
	// a model shows its request count only: a percentile over a handful of
	// requests says nothing.
	HwQualityTrafficMinRequests = 20

	hwQualityTrafficRefresh = 10 * time.Minute
	hwQualityTrafficDay     = 24 * 3600
)

// hwQualityTrafficWindowDays are the windows of the public figures.
var hwQualityTrafficWindowDays = []int{7, 30}

// HwQualityTrafficModel is the observed quality of one own model in one
// window. The figures are null while the window has fewer than
// HwQualityTrafficMinRequests requests, and the latency figures stay null when
// no request was served at all.
type HwQualityTrafficModel struct {
	Model      string `json:"model"`
	Requests   int64  `json:"requests"`
	Failed     int64  `json:"failed"`
	EnoughData bool   `json:"enough_data"`
	// SuccessPercent is the share of requests that got an answer, two decimals.
	SuccessPercent *float64 `json:"success_percent"`
	// MedianSeconds and P95Seconds are the full response time of a served
	// request in whole seconds, as the relay records it.
	MedianSeconds *int `json:"median_seconds"`
	P95Seconds    *int `json:"p95_seconds"`
	// OutputTokensPerSecond is the output tokens of the served requests over
	// their total response time, one decimal.
	OutputTokensPerSecond *float64 `json:"output_tokens_per_second"`
}

// HwQualityTrafficWindow lists the own models with at least one request in the
// window, in catalog order.
type HwQualityTrafficWindow struct {
	Days   int                     `json:"days"`
	Models []HwQualityTrafficModel `json:"models"`
}

// HwQualityTraffic is the public observed-quality answer.
type HwQualityTraffic struct {
	ComputedAt     int64                    `json:"computed_at"`
	RefreshSeconds int                      `json:"refresh_seconds"`
	MinRequests    int                      `json:"min_requests"`
	Windows        []HwQualityTrafficWindow `json:"windows"`
}

var hwQualityTrafficCache struct {
	sync.Mutex
	builtAt time.Time
	traffic *HwQualityTraffic
}

// HwQualityTrafficNow answers the observed quality, rebuilt from the logs at
// most once per refresh period. The lock is held while rebuilding, so a burst
// of visitors after the period ends costs one read, not one per visitor.
func HwQualityTrafficNow(now time.Time) (*HwQualityTraffic, error) {
	hwQualityTrafficCache.Lock()
	defer hwQualityTrafficCache.Unlock()
	cached := hwQualityTrafficCache.traffic
	if cached != nil && !now.Before(hwQualityTrafficCache.builtAt) && now.Sub(hwQualityTrafficCache.builtAt) < hwQualityTrafficRefresh {
		return cached, nil
	}
	traffic, err := HwReadQualityTrafficAt(now)
	if err != nil {
		return nil, err
	}
	hwQualityTrafficCache.builtAt = now
	hwQualityTrafficCache.traffic = traffic
	return traffic, nil
}

// HwReadQualityTrafficAt builds the observed quality of every own model for
// the windows ending at now, straight from the logs.
func HwReadQualityTrafficAt(now time.Time) (*HwQualityTraffic, error) {
	models := HwSoldModelNames()
	traffic := &HwQualityTraffic{
		ComputedAt:     now.Unix(),
		RefreshSeconds: int(hwQualityTrafficRefresh.Seconds()),
		MinRequests:    HwQualityTrafficMinRequests,
		Windows:        make([]HwQualityTrafficWindow, 0, len(hwQualityTrafficWindowDays)),
	}
	// The end is exclusive; one second past now keeps the current second in.
	to := now.Unix() + 1
	for _, days := range hwQualityTrafficWindowDays {
		buckets, failures, err := model.HwReadQualityTraffic(to-int64(days)*hwQualityTrafficDay, to, models)
		if err != nil {
			return nil, err
		}
		bucketsOf := make(map[string][]model.HwQualityLatencyBucket)
		for _, bucket := range buckets {
			bucketsOf[bucket.ModelName] = append(bucketsOf[bucket.ModelName], bucket)
		}
		failedOf := make(map[string]int64, len(failures))
		for _, failure := range failures {
			failedOf[failure.ModelName] = failure.Failed
		}
		window := HwQualityTrafficWindow{Days: days, Models: []HwQualityTrafficModel{}}
		for _, name := range models {
			entry := HwObservedModelQuality(name, bucketsOf[name], failedOf[name])
			if entry.Requests > 0 {
				window.Models = append(window.Models, entry)
			}
		}
		traffic.Windows = append(traffic.Windows, window)
	}
	return traffic, nil
}

// HwObservedModelQuality folds the latency buckets and the failed count of one
// model into its public figures. The percentiles use the nearest rank over the
// whole-second durations the relay records.
func HwObservedModelQuality(name string, buckets []model.HwQualityLatencyBucket, failed int64) HwQualityTrafficModel {
	entry := HwQualityTrafficModel{Model: name, Failed: max(failed, 0)}
	sorted := slices.Clone(buckets)
	slices.SortFunc(sorted, func(a, b model.HwQualityLatencyBucket) int { return cmp.Compare(a.UseTime, b.UseTime) })
	var served, outputTokens, busySeconds int64
	for _, bucket := range sorted {
		served += bucket.Requests
		if bucket.UseTime > 0 {
			outputTokens += bucket.OutputTokens
			busySeconds += int64(bucket.UseTime) * bucket.Requests
		}
	}
	entry.Requests = served + entry.Failed
	if entry.Requests < HwQualityTrafficMinRequests {
		return entry
	}
	entry.EnoughData = true
	success := math.Round(float64(served)*10000/float64(entry.Requests)) / 100
	entry.SuccessPercent = &success
	if served == 0 {
		return entry
	}
	median := hwLatencyAtRank(sorted, (served+1)/2)
	p95 := hwLatencyAtRank(sorted, (served*95+99)/100)
	entry.MedianSeconds = &median
	entry.P95Seconds = &p95
	if busySeconds > 0 {
		speed := math.Round(float64(outputTokens)*10/float64(busySeconds)) / 10
		entry.OutputTokensPerSecond = &speed
	}
	return entry
}

// hwLatencyAtRank returns the duration of the request at the given 1-based
// rank of buckets sorted by duration.
func hwLatencyAtRank(sorted []model.HwQualityLatencyBucket, rank int64) int {
	var seen int64
	for _, bucket := range sorted {
		seen += bucket.Requests
		if seen >= rank {
			return bucket.UseTime
		}
	}
	return sorted[len(sorted)-1].UseTime
}
