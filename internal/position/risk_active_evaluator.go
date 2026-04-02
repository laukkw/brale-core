package position

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"brale-core/internal/market"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/risk"
	"brale-core/internal/store"

	"go.uber.org/zap"
)

func (m *RiskMonitor) handleActivePosition(ctx context.Context, pos store.PositionRecord) error {
	logger := logging.FromContext(ctx).Named("risk").With(
		zap.String("symbol", pos.Symbol),
		zap.String("position_id", pos.PositionID),
	)
	pos, plan, quote, skip, err := m.loadActiveRiskContext(ctx, pos)
	if err != nil {
		return &riskMonitorOpError{Op: "load active risk context", Symbol: pos.Symbol, Err: err}
	}
	if skip {
		return nil
	}
	trigger, ok := risk.EvaluateRisk(plan, pos.Side, quote.Price)
	if !ok {
		return nil
	}
	logRiskTriggerHit(logger, trigger, quote.Price, plan.StopPrice)
	pos, plan, err = m.refreshRiskPlanOnTPHit(ctx, pos, plan, trigger, logger)
	if err != nil {
		return &riskMonitorOpError{Op: "refresh risk plan on tp hit", Symbol: pos.Symbol, Err: err}
	}
	closeQty, positionQty, reason := m.resolveCloseIntent(ctx, pos, plan, trigger, logger)
	if err := m.submitCloseIntent(ctx, pos, quote, closeQty, positionQty, reason, logger); err != nil {
		return &riskMonitorOpError{Op: "submit close intent", Symbol: pos.Symbol, Err: err}
	}
	return nil
}

func (m *RiskMonitor) loadActiveRiskContext(ctx context.Context, pos store.PositionRecord) (store.PositionRecord, risk.RiskPlan, market.PriceQuote, bool, error) {
	priceCtx, cancel := riskMonitorChildTimeout(ctx, riskMonitorPriceFetchTimeout)
	quote, err := m.PriceSource.MarkPrice(priceCtx, pos.Symbol)
	cancel()
	if err != nil {
		if errors.Is(err, market.ErrPriceUnavailable) {
			return store.PositionRecord{}, risk.RiskPlan{}, market.PriceQuote{}, true, nil
		}
		return store.PositionRecord{}, risk.RiskPlan{}, market.PriceQuote{}, false, fmt.Errorf("mark price for %s: %w", pos.Symbol, err)
	}
	if quote.Price == 0 {
		return store.PositionRecord{}, risk.RiskPlan{}, market.PriceQuote{}, true, nil
	}
	if m.Positions != nil && m.Positions.Cache != nil {
		pos = m.Positions.Cache.HydratePosition(pos)
	}
	if len(pos.RiskJSON) == 0 {
		return store.PositionRecord{}, risk.RiskPlan{}, market.PriceQuote{}, true, nil
	}
	plan, err := DecodeRiskPlan(pos.RiskJSON)
	if err != nil {
		return store.PositionRecord{}, risk.RiskPlan{}, market.PriceQuote{}, false, fmt.Errorf("decode risk plan for %s: %w", pos.Symbol, err)
	}
	return pos, plan, quote, false, nil
}

func logRiskTriggerHit(logger *zap.Logger, trigger risk.RiskTrigger, markPrice, stopPrice float64) {
	logger.Info("price trigger hit",
		zap.String("trigger_type", trigger.Type),
		zap.Float64("trigger_price", trigger.Price),
		zap.Float64("mark_price", markPrice),
		zap.Float64("stop_price", stopPrice),
		zap.String("level_id", trigger.LevelID),
		zap.Float64("qty_pct", trigger.QtyPct),
		zap.String("reason", trigger.Reason),
	)
}

func (m *RiskMonitor) refreshRiskPlanOnTPHit(ctx context.Context, pos store.PositionRecord, plan risk.RiskPlan, trigger risk.RiskTrigger, logger *zap.Logger) (store.PositionRecord, risk.RiskPlan, error) {
	if trigger.Type != "TAKE_PROFIT" {
		return pos, plan, nil
	}
	updatedPlan, changed := risk.MarkTPLevelHit(plan, trigger.LevelID)
	if !changed {
		return pos, plan, nil
	}
	updatedPlan = applyTP1BreakevenStop(updatedPlan, pos.Side, pos.AvgEntry)
	svc := RiskPlanService{Store: m.Store}
	if _, err := svc.ApplyUpdate(ctx, pos.PositionID, updatedPlan, "risk-tp-hit"); err != nil {
		logger.Error("risk plan tp hit update failed", zap.Error(err))
	} else {
		plan = updatedPlan
	}
	refreshed, ok, err := m.Store.FindPositionByID(ctx, pos.PositionID)
	if err != nil {
		logger.Warn("risk plan refresh failed", zap.Error(err))
		return store.PositionRecord{}, risk.RiskPlan{}, fmt.Errorf("reload position %s after tp hit: %w", pos.PositionID, err)
	}
	if !ok {
		return store.PositionRecord{}, risk.RiskPlan{}, fmt.Errorf("position %s not found after refresh", pos.PositionID)
	}
	pos = refreshed
	if decoded, decErr := DecodeRiskPlan(pos.RiskJSON); decErr == nil {
		plan = decoded
	} else {
		logger.Warn("risk plan decode failed", zap.Error(decErr))
	}
	return pos, plan, nil
}

func applyTP1BreakevenStop(plan risk.RiskPlan, side string, entry float64) risk.RiskPlan {
	if !tp1Hit(plan) {
		return plan
	}
	breakevenStop := computeBreakevenStop(entry)
	if breakevenStop <= 0 {
		return plan
	}
	if !shouldMoveStopToBreakeven(side, plan.StopPrice, breakevenStop) {
		return plan
	}
	plan.StopPrice = breakevenStop
	return plan
}

func tp1Hit(plan risk.RiskPlan) bool {
	if len(plan.TPLevels) == 0 {
		return false
	}
	return plan.TPLevels[0].Hit
}

func computeBreakevenStop(entry float64) float64 {
	if entry <= 0 {
		return 0
	}
	return entry
}

func shouldMoveStopToBreakeven(side string, currentStop float64, breakevenStop float64) bool {
	if breakevenStop <= 0 {
		return false
	}
	if currentStop <= 0 {
		return true
	}
	if strings.EqualFold(side, "short") {
		return currentStop > breakevenStop
	}
	return currentStop < breakevenStop
}
