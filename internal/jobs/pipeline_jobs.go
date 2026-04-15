package jobs

import (
	"context"
	"fmt"
	"time"

	"brale-core/internal/pkg/logging"

	"github.com/riverqueue/river"
	"go.uber.org/zap"
)

// ─── Observe Job ─────────────────────────────────────────────────

// ObserveArgs are the arguments for an observe task.
type ObserveArgs struct {
	Symbol string `json:"symbol"`
}

func (ObserveArgs) Kind() string { return "observe" }

// ObserveWorker executes market observation for a symbol.
type ObserveWorker struct {
	river.WorkerDefaults[ObserveArgs]
	Execute func(ctx context.Context, symbol string) error
}

func (w *ObserveWorker) Work(ctx context.Context, job *river.Job[ObserveArgs]) error {
	logger := logging.FromContext(ctx).Named("jobs").With(zap.String("symbol", job.Args.Symbol))
	start := time.Now()
	logger.Info("observe job started")

	if w.Execute == nil {
		return fmt.Errorf("observe executor not configured")
	}
	if err := w.Execute(ctx, job.Args.Symbol); err != nil {
		logger.Error("observe job failed", zap.Error(err), zap.Duration("elapsed", time.Since(start)))
		return err
	}

	logger.Info("observe job completed", zap.Duration("elapsed", time.Since(start)))
	return nil
}

// ─── Decide Job ──────────────────────────────────────────────────

// DecideArgs are the arguments for a decide task.
type DecideArgs struct {
	Symbol string `json:"symbol"`
}

func (DecideArgs) Kind() string { return "decide" }

// DecideWorker executes the decision pipeline for a symbol.
type DecideWorker struct {
	river.WorkerDefaults[DecideArgs]
	Execute func(ctx context.Context, symbol string) error
}

func (w *DecideWorker) Work(ctx context.Context, job *river.Job[DecideArgs]) error {
	logger := logging.FromContext(ctx).Named("jobs").With(zap.String("symbol", job.Args.Symbol))
	start := time.Now()
	logger.Info("decide job started")

	if w.Execute == nil {
		return fmt.Errorf("decide executor not configured")
	}
	if err := w.Execute(ctx, job.Args.Symbol); err != nil {
		logger.Error("decide job failed", zap.Error(err), zap.Duration("elapsed", time.Since(start)))
		return err
	}

	logger.Info("decide job completed", zap.Duration("elapsed", time.Since(start)))
	return nil
}

// ─── Reconcile Job ───────────────────────────────────────────────

// ReconcileArgs are the arguments for a reconciliation task.
type ReconcileArgs struct {
	Symbol string `json:"symbol"`
}

func (ReconcileArgs) Kind() string { return "reconcile" }

// ReconcileWorker runs position reconciliation.
type ReconcileWorker struct {
	river.WorkerDefaults[ReconcileArgs]
	Execute func(ctx context.Context, symbol string) error
}

func (w *ReconcileWorker) Work(ctx context.Context, job *river.Job[ReconcileArgs]) error {
	logger := logging.FromContext(ctx).Named("jobs").With(zap.String("symbol", job.Args.Symbol))
	start := time.Now()

	if w.Execute == nil {
		return fmt.Errorf("reconcile executor not configured")
	}
	if err := w.Execute(ctx, job.Args.Symbol); err != nil {
		logger.Error("reconcile job failed", zap.Error(err), zap.Duration("elapsed", time.Since(start)))
		return err
	}

	logger.Debug("reconcile job completed", zap.Duration("elapsed", time.Since(start)))
	return nil
}

// ─── Risk Monitor Job ─────────────────────────────────────────────

type RiskMonitorArgs struct {
	Symbol string `json:"symbol"`
}

func (RiskMonitorArgs) Kind() string { return "risk_monitor" }

type RiskMonitorWorker struct {
	river.WorkerDefaults[RiskMonitorArgs]
	Execute func(ctx context.Context, symbol string) error
}

func (w *RiskMonitorWorker) Work(ctx context.Context, job *river.Job[RiskMonitorArgs]) error {
	logger := logging.FromContext(ctx).Named("jobs").With(zap.String("symbol", job.Args.Symbol))
	start := time.Now()
	if w.Execute == nil {
		return fmt.Errorf("risk monitor executor not configured")
	}
	if err := w.Execute(ctx, job.Args.Symbol); err != nil {
		logger.Error("risk monitor job failed", zap.Error(err), zap.Duration("elapsed", time.Since(start)))
		return err
	}
	logger.Debug("risk monitor job completed", zap.Duration("elapsed", time.Since(start)))
	return nil
}
