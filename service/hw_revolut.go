package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
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
)

// Errors of the Revolut integration. Only these are returned to callers; an
// error text never carries a secret, a body or a header.
var (
	ErrHwRevolutNotConfigured            = errors.New("revolut merchant api is not configured")
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

// HwRevolutConfigFromEnv resolves the provider configuration. Without a secret
// key nothing is attempted.
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

// Webhook event names. The completed, cancelled and payment-failed events are
// documented; ORDER_REFUNDED and ORDER_CHARGEBACK are the names this integration
// expects for money being taken back, and have to be confirmed with sandbox keys
// like the rest of the provider contract.
const (
	hwRevolutEventOrderCompleted  = "ORDER_COMPLETED"
	hwRevolutEventOrderCancelled  = "ORDER_CANCELLED"
	hwRevolutEventPaymentDeclined = "ORDER_PAYMENT_DECLINED"
	hwRevolutEventPaymentFailed   = "ORDER_PAYMENT_FAILED"
	hwRevolutEventOrderRefunded   = "ORDER_REFUNDED"
	hwRevolutEventChargeback      = "ORDER_CHARGEBACK"
)

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

// hwRevolutHTTPClient returns the shared outbound client. It falls back to a
// plain client with a timeout in a process where InitHttpClient has not run (for
// example a unit test), instead of dereferencing nil.
func hwRevolutHTTPClient() *http.Client {
	if client := GetHttpClient(); client != nil {
		return client
	}
	return &http.Client{Timeout: 30 * time.Second}
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
