// Package pgstore implements store.Store backed by PostgreSQL via pgx/v5.
package pgstore

import (
	"context"
	"embed"
	"fmt"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/notifyport"
	"brale-core/internal/pgstore/queries"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"brale-core/internal/store"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// PGStore implements store.Store using pgx/v5 against PostgreSQL.
type PGStore struct {
	pool    *pgxpool.Pool
	queries *queries.Queries
	logger  *zap.Logger

	// dbOverride is a test seam for capturing query args without a live database.
	dbOverride queryer
}

var _ store.Store = (*PGStore)(nil)
var _ store.TxRunner = (*PGStore)(nil)

// New creates a PGStore backed by the given pgxpool.
func New(pool *pgxpool.Pool, logger *zap.Logger) *PGStore {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &PGStore{pool: pool, queries: queries.New(pool), logger: logger}
}

// Pool returns the underlying pgx pool for use by River or other subsystems.
func (s *PGStore) Pool() *pgxpool.Pool { return s.pool }

// Close closes the pool. Caller must invoke on shutdown.
func (s *PGStore) Close() { s.pool.Close() }

// RunMigrations applies all pending up migrations.
func RunMigrations(connStr string, logger *zap.Logger) error {
	return withMigrator(connStr, logger, func(m *migrate.Migrate) error {
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			return fmt.Errorf("apply migrations: %w", err)
		}
		return nil
	}, "database migrations applied")
}

func withMigrator(connStr string, logger *zap.Logger, fn func(*migrate.Migrate) error, successMsg string) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("open migrations source: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, connStr)
	if err != nil {
		return fmt.Errorf("create migrate instance: %w", err)
	}
	if err := fn(m); err != nil {
		srcErr, dbErr := m.Close()
		if srcErr != nil {
			return fmt.Errorf("%w; close migration source: %v", err, srcErr)
		}
		if dbErr != nil {
			return fmt.Errorf("%w; close migration db: %v", err, dbErr)
		}
		return err
	}
	srcErr, dbErr := m.Close()
	if srcErr != nil {
		return fmt.Errorf("close migration source: %w", srcErr)
	}
	if dbErr != nil {
		return fmt.Errorf("close migration db: %w", dbErr)
	}
	if logger != nil {
		logger.Info(successMsg)
	}
	return nil
}

// OpenPool creates and validates a pgxpool.Pool from a DSN.
func OpenPool(ctx context.Context, dbCfg config.DatabaseConfig) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dbCfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse pg dsn: %w", err)
	}
	if dbCfg.MaxOpenConns > 0 {
		cfg.MaxConns = int32(dbCfg.MaxOpenConns)
	}
	if dbCfg.MaxIdleConns > 0 {
		cfg.MinConns = int32(dbCfg.MaxIdleConns)
	}
	cfg.MaxConnIdleTime = 5 * time.Minute
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

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func (s *PGStore) db(ctx context.Context) queryer {
	if tx := notifyport.TxFromContext(ctx); tx != nil {
		return tx
	}
	if s.dbOverride != nil {
		return s.dbOverride
	}
	return s.pool
}

func (s *PGStore) sqlc(ctx context.Context) *queries.Queries {
	if tx := notifyport.TxFromContext(ctx); tx != nil {
		return s.queries.WithTx(tx)
	}
	return s.queries
}

func (s *PGStore) WithinTx(ctx context.Context, fn func(context.Context) error) error {
	if fn == nil {
		return fmt.Errorf("transaction callback is required")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	txCtx := notifyport.ContextWithTx(ctx, tx)
	if err := fn(txCtx); err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && rollbackErr != pgx.ErrTxClosed {
			return fmt.Errorf("transaction failed: %w; rollback also failed: %v", err, rollbackErr)
		}
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// queryRow is a convenience wrapper.
func (s *PGStore) queryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return s.db(ctx).QueryRow(ctx, sql, args...)
}

// query is a convenience wrapper.
func (s *PGStore) query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return s.db(ctx).Query(ctx, sql, args...)
}

// exec is a convenience wrapper.
func (s *PGStore) exec(ctx context.Context, sql string, args ...any) error {
	_, err := s.db(ctx).Exec(ctx, sql, args...)
	return err
}
