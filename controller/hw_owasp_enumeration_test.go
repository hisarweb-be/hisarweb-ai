package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// G5: a taken address gets the same answer as a fresh one unless the operator
// asks for the explicit message.
func TestHwTakenEmailAnswerIsNeutralByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	answer := func() string {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/verification", nil)
		hwTakenEmailAnswer(c)
		return recorder.Body.String()
	}
	assert.JSONEq(t, `{"success":true,"message":""}`, answer())

	t.Setenv("HW_REVEAL_TAKEN_EMAIL", "true")
	assert.Contains(t, answer(), `"success":false`)
}
