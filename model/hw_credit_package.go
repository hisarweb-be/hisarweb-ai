// HisarWeb AI credit ledger: known limits, recorded so a later change does not
// have to rediscover them.
//
//   - L1, pre-consume around the hourly boundary: a request that starts just
//     before the expiry run is priced against the balance it read, and the run
//     can retire the package it is spending from before settlement. users.quota
//     stays authoritative (settlement writes a delta), so nothing is charged
//     twice, but the very last part of an expiring package can be used once more
//     than its derived remainder.
//   - L3, upstream cache pattern: the user cache is a copy of users.quota that is
//     adjusted with deltas after a commit, like every upstream credit path. Two
//     nodes syncing the same user at the same moment can briefly serve a value
//     one delta behind; the next rehydration is authoritative.
//   - L8, MySQL: the connection must not enable clientFoundRows. Several checks
//     here read "no rows affected" as a signal (a re-used order reference, a
//     refused deduction, an already retired package), and that flag makes a
//     matched-but-unchanged UPDATE report a row, which would hide those cases.
package model

import (
	"cmp"
	"errors"
	"slices"
	"strings"
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
	// HwSourceBonus is the extra credit a large enough top-up earns (see
	// HwOrder.BonusQuota). It is booked next to the purchase package under the
	// same order reference, so a repeated webhook cannot book it twice and a
	// refund of the order takes it back.
	HwSourceBonus = "bonus"
)

// HwCreditValidityDays is how long a paid top-up keeps the whole balance valid.
// The public status answer announces it, so the pages never hardcode the number.
const HwCreditValidityDays = 30

const (
	hwCreditPackageValiditySeconds = HwCreditValidityDays * 24 * 3600
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

	// ErrHwOrderRefConflict rejects a second, different issuance under an order
	// reference that already belongs to another user or another amount.
	ErrHwOrderRefConflict = errors.New("order reference already granted with another user or amount")

	// ErrHwQuotaDeductionRefused reports a deduction that would take the balance
	// below zero. It cannot happen while the user row is locked; the condition is
	// the portable safety net for a path that forgot to lock.
	ErrHwQuotaDeductionRefused = errors.New("quota deduction refused: balance below the deducted amount")
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
	// ExtensionSeconds is the outage time added to ExpiresAt by the status
	// incidents (see hwAttachCreditExtensions). It is loaded from
	// hw_credit_extensions and never written on this row, so the extension stays
	// one fact per outage and package.
	ExtensionSeconds int64 `json:"extension_seconds" gorm:"-"`
}

// HwEffectiveExpiresAt is the expiry the customer actually has: the package's
// own expiry plus every outage extension. The derived fill, the retirement and
// the package list all compare against this value.
func (pkg HwCreditPackage) HwEffectiveExpiresAt() int64 {
	return pkg.ExpiresAt + pkg.ExtensionSeconds
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
	// ExpiresAt is the effective expiry, including the outage extensions.
	ExpiresAt int64 `json:"expires_at"`
}

func isValidHwCreditSource(source string) bool {
	switch source {
	case HwSourcePurchase, HwSourceFree, HwSourceAffiliate, HwSourceAdmin, HwSourceBonus:
		return true
	default:
		return false
	}
}

// GrantCreditPackage issues a credit package and raises users.quota by the same
// amount inside one transaction. It is idempotent on (source, orderRef): a
// second call for the same issuance changes nothing and is not an error.
//
// When tx is nil the function owns the transaction and syncs the user caches
// after commit (balance always, group for a purchase). A caller that passes its
// own tx must commit it and then call HwSyncCachesAfterGrant, otherwise the
// cached balance stays behind until it is rehydrated.
func GrantCreditPackage(tx *gorm.DB, userID int, quota int, source string, orderRef string, now int64) (bool, error) {
	if tx != nil {
		granted, _, err := GrantCreditPackageInTx(tx, userID, quota, source, orderRef, now)
		return granted, err
	}
	var granted bool
	var forfeited int
	err := DB.Transaction(func(inner *gorm.DB) error {
		var err error
		granted, forfeited, err = GrantCreditPackageInTx(inner, userID, quota, source, orderRef, now)
		return err
	})
	if err != nil {
		return false, err
	}
	if granted {
		HwSyncCachesAfterGrant(userID, quota, source)
		HwSyncCachesAfterFreeForfeit(userID, forfeited)
	}
	return granted, nil
}

