package model

import (
	"bytes"
	"fmt"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hwCreditTestSetup starts a test from an empty credit ledger. The shared test
// database is created once in TestMain, so the credit tables are migrated here.
func hwCreditTestSetup(t *testing.T) {
	t.Helper()
	t.Setenv(hwSignalSaltEnv, "test-only-signal-salt")
	require.NoError(t, DB.AutoMigrate(&HwCreditPackage{}, &HwUserSignal{}, &HwFirstPurchaseClaim{}, &HwFreeCreditClaim{}))
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

// hwCaptureErrorLog redirects the server error log into a buffer for the length
// of one assertion block. The model package logs through common.SysError, which
// writes to gin.DefaultErrorWriter behind a lock the swap respects.
func hwCaptureErrorLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	common.LogWriterMu.Lock()
	previous := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = buf
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultErrorWriter = previous
		common.LogWriterMu.Unlock()
	})
	return buf
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

	logBuf := hwCaptureErrorLog(t)
	retired, err := RetireExpiredPackages(now)
	require.Error(t, err, "the first error is still returned to the caller")
	assert.Contains(t, err.Error(), "hw-test-blocked-deduction", "the caller still gets the error text")
	assert.Equal(t, 1, retired, "the user after the failure is still retired")
	assert.Equal(t, 60, getUserQuotaFromDB(t, later.Id), "the later user's balance is lowered")
	assert.Equal(t, 90, getUserQuotaFromDB(t, blocked.Id), "the blocked user's balance is untouched")

	logs := logBuf.String()
	assert.Contains(t, logs, "credit package expiry failed for 1 users")
	assert.Contains(t, logs, "first class internal")
	assert.NotContains(t, logs, "hw-test-blocked-deduction", "the error text never reaches the log")

	// The periodic job wraps the same run and used to print the error text.
	logBuf.Reset()
	runHwCreditExpiryOnceSafely()
	assert.NotContains(t, logBuf.String(), "hw-test-blocked-deduction", "the job log also stays without the error text")
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
