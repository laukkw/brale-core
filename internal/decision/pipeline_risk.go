package decision

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"brale-core/internal/decision/features"
	"brale-core/internal/execution"
	"brale-core/internal/market"
	"brale-core/internal/pkg/errclass"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/pkg/parseutil"
	"brale-core/internal/position"
	"brale-core/internal/risk"
	"brale-core/internal/store"
	"brale-core/internal/strategy"

	"go.uber.org/zap"
)

func (p *Pipeline) applyRiskPlanUpdate(ctx context.Context, res SymbolResult, comp features.CompressionResult, posID string) (tightenExecution, error) {
	exec := newTightenExecution(res, comp)
	if p.RiskPlans == nil {
		exec.addBlocked(tightenBlockRiskPlanDisabled)
		return exec, nil
	}
	pos, plan, ok, err := p.loadRiskPlanForUpdate(ctx, res.Symbol, posID)
	if err != nil {
		return exec, err
	}
	if !ok {
		exec.addBlocked(tightenBlockRiskPlanMissing)
		return exec, nil
	}
	if !exec.Eligible {
		return exec, nil
	}
	if blocked, debounceSec, debounceRemain := p.isTightenDebounced(ctx, res.Symbol, pos); blocked {
		exec.DebounceSec = debounceSec
		exec.DebounceRemain = debounceRemain
		exec.Eligible = false
		exec.addBlocked(tightenBlockDebounce)
		return exec, nil
	}
	updateCtx, reason, err := p.buildTightenContext(ctx, res, comp, exec)
	if err != nil {
		if reason != "" {
			exec.addBlocked(reason)
		}
		return exec, err
	}
	if reason != "" {
		exec.addBlocked(reason)
		return exec, nil
	}
	executed, tpTightened, exitConfirmHit, err := p.applyTightenUpdate(ctx, pos, plan, updateCtx)
	if err != nil {
		return exec, err
	}
	exec.Executed = executed
	exec.TPTightened = tpTightened
	exec.ExitConfirmHit = exitConfirmHit
	if !executed {
		exec.addBlocked(tightenBlockNoTightenNeeded)
	}
	return exec, nil
}

type tightenContext struct {
	Binding        strategy.StrategyBinding
	MarkPrice      float64
	ATR            float64
	ATRChangePct   float64
	ATRChangePctOK bool
	GateSatisfied  bool
	ScoreBreakdown []RiskPlanUpdateScoreItem
	ScoreTotal     float64
	ScoreParseOK   bool
	CriticalExit   bool
}

type tightenExecution struct {
	Action         string
	Evaluated      bool
	Eligible       bool
	Executed       bool
	BlockedBy      []string
	MonitorGateHit bool
	DebounceSec    int64
	DebounceRemain int64
	ATRChangePct   float64
	ATRChangePctOK bool
	ATRThreshold   float64
	GateSatisfied  bool
	ScoreTotal     float64
	ScoreThreshold float64
	ScoreParseOK   bool
	ScoreBreakdown []RiskPlanUpdateScoreItem
	TPTightened    bool
	ExitConfirmHit bool
}

const (
	tightenBlockMonitorGate      = "monitor_gate"
	tightenBlockATRMissing       = "atr_missing"
	tightenBlockATRGate          = "atr_gate"
	tightenBlockATRValueMissing  = "atr_value_missing"
	tightenBlockScoreThreshold   = "score_threshold"
	tightenBlockScoreParseFailed = "score_parse"
	tightenBlockRiskPlanMissing  = "risk_plan_missing"
	tightenBlockRiskPlanDisabled = "risk_plan_disabled"
	tightenBlockPriceUnavailable = "price_unavailable"
	tightenBlockPriceSourceMiss  = "price_source_missing"
	tightenBlockBindingMissing   = "binding_missing"
	tightenBlockNoTightenNeeded  = "no_tighten_needed"
	tightenBlockNotEvaluated     = "not_evaluated"
	tightenBlockDebounce         = "tighten_debounce"
)

