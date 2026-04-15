package runtime

import (
	"context"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/decision"
	"brale-core/internal/decision/decisionutil"
	"brale-core/internal/llm"
	llmapp "brale-core/internal/llm/app"
	"brale-core/internal/llm/promptreg"
	"brale-core/internal/store"

	"go.uber.org/zap"
)

func buildSymbolAgents(ctx context.Context, sys config.SystemConfig, symbolCfg config.SymbolConfig, promptStore store.PromptRegistryStore) (decision.AgentService, decision.ProviderService, *llmapp.LLMRunTracker) {
	cache := llmapp.NewLLMStageCache()
	tracker := llmapp.NewLLMRunTracker()
	builder, err := loadPromptBuilder(ctx, promptStore, zap.NewNop())
	if err != nil {
		builder = fallbackPromptBuilder()
	}
	agentRunner := &decision.AgentRunner{
		Indicator: newLLMClient(sys, symbolCfg.LLM.Agent.Indicator),
		Structure: newLLMClient(sys, symbolCfg.LLM.Agent.Structure),
		Mechanics: newLLMClient(sys, symbolCfg.LLM.Agent.Mechanics),
	}
	providerRunner := &decision.ProviderRunner{
		Indicator: newLLMClient(sys, symbolCfg.LLM.Provider.Indicator),
		Structure: newLLMClient(sys, symbolCfg.LLM.Provider.Structure),
		Mechanics: newLLMClient(sys, symbolCfg.LLM.Provider.Mechanics),
	}
	decisionInterval := ""
	if len(symbolCfg.Intervals) > 0 {
		decisionInterval = decisionutil.SelectDecisionInterval(symbolCfg.Intervals)
	}
	return llmapp.LLMAgentService{Runner: agentRunner, Prompts: builder, Cache: cache, Tracker: tracker, DecisionInterval: decisionInterval}, llmapp.LLMProviderService{Runner: providerRunner, Prompts: builder, Cache: cache, Tracker: tracker}, tracker
}

func loadPromptBuilder(ctx context.Context, promptStore store.PromptRegistryStore, logger *zap.Logger) (llmapp.LLMPromptBuilder, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	loader := promptreg.NewLoader(promptStore, config.PromptRegistryDefaults(), logger)
	resolve := func(role, stage string) (string, string, error) {
		return loader.Resolve(ctx, role, stage)
	}
	agentIndicator, agentIndicatorVersion, err := resolve("agent", "indicator")
	if err != nil {
		return llmapp.LLMPromptBuilder{}, err
	}
	agentStructure, agentStructureVersion, err := resolve("agent", "structure")
	if err != nil {
		return llmapp.LLMPromptBuilder{}, err
	}
	agentMechanics, agentMechanicsVersion, err := resolve("agent", "mechanics")
	if err != nil {
		return llmapp.LLMPromptBuilder{}, err
	}
	providerIndicator, providerIndicatorVersion, err := resolve("provider", "indicator")
	if err != nil {
		return llmapp.LLMPromptBuilder{}, err
	}
	providerStructure, providerStructureVersion, err := resolve("provider", "structure")
	if err != nil {
		return llmapp.LLMPromptBuilder{}, err
	}
	providerMechanics, providerMechanicsVersion, err := resolve("provider", "mechanics")
	if err != nil {
		return llmapp.LLMPromptBuilder{}, err
	}
	providerInPosIndicator, providerInPosIndicatorVersion, err := resolve("provider_in_position", "indicator")
	if err != nil {
		return llmapp.LLMPromptBuilder{}, err
	}
	providerInPosStructure, providerInPosStructureVersion, err := resolve("provider_in_position", "structure")
	if err != nil {
		return llmapp.LLMPromptBuilder{}, err
	}
	providerInPosMechanics, providerInPosMechanicsVersion, err := resolve("provider_in_position", "mechanics")
	if err != nil {
		return llmapp.LLMPromptBuilder{}, err
	}
	riskFlatInit, riskFlatInitVersion, err := resolve("risk", "flat_init")
	if err != nil {
		return llmapp.LLMPromptBuilder{}, err
	}
	riskTighten, riskTightenVersion, err := resolve("risk", "tighten_update")
	if err != nil {
		return llmapp.LLMPromptBuilder{}, err
	}
	return llmapp.LLMPromptBuilder{
		AgentIndicatorSystem:      agentIndicator,
		AgentIndicatorVersion:     agentIndicatorVersion,
		AgentStructureSystem:      agentStructure,
		AgentStructureVersion:     agentStructureVersion,
		AgentMechanicsSystem:      agentMechanics,
		AgentMechanicsVersion:     agentMechanicsVersion,
		ProviderIndicatorSystem:   providerIndicator,
		ProviderIndicatorVersion:  providerIndicatorVersion,
		ProviderStructureSystem:   providerStructure,
		ProviderStructureVersion:  providerStructureVersion,
		ProviderMechanicsSystem:   providerMechanics,
		ProviderMechanicsVersion:  providerMechanicsVersion,
		ProviderInPosIndicatorSys: providerInPosIndicator,
		ProviderInPosIndicatorVer: providerInPosIndicatorVersion,
		ProviderInPosStructureSys: providerInPosStructure,
		ProviderInPosStructureVer: providerInPosStructureVersion,
		ProviderInPosMechanicsSys: providerInPosMechanics,
		ProviderInPosMechanicsVer: providerInPosMechanicsVersion,
		RiskFlatInitSystem:        riskFlatInit,
		RiskFlatInitVersion:       riskFlatInitVersion,
		RiskTightenSystem:         riskTighten,
		RiskTightenVersion:        riskTightenVersion,
		UserFormat:                llmapp.UserPromptFormatBullet,
	}, nil
}

