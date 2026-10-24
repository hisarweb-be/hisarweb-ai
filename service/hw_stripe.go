package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// Stripe Checkout is the card route next to Revolut. It reuses the order flow
// of hw_order.go: a Checkout Session is mapped onto hwRevolutOrderResponse, so
// the amount, currency and reference checks of hwSettleConfirmedOrder guard both
// providers. The Stripe session id (cs_...) is stored in the existing provider
// order column, and the prefix tells the two providers apart; no schema change.
//
// The Stripe account is shared with other HisarWeb products, so the webhook
// endpoint also receives sessions that are not ours. Those are acknowledged and
// ignored (only references starting with "hw-" are ours).
const (
	HwStripeSecretKeyEnv     = "HW_STRIPE_SECRET_KEY"
	HwStripeWebhookSecretEnv = "HW_STRIPE_WEBHOOK_SECRET"
	HwStripeAPIBaseEnv       = "HW_STRIPE_API_BASE"

	hwStripeDefaultAPIBase   = "https://api.stripe.com"
	hwStripeSessionPrefix    = "cs_"
	hwStripeWebhookTolerance = 5 * time.Minute
)

var (
	ErrHwStripeNotConfigured            = errors.New("stripe is not configured")
	ErrHwStripeInsecureBaseURL          = errors.New("stripe api base url must use https")
	ErrHwStripeWebhookSecretMissing     = errors.New("stripe webhook secret is not configured")
	ErrHwStripeWebhookSignatureRejected = errors.New("stripe webhook signature or timestamp is invalid")
	ErrHwStripeUnusableResponse         = errors.New("stripe returned an unusable response")
)

// HwStripeAPIError reports a failed Stripe call by status code only; the body can
// quote request data and is never kept.
type HwStripeAPIError struct {
	StatusCode int
	Transport  bool
}

func (e *HwStripeAPIError) Error() string {
	if e.Transport {
		return "stripe api request failed"
	}
	return "stripe api returned status " + strconv.Itoa(e.StatusCode)
}

// HwStripeConfig is resolved from the environment per call, so a rotated key is
// picked up without a restart.
type HwStripeConfig struct {
	BaseURL   string
	SecretKey string
}

// HwStripeConfigFromEnv resolves the Stripe configuration. The base URL must be
// https; a loopback http host is allowed only behind the same test-only hook as
// the Revolut client.
func HwStripeConfigFromEnv() (HwStripeConfig, error) {
	secretKey := strings.TrimSpace(os.Getenv(HwStripeSecretKeyEnv))
	if secretKey == "" {
		return HwStripeConfig{}, ErrHwStripeNotConfigured
	}
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv(HwStripeAPIBaseEnv)), "/")
	if baseURL == "" {
		baseURL = hwStripeDefaultAPIBase
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return HwStripeConfig{}, ErrHwStripeNotConfigured
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && hwRevolutAllowLoopbackHTTP && hwRevolutHostIsLoopback(parsed.Hostname())) {
		return HwStripeConfig{}, ErrHwStripeInsecureBaseURL
	}
	return HwStripeConfig{BaseURL: baseURL, SecretKey: secretKey}, nil
}

// HwStripePaymentAvailable reports whether Stripe can take a payment now: the key
// resolves and the webhook secret is set. Without the secret a paid order would
// never be confirmed.
func HwStripePaymentAvailable() bool {
	if _, err := HwStripeConfigFromEnv(); err != nil {
		return false
	}
	return strings.TrimSpace(os.Getenv(HwStripeWebhookSecretEnv)) != ""
}

// HwPaymentAvailable is what the top-up page asks: any provider is ready.
func HwPaymentAvailable() bool {
	return HwStripePaymentAvailable() || HwRevolutPaymentAvailable()
}

// hwProvider is what the reconcile and webhook paths need from a payment
// provider: read an order back, and close one that is not paid.
type hwProvider interface {
	GetOrder(ctx context.Context, orderID string) (hwRevolutOrderResponse, error)
	CancelOrder(ctx context.Context, orderID string) error
}

// hwProviderFor picks the provider that owns a stored provider order id.
func hwProviderFor(providerOrderID string) (hwProvider, error) {
	if strings.HasPrefix(providerOrderID, hwStripeSessionPrefix) {
		return HwStripeConfigFromEnv()
	}
	return HwRevolutConfigFromEnv()
}

