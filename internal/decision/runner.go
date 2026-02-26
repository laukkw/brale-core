// 本文件主要内容：实现决策 Runner 的核心执行流程。

package decision

import (
	"context"
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
	"brale-core/internal/pkg/logging"
	"brale-core/internal/snapshot"
	"brale-core/internal/strategy"

	"go.uber.org/zap"
)

type Runner struct {
	Snapshotter           Snapshotter
	Compressor            Compressor
	Agent                 AgentService
	Provider              ProviderService
	Bindings              map[string]strategy.StrategyBinding
	Configs               map[string]config.SymbolConfig
	Enabled               map[string]AgentEnabled
	Ruleflow              ruleflow.Evaluator
	NewsOverlayEnabled    bool
	NewsOverlayStaleAfter time.Duration
	mu                    sync.Mutex
}

type RunOptions struct {
	BuildPlan    bool
	ModeBySymbol map[string]decisionmode.Mode
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
		r.currentNewsOverlayPayload(),
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
	res.FSMNext = rfResult.FSMNext
	res.FSMActions = rfResult.FSMActions
	res.FSMRuleHit = rfResult.FSMRuleHit
	if !res.Gate.GlobalTradeable {
		logger.Info("gate blocked trade", zap.String("gate_reason", res.Gate.GateReason))
	}
	return res
}

func symbolError(symbol string, err error, code string) SymbolResult {
	return SymbolResult{Symbol: symbol, Err: err, Gate: gateError(code)}
}

func shouldSkipProvider(opts RunOptions, symbol string) bool {
	return isInPositionMode(opts, symbol)
}

func isInPositionMode(opts RunOptions, symbol string) bool {
	if opts.ModeBySymbol == nil {
		return false
	}
	return opts.ModeBySymbol[symbol] == decisionmode.ModeInPosition
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
