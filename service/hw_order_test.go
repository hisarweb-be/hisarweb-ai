package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Values used by the Revolut test double. They are obviously fake and never
// leave the test process.
const (
	hwOrderTestSignalSalt     = "test-only-order-signal-salt-0123456789"
	hwOrderTestProviderSecret = "test-only-provider-secret"
	hwOrderTestWebhookSecret  = "test-only-webhook-secret"
	hwOrderTestAPIVersion     = "test-only-api-version"
	hwOrderTestRate           = "40"
)

// hwFakeProviderOrder is one order as the test double remembers it.
type hwFakeProviderOrder struct {
	Id        string
	Reference string
	Amount    int64
	Currency  string
	Email     string
	State     string
}

// hwFakeProvider mimics the two Revolut Merchant endpoints the integration uses.
// It decodes the request with its own field names, so a renamed field in the
// production types makes a test fail instead of silently sending nothing.
type hwFakeProvider struct {
	Server *httptest.Server

	mu          sync.Mutex
	orders      map[string]*hwFakeProviderOrder
	byReference map[string]*hwFakeProviderOrder
	sequence    int
	created     int
	fetched     int

	state       string
	amountDelta int64
	currency    string
	fingerprint string
	failCreate  bool
}

func hwNewFakeProvider(t *testing.T) *hwFakeProvider {
	t.Helper()
	fake := &hwFakeProvider{
		orders:      make(map[string]*hwFakeProviderOrder),
		byReference: make(map[string]*hwFakeProviderOrder),
	}
	fake.Server = httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(fake.Server.Close)
	return fake
}

func (fake *hwFakeProvider) handle(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+hwOrderTestProviderSecret {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.Header.Get("Revolut-Api-Version") == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/orders":
		fake.create(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/orders/"):
		fake.get(w, r)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (fake *hwFakeProvider) create(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var request struct {
		Amount            int64  `json:"amount"`
		Currency          string `json:"currency"`
		Description       string `json:"description"`
		RedirectURL       string `json:"redirect_url"`
		MerchantOrderData struct {
			Reference string `json:"reference"`
		} `json:"merchant_order_data"`
		Customer struct {
			Email string `json:"email"`
		} `json:"customer"`
	}
	if err := common.Unmarshal(payload, &request); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.created++
	if fake.failCreate {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if request.MerchantOrderData.Reference == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	fake.sequence++
	order := &hwFakeProviderOrder{
		Id:        fmt.Sprintf("fake-order-%d", fake.sequence),
		Reference: request.MerchantOrderData.Reference,
		Amount:    request.Amount,
		Currency:  request.Currency,
		Email:     request.Customer.Email,
		State:     "pending",
	}
	fake.orders[order.Id] = order
	fake.byReference[order.Reference] = order
	hwWriteJSON(w, http.StatusCreated, hwFakeOrderPayload{
		Id:          order.Id,
		State:       order.State,
		CheckoutURL: fake.Server.URL + "/checkout/" + order.Id,
		Amount:      order.Amount,
		Currency:    order.Currency,
		MerchantOrderData: struct {
			Reference string `json:"reference"`
		}{Reference: order.Reference},
	})
}

func (fake *hwFakeProvider) get(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/orders/")
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.fetched++
	order, ok := fake.orders[id]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	state := order.State
	if fake.state != "" {
		state = fake.state
	}
	currency := order.Currency
	if fake.currency != "" {
		currency = fake.currency
	}
	payload := hwFakeOrderPayload{
		Id:          order.Id,
		State:       state,
		CheckoutURL: fake.Server.URL + "/checkout/" + order.Id,
		Amount:      order.Amount + fake.amountDelta,
		Currency:    currency,
		MerchantOrderData: struct {
			Reference string `json:"reference"`
		}{Reference: order.Reference},
	}
	if fake.fingerprint != "" {
		payload.Payments = []hwFakePayment{{PaymentMethod: hwFakePaymentMethod{Fingerprint: fake.fingerprint}}}
	}
	hwWriteJSON(w, http.StatusOK, payload)
}

// hwFakeOrderPayload is the test double's own view of the provider response. It
// names the fields where the provider has them; a production type that renames a
// field no longer decodes what this double sends.
type hwFakeOrderPayload struct {
	Id                string `json:"id"`
	State             string `json:"state"`
	CheckoutURL       string `json:"checkout_url"`
	Token             string `json:"token"`
	Amount            int64  `json:"amount"`
	Currency          string `json:"currency"`
	MerchantOrderData struct {
		Reference string `json:"reference"`
	} `json:"merchant_order_data"`
	Payments []hwFakePayment `json:"payments,omitempty"`
}

type hwFakePayment struct {
	PaymentMethod hwFakePaymentMethod `json:"payment_method"`
}

type hwFakePaymentMethod struct {
	Fingerprint string `json:"fingerprint"`
}

func hwWriteJSON(w http.ResponseWriter, status int, payload any) {
	encoded, err := common.Marshal(payload)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(encoded)
}

func (fake *hwFakeProvider) pay(reference string) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if order, ok := fake.byReference[reference]; ok {
		order.State = "completed"
	}
}

func (fake *hwFakeProvider) providerOrderID(reference string) string {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if order, ok := fake.byReference[reference]; ok {
		return order.Id
	}
	return ""
}

func (fake *hwFakeProvider) providerOrder(reference string) hwFakeProviderOrder {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if order, ok := fake.byReference[reference]; ok {
		return *order
	}
	return hwFakeProviderOrder{}
}

func (fake *hwFakeProvider) setFingerprint(fingerprint string) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.fingerprint = fingerprint
}

func (fake *hwFakeProvider) setAmountDelta(delta int64) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.amountDelta = delta
}

func (fake *hwFakeProvider) setCurrency(currency string) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.currency = currency
}

