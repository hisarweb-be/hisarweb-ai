package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// Environment names of the Revolut Merchant integration. Secrets are read from
// the environment only; they never appear in code, tests, logs or responses.
const (
	HwRevolutSecretKeyEnv     = "REVOLUT_SECRET_KEY"
	HwRevolutWebhookSecretEnv = "REVOLUT_WEBHOOK_SECRET"
	HwRevolutAPIBaseEnv       = "REVOLUT_API_BASE"
	HwRevolutAPIVersionEnv    = "REVOLUT_API_VERSION"

	hwRevolutDefaultAPIBase     = "https://merchant.revolut.com"
	hwRevolutDefaultAPIVersion  = "2024-09-01"
	hwRevolutMaxResponseBytes   = 1 << 20
	hwRevolutWebhookMaxClockGap = 5 * time.Minute
	hwRevolutRequestTimeout    = 30 * time.Second
)

// Errors of the Revolut integration. Only these are returned to callers; an
// error text never carries a secret, a body or a header.
var (
	ErrHwRevolutNotConfigured            = errors.New("revolut merchant api is not configured")
	ErrHwRevolutInsecureBaseURL          = errors.New("revolut api base url must use https")
	ErrHwRevolutWebhookSecretMissing     = errors.New("revolut webhook secret is not configured")
	ErrHwRevolutWebhookTimestampRejected = errors.New("revolut webhook timestamp is invalid or outside the accepted window")
	ErrHwRevolutWebhookSignatureRejected = errors.New("revolut webhook signature does not match")
	ErrHwRevolutUnusableResponse         = errors.New("revolut merchant api returned an unusable response")
)

// HwRevolutAPIError reports a failed provider call by status code only. The
// response body can quote request data, so it is never kept or logged.
type HwRevolutAPIError struct {
	StatusCode int
	Transport  bool
}

func (e *HwRevolutAPIError) Error() string {
	if e.Transport {
		return "revolut merchant api request failed"
	}
	return "revolut merchant api returned status " + strconv.Itoa(e.StatusCode)
}

// HwRevolutConfig is the resolved provider configuration of one call. It is
// built from the environment per call, so a rotated key is picked up without a
// restart.
type HwRevolutConfig struct {
	BaseURL    string
	SecretKey  string
	APIVersion string
}

// hwRevolutAllowLoopbackHTTP permits an http:// base URL on a loopback host. It
// is a test-only hook: the httptest doubles of the test suite run on
// 127.0.0.1 without TLS, and nothing else may downgrade the provider transport.
// Production code never sets it.
var hwRevolutAllowLoopbackHTTP bool

// HwRevolutPermitLoopbackHTTPForTest enables the loopback exception above. It
// exists for the test suite only.
func HwRevolutPermitLoopbackHTTPForTest() {
	hwRevolutAllowLoopbackHTTP = true
}

// hwRevolutHostIsLoopback reports whether a bare host name (no port, no
// brackets) is a loopback address.
func hwRevolutHostIsLoopback(hostname string) bool {
	switch hostname {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

// HwRevolutConfigFromEnv resolves the provider configuration. Without a secret
// key nothing is attempted. The base URL must be https: card-payment traffic is
// never sent over plain http, regardless of any process-wide TLS setting. The
// one exception is a loopback host with the test-only hook enabled, so the
// httptest double can serve the integration.
func HwRevolutConfigFromEnv() (HwRevolutConfig, error) {
	secretKey := strings.TrimSpace(os.Getenv(HwRevolutSecretKeyEnv))
	if secretKey == "" {
		return HwRevolutConfig{}, ErrHwRevolutNotConfigured
	}
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv(HwRevolutAPIBaseEnv)), "/")
	if baseURL == "" {
		baseURL = hwRevolutDefaultAPIBase
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return HwRevolutConfig{}, ErrHwRevolutNotConfigured
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && hwRevolutAllowLoopbackHTTP && hwRevolutHostIsLoopback(parsed.Hostname())) {
		return HwRevolutConfig{}, ErrHwRevolutInsecureBaseURL
	}
	apiVersion := strings.TrimSpace(os.Getenv(HwRevolutAPIVersionEnv))
	if apiVersion == "" {
		apiVersion = hwRevolutDefaultAPIVersion
	}
	return HwRevolutConfig{BaseURL: baseURL, SecretKey: secretKey, APIVersion: apiVersion}, nil
}

// The JSON names of the provider live in the types below and nowhere else. They
// are an assumption until they are measured with sandbox keys: the test double
// decodes the same names independently, so renaming a field here breaks a test
// instead of silently sending nothing.
type hwRevolutMerchantOrderData struct {
	Reference string `json:"reference"`
}