func newTightenExecution(res SymbolResult, comp features.CompressionResult) tightenExecution {
	if !strings.EqualFold(strings.TrimSpace(res.Gate.DecisionAction), "TIGHTEN") {
		return tightenExecution{}
	}
	exec := tightenExecution{
		Action:         "tighten",
		Evaluated:      res.InPositionEvaluated,
		ScoreThreshold: tightenV2ScoreThreshold,
		ATRThreshold:   tightenV2GateATRChangePctMin,
	}
	if !exec.Evaluated {
		exec.addBlocked(tightenBlockNotEvaluated)
		return exec
	}
	exec.MonitorGateHit = resolveMonitorGateHit(res.InPositionIndicator.MonitorTag, res.InPositionStructure.MonitorTag, res.InPositionMechanics.MonitorTag)
	exec.ATRChangePct, exec.ATRChangePctOK = readIndicatorATRChangePct(comp, res.Symbol)
	if !exec.MonitorGateHit {
		exec.addBlocked(tightenBlockMonitorGate)
	}
	if !exec.ATRChangePctOK {
		exec.addBlocked(tightenBlockATRMissing)
	} else if math.Abs(exec.ATRChangePct) < tightenV2GateATRChangePctMin {
		exec.addBlocked(tightenBlockATRGate)
	}
	exec.GateSatisfied = exec.MonitorGateHit && exec.ATRChangePctOK && math.Abs(exec.ATRChangePct) >= tightenV2GateATRChangePctMin
	exec.ScoreBreakdown, exec.ScoreTotal, exec.ScoreParseOK = buildTightenV2ScoreBreakdown(res, exec.ATRChangePctOK)
	if !exec.ScoreParseOK && exec.ATRChangePctOK {
		exec.addBlocked(tightenBlockScoreParseFailed)
	}
	if exec.ScoreParseOK && exec.ScoreTotal < tightenV2ScoreThreshold {
		exec.addBlocked(tightenBlockScoreThreshold)
	}
	exec.Eligible = exec.GateSatisfied && exec.ScoreParseOK && exec.ScoreTotal >= tightenV2ScoreThreshold
	return exec
}

func (e *tightenExecution) addBlocked(reason string) {
	if strings.TrimSpace(reason) == "" {
		return
	}
	for _, item := range e.BlockedBy {
		if item == reason {
			return
		}
	}
	e.BlockedBy = append(e.BlockedBy, reason)
}

func (e tightenExecution) toMap() map[string]any {
	out := map[string]any{
		"action":       e.Action,
		"evaluated":    e.Evaluated,
		"eligible":     e.Eligible,
		"executed":     e.Executed,
		"tp_tightened": e.TPTightened,
	}
	if e.ExitConfirmHit {
		out["exit_confirm_requested"] = true
	}
	if len(e.BlockedBy) > 0 {
		out["blocked_by"] = e.BlockedBy
	}
	out["gate"] = map[string]any{
		"monitor_gate_hit":       e.MonitorGateHit,
		"debounce_sec":           e.DebounceSec,
		"debounce_remaining_sec": e.DebounceRemain,
		"atr_change_pct":         e.ATRChangePct,
		"atr_change_pct_ok":      e.ATRChangePctOK,
		"atr_threshold":          e.ATRThreshold,
		"gate_satisfied":         e.GateSatisfied,
	}
	out["score"] = map[string]any{
		"total":     e.ScoreTotal,
		"threshold": e.ScoreThreshold,
		"parse_ok":  e.ScoreParseOK,
		"breakdown": formatTightenScoreBreakdown(e.ScoreBreakdown),
	}
	return out
}

func formatTightenScoreBreakdown(items []RiskPlanUpdateScoreItem) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]any{
			"signal":       item.Signal,
			"weight":       item.Weight,
			"value":        item.Value,
			"contribution": item.Contribution,
		})
	}
	return out
}