func (fake *hwFakeProvider) setFailCreate(fail bool) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.failCreate = fail
}

var hwOrderTestColumnsOnce sync.Once

// hwOrderTestSetup starts a test from an empty top-up ledger and points the
// integration at a fresh test double.
func hwOrderTestSetup(t *testing.T) *hwFakeProvider {
	t.Helper()
	t.Setenv("HW_SIGNAL_SALT", hwOrderTestSignalSalt)
	// The group promotion of a paid purchase quotes the dialect's group column,
	// which model startup resolves in InitDB. The service test harness opens its
	// own database, so resolve it here once, the same way startup does.
	hwOrderTestColumnsOnce.Do(func() {
		require.NoError(t, model.InitLogDB())
	})
	require.NoError(t, model.DB.AutoMigrate(
		&model.HwOrder{},
		&model.HwCreditPackage{},
		&model.HwUserSignal{},
		&model.HwFirstPurchaseClaim{},
	))
	hwOrderTestCleanTables(t)
	t.Cleanup(func() { hwOrderTestCleanTables(t) })

	fake := hwNewFakeProvider(t)
	t.Setenv(HwRevolutSecretKeyEnv, hwOrderTestProviderSecret)
	t.Setenv(HwRevolutWebhookSecretEnv, hwOrderTestWebhookSecret)
	t.Setenv(HwRevolutAPIBaseEnv, fake.Server.URL)
	t.Setenv(HwRevolutAPIVersionEnv, hwOrderTestAPIVersion)

	optionKeys := []string{
		model.HwTryPerUsdOption,
		model.HwMinTopUpTryOption,
		model.HwMaxTopUpTryOption,
		model.HwMaxOpenOrdersOption,
		model.HwCheckoutReturnURLOption,
	}
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	previous := make(map[string]string, len(optionKeys))
	for _, key := range optionKeys {
		previous[key] = common.OptionMap[key]
		delete(common.OptionMap, key)
	}
	common.OptionMap[model.HwTryPerUsdOption] = hwOrderTestRate
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		for _, key := range optionKeys {
			if value, ok := previous[key]; ok {
				common.OptionMap[key] = value
			} else {
				delete(common.OptionMap, key)
			}
		}
	})
	return fake
}

func hwOrderTestCleanTables(t *testing.T) {
	t.Helper()
	for _, table := range []string{"hw_orders", "hw_credit_packages", "hw_user_signals", "hw_first_purchase_claims"} {
		require.NoError(t, model.DB.Exec("DELETE FROM "+table).Error)
	}
}

var hwOrderTestPhoneSeq int64

func hwOrderTestUser(t *testing.T, inviterID int) model.User {
	t.Helper()
	user := model.User{
		Username:    "hw-order-" + common.GetRandomString(10),
		Password:    "unused-password-hash",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
		AffCode:     "hwo" + common.GetRandomString(8),
		InviterId:   inviterID,
		Email:       "hw-" + common.GetRandomString(8) + "@example.test",
	}
	require.NoError(t, model.DB.Create(&user).Error)
	return user
}

// hwOrderTestIdentify gives a user a device and a distinct phone signal, which
// the first-purchase claim and the affiliate reward both require.
func hwOrderTestIdentify(t *testing.T, userID int, device string) {
	t.Helper()
	hwOrderTestPhoneSeq++
	require.NoError(t, model.UpsertHwUserSignals(userID, model.HwUserSignals{
		DeviceToken: "device-" + device,
		Phone:       fmt.Sprintf("+90555%07d", hwOrderTestPhoneSeq),
	}))
}

