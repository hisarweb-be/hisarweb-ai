package controller

import (
	"io"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// hwTelegramWebhookMaxBodyBytes caps the update body this handler reads itself;
// the anonymous body limit middleware caps it before the handler runs as well.
const hwTelegramWebhookMaxBodyBytes = 1 << 20

// HwTelegramWebhook receives the bot's updates. It is anonymous; the secret
// header registered with setWebhook is the authentication. The handler reads
// the body itself and passes it on unchanged.
func HwTelegramWebhook(c *gin.Context) {
	rawBody, err := io.ReadAll(io.LimitReader(c.Request.Body, hwTelegramWebhookMaxBodyBytes+1))
	if err != nil || len(rawBody) > hwTelegramWebhookMaxBodyBytes {
		c.Status(http.StatusRequestEntityTooLarge)
		return
	}
	outcome := service.HwHandleTelegramWebhook(
		c.Request.Context(),
		rawBody,
		c.GetHeader(service.HwTelegramSecretHeader),
		time.Now(),
	)
	c.Status(outcome.HTTPStatus())
}
