package decision

import (
	"context"
	"fmt"
	"math"
	"strings"

	"brale-core/internal/decision/fsm"
	"brale-core/internal/decision/fund"
	"brale-core/internal/execution"
	riskcalc "brale-core/internal/risk"
	"brale-core/internal/risk/initexit"
	"brale-core/internal/strategy"
)

func (r *Runner) completeFlatExecutionPlan(ctx context.Context, symbol string, gate fund.GateDecision, plan *execution.ExecutionPlan, actions []fsm.Action, bind strategy.StrategyBinding, acct execution.AccountState, llmRiskMode bool) (*execution.ExecutionPlan, error) {
	if plan == nil {
		return nil, nil
	}
	if !llmRiskMode || !shouldCompleteExecutableFlatPlan(gate, plan, actions) {
		return plan, nil
	}
	return r.buildFlatLLMPlan(ctx, symbol, gate, plan, bind, acct)
}

func shouldCompleteExecutableFlatPlan(gate fund.GateDecision, plan *execution.ExecutionPlan, actions []fsm.Action) bool {
	if !gate.GlobalTradeable || plan == nil || !plan.Valid {
		return false
	}
	return hasFSMAction(actions, fsm.ActionOpen)
}

func (r *Runner) buildFlatLLMPlan(ctx context.Context, symbol string, gate fund.GateDecision, plan *execution.ExecutionPlan, bind strategy.StrategyBinding, acct execution.AccountState) (*execution.ExecutionPlan, error) {
	if plan == nil {
		return nil, nil
	}
	if r.FlatRiskInitLLM == nil {
		return nil, wrapLLMRiskFailure(symbol, llmRiskStageFlatInit, llmRiskReasonTransportFailure, fmt.Errorf("flat risk init llm callback is required"))
	}
	resolved := *plan
	patch, err := r.FlatRiskInitLLM(ctx, FlatRiskInitInput{Symbol: symbol, Gate: gate, Plan: resolved})
	if err != nil {
		return nil, wrapLLMRiskFailure(symbol, llmRiskStageFlatInit, llmRiskReasonTransportFailure, err)
	}
	if err := applyFlatRiskInitPatch(&resolved, patch); err != nil {
		return nil, wrapLLMRiskFailure(symbol, llmRiskStageFlatInit, classifyFlatRiskInitPatchError(err), err)
	}
	stopReason := "llm-generated"
	if patch != nil && patch.Reason != nil && strings.TrimSpace(*patch.Reason) != "" {
		stopReason = strings.TrimSpace(*patch.Reason)
	}
	if err := rescaleFlatPlan(&resolved, bind, acct, stopReason); err != nil {
		return nil, wrapLLMRiskFailure(symbol, llmRiskStageFlatInit, llmRiskReasonSchemaFailure, err)
	}
	resolved.LLMRiskTrace = cloneLLMRiskTrace(patch.Trace)
	return &resolved, nil
}

func applyFlatRiskInitPatch(plan *execution.ExecutionPlan, patch *initexit.BuildPatch) error {
	if patch == nil {
		return errFlatRiskPatchMissing
	}
	if patch.Entry == nil {
		return errFlatRiskEntryMissing
	}
	entry := *patch.Entry
	if entry <= 0 {
		return errFlatRiskEntryInvalid
	}
	if patch.StopLoss == nil {
		return errFlatRiskStopMissing
	}
	if len(patch.TakeProfits) == 0 {
		return errFlatRiskTPMissing
	}
	if len(patch.TakeProfitRatios) == 0 {
		return errFlatRiskRatioMissing
	}
	if len(patch.TakeProfits) != len(patch.TakeProfitRatios) {
		return errFlatRiskRatioInvalid
	}

	direction := strings.ToLower(strings.TrimSpace(plan.Direction))
	stop := *patch.StopLoss
	if direction == "long" {
		if !(stop < entry) {
			return errFlatRiskDirectionInvalid
		}
		last := entry
		for _, tp := range patch.TakeProfits {
			if tp <= entry || tp <= last {
				return errFlatRiskDirectionInvalid
			}
			last = tp
		}
	} else if direction == "short" {
		if !(stop > entry) {
			return errFlatRiskDirectionInvalid
		}
		last := entry
		for _, tp := range patch.TakeProfits {
			if tp >= entry || tp >= last {
				return errFlatRiskDirectionInvalid
			}
			last = tp
		}
	} else {
		return errFlatRiskDirectionInvalid
	}

	ratioSum := 0.0
	for _, ratio := range patch.TakeProfitRatios {
		if ratio <= 0 {
			return errFlatRiskRatioInvalid
		}
		ratioSum += ratio
	}
	if math.Abs(ratioSum-1.0) > 1e-6 {
		return errFlatRiskRatioInvalid
	}

	plan.StopLoss = stop
	plan.Entry = entry
	plan.TakeProfits = append([]float64(nil), patch.TakeProfits...)
	plan.TakeProfitRatios = append([]float64(nil), patch.TakeProfitRatios...)
	plan.PlanSource = execution.PlanSourceLLM
	return nil
}

