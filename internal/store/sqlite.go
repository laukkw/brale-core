package store

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func OpenSQLite(path string) (*gorm.DB, error) {
	return gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
}

type MigrateOptions struct {
	Full bool
}

func Migrate(db *gorm.DB, opts MigrateOptions) error {
	if err := migrateSchema(db, opts); err != nil {
		return err
	}
	return ensureOpenPositionUniqueIndex(db)
}

func migrateMinimal(db *gorm.DB) error {
	return migratePositionTables(db)
}

func migrateFull(db *gorm.DB) error {
	return migratePositionTables(db)
}

func migrateSchema(db *gorm.DB, opts MigrateOptions) error {
	if opts.Full {
		return migrateFull(db)
	}
	return migrateMinimal(db)
}

func migratePositionTables(db *gorm.DB) error {
	return db.AutoMigrate(
		&AgentEventRecord{},
		&ProviderEventRecord{},
		&GateEventRecord{},
		&RiskPlanHistoryRecord{},
		&PositionRecord{},
	)
}

func ensureOpenPositionUniqueIndex(db *gorm.DB) error {
	stmt := `CREATE UNIQUE INDEX IF NOT EXISTS idx_position_symbol_open
ON position_records(symbol)
WHERE status IN (
	'OPEN_SUBMITTING',
	'OPEN_PENDING',
	'OPEN_ABORTING',
	'OPEN_ACTIVE',
	'CLOSE_ARMED',
	'CLOSE_SUBMITTING',
	'CLOSE_PENDING'
);`
	return db.Exec(stmt).Error
}
