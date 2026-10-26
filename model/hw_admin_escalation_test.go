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
