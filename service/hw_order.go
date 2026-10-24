package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Defaults of the top-up settings, used while the operator has not overridden
// them. HwTryPerUsdOption has no default: the daily rate job (part D) delivers
// it, and without it the price of an order cannot be computed. The same holds
// for HwEurPerUsdOption, which only EUR orders need.
const (
	hwDefaultMinTopUpTry   = 100
	hwDefaultMaxTopUpTry   = 20000
	hwDefaultMaxOpenOrders = 3

	// hwMaxOrderMoneyMinor bounds an amount in minor units well below the 64-bit range,
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
// in whole units of the currency is taken from it; every amount that is charged
// or credited is computed from our own pricing. PackageTry is the old name of
// Amount (always lira) and is used when Amount is absent.
type HwTopUpRequest struct {
	Amount     int    `json:"amount"`
	PackageTry int    `json:"package_try"`
	Currency   string `json:"currency"`
	Language   string `json:"language"`
}

// HwRequestAmount is the list value the request asks for, in whole units of its
// currency.
func HwRequestAmount(request HwTopUpRequest) int {
	if request.Amount != 0 {
		return request.Amount
	}
	return request.PackageTry
}

// HwTopUpOrder is what the client receives: our own order reference and the
// provider's checkout page. Nothing else about the provider is exposed.
type HwTopUpOrder struct {
	MerchantRef string `json:"merchant_ref"`
	CheckoutURL string `json:"checkout_url"`
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

// hwTryPerUsd reads the TRY price of one US dollar that part D publishes. A
// rate that carries a date must not be in the future nor older than
// hwRateMaxAgeDays; an older or future-dated rate counts as missing. A rate
// without a date (set by hand) has no age to check.
func hwTryPerUsd(now time.Time) (decimal.Decimal, error) {
	value := hwOption(model.HwTryPerUsdOption)
	rate, err := decimal.NewFromString(value)
	if err != nil || !rate.IsPositive() {
		return decimal.Decimal{}, ErrHwTopUpRateMissing
	}
	if !hwRateDateUsable(hwOption(HwOptionTryPerUsdDate), now, false) {
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
	currency := HwRequestCurrency(request)
	if !model.HwOrderCurrencyAllowed(currency) {
		return HwTopUpOrder{}, ErrHwCurrencyUnsupported
	}
	rate, err := hwCurrencyRate(currency, now)
	if err != nil {
		return HwTopUpOrder{}, err
	}
	minAmount, maxAmount, err := HwTopUpRange(currency, now)
	if err != nil {
		return HwTopUpOrder{}, err
	}
	amount := HwRequestAmount(request)
	if amount < minAmount || amount > maxAmount {
		return HwTopUpOrder{}, ErrHwTopUpAmountOutOfRange
	}
	// Revolut only takes lira. Refuse before anything is claimed or stored.
	if currency != model.HwCurrencyTRY && !HwStripePaymentAvailable() {
		return HwTopUpOrder{}, ErrHwPaymentProviderUnavailable
	}
	discountCap, err := hwDiscountCap(currency, rate, now)
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

	listValue := decimal.NewFromInt(int64(amount))
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
	quote, err := model.HwComputePurchase(listValue, firstPurchase, discountCap)
	if err != nil {
		releaseClaim()
		return HwTopUpOrder{}, err
	}
	listQuota, err := common.WalletQuotaFromDecimalStrict(listValue.Div(rate).Mul(decimal.NewFromFloat(common.QuotaPerUnit)))
	if err != nil || listQuota <= 0 {
		releaseClaim()
		return HwTopUpOrder{}, ErrHwTopUpAmountOutOfRange
	}
	bonusQuota, bonusMinor, err := hwTopUpBonus(currency, amount, rate)
	if err != nil {
		releaseClaim()
		return HwTopUpOrder{}, ErrHwTopUpAmountOutOfRange
	}
	payableMinor := quote.Payable.Mul(decimal.NewFromInt(100)).Round(0)
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
		ListValueTryMinor: int64(amount) * 100,
		PayableTryMinor:   payableMinor.IntPart(),
		Currency:          currency,
		BonusQuota:        bonusQuota,
		BonusMinor:        bonusMinor,
		DiscountKind:      discountKind,
		Language:          hwLanguageTag(request.Language),
		Status:            model.HwOrderStatusCreated,
		CreatedAt:         now.Unix(),
	}
	if err := model.HwCreateOrder(order); err != nil {
		releaseClaim()
		return HwTopUpOrder{}, err
	}

	providerOrder, err := hwCreateProviderCheckout(ctx, order, user.Email, request.Language)
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

// hwCreateProviderCheckout opens the checkout at the active provider: Stripe
// when its environment is complete, otherwise Revolut.
func hwCreateProviderCheckout(ctx context.Context, order *model.HwOrder, email, language string) (hwRevolutOrderResponse, error) {
	if HwStripePaymentAvailable() {
		config, err := HwStripeConfigFromEnv()
		if err != nil {
			return hwRevolutOrderResponse{}, err
		}
		return config.CreateCheckout(ctx, order, email, language, hwCheckoutReturnURL())
	}
	// Revolut is only configured for lira: an order in another currency never
	// reaches it.
	if order.Currency != model.HwCurrencyTRY {
		return hwRevolutOrderResponse{}, ErrHwPaymentProviderUnavailable
	}
	config, err := HwRevolutConfigFromEnv()
	if err != nil {
		return hwRevolutOrderResponse{}, err
	}
	return config.CreateOrder(ctx, hwRevolutOrderRequest{
		Amount:            order.PayableTryMinor,
		Currency:          order.Currency,
		Description:       hwTopUpDescription(language),
		MerchantOrderData: &hwRevolutMerchantOrderData{Reference: order.MerchantRef},
		Customer:          &hwRevolutCustomer{Email: strings.TrimSpace(email)},
		RedirectURL:       hwOption(model.HwCheckoutReturnURLOption),
	})
}

// hwAbandonOrder closes an order whose checkout never came to exist and gives
// the first-purchase claim back.
func hwAbandonOrder(merchantRef string, releaseClaim func(), now time.Time) {
	releaseClaim()
	if _, err := model.HwMarkOrderNotPaid(merchantRef, model.HwOrderStatusFailed, now.Unix()); err != nil {
		common.SysError(common.LogText("could not close the failed order %s (class %s)", merchantRef, hwPaymentErrorClass(err)))
	}
}

// hwLanguageTag reduces a client language value to the two tags the site
// speaks. Anything that is not English falls back to Turkish, the default
// language of the site (SM-10), both for the provider description and for the
// order record the customer mail job reads.
func hwLanguageTag(language string) string {
	tag := strings.ToLower(strings.TrimSpace(language))
	if tag == hwLanguageEnglish || strings.HasPrefix(tag, hwLanguageEnglish+"-") {
		return hwLanguageEnglish
	}
	return hwLanguageTurkish
}

func hwTopUpDescription(language string) string {
	if hwLanguageTag(language) == hwLanguageTurkish {
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
// provider itself reports a completed payment whose reference, id, amount and
// currency match our own order.
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
		if strings.TrimSpace(event.OrderId) != "" {
			// The event names a provider order we cannot match yet. Answering
			// 503 makes the provider deliver it again instead of losing it; the
			// reconciliation round is the backstop that settles or closes the
			// order either way. The answer names no order, so it leaks nothing.
			common.SysLog(common.LogText("revolut webhook could not match an order by reference or provider id; answered unavailable so the event is retried"))
			return HwWebhookUnavailable
		}
		return HwWebhookIgnored
	}
	return hwApplyWebhookEvent(ctx, order, event, now)
}

// hwApplyWebhookEvent acts on one verified provider event for an order of ours.
// Stripe events are mapped onto the same event names before they get here.
func hwApplyWebhookEvent(ctx context.Context, order *model.HwOrder, event hwRevolutWebhookEvent, now time.Time) HwWebhookOutcome {
	switch event.Event {
	case hwRevolutEventOrderCompleted:
		return hwCompleteRevolutOrder(ctx, order, event, now)
	case hwRevolutEventOrderCancelled:
		outcome, action := hwReconcileOrderBeforeClose(ctx, order, model.HwOrderStatusCancelled, now)
		if outcome == HwWebhookAccepted && action != hwReconcileNone {
			common.SysLog(common.LogText("revolut webhook %s order %s while handling a cancellation event", action, order.MerchantRef))
		}
		return outcome
	case hwRevolutEventPaymentDeclined, hwRevolutEventPaymentFailed:
		// A refused or failed payment is one attempt, not the end of the order:
		// the customer can still pay it with another card. Closing here would
		// release the first-purchase claim while the order stays payable, and a
		// later payment would spend the discount a second time. The order stays
		// open and is settled or closed by a later event or by the
		// reconciliation round.
		common.SysLog(common.LogText("revolut webhook keeps order %s open after a refused or failed payment", order.MerchantRef))
		return HwWebhookAccepted
	default:
		// Unknown event types are logged by type only and accepted. Refund and
		// dispute events land here on purpose: their exact names are unconfirmed
		// (they must be measured with sandbox keys), and taking credit back is
		// the explicit job of the admin refund route.
		common.SysLog(common.LogText("revolut webhook accepted an unhandled event of type %s", event.Event))
		return HwWebhookIgnored
	}
}

func hwCompleteRevolutOrder(ctx context.Context, order *model.HwOrder, event hwRevolutWebhookEvent, now time.Time) HwWebhookOutcome {
	if order.Status == model.HwOrderStatusPaid || order.Status == model.HwOrderStatusRefunded {
		return HwWebhookAccepted
	}
	providerOrderID := strings.TrimSpace(event.OrderId)
	if order.RevolutOrderId != nil && strings.TrimSpace(*order.RevolutOrderId) != "" {
		providerOrderID = strings.TrimSpace(*order.RevolutOrderId)
	}
	if providerOrderID == "" {
		common.SysLog(common.LogText("revolut webhook cannot confirm order %s: no provider order id is known; answered unavailable so the event is retried", order.MerchantRef))
		return HwWebhookUnavailable
	}
	config, err := hwProviderFor(providerOrderID)
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
		// Not finished yet: answer 503 so the provider delivers the event
		// again once the payment is, and let the reconciliation round close the
		// order if it never completes.
		common.SysLog(common.LogText("revolut webhook left order %s unsettled: the provider reports state %s; answered unavailable so the event is retried", order.MerchantRef, providerOrder.State))
		return HwWebhookUnavailable
	}
	return hwSettleConfirmedOrder(order, providerOrder, providerOrderID, now)
}

// hwSettleConfirmedOrder credits an order from a provider report that says
// completed, after the report has been matched to the order it belongs to. It is
// shared by the completed-event path and the reconciliation round.
func hwSettleConfirmedOrder(order *model.HwOrder, providerOrder hwRevolutOrderResponse, requestedOrderID string, now time.Time) HwWebhookOutcome {
	if order.Status == model.HwOrderStatusPaid || order.Status == model.HwOrderStatusRefunded {
		return HwWebhookAccepted
	}
	reportedReference := ""
	if providerOrder.MerchantOrderData != nil {
		reportedReference = strings.TrimSpace(providerOrder.MerchantOrderData.Reference)
	}
	if reportedReference != order.MerchantRef {
		common.SysError(common.LogText("revolut refused credit for order %s: the provider report carries a different order reference", order.MerchantRef))
		return HwWebhookIgnored
	}
	if strings.TrimSpace(providerOrder.Id) != requestedOrderID {
		common.SysError(common.LogText("revolut refused credit for order %s: the provider report answers a different order id", order.MerchantRef))
		return HwWebhookIgnored
	}
	if providerOrder.Amount != order.PayableTryMinor || !strings.EqualFold(strings.TrimSpace(providerOrder.Currency), order.Currency) {
		// The amount and the currency come from our own order; a provider report
		// that disagrees is a reason not to credit, never a reason to credit what
		// the provider says.
		common.SysError(common.LogText("revolut refused credit for order %s: the confirmed amount or currency differs from our order", order.MerchantRef))
		return HwWebhookIgnored
	}
	fingerprint := hwRevolutCardFingerprint(providerOrder)
	settlement, err := model.HwSettleOrderPayment(order.MerchantRef, requestedOrderID, fingerprint, now.Unix())
	if err != nil {
		common.SysError(common.LogText("revolut webhook could not settle order %s (class %s)", order.MerchantRef, hwPaymentErrorClass(err)))
		return HwWebhookUnavailable
	}
	if !settlement.Paid {
		return HwWebhookAccepted
	}
	if settlement.Granted {
		model.HwSyncCachesAfterGrant(settlement.UserId, settlement.GrantedQuota, model.HwSourcePurchase)
		if settlement.BonusQuota > 0 {
			model.HwSyncCachesAfterGrant(settlement.UserId, settlement.BonusQuota, model.HwSourceBonus)
		}
		model.HwSyncCachesAfterFreeForfeit(settlement.UserId, settlement.ForfeitedFreeQuota)
	}
	// Re-record the card signal of every paid discounted order, so the
	// shared-card checks of later orders keep working even when the payment
	// arrived after other events had already moved the order around.
	if fingerprint != "" {
		if err := model.UpsertHwUserSignals(settlement.UserId, model.HwUserSignals{CardFingerprint: fingerprint}); err != nil {
			common.SysError(common.LogText("could not record the paying card signal of order %s (class %s)", order.MerchantRef, hwPaymentErrorClass(err)))
		}
	}
	if settlement.AdminNote != "" {
		common.SysError(common.LogText("order %s was settled without its first-purchase discount; the reason is recorded on the order for the admin", order.MerchantRef))
	}
	hwGrantAffiliateAfterPayment(settlement, order.MerchantRef, now)
	common.SysLog(common.LogText("revolut webhook credited the top-up of order %s", order.MerchantRef))
	return HwWebhookAccepted
}

// hwGrantAffiliateAfterPayment grants the affiliate reward of a settled payment
// and puts a failed grant in the retry queue instead of losing it. Only classes
// and counts are logged.
func hwGrantAffiliateAfterPayment(settlement model.HwOrderSettlement, merchantRef string, now time.Time) {
	granted, reason, err := model.HwGrantAffiliateReward(settlement.UserId, settlement.GrantedQuota, merchantRef, now.Unix())
	switch {
	case err != nil:
		common.SysError(common.LogText("affiliate reward of order %s failed (class %s); queueing it for retry", merchantRef, hwPaymentErrorClass(err)))
		if _, enqueueErr := model.HwEnqueueAffiliateRetry(settlement.UserId, settlement.GrantedQuota, merchantRef, now.Unix()); enqueueErr != nil {
			common.SysError(common.LogText("could not queue the affiliate reward of order %s for retry (class %s)", merchantRef, hwPaymentErrorClass(enqueueErr)))
		}
	case !granted:
		common.SysLog(common.LogText("no affiliate reward for order %s: %s", merchantRef, reason))
	}
}

// What a reconciliation did to one order, for the round's log lines.
const (
	hwReconcileSettled = "settled"
	hwReconcileClosed  = "closed"
	hwReconcileNone    = "left"
)

// hwCloseRevolutOrder closes an order locally and releases its first-purchase
// claim. Only call it once the provider side of the order is settled.
func hwCloseRevolutOrder(order *model.HwOrder, status string, now time.Time) HwWebhookOutcome {
	changed, err := model.HwMarkOrderNotPaid(order.MerchantRef, status, now.Unix())
	if err != nil {
		common.SysError(common.LogText("could not close order %s as %s (class %s)", order.MerchantRef, status, hwPaymentErrorClass(err)))
		return HwWebhookUnavailable
	}
	if changed {
		common.SysLog(common.LogText("order %s closed as %s", order.MerchantRef, status))
	}
	return HwWebhookAccepted
}

// hwReconcileOrderBeforeClose is the gate every local close passes through. The
// order is read back from the provider: a completed payment is settled as a
// payment, an order already cancelled there is closed, and anything else is
// cancelled at the provider first. Only a close the provider confirmed (or an
// order the provider never knew) releases the first-purchase claim, so a
// discount that was given back can never be spent on an order that is still
// payable. The returned action names what happened, for the round's log.
func hwReconcileOrderBeforeClose(ctx context.Context, order *model.HwOrder, closeStatus string, now time.Time) (HwWebhookOutcome, string) {
	if order.Status == model.HwOrderStatusPaid || order.Status == model.HwOrderStatusRefunded {
		return HwWebhookAccepted, hwReconcileNone
	}
	providerOrderID := ""
	if order.RevolutOrderId != nil {
		providerOrderID = strings.TrimSpace(*order.RevolutOrderId)
	}
	if providerOrderID == "" {
		// No provider order id was ever recorded, so there is nothing to
		// reconcile with. A late payment event for such an order is still
		// settled safely: HwSettleOrderPayment credits it without the discount
		// once the claim is gone.
		return hwCloseRevolutOrder(order, closeStatus, now), hwReconcileClosed
	}
	config, err := hwProviderFor(providerOrderID)
	if err != nil {
		common.SysError(common.LogText("could not reach the provider of order %s (class %s); the order stays open and its claim stays held", order.MerchantRef, hwPaymentErrorClass(err)))
		return HwWebhookUnavailable, hwReconcileNone
	}
	providerOrder, err := config.GetOrder(ctx, providerOrderID)
	if err != nil {
		common.SysError(common.LogText("could not read order %s back from the provider (class %s); the order stays open and its claim stays held", order.MerchantRef, hwPaymentErrorClass(err)))
		return HwWebhookUnavailable, hwReconcileNone
	}
	if providerOrder.State == hwRevolutStateCompleted {
		outcome := hwSettleConfirmedOrder(order, providerOrder, providerOrderID, now)
		if outcome == HwWebhookIgnored {
			// The provider says completed, but the report does not match our
			// order: money may have moved wrongly. Flag the order for an admin
			// instead of crediting it or releasing its claim, and leave it out
			// of later rounds until the note is cleared.
			if err := model.HwNoteOrder(order.MerchantRef, "provider reports this order completed with a different reference, id, amount or currency; not credited, kept open for admin review"); err != nil {
				common.SysError(common.LogText("could not flag order %s for admin review (class %s)", order.MerchantRef, hwPaymentErrorClass(err)))
			}
		}
		if outcome == HwWebhookAccepted {
			return outcome, hwReconcileSettled
		}
		return outcome, hwReconcileNone
	}
	if providerOrder.State != hwRevolutStateCancelled {
		if err := config.CancelOrder(ctx, providerOrderID); err != nil {
			common.SysError(common.LogText("could not cancel order %s at the provider (class %s); the order stays open and its claim stays held", order.MerchantRef, hwPaymentErrorClass(err)))
			return HwWebhookUnavailable, hwReconcileNone
		}
	}
	return hwCloseRevolutOrder(order, closeStatus, now), hwReconcileClosed
}

// HwExpiryRun counts what one reconciliation round did.
type HwExpiryRun struct {
	// Settled is how many orders the round settled as paid.
	Settled int
	// Closed is how many orders the round closed after the provider confirmed.
	Closed int
	// Retried is how many orders stayed open because the provider could not be
	// reached or refused the cancel; the next round tries them again.
	Retried int
}

const hwOrderExpiryJobInterval = 10 * time.Minute

// HwReconcileExpiredOrders works off the unpaid orders past the pending TTL.
// Every one of them is reconciled with the provider first: a paid order is
// settled as paid, everything else is cancelled at the provider before the
// local close releases the first-purchase claim.
func HwReconcileExpiredOrders(ctx context.Context, now time.Time) (HwExpiryRun, error) {
	var run HwExpiryRun
	orders, err := model.HwOpenOrdersPastTTL(now.Unix())
	if err != nil {
		return run, err
	}
	if len(orders) == 0 {
		return run, nil
	}
	for index := range orders {
		order := orders[index]
		outcome, action := hwReconcileOrderBeforeClose(ctx, &order, model.HwOrderStatusExpired, now)
		switch {
		case outcome == HwWebhookAccepted && action == hwReconcileSettled:
			run.Settled++
		case outcome == HwWebhookAccepted && action == hwReconcileClosed:
			run.Closed++
		default:
			run.Retried++
		}
	}
	return run, nil
}

var hwOrderExpiryOnce sync.Once

// StartHwOrderExpiryJob starts the reconciliation round that expires unpaid
// orders and works the affiliate retry queue. It runs once per process and only
// on the master node; every step is idempotent, so several masters are
// harmless.
func StartHwOrderExpiryJob() {
	hwOrderExpiryOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		go func() {
			common.SysLog(common.LogText("top-up order reconciliation job started: interval=%s", hwOrderExpiryJobInterval))
			runHwOrderReconcileOnceSafely()
			ticker := time.NewTicker(hwOrderExpiryJobInterval)
			defer ticker.Stop()
			for range ticker.C {
				runHwOrderReconcileOnceSafely()
			}
		}()
	})
}

