package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"brale-core/internal/pkg/logging"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"go.uber.org/zap"
)

// ─── NotifyRender Job ────────────────────────────────────────────

// NotifyRenderArgs triggers notification rendering (e.g. OG card generation).
type NotifyRenderArgs struct {
	EventType string          `json:"event_type"` // "decision", "position_open", "position_close", "risk_update", etc.
	Symbol    string          `json:"symbol"`
	Payload   json.RawMessage `json:"payload"`
}

func (NotifyRenderArgs) Kind() string { return "notify_render" }

// NotifyRenderWorker renders a notification payload into a deliverable format.
type NotifyRenderWorker struct {
	river.WorkerDefaults[NotifyRenderArgs]
	Render func(ctx context.Context, eventType, symbol string, payload json.RawMessage) (rendered json.RawMessage, err error)
}

func (w *NotifyRenderWorker) Work(ctx context.Context, job *river.Job[NotifyRenderArgs]) error {
	logger := logging.FromContext(ctx).Named("jobs").With(
		zap.String("event_type", job.Args.EventType),
		zap.String("symbol", job.Args.Symbol),
	)
	start := time.Now()

	if w.Render == nil {
		return fmt.Errorf("render function not configured")
	}
	rendered, err := w.Render(ctx, job.Args.EventType, job.Args.Symbol, job.Args.Payload)
	if err != nil {
		logger.Error("notify render failed", zap.Error(err))
		return err
	}

	// Enqueue a delivery job with the rendered payload.
	// We use the same River client from the job context.
	_, err = river.ClientFromContext[pgx.Tx](ctx).Insert(ctx, NotifyDeliverArgs{
		EventType: job.Args.EventType,
		Symbol:    job.Args.Symbol,
		Rendered:  rendered,
	}, nil)
	if err != nil {
		logger.Error("enqueue deliver job failed", zap.Error(err))
		return err
	}

	logger.Debug("notify render completed", zap.Duration("elapsed", time.Since(start)))
	return nil
}

// ─── NotifyDeliver Job ───────────────────────────────────────────

// NotifyDeliverArgs delivers a rendered notification to channels (webhook, telegram, feishu).
type NotifyDeliverArgs struct {
	EventType string          `json:"event_type"`
	Symbol    string          `json:"symbol"`
	Rendered  json.RawMessage `json:"rendered"`
}

func (NotifyDeliverArgs) Kind() string { return "notify_deliver" }

// NotifyDeliverWorker sends the rendered notification to configured channels.
type NotifyDeliverWorker struct {
	river.WorkerDefaults[NotifyDeliverArgs]
	Deliver func(ctx context.Context, eventType, symbol string, rendered json.RawMessage) error
}

func (w *NotifyDeliverWorker) Work(ctx context.Context, job *river.Job[NotifyDeliverArgs]) error {
	logger := logging.FromContext(ctx).Named("jobs").With(
		zap.String("event_type", job.Args.EventType),
		zap.String("symbol", job.Args.Symbol),
	)
	start := time.Now()

	if w.Deliver == nil {
		return fmt.Errorf("deliver function not configured")
	}
	if err := w.Deliver(ctx, job.Args.EventType, job.Args.Symbol, job.Args.Rendered); err != nil {
		logger.Error("notify deliver failed", zap.Error(err), zap.Int("attempt", job.Attempt))
		return err
	}

	logger.Info("notification delivered", zap.Duration("elapsed", time.Since(start)))
	return nil
}

func (w *NotifyDeliverWorker) NextRetry(job *river.Job[NotifyDeliverArgs]) time.Time {
	return time.Now().Add(time.Duration(job.Attempt*job.Attempt) * 10 * time.Second)
}
