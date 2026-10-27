package model

// HwAssistantFeedback is one thumbs up or down on an assistant answer, with
// an optional correction typed by an admin. Stored rows are fed back into
// later prompts (service.HwAssistantLearnedSection), so the assistant learns
// from approved and rejected answers. Nothing is stored for text that looks
// personal.

import (
	"errors"
	"net/url"
	"regexp"
	"strings"

	"gorm.io/gorm"
)

const (
	HwAssistantFeedbackMaxQuestionRunes   = 1000
	HwAssistantFeedbackMaxAnswerRunes     = 4000
	HwAssistantFeedbackMaxCorrectionRunes = 2000
	HwAssistantFeedbackMaxRecent          = 300
	HwAssistantFeedbackMaxList            = 200
	hwAssistantOwnHost                    = "api.hisarweb.be"
)

type HwAssistantFeedback struct {
	ID         int    `json:"id" gorm:"primaryKey"`
	Channel    string `json:"channel" gorm:"size:16"`
	Lang       string `json:"lang" gorm:"size:8"`
	Question   string `json:"question" gorm:"type:text"`
	Answer     string `json:"answer" gorm:"type:text"`
	Rating     int    `json:"rating"`
	Correction string `json:"correction" gorm:"type:text"`
	UserID     int    `json:"user_id" gorm:"index"`
	CreatedAt  int64  `json:"created_at" gorm:"index"`
}

var (
	hwAssistantFeedbackPersonalPattern = regexp.MustCompile(`@|\d{6,}`)
	hwAssistantFeedbackLinkPattern     = regexp.MustCompile(`(?i)https?://[^\s/?#]*`)
)

// hwAssistantFeedbackLooksPersonal reports an address, a long number or a
// link to anything but our own API host. A bare mention of the API host is
// fine.
func hwAssistantFeedbackLooksPersonal(text string) bool {
	if hwAssistantFeedbackPersonalPattern.MatchString(text) {
		return true
	}
	for _, link := range hwAssistantFeedbackLinkPattern.FindAllString(text, -1) {
		host := strings.ToLower(link[strings.Index(link, "://")+3:])
		if parsed, err := url.Parse("//" + host); err == nil {
			host = parsed.Hostname()
		}
		if host != hwAssistantOwnHost {
			return true
		}
	}
	return false
}

func hwAssistantTruncateRunes(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) > limit {
		runes = runes[:limit]
	}
	return string(runes)
}

// HwAssistantStoreFeedback saves row after bounding its text. It returns
// stored=false without error when the question or the correction looks
// personal.
func HwAssistantStoreFeedback(db *gorm.DB, row HwAssistantFeedback) (bool, error) {
	row.Question = hwAssistantTruncateRunes(row.Question, HwAssistantFeedbackMaxQuestionRunes)
	row.Answer = hwAssistantTruncateRunes(row.Answer, HwAssistantFeedbackMaxAnswerRunes)
	row.Correction = hwAssistantTruncateRunes(row.Correction, HwAssistantFeedbackMaxCorrectionRunes)
	if row.Question == "" {
		return false, errors.New("empty question")
	}
	if hwAssistantFeedbackLooksPersonal(row.Question) || hwAssistantFeedbackLooksPersonal(row.Correction) {
		return false, nil
	}
	if err := db.Create(&row).Error; err != nil {
		return false, err
	}
	return true, nil
}

// HwAssistantRecentFeedback returns the newest rows, one bounded query.
func HwAssistantRecentFeedback(db *gorm.DB, limit int) ([]HwAssistantFeedback, error) {
	return HwAssistantListFeedback(db, 0, limit)
}

// HwAssistantListFeedback returns the newest rows, optionally only those with
// the given rating (1 or -1; 0 means all).
func HwAssistantListFeedback(db *gorm.DB, rating int, limit int) ([]HwAssistantFeedback, error) {
	query := db.Order("created_at DESC, id DESC").Limit(min(max(limit, 1), HwAssistantFeedbackMaxRecent))
	if rating != 0 {
		query = query.Where("rating = ?", rating)
	}
	var rows []HwAssistantFeedback
	err := query.Find(&rows).Error
	return rows, err
}
