package controller

// The public side of the assistant. A clicked chip is answered by its id from
// the fixed list, without a model call. Free text goes to the model with the
// shared knowledge (service/hw_assistant_knowledge.go), bounded by a day
// budget per asker and for everyone together.

import (
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

const (
	// hwAssistantChatDayLimit bounds the model answers of one customer (or one
	// address without a login) per day, hwAssistantChatDayLimitAll those of
	// everyone together: the widget can never run up a large bill.
	hwAssistantChatDayLimit    = 30
	hwAssistantChatDayLimitAll = 3000
)

// hwAssistantChatCall answers through the gateway; tests replace it.
var hwAssistantChatCall = service.HwAssistantChat

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
	ID   string `json:"id"`
	Lang string `json:"lang"`
}

// HwAssistantAsk answers a clicked chip by its id and counts it. Free text
// never gets a keyword answer here; it goes to HwAssistantChat.
func HwAssistantAsk(c *gin.Context) {
	hwAssistantAskByID(c, false)
}

// hwAssistantAskByID is the chip answer of the widget and of the owner's
// panel, where the identity entry speaks as Altuner.
func hwAssistantAskByID(c *gin.Context, admin bool) {
	var request hwAssistantAskRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "invalid request")
		return
	}
	id := strings.TrimSpace(request.ID)
	answer, ok := service.HwAssistantAnswerByID(id, request.Lang, admin)
	if !ok {
		common.ApiSuccess(c, gin.H{"matched": false})
		return
	}
	_ = model.HwAssistantRecordHit(model.DB, "faq:"+id, common.GetTimestamp())
	common.ApiSuccess(c, gin.H{"matched": true, "id": id, "answer": answer})
}

type hwAssistantChatRequest struct {
	Messages []service.HwAdminAgentTurn `json:"messages"`
	Lang     string                     `json:"lang"`
}

// hwAssistantChatAllowed counts one model answer against the day budget of
// the asker and of everyone together. A counter that cannot be written counts
// as used up: the budget fails closed.
func hwAssistantChatAllowed(c *gin.Context, now int64) bool {
	day := time.Unix(now, 0).UTC().Format("2006-01-02")
	asker := "ip:" + c.ClientIP()
	if id := c.GetInt("id"); id > 0 {
		asker = "u:" + strconv.Itoa(id)
	}
	own, err := model.HwAssistantBumpCounter(model.DB, "hw:wchat:"+asker+":"+day, now)
	if err != nil || own > hwAssistantChatDayLimit {
		return false
	}
	all, err := model.HwAssistantBumpCounter(model.DB, "hw:wchat:all:"+day, now)
	return err == nil && all <= hwAssistantChatDayLimitAll
}

// HwAssistantChat answers free text of a customer or a visitor through the
// model, with the shared knowledge and voice. Attachments are dropped: the
// public widget sends text only. A model failure is answered with the
// fallback, never with the error.
func HwAssistantChat(c *gin.Context) {
	var request hwAssistantChatRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "invalid request")
		return
	}
	for index := range request.Messages {
		request.Messages[index].Attachments = nil
	}
	turns, err := service.HwAdminAgentValidate(request.Messages)
	if err != nil {
		common.ApiErrorMsg(c, "invalid messages")
		return
	}
	question := turns[len(turns)-1].Content
	now := common.GetTimestamp()
	if !hwAssistantChatAllowed(c, now) {
		common.ApiSuccess(c, gin.H{"reply": service.HwAssistantLimitedReply(request.Lang, question), "limited": true})
		return
	}
	_ = model.HwAssistantRecordHit(model.DB, "q:"+service.HwAssistantFold(question), now)
	reply, _ := hwAssistantChatCall(c.Request.Context(), service.HwAdminAgentConfigFromEnv(), service.HwAssistantChannelWidget, request.Lang, turns)
	common.ApiSuccess(c, gin.H{"reply": reply, "limited": false})
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