func hwOrderTestCreate(t *testing.T, userID int, packageTry int) HwTopUpOrder {
	t.Helper()
	order, err := HwCreateTopUpOrder(context.Background(), userID, HwTopUpRequest{PackageTry: packageTry, Language: "tr"}, time.Now())
	require.NoError(t, err)
	require.NotEmpty(t, order.MerchantRef)
	require.NotEmpty(t, order.CheckoutURL)
	return order
}

func hwOrderTestQuota(t *testing.T, userID int) int {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Select("id", "quota", "group").Where("id = ?", userID).First(&user).Error)
	return user.Quota
}

func hwOrderTestGroup(t *testing.T, userID int) string {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Select("id", "group").Where("id = ?", userID).First(&user).Error)
	return user.Group
}

func hwOrderTestListQuota(t *testing.T, packageTry int) int {
	t.Helper()
	rate, err := strconv.ParseFloat(hwOrderTestRate, 64)
	require.NoError(t, err)
	return common.QuotaRound(float64(packageTry) / rate * common.QuotaPerUnit)
}

func hwOrderTestEventBody(t *testing.T, event, orderID, reference string) []byte {
	t.Helper()
	body, err := common.Marshal(struct {
		Event               string `json:"event"`
		OrderId             string `json:"order_id"`
		MerchantOrderExtRef string `json:"merchant_order_ext_ref"`
	}{Event: event, OrderId: orderID, MerchantOrderExtRef: reference})
	require.NoError(t, err)
	return body
}

func hwOrderTestSign(body []byte, secret string, at time.Time) (timestamp string, signature string) {
	timestamp = strconv.FormatInt(at.UnixMilli(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("v1." + timestamp + "."))
	mac.Write(body)
	return timestamp, "v1=" + hex.EncodeToString(mac.Sum(nil))
}

func hwOrderTestWebhookSigned(t *testing.T, body []byte, secret string, at time.Time) HwWebhookOutcome {
	t.Helper()
	timestamp, signature := hwOrderTestSign(body, secret, at)
	return HwHandleRevolutWebhook(context.Background(), body, timestamp, signature, at)
}

func hwOrderTestWebhook(t *testing.T, event, orderID, reference string, at time.Time) HwWebhookOutcome {
	t.Helper()
	return hwOrderTestWebhookSigned(t, hwOrderTestEventBody(t, event, orderID, reference), os.Getenv(HwRevolutWebhookSecretEnv), at)
}

func hwOrderTestPaidWebhook(t *testing.T, fake *hwFakeProvider, reference string, at time.Time) HwWebhookOutcome {
	t.Helper()
	fake.pay(reference)
	return hwOrderTestWebhook(t, "ORDER_COMPLETED", fake.providerOrderID(reference), reference, at)
}

// hwOrderTestLogCapture redirects both server log writers into buffers, like the
// credit package tests do, so a test can search the log output.
type hwOrderTestLogCapture struct {
	Out bytes.Buffer
	Err bytes.Buffer
}

func (capture *hwOrderTestLogCapture) All() string {
	return capture.Out.String() + capture.Err.String()
}

func hwOrderTestCaptureLog(t *testing.T) *hwOrderTestLogCapture {
	t.Helper()
	capture := &hwOrderTestLogCapture{}
	common.LogWriterMu.Lock()
	previousOut := gin.DefaultWriter
	previousErr := gin.DefaultErrorWriter
	gin.DefaultWriter = &capture.Out
	gin.DefaultErrorWriter = &capture.Err
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter = previousOut
		gin.DefaultErrorWriter = previousErr
		common.LogWriterMu.Unlock()
	})
	return capture
}