// The JSON names of the Stripe Checkout Session live here and nowhere else.
type hwStripeSession struct {
	Id                string            `json:"id"`
	URL               string            `json:"url"`
	Status            string            `json:"status"`
	PaymentStatus     string            `json:"payment_status"`
	AmountTotal       int64             `json:"amount_total"`
	Currency          string            `json:"currency"`
	ClientReferenceId string            `json:"client_reference_id"`
	PaymentIntent     *hwStripeIntent   `json:"payment_intent"`
	Metadata          map[string]string `json:"metadata"`
}

// UnmarshalJSON accepts both shapes Stripe sends: a bare id (not expanded, and
// in webhook events) and the expanded object.
func (intent *hwStripeIntent) UnmarshalJSON(data []byte) error {
	var id string
	if common.Unmarshal(data, &id) == nil {
		intent.Id = id
		return nil
	}
	type plain hwStripeIntent
	return common.Unmarshal(data, (*plain)(intent))
}

type hwStripeIntent struct {
	Id           string          `json:"id"`
	LatestCharge *hwStripeCharge `json:"latest_charge"`
}

// hwStripeCharge is the part of a charge the integration reads: the paying
// card, what was paid and refunded, and the balance transaction that carries
// the fee Stripe took.
type hwStripeCharge struct {
	Id                   string `json:"id"`
	Amount               int64  `json:"amount"`
	Currency             string `json:"currency"`
	AmountRefunded       int64  `json:"amount_refunded"`
	PaymentMethodDetails struct {
		Card struct {
			Fingerprint string `json:"fingerprint"`
		} `json:"card"`
	} `json:"payment_method_details"`
	// BalanceTransaction is nil while Stripe has not booked the charge yet.
	BalanceTransaction *hwStripeBalanceTransaction `json:"balance_transaction"`
}

// hwStripeBalanceTransaction is what the charge cost. Amount and Fee are in the
// currency Stripe settles in (Currency), which can differ from the currency the
// customer paid in; Fee holds processing and conversion costs together.
type hwStripeBalanceTransaction struct {
	Id       string `json:"id"`
	Amount   int64  `json:"amount"`
	Fee      int64  `json:"fee"`
	Currency string `json:"currency"`
}

// UnmarshalJSON accepts a bare id (not expanded) next to the object; the bare
// form leaves Amount and Currency empty, which the refund plan refuses.
func (bt *hwStripeBalanceTransaction) UnmarshalJSON(data []byte) error {
	var id string
	if common.Unmarshal(data, &id) == nil {
		bt.Id = id
		return nil
	}
	type plain hwStripeBalanceTransaction
	return common.Unmarshal(data, (*plain)(bt))
}

// toOrder maps a session onto the provider-neutral order the settle step checks.
func (session hwStripeSession) toOrder() hwRevolutOrderResponse {
	state := "open"
	switch {
	case session.PaymentStatus == "paid":
		state = hwRevolutStateCompleted
	case session.Status == "expired":
		state = hwRevolutStateCancelled
	}
	order := hwRevolutOrderResponse{
		Id:                session.Id,
		State:             state,
		CheckoutURL:       session.URL,
		Amount:            session.AmountTotal,
		Currency:          session.Currency,
		MerchantOrderData: &hwRevolutMerchantOrderData{Reference: session.ClientReferenceId},
	}
	if intent := session.PaymentIntent; intent != nil && intent.LatestCharge != nil {
		fingerprint := intent.LatestCharge.PaymentMethodDetails.Card.Fingerprint
		order.Payments = []hwRevolutPayment{{PaymentMethod: hwRevolutPaymentMethod{Fingerprint: fingerprint}}}
	}
	return order
}

