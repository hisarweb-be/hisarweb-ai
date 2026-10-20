package service

import (
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

// Error codes this fork owns. They are the only codes the client may see for
// routing problems, so masking can keep our wording and replace everything the
// upstream produced.
const (
	HwErrorCodeServiceBusy        types.ErrorCode = "service_busy"
	HwErrorCodeStreamInterrupted  types.ErrorCode = "stream_interrupted"
	HwErrorCodeWindowLimitReached types.ErrorCode = "window_limit_reached"
)

// HwIsOwnErrorCode reports whether code belongs to an error this fork generated
// itself, so its message can be shown to the customer unchanged. Besides the
// routing codes above this covers the gateway's own rejections that happen
// before a supplier is involved: quota, request validation and access control.
// Supplier wording keeps arriving under different codes (for example the
// upstream quota variant "insufficient_quota") and is still replaced.
func HwIsOwnErrorCode(code string) bool {
	switch code {
	case string(HwErrorCodeServiceBusy), string(HwErrorCodeStreamInterrupted),
		string(HwErrorCodeWindowLimitReached),
		string(types.ErrorCodeInsufficientUserQuota), string(types.ErrorCodePreConsumeTokenQuotaFailed),
		string(types.ErrorCodeInvalidRequest), string(types.ErrorCodeAccessDenied),
		string(types.ErrorCodeSensitiveWordsDetected):
		return true
	}
	return false
}

// HwPrefersTurkish reports whether the request asked for Turkish. The service
// texts are fixed in code because the shipped catalogs only cover English and
// Chinese.
func HwPrefersTurkish(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	header := strings.TrimSpace(c.GetHeader("Accept-Language"))
	if header == "" {
		return false
	}
	first := header
	if idx := strings.IndexByte(first, ','); idx >= 0 {
		first = first[:idx]
	}
	if idx := strings.IndexByte(first, ';'); idx >= 0 {
		first = first[:idx]
	}
	first = strings.ToLower(strings.TrimSpace(first))
	return first == "tr" || strings.HasPrefix(first, "tr-")
}

// HwServiceBusyMessage is the customer-facing text used when no supplier could
// serve the request. It names no provider and no internal detail.
func HwServiceBusyMessage(c *gin.Context) string {
	if HwPrefersTurkish(c) {
		return "Hizmet şu anda yoğun, lütfen kısa süre sonra tekrar deneyin."
	}
	return "The service is currently busy. Please try again shortly."
}

// HwGenericErrorMessage returns a neutral message for one error class. Upstream
// wording never reaches the customer; the upstream text stays in the admin log.
func HwGenericErrorMessage(c *gin.Context, status int) string {
	turkish := HwPrefersTurkish(c)
	switch {
	case status == http.StatusTooManyRequests:
		if turkish {
			return "Çok fazla istek gönderildi, lütfen kısa süre sonra tekrar deneyin."
		}
		return "Too many requests. Please try again shortly."
	case status >= http.StatusInternalServerError:
		if turkish {
			return "Hizmet şu anda kullanılamıyor, lütfen kısa süre sonra tekrar deneyin."
		}
		return "The service is temporarily unavailable. Please try again shortly."
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		if turkish {
			return "Bu istek için yetkiniz yok."
		}
		return "This request is not permitted."
	case status == http.StatusNotFound:
		if turkish {
			return "İstenen kaynak bulunamadı."
		}
		return "The requested resource was not found."
	case status == http.StatusRequestEntityTooLarge:
		if turkish {
			return "İstek çok büyük."
		}
		return "The request is too large."
	case status == http.StatusBadRequest || status == http.StatusUnprocessableEntity:
		if turkish {
			return "İstek geçersiz."
		}
		return "The request is invalid."
	}
	if turkish {
		return "İstek şu anda işlenemedi."
	}
	return "The request could not be processed right now."
}

// HwServiceBusyError is the terminal error for an exhausted supplier route.
func HwServiceBusyError(c *gin.Context) *types.NewAPIError {
	return types.NewErrorWithStatusCode(
		errors.New(HwServiceBusyMessage(c)),
		HwErrorCodeServiceBusy,
		http.StatusServiceUnavailable,
		types.ErrOptionWithSkipRetry(),
	)
}
