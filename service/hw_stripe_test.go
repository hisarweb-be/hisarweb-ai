package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	hwStripeTestKey           = "sk_test_only_fake_key"
	hwStripeTestWebhookSecret = "whsec_test_only_fake_secret"
)

type hwFakeStripeSession struct {
	id, reference, currency, email, locale, successURL, idempotencyKey string
	amount                                                             int64
	status, paymentStatus                                              string
}

// hwFakeStripe mimics the Stripe endpoints the integration uses. It reads the
// form fields and writes the JSON with its own names, so a renamed field in the
// production code fails a test instead of silently sending nothing.
type hwFakeStripe struct {
	Server *httptest.Server

	mu           sync.Mutex
	sessions     map[string]*hwFakeStripeSession
	order        []string
	expired      int
	refundKeys   []string
	refundTarget []string
	failRefund   bool
	amountDelta  int64
	currencyOut  string
	extraIntent  bool
}

func hwNewFakeStripe(t *testing.T) *hwFakeStripe {
	t.Helper()
	fake := &hwFakeStripe{sessions: map[string]*hwFakeStripeSession{}}
	fake.Server = httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(fake.Server.Close)
	return fake
}

func (fake *hwFakeStripe) handle(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+hwStripeTestKey {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	require := func(ok bool) bool {
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
		}
		return ok
	}
	_ = r.ParseForm()
	fake.mu.Lock()
	defer fake.mu.Unlock()
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/v1/checkout/sessions":
		if !require(r.PostForm.Get("mode") == "payment") {
			return
		}
		amount, _ := strconv.ParseInt(r.PostForm.Get("line_items[0][price_data][unit_amount]"), 10, 64)
		session := &hwFakeStripeSession{
			id:             "cs_test_" + strconv.Itoa(len(fake.sessions)+1),
			reference:      r.PostForm.Get("client_reference_id"),
			currency:       r.PostForm.Get("line_items[0][price_data][currency]"),
			email:          r.PostForm.Get("customer_email"),
			locale:         r.PostForm.Get("locale"),
			successURL:     r.PostForm.Get("success_url"),
			idempotencyKey: r.Header.Get("Idempotency-Key"),
			amount:         amount,
			status:         "open",
			paymentStatus:  "unpaid",
		}
		if !require(r.PostForm.Get("metadata[merchant_ref]") == session.reference) {
			return
		}
		fake.sessions[session.id] = session
		fake.order = append(fake.order, session.id)
		fake.write(w, session)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/checkout/sessions/"):
		session := fake.sessions[strings.TrimPrefix(r.URL.Path, "/v1/checkout/sessions/")]
		if session == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fake.write(w, session)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/expire"):
		session := fake.sessions[strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/checkout/sessions/"), "/expire")]
		if session == nil || session.status != "open" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		session.status = "expired"
		fake.expired++
		fake.write(w, session)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/refunds":
		if fake.failRefund {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		fake.refundKeys = append(fake.refundKeys, r.Header.Get("Idempotency-Key"))
		fake.refundTarget = append(fake.refundTarget, r.PostForm.Get("payment_intent"))
		_, _ = w.Write([]byte(`{"id":"re_test_1","status":"succeeded"}`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (fake *hwFakeStripe) write(w http.ResponseWriter, session *hwFakeStripeSession) {
	amount := session.amount + fake.amountDelta
	currency := session.currency
	if fake.currencyOut != "" {
		currency = fake.currencyOut
	}
	body := map[string]any{
		"id": session.id, "url": "https://checkout.stripe.test/" + session.id,
		"status": session.status, "payment_status": session.paymentStatus,
		"amount_total": amount, "currency": currency, "client_reference_id": session.reference,
	}
	if session.paymentStatus == "paid" {
		body["payment_intent"] = map[string]any{"id": "pi_" + session.id, "latest_charge": map[string]any{
			"payment_method_details": map[string]any{"card": map[string]any{"fingerprint": "card-fp-" + session.reference}},
		}}
	}
	payload, _ := common.Marshal(body)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(payload)
}

func (fake *hwFakeStripe) pay(reference string) *hwFakeStripeSession {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, session := range fake.sessions {
		if session.reference == reference {
			session.status, session.paymentStatus = "complete", "paid"
			return session
		}
	}
	return nil
}

func (fake *hwFakeStripe) session(reference string) hwFakeStripeSession {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, session := range fake.sessions {
		if session.reference == reference {
			return *session
		}
	}
	return hwFakeStripeSession{}
}

func hwStripeTestSetup(t *testing.T) *hwFakeStripe {
	t.Helper()
	hwOrderTestSetup(t)
	fake := hwNewFakeStripe(t)
	t.Setenv(HwStripeSecretKeyEnv, hwStripeTestKey)
	t.Setenv(HwStripeWebhookSecretEnv, hwStripeTestWebhookSecret)
	t.Setenv(HwStripeAPIBaseEnv, fake.Server.URL)
	return fake
}

func hwStripeTestSign(body []byte, secret string, at time.Time) string {
	timestamp := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "." + string(body)))
	return "t=" + timestamp + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

// hwStripeTestEvent builds an event body the way Stripe sends it: the payment
// intent of the session is a bare id there.
func hwStripeTestEvent(t *testing.T, eventType, sessionID, reference string) []byte {
	t.Helper()
	body, err := common.Marshal(map[string]any{
		"id": "evt_1", "type": eventType,
		"data": map[string]any{"object": map[string]any{
			"id": sessionID, "client_reference_id": reference, "payment_intent": "pi_" + sessionID,
		}},
	})
	require.NoError(t, err)
	return body
}

func hwStripeTestWebhook(t *testing.T, eventType string, session hwFakeStripeSession, at time.Time) HwWebhookOutcome {
	t.Helper()
	body := hwStripeTestEvent(t, eventType, session.id, session.reference)
	return HwHandleStripeWebhook(context.Background(), body, hwStripeTestSign(body, hwStripeTestWebhookSecret, at), at)
}

func TestHwStripeFullPath(t *testing.T) {
	fake := hwStripeTestSetup(t)
	now := time.Now()
	buyer := hwOrderTestUser(t, 0)
	hwOrderTestIdentify(t, buyer.Id, "stripe-buyer")

	order := hwOrderTestCreate(t, buyer.Id, 600)
	session := fake.session(order.MerchantRef)
	assert.True(t, strings.HasPrefix(order.CheckoutURL, "https://checkout.stripe.test/cs_"))
	assert.EqualValues(t, 30000, session.amount, "the first purchase pays 300 of 600 TL")
	assert.Equal(t, "try", session.currency)
	assert.Equal(t, buyer.Email, session.email)
	assert.Equal(t, "tr", session.locale)
	assert.Equal(t, "hw-checkout-"+order.MerchantRef, session.idempotencyKey)
	assert.True(t, strings.HasSuffix(session.successURL, "/satin-al"), "the customer comes back to the top-up page")

	var stored model.HwOrder
	require.NoError(t, model.DB.Where("merchant_ref = ?", order.MerchantRef).First(&stored).Error)
	require.NotNil(t, stored.RevolutOrderId)
	assert.Equal(t, session.id, *stored.RevolutOrderId)

	// The customer has not paid: the event is not enough, Stripe's own report decides.
	assert.Equal(t, HwWebhookUnavailable, hwStripeTestWebhook(t, "checkout.session.completed", session, now))
	assert.Zero(t, hwOrderTestQuota(t, buyer.Id))

	fake.pay(order.MerchantRef)
	assert.Equal(t, HwWebhookAccepted, hwStripeTestWebhook(t, "checkout.session.completed", session, now))
	assert.Equal(t, hwOrderTestListQuota(t, 600), hwOrderTestQuota(t, buyer.Id))
	require.NoError(t, model.DB.Where("merchant_ref = ?", order.MerchantRef).First(&stored).Error)
	assert.Equal(t, model.HwOrderStatusPaid, stored.Status)

	var claim model.HwFirstPurchaseClaim
	require.NoError(t, model.DB.Where("user_id = ?", buyer.Id).First(&claim).Error)
	assert.NotEmpty(t, claim.CardFingerprint)
	assert.NotContains(t, claim.CardFingerprint, "card-fp-", "the claim stores a hash, not the raw fingerprint")
}

func TestHwStripeDuplicateWebhookCreditsOnce(t *testing.T) {
	fake := hwStripeTestSetup(t)
	now := time.Now()
	buyer := hwOrderTestUser(t, 0)
	hwOrderTestIdentify(t, buyer.Id, "stripe-dup")
	order := hwOrderTestCreate(t, buyer.Id, 600)
	fake.pay(order.MerchantRef)
	session := fake.session(order.MerchantRef)

	for _, eventType := range []string{"checkout.session.completed", "checkout.session.completed", "checkout.session.async_payment_succeeded"} {
		assert.Equal(t, HwWebhookAccepted, hwStripeTestWebhook(t, eventType, session, now))
	}
	assert.Equal(t, hwOrderTestListQuota(t, 600), hwOrderTestQuota(t, buyer.Id))
	var packages int64
	require.NoError(t, model.DB.Model(&model.HwCreditPackage{}).Where("source = ?", model.HwSourcePurchase).Count(&packages).Error)
	assert.EqualValues(t, 1, packages)
}

func TestHwStripeWebhookSignature(t *testing.T) {
	fake := hwStripeTestSetup(t)
	now := time.Now()
	buyer := hwOrderTestUser(t, 0)
	hwOrderTestIdentify(t, buyer.Id, "stripe-sig")
	order := hwOrderTestCreate(t, buyer.Id, 600)
	fake.pay(order.MerchantRef)
	session := fake.session(order.MerchantRef)
	body := hwStripeTestEvent(t, "checkout.session.completed", session.id, session.reference)
	good := hwStripeTestSign(body, hwStripeTestWebhookSecret, now)
	goodHex := good[strings.Index(good, "v1="):]

	cases := []struct {
		name   string
		body   []byte
		header string
		want   HwWebhookOutcome
	}{
		{"wrong secret", body, hwStripeTestSign(body, "whsec_other", now), HwWebhookRejected},
		{"wrong signature", body, "t=" + strconv.FormatInt(now.Unix(), 10) + ",v1=" + strings.Repeat("a", 64), HwWebhookRejected},
		{"missing header", body, "", HwWebhookRejected},
		{"changed body", append([]byte(" "), body...), good, HwWebhookRejected},
		{"stale timestamp", body, hwStripeTestSign(body, hwStripeTestWebhookSecret, now.Add(-6*time.Minute)), HwWebhookRejected},
		{"future timestamp", body, hwStripeTestSign(body, hwStripeTestWebhookSecret, now.Add(6*time.Minute)), HwWebhookRejected},
		{"no timestamp", body, goodHex, HwWebhookRejected},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.want, HwHandleStripeWebhook(context.Background(), testCase.body, testCase.header, now))
		})
	}
	assert.Zero(t, hwOrderTestQuota(t, buyer.Id), "no rejected event may credit")

	t.Run("second signature of a rotation matches", func(t *testing.T) {
		header := "t=" + strconv.FormatInt(now.Unix(), 10) + ",v1=" + strings.Repeat("b", 64) + "," + goodHex
		assert.Equal(t, HwWebhookAccepted, HwHandleStripeWebhook(context.Background(), body, header, now))
	})
	t.Run("missing webhook secret refuses everything", func(t *testing.T) {
		t.Setenv(HwStripeWebhookSecretEnv, "")
		assert.Equal(t, HwWebhookUnavailable, HwHandleStripeWebhook(context.Background(), body, good, now))
	})
}

func TestHwStripeRefusesAWrongAmountOrCurrency(t *testing.T) {
	for name, tamper := range map[string]func(*hwFakeStripe){
		"amount":   func(f *hwFakeStripe) { f.amountDelta = -100 },
		"currency": func(f *hwFakeStripe) { f.currencyOut = "usd" },
	} {
		t.Run(name, func(t *testing.T) {
			fake := hwStripeTestSetup(t)
			buyer := hwOrderTestUser(t, 0)
			hwOrderTestIdentify(t, buyer.Id, "stripe-tamper-"+name)
			order := hwOrderTestCreate(t, buyer.Id, 600)
			fake.pay(order.MerchantRef)
			tamper(fake)
			assert.Equal(t, HwWebhookIgnored, hwStripeTestWebhook(t, "checkout.session.completed", fake.session(order.MerchantRef), time.Now()))
			assert.Zero(t, hwOrderTestQuota(t, buyer.Id), "a report that disagrees with our order never credits")
			var stored model.HwOrder
			require.NoError(t, model.DB.Where("merchant_ref = ?", order.MerchantRef).First(&stored).Error)
			assert.NotEqual(t, model.HwOrderStatusPaid, stored.Status)
		})
	}
}

func TestHwStripeIgnoresEventsOfOtherProducts(t *testing.T) {
	fake := hwStripeTestSetup(t)
	now := time.Now()
	for _, reference := range []string{"", "bodybalance-123", "hw-does-not-exist"} {
		body := hwStripeTestEvent(t, "checkout.session.completed", "cs_test_foreign", reference)
		outcome := HwHandleStripeWebhook(context.Background(), body, hwStripeTestSign(body, hwStripeTestWebhookSecret, now), now)
		assert.Equal(t, HwWebhookIgnored, outcome, "the shared account must not trigger endless retries (reference %q)", reference)
		assert.Equal(t, http.StatusOK, outcome.HTTPStatus())
	}
	// A foreign session that carries the reference of an open order of ours
	// (a payment link of another product on the shared account) is ignored.
	buyer := hwOrderTestUser(t, 0)
	hwOrderTestIdentify(t, buyer.Id, "stripe-foreign")
	order := hwOrderTestCreate(t, buyer.Id, 600)
	foreign := fake.session(order.MerchantRef)
	foreign.id = "cs_test_not_ours"
	assert.Equal(t, HwWebhookIgnored, hwStripeTestWebhook(t, "checkout.session.expired", foreign, now))
	var open model.HwOrder
	require.NoError(t, model.DB.Where("merchant_ref = ?", order.MerchantRef).First(&open).Error)
	assert.Equal(t, model.HwOrderStatusPending, open.Status)

	body := []byte(`not json`)
	assert.Equal(t, HwWebhookBadRequest, HwHandleStripeWebhook(context.Background(), body, hwStripeTestSign(body, hwStripeTestWebhookSecret, now), now))
}

func TestHwStripeExpiresUnpaidOrders(t *testing.T) {
	fake := hwStripeTestSetup(t)
	now := time.Now()
	buyer := hwOrderTestUser(t, 0)
	hwOrderTestIdentify(t, buyer.Id, "stripe-expiry")

	stale := hwOrderTestCreate(t, buyer.Id, 600)
	require.NoError(t, model.DB.Model(&model.HwOrder{}).Where("merchant_ref = ?", stale.MerchantRef).Update("created_at", now.Unix()-25*3600).Error)
	run, err := HwReconcileExpiredOrders(context.Background(), now)
	require.NoError(t, err)
	assert.Equal(t, 1, run.Closed)
	assert.Equal(t, 1, fake.expired, "the session was expired at Stripe before the local close")
	var stored model.HwOrder
	require.NoError(t, model.DB.Where("merchant_ref = ?", stale.MerchantRef).First(&stored).Error)
	assert.Equal(t, model.HwOrderStatusExpired, stored.Status)

	// A customer who paid in the meantime is settled, not expired away.
	late := hwOrderTestCreate(t, buyer.Id, 600)
	fake.pay(late.MerchantRef)
	require.NoError(t, model.DB.Model(&model.HwOrder{}).Where("merchant_ref = ?", late.MerchantRef).Update("created_at", now.Unix()-25*3600).Error)
	run, err = HwReconcileExpiredOrders(context.Background(), now)
	require.NoError(t, err)
	assert.Equal(t, 1, run.Settled)
	assert.Equal(t, hwOrderTestListQuota(t, 600), hwOrderTestQuota(t, buyer.Id))

	// Stripe's own expiry event closes an open order the same way.
	expiring := hwOrderTestCreate(t, buyer.Id, 600)
	session := fake.session(expiring.MerchantRef)
	assert.Equal(t, HwWebhookAccepted, hwStripeTestWebhook(t, "checkout.session.expired", session, now))
	var cancelled model.HwOrder
	require.NoError(t, model.DB.Where("merchant_ref = ?", expiring.MerchantRef).First(&cancelled).Error)
	assert.Equal(t, model.HwOrderStatusCancelled, cancelled.Status)
}

func TestHwStripeAdminRefundReturnsTheMoney(t *testing.T) {
	fake := hwStripeTestSetup(t)
	now := time.Now()
	buyer := hwOrderTestUser(t, 0)
	hwOrderTestIdentify(t, buyer.Id, "stripe-refund")
	order := hwOrderTestCreate(t, buyer.Id, 600)
	fake.pay(order.MerchantRef)
	require.Equal(t, HwWebhookAccepted, hwStripeTestWebhook(t, "checkout.session.completed", fake.session(order.MerchantRef), now))

	fake.mu.Lock()
	fake.failRefund = true
	fake.mu.Unlock()
	_, err := HwAdminRefundOrder(order.MerchantRef, now)
	require.ErrorIs(t, err, ErrHwProviderRefundFailed)
	assert.Zero(t, hwOrderTestQuota(t, buyer.Id), "the credit is taken back first")

	fake.mu.Lock()
	fake.failRefund = false
	fake.mu.Unlock()
	_, err = HwAdminRefundOrder(order.MerchantRef, now.Add(time.Minute))
	require.NoError(t, err, "repeating the refund retries the card part")
	assert.Equal(t, []string{"pi_" + fake.session(order.MerchantRef).id}, fake.refundTarget)
	assert.Equal(t, []string{"hw-refund-" + order.MerchantRef}, fake.refundKeys)
}

func TestHwStripeIsChosenOnlyWhenConfigured(t *testing.T) {
	fake := hwStripeTestSetup(t)
	assert.True(t, HwPaymentAvailable())
	assert.True(t, HwStripePaymentAvailable())

	t.Run("without the webhook secret Stripe is not offered", func(t *testing.T) {
		t.Setenv(HwStripeWebhookSecretEnv, "")
		assert.False(t, HwStripePaymentAvailable())
		assert.True(t, HwPaymentAvailable(), "Revolut still is")
	})
	t.Run("an insecure base url is refused", func(t *testing.T) {
		t.Setenv(HwStripeAPIBaseEnv, "http://stripe.example.test")
		_, err := HwStripeConfigFromEnv()
		assert.ErrorIs(t, err, ErrHwStripeInsecureBaseURL)
	})
	t.Run("Revolut takes over when Stripe is off", func(t *testing.T) {
		t.Setenv(HwStripeSecretKeyEnv, "")
		buyer := hwOrderTestUser(t, 0)
		hwOrderTestIdentify(t, buyer.Id, "stripe-off")
		order := hwOrderTestCreate(t, buyer.Id, 600)
		assert.False(t, strings.Contains(order.CheckoutURL, "stripe"))
		assert.Empty(t, fake.session(order.MerchantRef).id, "no Stripe session was opened")
	})
	t.Run("nothing configured offers no payment", func(t *testing.T) {
		t.Setenv(HwStripeSecretKeyEnv, "")
		t.Setenv(HwRevolutSecretKeyEnv, "")
		assert.False(t, HwPaymentAvailable())
		_, err := url.Parse("")
		require.NoError(t, err)
	})
}
