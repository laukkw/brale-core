package decision

import (
	"context"
	"fmt"
	"testing"

	"brale-core/internal/config"
	"brale-core/internal/decision/agent"
	"brale-core/internal/decision/features"
	"brale-core/internal/decision/fund"
	"brale-core/internal/decision/provider"
	"brale-core/internal/llm"
	"brale-core/internal/memory"
	"brale-core/internal/prompt/positionprompt"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestRunnerRunAgentAndProviderStagesInjectsWorkingMemoryPromptContext(t *testing.T) {
	agentSvc := &memoryAwareAgentService{}
	providerSvc := &memoryAwareProviderService{}
	wm := &stubWorkingMemory{prompt: "- [1h前] dir=long"}
	runner := &Runner{
		Agent:         agentSvc,
		Provider:      providerSvc,
		WorkingMemory: wm,
		Configs: map[string]config.SymbolConfig{
			"BTCUSDT": {Symbol: "BTCUSDT", Intervals: []string{"15m"}},
		},
	}

	res, shouldFinalize := runner.runAgentAndProviderStages(context.Background(), "BTCUSDT", workingMemoryCompressionResult(123.4, 4.5), &runnerSymbolInputs{
		Enabled:        AgentEnabled{},
		ScoreThreshold: 0.3,
		ConfThreshold:  0.5,
		Logger:         zap.NewNop(),
		Config:         config.SymbolConfig{Symbol: "BTCUSDT", Intervals: []string{"15m"}},
	})
	if !shouldFinalize {
		t.Fatal("shouldFinalize=false want true")
	}
	if res.Err != nil {
		t.Fatalf("res.Err=%v", res.Err)
	}
	if agentSvc.prompt != wm.prompt {
		t.Fatalf("agent prompt context=%q want %q", agentSvc.prompt, wm.prompt)
	}
	if providerSvc.prompt != wm.prompt {
		t.Fatalf("provider prompt context=%q want %q", providerSvc.prompt, wm.prompt)
	}
	if wm.gotSymbol != "BTCUSDT" {
		t.Fatalf("symbol=%q want BTCUSDT", wm.gotSymbol)
	}
	if wm.gotPrice != 123.4 {
		t.Fatalf("price=%v want 123.4", wm.gotPrice)
	}
}

func TestLogDecisionStageStartedDoesNotDuplicateBoundSymbol(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core).With(zap.String("symbol", "BTCUSDT"))

	logDecisionStageStarted(logger, "agent stage started", AgentEnabled{
		Indicator: true,
		Structure: true,
		Mechanics: true,
	})

	if observed.Len() != 1 {
		t.Fatalf("logs=%d want 1", observed.Len())
	}
	symbolFields := 0
	for _, field := range observed.All()[0].Context {
		if field.Key == "symbol" {
			symbolFields++
		}
	}
	if symbolFields != 1 {
		t.Fatalf("symbol fields=%d want 1 (context=%v)", symbolFields, observed.All()[0].Context)
	}
}

