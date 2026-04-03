package decision

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"brale-core/internal/config"
	"brale-core/internal/decision/decisionmode"
	"brale-core/internal/decision/decisionutil"
	"brale-core/internal/decision/features"
	"brale-core/internal/decision/fund"
	"brale-core/internal/decision/provider"
	"brale-core/internal/decision/ruleflow"
	"brale-core/internal/execution"
	"brale-core/internal/risk/initexit"
	"brale-core/internal/strategy"
)

type Runner struct {
	Snapshotter     Snapshotter
	Compressor      Compressor
	Agent           AgentService
	Provider        ProviderService
	FlatRiskInitLLM FlatRiskInitLLM
	TightenRiskLLM  TightenRiskUpdateLLM
	Bindings        map[string]strategy.StrategyBinding
	Configs         map[string]config.SymbolConfig
	Enabled         map[string]AgentEnabled
	Ruleflow        ruleflow.Evaluator
	mu              sync.Mutex
}

type RunOptions struct {
	BuildPlan                bool
	ModeBySymbol             map[string]decisionmode.Mode
	RiskStrategyModeBySymbol map[string]string
}

type FlatRiskInitInput struct {
	Symbol string
	Gate   fund.GateDecision
	Plan   execution.ExecutionPlan
}

type FlatRiskInitLLM func(ctx context.Context, input FlatRiskInitInput) (*initexit.BuildPatch, error)

type TightenRiskUpdateInput struct {
	Symbol              string
	Gate                fund.GateDecision
	Side                string
	Entry               float64
	MarkPrice           float64
	ATR                 float64
	CurrentStopLoss     float64
	CurrentTakeProfits  []float64
	InPositionIndicator provider.InPositionIndicatorOut
	InPositionStructure provider.InPositionStructureOut
	InPositionMechanics provider.InPositionMechanicsOut
}

type TightenRiskUpdatePatch struct {
	StopLoss    *float64
	TakeProfits []float64
	Trace       *execution.LLMRiskTrace
}

type TightenRiskUpdateLLM func(ctx context.Context, input TightenRiskUpdateInput) (*TightenRiskUpdatePatch, error)

func (r *Runner) validate() error {
	if r.Snapshotter == nil || r.Compressor == nil || r.Agent == nil || r.Provider == nil {
		return fmt.Errorf("runner dependencies missing")
	}
	if r.Ruleflow == nil {
		r.Ruleflow = ruleflow.NewEngine()
	}
	if r.Enabled == nil {
		return fmt.Errorf("enabled config is required")
	}
	if r.Configs == nil {
		return fmt.Errorf("symbol config is required")
	}
	return nil
}

func appendPlanDerived(gate *fund.GateDecision, plan *execution.ExecutionPlan) {
	if gate == nil || plan == nil {
		return
	}
	if gate.Derived == nil {
		gate.Derived = map[string]any{}
	}
	planSource := strings.TrimSpace(plan.PlanSource)
	if planSource == "" {
		planSource = execution.PlanSourceGo
	}
	gate.Derived["plan"] = map[string]any{
		"direction":          plan.Direction,
		"entry":              plan.Entry,
		"stop_loss":          plan.StopLoss,
		"risk_pct":           plan.RiskPct,
		"position_size":      plan.PositionSize,
		"leverage":           plan.Leverage,
		"take_profits":       append([]float64(nil), plan.TakeProfits...),
		"take_profit_ratios": append([]float64(nil), plan.TakeProfitRatios...),
		"plan_source":        planSource,
	}
	if trace := llmRiskTraceMap(plan.LLMRiskTrace); trace != nil {
		gate.Derived["plan"].(map[string]any)["llm_trace"] = trace
	}
}

const (
	llmRiskStageFlatInit          = "flat_init"
	llmRiskReasonModeMissing      = "LLM_RISK_INIT_MODE_MISSING"
	llmRiskReasonModeMismatch     = "LLM_RISK_INIT_MODE_MISMATCH"
	llmRiskReasonTransportFailure = "LLM_RISK_INIT_TRANSPORT_FAILURE"
	llmRiskReasonSchemaFailure    = "LLM_RISK_INIT_SCHEMA_FAILURE"
	llmRiskReasonRatioFailure     = "LLM_RISK_INIT_RATIO_FAILURE"
	llmRiskReasonDirectionFailure = "LLM_RISK_INIT_DIRECTION_FAILURE"
)

var (
	errFlatRiskPatchMissing     = errors.New("flat risk patch missing")
	errFlatRiskEntryMissing     = errors.New("flat risk entry missing")
	errFlatRiskEntryInvalid     = errors.New("flat risk entry invalid")
	errFlatRiskStopMissing      = errors.New("flat risk stop_loss missing")
	errFlatRiskTPMissing        = errors.New("flat risk take_profits missing")
	errFlatRiskRatioMissing     = errors.New("flat risk take_profit_ratios missing")
	errFlatRiskRatioInvalid     = errors.New("flat risk ratios invalid")
	errFlatRiskDirectionInvalid = errors.New("flat risk direction invalid")
)

type llmRiskFailure struct {
	Symbol string
	Stage  string
	Reason string
	Err    error
}

func (e *llmRiskFailure) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return fmt.Sprintf("llm risk failed: symbol=%s stage=%s reason=%s", strings.TrimSpace(e.Symbol), strings.TrimSpace(e.Stage), strings.TrimSpace(e.Reason))
	}
	return fmt.Sprintf("llm risk failed: symbol=%s stage=%s reason=%s: %v", strings.TrimSpace(e.Symbol), strings.TrimSpace(e.Stage), strings.TrimSpace(e.Reason), e.Err)
}

func (e *llmRiskFailure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func wrapLLMRiskFailure(symbol, stage, reason string, err error) error {
	r := strings.TrimSpace(reason)
	if r == "" {
		r = llmRiskReasonSchemaFailure
	}
	return &llmRiskFailure{Symbol: strings.TrimSpace(symbol), Stage: strings.TrimSpace(stage), Reason: r, Err: err}
}

func llmRiskFailureReasonCode(err error) (string, bool) {
	var target *llmRiskFailure
	if !errors.As(err, &target) || target == nil {
		return "", false
	}
	r := strings.TrimSpace(target.Reason)
	if r == "" {
		return "", false
	}
	return r, true
}

func classifyFlatRiskInitPatchError(err error) string {
	switch {
	case errors.Is(err, errFlatRiskRatioInvalid):
		return llmRiskReasonRatioFailure
	case errors.Is(err, errFlatRiskDirectionInvalid):
		return llmRiskReasonDirectionFailure
	default:
		return llmRiskReasonSchemaFailure
	}
}

func pickCurrentPrice(comp features.CompressionResult, symbol string) (float64, bool) {
	indicator, err := decisionutil.PickIndicator(comp, symbol)
	if err != nil {
		return 0, false
	}
	if indicator.Close <= 0 {
		return 0, false
	}
	return indicator.Close, true
}
