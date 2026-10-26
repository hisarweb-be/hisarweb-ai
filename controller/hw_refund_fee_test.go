package controller

// The admin refund of a card payment deducts what the provider charged. When
// that cost is not booked yet (or cannot be read) the answer is a retryable
// 503 and nothing changes: no card refund, no revoked credit.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestHwAdminRefundAnswers503WhenTheProviderCostIsNotAvailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		model.DB = originalDB
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.HwOrder{}))
	// The provider cannot be reached: the same answer as a cost that is not booked.
	t.Setenv(service.HwStripeSecretKeyEnv, "")

	sessionID := "cs_test_refund_fee_unavailable"
	order := model.HwOrder{
		UserId: 1, MerchantRef: "hw-refund-fee-503", RevolutOrderId: &sessionID,
		ListQuota: 500000, ListValueTryMinor: 2000, PayableTryMinor: 1000,
		Currency: model.HwCurrencyEUR, DiscountKind: "first_purchase",
		Status: model.HwOrderStatusPaid, CreatedAt: 1, PaidAt: 2,
	}
	require.NoError(t, db.Create(&order).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/hw/admin/orders/refund", strings.NewReader(`{"merchant_ref":"hw-refund-fee-503"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.RequestIdKey, "hw-refund-fee-request")
	HwAdminRefundOrder(c)

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "not available yet")
	assert.Contains(t, recorder.Body.String(), "repeat the refund later")

	var stored model.HwOrder
	require.NoError(t, db.Where("merchant_ref = ?", order.MerchantRef).First(&stored).Error)
	assert.Equal(t, model.HwOrderStatusPaid, stored.Status, "the order stays paid")
	assert.Empty(t, stored.AdminNote, "nothing was written")
}

func hwRefundRouteDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	originalDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		model.DB = originalDB
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.HwOrder{}))
	return db
}

func hwRefundRouteCall(body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/hw/admin/orders/refund", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.RequestIdKey, "hw-refund-route-request")
	HwAdminRefundOrder(c)
	return recorder
}

// TestHwAdminRefundAnswersWhatIsWrongWithTheRequest proves each refusal gets
// its own status, so the admin knows whether to fix the reference, wait, or go
// to the provider, and that none of them touches the order.
func TestHwAdminRefundAnswersWhatIsWrongWithTheRequest(t *testing.T) {
	db := hwRefundRouteDB(t)
	orders := []model.HwOrder{
		{UserId: 1, MerchantRef: "hw-route-pending", ListQuota: 500000, ListValueTryMinor: 2000, PayableTryMinor: 1000,
			Currency: model.HwCurrencyEUR, Status: model.HwOrderStatusPending, CreatedAt: 1},
		{UserId: 1, MerchantRef: "hw-route-no-provider-id", ListQuota: 500000, ListValueTryMinor: 2000, PayableTryMinor: 1000,
			Currency: model.HwCurrencyEUR, Status: model.HwOrderStatusPaid, CreatedAt: 1, PaidAt: 2},
	}
	for i := range orders {
		require.NoError(t, db.Create(&orders[i]).Error)
	}

	cases := []struct {
		name     string
		body     string
		status   int
		contains string
	}{
		{"body that is not json", `not json`, http.StatusBadRequest, "Invalid request"},
		{"blank reference", `{"merchant_ref":"   "}`, http.StatusBadRequest, "Invalid request"},
		{"unknown order", `{"merchant_ref":"hw-route-unknown"}`, http.StatusNotFound, "Order not found"},
		{"order that was never paid", `{"merchant_ref":"hw-route-pending"}`, http.StatusConflict, "not in a refundable state"},
		{"paid order without a provider reference", `{"merchant_ref":"hw-route-no-provider-id"}`, http.StatusConflict, "no payment provider reference"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := hwRefundRouteCall(tc.body)
			assert.Equal(t, tc.status, recorder.Code)
			assert.Contains(t, recorder.Body.String(), tc.contains)
		})
	}

	var pending, paid model.HwOrder
	require.NoError(t, db.Where("merchant_ref = ?", "hw-route-pending").First(&pending).Error)
	require.NoError(t, db.Where("merchant_ref = ?", "hw-route-no-provider-id").First(&paid).Error)
	assert.Equal(t, model.HwOrderStatusPending, pending.Status)
	assert.Equal(t, model.HwOrderStatusPaid, paid.Status, "a refused refund leaves the paid order paid")
	assert.Empty(t, paid.AdminNote)
}