func runHwOrderReconcileOnceSafely() {
	defer func() {
		if r := recover(); r != nil {
			common.SysError(common.LogText("top-up order reconciliation job panicked: %v", r))
		}
	}()
	run, err := HwReconcileExpiredOrders(context.Background(), time.Now())
	if err != nil {
		common.SysError(common.LogText("top-up order reconciliation round failed (class %s)", hwPaymentErrorClass(err)))
	}
	if run.Settled+run.Closed+run.Retried > 0 {
		common.SysLog(common.LogText("top-up order reconciliation round: %d settled, %d closed, %d left for the next round", run.Settled, run.Closed, run.Retried))
	}
	granted, dropped, err := model.HwRetryPendingAffiliateRewards(common.GetTimestamp())
	if err != nil {
		common.SysError(common.LogText("affiliate reward retry round failed (class %s)", hwPaymentErrorClass(err)))
	}
	if granted > 0 || dropped > 0 {
		common.SysLog(common.LogText("affiliate reward retry round: %d granted, %d dropped after too many attempts", granted, dropped))
	}
}

// Errors of the admin refund route, answered with their own status by the
// controller.
var (
	ErrHwOrderNotFound      = errors.New("top-up order not found")
	ErrHwOrderNotRefundable = errors.New("top-up order is not in a refundable state")
)