func rescaleFlatPlan(plan *execution.ExecutionPlan, bind strategy.StrategyBinding, acct execution.AccountState, stopReason string) error {
	if plan == nil {
		return fmt.Errorf("plan is nil")
	}
	if plan.Entry <= 0 || plan.StopLoss <= 0 {
		return fmt.Errorf("entry/stop_loss must be > 0")
	}
	if plan.RiskPct <= 0 {
		return fmt.Errorf("risk_pct must be > 0")
	}
	stopDist := math.Abs(plan.Entry - plan.StopLoss)
	if stopDist <= 0 {
		return fmt.Errorf("stop distance must be > 0")
	}

	baseBalance := acct.Available
	if baseBalance <= 0 {
		baseBalance = acct.Equity
	}
	if baseBalance <= 0 {
		return fmt.Errorf("account equity/available must be > 0")
	}
	riskAmount := baseBalance * plan.RiskPct
	if riskAmount <= 0 {
		return fmt.Errorf("risk amount must be > 0")
	}
	positionSize := riskAmount / stopDist
	if positionSize <= 0 {
		return fmt.Errorf("position size must be > 0")
	}

	maxInvestPct := bind.RiskManagement.MaxInvestPct
	if maxInvestPct <= 0 {
		maxInvestPct = 1.0
	}
	maxInvestAmt := baseBalance * maxInvestPct
	if maxInvestAmt <= 0 {
		maxInvestAmt = baseBalance
	}
	leverageResult := riskcalc.ResolveLeverageAndLiquidation(plan.Entry, positionSize, maxInvestAmt, bind.RiskManagement.MaxLeverage, plan.Direction)
	if leverageResult.PositionSize <= 0 {
		return fmt.Errorf("position size invalid after leverage resolution")
	}
	if riskcalc.IsStopBeyondLiquidation(plan.Direction, plan.StopLoss, leverageResult.LiquidationPrice) {
		return fmt.Errorf("stop beyond liquidation")
	}

	plan.PositionSize = leverageResult.PositionSize
	plan.Leverage = leverageResult.Leverage
	plan.RiskAnnotations.RiskDistance = stopDist
	plan.RiskAnnotations.StopSource = "llm-flat"
	if strings.TrimSpace(stopReason) == "" {
		stopReason = "llm-generated"
	}
	plan.RiskAnnotations.StopReason = stopReason
	plan.RiskAnnotations.MaxInvestPct = maxInvestPct
	plan.RiskAnnotations.MaxInvestAmt = maxInvestAmt
	plan.RiskAnnotations.MaxLeverage = bind.RiskManagement.MaxLeverage
	plan.RiskAnnotations.LiqPrice = leverageResult.LiquidationPrice
	plan.RiskAnnotations.MMR = leverageResult.MMR
	plan.RiskAnnotations.Fee = leverageResult.Fee
	plan.PlanSource = execution.PlanSourceLLM
	return nil
}
