package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

// hwTakenEmailAnswer answers the "send me a verification code" request for an
// address that already has an account. By default it is the same success a
// fresh address gets, so the form cannot be used to find out who is registered
// (ASVS V2.2.1 and the Authentication Cheat Sheet on enumeration). A deployment
// that prefers the old explicit message sets HW_REVEAL_TAKEN_EMAIL=true.
func hwTakenEmailAnswer(c *gin.Context) {
	if common.GetEnvOrDefaultBool("HW_REVEAL_TAKEN_EMAIL", false) {
		common.ApiErrorT(c, "Email address is already in use")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}
