package decision

import (
	"context"
	"strings"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/decision/decisionmode"
	"brale-core/internal/decision/features"
	"brale-core/internal/decision/fsm"
	"brale-core/internal/decision/ruleflow"
	"brale-core/internal/execution"
	"brale-core/internal/llm"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/snapshot"
	"brale-core/internal/strategy"

	"go.uber.org/zap"
)

type runnerSymbolInputs struct {
	Binding           strategy.StrategyBinding
	Enabled           AgentEnabled
	Config            config.SymbolConfig
	ScoreThreshold    float64
	ConfThreshold     float64
	State             fsm.PositionState
	ExitConfirmCount  int
	LLMRiskMode       bool
	SkipProviderStage bool
	Logger            *zap.Logger
}

func (r *Runner) RunOnce(ctx context.Context, symbols, intervals []string, limit int, acct execution.AccountState, risk execution.RiskParams) ([]SymbolResult, snapshot.MarketSnapshot, features.CompressionResult, error) {
	return r.RunOnceWithOptions(ctx, symbols, intervals, limit, acct, risk, RunOptions{BuildPlan: true})
}

func (r *Runner) RunOnceWithOptions(ctx context.Context, symbols, intervals []string, limit int, acct execution.AccountState, risk execution.RiskParams, opts RunOptions) ([]SymbolResult, snapshot.MarketSnapshot, features.CompressionResult, error) {
	if err := r.validate(); err != nil {
		return nil, snapshot.MarketSnapshot{}, features.CompressionResult{}, err
	}
	snap, err := r.Snapshotter.Fetch(ctx, symbols, intervals, limit)
	if err != nil {
		return nil, snapshot.MarketSnapshot{}, features.CompressionResult{}, err
	}
	comp, _, err := r.Compressor.Compress(ctx, snap)
	if err != nil {
		return nil, snapshot.MarketSnapshot{}, features.CompressionResult{}, err
	}
	return r.runSymbols(ctx, symbols, comp, acct, risk, opts), snap, comp, nil
}

func (r *Runner) runSymbols(ctx context.Context, symbols []string, comp features.CompressionResult, acct execution.AccountState, risk execution.RiskParams, opts RunOptions) []SymbolResult {
	results := make([]SymbolResult, 0, len(symbols))
	for _, sym := range symbols {
		results = append(results, r.runSymbol(ctx, sym, comp, acct, risk, opts))
	}
	return results
}

func (r *Runner) runSymbol(ctx context.Context, symbol string, comp features.CompressionResult, acct execution.AccountState, risk execution.RiskParams, opts RunOptions) SymbolResult {
	ctx = llm.WithSessionSymbol(ctx, symbol)
	ctx = llm.WithSessionFlow(ctx, flowForSymbol(opts, symbol))
	inputs, errResult := r.loadRunnerSymbolInputs(ctx, symbol, opts)
	if errResult != nil {
		return *errResult
	}
	res, shouldFinalize := r.runAgentAndProviderStages(ctx, symbol, comp, inputs)
	if !shouldFinalize {
		return res
	}
	return r.evaluateRuleflowAndFinalize(ctx, symbol, comp, acct, risk, opts, inputs, res)
}

func (r *Runner) loadRunnerSymbolInputs(ctx context.Context, symbol string, opts RunOptions) (*runnerSymbolInputs, *SymbolResult) {
	logger := logging.FromContext(ctx).Named("decision").With(zap.String("symbol", symbol))
	bind, err := r.getBinding(symbol)
	if err != nil {
		logger.Error("binding not found", zap.Error(err))
		result := symbolError(symbol, err, "BINDING_MISSING")
		return nil, &result
	}
	enabled, err := r.getEnabled(symbol)
	if err != nil {
		logger.Error("enabled config missing", zap.Error(err))
		result := symbolError(symbol, err, "ENABLED_MISSING")
		return nil, &result
	}
	symbolCfg, cfgErr := r.getConfig(symbol)
	if cfgErr != nil {
		logger.Error("config missing", zap.Error(cfgErr))
		result := symbolError(symbol, cfgErr, "CONFIG_MISSING")
		return nil, &result
	}
	scoreThreshold, confThreshold := resolveConsensusThresholds(symbolCfg)
	state := fsm.StateFlat
	if isInPositionMode(opts, symbol) {
		state = fsm.StateInPosition
	}
	return &runnerSymbolInputs{
		Binding:           bind,
		Enabled:           enabled,
		Config:            symbolCfg,
		ScoreThreshold:    scoreThreshold,
		ConfThreshold:     confThreshold,
		State:             state,
		ExitConfirmCount:  0,
		LLMRiskMode:       isLLMRiskMode(opts, symbol),
		SkipProviderStage: shouldSkipProvider(opts, symbol),
		Logger:            logger,
	}, nil
}

func (r *Runner) runAgentAndProviderStages(ctx context.Context, symbol string, comp features.CompressionResult, inputs *runnerSymbolInputs) (SymbolResult, bool) {
	res, err := r.runAgentStage(ctx, symbol, comp, inputs.Enabled, inputs.Logger)
	if err != nil {
		return res, false
	}
	applyDirectionConsensus(&res, inputs.Enabled, inputs.ScoreThreshold, inputs.ConfThreshold)
	if inputs.SkipProviderStage {
		return res, false
	}
	providerRes, err := r.runProviderStage(ctx, symbol, inputs.Enabled, res, inputs.Logger)
	if err != nil {
		return providerRes, false
	}
	return providerRes, true
}

