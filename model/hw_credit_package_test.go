package model

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hwCreditTestSetup starts a test from an empty credit ledger. The shared test
// database is created once in TestMain, so the credit tables are migrated here.
func hwCreditTestSetup(t *testing.T) {
	t.Helper()
	t.Setenv(hwSignalSaltEnv, hwTestSignalSalt)
	// The extension table belongs to the same fixture: the derived fill of a
	// package reads it, so every package path needs it migrated.
	require.NoError(t, DB.AutoMigrate(&HwCreditPackage{}, &HwUserSignal{}, &HwFirstPurchaseClaim{}, &HwFreeCreditClaim{}, &HwCreditExtension{}, &HwAccountFlag{}))
	require.NoError(t, DB.Exec("DELETE FROM hw_credit_packages").Error)
	require.NoError(t, DB.Exec("DELETE FROM hw_user_signals").Error)
	require.NoError(t, DB.Exec("DELETE FROM hw_first_purchase_claims").Error)
	require.NoError(t, DB.Exec("DELETE FROM hw_free_credit_claims").Error)
	t.Cleanup(func() {
		DB.Exec("DELETE FROM hw_credit_packages")
		DB.Exec("DELETE FROM hw_user_signals")
		DB.Exec("DELETE FROM hw_first_purchase_claims")
		DB.Exec("DELETE FROM hw_free_credit_claims")
	})
}

func createHwTestUser(t *testing.T, quota int) User {
	t.Helper()
	user := User{
		Username:    "hw-user-" + common.GetRandomString(8),
		Password:    "unused-password-hash",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       hwGroupDefault,
		AuthVersion: 1,
		Quota:       quota,
		AffCode:     "hwaff" + common.GetRandomString(8),
	}
	require.NoError(t, DB.Create(&user).Error)
	t.Cleanup(func() { DB.Unscoped().Delete(&User{}, user.Id) })
	return user
}

func hwTestPackageByOrderRef(t *testing.T, orderRef string) HwCreditPackage {
	t.Helper()
	var pkg HwCreditPackage
	require.NoError(t, DB.Where("order_ref = ?", orderRef).First(&pkg).Error)
	return pkg
}

func TestHwCreditPackageSpendCrossesPackageBoundary(t *testing.T) {
	hwCreditTestSetup(t)
	now := int64(1_700_000_000)
	user := createHwTestUser(t, 0)

	granted, err := GrantCreditPackage(nil, user.Id, 5, HwSourcePurchase, "order-a", now)
	require.NoError(t, err)
	require.True(t, granted)
	// A expires first, so it is drained first.
	pkgA := hwTestPackageByOrderRef(t, "order-a")

	granted, err = GrantCreditPackage(nil, user.Id, 10, HwSourcePurchase, "order-b", now+1)
	require.NoError(t, err)
	require.True(t, granted)
	pkgB := hwTestPackageByOrderRef(t, "order-b")
	require.Greater(t, pkgB.ExpiresAt, pkgA.ExpiresAt)
	require.Equal(t, 15, getUserQuotaFromDB(t, user.Id))

	require.NoError(t, DecreaseUserQuota(user.Id, 8, true))
	require.Equal(t, 7, getUserQuotaFromDB(t, user.Id))

	views, err := ListUserCreditPackages(user.Id)
	require.NoError(t, err)
	require.Len(t, views, 2)
	assert.Equal(t, pkgB.Id, views[0].Id)
	assert.Equal(t, 7, views[0].RemainingQuota, "the later package keeps what is left after the earlier one is drained")
	assert.Equal(t, pkgA.Id, views[1].Id)
	assert.Equal(t, 0, views[1].RemainingQuota, "the earliest expiring package is drained first")
}

