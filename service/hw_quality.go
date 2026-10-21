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
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

// HwRunQualityMeasurement runs the fixed task set over the configured models,
// judges every answer blind, writes the JSON measurement file, and stores the
// pass. The run refuses to start with an incomplete configuration and aborts
// on the first failed call — a half-measured run would put made-up gaps into
// the results, so nothing is persisted and no file is written then.
func HwRunQualityMeasurement(ctx context.Context, opts HwQualityRunOptions) (*model.HwQualityRun, error) {
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