// GrantCreditPackageInTx issues a package on the caller's transaction. Beside
// whether the grant happened it reports the free trial credit the grant
// forfeited: a purchase by a frozen account retires what is left of the free
// package, because the freeze blocks that credit and paying must not be the way
// around it. Paid credit is never touched. A caller outside this package must
// lower the cached balance by the forfeited amount after commit, next to
// HwSyncCachesAfterGrant.
func GrantCreditPackageInTx(tx *gorm.DB, userID int, quota int, source string, orderRef string, now int64) (granted bool, forfeited int, err error) {
	if tx == nil || userID <= 0 || quota <= 0 || !isValidHwCreditSource(source) || orderRef == "" || now <= 0 {
		return false, 0, ErrHwInvalidCreditGrant
	}
	if err := common.ValidateWalletQuota(quota); err != nil {
		return false, 0, err
	}
	return grantCreditPackageTx(tx, userID, quota, source, orderRef, now)
}

// HwSyncCachesAfterGrant brings the user caches in line with a credit grant that
// was just committed: the quota delta is added to the cached balance, and a
// purchase also refreshes the cached group, because the grant promotes a free
// user to paid.
//
// GrantCreditPackage calls this itself when it owns the transaction. A caller
// that passes its own tx must commit it and then call this function, otherwise
// the cached balance (and for a purchase the cached group) stays behind until
// the cache is rehydrated.
func HwSyncCachesAfterGrant(userID int, quota int, source string) {
	syncCreditUserQuotaCache(userID, quota, "credit package")
	if source != HwSourcePurchase {
		return
	}
	if err := RefreshUserGroupCache(userID); err != nil {
		common.SysError(common.LogText("failed to refresh user group cache after paid purchase for user %d: %v", userID, err))
	}
}

// HwSyncCachesAfterFreeForfeit lowers the cached balance by the free credit a
// purchase of a frozen account retired. It belongs directly after
// HwSyncCachesAfterGrant of the same purchase; the stored balance already
// reflects both changes.
func HwSyncCachesAfterFreeForfeit(userID int, quota int) {
	if quota <= 0 {
		return
	}
	if err := cacheDecrUserQuota(userID, int64(quota)); err != nil {
		common.SysLog(common.LogText("failed to sync forfeited free credit to user quota cache: %s", err.Error()))
	}
}

func grantCreditPackageTx(tx *gorm.DB, userID int, quota int, source string, orderRef string, now int64) (bool, int, error) {
	validity := int64(hwCreditPackageValiditySeconds)
	if source == HwSourceFree {
		validity = hwFreeCreditValiditySeconds
	}
	if source == HwSourceAdmin && strings.HasPrefix(orderRef, "channel") {
		validity = hwChannelValiditySeconds
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
		return false, 0, result.Error
	}
	if result.RowsAffected == 0 {
		// The conflict target exists already. Only an identical issuance is a quiet
		// repeat; another user or another amount under the same reference is a
		// caller error, not a second grant.
		var existing HwCreditPackage
		if err := tx.Where("source = ? AND order_ref = ?", source, orderRef).First(&existing).Error; err != nil {
			return false, 0, err
		}
		if existing.UserId != userID || existing.GrantedQuota != quota {
			return false, 0, ErrHwOrderRefConflict
		}
		return false, 0, nil
	}
	if err := increaseUserQuotaInTx(tx, userID, quota); err != nil {
		return false, 0, err
	}
	if source == HwSourcePurchase {
		if err := HwPromoteToPaidGroup(tx, userID); err != nil {
			return false, 0, err
		}
		if err := hwResetCreditValidity(tx, userID, now); err != nil {
			return false, 0, err
		}
		// Paying lifts the account out of the free group. A frozen account must
		// not carry the trial credit along, so the remaining free package is
		// retired here, in the same transaction as the purchase.
		forfeited, err := hwForfeitFreePackagesForFrozenAccount(tx, userID, now)
		if err != nil {
			return false, 0, err
		}
		return true, forfeited, nil
	}
	return true, 0, nil
}

