// 本文件主要内容：基于 mark price 触发止盈止损并下发平仓意图。
package position

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"brale-core/internal/execution"
	"brale-core/internal/market"
	"brale-core/internal/pkg/errclass"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/risk"
	"brale-core/internal/store"

	"go.uber.org/zap"
)

type RiskMonitor struct {
	Store          store.Store
	PriceSource    market.PriceSource
	Positions      *PositionService
	PlanCache      *PlanCache
	AccountFetcher func(ctx context.Context, symbol string) (execution.AccountState, error)
}

func (m *RiskMonitor) RunOnce(ctx context.Context, symbol string) error {
	if err := m.validate(); err != nil {
		return err
	}
	if err := m.handleEntryArmed(ctx, symbol); err != nil {
		return err
	}
	positions, err := m.Store.ListPositionsByStatus(ctx, []string{PositionOpenActive})
	if err != nil {
		return err
	}
	for _, pos := range positions {
		if symbol != "" && !strings.EqualFold(pos.Symbol, symbol) {
			continue
		}
		if err := m.handleActivePosition(ctx, pos); err != nil {
			return err
		}
	}
	return nil
}

func (m *RiskMonitor) handleEntryArmed(ctx context.Context, symbol string) error {
	if m.PlanCache == nil {
		return nil
	}
	if strings.TrimSpace(symbol) != "" {
		return m.handlePlanEntry(ctx, strings.TrimSpace(symbol))
	}
	entries := m.PlanCache.ListEntries()
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		sym := strings.TrimSpace(entry.Plan.Symbol)
		if sym == "" {
			continue
		}
		if _, ok := seen[sym]; ok {
			continue
		}
		seen[sym] = struct{}{}
		if err := m.handlePlanEntry(ctx, sym); err != nil {
			return err
		}
	}
	return nil
}

func (m *RiskMonitor) validate() error {
	if m.Store == nil || m.PriceSource == nil || m.Positions == nil || m.PlanCache == nil {
		return riskValidationErrorf("store/price_source/positions/plan_cache is required")
	}
	return nil
}

func (m *RiskMonitor) handleActivePosition(ctx context.Context, pos store.PositionRecord) error {
	logger := logging.FromContext(ctx).Named("risk").With(
		zap.String("symbol", pos.Symbol),
		zap.String("position_id", pos.PositionID),
	)
	pos, plan, quote, skip, err := m.loadActiveRiskContext(ctx, pos)
	if err != nil {
		return err
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
		return err
	}
	closeQty, reason := m.resolveCloseIntent(ctx, pos, plan, trigger, logger)
	return m.submitCloseIntent(ctx, pos, quote, closeQty, reason, logger)
}

