package service

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Defaults of the top-up settings, used while the operator has not overridden
// them. HwTryPerUsdOption has no default: the daily rate job (part D) delivers
// it, and without it the price of an order cannot be computed.
const (
	hwDefaultMinTopUpTry   = 100
	hwDefaultMaxTopUpTry   = 20000
	hwDefaultMaxOpenOrders = 3

	// hwMaxOrderMoneyMinor bounds an amount in kuruş well below the 64-bit range,
	// so a misconfigured rate can never overflow the payable amount.
	hwMaxOrderMoneyMinor = 1_000_000_000_000
)

// Errors of the top-up order flow, as far as the API answers them differently.
var (
	ErrHwTopUpRateMissing           = errors.New("try per usd rate is not configured")
	ErrHwTopUpAmountOutOfRange      = errors.New("top-up amount is outside the allowed range")
	ErrHwTooManyOpenOrders          = errors.New("too many open top-up orders")
	ErrHwPaymentProviderUnavailable = errors.New("payment provider is unavailable")
)

// HwTopUpRequest is the client request to start a purchase. Only the list value
// in whole lira is taken from it; every amount that is charged or credited is
// computed from our own pricing.
type HwTopUpRequest struct {
	PackageTry int    `json:"package_try"`
	Language   string `json:"language"`
}

// HwTopUpOrder is what the client receives: our own order reference and the
// provider's checkout page. Nothing else about the provider is exposed.
type HwTopUpOrder struct {
	MerchantRef string `json:"merchant_ref"`
	CheckoutURL string `json:"checkout_url"`
}

// HwTopUpRange returns the list-value bounds in whole lira.
func HwTopUpRange() (minTry int, maxTry int) {
	minTry = hwIntOption(model.HwMinTopUpTryOption, hwDefaultMinTopUpTry)
	maxTry = hwIntOption(model.HwMaxTopUpTryOption, hwDefaultMaxTopUpTry)
	if maxTry < minTry {
		maxTry = minTry
	}
	return minTry, maxTry
}

func hwOption(key string) string {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return strings.TrimSpace(common.OptionMap[key])
}

