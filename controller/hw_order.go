package controller

import (
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// hwRevolutWebhookMaxBodyBytes caps the webhook body this handler reads itself;
// the anonymous body limit middleware caps it before the handler runs as well.
const hwRevolutWebhookMaxBodyBytes = 1 << 20

// HwCreateTopUpOrder starts a top-up purchase for the logged-in user. The
// request only names the list value in whole lira; the amount that is charged
// and the credit that follows are computed server-side.
func HwCreateTopUpOrder(c *gin.Context) {
	var request service.HwTopUpRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid request"))
		return
	}
	userID := c.GetInt("id")
	order, err := service.HwCreateTopUpOrder(c.Request.Context(), userID, request, time.Now())
	if err != nil {
		hwTopUpOrderError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"merchant_ref": order.MerchantRef,
		"checkout_url": order.CheckoutURL,
	})
}

func hwTopUpOrderError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrHwTopUpAmountOutOfRange):
		minTry, maxTry := service.HwTopUpRange()
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Top-up value must be between {{min}} TL and {{max}} TL", map[string]any{
			"min": minTry,
			"max": maxTry,
		}))
	case errors.Is(err, service.ErrHwTooManyOpenOrders):
		common.ApiErrorStatus(c, http.StatusConflict, common.NewMessage("You already have an order waiting for payment; finish or wait for it to expire before starting another"))
	case errors.Is(err, service.ErrHwTopUpRateMissing), errors.Is(err, service.ErrHwPaymentProviderUnavailable):
		common.ApiErrorStatus(c, http.StatusServiceUnavailable, common.NewMessage("Top-up is not available right now"))
	default:
		// The error text is not echoed and not logged: it can quote stored data.
		logger.LogError(c.Request.Context(), common.LogText("top-up order creation failed"))
		common.ApiError(c, common.NewMessage("Failed to start the top-up"))
	}
}

// HwRevolutWebhook receives the payment provider's events. It is anonymous; the
// signature of the raw body is the authentication. The handler reads the body
// itself and passes it on unchanged, because the signature covers those exact
// bytes.
func HwRevolutWebhook(c *gin.Context) {
	rawBody, err := io.ReadAll(io.LimitReader(c.Request.Body, hwRevolutWebhookMaxBodyBytes+1))
	if err != nil || len(rawBody) > hwRevolutWebhookMaxBodyBytes {
		c.Status(http.StatusRequestEntityTooLarge)
		return
	}
	outcome := service.HwHandleRevolutWebhook(
		c.Request.Context(),
		rawBody,
		c.GetHeader("Revolut-Request-Timestamp"),
		c.GetHeader("Revolut-Signature"),
		time.Now(),
	)
	c.Status(outcome.HTTPStatus())
}