func TestHwRetireExpiredPackages(t *testing.T) {
	now := int64(1_700_000_000)

	t.Run("lowers the balance by the remaining quota exactly once", func(t *testing.T) {
		hwCreditTestSetup(t)
		user := createHwTestUser(t, 60)
		_, err := GrantCreditPackage(nil, user.Id, 40, HwSourcePurchase, "order-exp", now)
		require.NoError(t, err)
		require.Equal(t, 100, getUserQuotaFromDB(t, user.Id))
		require.NoError(t, DB.Model(&HwCreditPackage{}).Where("user_id = ?", user.Id).Update("expires_at", now-1).Error)

		retired, err := RetireExpiredPackages(now)
		require.NoError(t, err)
		assert.Equal(t, 1, retired)
		assert.Equal(t, 60, getUserQuotaFromDB(t, user.Id))

		retired, err = RetireExpiredPackages(now)
		require.NoError(t, err)
		assert.Equal(t, 0, retired)
		assert.Equal(t, 60, getUserQuotaFromDB(t, user.Id))

		var pkg HwCreditPackage
		require.NoError(t, DB.Where("order_ref = ?", "order-exp").First(&pkg).Error)
		assert.Equal(t, now, pkg.RetiredAt)
		assert.Equal(t, 40, pkg.RetiredRemaining)
	})

	t.Run("a fully spent package changes nothing", func(t *testing.T) {
		hwCreditTestSetup(t)
		user := createHwTestUser(t, 0)
		_, err := GrantCreditPackage(nil, user.Id, 20, HwSourcePurchase, "order-spent", now)
		require.NoError(t, err)
		require.NoError(t, DecreaseUserQuota(user.Id, 20, true))
		require.NoError(t, DB.Model(&HwCreditPackage{}).Where("user_id = ?", user.Id).Update("expires_at", now-1).Error)

		retired, err := RetireExpiredPackages(now)
		require.NoError(t, err)
		assert.Equal(t, 1, retired)
		assert.Equal(t, 0, getUserQuotaFromDB(t, user.Id))

		var pkg HwCreditPackage
		require.NoError(t, DB.Where("order_ref = ?", "order-spent").First(&pkg).Error)
		assert.Equal(t, 0, pkg.RetiredRemaining)
	})

	t.Run("never lowers the balance below zero", func(t *testing.T) {
		hwCreditTestSetup(t)
		// The package claims more quota than the balance holds, so the derived
		// remainder equals the whole balance.
		user := createHwTestUser(t, 30)
		require.NoError(t, DB.Create(&HwCreditPackage{
			UserId:       user.Id,
			Source:       HwSourceAdmin,
			GrantedQuota: 200,
			CreatedAt:    now - 100,
			ExpiresAt:    now - 1,
			OrderRef:     "order-clamp",
		}).Error)

		retired, err := RetireExpiredPackages(now)
		require.NoError(t, err)
		assert.Equal(t, 1, retired)
		assert.Equal(t, 0, getUserQuotaFromDB(t, user.Id))

		var pkg HwCreditPackage
		require.NoError(t, DB.Where("order_ref = ?", "order-clamp").First(&pkg).Error)
		assert.Equal(t, 30, pkg.RetiredRemaining)
	})

	t.Run("keeps an overdrawn balance untouched", func(t *testing.T) {
		hwCreditTestSetup(t)
		user := createHwTestUser(t, -20)
		require.NoError(t, DB.Create(&HwCreditPackage{
			UserId:       user.Id,
			Source:       HwSourceAdmin,
			GrantedQuota: 40,
			CreatedAt:    now - 100,
			ExpiresAt:    now - 1,
			OrderRef:     "order-overdrawn",
		}).Error)

		retired, err := RetireExpiredPackages(now)
		require.NoError(t, err)
		assert.Equal(t, 1, retired)
		assert.Equal(t, -20, getUserQuotaFromDB(t, user.Id))
	})
}

