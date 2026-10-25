package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"math"
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

	// Cost side of a paid charge. settleAmount and settleFee are in the
	// settlement currency (settleAmount 0 means "same as the charge"); pendingFee
	// leaves the balance transaction unbooked; chargeRefunded is what the charge
	// shows as already returned; refundAmounts records every refund request.
	settleAmount, settleFee int64
	settleCurrency          string
	pendingFee              bool
	chargeRefunded          int64
	refundAmounts           []int64
	refundCalls             int
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
		if !require(r.PostForm.Get("adaptive_pricing[enabled]") == "false") {
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
		fake.write(w, session, false)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/checkout/sessions/"):
		session := fake.sessions[strings.TrimPrefix(r.URL.Path, "/v1/checkout/sessions/")]
		if session == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fake.write(w, session, strings.Contains(strings.Join(r.URL.Query()["expand[]"], ","), "balance_transaction"))
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/expire"):
		session := fake.sessions[strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/checkout/sessions/"), "/expire")]
		if session == nil || session.status != "open" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		session.status = "expired"
		fake.expired++
		fake.write(w, session, false)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/refunds":
		if fake.failRefund {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		fake.refundCalls++
		refundAmount, _ := strconv.ParseInt(r.PostForm.Get("amount"), 10, 64)
		fake.refundAmounts = append(fake.refundAmounts, refundAmount)
		fake.chargeRefunded += refundAmount
		fake.refundKeys = append(fake.refundKeys, r.Header.Get("Idempotency-Key"))
		fake.refundTarget = append(fake.refundTarget, r.PostForm.Get("payment_intent"))
		_, _ = w.Write([]byte(`{"id":"re_test_1","status":"succeeded"}`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (fake *hwFakeStripe) write(w http.ResponseWriter, session *hwFakeStripeSession, expandBalance bool) {
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
		charge := map[string]any{
			"id": "ch_" + session.id, "amount": amount, "currency": currency,
			"amount_refunded":        fake.chargeRefunded,
			"payment_method_details": map[string]any{"card": map[string]any{"fingerprint": "card-fp-" + session.reference}},
		}
		if !fake.pendingFee {
			settleAmount, settleCurrency := fake.settleAmount, fake.settleCurrency
			if settleAmount == 0 {
				settleAmount = session.amount
			}
			if settleCurrency == "" {
				settleCurrency = currency
			}
			if expandBalance {
				charge["balance_transaction"] = map[string]any{
					"id": "txn_" + session.id, "amount": settleAmount, "fee": fake.settleFee, "currency": settleCurrency,
				}
			} else {
				charge["balance_transaction"] = "txn_" + session.id
			}
		}
		body["payment_intent"] = map[string]any{"id": "pi_" + session.id, "latest_charge": charge}
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
	assert.Equal(t, []string{"hw-refund-v2-" + order.MerchantRef}, fake.refundKeys)
	assert.Equal(t, []int64{30000}, fake.refundAmounts, "no provider cost in this fake: the whole payment goes back")
}

// hwStripeTestSetEurRate gives EUR a fresh rate (units of euro per dollar).
func hwStripeTestSetEurRate(t *testing.T, rate string, now time.Time) {
	t.Helper()
	common.OptionMapRWMutex.Lock()
	defer common.OptionMapRWMutex.Unlock()
	common.OptionMap[model.HwEurPerUsdOption] = rate
	common.OptionMap[model.HwEurPerUsdDateOption] = now.UTC().Format("2006-01-02")
}

// hwStripeTestPaidOrder opens a first-purchase order in one currency, lets the
// fake Stripe take the payment and settles it through the webhook.
func hwStripeTestPaidOrder(t *testing.T, fake *hwFakeStripe, currency string, amount int, now time.Time) (model.User, model.HwOrder) {
	t.Helper()
	buyer := hwOrderTestUser(t, 0)
	hwOrderTestIdentify(t, buyer.Id, "refund-fee-"+currency)
	order, err := HwCreateTopUpOrder(context.Background(), buyer.Id, HwTopUpRequest{Amount: amount, Currency: currency, Language: "en"}, now)
	require.NoError(t, err)
	fake.pay(order.MerchantRef)
	require.Equal(t, HwWebhookAccepted, hwStripeTestWebhook(t, "checkout.session.completed", fake.session(order.MerchantRef), now))
	var stored model.HwOrder
	require.NoError(t, model.DB.Where("merchant_ref = ?", order.MerchantRef).First(&stored).Error)
	require.Equal(t, model.HwOrderStatusPaid, stored.Status)
	return buyer, stored
}

func hwStripeTestOrderStatus(t *testing.T, merchantRef string) string {
	t.Helper()
	var stored model.HwOrder
	require.NoError(t, model.DB.Where("merchant_ref = ?", merchantRef).First(&stored).Error)
	return stored.Status
}

// The seller carries the processing and conversion costs while the customer
// pays: the credit is the full list value over the rate and the price is the
// half of the list value, with no surcharge. On a refund those costs are taken
// off the money that goes back.
func TestHwStripeRefundDeductsProviderCostsPerCurrency(t *testing.T) {
	cases := []struct {
		name, currency string
		amount         int
		rate           float64
		payable        int64
		settleAmount   int64
		settleFee      int64
		settleCurrency string
		wantFee        int64
		wantRefund     int64
	}{
		// 30000 try paid, settled as 749 usd cents with a fee of 52: 52*30000/749 = 2082.8, rounded up.
		{"try settled in usd", "TRY", 600, 40, 30000, 749, 52, "usd", 2083, 27917},
		// Same currency: the fee is taken over as it is.
		{"eur settled in eur", "EUR", 20, 0.8, 1000, 1000, 59, "eur", 59, 941},
		// 1000 usd cents settled as 860 euro cents with a fee of 47: 47*1000/860 = 54.65, rounded up.
		{"usd settled in eur", "USD", 20, 1, 1000, 860, 47, "eur", 55, 945},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := hwStripeTestSetup(t)
			now := time.Now()
			hwStripeTestSetEurRate(t, "0.8", now)
			buyer, order := hwStripeTestPaidOrder(t, fake, tc.currency, tc.amount, now)
			assert.Equal(t, tc.payable, order.PayableTryMinor)
			assert.Equal(t, tc.currency, order.Currency)
			wantQuota := common.QuotaRound(float64(tc.amount) / tc.rate * common.QuotaPerUnit)
			assert.Equal(t, wantQuota, order.ListQuota, "credit is the list value over the rate")
			assert.Equal(t, wantQuota, hwOrderTestQuota(t, buyer.Id), "the customer gets the full credit, nothing is held back")

			fake.mu.Lock()
			fake.settleAmount, fake.settleFee, fake.settleCurrency = tc.settleAmount, tc.settleFee, tc.settleCurrency
			fake.pendingFee = true
			fake.mu.Unlock()

			// A fee that is not booked yet: nothing changes, nothing is refunded.
			_, err := HwAdminRefundOrder(order.MerchantRef, now)
			require.ErrorIs(t, err, ErrHwRefundFeeUnavailable)
			assert.Equal(t, model.HwOrderStatusPaid, hwStripeTestOrderStatus(t, order.MerchantRef))
			assert.Equal(t, wantQuota, hwOrderTestQuota(t, buyer.Id), "the credit stays until the fee is known")
			fake.mu.Lock()
			assert.Zero(t, fake.refundCalls, "never a full refund as a fallback")
			fake.pendingFee = false
			fake.mu.Unlock()

			result, err := HwAdminRefundOrder(order.MerchantRef, now.Add(time.Minute))
			require.NoError(t, err)
			assert.True(t, result.Refunded)
			assert.Equal(t, tc.currency, result.Currency)
			assert.Equal(t, tc.wantFee, result.FeeMinor)
			assert.Equal(t, tc.wantRefund, result.RefundedMinor)
			assert.Contains(t, result.Note, "provider costs deducted "+strconv.FormatInt(tc.wantFee, 10))
			assert.Contains(t, result.Note, "refunded "+strconv.FormatInt(tc.wantRefund, 10))
			assert.Zero(t, hwOrderTestQuota(t, buyer.Id))
			fake.mu.Lock()
			assert.Equal(t, []int64{tc.wantRefund}, fake.refundAmounts)
			assert.Equal(t, []string{"hw-refund-v2-" + order.MerchantRef}, fake.refundKeys)
			fake.mu.Unlock()

			// Repeating the refund sends nothing more and reports the same amounts.
			again, err := HwAdminRefundOrder(order.MerchantRef, now.Add(2*time.Minute))
			require.NoError(t, err)
			assert.False(t, again.Refunded)
			assert.Equal(t, tc.wantFee, again.FeeMinor)
			assert.Equal(t, tc.wantRefund, again.RefundedMinor)
			fake.mu.Lock()
			assert.Equal(t, 1, fake.refundCalls, "the card is refunded once")
			fake.mu.Unlock()
		})
	}
}

func TestHwStripeRefundWithCostsEqualToThePaymentSendsNoCardRefund(t *testing.T) {
	for _, currency := range []string{"TRY", "EUR", "USD"} {
		t.Run(currency, func(t *testing.T) {
			fake := hwStripeTestSetup(t)
			now := time.Now()
			hwStripeTestSetEurRate(t, "0.8", now)
			amount := 20
			if currency == "TRY" {
				amount = 600
			}
			buyer, order := hwStripeTestPaidOrder(t, fake, currency, amount, now)
			fake.mu.Lock()
			fake.settleFee = order.PayableTryMinor // as high as the payment itself
			fake.mu.Unlock()

			result, err := HwAdminRefundOrder(order.MerchantRef, now)
			require.NoError(t, err)
			assert.Equal(t, order.PayableTryMinor, result.FeeMinor)
			assert.Zero(t, result.RefundedMinor)
			assert.Contains(t, result.Note, "no card refund")
			assert.Equal(t, model.HwOrderStatusRefunded, hwStripeTestOrderStatus(t, order.MerchantRef))
			assert.Zero(t, hwOrderTestQuota(t, buyer.Id), "the credit is still taken back")
			fake.mu.Lock()
			assert.Zero(t, fake.refundCalls)
			fake.mu.Unlock()

			_, err = HwAdminRefundOrder(order.MerchantRef, now.Add(time.Minute))
			require.NoError(t, err)
			fake.mu.Lock()
			assert.Zero(t, fake.refundCalls)
			fake.mu.Unlock()
		})
	}
}

func TestHwStripeRefundSkipsAChargeThatIsAlreadyRefunded(t *testing.T) {
	fake := hwStripeTestSetup(t)
	now := time.Now()
	_, order := hwStripeTestPaidOrder(t, fake, "USD", 20, now)
	fake.mu.Lock()
	fake.chargeRefunded = 700 // refunded by hand in the Stripe dashboard
	fake.mu.Unlock()
	result, err := HwAdminRefundOrder(order.MerchantRef, now)
	require.NoError(t, err)
	assert.EqualValues(t, 700, result.RefundedMinor)
	fake.mu.Lock()
	assert.Zero(t, fake.refundCalls, "nothing is refunded on top of a refund that already exists")
	fake.mu.Unlock()
}

func TestHwStripeRefundRefusesAChargeThatDoesNotMatchTheOrder(t *testing.T) {
	fake := hwStripeTestSetup(t)
	now := time.Now()
	buyer, order := hwStripeTestPaidOrder(t, fake, "USD", 20, now)
	fake.mu.Lock()
	fake.amountDelta = 1
	fake.mu.Unlock()
	_, err := HwAdminRefundOrder(order.MerchantRef, now)
	require.ErrorIs(t, err, ErrHwRefundFeeUnavailable)
	assert.Equal(t, model.HwOrderStatusPaid, hwStripeTestOrderStatus(t, order.MerchantRef))
	assert.Positive(t, hwOrderTestQuota(t, buyer.Id))
}

// Every currency opens a session in its own currency with the price and the
// credit worked out here: the credit is the list value over the rate, the price
// is the first-purchase price of the list value, and a client cannot change
// either. A client that names no currency still buys in lira.
func TestHwStripeTopUpPerCurrency(t *testing.T) {
	cases := []struct {
		name         string
		request      HwTopUpRequest
		currency     string
		listMinor    int64
		payableMinor int64
		listUSD      float64
	}{
		{"old lira request", HwTopUpRequest{PackageTry: 600}, "TRY", 60000, 30000, 15},
		{"lira", HwTopUpRequest{Amount: 600, Currency: "try"}, "TRY", 60000, 30000, 15},
		{"euro", HwTopUpRequest{Amount: 20, Currency: "EUR"}, "EUR", 2000, 1000, 25},
		{"dollar", HwTopUpRequest{Amount: 20, Currency: "USD"}, "USD", 2000, 1000, 20},
		// The first-purchase discount stops at 1000 lira, in euro at the same value
		// (20 EUR) and in dollar likewise (25 USD): the rest is priced at the
		// standard rate.
		{"lira over the cap", HwTopUpRequest{Amount: 5000, Currency: "TRY"}, "TRY", 500000, 450000, 125},
		{"euro over the cap", HwTopUpRequest{Amount: 100, Currency: "EUR"}, "EUR", 10000, 9000, 125},
		{"dollar over the cap", HwTopUpRequest{Amount: 100, Currency: "USD"}, "USD", 10000, 8750, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := hwStripeTestSetup(t)
			now := time.Now()
			hwStripeTestSetEurRate(t, "0.8", now)
			buyer := hwOrderTestUser(t, 0)
			hwOrderTestIdentify(t, buyer.Id, "per-currency")
			tc.request.Language = "en"
			order, err := HwCreateTopUpOrder(context.Background(), buyer.Id, tc.request, now)
			require.NoError(t, err)
			var stored model.HwOrder
			require.NoError(t, model.DB.Where("merchant_ref = ?", order.MerchantRef).First(&stored).Error)
			assert.Equal(t, tc.currency, stored.Currency)
			assert.Equal(t, tc.listMinor, stored.ListValueTryMinor)
			assert.Equal(t, tc.payableMinor, stored.PayableTryMinor)
			assert.Equal(t, common.QuotaRound(tc.listUSD*common.QuotaPerUnit), stored.ListQuota)
			session := fake.session(order.MerchantRef)
			assert.Equal(t, strings.ToLower(tc.currency), session.currency)
			assert.Equal(t, tc.payableMinor, session.amount)
		})
	}
}

func TestHwStripeTopUpRefusals(t *testing.T) {
	fake := hwStripeTestSetup(t)
	now := time.Now()
	hwStripeTestSetEurRate(t, "0.8", now)
	buyer := hwOrderTestUser(t, 0)
	hwOrderTestIdentify(t, buyer.Id, "refusals")
	create := func(request HwTopUpRequest, at time.Time) error {
		_, err := HwCreateTopUpOrder(context.Background(), buyer.Id, request, at)
		return err
	}

	assert.ErrorIs(t, create(HwTopUpRequest{Amount: 20, Currency: "GBP"}, now), ErrHwCurrencyUnsupported)
	// 20000 lira is 400 euro at this rate and 500 dollar.
	assert.ErrorIs(t, create(HwTopUpRequest{Amount: 401, Currency: "EUR"}, now), ErrHwTopUpAmountOutOfRange)
	assert.ErrorIs(t, create(HwTopUpRequest{Amount: 1, Currency: "EUR"}, now), ErrHwTopUpAmountOutOfRange)
	assert.ErrorIs(t, create(HwTopUpRequest{Amount: 501, Currency: "USD"}, now), ErrHwTopUpAmountOutOfRange)
	// A rate older than seven days stops EUR; lira and dollar keep selling.
	stale := now.Add(8 * 24 * time.Hour)
	assert.ErrorIs(t, create(HwTopUpRequest{Amount: 20, Currency: "EUR"}, stale), ErrHwTopUpRateMissing)
	assert.NoError(t, create(HwTopUpRequest{Amount: 20, Currency: "USD"}, stale))
	assert.Equal(t, 1, len(fake.order), "only the accepted request opened a session")
}

// Without Stripe only the lira is sold (Revolut takes lira only), and the
// refusal comes before the first-purchase discount is claimed.
func TestHwTopUpOtherCurrencyNeedsStripeAndClaimsNothing(t *testing.T) {
	hwOrderTestSetup(t)
	now := time.Now()
	hwStripeTestSetEurRate(t, "0.8", now)
	buyer := hwOrderTestUser(t, 0)
	hwOrderTestIdentify(t, buyer.Id, "no-stripe")
	for _, currency := range []string{"EUR", "USD"} {
		_, err := HwCreateTopUpOrder(context.Background(), buyer.Id, HwTopUpRequest{Amount: 20, Currency: currency}, now)
		assert.ErrorIs(t, err, ErrHwPaymentProviderUnavailable, currency)
	}
	var claims int64
	require.NoError(t, model.DB.Model(&model.HwFirstPurchaseClaim{}).Count(&claims).Error)
	assert.Zero(t, claims)
	_, err := HwCreateTopUpOrder(context.Background(), buyer.Id, HwTopUpRequest{PackageTry: 600}, now)
	require.NoError(t, err, "the first purchase discount was still there for the lira order")
}

// A payment in another amount or currency than the order is never credited.
func TestHwStripeSettlementRefusesWrongAmountOrCurrency(t *testing.T) {
	for _, mutate := range []func(*hwFakeStripe){
		func(fake *hwFakeStripe) { fake.amountDelta = -1 },
		func(fake *hwFakeStripe) { fake.currencyOut = "usd" },
	} {
		fake := hwStripeTestSetup(t)
		now := time.Now()
		hwStripeTestSetEurRate(t, "0.8", now)
		buyer := hwOrderTestUser(t, 0)
		hwOrderTestIdentify(t, buyer.Id, "wrong-payment")
		order, err := HwCreateTopUpOrder(context.Background(), buyer.Id, HwTopUpRequest{Amount: 20, Currency: "EUR"}, now)
		require.NoError(t, err)
		fake.pay(order.MerchantRef)
		fake.mu.Lock()
		mutate(fake)
		fake.mu.Unlock()
		hwStripeTestWebhook(t, "checkout.session.completed", fake.session(order.MerchantRef), now)
		assert.Zero(t, hwOrderTestQuota(t, buyer.Id))
		assert.NotEqual(t, model.HwOrderStatusPaid, hwStripeTestOrderStatus(t, order.MerchantRef))
	}
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

// A cost that is higher than the payment, negative or so large that the
// conversion would overflow is not believable: nothing is refunded and nothing
// of the order changes, so the refund can be repeated once the data is right.
func TestHwStripeRefundRefusesUnbelievableCosts(t *testing.T) {
	for name, fee := range map[string]int64{"above the payment": 100000000, "overflow": math.MaxInt64 / 2, "negative": -1} {
		t.Run(name, func(t *testing.T) {
			fake := hwStripeTestSetup(t)
			now := time.Now()
			buyer, order := hwStripeTestPaidOrder(t, fake, "USD", 20, now)
			fake.mu.Lock()
			fake.settleFee = fee
			fake.mu.Unlock()
			_, err := HwAdminRefundOrder(order.MerchantRef, now)
			require.ErrorIs(t, err, ErrHwRefundFeeUnavailable)
			assert.Equal(t, model.HwOrderStatusPaid, hwStripeTestOrderStatus(t, order.MerchantRef))
			assert.Positive(t, hwOrderTestQuota(t, buyer.Id))
			fake.mu.Lock()
			assert.Zero(t, fake.refundCalls)
			fake.mu.Unlock()
		})
	}
}

// Revolut is configured for lira only. Without Stripe an order in another
// currency stops at the provider choice and never reaches the Revolut client.
func TestHwProviderCheckoutNeverSendsANonLiraOrderToRevolut(t *testing.T) {
	t.Setenv(HwStripeSecretKeyEnv, "")
	for _, currency := range []string{model.HwCurrencyEUR, model.HwCurrencyUSD, "GBP"} {
		order := &model.HwOrder{MerchantRef: "hw-no-revolut-" + currency, Currency: currency, PayableTryMinor: 1000}
		_, err := hwCreateProviderCheckout(context.Background(), order, "buyer@example.com", "en")
		assert.ErrorIs(t, err, ErrHwPaymentProviderUnavailable, currency)
	}
}

// TestHwStripeLira500PaidEarnsBonusAndThirtyDays replays a real 500 TL card
// payment end to end without money: checkout session, Stripe's paid report,
// the signed webhook, the 50 TL bonus and the 30-day validity, and a second
// top-up later that restarts the 30 days for the whole balance.
func TestHwStripeLira500PaidEarnsBonusAndThirtyDays(t *testing.T) {
	fake := hwStripeTestSetup(t)
	hwBonusTestEnable(t)
	now := time.Now()
	buyer := hwOrderTestUser(t, 0)
	hwOrderTestIdentify(t, buyer.Id, "stripe-500-buyer")

	order := hwOrderTestCreate(t, buyer.Id, 500)
	session := fake.session(order.MerchantRef)
	assert.Equal(t, "try", session.currency)
	stored := hwBonusTestOrder(t, order.MerchantRef)
	assert.EqualValues(t, 5000, stored.BonusMinor, "500 TL earns 50 TL bonus")

	fake.pay(order.MerchantRef)
	require.Equal(t, HwWebhookAccepted, hwStripeTestWebhook(t, "checkout.session.completed", session, now))
	assert.Equal(t, hwOrderTestListQuota(t, 500)+stored.BonusQuota, hwOrderTestQuota(t, buyer.Id), "500 TL is booked as 550 TL")

	validity, err := model.HwUserValidity(buyer.Id, now.Unix())
	require.NoError(t, err)
	assert.Equal(t, "active", validity.Status)
	assert.Equal(t, 30, validity.DaysRemaining)

	// Twenty days later the customer tops up again: the whole balance is valid 30 days from then.
	later := now.Add(20 * 24 * time.Hour)
	second := hwOrderTestCreate(t, buyer.Id, 500)
	secondSession := fake.session(second.MerchantRef)
	fake.pay(second.MerchantRef)
	require.Equal(t, HwWebhookAccepted, hwStripeTestWebhook(t, "checkout.session.completed", secondSession, later))
	validity, err = model.HwUserValidity(buyer.Id, later.Unix())
	require.NoError(t, err)
	assert.Equal(t, 30, validity.DaysRemaining, "a top-up restarts the 30 days")
}