type hwRevolutCustomer struct {
	Email string `json:"email,omitempty"`
}

type hwRevolutOrderRequest struct {
	Amount            int64                       `json:"amount"`
	Currency          string                      `json:"currency"`
	Description       string                      `json:"description,omitempty"`
	MerchantOrderData *hwRevolutMerchantOrderData `json:"merchant_order_data,omitempty"`
	Customer          *hwRevolutCustomer          `json:"customer,omitempty"`
	RedirectURL       string                      `json:"redirect_url,omitempty"`
}

type hwRevolutPaymentMethod struct {
	Fingerprint string `json:"fingerprint"`
}

type hwRevolutPayment struct {
	PaymentMethod hwRevolutPaymentMethod `json:"payment_method"`
}

type hwRevolutOrderResponse struct {
	Id                string                      `json:"id"`
	State             string                      `json:"state"`
	CheckoutURL       string                      `json:"checkout_url"`
	Token             string                      `json:"token"`
	Amount            int64                       `json:"amount"`
	Currency          string                      `json:"currency"`
	MerchantOrderData *hwRevolutMerchantOrderData `json:"merchant_order_data"`
	Payments          []hwRevolutPayment          `json:"payments"`
}

// hwRevolutStateCompleted is the provider order state that means the money
// arrived. Only this state writes credit.
const hwRevolutStateCompleted = "completed"

// hwRevolutWebhookEvent is the part of a webhook payload this integration uses.
type hwRevolutWebhookEvent struct {
	Event               string `json:"event"`
	OrderId             string `json:"order_id"`
	MerchantOrderExtRef string `json:"merchant_order_ext_ref"`
}

// Webhook event names. The completed, cancelled, declined and failed events are
// handled below. Refund and dispute events are NOT acted on automatically: their
// names (ORDER_REFUNDED, DISPUTE_*) are unconfirmed assumptions, so every event
// type not listed here is only logged (the type, never the payload) and answered
// with 200; taking credit back is an explicit admin action through the refund
// route.
const (
	hwRevolutEventOrderCompleted  = "ORDER_COMPLETED"
	hwRevolutEventOrderCancelled  = "ORDER_CANCELLED"
	hwRevolutEventPaymentDeclined = "ORDER_PAYMENT_DECLINED"
	hwRevolutEventPaymentFailed   = "ORDER_PAYMENT_FAILED"
)

// hwRevolutStateCancelled is the provider order state that means the order was
// closed without payment. Like every provider name here it is an assumption to
// be measured with sandbox keys.
const hwRevolutStateCancelled = "cancelled"

// HwVerifyRevolutWebhook checks the timestamp and the signature of a webhook.
//
// The signed text is "v1." + the timestamp exactly as the header carried it +
// "." + the raw body exactly as it was received. The signature header holds one
// or more "v1=<hex>" values separated by commas (the provider sends several
// while a secret is being rotated); the header matches when one of them equals
// the computed signature. The comparison is constant time, and a timestamp more
// than five minutes away from now is refused so a captured request cannot be
// replayed later.
func HwVerifyRevolutWebhook(rawBody []byte, timestampHeader, signatureHeader, secret string, now time.Time) error {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ErrHwRevolutWebhookSecretMissing
	}
	timestampHeader = strings.TrimSpace(timestampHeader)
	millis, err := strconv.ParseInt(timestampHeader, 10, 64)
	if err != nil {
		return ErrHwRevolutWebhookTimestampRejected
	}
	sent := time.UnixMilli(millis)
	if sent.Sub(now).Abs() > hwRevolutWebhookMaxClockGap {
		return ErrHwRevolutWebhookTimestampRejected
	}
	signatureHeader = strings.TrimSpace(signatureHeader)
	if signatureHeader == "" {
		return ErrHwRevolutWebhookSignatureRejected
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v1." + timestampHeader + "."))
	mac.Write(rawBody)
	expected := "v1=" + hex.EncodeToString(mac.Sum(nil))
	matched := false
	for candidate := range strings.SplitSeq(signatureHeader, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if hmac.Equal([]byte(candidate), []byte(expected)) {
			matched = true
		}
	}
	if !matched {
		return ErrHwRevolutWebhookSignatureRejected
	}
	return nil
}

// CreateOrder places an order at the provider.
func (config HwRevolutConfig) CreateOrder(ctx context.Context, request hwRevolutOrderRequest) (hwRevolutOrderResponse, error) {
	return config.doOrderRequest(ctx, http.MethodPost, "/api/orders", &request)
}