func TestHwRetireExpiredPackagesKeepsGoingAfterAFailedUser(t *testing.T) {
	hwCreditTestSetup(t)
	now := int64(1_700_000_000)
	deleted := createHwTestUser(t, 50)
	later := createHwTestUser(t, 60)
	require.Greater(t, later.Id, deleted.Id, "the deleted account is processed first")

	_, err := GrantCreditPackage(nil, deleted.Id, 40, HwSourcePurchase, "order-deleted", now)
	require.NoError(t, err)
	_, err = GrantCreditPackage(nil, later.Id, 40, HwSourcePurchase, "order-later", now)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&HwCreditPackage{}).
		Where("user_id IN ?", []int{deleted.Id, later.Id}).
		Update("expires_at", now-1).Error)
	require.NoError(t, DB.Delete(&User{}, deleted.Id).Error)

	retired, err := RetireExpiredPackages(now)
	require.NoError(t, err, "a deleted account is not an error of the whole run")
	assert.Equal(t, 2, retired)
	assert.Equal(t, 60, getUserQuotaFromDB(t, later.Id), "the next user is still retired")

	var deletedPkg HwCreditPackage
	require.NoError(t, DB.Where("order_ref = ?", "order-deleted").First(&deletedPkg).Error)
	assert.Equal(t, now, deletedPkg.RetiredAt, "the package of the deleted account is retired")
	assert.Zero(t, deletedPkg.RetiredRemaining, "nothing was deducted from a deleted account")

	var deletedUser User
	require.NoError(t, DB.Unscoped().Select("quota").First(&deletedUser, deleted.Id).Error)
	assert.Equal(t, 90, deletedUser.Quota, "the deleted account's balance is not touched")
}

func TestHwRetireExpiredPackagesFlushesPendingBatchUpdates(t *testing.T) {
	hwCreditTestSetup(t)
	resetBatchUpdateTestState(t)
	now := int64(1_700_000_000)
	user := createHwTestUser(t, 0)
	_, err := GrantCreditPackage(nil, user.Id, 100, HwSourcePurchase, "order-batch", now)
	require.NoError(t, err)

	common.BatchUpdateEnabled = true
	require.NoError(t, DecreaseUserQuota(user.Id, 100, false))
	require.Equal(t, 100, getUserQuotaFromDB(t, user.Id), "the consumption is still in the batch buffer")
	require.NoError(t, DB.Model(&HwCreditPackage{}).Where("user_id = ?", user.Id).Update("expires_at", now-1).Error)

	retired, err := RetireExpiredPackages(now)
	require.NoError(t, err)
	assert.Equal(t, 1, retired)
	assert.Equal(t, 0, getUserQuotaFromDB(t, user.Id))
	assert.Equal(t, 0, getBufferedUserQuota(user.Id), "the buffer is flushed, not deducted twice")

	var pkg HwCreditPackage
	require.NoError(t, DB.Where("order_ref = ?", "order-batch").First(&pkg).Error)
	assert.Zero(t, pkg.RetiredRemaining, "the package was already spent before it expired")
}

// getBufferedUserQuota returns the quota delta still waiting in the batch buffer.
func getBufferedUserQuota(userID int) int {
	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	defer batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
	return batchUpdateStores[BatchUpdateTypeUserQuota][userID]
}

// hwServerLogCapture holds what the server log writers received while swapped.
type hwServerLogCapture struct {
	Out bytes.Buffer
	Err bytes.Buffer
}

// All returns everything both log writers received.
func (c *hwServerLogCapture) All() string {
	return c.Out.String() + c.Err.String()
}

