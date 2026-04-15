// Package pgstore implements store.Store backed by PostgreSQL via pgx/v5.
package pgstore

import (
	"context"
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"brale-core/internal/store"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// PGStore implements store.Store using pgx/v5 against PostgreSQL.
type PGStore struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

var _ store.Store = (*PGStore)(nil)

// New creates a PGStore backed by the given pgxpool.
func New(pool *pgxpool.Pool, logger *zap.Logger) *PGStore {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &PGStore{pool: pool, logger: logger}
}

// Pool returns the underlying pgx pool for use by River or other subsystems.
func (s *PGStore) Pool() *pgxpool.Pool { return s.pool }

// Close closes the pool. Caller must invoke on shutdown.
func (s *PGStore) Close() { s.pool.Close() }

// RunMigrations applies all pending up migrations.
func RunMigrations(connStr string, logger *zap.Logger) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("open migrations source: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, connStr)
	if err != nil {
		return fmt.Errorf("create migrate instance: %w", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("apply migrations: %w", err)
	}
	srcErr, dbErr := m.Close()
	if srcErr != nil {
		return fmt.Errorf("close migration source: %w", srcErr)
	}
	if dbErr != nil {
		return fmt.Errorf("close migration db: %w", dbErr)
	}
	if logger != nil {
		logger.Info("database migrations applied")
	}
	return nil
}

// OpenPool creates and validates a pgxpool.Pool from a DSN.
func OpenPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse pg dsn: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pg pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping pg: %w", err)
	}
	return pool, nil
}

// queryRow is a convenience wrapper.
func (s *PGStore) queryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return s.pool.QueryRow(ctx, sql, args...)
}

// query is a convenience wrapper.
func (s *PGStore) query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return s.pool.Query(ctx, sql, args...)
}

// exec is a convenience wrapper.
func (s *PGStore) exec(ctx context.Context, sql string, args ...any) error {
	_, err := s.pool.Exec(ctx, sql, args...)
	return err
}