func (m *RiskMonitor) loadActiveRiskContext(ctx context.Context, pos store.PositionRecord) (store.PositionRecord, risk.RiskPlan, market.PriceQuote, bool, error) {
	quote, err := m.PriceSource.MarkPrice(ctx, pos.Symbol)
	if err != nil {
		if errors.Is(err, market.ErrPriceUnavailable) {
			return store.PositionRecord{}, risk.RiskPlan{}, market.PriceQuote{}, true, nil
		}
		return store.PositionRecord{}, risk.RiskPlan{}, market.PriceQuote{}, false, err
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
		return store.PositionRecord{}, risk.RiskPlan{}, market.PriceQuote{}, false, err
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
		return store.PositionRecord{}, risk.RiskPlan{}, err
	}
	if !ok {
		return store.PositionRecord{}, risk.RiskPlan{}, fmt.Errorf("position not found after refresh")
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

func (m *RiskMonitor) resolveCloseIntent(ctx context.Context, pos store.PositionRecord, plan risk.RiskPlan, trigger risk.RiskTrigger, logger *zap.Logger) (float64, string) {
	statusQty := float64(0)
	if trigger.Type == "TAKE_PROFIT" && trigger.QtyPct > 0 && trigger.QtyPct < 1 {
		qty, ok, err := fetchFreqtradeStatusAmount(ctx, m.Positions.Executor, pos.ExecutorPositionID)
		if err != nil {
			logger.Warn("freqtrade status amount fetch failed", zap.Error(err))
		} else if ok {
			statusQty = qty
		}
	}
	closeQty, reason := resolveCloseQty(pos, plan, trigger, statusQty)
	if pos.Qty > 0 && closeQty > pos.Qty {
		closeQty = pos.Qty
	}
	return closeQty, reason
}

func (m *RiskMonitor) submitCloseIntent(ctx context.Context, pos store.PositionRecord, quote market.PriceQuote, closeQty float64, reason string, logger *zap.Logger) error {
	if closeQty <= 0 {
		return nil
	}
	logger.Info("close intent arm",
		zap.String("reason", reason),
		zap.Float64("close_qty", closeQty),
		zap.Float64("position_qty", pos.Qty),
		zap.Float64("entry_price", pos.AvgEntry),
		zap.Float64("mark_price", quote.Price),
	)
	_, err := m.Positions.ArmClose(ctx, pos, reason, quote.Price, closeQty)
	if err != nil {
		logger.Error("arm close failed", zap.Error(err), zap.String("reason", reason))
		return err
	}
	return nil
}

func fetchFreqtradeStatusAmount(ctx context.Context, executor execution.Executor, tradeID string) (float64, bool, error) {
	type openTradesExecutor interface {
		ListOpenTrades(ctx context.Context) ([]execution.Trade, error)
	}
	lister, ok := executor.(openTradesExecutor)
	if !ok || lister == nil {
		return 0, false, nil
	}
	tradeID = strings.TrimSpace(tradeID)
	if tradeID == "" {
		return 0, false, nil
	}
	parsedID, err := strconv.Atoi(tradeID)
	if err != nil || parsedID <= 0 {
		return 0, false, nil
	}
	trades, err := lister.ListOpenTrades(ctx)
	if err != nil {
		return 0, false, err
	}
	for _, tr := range trades {
		if tr.ID == parsedID {
			amount := float64(tr.Amount)
			if amount > 0 {
				return amount, true, nil
			}
			return 0, false, nil
		}
	}
	return 0, false, nil
}

func resolveCloseQty(pos store.PositionRecord, plan risk.RiskPlan, trigger risk.RiskTrigger, statusQty float64) (float64, string) {
	closeQty := pos.Qty
	reason := "stop_loss_hit"
	if closeQty <= 0 && pos.InitialStake > 0 {
		closeQty = pos.InitialStake
	}
	switch trigger.Type {
	case "FORCE_EXIT":
		reason = strings.TrimSpace(trigger.Reason)
		if reason == "" {
			reason = "force_exit"
		}
	case "TAKE_PROFIT":
		reason = risk.FormatTPReason(trigger.LevelID)
		if trigger.QtyPct > 0 && trigger.QtyPct < 1 {
			baseQty := plan.InitialQty
			if statusQty > 0 {
				baseQty = statusQty
			}
			if baseQty <= 0 {
				baseQty = pos.InitialStake
			}
			if baseQty <= 0 {
				baseQty = pos.Qty
			}
			if baseQty > 0 {
				closeQty = baseQty * trigger.QtyPct
			}
		}
	}
	closeQty = floorCloseQty(closeQty)
	limitQty := pos.Qty
	if statusQty > 0 {
		limitQty = statusQty
	}
	if limitQty > 0 && closeQty > limitQty {
		closeQty = limitQty
	}
	if trigger.Type == "TAKE_PROFIT" && trigger.QtyPct == 1.0 && limitQty > 0 {
		dust := math.Max(limitQty*0.001, closeQtyPrecision)
		if limitQty-closeQty <= dust {
			closeQty = limitQty
		}
	}
	return closeQty, reason
}

const closeQtyPrecision = 1e-8

func floorCloseQty(value float64) float64 {
	if value <= 0 || closeQtyPrecision <= 0 {
		return value
	}
	return math.Floor(value/closeQtyPrecision) * closeQtyPrecision
}

func (m *RiskMonitor) handlePlanEntry(ctx context.Context, symbol string) error {
	entry, planSymbol, logger, ok := m.loadPlanEntry(ctx, symbol)
	if !ok {
		return nil
	}
	expired, err := m.expirePlanEntry(ctx, planSymbol, logger)
	if err != nil || expired {
		return err
	}
	if m.entryAlreadySubmitted(entry, logger) {
		return nil
	}
	quote, triggered, err := m.fetchTriggeredEntryQuote(ctx, entry.Plan, planSymbol, logger)
	if err != nil || !triggered {
		return err
	}
	acct, err := m.fetchAccountState(ctx, planSymbol, logger)
	if err != nil {
		return err
	}
	plan, err := m.refreshPlanSizing(entry.Plan, acct, logger)
	if err != nil {
		return err
	}
	if updated, ok := m.PlanCache.UpdatePlan(planSymbol, plan); ok {
		plan = updated.Plan
	}
	logger.Info("entry size refreshed",
		zap.Float64("available", acct.Available),
		zap.String("currency", strings.TrimSpace(acct.Currency)),
		zap.Float64("risk_pct", plan.RiskPct),
		zap.Float64("risk_distance", plan.RiskAnnotations.RiskDistance),
		zap.Float64("position_size", plan.PositionSize),
	)
	resp, err := m.Positions.SubmitOpenFromPlan(ctx, plan, quote.Price)
	if err != nil {
		return err
	}
	logger.Info("entry trigger hit",
		zap.Float64("mark_price", quote.Price),
		zap.Float64("entry_price", plan.Entry),
		zap.String("external_order_id", strings.TrimSpace(resp.ExternalID)),
	)
	return nil
}

func (m *RiskMonitor) loadPlanEntry(ctx context.Context, symbol string) (*PlanEntry, string, *zap.Logger, bool) {
	if strings.TrimSpace(symbol) == "" {
		return nil, "", nil, false
	}
	entry, ok := m.PlanCache.GetEntry(symbol)
	if !ok || entry == nil {
		return nil, "", nil, false
	}
	planSymbol := entry.Plan.Symbol
	if strings.TrimSpace(planSymbol) == "" {
		planSymbol = symbol
	}
	logger := logging.FromContext(ctx).Named("risk").With(
		zap.String("symbol", planSymbol),
		zap.String("position_id", entry.Plan.PositionID),
	)
	return entry, planSymbol, logger, true
}

func (m *RiskMonitor) expirePlanEntry(ctx context.Context, planSymbol string, logger *zap.Logger) (bool, error) {
	expired, expiredEntry := m.PlanCache.ExpireIfNeeded(planSymbol, time.Now().UTC())
	if !expired {
		return false, nil
	}
	if expiredEntry == nil {
		return true, nil
	}
	logger.Info("plan expired",
		zap.String("position_id", expiredEntry.Plan.PositionID),
		zap.String("symbol", planSymbol),
		zap.Float64("entry", expiredEntry.Plan.Entry),
		zap.Time("expires_at", expiredEntry.Plan.ExpiresAt),
	)
	reason := "plan_expired"
	if _, err := m.Positions.CancelOpenByEntry(ctx, *expiredEntry, reason); err != nil {
		logger.Error("open cancel failed", zap.Error(err), zap.String("reason", reason))
	} else {
		logger.Info("open cancel submitted", zap.String("reason", reason))
	}
	return true, nil
}

func (m *RiskMonitor) entryAlreadySubmitted(entry *PlanEntry, logger *zap.Logger) bool {
	if strings.TrimSpace(entry.ExternalID) == "" && strings.TrimSpace(entry.ClientOrderID) == "" {
		return false
	}
	logger.Debug("open already submitted",
		zap.String("client_order_id", strings.TrimSpace(entry.ClientOrderID)),
		zap.String("external_order_id", strings.TrimSpace(entry.ExternalID)),
		zap.Int64("submitted_at", entry.SubmittedAt),
	)
	return true
}

func (m *RiskMonitor) fetchTriggeredEntryQuote(ctx context.Context, plan execution.ExecutionPlan, planSymbol string, logger *zap.Logger) (market.PriceQuote, bool, error) {
	quote, err := m.PriceSource.MarkPrice(ctx, planSymbol)
	if err != nil {
		if errors.Is(err, market.ErrPriceUnavailable) {
			return market.PriceQuote{}, false, nil
		}
		return market.PriceQuote{}, false, err
	}
	if quote.Price == 0 {
		return market.PriceQuote{}, false, nil
	}
	if plan.Entry <= 0 {
		return market.PriceQuote{}, false, riskValidationErrorf("entry is required")
	}
	triggered := isEntryTriggered(plan.Direction, quote.Price, plan.Entry)
	if triggered {
		logger.Info("entry trigger check",
			zap.Float64("mark_price", quote.Price),
			zap.Float64("entry_price", plan.Entry),
			zap.String("side", plan.Direction),
		)
		return quote, true, nil
	}
	logger.Debug("entry trigger check",
		zap.Float64("mark_price", quote.Price),
		zap.Float64("entry_price", plan.Entry),
		zap.String("side", plan.Direction),
	)
	return market.PriceQuote{}, false, nil
}

func (m *RiskMonitor) fetchAccountState(ctx context.Context, planSymbol string, logger *zap.Logger) (execution.AccountState, error) {
	if m.AccountFetcher == nil {
		return execution.AccountState{}, riskValidationErrorf("account_fetcher is required")
	}
	acct, err := m.AccountFetcher(ctx, planSymbol)
	if err != nil {
		logger.Warn("account balance unavailable", zap.Error(err))
		return execution.AccountState{}, err
	}
	if acct.Available <= 0 {
		logger.Warn("account available balance invalid", zap.Float64("available", acct.Available), zap.String("currency", strings.TrimSpace(acct.Currency)))
		return execution.AccountState{}, riskValidationErrorf("account available balance unavailable")
	}
	return acct, nil
}

func (m *RiskMonitor) refreshPlanSizing(plan execution.ExecutionPlan, acct execution.AccountState, logger *zap.Logger) (execution.ExecutionPlan, error) {
	riskDist := plan.RiskAnnotations.RiskDistance
	if riskDist <= 0 {
		return plan, riskValidationErrorf("risk_distance is required")
	}
	if plan.RiskPct <= 0 {
		return plan, riskValidationErrorf("risk_pct is required")
	}
	positionSize := (acct.Available * plan.RiskPct) / riskDist
	if positionSize <= 0 {
		return plan, riskValidationErrorf("position_size invalid")
	}
	maxInvestPct := plan.RiskAnnotations.MaxInvestPct
	if maxInvestPct <= 0 {
		maxInvestPct = 1
	}
	maxInvestAmt := acct.Available * maxInvestPct
	if maxInvestAmt <= 0 {
		maxInvestAmt = acct.Available
	}
	maxLeverage := plan.RiskAnnotations.MaxLeverage
	if maxLeverage <= 0 {
		maxLeverage = plan.Leverage
	}
	leverageResult := risk.ResolveLeverageAndLiquidation(plan.Entry, positionSize, maxInvestAmt, maxLeverage, plan.Direction)
	if risk.IsStopBeyondLiquidation(plan.Direction, plan.StopLoss, leverageResult.LiquidationPrice) {
		logger.Warn("stop loss beyond liquidation",
			zap.Float64("stop_loss", plan.StopLoss),
			zap.Float64("liquidation_price", leverageResult.LiquidationPrice),
			zap.Float64("leverage", leverageResult.Leverage),
		)
		return plan, riskValidationErrorf("stop loss beyond liquidation")
	}
	plan.PositionSize = leverageResult.PositionSize
	plan.Leverage = leverageResult.Leverage
	return plan, nil
}

func isEntryTriggered(side string, mark float64, entry float64) bool {
	switch strings.ToLower(strings.TrimSpace(side)) {
	case "short":
		return mark >= entry
	default:
		return mark <= entry
	}
}

func clampMinOneFloat(value float64) float64 {
	return math.Max(value, 1)
}

type riskValidationError struct {
	msg string
}

func (e riskValidationError) Error() string {
	return e.msg
}

func (e riskValidationError) Classification() errclass.Classification {
	return errclass.Classification{
		Kind:      "validation",
		Scope:     "risk",
		Retryable: false,
		Action:    "abort",
		Reason:    "invalid_risk_monitor",
	}
}

func riskValidationErrorf(format string, args ...any) error {
	return riskValidationError{msg: fmt.Sprintf(format, args...)}
}
