package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// A relay key stored only as a hash (HW_TOKEN_KEY_HASH_ONLY, G2 part B) is shown
// once, in the response that creates it. Later requests for the full key get a
// 409 that says so; the step-up proof is still required first.

// hwTokenCreatedBody is the answer of the create endpoint. With hash-only
// storage it carries the key once under data.key; otherwise it is the body the
// endpoint always had.
func hwTokenCreatedBody(key string) gin.H {
	body := gin.H{"success": true, "message": ""}
	if model.HwTokenKeyHashOnly() {
		body["data"] = gin.H{"key": key, "shown_once": true}
	}
	return body
}

func hwKeyNotRecoverable(c *gin.Context) {
	c.JSON(http.StatusConflict, gin.H{
		"success": false,
		"code":    "KEY_NOT_RECOVERABLE",
		"message": "This key was shown once when it was created and is not stored. Create a new key.",
	})
}
