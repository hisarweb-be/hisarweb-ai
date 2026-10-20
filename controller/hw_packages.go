package controller

// HisarWeb AI credit packages: the balance a user holds, broken down per package
// with the expiry date of each. The wallet page only shows the total, so the
// top-up page needs this route to tell a customer what expires when.

import (
	"cmp"
	"errors"
	"net/http"
	"slices"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"

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
	common.ApiSuccess(c, gin.H{
		"packages":              packages,
		"total_remaining_quota": total,
		"quota_per_unit":        common.QuotaPerUnit,
		"try_per_usd":           tryPerUsd,
		"fx_date":               fxDate,
		"fx_source":             fxSource,
	})
}