// HwAdminRefundResult reports what the admin refund route did to an order.
type HwAdminRefundResult struct {
	// Found is false when no order carries the reference.
	Found bool
	// Refunded is true when this call moved the order to refunded.
	Refunded bool
	// GrantedQuota is what the purchase originally issued.
	GrantedQuota int
	// ReturnedQuota is what was subtracted from the balance.
	ReturnedQuota int
	// SpentQuota is what the customer had already spent; it stays recorded as
	// owed and is not recovered from other packages.
	SpentQuota int
	// BonusReturnedQuota is the unspent bonus of the order that was taken back
	// together with the purchase; zero when the order earned no bonus.
	BonusReturnedQuota int
	// Note is the admin note stored on the order.
	Note string
	// Currency, FeeMinor and RefundedMinor describe the card part of a Stripe
	// order in minor units of the order currency: the costs HisarWeb paid for the
	// payment that were deducted, and what went back to the card. Orders of other
	// providers are refunded outside this system and report zero.
	Currency      string
	FeeMinor      int64
	RefundedMinor int64
}

// HwAdminRefundOrder takes back the credit of one paid order: the purchase
// package is retired with its remaining value, the balance is lowered by that
// remainder (never below zero), the affiliate reward of the purchase is revoked,
// and the order moves to refunded with an admin note naming what stayed owed.
// For a card order the costs HisarWeb paid to Stripe (processing and currency
// conversion) are deducted from the money that goes back; the fee is read first,
// and when it cannot be read nothing changes and the refund can be repeated.
// Calling it again for the same order changes nothing.
func HwAdminRefundOrder(merchantRef string, now time.Time) (HwAdminRefundResult, error) {
	merchantRef = strings.TrimSpace(merchantRef)
	if merchantRef == "" {
		return HwAdminRefundResult{}, model.ErrHwInvalidOrder
	}
	var result HwAdminRefundResult
	order, found, err := model.HwFindOrderForWebhook(merchantRef, "")
	if err != nil {
		return result, err
	}
	if !found {
		return result, ErrHwOrderNotFound
	}
	result.Found = true
	alreadyRefunded := order.Status == model.HwOrderStatusRefunded
	if !alreadyRefunded && order.Status != model.HwOrderStatusPaid {
		return result, ErrHwOrderNotRefundable
	}
	// The card part is planned before anything changes, so an unreadable fee
	// leaves the order and the balance untouched and never turns into a full refund.
	plan, hasPlan, err := hwPlanProviderRefund(order)
	if err != nil {
		return result, err
	}
	result.Currency = order.Currency
	if hasPlan {
		result.FeeMinor = plan.FeeMinor
		result.RefundedMinor = plan.RefundMinor
		if plan.AlreadyRefundedMinor > 0 {
			result.RefundedMinor = plan.AlreadyRefundedMinor
		}
	}
	revocation, err := model.HwRevokeOrderCredit(merchantRef, now.Unix())
	if err != nil {
		return result, err
	}
	result.GrantedQuota = revocation.GrantedQuota
	result.ReturnedQuota = revocation.RemainingQuota
	result.SpentQuota = max(revocation.GrantedQuota-revocation.RemainingQuota, 0)
	result.BonusReturnedQuota = revocation.BonusReturnedQuota
	if alreadyRefunded {
		result.Note = order.AdminNote
		// A card refund that failed on the first call is retried here; Stripe's
		// idempotency key makes a repeat of one that went through harmless.
		return result, hwRefundAtProvider(order, plan, hasPlan)
	}
	if err := model.HwRevokeAffiliateReward(merchantRef, now.Unix()); err != nil {
		common.SysError(common.LogText("could not revoke the affiliate reward of order %s (class %s)", merchantRef, hwPaymentErrorClass(err)))
		return result, err
	}
	result.Note = fmt.Sprintf("refund: granted %d quota, returned %d quota, %d quota already spent stays owed",
		result.GrantedQuota, result.ReturnedQuota, result.SpentQuota)
	if result.BonusReturnedQuota > 0 {
		result.Note += fmt.Sprintf("; bonus: %d quota taken back with it", result.BonusReturnedQuota)
	}
	if hasPlan {
		result.Note += fmt.Sprintf("; card: paid %d, provider costs deducted %d, refunded %d (%s, minor units)",
			plan.PaidMinor, plan.FeeMinor, result.RefundedMinor, plan.Currency)
		if plan.RefundMinor <= 0 && plan.AlreadyRefundedMinor <= 0 {
			result.Note += "; no card refund: the costs are as high as the payment"
		}
	}
	changed, err := model.HwMarkOrderRefunded(merchantRef, result.Note, now.Unix())
	if err != nil {
		return result, err
	}
	result.Refunded = changed
	common.SysLog(common.LogText("admin refund took back order %s: %d of %d quota returned", merchantRef, result.ReturnedQuota, result.GrantedQuota))
	return result, hwRefundAtProvider(order, plan, hasPlan)
}

