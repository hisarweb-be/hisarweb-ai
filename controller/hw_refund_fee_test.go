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
