package controller

// The admin agent endpoints. Every route sits behind middleware.AdminAuth and
// each handler checks the role again, so a mistake in the router never opens
// the agent to a customer. Only the text-override read is public: the
// customer pages merge it over the interface texts at start-up.

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// Replaceable in tests; production calls the gateway and the bureau board.
var (
	hwAdminAgentAsk        = service.HwAdminAgentAsk
	hwAdminAgentCreateCard = service.HwBureauCreateCard
)

func hwRequireAdmin(c *gin.Context) bool {
	if c.GetInt("role") < common.RoleAdminUser {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "admin access required"})
		return false
	}
	return true
}

type hwAdminAgentChatRequest struct {
	Messages []service.HwAdminAgentTurn `json:"messages"`
}

// HwAdminAgentChat answers one chat turn of the owner.
func HwAdminAgentChat(c *gin.Context) {
	if !hwRequireAdmin(c) {
		return
	}
	var request hwAdminAgentChatRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "invalid request")
		return
	}
	if _, err := service.HwAdminAgentValidate(request.Messages); err != nil {
		common.ApiError(c, err)
		return
	}
	answer, err := hwAdminAgentAsk(c.Request.Context(), service.HwAdminAgentConfigFromEnv(), request.Messages)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, answer)
}

type hwAdminAgentTextRequest struct {
	Lang  string `json:"lang"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

// HwAdminAgentApplyText stores a text override: the one-click apply.
func HwAdminAgentApplyText(c *gin.Context) {
	if !hwRequireAdmin(c) {
		return
	}
	var request hwAdminAgentTextRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "invalid request")
		return
	}
	if err := model.HwSetTextOverride(model.DB, strings.TrimSpace(request.Lang), request.Key, strings.TrimSpace(request.Value), c.GetInt("id"), common.GetTimestamp()); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// HwAdminAgentRevertText removes a text override: the undo.
func HwAdminAgentRevertText(c *gin.Context) {
	if !hwRequireAdmin(c) {
		return
	}
	var request hwAdminAgentTextRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "invalid request")
		return
	}
	removed, err := model.HwDeleteTextOverride(model.DB, strings.TrimSpace(request.Lang), request.Key)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"removed": removed})
}

// HwAdminAgentListTexts lists every applied override.
func HwAdminAgentListTexts(c *gin.Context) {
	if !hwRequireAdmin(c) {
		return
	}
	rows, err := model.HwListTextOverrides(model.DB)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rows)
}

type hwAdminAgentEscalateRequest struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Page    string `json:"page"`
}

// HwAdminAgentEscalate stores the escalation and creates a card on the Bureau
// V2 board. The server may not reach the board; the escalation then stays
// queued for HwAdminAgentPendingEscalations and the caller is told so. Once the
// board has answered, the card exists and the caller always gets it, even when
// the row could not be updated.
func HwAdminAgentEscalate(c *gin.Context) {
	if !hwRequireAdmin(c) {
		return
	}
	var request hwAdminAgentEscalateRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "invalid request")
		return
	}
	row, err := model.HwCreateAdminEscalation(model.DB, request.Title, request.Summary, request.Page, c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	card, err := hwAdminAgentCreateCard(c.Request.Context(), service.HwBureauCardConfigFromEnv(), request.Title, request.Summary, request.Page)
	if err != nil {
		common.SysLog("hw: escalation " + strconv.Itoa(row.ID) + " queued, board not reached: " + err.Error())
		common.ApiSuccess(c, gin.H{"queued": true, "id": row.ID})
		return
	}
	if err := model.HwSetAdminEscalationCard(model.DB, row.ID, card); err != nil {
		// The card exists on the board and cannot be removed; answering with a
		// failure would make the operator create a second card for the same
		// problem, so the caller gets the card it already has.
		common.SysLog("hw: escalation " + strconv.Itoa(row.ID) + " has card " + card + " but the row was not updated: " + err.Error())
	}
	common.ApiSuccess(c, gin.H{"card": card, "id": row.ID})
}

// HwAdminAgentPendingEscalations lists escalations that have no board card yet.
func HwAdminAgentPendingEscalations(c *gin.Context) {
	if !hwRequireAdmin(c) {
		return
	}
	rows, err := model.HwListPendingAdminEscalations(model.DB)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rows)
}

type hwAdminAgentEscalationCardRequest struct {
	ID   int    `json:"id"`
	Card string `json:"card"`
}

// HwAdminAgentLinkEscalationCard records the card that a sync created.
func HwAdminAgentLinkEscalationCard(c *gin.Context) {
	if !hwRequireAdmin(c) {
		return
	}
	var request hwAdminAgentEscalationCardRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.ID <= 0 || strings.TrimSpace(request.Card) == "" {
		common.ApiErrorMsg(c, "invalid request")
		return
	}
	if err := model.HwSetAdminEscalationCard(model.DB, request.ID, strings.TrimSpace(request.Card)); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// HwTextOverrides is the public read of one language's overrides.
func HwTextOverrides(c *gin.Context) {
	lang := strings.TrimSpace(c.Query("lang"))
	if lang == "" {
		lang = "tr"
	}
	overrides, err := model.HwTextOverridesFor(model.DB, lang)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, overrides)
}