// CreateCheckout opens a Checkout Session for one order. The amount is a
// price_data in TRY built from our own order, never a fixed Stripe price. The
// idempotency key makes a retried request return the same session.
func (config HwStripeConfig) CreateCheckout(ctx context.Context, order *model.HwOrder, email, language, returnURL string) (hwRevolutOrderResponse, error) {
	form := url.Values{}
	form.Set("mode", "payment")
	form.Set("payment_method_types[0]", "card")
	// Adaptive Pricing would let the customer pay 2 to 4 % extra in a local
	// currency; HisarWeb carries all conversion costs, so it is off per session
	// whatever the Dashboard default is.
	form.Set("adaptive_pricing[enabled]", "false")
	form.Set("client_reference_id", order.MerchantRef)
	form.Set("metadata[merchant_ref]", order.MerchantRef)
	form.Set("payment_intent_data[metadata][merchant_ref]", order.MerchantRef)
	form.Set("line_items[0][quantity]", "1")
	form.Set("line_items[0][price_data][currency]", strings.ToLower(order.Currency))
	form.Set("line_items[0][price_data][unit_amount]", strconv.FormatInt(order.PayableTryMinor, 10))
	form.Set("line_items[0][price_data][product_data][name]", hwTopUpDescription(language))
	form.Set("success_url", returnURL)
	form.Set("cancel_url", returnURL)
	if hwLanguageTag(language) == hwLanguageEnglish {
		form.Set("locale", "en")
	} else {
		form.Set("locale", "tr")
	}
	if email = strings.TrimSpace(email); email != "" {
		form.Set("customer_email", email)
	}
	session, err := config.do(ctx, http.MethodPost, "/v1/checkout/sessions", form, "hw-checkout-"+order.MerchantRef)
	if err != nil {
		return hwRevolutOrderResponse{}, err
	}
	return session.toOrder(), nil
}

// GetOrder reads a session back, with the paying card's fingerprint expanded.
func (config HwStripeConfig) GetOrder(ctx context.Context, sessionID string) (hwRevolutOrderResponse, error) {
	session, err := config.getSession(ctx, sessionID)
	if err != nil {
		return hwRevolutOrderResponse{}, err
	}
	return session.toOrder(), nil
}

func (config HwStripeConfig) getSession(ctx context.Context, sessionID string) (hwStripeSession, error) {
	return config.getSessionExpanded(ctx, sessionID, "payment_intent.latest_charge")
}

func (config HwStripeConfig) getSessionExpanded(ctx context.Context, sessionID, expand string) (hwStripeSession, error) {
	sessionID = strings.TrimSpace(sessionID)
	if !strings.HasPrefix(sessionID, hwStripeSessionPrefix) {
		return hwStripeSession{}, ErrHwStripeUnusableResponse
	}
	query := url.Values{"expand[]": {expand}}
	return config.do(ctx, http.MethodGet, "/v1/checkout/sessions/"+url.PathEscape(sessionID)+"?"+query.Encode(), nil, "")
}

// CancelOrder expires an open session so it can no longer be paid.
func (config HwStripeConfig) CancelOrder(ctx context.Context, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if !strings.HasPrefix(sessionID, hwStripeSessionPrefix) {
		return ErrHwStripeUnusableResponse
	}
	_, err := config.do(ctx, http.MethodPost, "/v1/checkout/sessions/"+url.PathEscape(sessionID)+"/expire", url.Values{}, "")
	return err
}

// ErrHwRefundFeeUnavailable: the cost Stripe took for an order cannot be read
// yet (its balance transaction is not booked, or the lookup failed). Nothing was
// changed; the refund can be repeated later. It never falls back to a refund
// without the deduction.
var ErrHwRefundFeeUnavailable = errors.New("the provider cost of the payment is not available yet")

// HwStripeRefundPlan is what a refund of one paid order comes to, all in minor
// units of the order currency: the costs HisarWeb itself paid for the payment
// (processing and currency conversion) are deducted from what goes back.
type HwStripeRefundPlan struct {
	PaymentIntentID string
	Currency        string
	PaidMinor       int64
	// FeeMinor is Stripe's total cost converted to the order currency, rounded up.
	FeeMinor int64
	// RefundMinor is paid minus fee, never below zero.
	RefundMinor int64
	// AlreadyRefundedMinor is what Stripe already returned for the charge.
	AlreadyRefundedMinor int64
}