func hwIntOption(key string, fallback int) int {
	value := hwOption(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

// hwTryPerUsd reads the TRY price of one US dollar that part D publishes.
func hwTryPerUsd() (decimal.Decimal, error) {
	value := hwOption(model.HwTryPerUsdOption)
	rate, err := decimal.NewFromString(value)
	if err != nil || !rate.IsPositive() {
		return decimal.Decimal{}, ErrHwTopUpRateMissing
	}
	return rate, nil
}

// HwCreateTopUpOrder starts a purchase: it claims the first-purchase discount,
// prices the order from our own data, stores it and creates the provider
// checkout. A failure before the checkout exists releases the claim again, so a
// retry still gets the discount.
func HwCreateTopUpOrder(ctx context.Context, userID int, request HwTopUpRequest, now time.Time) (HwTopUpOrder, error) {
	if userID <= 0 {
		return HwTopUpOrder{}, model.ErrHwInvalidOrder
	}
	minTry, maxTry := HwTopUpRange()
	if request.PackageTry < minTry || request.PackageTry > maxTry {
		return HwTopUpOrder{}, ErrHwTopUpAmountOutOfRange
	}
	rate, err := hwTryPerUsd()
	if err != nil {
		return HwTopUpOrder{}, err
	}
	// The open-order count is advisory: two requests in the same instant can both
	// pass it. It bounds abandoned checkouts, not money, so a stale count is
	// harmless.
	openOrders, err := model.HwCountOpenOrders(userID)
	if err != nil {
		return HwTopUpOrder{}, err
	}
	if openOrders >= int64(hwIntOption(model.HwMaxOpenOrdersOption, hwDefaultMaxOpenOrders)) {
		return HwTopUpOrder{}, ErrHwTooManyOpenOrders
	}
	user, err := model.GetUserById(userID, false)
	if err != nil {
		return HwTopUpOrder{}, err
	}
	if user.Status != common.UserStatusEnabled {
		return HwTopUpOrder{}, model.ErrHwInvalidOrder
	}

	listValue := decimal.NewFromInt(int64(request.PackageTry))
	merchantRef := "hw-" + uuid.NewString()
	// Two questions decide the discount. HwIsFirstPaidPurchase says whether this
	// customer ever paid; the claim says whether the discount is still available
	// and records it for this order, so two concurrent orders cannot both take it
	// and a second account on the same phone or card is refused.
	firstPaidPurchase, err := model.HwIsFirstPaidPurchase(nil, userID)
	if err != nil {
		return HwTopUpOrder{}, err
	}
	firstPurchase := false
	if firstPaidPurchase {
		firstPurchase, _, err = model.HwClaimFirstPurchaseDiscount(nil, userID, merchantRef, now.Unix())
		if err != nil {
			return HwTopUpOrder{}, err
		}
	}
	releaseClaim := func() {
		if err := model.HwReleaseFirstPurchaseClaim(nil, userID, merchantRef); err != nil {
			common.SysError(common.LogText("could not release the first-purchase claim of order %s (class %s)", merchantRef, hwPaymentErrorClass(err)))
		}
	}
	quote, err := model.HwComputePurchase(listValue, firstPurchase)
	if err != nil {
		releaseClaim()
		return HwTopUpOrder{}, err
	}
	listQuota, err := common.WalletQuotaFromDecimalStrict(listValue.Div(rate).Mul(decimal.NewFromFloat(common.QuotaPerUnit)))
	if err != nil || listQuota <= 0 {
		releaseClaim()
		return HwTopUpOrder{}, ErrHwTopUpAmountOutOfRange
	}
	payableMinor := quote.PayableTRY.Mul(decimal.NewFromInt(100)).Round(0)
	if !payableMinor.IsPositive() || payableMinor.GreaterThan(decimal.NewFromInt(hwMaxOrderMoneyMinor)) {
		releaseClaim()
		return HwTopUpOrder{}, ErrHwTopUpAmountOutOfRange
	}

	discountKind := model.HwDiscountReturning
	if firstPurchase {
		discountKind = model.HwDiscountFirstPurchase
	}
	order := &model.HwOrder{
		UserId:            userID,
		MerchantRef:       merchantRef,
		ListQuota:         listQuota,
		ListValueTryMinor: int64(request.PackageTry) * 100,
		PayableTryMinor:   payableMinor.IntPart(),
		Currency:          model.HwOrderCurrency,
		DiscountKind:      discountKind,
		Status:            model.HwOrderStatusCreated,
		CreatedAt:         now.Unix(),
	}
	if err := model.HwCreateOrder(order); err != nil {
		releaseClaim()
		return HwTopUpOrder{}, err
	}

	config, err := HwRevolutConfigFromEnv()
	if err != nil {
		hwAbandonOrder(merchantRef, releaseClaim, now)
		return HwTopUpOrder{}, ErrHwPaymentProviderUnavailable
	}
	providerOrder, err := config.CreateOrder(ctx, hwRevolutOrderRequest{
		Amount:            order.PayableTryMinor,
		Currency:          model.HwOrderCurrency,
		Description:       hwTopUpDescription(request.Language),
		MerchantOrderData: &hwRevolutMerchantOrderData{Reference: merchantRef},
		Customer:          &hwRevolutCustomer{Email: strings.TrimSpace(user.Email)},
		RedirectURL:       hwOption(model.HwCheckoutReturnURLOption),
	})
	if err != nil {
		common.SysError(common.LogText("creating the payment order %s failed (class %s)", merchantRef, hwPaymentErrorClass(err)))
		hwAbandonOrder(merchantRef, releaseClaim, now)
		return HwTopUpOrder{}, ErrHwPaymentProviderUnavailable
	}
	if strings.TrimSpace(providerOrder.Id) == "" || strings.TrimSpace(providerOrder.CheckoutURL) == "" {
		common.SysError(common.LogText("creating the payment order %s returned no usable checkout", merchantRef))
		hwAbandonOrder(merchantRef, releaseClaim, now)
		return HwTopUpOrder{}, ErrHwPaymentProviderUnavailable
	}
	if err := model.HwAttachRevolutOrder(order.Id, providerOrder.Id, now.Unix()); err != nil {
		// The webhook finds this order by its own reference as well, so the
		// payment still settles even when recording the provider id failed.
		common.SysError(common.LogText("could not record the provider order id of order %s (class %s); the webhook settles it by reference", merchantRef, hwPaymentErrorClass(err)))
	}
	return HwTopUpOrder{MerchantRef: merchantRef, CheckoutURL: providerOrder.CheckoutURL}, nil
}

// hwAbandonOrder closes an order whose checkout never came to exist and gives
// the first-purchase claim back.
func hwAbandonOrder(merchantRef string, releaseClaim func(), now time.Time) {
	releaseClaim()
	if _, err := model.HwMarkOrderNotPaid(merchantRef, model.HwOrderStatusFailed, now.Unix()); err != nil {
		common.SysError(common.LogText("could not close the failed order %s (class %s)", merchantRef, hwPaymentErrorClass(err)))
	}
}

func hwTopUpDescription(language string) string {
	if strings.EqualFold(strings.TrimSpace(language), "tr") {
		return "HisarWeb AI kredi yüklemesi"
	}
	return "HisarWeb AI credit top-up"
}

// HwWebhookOutcome says how a provider webhook was answered. Only 2xx stops the
// provider from retrying.
type HwWebhookOutcome int

const (
	// HwWebhookAccepted: the event was handled (or was a harmless repeat).
	HwWebhookAccepted HwWebhookOutcome = iota
	// HwWebhookIgnored: the event is unknown or belongs to no order of ours.
	// Answered with 200 so the provider learns nothing about our orders.
	HwWebhookIgnored
	// HwWebhookBadRequest: the signature was valid but the payload is not a
	// webhook event.
	HwWebhookBadRequest
	// HwWebhookRejected: the signature or the timestamp did not check out.
	HwWebhookRejected
	// HwWebhookUnavailable: we cannot handle the event right now; the provider
	// should retry.
	HwWebhookUnavailable
)

// HTTPStatus maps an outcome to the answer of the webhook endpoint.
func (outcome HwWebhookOutcome) HTTPStatus() int {
	switch outcome {
	case HwWebhookRejected:
		return http.StatusUnauthorized
	case HwWebhookBadRequest:
		return http.StatusBadRequest
	case HwWebhookUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusOK
	}
}

// HwHandleRevolutWebhook verifies and processes one provider webhook. Without a
// webhook secret every event is refused with 503: silently accepting unsigned
// events would let anyone mint credit.
//
// The raw body is the signed input; it is never re-serialised. The body, the
// headers and the secrets are never logged, and credit is only written when the
// provider itself reports a completed payment whose amount and currency match
// our own order.
func HwHandleRevolutWebhook(ctx context.Context, rawBody []byte, timestampHeader, signatureHeader string, now time.Time) HwWebhookOutcome {
	if err := HwVerifyRevolutWebhook(rawBody, timestampHeader, signatureHeader, os.Getenv(HwRevolutWebhookSecretEnv), now); err != nil {
		if errors.Is(err, ErrHwRevolutWebhookSecretMissing) {
			common.SysError(common.LogText("revolut webhook refused: %s is not configured", HwRevolutWebhookSecretEnv))
			return HwWebhookUnavailable
		}
		common.SysLog(common.LogText("revolut webhook rejected: %s", err.Error()))
		return HwWebhookRejected
	}
	var event hwRevolutWebhookEvent
	if err := common.Unmarshal(rawBody, &event); err != nil {
		common.SysLog(common.LogText("revolut webhook rejected: the signed body is not an event"))
		return HwWebhookBadRequest
	}
	order, found, err := model.HwFindOrderForWebhook(event.MerchantOrderExtRef, event.OrderId)
	if err != nil {
		common.SysError(common.LogText("revolut webhook could not look up its order (class %s)", hwPaymentErrorClass(err)))
		return HwWebhookUnavailable
	}
	if !found {
		return HwWebhookIgnored
	}
	switch event.Event {
	case hwRevolutEventOrderCompleted:
		return hwCompleteRevolutOrder(ctx, order, event, now)
	case hwRevolutEventOrderCancelled:
		return hwCloseRevolutOrder(order, model.HwOrderStatusCancelled, now)
	case hwRevolutEventPaymentDeclined, hwRevolutEventPaymentFailed:
		return hwCloseRevolutOrder(order, model.HwOrderStatusFailed, now)
	case hwRevolutEventOrderRefunded, hwRevolutEventChargeback:
		return hwRevokeRevolutOrder(order, now)
	default:
		return HwWebhookIgnored
	}
}

func hwCompleteRevolutOrder(ctx context.Context, order *model.HwOrder, event hwRevolutWebhookEvent, now time.Time) HwWebhookOutcome {
	if order.Status == model.HwOrderStatusPaid {
		return HwWebhookAccepted
	}
	providerOrderID := strings.TrimSpace(event.OrderId)
	if order.RevolutOrderId != nil && strings.TrimSpace(*order.RevolutOrderId) != "" {
		providerOrderID = strings.TrimSpace(*order.RevolutOrderId)
	}
	if providerOrderID == "" {
		return HwWebhookIgnored
	}
	config, err := HwRevolutConfigFromEnv()
	if err != nil {
		common.SysError(common.LogText("revolut webhook cannot confirm order %s (class %s)", order.MerchantRef, hwPaymentErrorClass(err)))
		return HwWebhookUnavailable
	}
	providerOrder, err := config.GetOrder(ctx, providerOrderID)
	if err != nil {
		common.SysError(common.LogText("revolut webhook cannot confirm order %s (class %s)", order.MerchantRef, hwPaymentErrorClass(err)))
		return HwWebhookUnavailable
	}
	if providerOrder.State != hwRevolutStateCompleted {
		common.SysLog(common.LogText("revolut webhook left order %s unpaid: the provider does not report a completed payment", order.MerchantRef))
		return HwWebhookIgnored
	}
	if providerOrder.Amount != order.PayableTryMinor || !strings.EqualFold(strings.TrimSpace(providerOrder.Currency), order.Currency) {
		// The amount and the currency come from our own order; a provider report
		// that disagrees is a reason not to credit, never a reason to credit what
		// the provider says.
		common.SysError(common.LogText("revolut webhook refused credit for order %s: the confirmed amount or currency differs from our order", order.MerchantRef))
		return HwWebhookIgnored
	}
	fingerprint := hwRevolutCardFingerprint(providerOrder)
	settlement, err := model.HwSettleOrderPayment(order.MerchantRef, providerOrderID, fingerprint, now.Unix())
	if err != nil {
		common.SysError(common.LogText("revolut webhook could not settle order %s (class %s)", order.MerchantRef, hwPaymentErrorClass(err)))
		return HwWebhookUnavailable
	}
	if !settlement.Paid {
		return HwWebhookAccepted
	}
	if settlement.Granted {
		model.HwSyncCachesAfterGrant(settlement.UserId, settlement.ListQuota, model.HwSourcePurchase)
	}
	if fingerprint != "" {
		if err := model.UpsertHwUserSignals(settlement.UserId, model.HwUserSignals{CardFingerprint: fingerprint}); err != nil {
			common.SysError(common.LogText("could not record the paying card signal of order %s (class %s)", order.MerchantRef, hwPaymentErrorClass(err)))
		}
	}
	granted, reason, err := model.HwGrantAffiliateReward(settlement.UserId, settlement.ListQuota, order.MerchantRef, now.Unix())
	switch {
	case err != nil:
		common.SysError(common.LogText("affiliate reward of order %s failed (class %s)", order.MerchantRef, hwPaymentErrorClass(err)))
	case !granted:
		common.SysLog(common.LogText("no affiliate reward for order %s: %s", order.MerchantRef, reason))
	}
	common.SysLog(common.LogText("revolut webhook credited the top-up of order %s", order.MerchantRef))
	return HwWebhookAccepted
}

func hwCloseRevolutOrder(order *model.HwOrder, status string, now time.Time) HwWebhookOutcome {
	changed, err := model.HwMarkOrderNotPaid(order.MerchantRef, status, now.Unix())
	if err != nil {
		common.SysError(common.LogText("revolut webhook could not close order %s (class %s)", order.MerchantRef, hwPaymentErrorClass(err)))
		return HwWebhookUnavailable
	}
	if changed {
		common.SysLog(common.LogText("revolut webhook closed order %s as %s", order.MerchantRef, status))
	}
	return HwWebhookAccepted
}

func hwRevokeRevolutOrder(order *model.HwOrder, now time.Time) HwWebhookOutcome {
	if err := model.HwRevokeOrderCredit(order.MerchantRef, now.Unix()); err != nil {
		common.SysError(common.LogText("revolut webhook could not revoke the credit of order %s (class %s)", order.MerchantRef, hwPaymentErrorClass(err)))
		return HwWebhookUnavailable
	}
	if err := model.HwRevokeAffiliateReward(order.MerchantRef, now.Unix()); err != nil {
		common.SysError(common.LogText("revolut webhook could not revoke the affiliate reward of order %s (class %s)", order.MerchantRef, hwPaymentErrorClass(err)))
		return HwWebhookUnavailable
	}
	common.SysLog(common.LogText("revolut webhook revoked the credit of order %s", order.MerchantRef))
	return HwWebhookAccepted
}

// hwPaymentErrorClass reduces an error to a log-safe class. Log lines carry the
// class and never the error text: a provider body or a driver error can quote
// request data.
func hwPaymentErrorClass(err error) string {
	var apiErr *HwRevolutAPIError
	switch {
	case errors.Is(err, ErrHwRevolutNotConfigured):
		return "not_configured"
	case errors.Is(err, ErrHwRevolutUnusableResponse):
		return "unusable_response"
	case errors.As(err, &apiErr) && apiErr.Transport:
		return "transport"
	case errors.As(err, &apiErr):
		return "provider_status"
	case errors.Is(err, model.ErrHwInvalidOrder), errors.Is(err, model.ErrHwInvalidCreditGrant):
		return "invalid_request"
	}
	return "internal"
}