// hwCaptureServerLog redirects both server log writers into buffers for the
// length of one test. common.SysLog writes to gin.DefaultWriter and
// common.SysError to gin.DefaultErrorWriter, both behind a lock the swap
// respects.
func hwCaptureServerLog(t *testing.T) *hwServerLogCapture {
	t.Helper()
	capture := &hwServerLogCapture{}
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

func TestHwRetireExpiredPackagesLogsCountAndClassWithoutErrorText(t *testing.T) {
	hwCreditTestSetup(t)
	now := int64(1_700_000_000)
	blocked := createHwTestUser(t, 50)
	later := createHwTestUser(t, 60)
	require.Greater(t, later.Id, blocked.Id, "the blocked account is processed first")
	_, err := GrantCreditPackage(nil, blocked.Id, 40, HwSourcePurchase, "order-blocked", now)
	require.NoError(t, err)
	_, err = GrantCreditPackage(nil, later.Id, 40, HwSourcePurchase, "order-later", now)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&HwCreditPackage{}).
		Where("user_id IN ?", []int{blocked.Id, later.Id}).
		Update("expires_at", now-1).Error)

	// A BEFORE UPDATE trigger makes the balance deduction of the first user fail
	// with a driver error whose text quotes this marker; the log must not.
	// SQLite triggers take no bound parameters, so the id is formatted in; it is
	// an integer the database itself produced.
	require.NoError(t, DB.Exec(fmt.Sprintf(
		"CREATE TRIGGER hw_test_block_quota BEFORE UPDATE OF quota ON users WHEN NEW.id = %d "+
			"BEGIN SELECT RAISE(ABORT, 'hw-test-blocked-deduction'); END", blocked.Id)).Error)
	t.Cleanup(func() { DB.Exec("DROP TRIGGER IF EXISTS hw_test_block_quota") })

	logBuf := hwCaptureServerLog(t)
	retired, err := RetireExpiredPackages(now)
	require.Error(t, err, "the first error is still returned to the caller")
	assert.Contains(t, err.Error(), "hw-test-blocked-deduction", "the caller still gets the error text")
	assert.Equal(t, 1, retired, "the user after the failure is still retired")
	assert.Equal(t, 60, getUserQuotaFromDB(t, later.Id), "the later user's balance is lowered")
	assert.Equal(t, 90, getUserQuotaFromDB(t, blocked.Id), "the blocked user's balance is untouched")

	logs := logBuf.Err.String()
	assert.Contains(t, logs, "credit package expiry failed for 1 users")
	assert.Contains(t, logs, "first class internal")
	assert.NotContains(t, logs, "hw-test-blocked-deduction", "the error text never reaches the log")

	// The failed user keeps a consistent state: the transaction rolls back, so
	// the package is not half-retired and the balance keeps its value.
	var blockedPkg HwCreditPackage
	require.NoError(t, DB.Where("order_ref = ?", "order-blocked").First(&blockedPkg).Error)
	assert.Zero(t, blockedPkg.RetiredAt, "a failed deduction retires nothing")

	// The periodic job wraps the same run and used to print the error text.
	logBuf.Err.Reset()
	runHwCreditExpiryOnceSafely()
	assert.NotContains(t, logBuf.Err.String(), "hw-test-blocked-deduction", "the job log also stays without the error text")
}

func TestHwGrantCreditPackageIsIdempotent(t *testing.T) {
	hwCreditTestSetup(t)
	now := int64(1_700_000_000)
	user := createHwTestUser(t, 0)

	first, err := GrantCreditPackage(nil, user.Id, 100, HwSourcePurchase, "order-once", now)
	require.NoError(t, err)
	require.True(t, first)
	second, err := GrantCreditPackage(nil, user.Id, 100, HwSourcePurchase, "order-once", now)
	require.NoError(t, err)
	require.False(t, second)
	assert.Equal(t, 100, getUserQuotaFromDB(t, user.Id))

	// SQLite in this suite runs on a single connection, so these goroutines queue
	// instead of writing concurrently. The case still pins the contract: ten
	// racing grants of one issuance credit the balance exactly once.
	var wg sync.WaitGroup
	results := make([]bool, 10)
	errs := make([]error, 10)
	for i := range 10 {
		wg.Go(func() {
			results[i], errs[i] = GrantCreditPackage(nil, user.Id, 50, HwSourceAdmin, "order-race", now)
		})
	}
	wg.Wait()

	granted := 0
	for i := range results {
		require.NoError(t, errs[i])
		if results[i] {
			granted++
		}
	}
	assert.Equal(t, 1, granted)
	assert.Equal(t, 150, getUserQuotaFromDB(t, user.Id))
}

