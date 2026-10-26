package model

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A pasted summary is stored at the length the bureau card keeps, so the
// stored row and the board card never drift apart.
func TestHwCreateAdminEscalationCapsSummary(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&HwAdminEscalation{}))

	row, err := HwCreateAdminEscalation(db, "Slow page", "  "+strings.Repeat("ç", 10000)+"  ", "/console", 1)
	require.NoError(t, err)

	var stored HwAdminEscalation
	require.NoError(t, db.First(&stored, row.ID).Error)
	assert.Equal(t, hwAdminEscalationSummaryMaxRunes, len([]rune(stored.Summary)))
	assert.Equal(t, strings.Repeat("ç", hwAdminEscalationSummaryMaxRunes), stored.Summary)
}

// An escalation waits in the pending list until the board answers with a card;
// once the card reference is stored it leaves the list, and an unknown id is an
// error instead of a silent no-op.
func TestHwAdminEscalationLeavesThePendingListOnceItHasACard(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&HwAdminEscalation{}))

	first, err := HwCreateAdminEscalation(db, "First", "a", "/a", 1)
	require.NoError(t, err)
	second, err := HwCreateAdminEscalation(db, "Second", "b", "/b", 1)
	require.NoError(t, err)

	pending, err := HwListPendingAdminEscalations(db)
	require.NoError(t, err)
	require.Len(t, pending, 2)
	assert.Equal(t, first.ID, pending[0].ID, "oldest first")

	require.NoError(t, HwSetAdminEscalationCard(db, first.ID, strings.Repeat("c", 100)))

	pending, err = HwListPendingAdminEscalations(db)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, second.ID, pending[0].ID)

	var stored HwAdminEscalation
	require.NoError(t, db.First(&stored, first.ID).Error)
	assert.Equal(t, strings.Repeat("c", 64), stored.CardRef, "card reference is capped at the column size")

	assert.EqualError(t, HwSetAdminEscalationCard(db, 9999, "x"), "escalation not found")
}
