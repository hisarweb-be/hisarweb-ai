package model

import (
	"cmp"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Credit package sources. Every grant of balance goes through
// GrantCreditPackage and is recorded with one of these sources.
const (
	HwSourcePurchase  = "purchase"
	HwSourceFree      = "free"
	HwSourceAffiliate = "affiliate"
	HwSourceAdmin     = "admin"
)

const (
	hwCreditPackageValiditySeconds = 30 * 24 * 3600
	hwFreeCreditValiditySeconds    = 14 * 24 * 3600
	hwCreditExpiryJobInterval      = time.Hour

	// Group names owned by this fork. Everything else (default, admin groups,
	// operator groups) is left untouched by the package code.
	hwGroupDefault = "default"
	hwGroupFree    = "free"
	hwGroupPaid    = "paid"
)

var (
	// ErrHwInvalidCreditGrant rejects a grant that would break the ledger
	// invariants: unknown source, non-positive quota, empty order reference.
	ErrHwInvalidCreditGrant = errors.New("invalid credit package grant")
)

// HwCreditPackage is one issued credit package. The remaining quota per package
// is derived from users.quota (see HwPackageRemaining); only retirement is
// persisted, because it is the one transition that changes the balance itself.
type HwCreditPackage struct {
	Id               int    `json:"id" gorm:"primaryKey"`
	UserId           int    `json:"user_id" gorm:"not null;index"`
	Source           string `json:"source" gorm:"type:varchar(32);not null;uniqueIndex:idx_hw_credit_packages_source_order_ref,priority:1"`
	GrantedQuota     int    `json:"granted_quota" gorm:"not null"`
	CreatedAt        int64  `json:"created_at" gorm:"not null;default:0"`
	ExpiresAt        int64  `json:"expires_at" gorm:"not null;index"`
	OrderRef         string `json:"order_ref" gorm:"type:varchar(128);not null;uniqueIndex:idx_hw_credit_packages_source_order_ref,priority:2"`
	RetiredAt        int64  `json:"retired_at" gorm:"not null;default:0"`
	RetiredRemaining int    `json:"retired_remaining" gorm:"not null;default:0"`
}

func (HwCreditPackage) TableName() string {
	return "hw_credit_packages"
}

// HwPackageView is one package together with the quota the derived fill assigns
// to it for the user's current balance.
type HwPackageView struct {
	Id             int    `json:"id"`
	UserId         int    `json:"user_id"`
	Source         string `json:"source"`
	GrantedQuota   int    `json:"granted_quota"`
	RemainingQuota int    `json:"remaining_quota"`
	CreatedAt      int64  `json:"created_at"`
	ExpiresAt      int64  `json:"expires_at"`
}

func isValidHwCreditSource(source string) bool {
	switch source {
	case HwSourcePurchase, HwSourceFree, HwSourceAffiliate, HwSourceAdmin:
		return true
	default:
		return false
	}
}

// GrantCreditPackage issues a credit package and raises users.quota by the same
// amount inside one transaction. It is idempotent on (source, orderRef): a
// second call for the same issuance changes nothing and is not an error.
//
// When tx is nil the function owns the transaction and syncs the user quota
// cache after commit (and the group cache for a purchase). A caller that passes
// its own tx must commit it and then call syncCreditUserQuotaCache itself,
// otherwise the cached balance stays behind until it is rehydrated.
func GrantCreditPackage(tx *gorm.DB, userID int, quota int, source string, orderRef string, now int64) (bool, error) {
	if userID <= 0 || quota <= 0 || !isValidHwCreditSource(source) || orderRef == "" || now <= 0 {
		return false, ErrHwInvalidCreditGrant
	}
	if err := common.ValidateWalletQuota(quota); err != nil {
		return false, err
	}
	if tx != nil {
		return grantCreditPackageTx(tx, userID, quota, source, orderRef, now)
	}
	var granted bool
	err := DB.Transaction(func(inner *gorm.DB) error {
		var err error
		granted, err = grantCreditPackageTx(inner, userID, quota, source, orderRef, now)
		return err
	})
	if err != nil {
		return false, err
	}
	if granted {
		syncCreditUserQuotaCache(userID, quota, "credit package")
		if source == HwSourcePurchase {
			if err := RefreshUserGroupCache(userID); err != nil {
				common.SysError(common.LogText("failed to refresh user group cache after paid purchase for user %d: %v", userID, err))
			}
		}
	}
	return granted, nil
}

func grantCreditPackageTx(tx *gorm.DB, userID int, quota int, source string, orderRef string, now int64) (bool, error) {
	validity := int64(hwCreditPackageValiditySeconds)
	if source == HwSourceFree {
		validity = hwFreeCreditValiditySeconds
	}
	pkg := HwCreditPackage{
		UserId:       userID,
		Source:       source,
		GrantedQuota: quota,
		CreatedAt:    now,
		ExpiresAt:    now + validity,
		OrderRef:     orderRef,
	}
	// Rely on the unique index instead of a read-then-write check: two racing
	// grants for the same issuance must not both pass. ON CONFLICT DO NOTHING is
	// emitted by all three supported dialects; a conflicting row yields zero
	// affected rows, which is exactly the "already issued" signal.
	result := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "source"}, {Name: "order_ref"}},
		DoNothing: true,
	}).Create(&pkg)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 0 {
		return false, nil
	}
	if err := increaseUserQuotaInTx(tx, userID, quota); err != nil {
		return false, err
	}
	if source == HwSourcePurchase {
		if err := HwPromoteToPaidGroup(tx, userID); err != nil {
			return false, err
		}
	}
	return true, nil
}