func (r *Runner) evaluateRuleflowAndFinalize(ctx context.Context, symbol string, comp features.CompressionResult, acct execution.AccountState, risk execution.RiskParams, opts RunOptions, inputs *runnerSymbolInputs, res SymbolResult) SymbolResult {
	rfEngine := r.ensureRuleflowEngine()
	rfInput := buildRuleflowInput(symbol, res, inputs.Binding, inputs.State, "", inputs.ExitConfirmCount, opts.BuildPlan, comp, acct, risk, ruleflow.InPositionOutputs{}, ruleflow.HardGuardPosition{})
	rfStart := time.Now()
	rfResult, err := rfEngine.Evaluate(ctx, inputs.Binding.RuleChainPath, rfInput)
	if err != nil {
		inputs.Logger.Error("ruleflow evaluate failed", zap.Error(err))
		res.Err = err
		res.Gate = gateError("RULEFLOW_ERROR")
		return res
	}
	inputs.Logger.Debug("ruleflow complete", zap.Duration("latency", time.Since(rfStart)))
	applyRuleflowResult(&res, rfResult, comp, symbol, inputs.Enabled, inputs.ScoreThreshold, inputs.ConfThreshold, inputs.LLMRiskMode)
	if inputs.State == fsm.StateFlat && shouldRunFlatLLMRiskInit(res, inputs.LLMRiskMode) {
		if err := r.applyFlatLLMRiskInit(ctx, symbol, &res, inputs.Binding, acct); err != nil {
			inputs.Logger.Error("flat llm risk-init failed", zap.Error(err))
			res.Err = err
			if reasonCode, ok := llmRiskFailureReasonCode(err); ok {
				res.Gate = gateError(reasonCode)
			} else {
				res.Gate = gateError("LLM_RISK_INIT_ERROR")
			}
			return res
		}
	}
	appendPlanDerived(&res.Gate, res.Plan)
	if !res.Gate.GlobalTradeable {
		inputs.Logger.Info("gate blocked trade", zap.String("gate_reason", res.Gate.GateReason))
	}
	return res
}

func flowForSymbol(opts RunOptions, symbol string) llm.LLMFlow {
	if isInPositionMode(opts, symbol) {
		return llm.LLMFlowInPosition
	}
	return llm.LLMFlowFlat
}

func symbolError(symbol string, err error, code string) SymbolResult {
	return SymbolResult{Symbol: symbol, Err: err, Gate: gateError(code)}
}

func shouldSkipProvider(opts RunOptions, symbol string) bool {
	return isInPositionMode(opts, symbol)
}

func shouldRunFlatLLMRiskInit(res SymbolResult, llmRiskMode bool) bool {
	if !llmRiskMode {
		return false
	}
	if !res.Gate.GlobalTradeable || res.Plan == nil || !res.Plan.Valid {
		return false
	}
	return hasFSMAction(res.FSMActions, fsm.ActionOpen)
}

func isInPositionMode(opts RunOptions, symbol string) bool {
	if opts.ModeBySymbol == nil {
		return false
	}
	return opts.ModeBySymbol[symbol] == decisionmode.ModeInPosition
}

func isLLMRiskMode(opts RunOptions, symbol string) bool {
	if opts.RiskStrategyModeBySymbol == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(opts.RiskStrategyModeBySymbol[symbol]), execution.PlanSourceLLM)
}

func applyDirectionConsensus(res *SymbolResult, enabled AgentEnabled, scoreThreshold, confThreshold float64) {
	if res == nil {
		return
	}
	consensus := computeDirectionConsensus(enabled, res.AgentIndicator, res.AgentStructure, res.AgentMechanics, scoreThreshold, confThreshold)
	res.ConsensusDirection = consensus.Direction
	res.ConsensusScore = consensus.Score
	res.ConsensusConfidence = consensus.Confidence
	res.ConsensusAgreement = consensus.Agreement
}

func applyRuleflowResult(res *SymbolResult, rfResult ruleflow.Result, comp features.CompressionResult, symbol string, enabled AgentEnabled, scoreThreshold, confThreshold float64, llmRiskMode bool) {
	if res == nil {
		return
	}
	res.RuleflowResult = &rfResult
	res.Gate = rfResult.Gate
	if res.Gate.Derived == nil {
		res.Gate.Derived = map[string]any{}
	}
	if price, ok := pickCurrentPrice(comp, symbol); ok {
		res.Gate.Derived["current_price"] = price
	}
	res.Gate.Derived["direction_consensus"] = buildDirectionConsensusDerived(enabled, *res, scoreThreshold, confThreshold)
	res.Plan = rfResult.Plan
	if res.Plan != nil {
		planSource := strings.ToLower(strings.TrimSpace(res.Plan.PlanSource))
		if planSource != execution.PlanSourceLLM {
			planSource = execution.PlanSourceGo
		}
		if planSource == execution.PlanSourceGo && llmRiskMode {
			planSource = execution.PlanSourceLLM
		}
		res.Plan.PlanSource = planSource
	}
	res.FSMNext = rfResult.FSMNext
	res.FSMActions = rfResult.FSMActions
	res.FSMRuleHit = rfResult.FSMRuleHit
}