func TestHwRevolutWebhookSignature(t *testing.T) {
	hwOrderTestSetup(t)
	at := time.Now()
	body := hwOrderTestEventBody(t, "ORDER_COMPLETED", "unknown-provider-order", "hw-unknown")
	timestamp, signature := hwOrderTestSign(body, hwOrderTestWebhookSecret, at)
	staleTimestamp, staleSignature := hwOrderTestSign(body, hwOrderTestWebhookSecret, at.Add(-6*time.Minute))
	futureTimestamp, futureSignature := hwOrderTestSign(body, hwOrderTestWebhookSecret, at.Add(6*time.Minute))

	cases := []struct {
		name      string
		body      []byte
		timestamp string
		signature string
		now       time.Time
		want      HwWebhookOutcome
	}{
		{"valid signature", body, timestamp, signature, at, HwWebhookIgnored},
		{"valid signature is the second of two", body, timestamp, "v1=" + strings.Repeat("a", 64) + "," + signature, at, HwWebhookIgnored},
		{"wrong signature", body, timestamp, "v1=" + strings.Repeat("b", 64), at, HwWebhookRejected},
		{"missing signature", body, timestamp, "", at, HwWebhookRejected},
		{"missing timestamp", body, "", signature, at, HwWebhookRejected},
		{"changed body", []byte(`{"event":"ORDER_COMPLETED"} `), timestamp, signature, at, HwWebhookRejected},
		{"wrong version prefix", body, timestamp, "v2=" + strings.TrimPrefix(signature, "v1="), at, HwWebhookRejected},
		{"stale timestamp", body, staleTimestamp, staleSignature, at, HwWebhookRejected},
		{"future timestamp", body, futureTimestamp, futureSignature, at, HwWebhookRejected},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			outcome := HwHandleRevolutWebhook(context.Background(), testCase.body, testCase.timestamp, testCase.signature, testCase.now)
			assert.Equal(t, testCase.want, outcome)
		})
	}

	t.Run("missing webhook secret refuses everything", func(t *testing.T) {
		t.Setenv(HwRevolutWebhookSecretEnv, "")
		outcome := HwHandleRevolutWebhook(context.Background(), body, timestamp, signature, at)
		assert.Equal(t, HwWebhookUnavailable, outcome)
		assert.Equal(t, http.StatusServiceUnavailable, outcome.HTTPStatus())
	})
}

func TestHwTopUpFullPath(t *testing.T) {
	fake := hwOrderTestSetup(t)
	now := time.Now()
	expectedQuota := hwOrderTestListQuota(t, 600)

	inviter := hwOrderTestUser(t, 0)
	hwOrderTestIdentify(t, inviter.Id, "inviter")
	buyer := hwOrderTestUser(t, inviter.Id)
	hwOrderTestIdentify(t, buyer.Id, "buyer")

	order := hwOrderTestCreate(t, buyer.Id, 600)
	providerOrder := fake.providerOrder(order.MerchantRef)
	assert.EqualValues(t, 30000, providerOrder.Amount, "the first purchase pays 300 of 600 TL")
	assert.Equal(t, model.HwOrderCurrency, providerOrder.Currency)
	assert.Equal(t, buyer.Email, providerOrder.Email)

	fake.setFingerprint("card-fingerprint-abc")
	require.Equal(t, HwWebhookAccepted, hwOrderTestPaidWebhook(t, fake, order.MerchantRef, now))

	assert.Equal(t, expectedQuota, hwOrderTestQuota(t, buyer.Id), "the full list value is credited")
	assert.Equal(t, "paid", hwOrderTestGroup(t, buyer.Id))

	var purchase model.HwCreditPackage
	require.NoError(t, model.DB.Where("source = ? AND order_ref = ?", model.HwSourcePurchase, order.MerchantRef).First(&purchase).Error)
	assert.Equal(t, expectedQuota, purchase.GrantedQuota)
	assert.EqualValues(t, 30*24*3600, purchase.ExpiresAt-purchase.CreatedAt)

	var affiliate model.HwCreditPackage
	require.NoError(t, model.DB.Where("source = ? AND order_ref = ?", model.HwSourceAffiliate, "aff:"+order.MerchantRef).First(&affiliate).Error)
	assert.Equal(t, common.QuotaRound(float64(expectedQuota)*model.HwAffiliatePercent/100), affiliate.GrantedQuota)
	assert.Equal(t, inviter.Id, affiliate.UserId)
	assert.EqualValues(t, 30*24*3600, affiliate.ExpiresAt-affiliate.CreatedAt)
	assert.Equal(t, common.QuotaRound(float64(expectedQuota)*model.HwAffiliatePercent/100), hwOrderTestQuota(t, inviter.Id))

	var claim model.HwFirstPurchaseClaim
	require.NoError(t, model.DB.Where("user_id = ?", buyer.Id).First(&claim).Error)
	assert.NotEmpty(t, claim.CardFingerprint)
	assert.NotEqual(t, "card-fingerprint-abc", claim.CardFingerprint, "the claim stores a hash, not the raw fingerprint")

	var cardSignal model.HwUserSignal
	require.NoError(t, model.DB.Where("user_id = ? AND kind = ?", buyer.Id, model.HwSignalKindCard).First(&cardSignal).Error)
	assert.NotEqual(t, "card-fingerprint-abc", cardSignal.Hash)

	var paidOrder model.HwOrder
	require.NoError(t, model.DB.Where("merchant_ref = ?", order.MerchantRef).First(&paidOrder).Error)
	assert.Equal(t, model.HwOrderStatusPaid, paidOrder.Status)
	assert.EqualValues(t, now.Unix(), paidOrder.PaidAt)
	require.NotNil(t, paidOrder.RevolutOrderId)
	assert.Equal(t, fake.providerOrderID(order.MerchantRef), *paidOrder.RevolutOrderId)

	// A repeated event must not credit again.
	require.Equal(t, HwWebhookAccepted, hwOrderTestPaidWebhook(t, fake, order.MerchantRef, now.Add(time.Minute)))
	assert.Equal(t, expectedQuota, hwOrderTestQuota(t, buyer.Id))
	var purchaseCount, affiliateCount int64
	require.NoError(t, model.DB.Model(&model.HwCreditPackage{}).Where("source = ?", model.HwSourcePurchase).Count(&purchaseCount).Error)
	require.NoError(t, model.DB.Model(&model.HwCreditPackage{}).Where("source = ?", model.HwSourceAffiliate).Count(&affiliateCount).Error)
	assert.EqualValues(t, 1, purchaseCount)
	assert.EqualValues(t, 1, affiliateCount)
}