func TestHwGrantCreditPackageRejectsOrderRefConflict(t *testing.T) {
	hwCreditTestSetup(t)
	now := int64(1_700_000_000)
	first := createHwTestUser(t, 0)
	second := createHwTestUser(t, 0)

	granted, err := GrantCreditPackage(nil, first.Id, 100, HwSourcePurchase, "order-shared", now)
	require.NoError(t, err)
	require.True(t, granted)

	granted, err = GrantCreditPackage(nil, second.Id, 100, HwSourcePurchase, "order-shared", now)
	require.ErrorIs(t, err, ErrHwOrderRefConflict, "another user under the same reference")
	assert.False(t, granted)
	assert.Equal(t, 0, getUserQuotaFromDB(t, second.Id))
	assert.Equal(t, 100, getUserQuotaFromDB(t, first.Id))

	granted, err = GrantCreditPackage(nil, first.Id, 150, HwSourcePurchase, "order-shared", now)
	require.ErrorIs(t, err, ErrHwOrderRefConflict, "another amount under the same reference")
	assert.False(t, granted)
	assert.Equal(t, 100, getUserQuotaFromDB(t, first.Id))

	granted, err = GrantCreditPackage(nil, first.Id, 100, HwSourcePurchase, "order-shared", now)
	require.NoError(t, err, "the identical issuance stays an idempotent repeat")
	assert.False(t, granted)
	assert.Equal(t, 100, getUserQuotaFromDB(t, first.Id))
}

func TestHwDecreaseUserQuotaSubtractsAndNeverGoesBelowZero(t *testing.T) {
	hwCreditTestSetup(t)
	user := createHwTestUser(t, 10)

	require.NoError(t, hwDecreaseUserQuota(DB, user.Id, 10))
	assert.Equal(t, 0, getUserQuotaFromDB(t, user.Id))

	err := hwDecreaseUserQuota(DB, user.Id, 1)
	require.ErrorIs(t, err, ErrHwQuotaDeductionRefused)
	assert.Equal(t, 0, getUserQuotaFromDB(t, user.Id), "an overdrawn deduction is refused, not written")
}

func TestHwPackageRemaining(t *testing.T) {
	packages := func() []HwCreditPackage {
		return []HwCreditPackage{
			{Id: 1, Source: HwSourcePurchase, GrantedQuota: 10, ExpiresAt: 300},
			{Id: 2, Source: HwSourcePurchase, GrantedQuota: 10, ExpiresAt: 300},
			{Id: 3, Source: HwSourcePurchase, GrantedQuota: 10, ExpiresAt: 300},
			{Id: 4, Source: HwSourcePurchase, GrantedQuota: 20, ExpiresAt: 200},
			{Id: 5, Source: HwSourceAffiliate, GrantedQuota: 40, ExpiresAt: 400, RetiredAt: 10, RetiredRemaining: 40},
		}
	}
	remainingOf := func(views []HwPackageView) map[int]int {
		remaining := make(map[int]int, len(views))
		for _, view := range views {
			remaining[view.Id] = view.RemainingQuota
		}
		return remaining
	}

	t.Run("no packages", func(t *testing.T) {
		assert.Empty(t, HwPackageRemaining(7, 50, nil))
	})

	t.Run("one package", func(t *testing.T) {
		views := HwPackageRemaining(7, 30, []HwCreditPackage{{Id: 1, GrantedQuota: 100, ExpiresAt: 300}})
		require.Len(t, views, 1)
		assert.Equal(t, 30, views[0].RemainingQuota)
		assert.Equal(t, 7, views[0].UserId)
	})

	t.Run("equal expiry fills by highest id first", func(t *testing.T) {
		views := HwPackageRemaining(7, 25, packages())
		assert.Equal(t, map[int]int{1: 5, 2: 10, 3: 10, 4: 0}, remainingOf(views))
	})

	t.Run("balance above the package sum never expires", func(t *testing.T) {
		views := HwPackageRemaining(7, 100, packages())
		assert.Equal(t, map[int]int{1: 10, 2: 10, 3: 10, 4: 20}, remainingOf(views))
	})

	t.Run("zero balance", func(t *testing.T) {
		views := HwPackageRemaining(7, 0, packages())
		assert.Equal(t, map[int]int{1: 0, 2: 0, 3: 0, 4: 0}, remainingOf(views))
	})

	t.Run("negative balance", func(t *testing.T) {
		views := HwPackageRemaining(7, -5, packages())
		assert.Equal(t, map[int]int{1: 0, 2: 0, 3: 0, 4: 0}, remainingOf(views))
	})
}