func applyTightenExecutionDerived(res *SymbolResult, exec tightenExecution) {
	if res == nil || strings.TrimSpace(exec.Action) == "" {
		return
	}
	if res.Gate.Derived == nil {
		res.Gate.Derived = map[string]any{}
	}
	res.Gate.Derived["execution"] = exec.toMap()
}

func deriveCurrentPrice(derived map[string]any) float64 {
	if len(derived) == 0 {
		return 0
	}
	value, ok := derived["current_price"]
	if !ok || value == nil {
		return 0
	}
	return parseutil.Float(value)
}

func (p *Pipeline) loadRiskPlanForUpdate(ctx context.Context, symbol string, posID string) (store.PositionRecord, risk.RiskPlan, bool, error) {
	pos, err := p.loadPositionRecord(ctx, symbol, posID)
	if err != nil {
		return store.PositionRecord{}, risk.RiskPlan{}, false, err
	}
	if p.Positioner != nil && p.Positioner.Cache != nil {
		pos = p.Positioner.Cache.HydratePosition(pos)
	}
	if len(pos.RiskJSON) == 0 {
		p.notifyMissingRiskPlan(ctx, pos)
		return store.PositionRecord{}, risk.RiskPlan{}, false, nil
	}
	plan, decodeErr := position.DecodeRiskPlan(pos.RiskJSON)
	if decodeErr != nil {
		return store.PositionRecord{}, risk.RiskPlan{}, false, fmt.Errorf("decode risk plan for position %s: %w", strings.TrimSpace(pos.PositionID), decodeErr)
	}
	return pos, plan, true, nil
}

func (p *Pipeline) isTightenDebounced(ctx context.Context, symbol string, pos store.PositionRecord) (bool, int64, int64) {
	if p == nil {
		return false, 0, 0
	}
	bind, err := p.getBinding(symbol)
	if err != nil {
		return false, 0, 0
	}
	minIntervalSec := bind.RiskManagement.TightenATR.MinUpdateIntervalSec
	if minIntervalSec <= 0 || p.Store == nil || strings.TrimSpace(pos.PositionID) == "" {
		return false, minIntervalSec, 0
	}
	latest, ok, err := p.Store.FindLatestRiskPlanHistory(ctx, pos.PositionID)
	if err != nil || !ok {
		return false, minIntervalSec, 0
	}
	source := strings.ToLower(strings.TrimSpace(latest.Source))
	if source != "monitor-tighten" && source != "monitor-breakeven" {
		return false, minIntervalSec, 0
	}
	if latest.CreatedAt.IsZero() {
		return false, minIntervalSec, 0
	}
	elapsedSec := int64(time.Since(latest.CreatedAt).Seconds())
	remainingSec := minIntervalSec - elapsedSec
	if remainingSec > 0 {
		return true, minIntervalSec, remainingSec
	}
	return false, minIntervalSec, 0
}

func (p *Pipeline) buildTightenContext(ctx context.Context, res SymbolResult, comp features.CompressionResult, exec tightenExecution) (tightenContext, string, error) {
	_, atr, err := pickIndicatorValues(comp, res.Symbol)
	if err != nil || atr <= 0 {
		return tightenContext{}, tightenBlockATRValueMissing, nil
	}
	markPrice := deriveCurrentPrice(res.Gate.Derived)
	if markPrice <= 0 {
		if p.PriceSource == nil {
			return tightenContext{}, tightenBlockPriceSourceMiss, nil
		}
		quote, err := p.PriceSource.MarkPrice(ctx, res.Symbol)
		if err != nil {
			if errors.Is(err, market.ErrPriceUnavailable) {
				return tightenContext{}, tightenBlockPriceUnavailable, nil
			}
			return tightenContext{}, tightenBlockPriceUnavailable, err
		}
		markPrice = quote.Price
		if markPrice <= 0 {
			return tightenContext{}, tightenBlockPriceUnavailable, nil
		}
	}
	bind, err := p.getBinding(res.Symbol)
	if err != nil {
		return tightenContext{}, tightenBlockBindingMissing, err
	}
	return tightenContext{
		Binding:        bind,
		MarkPrice:      markPrice,
		ATR:            atr,
		ATRChangePct:   exec.ATRChangePct,
		ATRChangePctOK: exec.ATRChangePctOK,
		GateSatisfied:  exec.GateSatisfied,
		ScoreBreakdown: exec.ScoreBreakdown,
		ScoreTotal:     exec.ScoreTotal,
		ScoreParseOK:   exec.ScoreParseOK,
		CriticalExit: strings.EqualFold(strings.TrimSpace(res.InPositionStructure.MonitorTag), "exit") &&
			strings.EqualFold(strings.TrimSpace(string(res.InPositionStructure.ThreatLevel)), "critical"),
	}, "", nil
}