func TestHwTopUpAffiliateSkipsASharedDevice(t *testing.T) {
	fake := hwOrderTestSetup(t)
	now := time.Now()

	inviter := hwOrderTestUser(t, 0)
	hwOrderTestIdentify(t, inviter.Id, "shared-device")
	buyer := hwOrderTestUser(t, inviter.Id)
	hwOrderTestPhoneSeq++
	require.NoError(t, model.UpsertHwUserSignals(buyer.Id, model.HwUserSignals{
		DeviceToken: "device-shared-device",
		Phone:       fmt.Sprintf("+90555%07d", hwOrderTestPhoneSeq),
	}))

	order := hwOrderTestCreate(t, buyer.Id, 600)
	require.Equal(t, HwWebhookAccepted, hwOrderTestPaidWebhook(t, fake, order.MerchantRef, now))

	var affiliateCount int64
	require.NoError(t, model.DB.Model(&model.HwCreditPackage{}).Where("source = ?", model.HwSourceAffiliate).Count(&affiliateCount).Error)
	assert.EqualValues(t, 0, affiliateCount, "the inviter shares the buying device")
	assert.Zero(t, hwOrderTestQuota(t, inviter.Id))
}

func TestHwTopUpDuplicateWebhooksCreditOnce(t *testing.T) {
	fake := hwOrderTestSetup(t)
	now := time.Now()
	expectedQuota := hwOrderTestListQuota(t, 600)

	inviter := hwOrderTestUser(t, 0)
	hwOrderTestIdentify(t, inviter.Id, "dup-inviter")
	buyer := hwOrderTestUser(t, inviter.Id)
	hwOrderTestIdentify(t, buyer.Id, "dup-buyer")

	order := hwOrderTestCreate(t, buyer.Id, 600)
	fake.pay(order.MerchantRef)
	providerOrderID := fake.providerOrderID(order.MerchantRef)
	body := hwOrderTestEventBody(t, "ORDER_COMPLETED", providerOrderID, order.MerchantRef)

	outcomes := make([]HwWebhookOutcome, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for index := range outcomes {
		wg.Go(func() {
			<-start
			outcomes[index] = hwOrderTestWebhookSigned(t, body, hwOrderTestWebhookSecret, now)
		})
	}
	close(start)
	wg.Wait()
	assert.Equal(t, []HwWebhookOutcome{HwWebhookAccepted, HwWebhookAccepted}, outcomes)
	assert.Equal(t, expectedQuota, hwOrderTestQuota(t, buyer.Id))

	var purchase model.HwCreditPackage
	require.NoError(t, model.DB.Where("source = ? AND order_ref = ?", model.HwSourcePurchase, order.MerchantRef).First(&purchase).Error,
		"the credited package must carry our own order reference")
	assert.Equal(t, expectedQuota, purchase.GrantedQuota)
	var purchaseCount, affiliateCount, paidCount int64
	require.NoError(t, model.DB.Model(&model.HwCreditPackage{}).Where("source = ?", model.HwSourcePurchase).Count(&purchaseCount).Error)
	require.NoError(t, model.DB.Model(&model.HwCreditPackage{}).Where("source = ?", model.HwSourceAffiliate).Count(&affiliateCount).Error)
	require.NoError(t, model.DB.Model(&model.HwOrder{}).Where("merchant_ref = ? AND status = ?", order.MerchantRef, model.HwOrderStatusPaid).Count(&paidCount).Error)
	assert.EqualValues(t, 1, purchaseCount)
	assert.EqualValues(t, 1, affiliateCount)
	assert.EqualValues(t, 1, paidCount)

	// A later repeat must not rewrite the moment of payment.
	var paidOrder model.HwOrder
	require.NoError(t, model.DB.Where("merchant_ref = ?", order.MerchantRef).First(&paidOrder).Error)
	firstPaidAt := paidOrder.PaidAt
	require.Equal(t, HwWebhookAccepted, hwOrderTestWebhookSigned(t, body, hwOrderTestWebhookSecret, now.Add(time.Hour)))
	require.NoError(t, model.DB.Where("merchant_ref = ?", order.MerchantRef).First(&paidOrder).Error)
	assert.EqualValues(t, firstPaidAt, paidOrder.PaidAt)
	assert.Equal(t, expectedQuota, hwOrderTestQuota(t, buyer.Id))
}

func TestHwTopUpRefusesAMismatchedProviderReport(t *testing.T) {
	fake := hwOrderTestSetup(t)
	now := time.Now()
	buyer := hwOrderTestUser(t, 0)
	hwOrderTestIdentify(t, buyer.Id, "mismatch")

	order := hwOrderTestCreate(t, buyer.Id, 600)
	fake.setAmountDelta(100)
	require.Equal(t, HwWebhookIgnored, hwOrderTestPaidWebhook(t, fake, order.MerchantRef, now))
	assert.Zero(t, hwOrderTestQuota(t, buyer.Id), "a larger reported amount must not credit the order")

	fake.setAmountDelta(0)
	fake.setCurrency("USD")
	require.Equal(t, HwWebhookIgnored, hwOrderTestPaidWebhook(t, fake, order.MerchantRef, now))
	assert.Zero(t, hwOrderTestQuota(t, buyer.Id), "another currency must not credit the order")

	fake.setCurrency("")
	require.Equal(t, HwWebhookAccepted, hwOrderTestPaidWebhook(t, fake, order.MerchantRef, now))
	assert.Equal(t, hwOrderTestListQuota(t, 600), hwOrderTestQuota(t, buyer.Id))
}

func TestHwTopUpIgnoresAmountsFromTheClient(t *testing.T) {
	fake := hwOrderTestSetup(t)
	buyer := hwOrderTestUser(t, 0)
	hwOrderTestIdentify(t, buyer.Id, "client-amount")

	var request HwTopUpRequest
	require.NoError(t, common.Unmarshal([]byte(`{"package_try":600,"amount":1,"payable_try_minor":1,"list_quota":1}`), &request))
	order, err := HwCreateTopUpOrder(context.Background(), buyer.Id, request, time.Now())
	require.NoError(t, err)
	assert.EqualValues(t, 30000, fake.providerOrder(order.MerchantRef).Amount)

	var stored model.HwOrder
	require.NoError(t, model.DB.Where("merchant_ref = ?", order.MerchantRef).First(&stored).Error)
	assert.Equal(t, hwOrderTestListQuota(t, 600), stored.ListQuota)
	assert.EqualValues(t, 30000, stored.PayableTryMinor)
	assert.EqualValues(t, 60000, stored.ListValueTryMinor)

	_, err = HwCreateTopUpOrder(context.Background(), buyer.Id, HwTopUpRequest{PackageTry: 600000}, time.Now())
	assert.ErrorIs(t, err, ErrHwTopUpAmountOutOfRange)
}

func TestHwFirstPurchaseDiscountIsClaimedOnce(t *testing.T) {
	fake := hwOrderTestSetup(t)
	now := time.Now()
	buyer := hwOrderTestUser(t, 0)
	hwOrderTestIdentify(t, buyer.Id, "discount")

	orders := make([]HwTopUpOrder, 2)
	errs := make([]error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for index := range orders {
		wg.Go(func() {
			<-start
			orders[index], errs[index] = HwCreateTopUpOrder(context.Background(), buyer.Id, HwTopUpRequest{PackageTry: 600}, now)
		})
	}
	close(start)
	wg.Wait()
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])

	var stored []model.HwOrder
	require.NoError(t, model.DB.Where("user_id = ?", buyer.Id).Order("id").Find(&stored).Error)
	require.Len(t, stored, 2)
	discounts := map[string]int64{}
	for _, order := range stored {
		discounts[order.DiscountKind] = order.PayableTryMinor
	}
	assert.Equal(t, map[string]int64{
		model.HwDiscountFirstPurchase: 30000,
		model.HwDiscountReturning:     51000,
	}, discounts, "exactly one of two concurrent orders gets the first-purchase price")

	// Cancelling the discounted order gives the discount back.
	var discounted model.HwOrder
	require.NoError(t, model.DB.Where("user_id = ? AND discount_kind = ?", buyer.Id, model.HwDiscountFirstPurchase).First(&discounted).Error)
	require.Equal(t, HwWebhookAccepted, hwOrderTestWebhook(t, "ORDER_CANCELLED", fake.providerOrderID(discounted.MerchantRef), discounted.MerchantRef, now))

	retried := hwOrderTestCreate(t, buyer.Id, 600)
	var retriedOrder model.HwOrder
	require.NoError(t, model.DB.Where("merchant_ref = ?", retried.MerchantRef).First(&retriedOrder).Error)
	assert.Equal(t, model.HwDiscountFirstPurchase, retriedOrder.DiscountKind)
	assert.EqualValues(t, 30000, retriedOrder.PayableTryMinor)

	// An expired first order also gives the discount back.
	require.NoError(t, model.DB.Model(&model.HwOrder{}).Where("merchant_ref = ?", retried.MerchantRef).Update("created_at", now.Unix()-25*3600).Error)
	closed, err := model.HwExpirePendingOrders(now.Unix())
	require.NoError(t, err)
	assert.EqualValues(t, 1, closed)
	var expiredOrder model.HwOrder
	require.NoError(t, model.DB.Where("merchant_ref = ?", retried.MerchantRef).First(&expiredOrder).Error)
	assert.Equal(t, model.HwOrderStatusExpired, expiredOrder.Status)

	afterExpiry := hwOrderTestCreate(t, buyer.Id, 600)
	var afterExpiryOrder model.HwOrder
	require.NoError(t, model.DB.Where("merchant_ref = ?", afterExpiry.MerchantRef).First(&afterExpiryOrder).Error)
	assert.Equal(t, model.HwDiscountFirstPurchase, afterExpiryOrder.DiscountKind)

	// A second paid purchase gets the standard discount.
	require.Equal(t, HwWebhookAccepted, hwOrderTestPaidWebhook(t, fake, afterExpiry.MerchantRef, now))
	next := hwOrderTestCreate(t, buyer.Id, 600)
	var nextOrder model.HwOrder
	require.NoError(t, model.DB.Where("merchant_ref = ?", next.MerchantRef).First(&nextOrder).Error)
	assert.Equal(t, model.HwDiscountReturning, nextOrder.DiscountKind)
	assert.EqualValues(t, 51000, nextOrder.PayableTryMinor)
}

