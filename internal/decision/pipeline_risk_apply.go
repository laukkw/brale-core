package decision

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"brale-core/internal/decision/fund"
	"brale-core/internal/execution"
	"brale-core/internal/llm"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/pkg/parseutil"
	"brale-core/internal/risk"
	"brale-core/internal/store"

	"go.uber.org/zap"
)

type tightenUpdateResult struct {
	Executed       bool
	TPTightened    bool
	ExitConfirmHit bool
	PlanSource     string
	StopLoss       float64
	TakeProfits    []float64
	LLMRiskTrace   *execution.LLMRiskTrace
}

func newTightenUpdateResult(planSource string, stopLoss float64, takeProfits []float64) tightenUpdateResult {
	return tightenUpdateResult{
		PlanSource:  planSource,
		StopLoss:    stopLoss,
		TakeProfits: slices.Clone(takeProfits),
	}
}

func (p *Pipeline) applyTightenUpdate(ctx context.Context, pos store.PositionRecord, plan risk.RiskPlan, updateCtx tightenContext) (tightenUpdateResult, error) {
	oldStop := plan.StopPrice
	plan, watermarksUpdated := updateWatermarks(plan, pos.Side, pos.AvgEntry, updateCtx.MarkPrice)
	if tp1Hit(plan) {
		result, err := p.applyBreakevenUpdate(ctx, pos, plan, oldStop, updateCtx, watermarksUpdated)
		return result, err
	}
	return p.applyStructureTighten(ctx, pos, plan, oldStop, updateCtx, watermarksUpdated)
}

func (p *Pipeline) applyBreakevenUpdate(ctx context.Context, pos store.PositionRecord, plan risk.RiskPlan, oldStop float64, updateCtx tightenContext, watermarksUpdated bool) (tightenUpdateResult, error) {
	plan.StopPrice = computeBreakevenStop(pos.Side, pos.AvgEntry, updateCtx.Binding.RiskManagement.BreakevenFeePct)
	result := newTightenUpdateResult(execution.PlanSourceGo, plan.StopPrice, riskPlanTakeProfits(plan))
	if plan.StopPrice <= 0 {
		return result, p.applyWatermarkUpdate(ctx, pos, plan, "monitor-breakeven", watermarksUpdated)
	}
	err := p.withStoreTx(ctx, func(runCtx context.Context) error {
		if _, err := p.riskPlans().ApplyUpdate(runCtx, pos.PositionID, plan, "monitor-breakeven"); err != nil {
			return err
		}
		if plan.StopPrice == oldStop {
			return nil
		}
		return p.logRiskPlanUpdate(runCtx, pos, plan, oldStop, "monitor-breakeven", updateCtx.MarkPrice, updateCtx.ATR, updateCtx.ATRChangePct, updateCtx.GateSatisfied, updateCtx.ScoreTotal, tightenV2ScoreThreshold, updateCtx.ScoreBreakdown, updateCtx.ScoreParseOK, "monitor-breakeven", false)
	})
	result.Executed = err == nil && plan.StopPrice != oldStop
	return result, err
}