func (p *Pipeline) applyTightenUpdate(ctx context.Context, pos store.PositionRecord, plan risk.RiskPlan, updateCtx tightenContext) (bool, bool, bool, error) {
	oldStop := plan.StopPrice
	plan, watermarksUpdated := updateWatermarks(plan, pos.Side, pos.AvgEntry, updateCtx.MarkPrice)
	if tp1Hit(plan) {
		executed, tpTightened, err := p.applyBreakevenUpdate(ctx, pos, plan, oldStop, updateCtx, watermarksUpdated)
		return executed, tpTightened, false, err
	}
	return p.applyStructureTighten(ctx, pos, plan, oldStop, updateCtx, watermarksUpdated)
}

func (p *Pipeline) applyBreakevenUpdate(ctx context.Context, pos store.PositionRecord, plan risk.RiskPlan, oldStop float64, updateCtx tightenContext, watermarksUpdated bool) (bool, bool, error) {
	plan.StopPrice = computeBreakevenStop(pos.Side, pos.AvgEntry, updateCtx.Binding.RiskManagement.BreakevenFeePct)
	if plan.StopPrice <= 0 {
		return false, false, p.applyWatermarkUpdate(ctx, pos, plan, "monitor-breakeven", watermarksUpdated)
	}
	_, err := p.RiskPlans.ApplyUpdate(ctx, pos.PositionID, plan, "monitor-breakeven")
	executed := err == nil && plan.StopPrice != oldStop
	if executed {
		p.logRiskPlanUpdate(ctx, pos, plan, oldStop, "monitor-breakeven", updateCtx.MarkPrice, updateCtx.ATR, updateCtx.ATRChangePct, updateCtx.GateSatisfied, updateCtx.ScoreTotal, tightenV2ScoreThreshold, updateCtx.ScoreBreakdown, updateCtx.ScoreParseOK, "monitor-breakeven", false)
	}
	return executed, false, err
}