func TestHwTopUpClosedOrdersNeverCredit(t *testing.T) {
	fake := hwOrderTestSetup(t)
	now := time.Now()
	buyer := hwOrderTestUser(t, 0)
	hwOrderTestIdentify(t, buyer.Id, "closed")

	cancelled := hwOrderTestCreate(t, buyer.Id, 600)
	require.Equal(t, HwWebhookAccepted, hwOrderTestWebhook(t, "ORDER_CANCELLED", fake.providerOrderID(cancelled.MerchantRef), cancelled.MerchantRef, now))
	var cancelledOrder model.HwOrder
	require.NoError(t, model.DB.Where("merchant_ref = ?", cancelled.MerchantRef).First(&cancelledOrder).Error)
	assert.Equal(t, model.HwOrderStatusCancelled, cancelledOrder.Status)
	assert.Zero(t, hwOrderTestQuota(t, buyer.Id))
	var packages, claims int64
	require.NoError(t, model.DB.Model(&model.HwCreditPackage{}).Count(&packages).Error)
	require.NoError(t, model.DB.Model(&model.HwFirstPurchaseClaim{}).Where("user_id = ?", buyer.Id).Count(&claims).Error)
	assert.Zero(t, packages)
	assert.Zero(t, claims, "a cancelled order gives the first-purchase claim back")

	declined := hwOrderTestCreate(t, buyer.Id, 600)
	require.Equal(t, HwWebhookAccepted, hwOrderTestWebhook(t, "ORDER_PAYMENT_DECLINED", fake.providerOrderID(declined.MerchantRef), declined.MerchantRef, now))
	var declinedOrder model.HwOrder
	require.NoError(t, model.DB.Where("merchant_ref = ?", declined.MerchantRef).First(&declinedOrder).Error)
	assert.Equal(t, model.HwOrderStatusFailed, declinedOrder.Status)
	assert.Zero(t, hwOrderTestQuota(t, buyer.Id))
	require.NoError(t, model.DB.Model(&model.HwFirstPurchaseClaim{}).Where("user_id = ?", buyer.Id).Count(&claims).Error)
	assert.Zero(t, claims)
}

