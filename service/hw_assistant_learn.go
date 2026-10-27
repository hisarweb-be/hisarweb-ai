package service

// The learning loop of the assistant: thumbs and corrections stored by
// model.HwAssistantStoreFeedback come back here as a prompt section, so an
// approved answer is reused for a similar question and a rejected one is not
// repeated.

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const (
	hwAssistantLearnedMaxRows            = 4
	hwAssistantLearnedMaxAnswerRunes     = 600
	hwAssistantLearnedMaxSectionRunes    = 3000
	hwAssistantLearnedApprovedHeading    = "Approved answers to similar questions, follow their content (rewrite in the current language):"
	hwAssistantLearnedRejectedHeading    = "Answers that were rated wrong, do not repeat them:"
	hwAssistantLearnedMinSharedWords     = 2
	hwAssistantLearnedShortQuestionWords = 3
	// hwAssistantFeedbackDayLimitAll bounds the stored customer thumbs per day.
	hwAssistantFeedbackDayLimitAll = 2000
)

// hwAssistantStopWords are short everyday words (tr, nl, en) that say nothing
// about the topic of a question.
var hwAssistantStopWords = func() map[string]bool {
	set := map[string]bool{}
	for _, word := range strings.Fields(`
		a an the is are was be to of in on at for and or do does can how what why my me i you your it this that with from by as if not no
		bir bu su ve ile icin mi mu nasil ne nedir var yok ben beni benim sen siz biz da de ki
		de het een en of is zijn ik je jij mijn dit dat die op in te van voor met niet wat hoe kan kun ook naar om aan`) {
		set[word] = true
	}
	return set
}()

func hwAssistantContentWords(text string) map[string]bool {
	words := map[string]bool{}
	for _, word := range strings.FieldsFunc(HwAssistantFold(text), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	}) {
		if len(word) > 1 && !hwAssistantStopWords[word] {
			words[word] = true
		}
	}
	return words
}

func hwAssistantClipRunes(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return string(runes)
}

// HwAssistantLearnedSection picks the stored feedback rows that resemble
// question and formats them for the model prompt. It returns "" when nothing
// is similar enough.
func HwAssistantLearnedSection(question string, rows []model.HwAssistantFeedback) string {
	wanted := hwAssistantContentWords(question)
	if len(wanted) == 0 {
		return ""
	}
	need := hwAssistantLearnedMinSharedWords
	if len(wanted) < hwAssistantLearnedShortQuestionWords {
		need = (len(wanted)*6 + 9) / 10 // 60 %, rounded up
	}
	type scored struct {
		row   model.HwAssistantFeedback
		score int
	}
	var picked []scored
	for _, row := range rows {
		shared := 0
		for word := range hwAssistantContentWords(row.Question) {
			if wanted[word] {
				shared++
			}
		}
		if shared >= need {
			picked = append(picked, scored{row, shared})
		}
	}
	sort.SliceStable(picked, func(i, j int) bool { return picked[i].score > picked[j].score })
	if len(picked) > hwAssistantLearnedMaxRows {
		picked = picked[:hwAssistantLearnedMaxRows]
	}
	var approved, rejected []string
	for _, item := range picked {
		answer := strings.TrimSpace(item.row.Correction)
		if answer == "" {
			answer = item.row.Answer
		}
		line := "Q: " + hwAssistantClipRunes(item.row.Question, 300) + " A: " + hwAssistantClipRunes(answer, hwAssistantLearnedMaxAnswerRunes)
		switch {
		case item.row.Correction != "" || item.row.Rating > 0:
			approved = append(approved, line)
		case item.row.Rating < 0:
			rejected = append(rejected, line)
		}
	}
	var parts []string
	if len(approved) > 0 {
		parts = append(parts, hwAssistantLearnedApprovedHeading+"\n"+strings.Join(approved, "\n"))
	}
	if len(rejected) > 0 {
		parts = append(parts, hwAssistantLearnedRejectedHeading+"\n"+strings.Join(rejected, "\n"))
	}
	return hwAssistantClipRunes(strings.Join(parts, "\n\n"), hwAssistantLearnedMaxSectionRunes)
}

// HwAssistantLearnedFor loads the recent feedback of the owner's panel and
// returns the section for question; "" on any error or without a database.
// Only admin rows teach: a widget or Telegram thumb is anonymous and carries
// the question and answer the client sent, so one fake thumb up would steer
// what every customer is told. Those rows stay for the owner to review.
func HwAssistantLearnedFor(question string) string {
	if model.DB == nil {
		return ""
	}
	rows, err := model.HwAssistantRecentChannelFeedback(model.DB, HwAssistantChannelAdmin, model.HwAssistantFeedbackMaxRecent)
	if err != nil {
		return ""
	}
	return HwAssistantLearnedSection(question, rows)
}

// HwAssistantRecordFeedback stores one thumb (and an admin correction). It
// returns stored=false when the text looks personal.
func HwAssistantRecordFeedback(channel, lang, question, answer string, rating, userID int, correction string) (bool, error) {
	if rating != 1 && rating != -1 {
		return false, errors.New("invalid rating")
	}
	if strings.TrimSpace(question) == "" || strings.TrimSpace(answer) == "" {
		return false, errors.New("empty question or answer")
	}
	if model.DB == nil {
		return false, errors.New("no database")
	}
	if channel != HwAssistantChannelAdmin {
		// Customer thumbs are stored for review only; the day cap keeps an
		// anonymous flood from filling the table. A counter that cannot be
		// written stores nothing.
		now := common.GetTimestamp()
		count, err := model.HwAssistantBumpCounter(model.DB, "hw:fb:all:"+time.Unix(now, 0).UTC().Format("2006-01-02"), now)
		if err != nil || count > hwAssistantFeedbackDayLimitAll {
			return false, nil
		}
	}
	return model.HwAssistantStoreFeedback(model.DB, model.HwAssistantFeedback{
		Channel:    hwAssistantClipRunes(channel, 16),
		Lang:       hwAssistantClipRunes(lang, 8),
		Question:   question,
		Answer:     answer,
		Rating:     rating,
		Correction: correction,
		UserID:     userID,
		CreatedAt:  common.GetTimestamp(),
	})
}
