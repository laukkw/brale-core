package ruleflow

import (
	"strings"
	"time"

	"brale-core/internal/decision/fsm"
	"brale-core/internal/decision/fund"
	"brale-core/internal/execution"
)

func parseResult(data map[string]any) (Result, error) {
	plan := parsePlan(data)
	result := Result{
		Gate:       parseGate(data),
		Plan:       plan,
		FSMNext:    parseFSMNext(data),
		FSMRuleHit: parseFSMRuleHit(data),
	}
	result.FSMActions = parseFSMActions(data)
	result.ExitConfirmCount = toInt(data["exit_confirm_count"])
	return result, nil
}

func parseGate(data map[string]any) fund.GateDecision {
	gate := fund.GateDecision{}
	gateRaw, ok := data["gate"].(map[string]any)
	if !ok {
		return gate
	}
	gate.DecisionAction = toString(gateRaw["action"])
	gate.GateReason = toString(gateRaw["reason"])
	gate.Direction = toString(gateRaw["direction"])
	gate.Grade = toInt(gateRaw["grade"])
	gate.GlobalTradeable = toBool(gateRaw["tradeable"])
	if derived, ok := gateRaw["derived"].(map[string]any); ok {
		gate.Derived = derived
	}
	if hitRaw, ok := gateRaw["rule_hit"].(map[string]any); ok {
		gate.RuleHit = &fund.GateRuleHit{
			Name:      toString(hitRaw["name"]),
			Priority:  toInt(hitRaw["priority"]),
			Action:    toString(hitRaw["action"]),
			Reason:    toString(hitRaw["reason"]),
			Grade:     toInt(hitRaw["grade"]),
			Direction: toString(hitRaw["direction"]),
			Default:   toBool(hitRaw["default"]),
		}
	}
	return gate
}

func parsePlan(data map[string]any) *execution.ExecutionPlan {
	planMap, ok := data["plan"].(map[string]any)
	if !ok {
		return nil
	}
	plan := &execution.ExecutionPlan{
		Symbol:             toString(data["symbol"]),
		Valid:              toBool(planMap["valid"]),
		InvalidReason:      toString(planMap["invalid_reason"]),
		Direction:          toString(planMap["direction"]),
		Entry:              toFloat(planMap["entry"]),
		StopLoss:           toFloat(planMap["stop_loss"]),
		RiskPct:            toFloat(planMap["risk_pct"]),
		PositionSize:       toFloat(planMap["position_size"]),
		Leverage:           toFloat(planMap["leverage"]),
		RMultiple:          toFloat(planMap["r_multiple"]),
		Template:           toString(planMap["template"]),
		PositionID:         toString(planMap["position_id"]),
		StrategyID:         toString(planMap["strategy_id"]),
		SystemConfigHash:   toString(planMap["system_config_hash"]),
		StrategyConfigHash: toString(planMap["strategy_config_hash"]),
		CreatedAt:          time.Now().UTC(),
	}
	if riskRaw, ok := planMap["risk_annotations"].(map[string]any); ok {
		plan.RiskAnnotations = execution.RiskAnnotations{
			StopSource:   toString(riskRaw["stop_source"]),
			StopReason:   toString(riskRaw["stop_reason"]),
			RiskDistance: toFloat(riskRaw["risk_distance"]),
			ATR:          toFloat(riskRaw["atr"]),
			BufferATR:    toFloat(riskRaw["buffer_atr"]),
			MaxInvestPct: toFloat(riskRaw["max_invest_pct"]),
			MaxInvestAmt: toFloat(riskRaw["max_invest_amt"]),
			MaxLeverage:  toFloat(riskRaw["max_leverage"]),
			LiqPrice:     toFloat(riskRaw["liquidation_price"]),
			MMR:          toFloat(riskRaw["mmr"]),
			Fee:          toFloat(riskRaw["fee"]),
		}
	}
	if ratios, ok := planMap["take_profit_ratios"].([]any); ok {
		plan.TakeProfitRatios = toFloatSlice(ratios)
	}
	if tps, ok := planMap["take_profits"].([]any); ok {
		plan.TakeProfits = toFloatSlice(tps)
	}
	return plan
}

func parseFSMNext(data map[string]any) fsm.PositionState {
	fsmMap, ok := data["fsm"].(map[string]any)
	if !ok {
		return fsm.PositionState("")
	}
	return fsm.PositionState(strings.ToUpper(toString(fsmMap["next_state"])))
}

func parseFSMActions(data map[string]any) []fsm.Action {
	fsmMap, ok := data["fsm"].(map[string]any)
	if !ok {
		return nil
	}
	actionsRaw, ok := fsmMap["actions"].([]any)
	if !ok {
		return nil
	}
	out := make([]fsm.Action, 0, len(actionsRaw))
	for _, item := range actionsRaw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, fsm.Action{Type: fsm.ActionType(toString(m["type"])), Reason: toString(m["reason"])})
		}
	}
	return out
}

func parseFSMRuleHit(data map[string]any) fsm.RuleHit {
	fsmMap, ok := data["fsm"].(map[string]any)
	if !ok {
		return fsm.RuleHit{}
	}
	hitRaw, ok := fsmMap["rule_hit"].(map[string]any)
	if !ok {
		return fsm.RuleHit{}
	}
	return fsm.RuleHit{
		Name:     toString(hitRaw["name"]),
		Priority: toInt(hitRaw["priority"]),
		Action:   toString(hitRaw["action"]),
		Reason:   toString(hitRaw["reason"]),
		Next:     toString(hitRaw["next"]),
		Default:  toBool(hitRaw["default"]),
	}
}