// PlanRefund reads the session with the fee expanded and works out the refund.
// It only reads, so it runs before any state of the order changes.
func (config HwStripeConfig) PlanRefund(ctx context.Context, sessionID string, order *model.HwOrder) (HwStripeRefundPlan, error) {
	session, err := config.getSessionExpanded(ctx, sessionID, "payment_intent.latest_charge.balance_transaction")
	if err != nil {
		return HwStripeRefundPlan{}, err
	}
	intent := session.PaymentIntent
	if intent == nil || intent.Id == "" || intent.LatestCharge == nil {
		return HwStripeRefundPlan{}, ErrHwStripeUnusableResponse
	}
	charge := intent.LatestCharge
	if charge.Amount <= 0 || charge.Amount != order.PayableTryMinor || !strings.EqualFold(charge.Currency, order.Currency) || charge.AmountRefunded < 0 {
		return HwStripeRefundPlan{}, ErrHwStripeUnusableResponse
	}
	bt := charge.BalanceTransaction
	if bt == nil {
		return HwStripeRefundPlan{}, ErrHwRefundFeeUnavailable
	}
	if bt.Amount <= 0 || bt.Fee < 0 || bt.Currency == "" {
		return HwStripeRefundPlan{}, ErrHwStripeUnusableResponse
	}
	// The fee is in the settlement currency; charge.amount / bt.amount is the
	// rate Stripe itself applied, so the fee in the order currency is
	// fee * charge.amount / bt.amount, rounded up to the smallest unit. Big
	// integers: the product of two provider amounts must not overflow. A cost
	// that is negative or higher than the payment itself is not believable and
	// plans nothing.
	product := new(big.Int).Mul(big.NewInt(bt.Fee), big.NewInt(charge.Amount))
	product.Add(product, big.NewInt(bt.Amount-1))
	feeBig := product.Div(product, big.NewInt(bt.Amount))
	if feeBig.Sign() < 0 || !feeBig.IsInt64() || feeBig.Int64() > charge.Amount {
		return HwStripeRefundPlan{}, ErrHwStripeUnusableResponse
	}
	fee := feeBig.Int64()
	return HwStripeRefundPlan{
		PaymentIntentID:      intent.Id,
		Currency:             strings.ToUpper(charge.Currency),
		PaidMinor:            charge.Amount,
		FeeMinor:             fee,
		RefundMinor:          charge.Amount - fee,
		AlreadyRefundedMinor: charge.AmountRefunded,
	}, nil
}

// hwStripeRefundKeyPrefix versions the refund idempotency key: the amount of the
// refund changed meaning (paid minus costs), and Stripe rejects a key reused
// with other parameters.
const hwStripeRefundKeyPrefix = "hw-refund-v2-"

// Refund returns the planned amount to the card. The idempotency key is derived
// from the order, so repeating the admin refund never refunds twice; a charge
// that already shows a refund is left alone (Stripe forgets keys after a day).
// A plan with nothing to return sends nothing.
func (config HwStripeConfig) Refund(ctx context.Context, merchantRef string, plan HwStripeRefundPlan) error {
	if plan.RefundMinor <= 0 || plan.AlreadyRefundedMinor > 0 {
		return nil
	}
	form := url.Values{
		"payment_intent":         {plan.PaymentIntentID},
		"amount":                 {strconv.FormatInt(plan.RefundMinor, 10)},
		"metadata[merchant_ref]": {merchantRef},
	}
	_, err := config.do(ctx, http.MethodPost, "/v1/refunds", form, hwStripeRefundKeyPrefix+merchantRef)
	return err
}