// hwForfeitFreePackagesForFrozenAccount retires the active free packages of an
// account that is under review at the moment it pays. The remaining quota is
// derived the same way the expiry job derives it, so the amount taken from the
// balance is exactly what the fill assigned to the free package. Paid packages
// and any balance that belongs to no package are untouched. A purchase of an
// account that is not frozen changes nothing here.
func hwForfeitFreePackagesForFrozenAccount(tx *gorm.DB, userID int, now int64) (int, error) {
	if tx == nil || userID <= 0 {
		return 0, nil
	}
	var frozenRows int64
	if err := tx.Model(&HwAccountFlag{}).Where("user_id = ? AND frozen = ?", userID, true).Count(&frozenRows).Error; err != nil {
		return 0, err
	}
	if frozenRows == 0 {
		return 0, nil
	}
	var user User
	if err := tx.Select("id", "quota").Where("id = ?", userID).First(&user).Error; err != nil {
		return 0, err
	}
	var pkgs []HwCreditPackage
	if err := tx.Where("user_id = ? AND retired_at = 0", userID).Find(&pkgs).Error; err != nil {
		return 0, err
	}
	// The outage extensions move a package in the fill order, so the amount
	// forfeited here has to be the one the package list shows.
	if err := hwAttachCreditExtensions(tx, pkgs); err != nil {
		return 0, err
	}
	remaining := make(map[int]int, len(pkgs))
	for _, view := range HwPackageRemaining(userID, user.Quota, pkgs) {
		remaining[view.Id] = view.RemainingQuota
	}
	forfeited := 0
	for _, pkg := range pkgs {
		if pkg.Source != HwSourceFree {
			continue
		}
		rest := remaining[pkg.Id]
		forfeited += rest
		if err := tx.Model(&HwCreditPackage{}).
			Where("id = ? AND retired_at = 0", pkg.Id).
			UpdateColumns(map[string]any{"retired_at": now, "retired_remaining": rest}).Error; err != nil {
			return 0, err
		}
	}
	if forfeited == 0 {
		return 0, nil
	}
	if err := hwDecreaseUserQuota(tx, userID, forfeited); err != nil {
		return 0, err
	}
	return forfeited, nil
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
// A package whose expiry was extended by an outage is placed and filled at its
// effective expiry (ExpiresAt plus ExtensionSeconds).
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
		if order := cmp.Compare(b.HwEffectiveExpiresAt(), a.HwEffectiveExpiresAt()); order != 0 {
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
			ExpiresAt:      pkg.HwEffectiveExpiresAt(),
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
	if err := hwAttachCreditExtensions(DB, pkgs); err != nil {
		return nil, err
	}
	return HwPackageRemaining(userID, user.Quota, pkgs), nil
}

// HwUserHasActiveFreeCredit reports whether the user holds an active free
// trial package, the same rule ListUserCreditPackages applies, in one count
// instead of loading every package with its extensions.
func HwUserHasActiveFreeCredit(userID int) (bool, error) {
	if userID <= 0 {
		return false, ErrHwInvalidCreditGrant
	}
	var count int64
	err := DB.Model(&HwCreditPackage{}).
		Where("user_id = ? AND source = ? AND retired_at = 0 AND granted_quota > 0", userID, HwSourceFree).
		Count(&count).Error
	return count > 0, err
}

// RetireExpiredPackages retires every expired active package and lowers the
// owning balances by the quota those packages still hold. It returns the number
// of retired packages. Running it twice changes nothing.
//
// Each user is handled on its own: a user that fails (for example a deleted
// account) is logged with its id and error class only, the remaining users are
// still processed, and the first error is returned at the end. The closing log
// line counts the failed users and names the first error class; the error text
// itself never reaches the log, because a driver error can quote row data.
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
	failed := 0
	var firstErr error
	firstClass := ""
	for _, userID := range userIDs {
		retired, err := retireExpiredPackagesForUser(userID, now)
		if err != nil {
			if firstErr == nil {
				firstErr = err
				firstClass = hwErrorClass(err)
			}
			failed++
			common.SysError(common.LogText("credit package expiry failed for user %d (class %s); continuing with the next user", userID, hwErrorClass(err)))
			continue
		}
		total += retired
	}
	if failed > 0 {
		common.SysError(common.LogText("credit package expiry failed for %d users (first class %s); the first error is returned to the caller", failed, firstClass))
	}
	return total, firstErr
}