// increaseUserQuotaInTx mirrors increaseUserQuota but runs on the caller's
// transaction, so a package grant and the balance it credits commit together.
func increaseUserQuotaInTx(tx *gorm.DB, id int, quota int) error {
	result := tx.Model(&User{}).
		Where("id = ? AND quota <= ?", id, common.MaxWalletQuota-quota).
		Update("quota", gorm.Expr("quota + ?", quota))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var count int64
	if err := tx.Model(&User{}).Where("id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}
	return ErrWalletQuotaLimitExceeded
}

// HwPromoteToPaidGroup moves a regular user to the paid group. Only the
// upstream default and the free group are upgraded; administrators and users in
// an operator-assigned group are left alone.
func HwPromoteToPaidGroup(tx *gorm.DB, userID int) error {
	if tx == nil {
		tx = DB
	}
	return tx.Model(&User{}).
		Where("id = ? AND role = ? AND "+commonGroupCol+" IN ?", userID, common.RoleCommonUser, []string{hwGroupDefault, hwGroupFree}).
		Update("group", hwGroupPaid).Error
}

// HwPackageRemaining derives the remaining quota per active package from the
// user's current balance.
//
// Active packages are filled backwards, latest expiry first (ties broken by
// highest id first), each up to GrantedQuota. The package that expires first is
// therefore the one that stays empty first, which makes "spend from the earliest
// expiring package" a property of the balance itself instead of a mutation hook.
// Balance above the sum of all active packages (legacy or manually adjusted
// quota) belongs to no package and never expires.
//
// The returned views follow the fill order: latest expiry first.
func HwPackageRemaining(userID int, quotaNow int, pkgs []HwCreditPackage) []HwPackageView {
	active := make([]HwCreditPackage, 0, len(pkgs))
	for _, pkg := range pkgs {
		if pkg.RetiredAt == 0 && pkg.GrantedQuota > 0 {
			active = append(active, pkg)
		}
	}
	slices.SortFunc(active, func(a, b HwCreditPackage) int {
		if order := cmp.Compare(b.ExpiresAt, a.ExpiresAt); order != 0 {
			return order
		}
		return cmp.Compare(b.Id, a.Id)
	})

	views := make([]HwPackageView, 0, len(active))
	left := max(quotaNow, 0)
	for _, pkg := range active {
		remaining := min(pkg.GrantedQuota, left)
		left -= remaining
		views = append(views, HwPackageView{
			Id:             pkg.Id,
			UserId:         userID,
			Source:         pkg.Source,
			GrantedQuota:   pkg.GrantedQuota,
			RemainingQuota: remaining,
			CreatedAt:      pkg.CreatedAt,
			ExpiresAt:      pkg.ExpiresAt,
		})
	}
	return views
}

// ListUserCreditPackages returns the active packages of a user with their
// derived remaining quota, read from the database (never from the balance cache).
func ListUserCreditPackages(userID int) ([]HwPackageView, error) {
	if userID <= 0 {
		return nil, ErrHwInvalidCreditGrant
	}
	var user User
	if err := DB.Select("id", "quota").Where("id = ?", userID).First(&user).Error; err != nil {
		return nil, err
	}
	var pkgs []HwCreditPackage
	if err := DB.Where("user_id = ? AND retired_at = 0", userID).Find(&pkgs).Error; err != nil {
		return nil, err
	}
	return HwPackageRemaining(userID, user.Quota, pkgs), nil
}

// RetireExpiredPackages retires every expired active package and lowers the
// owning balances by the quota those packages still hold. It returns the number
// of retired packages. Running it twice changes nothing.
func RetireExpiredPackages(now int64) (int, error) {
	if now <= 0 {
		return 0, ErrHwInvalidCreditGrant
	}
	var userIDs []int
	if err := DB.Model(&HwCreditPackage{}).
		Where("retired_at = 0 AND expires_at <= ?", now).
		Distinct().
		Pluck("user_id", &userIDs).Error; err != nil {
		return 0, err
	}
	slices.Sort(userIDs)
	total := 0
	for _, userID := range userIDs {
		retired, err := retireExpiredPackagesForUser(userID, now)
		if err != nil {
			return total, err
		}
		total += retired
	}
	return total, nil
}

// RetireExpiredPackagesForUser is the lazy path for a single user: it runs the
// same retirement the periodic job runs.
func RetireExpiredPackagesForUser(userID int, now int64) error {
	_, err := retireExpiredPackagesForUser(userID, now)
	return err
}

func retireExpiredPackagesForUser(userID int, now int64) (int, error) {
	if userID <= 0 || now <= 0 {
		return 0, ErrHwInvalidCreditGrant
	}
	retired := 0
	decrease := 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Select("id", "quota").Where("id = ?", userID).First(&user).Error; err != nil {
			return err
		}
		var pkgs []HwCreditPackage
		if err := tx.Where("user_id = ? AND retired_at = 0", userID).Find(&pkgs).Error; err != nil {
			return err
		}
		// Derive the fill over every active package, not only the expiring ones:
		// the expiring package holds whatever is left after the later ones.
		remaining := make(map[int]int, len(pkgs))
		for _, view := range HwPackageRemaining(userID, user.Quota, pkgs) {
			remaining[view.Id] = view.RemainingQuota
		}
		expiringQuota := 0
		for _, pkg := range pkgs {
			if pkg.ExpiresAt > now {
				continue
			}
			rest := remaining[pkg.Id]
			expiringQuota += rest
			if err := tx.Model(&HwCreditPackage{}).
				Where("id = ? AND retired_at = 0", pkg.Id).
				UpdateColumns(map[string]any{"retired_at": now, "retired_remaining": rest}).Error; err != nil {
				return err
			}
			retired++
		}
		if retired == 0 {
			return nil
		}
		// An overdrawn balance keeps its negative value: only positive balance can
		// be lowered, and never below zero.
		decrease = min(expiringQuota, max(user.Quota, 0))
		if decrease == 0 {
			return nil
		}
		return tx.Model(&User{}).Where("id = ?", userID).Update("quota", user.Quota-decrease).Error
	})
	if err != nil {
		return 0, err
	}
	if decrease > 0 {
		if err := cacheDecrUserQuota(userID, int64(decrease)); err != nil {
			common.SysError(common.LogText("failed to decrease user quota cache after credit package expiry: %s", err.Error()))
		}
	}
	return retired, nil
}

