package reconcile

import (
	"testing"

	"brale-core/internal/store"

	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dir := t.TempDir()
	path := dir + "/brale-core.db"
	db, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := store.Migrate(db, store.MigrateOptions{Full: true}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}
