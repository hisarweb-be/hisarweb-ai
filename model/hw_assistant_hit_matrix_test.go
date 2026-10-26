package model

import (
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestHwAssistantRecordHitDatabaseMatrix counts the same key twice on every
// database the product runs on. PostgreSQL rejected the increment with
// "column reference \"hits\" is ambiguous" because ON CONFLICT DO UPDATE sees
// the column in the target row and in excluded; SQLite accepted it, so the
// controller test could never catch it. The table name has to stay the real
// one: the counter expression in hw_assistant_hit.go names the table.
func TestHwAssistantRecordHitDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var dialector gorm.Dialector
			switch dialect {
			case "sqlite":
				dialector = sqlite.Open("file:hw_assistant_hit_matrix?mode=memory&cache=shared")
			case "mysql":
				dsn := os.Getenv("TEST_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_MYSQL_DSN is not configured")
				}
				dialector = mysql.Open(dsn)
			case "postgres":
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_POSTGRES_DSN is not configured")
				}
				dialector = postgres.Open(dsn)
			}
			db, err := gorm.Open(dialector, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			previousDB, previousType := DB, common.MainDatabaseType()
			DB = db
			common.SetMainDatabaseType(common.DatabaseType(dialect))
			initCol()
			prefix := "q:matrix " + dialect
			key := prefix + " teller"
			t.Cleanup(func() {
				require.NoError(t, db.Where(commonKeyCol+" = ?", key).Delete(&HwAssistantHit{}).Error)
				DB = previousDB
				common.SetMainDatabaseType(previousType)
				initCol()
				require.NoError(t, sqlDB.Close())
			})
			require.NoError(t, db.AutoMigrate(&HwAssistantHit{}))
			require.NoError(t, db.Where(commonKeyCol+" = ?", key).Delete(&HwAssistantHit{}).Error)

			require.NoError(t, HwAssistantRecordHit(db, key, 100))
			require.NoError(t, HwAssistantRecordHit(db, key, 200), "the second hit updates the existing counter")

			rows, err := HwAssistantHitsByPrefix(db, "q:matrix "+dialect, 10)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			assert.Equal(t, 2, rows[0].Hits)
			assert.Equal(t, int64(200), rows[0].LastAt)
			assert.Equal(t, "teller", strings.TrimPrefix(rows[0].Key, " "))
		})
	}
}
