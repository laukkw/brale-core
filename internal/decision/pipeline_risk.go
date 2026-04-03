package decision

import (
	"context"
	"math"
	"strings"

	"brale-core/internal/decision/features"
	"brale-core/internal/decision/fund"
	"brale-core/internal/decision/provider"
	"brale-core/internal/execution"
	"brale-core/internal/strategy"
)

func (p *Pipeline) applyRiskPlanUpdate(ctx context.Context, res SymbolResult, comp features.CompressionResult, posID string) (tightenExecution, error) {
	exec := newTightenExecution(res, comp)
	if p.riskPlans() == nil {
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
	updateResult, err := p.applyTightenUpdate(ctx, pos, plan, updateCtx)
	if err != nil {
		return exec, err
	}
	exec.Executed = updateResult.Executed
	exec.TPTightened = updateResult.TPTightened
	exec.ExitConfirmHit = updateResult.ExitConfirmHit
	exec.PlanSource = updateResult.PlanSource
	exec.StopLoss = updateResult.StopLoss
	exec.TakeProfits = append([]float64(nil), updateResult.TakeProfits...)
	exec.LLMRiskTrace = cloneLLMRiskTrace(updateResult.LLMRiskTrace)
	if !updateResult.Executed {
		exec.addBlocked(tightenBlockNoTightenNeeded)
	}
	return exec, nil
}

type tightenContext struct {
	Binding        strategy.StrategyBinding
	Gate           fund.GateDecision
	InPosIndicator provider.InPositionIndicatorOut
	InPosStructure provider.InPositionStructureOut
	InPosMechanics provider.InPositionMechanicsOut
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
	PlanSource     string
	StopLoss       float64
	TakeProfits    []float64
	LLMRiskTrace   *execution.LLMRiskTrace
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
	if src := strings.TrimSpace(e.PlanSource); src != "" {
		out["plan_source"] = strings.ToLower(src)
	}
	if e.StopLoss > 0 {
		out["stop_loss"] = e.StopLoss
	}
	if len(e.TakeProfits) > 0 {
		out["take_profits"] = append([]float64(nil), e.TakeProfits...)
	}
	if trace := llmRiskTraceMap(e.LLMRiskTrace); trace != nil {
		out["llm_trace"] = trace
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