// A frozen account that pays must not carry its remaining free credit into the
// paid group: the freeze blocks the trial credit, and the purchase would
// otherwise be the way around it. Paid credit stays untouched.
func TestHwPurchaseByFrozenAccountForfeitsTheFreeCredit(t *testing.T) {
	hwCreditTestSetup(t)
	require.NoError(t, DB.AutoMigrate(&HwAccountFlag{}))
	require.NoError(t, DB.Exec("DELETE FROM hw_account_flags").Error)
	t.Cleanup(func() {
		DB.Exec("DELETE FROM hw_account_flags")
	})
	now := int64(1_700_000_000)

	t.Run("a frozen account keeps only what it paid for", func(t *testing.T) {
		user := createHwTestUser(t, 0)
		freeRef := fmt.Sprintf("free:%d", user.Id)
		granted, err := GrantCreditPackage(nil, user.Id, 100, HwSourceFree, freeRef, now)
		require.NoError(t, err)
		require.True(t, granted)

		frozen, err := FreezeHwAccounts([]int{user.Id}, HwFreezeReasonSharedPhone, now+10)
		require.NoError(t, err)
		require.Equal(t, 1, frozen)

		granted, err = GrantCreditPackage(nil, user.Id, 50, HwSourcePurchase, "order-frozen", now+20)
		require.NoError(t, err)
		require.True(t, granted)

		assert.Equal(t, 50, getUserQuotaFromDB(t, user.Id), "only the paid credit may be spent after the purchase")
		freePackage := hwTestPackageByOrderRef(t, freeRef)
		assert.Equal(t, now+20, freePackage.RetiredAt, "the free package is retired at the moment of payment")
		assert.Equal(t, 100, freePackage.RetiredRemaining)
		purchasePackage := hwTestPackageByOrderRef(t, "order-frozen")
		assert.Zero(t, purchasePackage.RetiredAt, "the paid package stays usable in full")

		var reloaded User
		require.NoError(t, DB.Where("id = ?", user.Id).First(&reloaded).Error)
		assert.Equal(t, hwGroupPaid, reloaded.Group)
	})

	t.Run("an account in good standing keeps its free credit", func(t *testing.T) {
		user := createHwTestUser(t, 0)
		freeRef := fmt.Sprintf("free:%d", user.Id)
		granted, err := GrantCreditPackage(nil, user.Id, 100, HwSourceFree, freeRef, now)
		require.NoError(t, err)
		require.True(t, granted)

		granted, err = GrantCreditPackage(nil, user.Id, 50, HwSourcePurchase, "order-clear", now+20)
		require.NoError(t, err)
		require.True(t, granted)

		assert.Equal(t, 150, getUserQuotaFromDB(t, user.Id))
		assert.Zero(t, hwTestPackageByOrderRef(t, freeRef).RetiredAt)
	})
}

