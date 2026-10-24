package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// HwAdminUnfreeze thaws one account after an abuse review. The route sits
// behind AdminAuth; the handler checks the role again so a routing mistake can
// never expose it. The response carries the user ID and the outcome only.
func HwAdminUnfreeze(c *gin.Context) {
	if c.GetInt("role") < common.RoleAdminUser {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "Only administrators can unfreeze accounts",
		})
		return
	}
	var request struct {
		UserId int `json:"user_id"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.UserId <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	unfrozen, err := model.HwUnfreezeAccount(request.UserId, c.GetInt("id"), common.GetTimestamp())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !unfrozen {
		common.ApiErrorT(c, "Account is not frozen")
		return
	}
	common.SysLog(common.LogText("hw: account %d unfrozen by admin %d", request.UserId, c.GetInt("id")))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    gin.H{"user_id": request.UserId},
	})
}

// HwAdminChannelGrant gives a partner account the channel credit and caps its
// keys. It can be repeated: the TRY bonus follows once the account has done the
// free steps.
func HwAdminChannelGrant(c *gin.Context) {
	if c.GetInt("role") < common.RoleAdminUser {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "Only administrators can grant channel credit",
		})
		return
	}
	var request struct {
		UserId int `json:"user_id"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.UserId <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	result, err := model.HwGrantChannel(request.UserId, common.GetTimestamp())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.SysLog(common.LogText("hw: channel credit for account %d by admin %d", request.UserId, c.GetInt("id")))
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": result})
}

// HwAdminChannels lists the partner channels with signups, paying customers and
// earned credit.
func HwAdminChannels(c *gin.Context) {
	if c.GetInt("role") < common.RoleAdminUser {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "Only administrators can read channels",
		})
		return
	}
	rows, err := model.HwListChannels()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": rows})
}
