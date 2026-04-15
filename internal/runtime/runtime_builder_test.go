package runtime

import (
	"context"
	"testing"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/decision"
	llmapp "brale-core/internal/llm/app"
	"brale-core/internal/position"
	"brale-core/internal/strategy"
)

func TestNormalizeSymbol(t *testing.T) {
	cases := map[string]string{
		"":        "",
		"btc":     "BTCUSDT",
		"BTC":     "BTCUSDT",
		"BTCUSDT": "BTCUSDT",
		" eth ":   "ETHUSDT",
	}
	for input, want := range cases {
		got := NormalizeSymbol(input)
		if got != want {
			t.Fatalf("NormalizeSymbol(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestValidateInitialExitStructureInterval(t *testing.T) {
	symbolCfg := config.SymbolConfig{
		Symbol:    "BTCUSDT",
		Intervals: []string{"1h", "4h", "1d"},
	}
	stratCfg := config.StrategyConfig{
		Symbol: "BTCUSDT",
		RiskManagement: config.RiskManagementConfig{
			InitialExit: config.InitialExitConfig{
				Policy:            "atr_structure_v1",
				StructureInterval: "4h",
			},
		},
	}
	if err := validateInitialExitStructureInterval(symbolCfg, stratCfg); err != nil {
		t.Fatalf("expected valid interval, got %v", err)
	}

	stratCfg.RiskManagement.InitialExit.StructureInterval = "15m"
	if err := validateInitialExitStructureInterval(symbolCfg, stratCfg); err == nil {
		t.Fatalf("expected invalid interval error")
	}
}

func TestBuildSymbolRuntimeInjectsCoreDependencies(t *testing.T) {
	sys := testRuntimeSystemConfig()
	symbolCfg := testRuntimeSymbolConfig()

	agentSvc, providerSvc, tracker := buildSymbolAgents(context.Background(), sys, symbolCfg, nil)
	if tracker == nil {
		t.Fatalf("tracker should not be nil")
	}

	agentImpl, ok := agentSvc.(llmapp.LLMAgentService)
	if !ok {
		t.Fatalf("agent service type=%T, want llmapp.LLMAgentService", agentSvc)
	}
	if agentImpl.Cache == nil {
		t.Fatalf("agent cache should not be nil")
	}

	providerImpl, ok := providerSvc.(llmapp.LLMProviderService)
	if !ok {
		t.Fatalf("provider service type=%T, want llmapp.LLMProviderService", providerSvc)
	}
	if providerImpl.Cache == nil {
		t.Fatalf("provider cache should not be nil")
	}

	pipeline, err := buildPipeline(config.SystemConfig{}, nil, nil, &position.PositionService{PlanCache: position.NewPlanCache()}, nil, nil, time.Minute, symbolCfg.Symbol, strategy.StrategyBinding{}, symbolCfg, config.StrategyConfig{}, &decision.Runner{}, decision.NewExitConfirmCache(), nil, nil, nil)
	if err != nil {
		t.Fatalf("build pipeline: %v", err)
	}
	if pipeline.Core.Positioner == nil || pipeline.Core.PlanCache == nil {
		t.Fatalf("pipeline core deps not wired: %+v", pipeline.Core)
	}
	if pipeline.Core.Positioner.PlanCache != pipeline.Core.PlanCache {
		t.Fatalf("pipeline core plan cache mismatch: positioner=%p core=%p", pipeline.Core.Positioner.PlanCache, pipeline.Core.PlanCache)
	}
}

func TestBuildSymbolRuntimeWiresRiskCallbacks(t *testing.T) {
	sys := testRuntimeSystemConfig()
	symbolCfg := testRuntimeSymbolConfig()
	stratCfg := config.StrategyConfig{
		Symbol: "BTCUSDT",
		RiskManagement: config.RiskManagementConfig{
			RiskPerTradePct: 0.01,
		},
	}
	enabledCfg := config.AgentEnabled{Indicator: true, Structure: true, Mechanics: false}
	enabledApp := decision.AgentEnabled{Indicator: true, Structure: true, Mechanics: false}
	positioner := &position.PositionService{PlanCache: position.NewPlanCache()}

	rt, err := buildSymbolRuntimeFromConfig(context.TODO(), sys, symbolCfg, stratCfg, strategy.StrategyBinding{}, nil, nil, positioner, &position.RiskPlanService{}, nil, time.Minute, enabledCfg, enabledApp, false)
	if err != nil {
		t.Fatalf("build symbol runtime: %v", err)
	}
	if rt.Pipeline == nil {
		t.Fatalf("pipeline should not be nil")
	}

	agentImpl, ok := rt.Pipeline.Runner.Agent.(llmapp.LLMAgentService)
	if !ok {
		t.Fatalf("runner agent type=%T, want llmapp.LLMAgentService", rt.Pipeline.Runner.Agent)
	}
	providerImpl, ok := rt.Pipeline.Runner.Provider.(llmapp.LLMProviderService)
	if !ok {
		t.Fatalf("runner provider type=%T, want llmapp.LLMProviderService", rt.Pipeline.Runner.Provider)
	}
	if providerImpl.Cache == nil {
		t.Fatalf("provider cache should not be nil")
	}
	if agentImpl.Cache == nil {
		t.Fatalf("agent cache should not be nil")
	}
	if rt.Pipeline.Runner.FlatRiskInitLLM == nil {
		t.Fatalf("runner flat risk init callback should be wired")
	}
	if rt.Pipeline.Runner.TightenRiskLLM == nil {
		t.Fatalf("runner tighten risk callback should be wired")
	}
	if rt.Pipeline.TightenRiskLLM == nil {
		t.Fatalf("pipeline tighten risk callback should be wired")
	}
}

func TestBuildSymbolRuntimeWiresEpisodicAndSemanticMemory(t *testing.T) {
	sys := testRuntimeSystemConfig()
	symbolCfg := testRuntimeSymbolConfig()
	symbolCfg.Memory = config.MemoryConfig{
		Enabled:              true,
		WorkingMemorySize:    config.DefaultWorkingMemorySize,
		EpisodicEnabled:      true,
		EpisodicTTLDays:      config.DefaultEpisodicTTLDays,
		EpisodicMaxPerSymbol: config.DefaultEpisodicMaxPerSymbol,
		SemanticEnabled:      true,
		SemanticMaxRules:     config.DefaultSemanticMaxRules,
	}
	stratCfg := config.StrategyConfig{Symbol: "BTCUSDT"}
	enabledCfg := config.AgentEnabled{Indicator: true, Structure: true, Mechanics: false}
	enabledApp := decision.AgentEnabled{Indicator: true, Structure: true, Mechanics: false}
	positioner := &position.PositionService{PlanCache: position.NewPlanCache()}

	rt, err := buildSymbolRuntimeFromConfig(context.TODO(), sys, symbolCfg, stratCfg, strategy.StrategyBinding{}, nil, nil, positioner, &position.RiskPlanService{}, nil, time.Minute, enabledCfg, enabledApp, false)
	if err != nil {
		t.Fatalf("build symbol runtime: %v", err)
	}
	if rt.Pipeline == nil || rt.Pipeline.Runner == nil {
		t.Fatalf("pipeline or runner should not be nil")
	}
	if rt.Pipeline.WorkingMemory == nil {
		t.Fatalf("working memory should be wired")
	}
	if rt.Pipeline.EpisodicMemory == nil || rt.Pipeline.Runner.EpisodicMemory == nil {
		t.Fatalf("episodic memory should be wired on pipeline and runner")
	}
	if rt.Pipeline.SemanticMemory == nil || rt.Pipeline.Runner.SemanticMemory == nil {
		t.Fatalf("semantic memory should be wired on pipeline and runner")
	}
}

func TestBuildSymbolAgentsIncludesFlatRiskPromptDefault(t *testing.T) {
	sys := testRuntimeSystemConfig()
	symbolCfg := testRuntimeSymbolConfig()

	agentSvc, providerSvc, _ := buildSymbolAgents(context.Background(), sys, symbolCfg, nil)
	agentImpl, ok := agentSvc.(llmapp.LLMAgentService)
	if !ok {
		t.Fatalf("agent service type=%T, want llmapp.LLMAgentService", agentSvc)
	}
	providerImpl, ok := providerSvc.(llmapp.LLMProviderService)
	if !ok {
		t.Fatalf("provider service type=%T, want llmapp.LLMProviderService", providerSvc)
	}
	if agentImpl.Prompts.ProviderStructureSystem == "" {
		t.Fatalf("agent prompt builder missing ProviderStructureSystem")
	}
	if providerImpl.Prompts.ProviderStructureSystem == "" {
		t.Fatalf("provider prompt builder missing ProviderStructureSystem")
	}
}

func TestBuildSymbolAgentsInjectsDecisionInterval(t *testing.T) {
	sys := testRuntimeSystemConfig()
	symbolCfg := testRuntimeSymbolConfig()
	symbolCfg.Intervals = []string{"1h", "4h", "1d"}

	agentSvc, _, _ := buildSymbolAgents(context.Background(), sys, symbolCfg, nil)
	agentImpl, ok := agentSvc.(llmapp.LLMAgentService)
	if !ok {
		t.Fatalf("agent service type=%T, want llmapp.LLMAgentService", agentSvc)
	}
	if agentImpl.DecisionInterval != "1h" {
		t.Fatalf("decision interval=%q, want 1h", agentImpl.DecisionInterval)
	}
}

func TestBuildMetricsServiceRequiresMechanicsAndIntervals(t *testing.T) {
	symbolCfg := config.SymbolConfig{
		Symbol:    "BTCUSDT",
		Intervals: []string{"1h"},
	}
	enabled := config.AgentEnabled{Mechanics: true}

	if svc := buildMetricsService(symbolCfg, enabled); svc == nil {
		t.Fatalf("buildMetricsService(...) = nil, want service")
	}
	if svc := buildMetricsService(symbolCfg, config.AgentEnabled{}); svc != nil {
		t.Fatalf("buildMetricsService with mechanics disabled = %v, want nil", svc)
	}
}

func testRuntimeSystemConfig() config.SystemConfig {
	return config.SystemConfig{
		ExecutionSystem: "paper",
		LLMModels: map[string]config.LLMModelConfig{
			"agent-indicator":    {},
			"agent-structure":    {},
			"agent-mechanics":    {},
			"provider-indicator": {},
			"provider-structure": {},
			"provider-mechanics": {},
		},
	}
}

func testRuntimeSymbolConfig() config.SymbolConfig {
	return config.SymbolConfig{
		Hash:       "symbol-hash",
		Symbol:     "BTCUSDT",
		Intervals:  []string{"1h"},
		KlineLimit: 200,
		LLM: config.SymbolLLMConfig{
			Agent: config.LLMRoleSet{
				Indicator: config.LLMRoleConfig{Model: "agent-indicator"},
				Structure: config.LLMRoleConfig{Model: "agent-structure"},
				Mechanics: config.LLMRoleConfig{Model: "agent-mechanics"},
			},
			Provider: config.LLMRoleSet{
				Indicator: config.LLMRoleConfig{Model: "provider-indicator"},
				Structure: config.LLMRoleConfig{Model: "provider-structure"},
				Mechanics: config.LLMRoleConfig{Model: "provider-mechanics"},
			},
		},
	}
}
