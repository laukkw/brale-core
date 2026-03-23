package decision

import (
	"encoding/json"
	"fmt"
	"strings"

	"brale-core/internal/decision/features"
	"brale-core/internal/execution"
	"brale-core/internal/risk"
	"brale-core/internal/strategy"
)

const (
	tightenV2GateATRChangePctMin = 0.4
	tightenV2ScoreThreshold      = 4.5

	tightenV2ScoreMonitorTag         = 2.0
	tightenV2ScoreMomentumStalling   = 1.0
	tightenV2ScoreDivergenceDetected = 1.5
	tightenV2ScoreThreatLevelHigh    = 1.0
	tightenV2ScoreCrowdingReversal   = 1.0
	tightenV2ScoreAdverseLiquidation = 2.0
	tightenV2ScoreNoiseHigh          = 0.5
)

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

func resolveTightenPlanSource(bind strategy.StrategyBinding) string {
	if strings.EqualFold(strings.TrimSpace(bind.RiskManagement.RiskStrategy.Mode), execution.PlanSourceLLM) {
		return execution.PlanSourceLLM
	}
	return execution.PlanSourceGo
}

func riskPlanTakeProfits(plan risk.RiskPlan) []float64 {
	out := make([]float64, 0, len(plan.TPLevels))
	for _, level := range plan.TPLevels {
		if level.Price > 0 {
			out = append(out, level.Price)
		}
	}
	return out
}

func applyTightenRiskPatch(plan risk.RiskPlan, side string, entry float64, markPrice float64, patch *TightenRiskUpdatePatch) (risk.RiskPlan, bool, error) {
	if patch == nil || patch.StopLoss == nil {
		return plan, false, fmt.Errorf("tighten llm patch stop_loss is required")
	}
	if len(patch.TakeProfits) == 0 {
		return plan, false, fmt.Errorf("tighten llm patch take_profits is required")
	}
	stop := *patch.StopLoss
	if stop <= 0 {
		return plan, false, fmt.Errorf("tighten llm patch stop_loss must be > 0")
	}
	direction := strings.ToLower(strings.TrimSpace(side))
	if direction != "long" && direction != "short" {
		return plan, false, fmt.Errorf("tighten llm patch side must be long/short")
	}
	if !isStopImproved(direction, plan.StopPrice, stop, markPrice) {
		return plan, false, fmt.Errorf("tighten llm patch stop_loss is not improved")
	}
	last := entry
	for _, tp := range patch.TakeProfits {
		if tp <= 0 {
			return plan, false, fmt.Errorf("tighten llm patch take_profit must be > 0")
		}
		if direction == "long" {
			if tp <= last {
				return plan, false, fmt.Errorf("tighten llm patch take_profits must be strictly increasing")
			}
		} else {
			if tp >= last {
				return plan, false, fmt.Errorf("tighten llm patch take_profits must be strictly decreasing")
			}
		}
		last = tp
	}
	plan.StopPrice = stop
	tpTightened := false
	for idx := range plan.TPLevels {
		if idx >= len(patch.TakeProfits) {
			break
		}
		next := patch.TakeProfits[idx]
		if plan.TPLevels[idx].Price != next {
			plan.TPLevels[idx].Price = next
			tpTightened = true
		}
	}
	return plan, tpTightened, nil
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