func fallbackPromptBuilder() llmapp.LLMPromptBuilder {
	defaults := config.DefaultPromptDefaults()
	return llmapp.LLMPromptBuilder{
		AgentIndicatorSystem:      defaults.AgentIndicator,
		AgentIndicatorVersion:     "builtin",
		AgentStructureSystem:      defaults.AgentStructure,
		AgentStructureVersion:     "builtin",
		AgentMechanicsSystem:      defaults.AgentMechanics,
		AgentMechanicsVersion:     "builtin",
		ProviderIndicatorSystem:   defaults.ProviderIndicator,
		ProviderIndicatorVersion:  "builtin",
		ProviderStructureSystem:   defaults.ProviderStructure,
		ProviderStructureVersion:  "builtin",
		ProviderMechanicsSystem:   defaults.ProviderMechanics,
		ProviderMechanicsVersion:  "builtin",
		ProviderInPosIndicatorSys: defaults.ProviderInPositionIndicator,
		ProviderInPosIndicatorVer: "builtin",
		ProviderInPosStructureSys: defaults.ProviderInPositionStructure,
		ProviderInPosStructureVer: "builtin",
		ProviderInPosMechanicsSys: defaults.ProviderInPositionMechanics,
		ProviderInPosMechanicsVer: "builtin",
		RiskFlatInitSystem:        defaults.RiskFlatInit,
		RiskFlatInitVersion:       "builtin",
		RiskTightenSystem:         defaults.RiskTightenUpdate,
		RiskTightenVersion:        "builtin",
		UserFormat:                llmapp.UserPromptFormatBullet,
	}
}

func NewLLMClient(sys config.SystemConfig, role config.LLMRoleConfig) *llm.OpenAIClient {
	temp := 0.0
	if role.Temperature != nil {
		temp = *role.Temperature
	}
	modelCfg, _ := config.LookupLLMModelConfig(sys, role.Model)
	timeoutSec := 30
	if modelCfg.TimeoutSec != nil {
		timeoutSec = *modelCfg.TimeoutSec
	}
	structuredOutput := false
	if modelCfg.StructuredOutput != nil {
		structuredOutput = *modelCfg.StructuredOutput
	}
	return &llm.OpenAIClient{
		Endpoint:         modelCfg.Endpoint,
		Model:            role.Model,
		APIKey:           modelCfg.APIKey,
		Timeout:          time.Duration(timeoutSec) * time.Second,
		Temperature:      temp,
		StructuredOutput: structuredOutput,
	}
}

func newLLMClient(sys config.SystemConfig, role config.LLMRoleConfig) *llm.OpenAIClient {
	return NewLLMClient(sys, role)
}
