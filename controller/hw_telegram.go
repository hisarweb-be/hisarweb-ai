package controller

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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

// HwTelegramLinkStart hands a logged-in user a one-time deep link to the bot.
// Opening it makes Telegram send "/start <token>" to the webhook, which links
// the sender's Telegram account to this user. Only the link is returned; the
// token behind it is never logged and only its hash is stored.
func HwTelegramLinkStart(c *gin.Context) {
	botName := hwTelegramBotName()
	if botName == "" || !service.HwTelegramBotConfigured() {
		common.ApiErrorT(c, "Telegram bot is not configured.")
		return
	}
	userID := c.GetInt("id")
	user, err := model.GetUserById(userID, false)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorStatus(c, http.StatusNotFound, common.NewMessage("User not found"))
			return
		}
		logger.LogError(c.Request.Context(), common.LogText("reading the user for a Telegram link failed"))
		common.ApiError(c, common.NewMessage("Failed to create the Telegram link"))
		return
	}
	linked := strings.TrimSpace(user.TelegramId) != ""
	if linked {
		// A linked account that still owes the phone step gets a link as well:
		// opening it in the same Telegram account answers with the share-contact
		// button, which is what the customer needs next. Only a finished phone
		// step ends the links.
		phoneShared, err := model.HwUserHasPhoneSignal(userID)
		if err != nil {
			logger.LogError(c.Request.Context(), common.LogText("reading the phone step failed"))
		}
		if phoneShared {
			common.ApiSuccess(c, gin.H{"linked": true})
			return
		}
	}
	raw, expiresAt, err := model.HwCreateTelegramLinkToken(userID, time.Now().Unix())
	if err != nil {
		logger.LogError(c.Request.Context(), common.LogText("creating a Telegram link token failed"))
		common.ApiError(c, common.NewMessage("Failed to create the Telegram link"))
		return
	}
	common.ApiSuccess(c, gin.H{
		"linked":     linked,
		"url":        "https://t.me/" + botName + "?start=" + raw,
		"expires_at": expiresAt,
	})
}

// HwTelegramLinkStatus answers how far the logged-in user is with the Telegram
// part of the start bonus, so the link dialog can follow the bot conversation
// without a reload: linked, phone number shared, and free credit granted. It
// returns three flags and nothing that identifies the Telegram account.
func HwTelegramLinkStatus(c *gin.Context) {
	userID := c.GetInt("id")
	user, err := model.GetUserById(userID, false)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorStatus(c, http.StatusNotFound, common.NewMessage("User not found"))
			return
		}
		logger.LogError(c.Request.Context(), common.LogText("reading the user for the Telegram link status failed"))
		common.ApiError(c, common.NewMessage("Failed to read the Telegram link status"))
		return
	}
	phoneShared, err := model.HwUserHasPhoneSignal(userID)
	if err != nil {
		logger.LogError(c.Request.Context(), common.LogText("reading the phone step failed"))
		common.ApiError(c, common.NewMessage("Failed to read the Telegram link status"))
		return
	}
	bonusGranted, err := model.HwUserHasActiveFreeCredit(userID)
	if err != nil {
		logger.LogError(c.Request.Context(), common.LogText("reading the free credit failed"))
		common.ApiError(c, common.NewMessage("Failed to read the Telegram link status"))
		return
	}
	common.ApiSuccess(c, gin.H{
		"linked":        strings.TrimSpace(user.TelegramId) != "",
		"phone_shared":  phoneShared,
		"bonus_granted": bonusGranted,
	})
}

// hwTelegramBotName is the configured bot user name without its leading @, or
// empty when it is missing or not a name Telegram can carry. Refusing a bad
// value keeps it out of the URL the customer receives.
func hwTelegramBotName() string {
	name := strings.TrimPrefix(strings.TrimSpace(common.TelegramBotName), "@")
	if len(name) < 5 || len(name) > 32 {
		return ""
	}
	for i := range len(name) {
		switch c := name[i]; {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '_':
		default:
			return ""
		}
	}
	return name
}
