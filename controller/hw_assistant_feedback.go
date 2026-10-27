package controller

// Thumbs up/down on assistant answers. The stored rows teach later prompts
// (service.HwAssistantLearnedSection); admins can also add a correction and
// list what was rated.

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
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

// HwAssistantFeedback stores a thumb from the public widget.
func HwAssistantFeedback(c *gin.Context) {
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

// HwAssistantListFeedback lists the newest feedback, optionally one rating.
func HwAssistantListFeedback(c *gin.Context) {
	if !hwRequireAdmin(c) {
		return
	}
	rating, _ := strconv.Atoi(c.Query("rating"))
	if rating != 0 && rating != 1 && rating != -1 {
		common.ApiErrorMsg(c, "invalid rating")
		return
	}
	rows, err := model.HwAssistantListFeedback(model.DB, rating, model.HwAssistantFeedbackMaxList)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rows)
}