// ErrHwProviderRefundFailed: the credit was taken back but the money could not
// be returned to the card yet; repeating the refund retries only that part.
var ErrHwProviderRefundFailed = errors.New("credit was taken back but the card refund failed")

// hwStripeOrderSession is the Checkout Session id of a Stripe order, or false
// for orders of other providers.
func hwStripeOrderSession(order *model.HwOrder) (string, bool) {
	if order.RevolutOrderId == nil {
		return "", false
	}
	sessionID := strings.TrimSpace(*order.RevolutOrderId)
	return sessionID, strings.HasPrefix(sessionID, hwStripeSessionPrefix)
}

// hwPlanProviderRefund works out the card refund of a Stripe order. Orders of
// other providers are refunded outside this system, as before. Any failure
// reads as ErrHwRefundFeeUnavailable: nothing was changed yet and the refund can
// be repeated.
func hwPlanProviderRefund(order *model.HwOrder) (HwStripeRefundPlan, bool, error) {
	sessionID, ok := hwStripeOrderSession(order)
	if !ok {
		return HwStripeRefundPlan{}, false, nil
	}
	config, err := HwStripeConfigFromEnv()
	var plan HwStripeRefundPlan
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), hwRevolutRequestTimeout)
		defer cancel()
		plan, err = config.PlanRefund(ctx, sessionID, order)
	}
	if err != nil {
		common.SysError(common.LogText("could not read the provider cost of order %s, refund not started (class %s)", order.MerchantRef, hwPaymentErrorClass(err)))
		return HwStripeRefundPlan{}, false, ErrHwRefundFeeUnavailable
	}
	return plan, true, nil
}

