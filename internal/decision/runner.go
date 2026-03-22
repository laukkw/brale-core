// 本文件主要内容：实现决策 Runner 的核心执行流程。

package decision

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/decision/agent"
	"brale-core/internal/decision/decisionmode"
	"brale-core/internal/decision/decisionutil"
	"brale-core/internal/decision/direction"
	"brale-core/internal/decision/features"
	"brale-core/internal/decision/fsm"
	"brale-core/internal/decision/fund"
	"brale-core/internal/decision/provider"
	"brale-core/internal/decision/ruleflow"
	"brale-core/internal/execution"
	"brale-core/internal/llm"
	"brale-core/internal/pkg/logging"
	riskcalc "brale-core/internal/risk"
	"brale-core/internal/risk/initexit"
	"brale-core/internal/snapshot"
	"brale-core/internal/strategy"

	"go.uber.org/zap"
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
}

type TightenRiskUpdateLLM func(ctx context.Context, input TightenRiskUpdateInput) (*TightenRiskUpdatePatch, error)

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
	logger := logging.FromContext(ctx).Named("decision").With(zap.String("symbol", symbol))
	// 获取币种的详细策略配置
	bind, err := r.getBinding(symbol)
	if err != nil {
		logger.Error("binding not found", zap.Error(err))
		return symbolError(symbol, err, "BINDING_MISSING")
	}
	// 获取币种启用的agent , 但是目前我都启用了，所以此功能等未来如果有很多个agent 的时候可以再拆分处理
	enabled, err := r.getEnabled(symbol)
	if err != nil {
		logger.Error("enabled config missing", zap.Error(err))
		return symbolError(symbol, err, "ENABLED_MISSING")
	}
	symbolCfg, cfgErr := r.getConfig(symbol)
	if cfgErr != nil {
		logger.Error("config missing", zap.Error(cfgErr))
		return symbolError(symbol, cfgErr, "CONFIG_MISSING")
	}
	scoreThreshold, confThreshold := resolveConsensusThresholds(symbolCfg)
	// 获得agent 阶段的socre 和 confidence
	res, err := r.runAgentStage(ctx, symbol, comp, enabled, logger)
	if err != nil {
		return res
	}
	// 具体的计算的分的逻辑在这里。将多个gent的得分 算出一个共识的方向。
	consensus := computeDirectionConsensus(enabled, res.AgentIndicator, res.AgentStructure, res.AgentMechanics, scoreThreshold, confThreshold)
	res.ConsensusDirection = consensus.Direction
	res.ConsensusScore = consensus.Score
	res.ConsensusConfidence = consensus.Confidence
	res.ConsensusAgreement = consensus.Agreement
	// 如果是持仓状态的话就会跳过一下的部分。不过依旧会保留之前的agent 结果，在持仓和未持仓的情况下，agent都是一致的。
	if shouldSkipProvider(opts, symbol) {
		return res
	}
	// 来生成provider 阶段的结果
	providerRes, err := r.runProviderStage(ctx, symbol, enabled, res, logger)
	if err != nil {
		return providerRes
	}
	res = providerRes
	state := fsm.StateFlat
	if isInPositionMode(opts, symbol) {
		state = fsm.StateInPosition
	}
	exitConfirmCount := 0
	// 构建rulego的数据，然后交给rule engine 开始执行整个rule
	rfEngine := r.ensureRuleflowEngine()
	rfInput := buildRuleflowInput(
		symbol,
		res,
		bind,
		state,
		"",
		exitConfirmCount,
		opts.BuildPlan,
		comp,
		acct,
		risk,
		ruleflow.InPositionOutputs{},
		ruleflow.HardGuardPosition{},
	)

	rfStart := time.Now()
	rfResult, err := rfEngine.Evaluate(ctx, bind.RuleChainPath, rfInput)
	if err != nil {
		logger.Error("ruleflow evaluate failed", zap.Error(err))
		res.Err = err
		res.Gate = gateError("RULEFLOW_ERROR")
		return res
	}

	logger.Debug("ruleflow complete", zap.Duration("latency", time.Since(rfStart)))
	res.RuleflowResult = &rfResult
	res.Gate = rfResult.Gate
	if res.Gate.Derived == nil {
		res.Gate.Derived = map[string]any{}
	}
	if price, ok := pickCurrentPrice(comp, symbol); ok {
		res.Gate.Derived["current_price"] = price
	}
	res.Gate.Derived["direction_consensus"] = buildDirectionConsensusDerived(enabled, res, scoreThreshold, confThreshold)
	res.Plan = rfResult.Plan
	if res.Plan != nil {
		planSource := strings.ToLower(strings.TrimSpace(res.Plan.PlanSource))
		if planSource != execution.PlanSourceLLM {
			planSource = execution.PlanSourceGo
		}
		if planSource == execution.PlanSourceGo && isLLMRiskMode(opts, symbol) {
			planSource = execution.PlanSourceLLM
		}
		res.Plan.PlanSource = planSource
	}
	res.FSMNext = rfResult.FSMNext
	res.FSMActions = rfResult.FSMActions
	res.FSMRuleHit = rfResult.FSMRuleHit
	llmRiskMode := res.Plan != nil && strings.EqualFold(strings.TrimSpace(res.Plan.PlanSource), execution.PlanSourceLLM)
	if !llmRiskMode {
		llmRiskMode = isLLMRiskMode(opts, symbol)
	}
	if state == fsm.StateFlat && shouldRunFlatLLMRiskInit(res, llmRiskMode) {
		if err := r.applyFlatLLMRiskInit(ctx, symbol, &res, bind, acct); err != nil {
			logger.Error("flat llm risk-init failed", zap.Error(err))
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
		logger.Info("gate blocked trade", zap.String("gate_reason", res.Gate.GateReason))
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

func (r *Runner) applyFlatLLMRiskInit(ctx context.Context, symbol string, res *SymbolResult, bind strategy.StrategyBinding, acct execution.AccountState) error {
	if r == nil || res == nil || res.Plan == nil {
		return nil
	}
	if r.FlatRiskInitLLM == nil {
		return wrapLLMRiskFailure(symbol, llmRiskStageFlatInit, llmRiskReasonTransportFailure, fmt.Errorf("flat risk init llm callback is required"))
	}
	patch, err := r.FlatRiskInitLLM(ctx, FlatRiskInitInput{Symbol: symbol, Gate: res.Gate, Plan: *res.Plan})
	if err != nil {
		return wrapLLMRiskFailure(symbol, llmRiskStageFlatInit, llmRiskReasonTransportFailure, err)
	}
	if err := applyFlatRiskInitPatch(res.Plan, patch); err != nil {
		return wrapLLMRiskFailure(symbol, llmRiskStageFlatInit, classifyFlatRiskInitPatchError(err), err)
	}
	if err := rescaleFlatPlan(res.Plan, bind, acct); err != nil {
		return wrapLLMRiskFailure(symbol, llmRiskStageFlatInit, llmRiskReasonSchemaFailure, err)
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
}

const (
	llmRiskStageFlatInit          = "flat_init"
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
	return &llmRiskFailure{
		Symbol: strings.TrimSpace(symbol),
		Stage:  strings.TrimSpace(stage),
		Reason: r,
		Err:    err,
	}
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

func rescaleFlatPlan(plan *execution.ExecutionPlan, bind strategy.StrategyBinding, acct execution.AccountState) error {
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
	plan.RiskAnnotations.StopReason = "llm-generated"
	plan.RiskAnnotations.MaxInvestPct = maxInvestPct
	plan.RiskAnnotations.MaxInvestAmt = maxInvestAmt
	plan.RiskAnnotations.MaxLeverage = bind.RiskManagement.MaxLeverage
	plan.RiskAnnotations.LiqPrice = leverageResult.LiquidationPrice
	plan.RiskAnnotations.MMR = leverageResult.MMR
	plan.RiskAnnotations.Fee = leverageResult.Fee
	plan.PlanSource = execution.PlanSourceLLM
	return nil
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

func (r *Runner) runAgentStage(ctx context.Context, symbol string, comp features.CompressionResult, enabled AgentEnabled, logger *zap.Logger) (SymbolResult, error) {
	start := time.Now()
	ind, st, mech, agentPrompts, err := r.analyze(ctx, symbol, comp, enabled)
	if err != nil {
		logger.Error("agent analyze failed", zap.Error(err))
		return symbolError(symbol, err, fmt.Sprintf("AGENT_ERROR:%v", err)), err
	}
	logger.Debug("agent analyze complete", zap.Duration("latency", time.Since(start)))
	return SymbolResult{
		Symbol:         symbol,
		EnabledAgents:  enabled,
		AgentIndicator: ind,
		AgentStructure: st,
		AgentMechanics: mech,
		AgentPrompts:   agentPrompts,
	}, nil
}

func (r *Runner) runProviderStage(ctx context.Context, symbol string, enabled AgentEnabled, res SymbolResult, logger *zap.Logger) (SymbolResult, error) {
	start := time.Now()
	providerEnabled := enabled
	// 首先错误的直接置为false
	if res.AgentPrompts.Indicator.Error != "" {
		providerEnabled.Indicator = false
	}
	if res.AgentPrompts.Structure.Error != "" {
		providerEnabled.Structure = false
	}
	if res.AgentPrompts.Mechanics.Error != "" {
		providerEnabled.Mechanics = false
	}
	// 打包agent 的结果 交给provider 来调用。
	pInd, pSt, pMech, providerPrompts, err := r.judge(ctx, symbol, res.AgentIndicator, res.AgentStructure, res.AgentMechanics, providerEnabled)
	if err != nil {
		logger.Error("provider judge failed", zap.Error(err))
		res.Err = err
		res.Gate = gateError(fmt.Sprintf("PROVIDER_ERROR:%v", err))
		return res, err
	}
	logger.Debug("provider judge complete", zap.Duration("latency", time.Since(start)))
	res.Providers = fund.ProviderBundle{
		Indicator: pInd,
		Structure: pSt,
		Mechanics: pMech,
		Enabled: fund.ProviderEnabled{
			Indicator: providerEnabled.Indicator && providerPrompts.Indicator.Error == "",
			Structure: providerEnabled.Structure && providerPrompts.Structure.Error == "",
			Mechanics: providerEnabled.Mechanics && providerPrompts.Mechanics.Error == "",
		},
	}
	res.ProviderPrompts = providerPrompts
	return res, nil
}

func (r *Runner) getBinding(symbol string) (strategy.StrategyBinding, error) {
	bind, ok := r.Bindings[symbol]
	if !ok {
		return strategy.StrategyBinding{}, fmt.Errorf("binding not found")
	}
	return bind, nil
}

func (r *Runner) getConfig(symbol string) (config.SymbolConfig, error) {
	cfg, ok := r.Configs[symbol]
	if !ok {
		return config.SymbolConfig{}, fmt.Errorf("config not found")
	}
	return cfg, nil
}

func (r *Runner) getEnabled(symbol string) (AgentEnabled, error) {
	enabled, ok := r.Enabled[symbol]
	if !ok {
		return AgentEnabled{}, fmt.Errorf("enabled config not found")
	}
	return enabled, nil
}

func (r *Runner) analyze(ctx context.Context, symbol string, comp features.CompressionResult, enabled AgentEnabled) (agent.IndicatorSummary, agent.StructureSummary, agent.MechanicsSummary, AgentPromptSet, error) {
	ind, st, mech, prompts, err := r.Agent.Analyze(ctx, symbol, comp, enabled)
	if err != nil {
		return agent.IndicatorSummary{}, agent.StructureSummary{}, agent.MechanicsSummary{}, AgentPromptSet{}, err
	}
	return ind, st, mech, prompts, nil
}

func (r *Runner) judge(ctx context.Context, symbol string, ind agent.IndicatorSummary, st agent.StructureSummary, mech agent.MechanicsSummary, enabled AgentEnabled) (provider.IndicatorProviderOut, provider.StructureProviderOut, provider.MechanicsProviderOut, ProviderPromptSet, error) {
	return r.Provider.Judge(ctx, symbol, ind, st, mech, enabled)
}

func computeDirectionConsensus(enabled AgentEnabled, ind agent.IndicatorSummary, st agent.StructureSummary, mech agent.MechanicsSummary, scoreThreshold, confThreshold float64) direction.Consensus {
	indConf := ind.MovementConfidence
	stConf := st.MovementConfidence
	mechConf := mech.MovementConfidence
	if !enabled.Indicator {
		indConf = 0
	}
	if !enabled.Structure {
		stConf = 0
	}
	if !enabled.Mechanics {
		mechConf = 0
	}
	return direction.ComputeConsensusWithThresholds(
		direction.Evidence{Source: direction.SourceIndicator, Score: ind.MovementScore, Confidence: indConf},
		direction.Evidence{Source: direction.SourceStructure, Score: st.MovementScore, Confidence: stConf},
		direction.Evidence{Source: direction.SourceMechanics, Score: mech.MovementScore, Confidence: mechConf},
		scoreThreshold,
		confThreshold,
	)
}

func (r *Runner) ensureRuleflowEngine() ruleflow.Evaluator {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Ruleflow == nil {
		r.Ruleflow = ruleflow.NewEngine()
	}
	return r.Ruleflow
}

func gateError(reason string) fund.GateDecision {
	r := strings.TrimSpace(reason)
	if r == "" {
		r = "GATE_ERROR"
	}
	return fund.GateDecision{
		GlobalTradeable: false,
		DecisionAction:  "VETO",
		GateReason:      r,
		Direction:       "none",
		Grade:           0,
	}
}

func buildDirectionConsensusDerived(enabled AgentEnabled, res SymbolResult, scoreThreshold, confThreshold float64) map[string]any {
	return map[string]any{
		"score":                res.ConsensusScore,
		"confidence":           res.ConsensusConfidence,
		"agreement":            res.ConsensusAgreement,
		"direction":            res.ConsensusDirection,
		"score_threshold":      scoreThreshold,
		"confidence_threshold": confThreshold,
		"score_passed":         math.Abs(res.ConsensusScore) >= scoreThreshold,
		"confidence_passed":    res.ConsensusConfidence >= confThreshold,
		"passed":               direction.IsConsensusPassedWithThresholds(res.ConsensusScore, res.ConsensusConfidence, scoreThreshold, confThreshold),
		"sources": map[string]any{
			"indicator": buildDirectionConsensusSource(enabled.Indicator, res.AgentIndicator.MovementScore, res.AgentIndicator.MovementConfidence),
			"structure": buildDirectionConsensusSource(enabled.Structure, res.AgentStructure.MovementScore, res.AgentStructure.MovementConfidence),
			"mechanics": buildDirectionConsensusSource(enabled.Mechanics, res.AgentMechanics.MovementScore, res.AgentMechanics.MovementConfidence),
		},
	}
}

func resolveConsensusThresholds(cfg config.SymbolConfig) (float64, float64) {
	scoreThreshold := cfg.Consensus.ScoreThreshold
	confThreshold := cfg.Consensus.ConfidenceThreshold
	if scoreThreshold <= 0 {
		scoreThreshold = direction.ThresholdScore()
	}
	if confThreshold <= 0 {
		confThreshold = direction.ThresholdConfidence()
	}
	return scoreThreshold, confThreshold
}

func buildDirectionConsensusSource(enabled bool, score, confidence float64) map[string]any {
	usedConfidence := confidence
	if !enabled {
		usedConfidence = 0
	}
	return map[string]any{
		"enabled":        enabled,
		"score":          score,
		"confidence":     usedConfidence,
		"raw_confidence": confidence,
	}
}