var hwCreditExpiryOnce sync.Once

// StartHwCreditExpiryJob starts the hourly credit package expiry job. It runs
// once per process and only on the master node; the job itself is idempotent, so
// multiple masters would be safe as well.
func StartHwCreditExpiryJob() {
	hwCreditExpiryOnce.Do(func() {
		if _, err := HwSignalSalt(); err != nil {
			common.SysError(common.LogText("HW_SIGNAL_SALT is missing: abuse signals are not stored, so the free trial credit and the affiliate reward refuse to run; set HW_SIGNAL_SALT and restart"))
		}
		if !common.IsMasterNode {
			return
		}
		go func() {
			common.SysLog(common.LogText("credit package expiry job started: interval=%s", hwCreditExpiryJobInterval))
			runHwCreditExpiryOnceSafely()
			ticker := time.NewTicker(hwCreditExpiryJobInterval)
			defer ticker.Stop()
			for range ticker.C {
				runHwCreditExpiryOnceSafely()
			}
		}()
	})
}

func runHwCreditExpiryOnceSafely() {
	defer func() {
		if r := recover(); r != nil {
			common.SysError(common.LogText("credit package expiry job panicked: %v", r))
		}
	}()
	retired, err := RetireExpiredPackages(common.GetTimestamp())
	if err != nil {
		common.SysError(common.LogText("credit package expiry job failed: %s", err.Error()))
		return
	}
	if retired > 0 {
		common.SysLog(common.LogText("credit package expiry job retired %d packages", retired))
	}
}
