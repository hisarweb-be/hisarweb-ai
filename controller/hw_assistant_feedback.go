package controller

// Thumbs up/down on assistant answers. The stored rows teach later prompts
// (service.HwAssistantLearnedSection); admins can also add a correction and
// list what was rated.

import (
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type hwAssistantFeedbackRequest struct {
	Question   string `json:"question"`
	Answer     string `json:"answer"`
	Rating     int    `json:"rating"`
	Lang       string `json:"lang"`
	Correction string `json:"correction"`
}

// hwAssistantStoreFeedbackRequest validates the body and stores it. The
// correction is only kept for the admin channel.
func hwAssistantStoreFeedbackRequest(c *gin.Context, channel string, userID int, withCorrection bool) {
	var request hwAssistantFeedbackRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "invalid request")
		return
	}
	question := strings.TrimSpace(request.Question)
	answer := strings.TrimSpace(request.Answer)
	if question == "" || answer == "" || (request.Rating != 1 && request.Rating != -1) ||
		utf8.RuneCountInString(question) > model.HwAssistantFeedbackMaxQuestionRunes*3 ||
		utf8.RuneCountInString(answer) > model.HwAssistantFeedbackMaxAnswerRunes*3 {
		common.ApiErrorMsg(c, "invalid feedback")
		return
	}
	correction := ""
	if withCorrection {
		correction = request.Correction
	}
	stored, err := service.HwAssistantRecordFeedback(channel, strings.TrimSpace(request.Lang), question, answer, request.Rating, userID, correction)
	if err != nil {
		common.SysError(common.LogText("assistant feedback was not stored: %s", err.Error()))
		common.ApiErrorMsg(c, "feedback not stored")
		return
	}
	common.ApiSuccess(c, gin.H{"stored": stored})
}

// hwAssistantFeedbackDayLimit bounds the thumbs of one address per day.
const hwAssistantFeedbackDayLimit = 50

// HwAssistantFeedback stores a thumb from the public widget. Over the day
// limit of the address the thumb is accepted but not stored.
func HwAssistantFeedback(c *gin.Context) {
	now := common.GetTimestamp()
	key := "hw:fb:ip:" + c.ClientIP() + ":" + time.Unix(now, 0).UTC().Format("2006-01-02")
	if count, err := model.HwAssistantBumpCounter(model.DB, key, now); err != nil || count > hwAssistantFeedbackDayLimit {
		common.ApiSuccess(c, gin.H{"stored": false})
		return
	}
	hwAssistantStoreFeedbackRequest(c, "widget", c.GetInt("id"), false)
}

// HwAdminAgentFeedback stores a thumb, with an optional correction, from the
// owner's panel.
func HwAdminAgentFeedback(c *gin.Context) {
	if !hwRequireAdmin(c) {
		return
	}
	hwAssistantStoreFeedbackRequest(c, "admin", c.GetInt("id"), true)
}

// HwAssistantListFeedback lists the newest feedback, optionally one rating
// and one review state (open, approved, rejected, abuse).
func HwAssistantListFeedback(c *gin.Context) {
	if !hwRequireAdmin(c) {
		return
	}
	rating, _ := strconv.Atoi(c.Query("rating"))
	if rating != 0 && rating != 1 && rating != -1 {
		common.ApiErrorMsg(c, "invalid rating")
		return
	}
	review := c.Query("review")
	switch review {
	case "", model.HwAssistantReviewOpen, model.HwAssistantReviewApproved, model.HwAssistantReviewRejected, model.HwAssistantReviewAbuse:
	default:
		common.ApiErrorMsg(c, "invalid review")
		return
	}
	rows, err := model.HwAssistantListFeedback(model.DB, rating, review, model.HwAssistantFeedbackMaxList)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rows)
}

// HwAssistantReviewFeedback stores the verdict of the bureau on one customer
// thumb: approved (teaches), rejected or abuse (never teaches).
func HwAssistantReviewFeedback(c *gin.Context) {
	if !hwRequireAdmin(c) {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid id")
		return
	}
	var request struct {
		Verdict string `json:"verdict"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "invalid request")
		return
	}
	switch request.Verdict {
	case model.HwAssistantReviewApproved, model.HwAssistantReviewRejected, model.HwAssistantReviewAbuse:
	default:
		common.ApiErrorMsg(c, "invalid verdict")
		return
	}
	if err := model.HwAssistantReviewFeedback(model.DB, id, request.Verdict); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorMsg(c, "feedback not found")
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": id, "review": request.Verdict})
}
