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
)

// hwRevolutWebhookMaxBodyBytes caps the webhook body this handler reads itself;
// the anonymous body limit middleware caps it before the handler runs as well.
const hwRevolutWebhookMaxBodyBytes = 1 << 20

// HwCreateTopUpOrder starts a top-up purchase for the logged-in user. The
// request only names the list value in whole units of one currency (TRY, EUR or
// USD); the amount that is charged and the credit that follows are computed
// server-side.
func HwCreateTopUpOrder(c *gin.Context) {
	var request service.HwTopUpRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid request"))
		return
	}
	userID := c.GetInt("id")
	order, err := service.HwCreateTopUpOrder(c.Request.Context(), userID, request, time.Now())
	if err != nil {
		hwTopUpOrderError(c, err, service.HwRequestCurrency(request))
		return
	}
	common.ApiSuccess(c, gin.H{
		"merchant_ref": order.MerchantRef,
		"checkout_url": order.CheckoutURL,
	})
}

func hwTopUpOrderError(c *gin.Context, err error, currency string) {
	switch {
	case errors.Is(err, service.ErrHwCurrencyUnsupported):
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid request"))
	case errors.Is(err, service.ErrHwTopUpAmountOutOfRange):
		minAmount, maxAmount, rangeErr := service.HwTopUpRange(currency, time.Now())
		if rangeErr != nil {
			common.ApiErrorStatus(c, http.StatusServiceUnavailable, common.NewMessage("Top-up is not available right now"))
			return
		}
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Top-up value must be between {{min}} {{currency}} and {{max}} {{currency}}", map[string]any{
			"min":      minAmount,
			"max":      maxAmount,
			"currency": currency,
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

// HwStripeWebhook receives Stripe's events. Same rules as the Revolut webhook:
// anonymous, the Stripe-Signature over the raw body is the authentication.
func HwStripeWebhook(c *gin.Context) {
	rawBody, err := io.ReadAll(io.LimitReader(c.Request.Body, hwRevolutWebhookMaxBodyBytes+1))
	if err != nil || len(rawBody) > hwRevolutWebhookMaxBodyBytes {
		c.Status(http.StatusRequestEntityTooLarge)
		return
	}
	outcome := service.HwHandleStripeWebhook(c.Request.Context(), rawBody, c.GetHeader("Stripe-Signature"), time.Now())
	c.Status(outcome.HTTPStatus())
}

// HwAdminRefundOrder is the admin route that takes back the credit of one paid
// order (see service.HwAdminRefundOrder). Refund and dispute webhook events are
// not acted on automatically — their event names are unconfirmed — so this route
// is the one deliberate path that reverses a purchase.
func HwAdminRefundOrder(c *gin.Context) {
	var request struct {
		MerchantRef string `json:"merchant_ref"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.MerchantRef) == "" {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid request"))
		return
	}
	result, err := service.HwAdminRefundOrder(request.MerchantRef, time.Now())
	if err != nil {
		switch {
		case errors.Is(err, service.ErrHwOrderNotFound):
			common.ApiErrorStatus(c, http.StatusNotFound, common.NewMessage("Order not found"))
		case errors.Is(err, service.ErrHwOrderNotRefundable), errors.Is(err, model.ErrHwInvalidOrder):
			common.ApiErrorStatus(c, http.StatusConflict, common.NewMessage("Order is not in a refundable state"))
		case errors.Is(err, service.ErrHwRefundFeeUnavailable):
			// Nothing was changed: the cost of the payment is not booked yet (or
			// cannot be read), so the refund can simply be repeated later.
			common.ApiErrorStatus(c, http.StatusServiceUnavailable, common.NewMessage("The payment provider's cost for this order is not available yet; nothing was changed, repeat the refund later"))
		case errors.Is(err, service.ErrHwProviderRefundFailed):
			common.ApiErrorStatus(c, http.StatusBadGateway, common.NewMessage("The credit was taken back but the card refund failed; repeat the refund to retry it"))
		default:
			logger.LogError(c.Request.Context(), common.LogText("admin refund failed"))
			common.ApiError(c, common.NewMessage("Failed to refund the order"))
		}
		return
	}
	common.ApiSuccess(c, gin.H{
		"merchant_ref":         request.MerchantRef,
		"found":                result.Found,
		"refunded":             result.Refunded,
		"granted_quota":        result.GrantedQuota,
		"returned_quota":       result.ReturnedQuota,
		"spent_quota":          result.SpentQuota,
		"bonus_returned_quota": result.BonusReturnedQuota,
		"note":                 result.Note,
		"currency":             result.Currency,
		"fee_minor":            result.FeeMinor,
		"refunded_minor":       result.RefundedMinor,
	})
}