func TestHwTopUpExpiresUnpaidOrders(t *testing.T) {
	hwOrderTestSetup(t)
	now := time.Now()
	buyer := hwOrderTestUser(t, 0)
	hwOrderTestIdentify(t, buyer.Id, "expiry")
	order := hwOrderTestCreate(t, buyer.Id, 600)

	closed, err := model.HwExpirePendingOrders(now.Unix())
	require.NoError(t, err)
	assert.Zero(t, closed, "an order younger than the pending window stays open")

	require.NoError(t, model.DB.Model(&model.HwOrder{}).Where("merchant_ref = ?", order.MerchantRef).Update("created_at", now.Unix()-25*3600).Error)
	closed, err = model.HwExpirePendingOrders(now.Unix())
	require.NoError(t, err)
	assert.EqualValues(t, 1, closed)

	var expired model.HwOrder
	require.NoError(t, model.DB.Where("merchant_ref = ?", order.MerchantRef).First(&expired).Error)
	assert.Equal(t, model.HwOrderStatusExpired, expired.Status)
	var claims int64
	require.NoError(t, model.DB.Model(&model.HwFirstPurchaseClaim{}).Where("user_id = ?", buyer.Id).Count(&claims).Error)
	assert.Zero(t, claims)
}

func TestHwTopUpRefundRevokesCreditAndReward(t *testing.T) {
	fake := hwOrderTestSetup(t)
	now := time.Now()
	expectedQuota := hwOrderTestListQuota(t, 600)
	inviter := hwOrderTestUser(t, 0)
	hwOrderTestIdentify(t, inviter.Id, "refund-inviter")
	buyer := hwOrderTestUser(t, inviter.Id)
	hwOrderTestIdentify(t, buyer.Id, "refund-buyer")

	order := hwOrderTestCreate(t, buyer.Id, 600)
	require.Equal(t, HwWebhookAccepted, hwOrderTestPaidWebhook(t, fake, order.MerchantRef, now))
	require.Equal(t, expectedQuota, hwOrderTestQuota(t, buyer.Id))

	require.Equal(t, HwWebhookAccepted, hwOrderTestWebhook(t, "ORDER_REFUNDED", fake.providerOrderID(order.MerchantRef), order.MerchantRef, now.Add(time.Minute)))
	assert.Zero(t, hwOrderTestQuota(t, buyer.Id), "the unspent credit is taken back")
	assert.Zero(t, hwOrderTestQuota(t, inviter.Id), "the affiliate reward is taken back")

	// A second refund changes nothing.
	require.Equal(t, HwWebhookAccepted, hwOrderTestWebhook(t, "ORDER_REFUNDED", fake.providerOrderID(order.MerchantRef), order.MerchantRef, now.Add(2*time.Minute)))
	assert.Zero(t, hwOrderTestQuota(t, buyer.Id))
}

