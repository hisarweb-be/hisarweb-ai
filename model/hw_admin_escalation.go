package model

// HwAdminEscalation is a problem the admin assistant passed on to the bureau.
// The gateway server cannot always reach the bureau board (it sits on a
// private network), so every escalation is stored first and the board card is
// created when the board answers. A row without a CardRef waits for a sync
// from a machine that can reach the board.

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

type HwAdminEscalation struct {
	ID        int    `json:"id" gorm:"primaryKey"`
	Title     string `json:"title" gorm:"size:200;not null"`
	Summary   string `json:"summary" gorm:"type:text;not null"`
	Page      string `json:"page" gorm:"size:300"`
	CardRef   string `json:"card_ref" gorm:"size:64;index"`
	CreatedBy int    `json:"created_by"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime"`
}

// hwAdminEscalationSummaryMaxRunes matches the description limit of the bureau
// card (hwBureauCardMaxRunes in service), so the row never holds more than the
// board shows.
const hwAdminEscalationSummaryMaxRunes = 4000

func HwCreateAdminEscalation(db *gorm.DB, title, summary, page string, adminID int) (*HwAdminEscalation, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, errors.New("title is required")
	}
	row := &HwAdminEscalation{
		Title: truncateRunes(title, 200), Summary: truncateRunes(strings.TrimSpace(summary), hwAdminEscalationSummaryMaxRunes),
		Page: truncateRunes(strings.TrimSpace(page), 300), CreatedBy: adminID,
	}
	return row, db.Create(row).Error
}

func HwListPendingAdminEscalations(db *gorm.DB) ([]HwAdminEscalation, error) {
	var rows []HwAdminEscalation
	return rows, db.Where("card_ref = ''").Order("id").Limit(100).Find(&rows).Error
}

func HwSetAdminEscalationCard(db *gorm.DB, id int, cardRef string) error {
	result := db.Model(&HwAdminEscalation{}).Where("id = ?", id).Update("card_ref", truncateRunes(cardRef, 64))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("escalation not found")
	}
	return nil
}

func truncateRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
}
