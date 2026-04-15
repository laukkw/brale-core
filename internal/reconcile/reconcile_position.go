package reconcile

import (
	"context"
	"strings"
	"time"

	"brale-core/internal/execution"
	"brale-core/internal/pkg/logging"
	symbolpkg "brale-core/internal/pkg/symbol"
	"brale-core/internal/position"
	"brale-core/internal/store"

	"go.uber.org/zap"
)

func (s *ReconcileService) reconcilePosition(ctx context.Context, pos store.PositionRecord, byID map[string]execution.ExternalPosition, bySymbol map[string][]execution.ExternalPosition) error {
	logger := logging.FromContext(ctx).Named("reconcile").With(
		zap.String("symbol", pos.Symbol),
		zap.String("position_id", pos.PositionID),
	)
	extPos, ok := resolveExternalPosition(pos, byID, bySymbol)
	hasExternal := ok && extPos.Quantity > 0
	if !hasExternal {
		if s.Cache != nil {
			s.Cache.DeleteByID(pos.PositionID)
		}
		return s.handleExternalMissing(ctx, pos, logger)
	}
	return s.handleExternalPresent(ctx, pos, extPos, logger)
}

func (s *ReconcileService) handleExternalMissing(ctx context.Context, pos store.PositionRecord, logger *zap.Logger) error {
	handled, err := s.handleMissingExternal(ctx, pos)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	if pos.Status == position.PositionClosed {
		return nil
	}
	if s.recheckExternalPosition(ctx, pos, logger) {
		return nil
	}
	closedPos, err := s.finalizeExternalMissing(ctx, pos, logger)
	if err != nil {
		return err
	}
	summary := s.buildExternalMissingSummary(ctx, closedPos)
	logExternalMissingSummary(ctx, closedPos, summary)
	if s.Notifier != nil {
		notice := PositionCloseSummaryNotice{
			Symbol:             closedPos.Symbol,
			Direction:          strings.TrimSpace(closedPos.Side),
			Qty:                summary.Qty,
			EntryPrice:         summary.EntryPrice,
			ExitPrice:          summary.ExitPrice,
			StopPrice:          summary.StopPrice,
			TakeProfits:        summary.TakeProfits,
			Reason:             "external_missing",
			RiskPct:            closedPos.RiskPct,
			Leverage:           closedPos.Leverage,
			PnLAmount:          summary.PnL,
			PnLPct:             summary.PnLPct,
			PositionID:         closedPos.PositionID,
			ExecutorPositionID: strings.TrimSpace(closedPos.ExecutorPositionID),
		}
		if err := s.Notifier.SendPositionCloseSummary(ctx, notice); err != nil {
			logging.FromContext(ctx).Named("reconcile").Error("position close summary notify failed", zap.Error(err))
		}
	}
	if s.Reflector != nil {
		go s.Reflector.ReflectOnClose(ctx, closedPos, summary.ExitPrice)
	}
	return nil
}

type externalMissingSummary struct {
	StopPrice   float64
	TakeProfits []float64
	EntryPrice  float64
	ExitPrice   float64
	Qty         float64
	PnL         float64
	PnLPct      float64
}

func (s *ReconcileService) recheckExternalPosition(ctx context.Context, pos store.PositionRecord, logger *zap.Logger) bool {
	tradeID := strings.TrimSpace(pos.ExecutorPositionID)
	if tradeID == "" || s.Executor == nil {
		return false
	}
	ext, err := s.Executor.GetOpenPositions(ctx, pos.Symbol)
	if err != nil {
		logger.Warn("reconcile external missing recheck failed", zap.Error(err), zap.String("external_position_id", tradeID))
		return false
	}
	byID, _ := indexExternalPositions(ext)
	if extPos, ok := byID[tradeID]; ok && extPos.Quantity > 0 {
		logger.Info("reconcile external position still open", zap.String("external_position_id", tradeID), zap.Float64("qty", extPos.Quantity))
		return true
	}
	return false
}

