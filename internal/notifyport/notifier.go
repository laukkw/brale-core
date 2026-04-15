// Package notifyport defines the notification port interface and its River-backed async implementation.
package notifyport

import (
	"context"
	"encoding/json"
	"fmt"

	"brale-core/internal/jobs"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"go.uber.org/zap"
)

// Notifier is the interface for emitting notification events.
type Notifier interface {
	NotifyPositionOpen(ctx context.Context, n PositionOpenNotice) error
	NotifyPositionClose(ctx context.Context, n PositionCloseNotice) error
	NotifyPositionCloseSummary(ctx context.Context, n PositionCloseSummaryNotice) error
	NotifyRiskPlanUpdate(ctx context.Context, n RiskPlanUpdateNotice) error
	NotifyError(ctx context.Context, n ErrorNotice) error
}

// RiverNotifier enqueues notification jobs via River InsertTx within
// the caller's database transaction, ensuring atomicity with business writes.
type RiverNotifier struct {
	client *river.Client[pgx.Tx]
	logger *zap.Logger
}

// NewRiverNotifier wraps a River client for transactional notification enqueue.
// When client is nil, all Notify methods are no-ops (for tests / disabled mode).
func NewRiverNotifier(client *river.Client[pgx.Tx], logger *zap.Logger) *RiverNotifier {
	return &RiverNotifier{client: client, logger: logger}
}

func (n *RiverNotifier) enqueue(ctx context.Context, eventType, symbol string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal %s payload: %w", eventType, err)
	}

	if n.client == nil {
		if n.logger != nil {
			n.logger.Debug("notification skipped (no river client)", zap.String("event_type", eventType))
		}
		return nil
	}

	args := jobs.NotifyRenderArgs{
		EventType: eventType,
		Symbol:    symbol,
		Payload:   json.RawMessage(data),
	}

	// Try to get a pgx tx from context for transactional enqueue.
	// If no tx in context, fall back to direct insert.
	if tx := TxFromContext(ctx); tx != nil {
		_, err = n.client.InsertTx(ctx, tx, args, nil)
	} else {
		_, err = n.client.Insert(ctx, args, nil)
	}
	if err != nil {
		return fmt.Errorf("enqueue %s notification: %w", eventType, err)
	}
	return nil
}

func (n *RiverNotifier) NotifyPositionOpen(ctx context.Context, notice PositionOpenNotice) error {
	return n.enqueue(ctx, "position_open", notice.Symbol, notice)
}

func (n *RiverNotifier) NotifyPositionClose(ctx context.Context, notice PositionCloseNotice) error {
	return n.enqueue(ctx, "position_close", notice.Symbol, notice)
}

func (n *RiverNotifier) NotifyPositionCloseSummary(ctx context.Context, notice PositionCloseSummaryNotice) error {
	return n.enqueue(ctx, "position_close_summary", notice.Symbol, notice)
}

func (n *RiverNotifier) NotifyRiskPlanUpdate(ctx context.Context, notice RiskPlanUpdateNotice) error {
	return n.enqueue(ctx, "risk_update", notice.Symbol, notice)
}

func (n *RiverNotifier) NotifyError(ctx context.Context, notice ErrorNotice) error {
	return n.enqueue(ctx, "error", notice.Symbol, notice)
}

// ─── Transaction context key ─────────────────────────────────────

type txKeyType struct{}

var txKey = txKeyType{}

// ContextWithTx returns a new context carrying a pgx transaction for River InsertTx.
func ContextWithTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, txKey, tx)
}

// TxFromContext extracts a pgx transaction from context, or nil.
func TxFromContext(ctx context.Context) pgx.Tx {
	if v, ok := ctx.Value(txKey).(pgx.Tx); ok {
		return v
	}
	return nil
}

// ─── No-op notifier ──────────────────────────────────────────────

// NoopNotifier discards all notifications. Useful for tests and backtest mode.
type NoopNotifier struct{}

func (NoopNotifier) NotifyPositionOpen(context.Context, PositionOpenNotice) error              { return nil }
func (NoopNotifier) NotifyPositionClose(context.Context, PositionCloseNotice) error            { return nil }
func (NoopNotifier) NotifyPositionCloseSummary(context.Context, PositionCloseSummaryNotice) error { return nil }
func (NoopNotifier) NotifyRiskPlanUpdate(context.Context, RiskPlanUpdateNotice) error          { return nil }
func (NoopNotifier) NotifyError(context.Context, ErrorNotice) error                            { return nil }

var _ Notifier = (*RiverNotifier)(nil)
var _ Notifier = NoopNotifier{}