func (p *Pipeline) applyStructureTighten(ctx context.Context, pos store.PositionRecord, plan risk.RiskPlan, oldStop float64, updateCtx tightenContext, watermarksUpdated bool) (bool, bool, bool, error) {
	tightenMultiplier := updateCtx.Binding.RiskManagement.TightenATR.StructureThreatened
	if tightenMultiplier <= 0 {
		return false, false, false, p.applyWatermarkUpdate(ctx, pos, plan, "monitor-tighten", watermarksUpdated)
	}
	anchor := resolveTightenAnchor(plan, pos.Side, updateCtx.MarkPrice)
	newStop := computeTightenStop(pos.Side, anchor, updateCtx.ATR, tightenMultiplier, updateCtx.Binding.RiskManagement.SlippageBufferPct)
	if newStop <= 0 {
		return false, false, false, p.applyWatermarkUpdate(ctx, pos, plan, "monitor-tighten", watermarksUpdated)
	}
	if !isStopImproved(pos.Side, plan.StopPrice, newStop, updateCtx.MarkPrice) {
		crossed := isStopCrossed(pos.Side, newStop, updateCtx.MarkPrice)
		if crossed {
			fallbackStop := computeTightenStop(pos.Side, updateCtx.MarkPrice, updateCtx.ATR, tightenMultiplier, updateCtx.Binding.RiskManagement.SlippageBufferPct)
			if fallbackStop > 0 && isStopImproved(pos.Side, plan.StopPrice, fallbackStop, updateCtx.MarkPrice) {
				newStop = fallbackStop
			} else {
				if updateCtx.CriticalExit {
					return false, false, true, p.applyWatermarkUpdate(ctx, pos, plan, "monitor-tighten", watermarksUpdated)
				}
				return false, false, false, p.applyWatermarkUpdate(ctx, pos, plan, "monitor-tighten", watermarksUpdated)
			}
		} else {
			return false, false, false, p.applyWatermarkUpdate(ctx, pos, plan, "monitor-tighten", watermarksUpdated)
		}
	}
	plan.StopPrice = newStop
	plan, tpTightened := risk.TightenTPLevels(
		plan,
		pos.Side,
		pos.AvgEntry,
		updateCtx.ATR,
		updateCtx.Binding.RiskManagement.TightenATR.TP1ATR,
		updateCtx.Binding.RiskManagement.TightenATR.TP2ATR,
		updateCtx.Binding.RiskManagement.TightenATR.MinTPDistancePct,
		updateCtx.Binding.RiskManagement.TightenATR.MinTPGapPct,
	)
	_, err := p.RiskPlans.ApplyUpdate(ctx, pos.PositionID, plan, "monitor-tighten")
	executed := err == nil && plan.StopPrice != oldStop
	if executed {
		p.logRiskPlanUpdate(ctx, pos, plan, oldStop, "monitor-tighten", updateCtx.MarkPrice, updateCtx.ATR, updateCtx.ATRChangePct, updateCtx.GateSatisfied, updateCtx.ScoreTotal, tightenV2ScoreThreshold, updateCtx.ScoreBreakdown, updateCtx.ScoreParseOK, "monitor-tighten", tpTightened)
	}
	return executed, tpTightened, false, err
}

func (p *Pipeline) applyWatermarkUpdate(ctx context.Context, pos store.PositionRecord, plan risk.RiskPlan, source string, updated bool) error {
	if !updated {
		return nil
	}
	_, err := p.RiskPlans.ApplyUpdate(ctx, pos.PositionID, plan, source)
	return err
}

func (p *Pipeline) logRiskPlanUpdate(ctx context.Context, pos store.PositionRecord, plan risk.RiskPlan, oldStop float64, source string, markPrice float64, atr float64, volatility float64, gateSatisfied bool, scoreTotal float64, scoreThreshold float64, scoreBreakdown []RiskPlanUpdateScoreItem, parseOK bool, tightenReason string, tpTightened bool) {
	logger := logging.FromContext(ctx).Named("risk")
	tpPrices := make([]float64, 0, len(plan.TPLevels))
	for _, level := range plan.TPLevels {
		if level.Price > 0 {
			tpPrices = append(tpPrices, level.Price)
		}
	}
	logger.Info("risk plan updated",
		zap.String("symbol", pos.Symbol),
		zap.String("position_id", pos.PositionID),
		zap.String("direction", strings.TrimSpace(pos.Side)),
		zap.Float64("entry", pos.AvgEntry),
		zap.Float64("stop_prev", oldStop),
		zap.Float64("stop_new", plan.StopPrice),
		zap.Float64s("take_profits", tpPrices),
		zap.String("source", source),
		zap.Float64("mark_price", markPrice),
		zap.Float64("atr", atr),
		zap.Float64("risk_pct", pos.RiskPct),
		zap.Float64("leverage", pos.Leverage),
	)
	if p.Notifier == nil {
		return
	}
	if err := p.Notifier.SendRiskPlanUpdate(ctx, RiskPlanUpdateNotice{
		Symbol:         pos.Symbol,
		Direction:      strings.TrimSpace(pos.Side),
		EntryPrice:     pos.AvgEntry,
		OldStop:        oldStop,
		NewStop:        plan.StopPrice,
		TakeProfits:    tpPrices,
		Source:         source,
		MarkPrice:      markPrice,
		ATR:            atr,
		Volatility:     volatility,
		GateSatisfied:  gateSatisfied,
		ScoreTotal:     scoreTotal,
		ScoreThreshold: scoreThreshold,
		ScoreBreakdown: scoreBreakdown,
		ParseOK:        parseOK,
		TightenReason:  tightenReason,
		TPTightened:    tpTightened,
		RiskPct:        pos.RiskPct,
		Leverage:       pos.Leverage,
		PositionID:     pos.PositionID,
	}); err != nil {
		logger.Error("risk plan notify failed", zap.Error(err))
	}
}

