package reconcile

import (
	"context"
	"errors"
	"strings"
	"time"

	"brale-core/internal/execution"
	"brale-core/internal/market"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/position"
	"brale-core/internal/store"

	"go.uber.org/zap"
)

func (s *ReconcileService) handleMissingExternal(ctx context.Context, pos store.PositionRecord) (bool, error) {
	switch pos.Status {
	case position.PositionOpenSubmitting, position.PositionOpenPending:
		if err := s.tryAbortByDrift(ctx, pos); err != nil {
			return true, err
		}
		if strings.TrimSpace(pos.ExecutorPositionID) != "" {
			if err := s.abortIfCanceled(ctx, pos); err != nil {
				return true, err
			}
		}
		return true, nil
	case position.PositionOpenAborting:
		if err := s.progressAbort(ctx, pos); err != nil {
			return true, err
		}
		return true, nil
	case position.PositionOpenAborted:
		return true, nil
	default:
		return false, nil
	}
}

func (s *ReconcileService) tryAbortByDrift(ctx context.Context, pos store.PositionRecord) error {
	if s.PlanCache == nil {
		return nil
	}
	entry, ok := s.PlanCache.GetEntry(pos.Symbol)
	if !ok || entry == nil {
		return nil
	}
	plan := entry.Plan
	if strings.TrimSpace(plan.PositionID) != "" && plan.PositionID != pos.PositionID {
		return nil
	}
	planSymbol := strings.TrimSpace(plan.Symbol)
	if planSymbol == "" {
		planSymbol = pos.Symbol
	}
	now := time.Now().UTC()
	if nowMillis := s.nowMillis(); nowMillis > 0 {
		now = time.UnixMilli(nowMillis).UTC()
	}
	shouldAbort, reason := shouldAbortEntry(plan, now)
	if !shouldAbort {
		return nil
	}
	mark := float64(0)
	if strings.TrimSpace(planSymbol) != "" {
		price, ok, err := s.loadMarkPrice(ctx, planSymbol)
		if err != nil {
			return err
		}
		if ok {
			mark = price
		}
	}
	return s.startAbort(ctx, pos, reason, mark)
}

func (s *ReconcileService) abortIfCanceled(ctx context.Context, pos store.PositionRecord) error {
	status, ok, err := s.loadBridgeStatus(ctx, pos)
	if err != nil {
		logAbortStatusFetchDegraded(ctx, pos, err, "pending")
		return nil
	}
	if !ok {
		return nil
	}
	if !isTerminalCancelStatus(status) {
		return nil
	}
	reason := "order_" + status
	return s.finalizeAbort(ctx, pos, reason)
}

func (s *ReconcileService) progressAbort(ctx context.Context, pos store.PositionRecord) error {
	logAbortStuckWarning(ctx, pos)
	status, ok, err := s.loadBridgeStatus(ctx, pos)
	if err != nil {
		logAbortStatusFetchDegraded(ctx, pos, err, "aborting")
		return s.cancelEntry(ctx, pos)
	}
	if ok && isTerminalCancelStatus(status) {
		reason := pos.AbortReason
		if reason == "" {
			reason = "order_" + status
		}
		return s.finalizeAbort(ctx, pos, reason)
	}
	return s.cancelEntry(ctx, pos)
}

func (s *ReconcileService) startAbort(ctx context.Context, pos store.PositionRecord, reason string, mark float64) error {
	now := s.nowMillis()
	updates := map[string]any{
		"status":           position.PositionOpenAborting,
		"abort_reason":     reason,
		"abort_started_at": now,
		"version":          pos.Version + 1,
	}
	if s.Cache != nil {
		s.Cache.UpdatePrice(pos.PositionID, pos.Symbol, mark, now)
	}
	ok, err := s.Store.UpdatePosition(ctx, pos.PositionID, pos.Version, updates)
	if err != nil || !ok {
		return err
	}
	return s.cancelEntry(ctx, pos)
}

