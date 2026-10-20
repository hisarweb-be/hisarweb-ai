package model

// Quality measurement storage for HisarWeb AI. One run is one pass of the fixed
// task set over a set of models; every task result is stored per model with the
// tokens and latency the customer route reported, plus the score of the blind
// judge. The JSON measurement file on disk is the durable record the site links
// to; these tables are the queryable copy.

import (
	"errors"

	"gorm.io/gorm"
)

// Reference markers for the comparison with the original first-party models.
// A claude-family model is "pending": the comparison runs later through the
// owner's subscription, and nothing here may claim equality with the original.
// Everything else has no reachable original and is marked "none".
const (
	HwQualityReferenceNone    = "none"
	HwQualityReferencePending = "pending"
)

// HwQualityRun is one completed measurement pass.
type HwQualityRun struct {
	Id int `json:"id" gorm:"primaryKey"`
	// CreatedAt is the unix time the run started, FinishedAt when the last
	// task was judged.
	CreatedAt  int64 `json:"created_at" gorm:"not null"`
	FinishedAt int64 `json:"finished_at" gorm:"not null;default:0"`
	// FileName is the base name of the JSON measurement file written for this
	// run (no directory, no secret content).
	FileName   string `json:"file_name" gorm:"type:varchar(255);not null;default:''"`
	TaskCount  int    `json:"task_count" gorm:"not null;default:0"`
	TasksetVer string `json:"taskset_version" gorm:"type:varchar(32);not null;default:''"`
	JudgeModel string `json:"judge_model" gorm:"type:varchar(128);not null;default:''"`
}

func (HwQualityRun) TableName() string {
	return "hw_quality_runs"
}

// HwQualityScore is one judged task result of one run.
type HwQualityScore struct {
	Id     int    `json:"id" gorm:"primaryKey"`
	RunId  int    `json:"run_id" gorm:"not null;index"`
	Model  string `json:"model" gorm:"type:varchar(128);not null"`
	TaskId string `json:"task_id" gorm:"type:varchar(64);not null"`
	// Score is the blind judge's verdict, 0-10.
	Score     int    `json:"score" gorm:"not null"`
	TokensIn  int    `json:"tokens_in" gorm:"not null;default:0"`
	TokensOut int    `json:"tokens_out" gorm:"not null;default:0"`
	LatencyMs int64  `json:"latency_ms" gorm:"not null;default:0"`
	JudgedAt  int64  `json:"judged_at" gorm:"not null;default:0"`
	Reference string `json:"reference" gorm:"type:varchar(16);not null;default:''"`
}

func (HwQualityScore) TableName() string {
	return "hw_quality_scores"
}

// HwSaveQualityRun stores a completed run and all its task results in one
// transaction. The JSON measurement file must already exist when this is
// called, so the database never references a file that was not written.
func HwSaveQualityRun(run *HwQualityRun, scores []HwQualityScore) error {
	if run == nil {
		return errors.New("nil quality run")
	}
	if len(scores) == 0 {
		return errors.New("quality run without scores")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(run).Error; err != nil {
			return err
		}
		for i := range scores {
			scores[i].RunId = run.Id
		}
		return tx.Create(&scores).Error
	})
}

// HwLatestQualityRun returns the newest run and its task results. The scores of
// a healthy run are never empty, but the caller treats an empty list the same
// as no run: an empty answer, never a made-up one.
func HwLatestQualityRun() (*HwQualityRun, []HwQualityScore, error) {
	var run HwQualityRun
	if err := DB.Order("id DESC").First(&run).Error; err != nil {
		return nil, nil, err
	}
	var scores []HwQualityScore
	if err := DB.Where("run_id = ?", run.Id).Order("model ASC, task_id ASC").Find(&scores).Error; err != nil {
		return nil, nil, err
	}
	return &run, scores, nil
}