func (s *ReconcileService) finalizeExternalMissing(ctx context.Context, pos store.PositionRecord, logger *zap.Logger) (store.PositionRecord, error) {
	if s.Cache != nil {
		s.Cache.DeleteByID(pos.PositionID)
	}
	if _, err := s.Store.UpdatePositionPatch(ctx, store.PositionPatch{
		PositionID:       pos.PositionID,
		ExpectedVersion:  pos.Version,
		NextVersion:      pos.Version + 1,
		Status:           store.PtrString(position.PositionClosed),
		CloseIntentID:    store.PtrString(""),
		CloseSubmittedAt: store.PtrInt64(0),
	}); err != nil {
		return pos, err
	}
	logger.Info("reconcile position closed",
		zap.String("prev_status", pos.Status),
		zap.String("new_status", position.PositionClosed),
		zap.String("reason", "external_missing"),
	)
	if s.Cache != nil {
		pos = s.Cache.HydratePosition(pos)
	}
	return pos, nil
}

func (s *ReconcileService) buildExternalMissingSummary(ctx context.Context, pos store.PositionRecord) externalMissingSummary {
	stopPrice, tpPrices := resolveRiskPlanSummary(pos)
	entryPrice, exitPrice, qty := s.resolveExitMetrics(ctx, pos)
	pnl, pnlPct := computeExternalMissingPnL(pos, entryPrice, exitPrice, qty)
	return externalMissingSummary{
		StopPrice:   stopPrice,
		TakeProfits: tpPrices,
		EntryPrice:  entryPrice,
		ExitPrice:   exitPrice,
		Qty:         qty,
		PnL:         pnl,
		PnLPct:      pnlPct,
	}
}

func resolveRiskPlanSummary(pos store.PositionRecord) (float64, []float64) {
	stopPrice := float64(0)
	tpPrices := []float64{}
	if len(pos.RiskJSON) == 0 {
		return stopPrice, tpPrices
	}
	plan, err := position.DecodeRiskPlan(pos.RiskJSON)
	if err != nil {
		return stopPrice, tpPrices
	}
	stopPrice = plan.StopPrice
	for _, level := range plan.TPLevels {
		if level.Price > 0 {
			tpPrices = append(tpPrices, level.Price)
		}
	}
	return stopPrice, tpPrices
}

func (s *ReconcileService) resolveExitMetrics(ctx context.Context, pos store.PositionRecord) (float64, float64, float64) {
	entryPrice := pos.AvgEntry
	qty := pos.Qty
	if qty <= 0 && pos.InitialStake > 0 && entryPrice > 0 {
		qty = pos.InitialStake / entryPrice
	}
	exitPrice := pos.LastPrice
	if exitPrice <= 0 {
		if price, ok, err := s.loadMarkPrice(ctx, pos.Symbol); err == nil && ok {
			exitPrice = price
		}
	}
	if entryPrice <= 0 && pos.InitialStake > 0 && qty > 0 {
		entryPrice = pos.InitialStake / qty
	}
	return entryPrice, exitPrice, qty
}

func computeExternalMissingPnL(pos store.PositionRecord, entryPrice, exitPrice, qty float64) (float64, float64) {
	if entryPrice <= 0 || exitPrice <= 0 || qty <= 0 {
		return 0, 0
	}
	diff := exitPrice - entryPrice
	if strings.EqualFold(pos.Side, "short") {
		diff = entryPrice - exitPrice
	}
	pnl := diff * qty
	pnlPct := diff / entryPrice
	return pnl, pnlPct
}

func logExternalMissingSummary(ctx context.Context, pos store.PositionRecord, summary externalMissingSummary) {
	logging.FromContext(ctx).Named("reconcile").Info("position closed summary",
		zap.String("symbol", pos.Symbol),
		zap.String("direction", strings.TrimSpace(pos.Side)),
		zap.Float64("qty", summary.Qty),
		zap.Float64("entry", summary.EntryPrice),
		zap.Float64("exit", summary.ExitPrice),
		zap.Float64("pnl", summary.PnL),
		zap.Float64("pnl_pct", summary.PnLPct),
		zap.Float64("stop", summary.StopPrice),
		zap.Float64s("take_profits", summary.TakeProfits),
		zap.String("reason", "external_missing"),
		zap.Float64("risk_pct", pos.RiskPct),
		zap.Float64("leverage", pos.Leverage),
	)
}

