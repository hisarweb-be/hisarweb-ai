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

	// Review verdicts on customer rows (widget, Telegram), set by the bureau
	// (Paperclip). Approved rows teach like admin rows; rejected and abuse
	// rows never do. An empty review means not yet reviewed.
	HwAssistantReviewApproved = "approved"
	HwAssistantReviewRejected = "rejected"
	HwAssistantReviewAbuse    = "abuse"
	// HwAssistantReviewOpen is the list filter for customer rows without a
	// verdict.
	HwAssistantReviewOpen   = "open"
	hwAssistantAdminChannel = "admin"
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
	Review     string `json:"review" gorm:"size:16;default:'';index"`
	CreatedAt  int64  `json:"created_at" gorm:"index"`
}

var (
	hwAssistantFeedbackPersonalPattern = regexp.MustCompile(`@|\d{6,}|(?i)\bsk-[a-z0-9]|[A-Za-z0-9]{32,}`)
	hwAssistantFeedbackLinkPattern     = regexp.MustCompile(`(?i)https?://[^\s/?#]*`)
)

// hwAssistantFeedbackLooksPersonal reports an address, a long number, a
// key-like string (sk-..., or 32+ letters and digits in one run) or a
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
// stored=false without error when the question, the answer or the
// correction looks personal.
func HwAssistantStoreFeedback(db *gorm.DB, row HwAssistantFeedback) (bool, error) {
	row.Question = hwAssistantTruncateRunes(row.Question, HwAssistantFeedbackMaxQuestionRunes)
	row.Answer = hwAssistantTruncateRunes(row.Answer, HwAssistantFeedbackMaxAnswerRunes)
	row.Correction = hwAssistantTruncateRunes(row.Correction, HwAssistantFeedbackMaxCorrectionRunes)
	if row.Question == "" {
		return false, errors.New("empty question")
	}
	if hwAssistantFeedbackLooksPersonal(row.Question) || hwAssistantFeedbackLooksPersonal(row.Answer) ||
		hwAssistantFeedbackLooksPersonal(row.Correction) {
		return false, nil
	}
	if err := db.Create(&row).Error; err != nil {
		return false, err
	}
	return true, nil
}

// HwAssistantRecentFeedback returns the newest rows, one bounded query.
func HwAssistantRecentFeedback(db *gorm.DB, limit int) ([]HwAssistantFeedback, error) {
	return HwAssistantListFeedback(db, 0, "", limit)
}

// HwAssistantLearnableFeedback returns the newest rows that may teach the
// assistant: admin rows and customer rows the bureau approved. One bounded
// query.
func HwAssistantLearnableFeedback(db *gorm.DB, limit int) ([]HwAssistantFeedback, error) {
	var rows []HwAssistantFeedback
	err := db.Where("channel = ? OR review = ?", hwAssistantAdminChannel, HwAssistantReviewApproved).
		Order("created_at DESC, id DESC").Limit(min(max(limit, 1), HwAssistantFeedbackMaxRecent)).Find(&rows).Error
	return rows, err
}

// HwAssistantListFeedback returns the newest rows, optionally only those with
// the given rating (1 or -1; 0 means all) and review ("" means all,
// HwAssistantReviewOpen the customer rows without a verdict, else that
// verdict).
func HwAssistantListFeedback(db *gorm.DB, rating int, review string, limit int) ([]HwAssistantFeedback, error) {
	query := db.Order("created_at DESC, id DESC").Limit(min(max(limit, 1), HwAssistantFeedbackMaxRecent))
	if rating != 0 {
		query = query.Where("rating = ?", rating)
	}
	switch review {
	case "":
	case HwAssistantReviewOpen:
		query = query.Where("channel <> ? AND (review IS NULL OR review = ?)", hwAssistantAdminChannel, "")
	default:
		query = query.Where("review = ?", review)
	}
	var rows []HwAssistantFeedback
	err := query.Find(&rows).Error
	return rows, err
}

// HwAssistantReviewFeedback stores the verdict of the bureau on one customer
// row. It returns gorm.ErrRecordNotFound for an unknown id or an admin row.
func HwAssistantReviewFeedback(db *gorm.DB, id int, verdict string) error {
	switch verdict {
	case HwAssistantReviewApproved, HwAssistantReviewRejected, HwAssistantReviewAbuse:
	default:
		return errors.New("invalid verdict")
	}
	result := db.Model(&HwAssistantFeedback{}).Where("id = ? AND channel <> ?", id, hwAssistantAdminChannel).
		Update("review", verdict)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