func (p *Pipeline) notifyMissingRiskPlan(ctx context.Context, pos store.PositionRecord) {
	logger := logging.FromContext(ctx).Named("risk")
	logger.Warn("missing risk plan on tighten",
		zap.String("symbol", pos.Symbol),
		zap.String("direction", strings.TrimSpace(pos.Side)),
		zap.Float64("qty", pos.Qty),
		zap.Float64("leverage", pos.Leverage),
		zap.Time("opened_at", pos.CreatedAt),
	)
	if p.Notifier == nil {
		return
	}
	openedAt := "-"
	if !pos.CreatedAt.IsZero() {
		openedAt = pos.CreatedAt.Format(time.RFC3339)
	}
	message := fmt.Sprintf(
		"风控计划缺失，无法更新止损。\n- 币种: %s\n- 方向: %s\n- 持仓大小: %s\n- 杠杆倍数: %s\n- 开仓时间: %s\n该仓位并未找到开仓计划，请手动处理。",
		strings.TrimSpace(pos.Symbol),
		strings.TrimSpace(pos.Side),
		formatRiskFloat(pos.Qty),
		formatRiskFloat(pos.Leverage),
		openedAt,
	)
	if err := p.Notifier.SendError(ctx, message); err != nil {
		logger.Error("missing risk plan notify failed", zap.Error(err))
	}
}

func formatRiskFloat(value float64) string {
	if value == 0 {
		return "0"
	}
	text := fmt.Sprintf("%.8f", value)
	text = strings.TrimRight(text, "0")
	text = strings.TrimRight(text, ".")
	return text
}

const (
	// Tighten V2 触发门槛：任一路 monitor_tag=tighten + ATR 波动变化百分比。
	tightenV2GateATRChangePctMin = 0.4

	// Tighten V2 评分阈值（总分达到才触发）。
	tightenV2ScoreThreshold = 4.5

	// Tighten V2 评分权重：monitor_tag=tighten。
	tightenV2ScoreMonitorTag = 2.0
	// Tighten V2 评分权重：动能不持续（momentum_sustaining=false）。
	tightenV2ScoreMomentumStalling = 1.0
	// Tighten V2 评分权重：背离出现（divergence_detected=true）。
	tightenV2ScoreDivergenceDetected = 1.5
	// Tighten V2 评分权重：结构威胁为 high。
	tightenV2ScoreThreatLevelHigh = 1.0
	// Tighten V2 评分权重：拥挤反转。
	tightenV2ScoreCrowdingReversal = 1.0
	// Tighten V2 评分权重：清算对手盘风险。
	tightenV2ScoreAdverseLiquidation = 2.0
	// Tighten V2 评分权重：噪音为 high。
	tightenV2ScoreNoiseHigh = 0.5
)

