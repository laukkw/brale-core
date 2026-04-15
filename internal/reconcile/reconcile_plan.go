package reconcile

import (
	"context"
	"encoding/json"
	"strings"

	"brale-core/internal/execution"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/position"
	"brale-core/internal/risk"
	"brale-core/internal/store"

	"go.uber.org/zap"
)

func (s *ReconcileService) reconcilePlanEntries(ctx context.Context, symbol string, byID map[string]execution.ExternalPosition, bySymbol map[string][]execution.ExternalPosition) error {
	if s.PlanCache == nil {
		return nil
	}
	if strings.TrimSpace(symbol) != "" {
		return s.reconcilePlanEntry(ctx, symbol, byID, bySymbol)
	}
	symbols, err := s.Store.ListSymbols(ctx)
	if err != nil {
		return err
	}
	for _, sym := range symbols {
		if err := s.reconcilePlanEntry(ctx, sym, byID, bySymbol); err != nil {
			return err
		}
	}
	return nil
}

func (s *ReconcileService) reconcilePlanEntry(ctx context.Context, symbol string, byID map[string]execution.ExternalPosition, bySymbol map[string][]execution.ExternalPosition) error {
	if s.PlanCache == nil {
		return nil
	}
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return nil
	}
	entry, ok := s.PlanCache.GetEntry(symbol)
	if !ok || entry == nil {
		return nil
	}
	plan := entry.Plan
	planSymbol := strings.TrimSpace(plan.Symbol)
	if planSymbol == "" {
		return nil
	}
	logger := logging.FromContext(ctx).Named("reconcile").With(
		zap.String("symbol", planSymbol),
		zap.String("position_id", plan.PositionID),
	)

	if entry.ExternalID != "" {
		if ext, ok := byID[entry.ExternalID]; ok && ext.Quantity > 0 {
			return s.openPositionFromPlan(ctx, plan, ext, logger)
		}
		return nil
	}
	candidates := bySymbol[strings.ToUpper(planSymbol)]
	if len(candidates) == 1 && candidates[0].Quantity > 0 {
		return s.openPositionFromPlan(ctx, plan, candidates[0], logger)
	}
	return nil
}

func (s *ReconcileService) reconcilePlanOrders(ctx context.Context, symbol string) error {
	if s.PlanCache == nil || s.Executor == nil {
		return nil
	}
	if s.Store == nil {
		return reconcileValidationErrorf("store is required")
	}
	symbolFilter := strings.TrimSpace(symbol)
	entries := s.PlanCache.ListEntries()
	for _, entry := range entries {
		plan := entry.Plan
		planSymbol := strings.TrimSpace(plan.Symbol)
		if planSymbol == "" {
			continue
		}
		if symbolFilter != "" && !strings.EqualFold(planSymbol, symbolFilter) {
			continue
		}
		if entry.ExternalID == "" {
			continue
		}
		ord, err := s.Executor.GetOrder(ctx, entry.ExternalID)
		if err != nil {
			continue
		}
		if ord.FilledQty <= 0 {
			continue
		}
		avgEntry := ord.Price
		if avgEntry <= 0 {
			avgEntry = plan.Entry
		}
		ext := execution.ExternalPosition{
			PositionID: entry.ExternalID,
			Symbol:     planSymbol,
			Side:       plan.Direction,
			Quantity:   ord.FilledQty,
			AvgEntry:   avgEntry,
			UpdatedAt:  s.nowMillis(),
		}
		logger := logging.FromContext(ctx).Named("reconcile").With(
			zap.String("symbol", planSymbol),
			zap.String("position_id", plan.PositionID),
		)
		if err := s.openPositionFromPlan(ctx, plan, ext, logger); err != nil {
			return err
		}
	}
	return nil
}

