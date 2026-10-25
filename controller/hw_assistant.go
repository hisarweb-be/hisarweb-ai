package controller

// The public side of the assistant. The fixed answers need no login and no
// model call. A question without a fixed answer is only counted (and never
// stored when it looks personal); admins get the model chat through the admin
// agent, customers are pointed at support.

import (
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

const hwAssistantMaxQuestionRunes = 300

func hwAssistantHits() map[string]int {
	rows, err := model.HwAssistantHitsByPrefix(model.DB, "faq:", 100)
	hits := make(map[string]int, len(rows))
	if err != nil {
		return hits
	}
	for _, row := range rows {
		hits[row.Key] = row.Hits
	}
	return hits
}

// HwAssistantFAQ lists the fixed questions, most asked first.
func HwAssistantFAQ(c *gin.Context) {
	common.ApiSuccess(c, gin.H{"faq": service.HwAssistantFAQList(c.Query("lang"), hwAssistantHits())})
}

type hwAssistantAskRequest struct {
	Question string `json:"question"`
	Lang     string `json:"lang"`
}

// HwAssistantAsk answers from the fixed list, or says it has no fixed answer.
func HwAssistantAsk(c *gin.Context) {
	var request hwAssistantAskRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "invalid request")
		return
	}
	question := strings.TrimSpace(request.Question)
	if question == "" || utf8.RuneCountInString(question) > hwAssistantMaxQuestionRunes {
		common.ApiErrorMsg(c, "invalid question")
		return
	}
	now := common.GetTimestamp()
	id, answer, ok := service.HwAssistantAnswerFor(question, request.Lang)
	if !ok {
		_ = model.HwAssistantRecordHit(model.DB, "q:"+service.HwAssistantFold(question), now)
		common.ApiSuccess(c, gin.H{"matched": false})
		return
	}
	_ = model.HwAssistantRecordHit(model.DB, "faq:"+id, now)
	common.ApiSuccess(c, gin.H{"matched": true, "id": id, "answer": answer})
}

// HwAssistantOpenQuestions lists the most asked questions without a fixed
// answer, so an admin knows which answer to add next.
func HwAssistantOpenQuestions(c *gin.Context) {
	if !hwRequireAdmin(c) {
		return
	}
	rows, err := model.HwAssistantHitsByPrefix(model.DB, "q:", 50)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rows)
}