func (s *ReconcileService) handleExternalPresent(ctx context.Context, pos store.PositionRecord, extPos execution.ExternalPosition, logger *zap.Logger) error {
	prevStatus := pos.Status
	prevQty := pos.Qty
	patch := store.PositionPatch{
		PositionID:      pos.PositionID,
		ExpectedVersion: pos.Version,
		NextVersion:     pos.Version + 1,
	}
	updates := patch.Updates()
	if s.Cache != nil {
		s.Cache.UpdateFromExternal(extPos)
	}
	if extPos.Quantity > 0 {
		patch.Qty = store.PtrFloat64(extPos.Quantity)
		updates["qty"] = extPos.Quantity
	}
	if extPos.AvgEntry > 0 {
		patch.AvgEntry = store.PtrFloat64(extPos.AvgEntry)
		updates["avg_entry"] = extPos.AvgEntry
	}
	if strings.TrimSpace(extPos.PositionID) != "" && extPos.PositionID != pos.ExecutorPositionID {
		patch.ExecutorPositionID = store.PtrString(extPos.PositionID)
		updates["executor_position_id"] = extPos.PositionID
	}
	if pos.InitialStake == 0 && extPos.InitialStake > 0 {
		patch.InitialStake = store.PtrFloat64(extPos.InitialStake)
		updates["initial_stake"] = extPos.InitialStake
	}
	status, reasons := s.applyStatusTransition(pos)
	if s.Cache != nil {
		pos = s.Cache.HydratePosition(pos)
	}
	if isCloseFlowStatus(prevStatus) && s.shouldRecoverCloseTimeout(pos, s.nowTime()) {
		status = position.PositionOpenActive
		reasons = append(reasons, "close_timeout_recovered")
	}
	if prevQty > 0 && extPos.Quantity > 0 && extPos.Quantity < prevQty {
		switch prevStatus {
		case position.PositionCloseArmed, position.PositionCloseSubmitting, position.PositionClosePending:
			status = position.PositionOpenActive
			reasons = append(reasons, "partial_close_confirmed")
		}
	}
	patch.Status = store.PtrString(status)
	updates["status"] = status
	if status == position.PositionOpenActive && isCloseFlowStatus(prevStatus) {
		patch.CloseIntentID = store.PtrString("")
		patch.CloseSubmittedAt = store.PtrInt64(0)
		updates["close_intent_id"] = ""
		updates["close_submitted_at"] = int64(0)
	}
	if len(updates) > 1 || status != pos.Status {
		if _, err := s.Store.UpdatePositionPatch(ctx, patch); err != nil {
			return err
		}
	}
	changed := prevStatus != status
	if changed {
		logger.Info("reconcile position updated",
			zap.String("prev_status", prevStatus),
			zap.String("new_status", status),
			zap.String("reason", strings.Join(reasons, ",")),
			zap.String("external_position_id", extPos.PositionID),
		)
	}
	return nil
}

func (s *ReconcileService) applyStatusTransition(pos store.PositionRecord) (string, []string) {
	status := pos.Status
	reasons := make([]string, 0, 2)
	switch status {
	case position.PositionOpenSubmitting, position.PositionOpenPending:
		status = position.PositionOpenActive
		reasons = append(reasons, "open_confirmed")
	case position.PositionOpenAborting:
		status = position.PositionOpenActive
		reasons = append(reasons, "abort_filled")
	case position.PositionOpenAborted:
		status = position.PositionOpenActive
		reasons = append(reasons, "abort_after_filled")
	case position.PositionOpenActive:
	}
	return status, reasons
}

func resolveExternalPosition(pos store.PositionRecord, byID map[string]execution.ExternalPosition, bySymbol map[string][]execution.ExternalPosition) (execution.ExternalPosition, bool) {
	if pos.ExecutorPositionID != "" {
		if ext, ok := byID[pos.ExecutorPositionID]; ok {
			return ext, true
		}
	}
	candidates := bySymbol[symbolpkg.Normalize(pos.Symbol)]
	if len(candidates) == 1 {
		return candidates[0], true
	}
	return execution.ExternalPosition{}, false
}

func isCloseFlowStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case position.PositionCloseArmed, position.PositionCloseSubmitting, position.PositionClosePending:
		return true
	default:
		return false
	}
}

func (s *ReconcileService) shouldRecoverCloseTimeout(pos store.PositionRecord, now time.Time) bool {
	if !isCloseFlowStatus(pos.Status) {
		return false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	startedAt := closeSubmittedAt(pos)
	if startedAt.IsZero() {
		return false
	}
	return now.Sub(startedAt) >= s.closeRecoverAfter()
}

func closeSubmittedAt(pos store.PositionRecord) time.Time {
	if pos.CloseSubmittedAt > 0 {
		return time.UnixMilli(pos.CloseSubmittedAt).UTC()
	}
	if !pos.UpdatedAt.IsZero() {
		return pos.UpdatedAt.UTC()
	}
	return time.Time{}
}