func (p *Pipeline) applyStructureTighten(ctx context.Context, pos store.PositionRecord, plan risk.RiskPlan, oldStop float64, updateCtx tightenContext, watermarksUpdated bool) (tightenUpdateResult, error) {
	tightenMultiplier := updateCtx.Binding.RiskManagement.TightenATR.StructureThreatened
	baseResult := newTightenUpdateResult(resolveTightenPlanSource(updateCtx.Binding), plan.StopPrice, riskPlanTakeProfits(plan))
	if tightenMultiplier <= 0 {
		return baseResult, p.applyWatermarkUpdate(ctx, pos, plan, "monitor-tighten", watermarksUpdated)
	}
	anchor := resolveTightenAnchor(plan, pos.Side, updateCtx.MarkPrice)
	newStop := computeTightenStop(pos.Side, anchor, updateCtx.ATR, tightenMultiplier, updateCtx.Binding.RiskManagement.SlippageBufferPct)
	if newStop <= 0 {
		return baseResult, p.applyWatermarkUpdate(ctx, pos, plan, "monitor-tighten", watermarksUpdated)
	}
	if !isStopImproved(pos.Side, plan.StopPrice, newStop, updateCtx.MarkPrice) {
		crossed := isStopCrossed(pos.Side, newStop, updateCtx.MarkPrice)
		if crossed {
			fallbackStop := computeTightenStop(pos.Side, updateCtx.MarkPrice, updateCtx.ATR, tightenMultiplier, updateCtx.Binding.RiskManagement.SlippageBufferPct)
			if fallbackStop > 0 && isStopImproved(pos.Side, plan.StopPrice, fallbackStop, updateCtx.MarkPrice) {
				newStop = fallbackStop
			} else {
				if updateCtx.CriticalExit {
					baseResult.ExitConfirmHit = true
					return baseResult, p.applyWatermarkUpdate(ctx, pos, plan, "monitor-tighten", watermarksUpdated)
				}
				return baseResult, p.applyWatermarkUpdate(ctx, pos, plan, "monitor-tighten", watermarksUpdated)
			}
		} else {
			return baseResult, p.applyWatermarkUpdate(ctx, pos, plan, "monitor-tighten", watermarksUpdated)
		}
	}
	planSource := resolveTightenPlanSource(updateCtx.Binding)
	tightenPlan, tpTightened, llmTrace, err := p.buildTightenPlan(ctx, pos, plan, updateCtx, newStop)
	if err != nil {
		return baseResult, err
	}
	plan = tightenPlan
	result := newTightenUpdateResult(planSource, plan.StopPrice, riskPlanTakeProfits(plan))
	result.LLMRiskTrace = cloneLLMRiskTrace(llmTrace)
	err = p.withStoreTx(ctx, func(runCtx context.Context) error {
		if _, err := p.riskPlans().ApplyUpdate(runCtx, pos.PositionID, plan, "monitor-tighten"); err != nil {
			return err
		}
		if plan.StopPrice == oldStop {
			return nil
		}
		return p.logRiskPlanUpdate(runCtx, pos, plan, oldStop, "monitor-tighten", updateCtx.MarkPrice, updateCtx.ATR, updateCtx.ATRChangePct, updateCtx.GateSatisfied, updateCtx.ScoreTotal, tightenV2ScoreThreshold, updateCtx.ScoreBreakdown, updateCtx.ScoreParseOK, "monitor-tighten", tpTightened)
	})
	result.Executed = err == nil && plan.StopPrice != oldStop
	result.TPTightened = tpTightened
	return result, err
}

func (p *Pipeline) buildTightenPlan(ctx context.Context, pos store.PositionRecord, plan risk.RiskPlan, updateCtx tightenContext, newStop float64) (risk.RiskPlan, bool, *execution.LLMRiskTrace, error) {
	planSource := resolveTightenPlanSource(updateCtx.Binding)
	tightenPlan := plan
	tightenPlan.StopPrice = newStop
	if planSource == execution.PlanSourceLLM {
		if p.TightenRiskLLM == nil {
			return plan, false, nil, fmt.Errorf("tighten risk llm callback is required")
		}
		runCtx := llm.WithSessionSymbol(ctx, pos.Symbol)
		runCtx = llm.WithSessionFlow(runCtx, llm.LLMFlowInPosition)
		patch, err := p.TightenRiskLLM(runCtx, TightenRiskUpdateInput{
			Symbol:              pos.Symbol,
			Gate:                updateCtx.Gate,
			Side:                pos.Side,
			Entry:               pos.AvgEntry,
			MarkPrice:           updateCtx.MarkPrice,
			ATR:                 updateCtx.ATR,
			UnrealizedPnlPct:    computeUnrealizedPnlPct(pos.Side, pos.AvgEntry, updateCtx.MarkPrice),
			PositionAgeMin:      computePositionAgeMin(pos.CreatedAt),
			TP1Hit:              tp1Hit(plan),
			DistanceToLiqPct:    computeDistanceToLiqPct(updateCtx.Gate, updateCtx.MarkPrice),
			CurrentStopLoss:     plan.StopPrice,
			CurrentTakeProfits:  riskPlanTakeProfits(plan),
			AgentIndicator:      updateCtx.AgentIndicator,
			AgentStructure:      updateCtx.AgentStructure,
			AgentMechanics:      updateCtx.AgentMechanics,
			StructureAnchors:    updateCtx.StructureAnchors,
			InPositionIndicator: updateCtx.InPosIndicator,
			InPositionStructure: updateCtx.InPosStructure,
			InPositionMechanics: updateCtx.InPosMechanics,
		})
		if err != nil {
			return plan, false, nil, err
		}
		nextPlan, tpTightened, err := applyTightenRiskPatch(tightenPlan, pos.Side, pos.AvgEntry, updateCtx.MarkPrice, patch)
		return nextPlan, tpTightened, cloneLLMRiskTrace(patch.Trace), err
	}
	tightenPlan, tpTightened := risk.TightenTPLevels(
		tightenPlan,
		pos.Side,
		pos.AvgEntry,
		updateCtx.ATR,
		updateCtx.Binding.RiskManagement.TightenATR.TP1ATR,
		updateCtx.Binding.RiskManagement.TightenATR.TP2ATR,
		updateCtx.Binding.RiskManagement.TightenATR.MinTPDistancePct,
		updateCtx.Binding.RiskManagement.TightenATR.MinTPGapPct,
	)
	return tightenPlan, tpTightened, nil, nil
}

