package controller

// HisarWeb AI credit packages: the balance a user holds, broken down per package
// with the expiry date of each. The wallet page only shows the total, so the
// top-up page needs this route to tell a customer what expires when.

import (
	"cmp"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// hwPackageAnswer is one credit package as the customer sees it. The ledger ids
// stay in the database: they identify rows, not packages.
type hwPackageAnswer struct {
	Source         string `json:"source"`
	GrantedQuota   int    `json:"granted_quota"`
	RemainingQuota int    `json:"remaining_quota"`
	CreatedAt      int64  `json:"created_at"`
	ExpiresAt      int64  `json:"expires_at"`
}

// HwListUserPackages answers the active credit packages of the logged-in user.
// It also names the quota unit and the TRY rate of the current price snapshot,
// so the page can show every package in lira without a second source of truth;
// without a snapshot the rate is empty and the page shows no lira value rather
// than an invented one.
func HwListUserPackages(c *gin.Context) {
	userID := c.GetInt("id")
	views, err := model.ListUserCreditPackages(userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorStatus(c, http.StatusNotFound, common.NewMessage("User not found"))
			return
		}
		logger.LogError(c.Request.Context(), common.LogText("listing credit packages failed"))
		common.ApiError(c, common.NewMessage("Failed to read the credit packages"))
		return
	}
	// Soonest expiry first: that is the order in which the balance is spent.
	slices.SortFunc(views, func(a, b model.HwPackageView) int {
		if order := cmp.Compare(a.ExpiresAt, b.ExpiresAt); order != 0 {
			return order
		}
		return cmp.Compare(a.Id, b.Id)
	})
	total := 0
	packages := make([]hwPackageAnswer, 0, len(views))
	for _, view := range views {
		total += view.RemainingQuota
		packages = append(packages, hwPackageAnswer{
			Source:         view.Source,
			GrantedQuota:   view.GrantedQuota,
			RemainingQuota: view.RemainingQuota,
			CreatedAt:      view.CreatedAt,
			ExpiresAt:      view.ExpiresAt,
		})
	}
	tryPerUsd, fxDate, fxSource := "", "", ""
	if snapshot, snapshotErr := model.HwLatestPriceSnapshot(); snapshotErr == nil {
		tryPerUsd, fxDate, fxSource = snapshot.TryPerUsd, snapshot.FxDate, snapshot.FxSourceUrl
	}
	// The dashboard marks the phone step of the start bonus as done from this
	// flag. A failing lookup must not break the card that shows the packages,
	// so it reads as "not done" and the failure only reaches the server log.
	phoneShared := false
	if shared, sharedErr := model.HwUserHasPhoneSignal(userID); sharedErr == nil {
		phoneShared = shared
	} else {
		logger.LogError(c.Request.Context(), common.LogText("reading the phone step failed"))
	}
	validity, validityErr := model.HwUserValidity(userID, time.Now().Unix())
	if validityErr != nil {
		// The card that counts the days must not take the package list down
		// with it; the page shows "no validity" and the server log has the cause.
		logger.LogError(c.Request.Context(), common.LogText("reading the credit validity failed"))
		validity = model.HwValiditySummary{Status: model.HwValidityNone}
	}
	common.ApiSuccess(c, gin.H{
		"validity":              validity,
		"bonus_rules":           service.HwBonusRules(),
		"packages":              packages,
		"total_remaining_quota": total,
		"quota_per_unit":        common.QuotaPerUnit,
		"try_per_usd":           tryPerUsd,
		"fx_date":               fxDate,
		"fx_source":             fxSource,
		"phone_shared":          phoneShared,
	})
}

// hwOrderAnswer is one settled top-up as the payment history shows it. The
// amounts are in minor units of Currency. BonusMinor is the extra credit the
// order earned, in the same currency; zero means no bonus.
type hwOrderAnswer struct {
	MerchantRef  string `json:"merchant_ref"`
	Status       string `json:"status"`
	Currency     string `json:"currency"`
	ListMinor    int64  `json:"list_minor"`
	PayableMinor int64  `json:"payable_minor"`
	BonusMinor   int64  `json:"bonus_minor"`
	BonusQuota   int    `json:"bonus_quota"`
	CreatedAt    int64  `json:"created_at"`
	PaidAt       int64  `json:"paid_at"`
}

// HwListUserOrders answers the paid and refunded top-ups of the logged-in user,
// newest first, with the bonus each one earned.
func HwListUserOrders(c *gin.Context) {
	orders, err := model.HwListUserOrders(c.GetInt("id"), 50)
	if err != nil {
		logger.LogError(c.Request.Context(), common.LogText("listing top-up orders failed"))
		common.ApiError(c, common.NewMessage("Failed to read the payment history"))
		return
	}
	answers := make([]hwOrderAnswer, 0, len(orders))
	for _, order := range orders {
		answers = append(answers, hwOrderAnswer{
			MerchantRef:  order.MerchantRef,
			Status:       order.Status,
			Currency:     order.Currency,
			ListMinor:    order.ListValueTryMinor,
			PayableMinor: order.PayableTryMinor,
			BonusMinor:   order.BonusMinor,
			BonusQuota:   order.BonusQuota,
			CreatedAt:    order.CreatedAt,
			PaidAt:       order.PaidAt,
		})
	}
	common.ApiSuccess(c, gin.H{"orders": answers})
}
