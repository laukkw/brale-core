package decision

import (
	"context"
	"fmt"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/decision/agent"
	"brale-core/internal/decision/decisionutil"
	"brale-core/internal/decision/features"
	"brale-core/internal/decision/fund"
	"brale-core/internal/decision/provider"
	"brale-core/internal/decision/ruleflow"
	"brale-core/internal/strategy"

	"go.uber.org/zap"
)

func (r *Runner) runAgentStage(ctx context.Context, symbol string, comp features.CompressionResult, enabled AgentEnabled, logger *zap.Logger) (SymbolResult, error) {
	start := time.Now()
	logDecisionStageStarted(logger, "agent stage started", enabled)
	ind, st, mech, agentPrompts, agentInputs, err := r.analyze(ctx, symbol, comp, enabled)
	if err != nil {
		logger.Error("agent analyze failed", zap.Error(err))
		return symbolError(symbol, err, fmt.Sprintf("AGENT_ERROR:%v", err)), err
	}
	logger.Debug("agent analyze complete", zap.Duration("latency", time.Since(start)))
	return SymbolResult{Symbol: symbol, EnabledAgents: enabled, AgentIndicator: ind, AgentStructure: st, AgentMechanics: mech, AgentPrompts: agentPrompts, AgentInputs: agentInputs}, nil
}

func (r *Runner) runProviderStage(ctx context.Context, symbol string, enabled AgentEnabled, res SymbolResult, dataCtx ProviderDataContext, logger *zap.Logger) (SymbolResult, error) {
	start := time.Now()
	logDecisionStageStarted(logger, "provider stage started", enabled)
	providerEnabled := providerEnabledFromAgentPrompts(enabled, res.AgentPrompts)
	pInd, pSt, pMech, providerPrompts, err := r.judge(ctx, symbol, res.AgentIndicator, res.AgentStructure, res.AgentMechanics, providerEnabled, dataCtx)
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

func logDecisionStageStarted(logger *zap.Logger, message string, enabled AgentEnabled) {
	logger.Info(message,
		zap.Bool("indicator", enabled.Indicator),
		zap.Bool("structure", enabled.Structure),
		zap.Bool("mechanics", enabled.Mechanics),
	)
}

func providerEnabledFromAgentPrompts(enabled AgentEnabled, prompts AgentPromptSet) AgentEnabled {
	providerEnabled := enabled
	if prompts.Indicator.Error != "" {
		providerEnabled.Indicator = false
	}
	if prompts.Structure.Error != "" {
		providerEnabled.Structure = false
	}
	if prompts.Mechanics.Error != "" {
		providerEnabled.Mechanics = false
	}
	return providerEnabled
}

func (r *Runner) getBinding(symbol string) (strategy.StrategyBinding, error) {
	symbol = decisionutil.NormalizeSymbol(symbol)
	bind, ok := r.Bindings[symbol]
	if !ok {
		return strategy.StrategyBinding{}, fmt.Errorf("binding not found")
	}
	return bind, nil
}

func (r *Runner) getConfig(symbol string) (config.SymbolConfig, error) {
	symbol = decisionutil.NormalizeSymbol(symbol)
	cfg, ok := r.Configs[symbol]
	if !ok {
		return config.SymbolConfig{}, fmt.Errorf("config not found")
	}
	return cfg, nil
}

func (r *Runner) getEnabled(symbol string) (AgentEnabled, error) {
	symbol = decisionutil.NormalizeSymbol(symbol)
	enabled, ok := r.Enabled[symbol]
	if !ok {
		return AgentEnabled{}, fmt.Errorf("enabled config not found")
	}
	return enabled, nil
}

func (r *Runner) analyze(ctx context.Context, symbol string, comp features.CompressionResult, enabled AgentEnabled) (agent.IndicatorSummary, agent.StructureSummary, agent.MechanicsSummary, AgentPromptSet, AgentInputSet, error) {
	ind, st, mech, prompts, inputs, err := r.Agent.Analyze(ctx, symbol, comp, enabled)
	if err != nil {
		return agent.IndicatorSummary{}, agent.StructureSummary{}, agent.MechanicsSummary{}, AgentPromptSet{}, AgentInputSet{}, err
	}
	return ind, st, mech, prompts, inputs, nil
}

func (r *Runner) judge(ctx context.Context, symbol string, ind agent.IndicatorSummary, st agent.StructureSummary, mech agent.MechanicsSummary, enabled AgentEnabled, dataCtx ProviderDataContext) (provider.IndicatorProviderOut, provider.StructureProviderOut, provider.MechanicsProviderOut, ProviderPromptSet, error) {
	return r.Provider.Judge(ctx, symbol, ind, st, mech, enabled, dataCtx)
}

func (r *Runner) ensureRuleflowEngine() ruleflow.Evaluator {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Ruleflow == nil {
		r.Ruleflow = ruleflow.NewEngine()
	}
	return r.Ruleflow
}