// hwRefundAtProvider returns the planned money of a Stripe order to the card.
// The credit is taken back first, so a failure here never leaves the customer
// with both.
func hwRefundAtProvider(order *model.HwOrder, plan HwStripeRefundPlan, hasPlan bool) error {
	if !hasPlan {
		return nil
	}
	config, err := HwStripeConfigFromEnv()
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), hwRevolutRequestTimeout)
		defer cancel()
		err = config.Refund(ctx, order.MerchantRef, plan)
	}
	if err != nil {
		common.SysError(common.LogText("card refund of order %s failed (class %s)", order.MerchantRef, hwPaymentErrorClass(err)))
		return ErrHwProviderRefundFailed
	}
	common.SysLog(common.LogText("card refund of order %s handled with the provider", order.MerchantRef))
	return nil
}

// hwPaymentErrorClass reduces an error to a log-safe class. Log lines carry the
// class and never the error text: a provider body or a driver error can quote
// request data.
func hwPaymentErrorClass(err error) string {
	var apiErr *HwRevolutAPIError
	var stripeErr *HwStripeAPIError
	switch {
	case errors.Is(err, ErrHwRevolutNotConfigured), errors.Is(err, ErrHwStripeNotConfigured):
		return "not_configured"
	case errors.Is(err, ErrHwRevolutUnusableResponse), errors.Is(err, ErrHwStripeUnusableResponse):
		return "unusable_response"
	case errors.As(err, &apiErr) && apiErr.Transport, errors.As(err, &stripeErr) && stripeErr.Transport:
		return "transport"
	case errors.As(err, &apiErr), errors.As(err, &stripeErr):
		return "provider_status"
	case errors.Is(err, model.ErrHwInvalidOrder), errors.Is(err, model.ErrHwInvalidCreditGrant):
		return "invalid_request"
	}
	return "internal"
}