// GetOrder reads the current state of a provider order. The webhook uses it to
// confirm a payment before any credit is written.
func (config HwRevolutConfig) GetOrder(ctx context.Context, orderID string) (hwRevolutOrderResponse, error) {
	if strings.TrimSpace(orderID) == "" {
		return hwRevolutOrderResponse{}, ErrHwRevolutUnusableResponse
	}
	return config.doOrderRequest(ctx, http.MethodGet, "/api/orders/"+url.PathEscape(strings.TrimSpace(orderID)), nil)
}

// CancelOrder asks the provider to close an order so it can no longer be paid.
// It runs before a local close releases a first-purchase claim, so a discount
// that was given back can never be spent on an order that is still payable.
//
// ASSUMPTION, to be measured with sandbox keys: the endpoint is
// POST /api/orders/{id}/cancel and a 2xx answer means the order is closed there.
// Until that is confirmed against the sandbox, a wrong endpoint surfaces as a
// provider-status error: the close is then retried on the next reconciliation
// round and the claim stays held, which fails safe.
func (config HwRevolutConfig) CancelOrder(ctx context.Context, orderID string) error {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return ErrHwRevolutUnusableResponse
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, config.BaseURL+"/api/orders/"+url.PathEscape(orderID)+"/cancel", nil)
	if err != nil {
		return ErrHwRevolutNotConfigured
	}
	httpRequest.Header.Set("Authorization", "Bearer "+config.SecretKey)
	httpRequest.Header.Set("Revolut-Api-Version", config.APIVersion)
	response, err := hwRevolutHTTPClient().Do(httpRequest)
	if err != nil {
		return &HwRevolutAPIError{Transport: true}
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, hwRevolutMaxResponseBytes))
	if response.StatusCode < http.StatusOK || response.StatusCode > 299 {
		return &HwRevolutAPIError{StatusCode: response.StatusCode}
	}
	return nil
}

func (config HwRevolutConfig) doOrderRequest(ctx context.Context, method, path string, request *hwRevolutOrderRequest) (hwRevolutOrderResponse, error) {
	var body io.Reader
	if request != nil {
		encoded, err := common.Marshal(request)
		if err != nil {
			return hwRevolutOrderResponse{}, err
		}
		body = bytes.NewReader(encoded)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, method, config.BaseURL+path, body)
	if err != nil {
		return hwRevolutOrderResponse{}, ErrHwRevolutNotConfigured
	}
	httpRequest.Header.Set("Authorization", "Bearer "+config.SecretKey)
	httpRequest.Header.Set("Revolut-Api-Version", config.APIVersion)
	if request != nil {
		httpRequest.Header.Set("Content-Type", "application/json")
	}
	response, err := hwRevolutHTTPClient().Do(httpRequest)
	if err != nil {
		return hwRevolutOrderResponse{}, &HwRevolutAPIError{Transport: true}
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, hwRevolutMaxResponseBytes))
	if err != nil {
		return hwRevolutOrderResponse{}, &HwRevolutAPIError{Transport: true}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode > 299 {
		return hwRevolutOrderResponse{}, &HwRevolutAPIError{StatusCode: response.StatusCode}
	}
	var order hwRevolutOrderResponse
	if err := common.Unmarshal(payload, &order); err != nil {
		return hwRevolutOrderResponse{}, ErrHwRevolutUnusableResponse
	}
	return order, nil
}

// hwRevolutHTTPClient returns the dedicated outbound client of this
// integration. It is deliberately not the process-wide client: that one can be
// configured to skip certificate verification (TLS_INSECURE_SKIP_VERIFY or an
// injected CA bundle), and card-payment traffic must always verify the
// provider's certificate. The client has a fixed 30 s timeout and its own
// transport with TLS verification pinned on.
var (
	hwRevolutClientOnce sync.Once
	hwRevolutClient     *http.Client
)

func hwRevolutHTTPClient() *http.Client {
	hwRevolutClientOnce.Do(func() {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		// InsecureSkipVerify is explicitly false: this integration never
		// downgrades certificate verification, whatever the process-wide
		// transport allows elsewhere.
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: false}
		hwRevolutClient = &http.Client{Timeout: hwRevolutRequestTimeout, Transport: transport}
	})
	return hwRevolutClient
}

// hwRevolutCardFingerprint returns the provider's fingerprint of the paying
// card, if the order response carries one. It is the only card detail this
// integration stores, and it is hashed before it reaches the database; a card
// number is never read from the provider.
func hwRevolutCardFingerprint(order hwRevolutOrderResponse) string {
	for _, payment := range order.Payments {
		if fingerprint := strings.TrimSpace(payment.PaymentMethod.Fingerprint); fingerprint != "" {
			return fingerprint
		}
	}
	return ""
}