// hwErrorClass maps an error of the expiry run to its log class. Log lines carry
// the class and never the error text: a driver error can quote row data.
func hwErrorClass(err error) string {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return "record_not_found"
	case errors.Is(err, ErrWalletQuotaLimitExceeded):
		return "wallet_quota_limit"
	case errors.Is(err, ErrHwInvalidCreditGrant):
		return "invalid_grant"
	}
	return "internal"
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
	hwFlushPendingQuotaUpdates()
	retired := 0
	decrease := 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		userGone := false
		err := lockForUpdate(tx).Select("id", "quota").Where("id = ?", userID).First(&user).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			userGone = true
		} else if err != nil {
			return err
		}
		var pkgs []HwCreditPackage
		if err := tx.Where("user_id = ? AND retired_at = 0", userID).Find(&pkgs).Error; err != nil {
			return err
		}
		// The outage extensions are part of the expiry this run compares with, so
		// they are loaded before the fill assigns the remaining quota.
		if err := hwAttachCreditExtensions(tx, pkgs); err != nil {
			return err
		}
		// Derive the fill over every active package, not only the expiring ones:
		// the expiring package holds whatever is left after the later ones. A
		// deleted account has no balance to fill from, so its packages retire
		// with an empty remainder.
		remaining := make(map[int]int, len(pkgs))
		if !userGone {
			for _, view := range HwPackageRemaining(userID, user.Quota, pkgs) {
				remaining[view.Id] = view.RemainingQuota
			}
		}
		expiringQuota := 0
		for _, pkg := range pkgs {
			if pkg.HwEffectiveExpiresAt() > now {
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
		// A deleted account keeps its balance untouched: the packages are only
		// marked retired, because there is no account left to spend them.
		if retired == 0 || userGone {
			return nil
		}
		// An overdrawn balance keeps its negative value: only positive balance can
		// be lowered, and never below zero.
		decrease = min(expiringQuota, max(user.Quota, 0))
		if decrease == 0 {
			return nil
		}
		return hwDecreaseUserQuota(tx, userID, decrease)
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

// hwDecreaseUserQuota subtracts a quota delta from a balance and never goes
// below zero. It writes `quota - ?` and not an absolute value, so a balance
// change between the read and the write is not overwritten. The caller holds the
// user row with lockForUpdate, so `quota >= ?` holds on MySQL and PostgreSQL;
// SQLite has no row locks, and there the condition is the safety net: an
// impossible state becomes an error instead of a negative balance.
func hwDecreaseUserQuota(tx *gorm.DB, userID int, quota int) error {
	if quota <= 0 {
		return nil
	}
	result := tx.Model(&User{}).
		Where("id = ? AND quota >= ?", userID, quota).
		Update("quota", gorm.Expr("quota - ?", quota))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrHwQuotaDeductionRefused
	}
	return nil
}

// hwFlushPendingQuotaUpdates writes buffered quota deltas to the database before
// the ledger reads users.quota. With BATCH_UPDATE_ENABLED the consumption of the
// last interval still sits in memory; retiring a package against the stored
// balance would deduct that consumption a second time when the buffer flushes.
// Flushing always works here because the batch updater lives in this package; a
// node that does not hold the buffer flushes it on its own interval.
func hwFlushPendingQuotaUpdates() {
	if !common.BatchUpdateEnabled {
		return
	}
	common.SysLog(common.LogText("credit package expiry flushes the batch update buffer first"))
	batchUpdate()
}

var hwCreditExpiryOnce sync.Once

// StartHwCreditExpiryJob starts the hourly credit package expiry job. It runs
// once per process and only on the master node; the job itself is idempotent, so
// multiple masters would be safe as well.
func StartHwCreditExpiryJob() {
	hwCreditExpiryOnce.Do(func() {
		if _, err := HwSignalSalt(); err != nil {
			if errors.Is(err, ErrHwSignalSaltTooShort) {
				common.SysError(common.LogText("HW_SIGNAL_SALT is shorter than %d bytes: abuse signals are not stored, so the free trial credit and the affiliate reward refuse to run; set a longer HW_SIGNAL_SALT and restart", hwSignalSaltMinBytes))
			} else {
				common.SysError(common.LogText("HW_SIGNAL_SALT is missing: abuse signals are not stored, so the free trial credit and the affiliate reward refuse to run; set HW_SIGNAL_SALT and restart"))
			}
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
		// Already logged inside RetireExpiredPackages as a user count and an
		// error class. The text stays out of the log on purpose.
		return
	}
	if retired > 0 {
		common.SysLog(common.LogText("credit package expiry job retired %d packages", retired))
	}
}