func validatePlan(plan execution.ExecutionPlan) error {
	if plan.PositionID == "" {
		return planValidationErrorf("plan position_id is required")
	}
	if plan.CreatedAt.IsZero() {
		return planValidationErrorf("plan created_at is required")
	}
	return nil
}

const validationScope errclass.Scope = "decision"
const validationReason = "invalid_plan"

func planValidationErrorf(format string, args ...any) error {
	return errclass.ValidationErrorf(validationScope, validationReason, format, args...)
}

func resolveMonitorGateHit(indicatorTag string, structureTag string, mechanicsTag string) bool {
	return strings.EqualFold(strings.TrimSpace(indicatorTag), "tighten") ||
		strings.EqualFold(strings.TrimSpace(structureTag), "tighten") ||
		strings.EqualFold(strings.TrimSpace(mechanicsTag), "tighten")
}

func readIndicatorATRChangePct(comp features.CompressionResult, symbol string) (float64, bool) {
	indicatorJSON, ok := PickIndicatorJSON(comp, symbol)
	if !ok {
		return 0, false
	}
	var payload struct {
		Data struct {
			ATR struct {
				ChangePct *float64 `json:"change_pct"`
			} `json:"atr"`
		} `json:"data"`
	}
	if err := json.Unmarshal(indicatorJSON.RawJSON, &payload); err != nil {
		return 0, false
	}
	if payload.Data.ATR.ChangePct == nil {
		return 0, false
	}
	return *payload.Data.ATR.ChangePct, true
}

func buildTightenV2ScoreBreakdown(res SymbolResult, atrChangePctOK bool) ([]RiskPlanUpdateScoreItem, float64, bool) {
	items := make([]RiskPlanUpdateScoreItem, 0, 7)
	if !res.InPositionEvaluated {
		return items, 0, false
	}
	total := 0.0
	parseOK := atrChangePctOK
	indicatorTag := strings.ToLower(strings.TrimSpace(res.InPositionIndicator.MonitorTag))
	structureTag := strings.ToLower(strings.TrimSpace(res.InPositionStructure.MonitorTag))
	mechanicsTag := strings.ToLower(strings.TrimSpace(res.InPositionMechanics.MonitorTag))
	monitorTagHit := indicatorTag == "tighten" || structureTag == "tighten" || mechanicsTag == "tighten"
	items, total = appendScoreItem(items, total, "monitor_tag", tightenV2ScoreMonitorTag, formatTightenValueBool(monitorTagHit), monitorTagHit)
	items, total = appendScoreItem(items, total, "momentum_stalling", tightenV2ScoreMomentumStalling, formatTightenValueBool(!res.InPositionIndicator.MomentumSustaining), !res.InPositionIndicator.MomentumSustaining)
	items, total = appendScoreItem(items, total, "divergence_detected", tightenV2ScoreDivergenceDetected, formatTightenValueBool(res.InPositionIndicator.DivergenceDetected), res.InPositionIndicator.DivergenceDetected)
	threatLevel := strings.ToLower(strings.TrimSpace(string(res.InPositionStructure.ThreatLevel)))
	threatLevelHigh := threatLevel == "high" || threatLevel == "critical"
	items, total = appendScoreItem(items, total, "threat_level_high", tightenV2ScoreThreatLevelHigh, threatLevel, threatLevelHigh)
	items, total = appendScoreItem(items, total, "crowding_reversal", tightenV2ScoreCrowdingReversal, formatTightenValueBool(res.InPositionMechanics.CrowdingReversal), res.InPositionMechanics.CrowdingReversal)
	items, total = appendScoreItem(items, total, "adverse_liquidation", tightenV2ScoreAdverseLiquidation, formatTightenValueBool(res.InPositionMechanics.AdverseLiquidation), res.InPositionMechanics.AdverseLiquidation)
	items, total = appendScoreItem(items, total, "noise_high", tightenV2ScoreNoiseHigh, strings.ToLower(strings.TrimSpace(string(res.AgentIndicator.Noise))), strings.EqualFold(string(res.AgentIndicator.Noise), "high"))
	if !atrChangePctOK {
		parseOK = false
	}
	return items, total, parseOK
}

