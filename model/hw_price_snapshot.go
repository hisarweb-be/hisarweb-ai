package model

// Daily price snapshots for the HisarWeb AI TL pricing. One row is the state of
// the world at one successful fetch: the open LiteLLM price list subset for the
// models we sell plus the TCMB USD/TRY rate of that day.
//
// Money is never stored as a float. Model prices are integer micro-dollars per
// million tokens ($2.00 per million = 2000000) and the exchange rate is decimal
// text exactly as the source published it.

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
)

// ErrHwPriceSnapshotMissing reports that no price snapshot exists yet. Callers
// answer with an honest "no price data" instead of a fabricated value.
var ErrHwPriceSnapshotMissing = errors.New("no price snapshot available")

// HwModelPrice is the official list price of one model, in integer micro-dollars
// per million tokens (1 micro-dollar = $0.000001).
type HwModelPrice struct {
	InputMicroUsd  int64 `json:"in"`
	OutputMicroUsd int64 `json:"out"`
}

// HwPriceSnapshot is one successful daily fetch of both pricing sources.
type HwPriceSnapshot struct {
	Id int `json:"id" gorm:"primaryKey"`
	// PriceSourceUrl and FxSourceUrl are the URLs the data was actually read
	// from, so a later audit can trace every shown price back to its source.
	PriceSourceUrl string `json:"price_source_url" gorm:"type:varchar(512);not null"`
	FxSourceUrl    string `json:"fx_source_url" gorm:"type:varchar(512);not null"`
	// FetchedAt is the unix time of the successful fetch.
	FetchedAt int64 `json:"fetched_at" gorm:"not null"`
	// FxDate is the validity date of the rate as published by the source
	// (YYYY-MM-DD).
	FxDate string `json:"fx_date" gorm:"type:varchar(10);not null"`
	// TryPerUsd is the TRY per USD rate as decimal text (e.g. "49.5523").
	TryPerUsd string `json:"try_per_usd" gorm:"type:varchar(32);not null"`
	// EurPerUsd is the EUR per USD cross rate of the same bulletin as decimal
	// text, or empty when the bulletin had no usable EUR rate. It only travels
	// from the fetch to the rate options and is not stored.
	EurPerUsd string `json:"-" gorm:"-"`
	// Models maps our own model names to their list price, stored as JSON
	// text. Only the models we sell are kept, so the column stays small on
	// every supported database.
	Models string `json:"models" gorm:"type:text;not null"`
	// ListedCount is how many models the upstream price list contained; it
	// documents how complete the fetch was.
	ListedCount int `json:"listed_count" gorm:"not null;default:0"`
	// StaleAt is the unix time the snapshot was marked outdated after a
	// failed refresh. 0 means the snapshot is current.
	StaleAt int64 `json:"stale_at" gorm:"not null;default:0"`
}

func (HwPriceSnapshot) TableName() string {
	return "hw_price_snapshots"
}

// HwSavePriceSnapshot appends one successful fetch. Snapshots are never
// overwritten or deleted; the newest row is the current one.
func HwSavePriceSnapshot(snapshot *HwPriceSnapshot) error {
	if snapshot == nil {
		return errors.New("nil price snapshot")
	}
	return DB.Create(snapshot).Error
}

// HwLatestPriceSnapshot returns the newest snapshot.
func HwLatestPriceSnapshot() (*HwPriceSnapshot, error) {
	var snapshot HwPriceSnapshot
	if err := DB.Order("id DESC").First(&snapshot).Error; err != nil {
		return nil, err
	}
	return &snapshot, nil
}

// HwMarkPriceSnapshotsStale marks every still-current snapshot outdated,
// because a refresh attempt failed. The rows (and their dates) stay readable;
// nothing is replaced by a made-up value.
func HwMarkPriceSnapshotsStale(now int64) error {
	if now <= 0 {
		return errors.New("stale mark time must be positive")
	}
	return DB.Model(&HwPriceSnapshot{}).
		Where("stale_at = 0").
		Update("stale_at", now).Error
}

// HwSnapshotModelPrices decodes the snapshot's model price map. A snapshot
// without any decodable entries yields an empty map, not an error: it was
// written by a fetch that found none of our models.
func (s *HwPriceSnapshot) HwSnapshotModelPrices() (map[string]HwModelPrice, error) {
	prices := make(map[string]HwModelPrice)
	if s == nil || s.Models == "" {
		return prices, nil
	}
	if err := common.Unmarshal([]byte(s.Models), &prices); err != nil {
		return nil, err
	}
	return prices, nil
}
