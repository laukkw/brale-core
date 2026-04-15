// Package jobs defines River job types and workers for brale-core.
// River replaces the built-in time.Ticker scheduler as the sole scheduling backend.
package jobs

import (
	"context"
	"fmt"
	"time"

	"brale-core/internal/pkg/logging"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"go.uber.org/zap"
)

// Client wraps the River client and provides lifecycle management.
type Client struct {
	inner  *river.Client[pgx.Tx]
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// RunMigrations runs River's internal schema migrations (river_job, river_queue, etc.).
func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return fmt.Errorf("create river migrator: %w", err)
	}
	_, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	if err != nil {
		return fmt.Errorf("run river migrations: %w", err)
	}
	return nil
}

// NewClient creates a River client backed by the given pgx pool.
// Workers must be added before calling Start.
func NewClient(ctx context.Context, pool *pgxpool.Pool, workers *river.Workers, periodicJobs []*river.PeriodicJob, logger *zap.Logger) (*Client, error) {
	if logger == nil {
		logger = logging.L().Named("river")
	}

	cfg := &river.Config{
		Workers:      workers,
		Queues:       map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 10}},
		PeriodicJobs: periodicJobs,
		FetchCooldown: 200 * time.Millisecond,
	}

	client, err := river.NewClient(riverpgxv5.New(pool), cfg)
	if err != nil {
		return nil, fmt.Errorf("create river client: %w", err)
	}

	return &Client{inner: client, pool: pool, logger: logger}, nil
}

// Start begins processing jobs.
func (c *Client) Start(ctx context.Context) error {
	if err := c.inner.Start(ctx); err != nil {
		return fmt.Errorf("start river client: %w", err)
	}
	c.logger.Info("river job client started")
	return nil
}

// Stop gracefully shuts down the River client.
func (c *Client) Stop(ctx context.Context) error {
	c.inner.Stop(ctx)
	c.logger.Info("river job client stopped")
	return nil
}

// Inner returns the underlying river.Client for InsertTx operations.
func (c *Client) Inner() *river.Client[pgx.Tx] {
	return c.inner
}