func TestHwTopUpLogsNoSecrets(t *testing.T) {
	fake := hwOrderTestSetup(t)
	capture := hwOrderTestCaptureLog(t)
	now := time.Now()
	marker := "prompt-marker-9f3a"
	buyer := hwOrderTestUser(t, 0)
	hwOrderTestIdentify(t, buyer.Id, "logs")

	fake.setFailCreate(true)
	_, err := HwCreateTopUpOrder(context.Background(), buyer.Id, HwTopUpRequest{PackageTry: 600, Language: marker}, now)
	require.ErrorIs(t, err, ErrHwPaymentProviderUnavailable)
	fake.setFailCreate(false)

	body := hwOrderTestEventBody(t, "ORDER_COMPLETED", "unknown", "hw-unknown")
	require.Equal(t, HwWebhookRejected, HwHandleRevolutWebhook(context.Background(), body, "1700000000000", "v1=deadbeef", now))

	order := hwOrderTestCreate(t, buyer.Id, 600)
	fake.setFingerprint("card-fingerprint-abc")
	require.Equal(t, HwWebhookAccepted, hwOrderTestPaidWebhook(t, fake, order.MerchantRef, now))

	logs := capture.All()
	require.Contains(t, logs, "revolut webhook credited the top-up", "the capture must see the log lines")
	for _, secret := range []string{
		hwOrderTestProviderSecret,
		hwOrderTestWebhookSecret,
		"card-fingerprint-abc",
		"Bearer",
		"v1=",
		marker,
	} {
		assert.NotContains(t, logs, secret)
	}
}