func computeUnrealizedPnlPct(side string, entry, markPrice float64) float64 {
	if entry <= 0 || markPrice <= 0 {
		return 0
	}
	directionSign := 1.0
	if strings.EqualFold(strings.TrimSpace(side), "short") {
		directionSign = -1
	}
	return ((markPrice - entry) / entry) * directionSign
}

func computePositionAgeMin(createdAt time.Time) int64 {
	if createdAt.IsZero() {
		return 0
	}
	age := int64(time.Since(createdAt).Minutes())
	if age < 0 {
		return 0
	}
	return age
}

func computeDistanceToLiqPct(gate fund.GateDecision, markPrice float64) float64 {
	if markPrice <= 0 {
		return 0
	}
	liqPrice := extractLiquidationPrice(gate)
	if liqPrice <= 0 {
		return 0
	}
	return math.Abs(markPrice-liqPrice) / markPrice
}

func extractLiquidationPrice(gate fund.GateDecision) float64 {
	if len(gate.Derived) == 0 {
		return 0
	}
	planMap, ok := gate.Derived["plan"].(map[string]any)
	if !ok {
		return 0
	}
	return parseutil.Float(planMap["liquidation_price"])
}

func (p *Pipeline) applyWatermarkUpdate(ctx context.Context, pos store.PositionRecord, plan risk.RiskPlan, source string, updated bool) error {
	if !updated {
		return nil
	}
	_, err := p.riskPlans().ApplyUpdate(ctx, pos.PositionID, plan, source)
	return err
}

func (p *Pipeline) logRiskPlanUpdate(ctx context.Context, pos store.PositionRecord, plan risk.RiskPlan, oldStop float64, source string, markPrice float64, atr float64, volatility float64, gateSatisfied bool, scoreTotal float64, scoreThreshold float64, scoreBreakdown []RiskPlanUpdateScoreItem, parseOK bool, tightenReason string, tpTightened bool) error {
	logger := logging.FromContext(ctx).Named("risk")
	stopReason := strings.TrimSpace(tightenReason)
	if stopReason == "" {
		stopReason = strings.TrimSpace(source)
	}
	planStopReason := strings.TrimSpace(pos.StopReason)
	if planStopReason == "" {
		planStopReason = stopReason
	}
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
		zap.String("stop_reason", stopReason),
		zap.String("plan_stop_reason", planStopReason),
		zap.Float64("risk_pct", pos.RiskPct),
		zap.Float64("leverage", pos.Leverage),
	)
	if p.Notifier == nil {
		return nil
	}
	if err := p.Notifier.SendRiskPlanUpdate(ctx, RiskPlanUpdateNotice{
		Symbol:         pos.Symbol,
		Direction:      strings.TrimSpace(pos.Side),
		EntryPrice:     pos.AvgEntry,
		OldStop:        oldStop,
		NewStop:        plan.StopPrice,
		TakeProfits:    tpPrices,
		Source:         source,
		StopReason:     stopReason,
		Reason:         planStopReason,
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
		return err
	}
	return nil
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
	notice := ErrorNotice{
		Severity:  "warn",
		Component: "risk_monitor",
		Symbol:    strings.TrimSpace(pos.Symbol),
		Message:   message,
	}
	if err := p.Notifier.SendError(ctx, notice); err != nil {
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