// The same hole in the other order: an account that paid first and is frozen
// later must lose its remaining free credit, while an account that never paid
// keeps it so a thaw loses nothing.
func TestHwFreezeAfterPurchaseForfeitsTheFreeCredit(t *testing.T) {
	hwCreditTestSetup(t)
	require.NoError(t, DB.AutoMigrate(&HwAccountFlag{}))
	require.NoError(t, DB.Exec("DELETE FROM hw_account_flags").Error)
	t.Cleanup(func() {
		DB.Exec("DELETE FROM hw_account_flags")
	})
	now := int64(1_700_000_000)

	t.Run("a payer frozen later keeps only what it paid for", func(t *testing.T) {
		user := createHwTestUser(t, 0)
		freeRef := fmt.Sprintf("free:%d", user.Id)
		granted, err := GrantCreditPackage(nil, user.Id, 100, HwSourceFree, freeRef, now)
		require.NoError(t, err)
		require.True(t, granted)
		granted, err = GrantCreditPackage(nil, user.Id, 50, HwSourcePurchase, "order-first", now+10)
		require.NoError(t, err)
		require.True(t, granted)
		require.Equal(t, 150, getUserQuotaFromDB(t, user.Id))

		frozen, err := FreezeHwAccounts([]int{user.Id}, HwFreezeReasonSharedPhone, now+20)
		require.NoError(t, err)
		require.Equal(t, 1, frozen)

		assert.Equal(t, 50, getUserQuotaFromDB(t, user.Id), "only the paid credit may be spent after the freeze")
		freePackage := hwTestPackageByOrderRef(t, freeRef)
		assert.Equal(t, now+20, freePackage.RetiredAt)
		assert.Equal(t, 100, freePackage.RetiredRemaining)
	})

	t.Run("an account that never paid keeps its trial credit", func(t *testing.T) {
		user := createHwTestUser(t, 0)
		freeRef := fmt.Sprintf("free:%d", user.Id)
		granted, err := GrantCreditPackage(nil, user.Id, 100, HwSourceFree, freeRef, now)
		require.NoError(t, err)
		require.True(t, granted)

		frozen, err := FreezeHwAccounts([]int{user.Id}, HwFreezeReasonSharedPhone, now+20)
		require.NoError(t, err)
		require.Equal(t, 1, frozen)

		assert.Equal(t, 100, getUserQuotaFromDB(t, user.Id))
		assert.Equal(t, int64(0), hwTestPackageByOrderRef(t, freeRef).RetiredAt)
	})
}