func appendScoreItem(items []RiskPlanUpdateScoreItem, total float64, signal string, weight float64, value string, hit bool) ([]RiskPlanUpdateScoreItem, float64) {
	contribution := 0.0
	if hit {
		contribution = weight
		total += weight
	}
	items = append(items, RiskPlanUpdateScoreItem{
		Signal:       signal,
		Weight:       weight,
		Value:        value,
		Contribution: contribution,
	})
	return items, total
}

func formatTightenValueBool(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func pickIndicatorValues(comp features.CompressionResult, symbol string) (float64, float64, error) {
	indicator, err := PickIndicator(comp, symbol)
	if err != nil {
		return 0, 0, err
	}
	return indicator.Close, indicator.ATR, nil
}

func computeTightenStop(side string, anchorPrice float64, atr float64, multiplier float64, slippagePct float64) float64 {
	if atr <= 0 || multiplier <= 0 || anchorPrice <= 0 {
		return 0
	}
	if strings.EqualFold(side, "short") {
		stop := anchorPrice + atr*multiplier
		return applySlippageBuffer(side, stop, slippagePct)
	}
	stop := anchorPrice - atr*multiplier
	return applySlippageBuffer(side, stop, slippagePct)
}

func applySlippageBuffer(side string, stop float64, slippagePct float64) float64 {
	if stop <= 0 || slippagePct <= 0 {
		return stop
	}
	if strings.EqualFold(side, "short") {
		return stop * (1 + slippagePct)
	}
	return stop * (1 - slippagePct)
}

func updateWatermarks(plan risk.RiskPlan, side string, entry float64, markPrice float64) (risk.RiskPlan, bool) {
	updated := false
	if entry > 0 {
		if plan.HighWaterMark <= 0 {
			plan.HighWaterMark = entry
			updated = true
		}
		if plan.LowWaterMark <= 0 {
			plan.LowWaterMark = entry
			updated = true
		}
	}
	if markPrice <= 0 {
		return plan, updated
	}
	if strings.EqualFold(side, "short") {
		if plan.LowWaterMark <= 0 || markPrice < plan.LowWaterMark {
			plan.LowWaterMark = markPrice
			updated = true
		}
		return plan, updated
	}
	if plan.HighWaterMark <= 0 || markPrice > plan.HighWaterMark {
		plan.HighWaterMark = markPrice
		updated = true
	}
	return plan, updated
}

func resolveTightenAnchor(plan risk.RiskPlan, side string, markPrice float64) float64 {
	if strings.EqualFold(side, "short") {
		if plan.LowWaterMark > 0 {
			return plan.LowWaterMark
		}
		return markPrice
	}
	if plan.HighWaterMark > 0 {
		return plan.HighWaterMark
	}
	return markPrice
}

func tp1Hit(plan risk.RiskPlan) bool {
	if len(plan.TPLevels) == 0 {
		return false
	}
	return plan.TPLevels[0].Hit
}

func computeBreakevenStop(side string, entry float64, feePct float64) float64 {
	if entry <= 0 {
		return 0
	}
	fee := entry * feePct
	if strings.EqualFold(side, "short") {
		return entry - fee
	}
	return entry + fee
}

func isStopImproved(side string, prevStop float64, nextStop float64, currentPrice float64) bool {
	if currentPrice <= 0 {
		return false
	}
	if strings.EqualFold(side, "short") {
		return nextStop < prevStop && nextStop > currentPrice
	}
	return nextStop > prevStop && nextStop < currentPrice
}

func isStopCrossed(side string, stop float64, currentPrice float64) bool {
	if stop <= 0 || currentPrice <= 0 {
		return false
	}
	if strings.EqualFold(side, "short") {
		return stop <= currentPrice
	}
	return stop >= currentPrice
}