func (s *ReconcileService) finalizeAbort(ctx context.Context, pos store.PositionRecord, reason string) error {
	now := s.nowMillis()
	updates := map[string]any{
		"status":             position.PositionOpenAborted,
		"abort_finalized_at": now,
		"version":            pos.Version + 1,
	}
	if strings.TrimSpace(reason) != "" {
		updates["abort_reason"] = reason
	}
	_, err := s.Store.UpdatePosition(ctx, pos.PositionID, pos.Version, updates)
	if err != nil {
		return err
	}
	if s.PlanCache != nil {
		s.PlanCache.ClearIfMatch(pos.Symbol, pos.ExecutorPositionID, pos.OpenIntentID)
	}
	return err
}

func (s *ReconcileService) cancelEntry(ctx context.Context, pos store.PositionRecord) error {
	tradeID := strings.TrimSpace(pos.ExecutorPositionID)
	if tradeID == "" {
		return reconcileValidationErrorf("executor_position_id is required")
	}
	_, err := s.Executor.CancelOrder(ctx, execution.CancelOrderReq{ExternalID: tradeID})
	return err
}

func (s *ReconcileService) loadMarkPrice(ctx context.Context, symbol string) (float64, bool, error) {
	if s.PriceSource == nil {
		return 0, false, nil
	}
	quote, err := s.PriceSource.MarkPrice(ctx, symbol)
	if err != nil {
		if errors.Is(err, market.ErrPriceUnavailable) {
			return 0, false, nil
		}
		return 0, false, err
	}
	if quote.Price <= 0 {
		return 0, false, nil
	}
	return quote.Price, true, nil
}

func (s *ReconcileService) loadBridgeStatus(ctx context.Context, pos store.PositionRecord) (string, bool, error) {
	tradeID := strings.TrimSpace(pos.ExecutorPositionID)
	if tradeID == "" {
		return "", false, reconcileValidationErrorf("executor_position_id is required")
	}
	if s.OrderStatusFetcher == nil {
		return "", false, nil
	}
	status, err := s.OrderStatusFetcher.Fetch(ctx, tradeID)
	if err != nil {
		if errors.Is(err, execution.ErrOrderNotFound) {
			return "canceled", true, nil
		}
		return "", false, err
	}
	value := strings.TrimSpace(status.Status)
	if value == "" {
		return "", false, nil
	}
	return value, true, nil
}

func (s *ReconcileService) nowMillis() int64 {
	if s.Now != nil {
		return s.Now()
	}
	return int64(0)
}

func shouldAbortEntry(plan execution.ExecutionPlan, now time.Time) (bool, string) {
	if plan.ExpiresAt.IsZero() {
		return false, ""
	}
	if now.After(plan.ExpiresAt) || now.Equal(plan.ExpiresAt) {
		return true, "plan_expired"
	}
	return false, ""
}

func isTerminalCancelStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "canceled", "cancelled", "expired", "rejected":
		return true
	default:
		return false
	}
}

func logAbortStatusFetchDegraded(ctx context.Context, pos store.PositionRecord, err error, stage string) {
	logging.FromContext(ctx).Named("reconcile").Warn("order status fetch degraded, continue cancel flow",
		zap.String("stage", strings.TrimSpace(stage)),
		zap.String("symbol", pos.Symbol),
		zap.String("position_id", pos.PositionID),
		zap.String("external_order_id", pos.ExecutorPositionID),
		zap.Error(err),
	)
}

func logAbortStuckWarning(ctx context.Context, pos store.PositionRecord) {
	if pos.AbortStartedAt <= 0 {
		return
	}
	elapsed := time.Since(time.UnixMilli(pos.AbortStartedAt))
	if elapsed < 15*time.Minute {
		return
	}
	logging.FromContext(ctx).Named("reconcile").Warn("abort flow seems stuck",
		zap.String("symbol", pos.Symbol),
		zap.String("position_id", pos.PositionID),
		zap.String("external_order_id", pos.ExecutorPositionID),
		zap.Duration("elapsed", elapsed),
	)
}
