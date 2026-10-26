package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHwNoteOrderOnlyAnnotatesOpenOrders(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&HwOrder{}))
	require.NoError(t, DB.Exec("DELETE FROM hw_orders").Error)
	t.Cleanup(func() { DB.Exec("DELETE FROM hw_orders") })

	newOrder := func(ref, status string) {
		require.NoError(t, DB.Create(&HwOrder{
			UserId: 1, MerchantRef: ref, ListQuota: 100, ListValueTryMinor: 100, PayableTryMinor: 100,
			Currency: "EUR", DiscountKind: HwDiscountReturning, Status: status, CreatedAt: 1_800_000_000,
		}).Error)
	}
	newOrder("ref-created", HwOrderStatusCreated)
	newOrder("ref-pending", HwOrderStatusPending)
	newOrder("ref-paid", HwOrderStatusPaid)
	note := func(ref string) string {
		var order HwOrder
		require.NoError(t, DB.Where("merchant_ref = ?", ref).First(&order).Error)
		return order.AdminNote
	}

	require.ErrorIs(t, HwNoteOrder("  ", "why"), ErrHwInvalidOrder)
	require.ErrorIs(t, HwNoteOrder("ref-created", "   "), ErrHwInvalidOrder)

	require.NoError(t, HwNoteOrder("ref-created", "check this"))
	require.NoError(t, HwNoteOrder(" ref-pending ", "and this"))
	assert.Equal(t, "check this", note("ref-created"))
	assert.Equal(t, "and this", note("ref-pending"))

	// A settled order keeps its settlement note, and an unknown reference is a
	// silent no-op rather than an error.
	require.NoError(t, HwNoteOrder("ref-paid", "late note"))
	require.NoError(t, HwNoteOrder("ref-missing", "nobody"))
	assert.Empty(t, note("ref-paid"))
}