// An outage extension moves a package's expiry, and with it the place that
// package takes in the derived fill. A refund and a forfeit must subtract the
// same amount the customer sees in the package list, so both have to read the
// extensions before they derive the fill.
func TestHwRefundAndForfeitCountTheOutageExtension(t *testing.T) {
	hwStatusTestSetup(t)
	hwCreditTestSetup(t)
	require.NoError(t, DB.AutoMigrate(&HwAccountFlag{}))
	require.NoError(t, DB.Exec("DELETE FROM hw_account_flags").Error)
	t.Cleanup(func() {
		DB.Exec("DELETE FROM hw_account_flags")
	})
	now := int64(1_700_000_000)
	// An outage extension of thirty days moves the package behind the one that
	// expires later, so the derived fill reaches it last instead of first.
	extend := func(t *testing.T, orderRef string) {
		t.Helper()
		pkg := hwTestPackageByOrderRef(t, orderRef)
		incident := hwStatusTestIncident(t, now-3600, 16*60)
		require.NoError(t, DB.Create(&HwCreditExtension{
			PackageId: pkg.Id, IncidentId: incident.Id, Minutes: 30 * 24 * 60, CreatedAt: now,
		}).Error)
	}
	// Three packages of 50 in order of expiry: paid, free, paid. Spending the
	// balance from 150 down to 60 drains the first package to 0 and leaves the
	// free package 10, unless the extension of the first package pushes it
	// behind the free one — then it holds 50 and the free package 0. The order
	// references carry a suffix, because the unique (source, order_ref) index
	// spans the whole test database.
	setup := func(t *testing.T, suffix string) (User, string, string) {
		t.Helper()
		user := createHwTestUser(t, 0)
		first := "order-ext-" + suffix
		free := "free:" + suffix
		_, err := GrantCreditPackage(nil, user.Id, 50, HwSourcePurchase, first, now)
		require.NoError(t, err)
		_, err = GrantCreditPackage(nil, user.Id, 50, HwSourceFree, free, now+1)
		require.NoError(t, err)
		_, err = GrantCreditPackage(nil, user.Id, 50, HwSourcePurchase, "order-later-"+suffix, now+2)
		require.NoError(t, err)
		require.NoError(t, DecreaseUserQuota(user.Id, 90, true))
		require.Equal(t, 60, getUserQuotaFromDB(t, user.Id))
		return user, first, free
	}
	remainingOf := func(t *testing.T, userID int, orderRef string) int {
		t.Helper()
		views, err := ListUserCreditPackages(userID)
		require.NoError(t, err)
		id := hwTestPackageByOrderRef(t, orderRef).Id
		for _, view := range views {
			if view.Id == id {
				return view.RemainingQuota
			}
		}
		t.Fatalf("package %s missing from the package list", orderRef)
		return 0
	}

	t.Run("refund subtracts what the package list shows", func(t *testing.T) {
		user, orderRef, _ := setup(t, "refund")
		extend(t, orderRef)
		want := remainingOf(t, user.Id, orderRef)
		require.Equal(t, 50, want, "the extended package keeps the quota the customer sees")

		revocation, err := HwRevokeOrderCredit(nil, orderRef, now+3)
		require.NoError(t, err)
		assert.True(t, revocation.Retired)
		assert.Equal(t, want, revocation.RemainingQuota)
		assert.Equal(t, 60-want, getUserQuotaFromDB(t, user.Id), "the balance drops by what the package still held")
	})

	t.Run("forfeit subtracts what the package list shows", func(t *testing.T) {
		user, orderRef, freeRef := setup(t, "forfeit")
		extend(t, orderRef)
		require.NoError(t, DB.Create(&HwAccountFlag{UserId: user.Id, Frozen: true, Reason: HwFreezeReasonSharedPhone, FrozenAt: now + 3}).Error)
		want := remainingOf(t, user.Id, freeRef)
		require.Equal(t, 0, want, "the extended package holds what the customer sees; the free package is empty")

		_, err := hwForfeitFreePackagesForFrozenAccount(DB, user.Id, now+4)
		require.NoError(t, err)
		assert.Equal(t, want, hwTestPackageByOrderRef(t, freeRef).RetiredRemaining)
		assert.Equal(t, 60-want, getUserQuotaFromDB(t, user.Id), "the balance drops by what the free package still held")
	})
}

func TestHwUserHasActiveFreeCredit(t *testing.T) {
	hwCreditTestSetup(t)
	user := createHwTestUser(t, 0)
	now := time.Now().Unix()

	_, err := HwUserHasActiveFreeCredit(0)
	require.ErrorIs(t, err, ErrHwInvalidCreditGrant)

	has, err := HwUserHasActiveFreeCredit(user.Id)
	require.NoError(t, err)
	assert.False(t, has)

	// A purchase package is not a free trial.
	require.NoError(t, DB.Create(&HwCreditPackage{UserId: user.Id, Source: HwSourcePurchase, GrantedQuota: 100, CreatedAt: now, ExpiresAt: now + 3600, OrderRef: "order-a"}).Error)
	has, err = HwUserHasActiveFreeCredit(user.Id)
	require.NoError(t, err)
	assert.False(t, has)

	free := HwCreditPackage{UserId: user.Id, Source: HwSourceFree, GrantedQuota: 100, CreatedAt: now, ExpiresAt: now + 3600, OrderRef: "free-a"}
	require.NoError(t, DB.Create(&free).Error)
	has, err = HwUserHasActiveFreeCredit(user.Id)
	require.NoError(t, err)
	assert.True(t, has)

	// A retired free package no longer counts, and another user's never does.
	require.NoError(t, DB.Model(&free).Update("retired_at", now).Error)
	has, err = HwUserHasActiveFreeCredit(user.Id)
	require.NoError(t, err)
	assert.False(t, has)
}