func TestPipelineRecordWorkingMemoryBuildsEntry(t *testing.T) {
	wm := &stubWorkingMemory{}
	roundID, err := llm.NewRoundID("round-working-memory")
	if err != nil {
		t.Fatalf("round id: %v", err)
	}
	ctx := llm.WithSessionRoundID(context.Background(), roundID)
	p := &Pipeline{
		Runner:        &Runner{Configs: map[string]config.SymbolConfig{"BTCUSDT": {Symbol: "BTCUSDT", Intervals: []string{"15m"}}}},
		WorkingMemory: wm,
	}

	p.recordWorkingMemory(ctx, SymbolResult{
		Symbol:             "BTCUSDT",
		Gate:               fund.GateDecision{DecisionAction: "ALLOW", GateReason: "BREAKOUT"},
		ConsensusDirection: "long",
		ConsensusScore:     0.62,
	}, workingMemoryCompressionResult(123.4, 4.5))

	if len(wm.entries) != 1 {
		t.Fatalf("entries=%d want 1", len(wm.entries))
	}
	entry := wm.entries[0]
	if entry.RoundID != roundID.String() {
		t.Fatalf("round_id=%q want %q", entry.RoundID, roundID.String())
	}
	if entry.Direction != "long" {
		t.Fatalf("direction=%q want long", entry.Direction)
	}
	if entry.GateAction != "ALLOW" {
		t.Fatalf("gate_action=%q want ALLOW", entry.GateAction)
	}
	if entry.GateReason != "BREAKOUT" {
		t.Fatalf("gate_reason=%q want BREAKOUT", entry.GateReason)
	}
	if entry.Score != 0.62 {
		t.Fatalf("score=%v want 0.62", entry.Score)
	}
	if entry.PriceAtTime != 123.4 {
		t.Fatalf("price_at_time=%v want 123.4", entry.PriceAtTime)
	}
	if entry.ATR != 4.5 {
		t.Fatalf("atr=%v want 4.5", entry.ATR)
	}
	if entry.Outcome != memory.OutcomePending {
		t.Fatalf("outcome=%q want pending", entry.Outcome)
	}
}

type memoryAwareAgentService struct {
	prompt string
}

func (s *memoryAwareAgentService) Analyze(ctx context.Context, _ string, _ features.CompressionResult, _ AgentEnabled) (agent.IndicatorSummary, agent.StructureSummary, agent.MechanicsSummary, AgentPromptSet, AgentInputSet, error) {
	s.prompt = memory.PromptContextFromContext(ctx)
	return agent.IndicatorSummary{}, agent.StructureSummary{}, agent.MechanicsSummary{}, AgentPromptSet{}, AgentInputSet{}, nil
}

type memoryAwareProviderService struct {
	prompt string
}

func (s *memoryAwareProviderService) Judge(ctx context.Context, _ string, _ agent.IndicatorSummary, _ agent.StructureSummary, _ agent.MechanicsSummary, _ AgentEnabled, _ ProviderDataContext) (provider.IndicatorProviderOut, provider.StructureProviderOut, provider.MechanicsProviderOut, ProviderPromptSet, error) {
	s.prompt = memory.PromptContextFromContext(ctx)
	return provider.IndicatorProviderOut{}, provider.StructureProviderOut{}, provider.MechanicsProviderOut{}, ProviderPromptSet{}, nil
}

func (s *memoryAwareProviderService) JudgeInPosition(ctx context.Context, _ string, _ agent.IndicatorSummary, _ agent.StructureSummary, _ agent.MechanicsSummary, _ positionprompt.Summary, _ AgentEnabled, _ ProviderDataContext) (provider.InPositionIndicatorOut, provider.InPositionStructureOut, provider.InPositionMechanicsOut, ProviderPromptSet, error) {
	s.prompt = memory.PromptContextFromContext(ctx)
	return provider.InPositionIndicatorOut{}, provider.InPositionStructureOut{}, provider.InPositionMechanicsOut{}, ProviderPromptSet{}, nil
}

type stubWorkingMemory struct {
	prompt    string
	gotSymbol string
	gotPrice  float64
	entries   []memory.Entry
}

func (s *stubWorkingMemory) FormatForPrompt(symbol string, currentPrice float64) string {
	s.gotSymbol = symbol
	s.gotPrice = currentPrice
	if s.prompt != "" {
		return s.prompt
	}
	return "- [1h前] dir=neutral"
}

func (s *stubWorkingMemory) Push(_ string, entry memory.Entry) {
	s.entries = append(s.entries, entry)
}

func workingMemoryCompressionResult(price, atr float64) features.CompressionResult {
	return features.CompressionResult{
		Indicators: map[string]map[string]features.IndicatorJSON{
			"BTCUSDT": {
				"15m": {
					Symbol:   "BTCUSDT",
					Interval: "15m",
					RawJSON:  []byte(fmt.Sprintf(`{"market":{"current_price":%.1f},"data":{"atr":{"latest":%.1f}}}`, price, atr)),
				},
			},
		},
	}
}