// do sends one Stripe request. Refund responses decode into the session type
// without harm: the fields they do not carry stay empty and are not read.
func (config HwStripeConfig) do(ctx context.Context, method, path string, form url.Values, idempotencyKey string) (hwStripeSession, error) {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	request, err := http.NewRequestWithContext(ctx, method, config.BaseURL+path, body)
	if err != nil {
		return hwStripeSession{}, ErrHwStripeNotConfigured
	}
	request.Header.Set("Authorization", "Bearer "+config.SecretKey)
	if form != nil {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	response, err := hwRevolutHTTPClient().Do(request)
	if err != nil {
		return hwStripeSession{}, &HwStripeAPIError{Transport: true}
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, hwRevolutMaxResponseBytes))
	if err != nil {
		return hwStripeSession{}, &HwStripeAPIError{Transport: true}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode > 299 {
		return hwStripeSession{}, &HwStripeAPIError{StatusCode: response.StatusCode}
	}
	var session hwStripeSession
	if err := common.Unmarshal(payload, &session); err != nil {
		return hwStripeSession{}, ErrHwStripeUnusableResponse
	}
	return session, nil
}

// hwCheckoutReturnURL is where Stripe sends the customer after paying or
// leaving: the configured return URL, else the top-up page of this site.
func hwCheckoutReturnURL() string {
	if returnURL := hwOption(model.HwCheckoutReturnURLOption); returnURL != "" {
		return returnURL
	}
	return strings.TrimRight(system_setting.ServerAddress, "/") + "/satin-al"
}

// HwVerifyStripeWebhook checks the Stripe-Signature header: "t=<unix>,v1=<hex>".
// The signed text is "<t>.<raw body>" under HMAC-SHA256 with the endpoint secret;
// any one v1 value may match (several appear while a secret is rotated), the
// comparison is constant time, and a timestamp more than five minutes away is
// refused so a captured request cannot be replayed later.
func HwVerifyStripeWebhook(rawBody []byte, signatureHeader, secret string, now time.Time) error {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ErrHwStripeWebhookSecretMissing
	}
	var timestamp string
	var signatures []string
	for part := range strings.SplitSeq(signatureHeader, ",") {
		key, value, _ := strings.Cut(strings.TrimSpace(part), "=")
		switch key {
		case "t":
			timestamp = value
		case "v1":
			signatures = append(signatures, value)
		}
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Unix(seconds, 0).Sub(now).Abs() > hwStripeWebhookTolerance {
		return ErrHwStripeWebhookSignatureRejected
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(rawBody)
	expected := hex.EncodeToString(mac.Sum(nil))
	matched := false
	for _, candidate := range signatures {
		if hmac.Equal([]byte(candidate), []byte(expected)) {
			matched = true
		}
	}
	if !matched {
		return ErrHwStripeWebhookSignatureRejected
	}
	return nil
}

type hwStripeWebhookEvent struct {
	Type string `json:"type"`
	Data struct {
		Object hwStripeSession `json:"object"`
	} `json:"data"`
}

// HwHandleStripeWebhook verifies and processes one Stripe event. Credit is only
// written when Stripe itself reports the session paid with the reference, id,
// amount and currency of our own order (see hwSettleConfirmedOrder). The body,
// the header and the secrets are never logged.
func HwHandleStripeWebhook(ctx context.Context, rawBody []byte, signatureHeader string, now time.Time) HwWebhookOutcome {
	if err := HwVerifyStripeWebhook(rawBody, signatureHeader, os.Getenv(HwStripeWebhookSecretEnv), now); err != nil {
		if errors.Is(err, ErrHwStripeWebhookSecretMissing) {
			common.SysError(common.LogText("stripe webhook refused: %s is not configured", HwStripeWebhookSecretEnv))
			return HwWebhookUnavailable
		}
		common.SysLog(common.LogText("stripe webhook rejected: %s", err.Error()))
		return HwWebhookRejected
	}
	var event hwStripeWebhookEvent
	if err := common.Unmarshal(rawBody, &event); err != nil {
		common.SysLog(common.LogText("stripe webhook rejected: the signed body is not an event"))
		return HwWebhookBadRequest
	}
	session := event.Data.Object
	if !strings.HasPrefix(session.ClientReferenceId, "hw-") {
		return HwWebhookIgnored
	}
	order, found, err := model.HwFindOrderForWebhook(session.ClientReferenceId, session.Id)
	if err != nil {
		common.SysError(common.LogText("stripe webhook could not look up its order (class %s)", hwPaymentErrorClass(err)))
		return HwWebhookUnavailable
	}
	if !found {
		// Unlike Revolut no retry: the account is shared, and a session of
		// another environment must not be redelivered forever. The
		// reconciliation round settles any order of ours that was missed.
		return HwWebhookIgnored
	}
	// A foreign session (a payment link of another product on the shared
	// account) can carry our reference: it must not touch our order.
	if order.RevolutOrderId != nil && *order.RevolutOrderId != "" && *order.RevolutOrderId != session.Id {
		return HwWebhookIgnored
	}
	mapped := hwRevolutWebhookEvent{Event: event.Type, OrderId: session.Id, MerchantOrderExtRef: session.ClientReferenceId}
	switch event.Type {
	case "checkout.session.completed", "checkout.session.async_payment_succeeded":
		mapped.Event = hwRevolutEventOrderCompleted
	case "checkout.session.expired":
		mapped.Event = hwRevolutEventOrderCancelled
	}
	return hwApplyWebhookEvent(ctx, order, mapped, now)
}