func (s *ReconcileService) openPositionFromPlan(ctx context.Context, plan execution.ExecutionPlan, ext execution.ExternalPosition, logger *zap.Logger) error {
	if s.Store == nil {
		return reconcileValidationErrorf("store is required")
	}
	if strings.TrimSpace(plan.PositionID) == "" {
		return reconcileValidationErrorf("position_id is required")
	}
	if ext.Quantity <= 0 {
		return nil
	}
	rec := store.PositionRecord{
		PositionID:         plan.PositionID,
		Symbol:             ext.Symbol,
		Side:               plan.Direction,
		Status:             position.PositionOpenActive,
		ExecutorName:       s.Executor.Name(),
		ExecutorPositionID: ext.PositionID,
		Version:            1,
		Source:             "entry_fill",
		StopReason:         strings.TrimSpace(plan.RiskAnnotations.StopReason),
		Qty:                ext.Quantity,
		AvgEntry:           ext.AvgEntry,
		InitialStake:       ext.InitialStake,
		RiskPct:            plan.RiskPct,
		Leverage:           plan.Leverage,
	}
	riskPlan := risk.BuildRiskPlan(risk.RiskPlanInput{
		Entry:            plan.Entry,
		StopLoss:         plan.StopLoss,
		PositionSize:     plan.PositionSize,
		TakeProfits:      plan.TakeProfits,
		TakeProfitRatios: plan.TakeProfitRatios,
	})
	raw, err := json.Marshal(risk.CompactRiskPlan(riskPlan))
	if err != nil {
		return err
	}
	rec.RiskJSON = raw
	stopPrice := riskPlan.StopPrice
	tpPrices := make([]float64, 0, len(riskPlan.TPLevels))
	for _, level := range riskPlan.TPLevels {
		if level.Price > 0 {
			tpPrices = append(tpPrices, level.Price)
		}
	}
	symbol := strings.TrimSpace(plan.Symbol)
	if symbol == "" {
		symbol = strings.TrimSpace(ext.Symbol)
	}
	stopReason := strings.TrimSpace(plan.RiskAnnotations.StopReason)
	if err := reconcileWithinStoreTx(ctx, s.Store, func(runCtx context.Context) error {
		if err := s.Store.SavePosition(runCtx, &rec); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				return nil
			}
			return err
		}
		if s.RiskPlans != nil {
			if _, err := s.RiskPlans.SaveHistory(runCtx, rec.PositionID, riskPlan, "entry_fill"); err != nil {
				return err
			}
		}
		if stopPrice <= 0 && len(tpPrices) == 0 {
			return nil
		}
		logger.Info("position open detail",
			zap.String("symbol", symbol),
			zap.String("direction", strings.TrimSpace(plan.Direction)),
			zap.Float64("qty", ext.Quantity),
			zap.Float64("entry", ext.AvgEntry),
			zap.Float64("stop", stopPrice),
			zap.Float64s("take_profits", tpPrices),
			zap.String("stop_reason", stopReason),
			zap.Float64("risk_pct", plan.RiskPct),
			zap.Float64("leverage", plan.Leverage),
		)
		if s.Notifier == nil {
			return nil
		}
		if err := s.Notifier.SendPositionOpen(runCtx, PositionOpenNotice{
			Symbol:      symbol,
			Direction:   strings.TrimSpace(plan.Direction),
			Qty:         ext.Quantity,
			EntryPrice:  ext.AvgEntry,
			StopPrice:   stopPrice,
			TakeProfits: tpPrices,
			StopReason:  stopReason,
			RiskPct:     plan.RiskPct,
			Leverage:    plan.Leverage,
			PositionID:  plan.PositionID,
		}); err != nil {
			logger.Error("position open notify failed", zap.Error(err))
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	if s.Cache != nil {
		s.Cache.UpdateFromExternal(ext)
	}
	if s.PlanCache != nil {
		s.PlanCache.Remove(plan.Symbol)
	}
	logger.Info("open filled persisted",
		zap.String("external_order_id", ext.PositionID),
		zap.Float64("qty", ext.Quantity),
		zap.Float64("avg_entry", ext.AvgEntry),
	)
	return nil
}

func reconcileWithinStoreTx(ctx context.Context, st store.Store, fn func(context.Context) error) error {
	if fn == nil {
		return nil
	}
	txRunner, ok := st.(store.TxRunner)
	if !ok {
		return fn(ctx)
	}
	return txRunner.WithinTx(ctx, fn)
}
