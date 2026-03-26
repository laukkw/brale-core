package decision

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/decision/decisionfmt"
	"brale-core/internal/decision/decisionmode"
	"brale-core/internal/decision/fund"
	"brale-core/internal/execution"
	"brale-core/internal/llm"
	"brale-core/internal/snapshot"
	"brale-core/internal/strategy"

	"go.uber.org/zap"
)

type countingNotifier struct {
	messages []string
}

func (c *countingNotifier) SendGate(context.Context, decisionfmt.DecisionInput, decisionfmt.DecisionReport) error {
	return nil
}

func (c *countingNotifier) SendRiskPlanUpdate(context.Context, RiskPlanUpdateNotice) error {
	return nil
}

func (c *countingNotifier) SendError(_ context.Context, message string) error {
	c.messages = append(c.messages, message)
	return nil
}

func TestEnrichRunOptionsInjectsRiskModeFromBinding(t *testing.T) {
	p := &Pipeline{
		Bindings: map[string]strategy.StrategyBinding{
			"ETHUSDT": {RiskManagement: config.RiskManagementConfig{RiskStrategy: config.RiskStrategyConfig{Mode: "llm"}}},
			"BTCUSDT": {RiskManagement: config.RiskManagementConfig{RiskStrategy: config.RiskStrategyConfig{Mode: "native"}}},
		},
	}

	runOpts := p.enrichRunOptions(
		RunOptions{BuildPlan: true},
		[]string{"ETHUSDT", "BTCUSDT"},
		map[string]decisionmode.Mode{"ETHUSDT": decisionmode.ModeFlat, "BTCUSDT": decisionmode.ModeFlat},
	)

	if got := runOpts.RiskStrategyModeBySymbol["ETHUSDT"]; got != execution.PlanSourceLLM {
		t.Fatalf("ETHUSDT risk mode=%q want=%q", got, execution.PlanSourceLLM)
	}
	if got := runOpts.RiskStrategyModeBySymbol["BTCUSDT"]; got != execution.PlanSourceGo {
		t.Fatalf("BTCUSDT risk mode=%q want=%q", got, execution.PlanSourceGo)
	}
}

func TestEnrichRunOptionsKeepsExplicitRiskMode(t *testing.T) {
	p := &Pipeline{
		Bindings: map[string]strategy.StrategyBinding{
			"ETHUSDT": {RiskManagement: config.RiskManagementConfig{RiskStrategy: config.RiskStrategyConfig{Mode: "native"}}},
		},
	}

	runOpts := p.enrichRunOptions(
		RunOptions{
			BuildPlan: true,
			RiskStrategyModeBySymbol: map[string]string{
				"ETHUSDT": execution.PlanSourceLLM,
			},
		},
		[]string{"ETHUSDT"},
		map[string]decisionmode.Mode{"ETHUSDT": decisionmode.ModeFlat},
	)

	if got := runOpts.RiskStrategyModeBySymbol["ETHUSDT"]; got != execution.PlanSourceLLM {
		t.Fatalf("ETHUSDT risk mode override=%q want=%q", got, execution.PlanSourceLLM)
	}
}

func TestValidatePlanRejectsIncompleteValidLLMPlan(t *testing.T) {
	err := validatePlan(execution.ExecutionPlan{
		Symbol:           "ETHUSDT",
		Valid:            true,
		PlanSource:       execution.PlanSourceLLM,
		PositionID:       "ETHUSDT-1",
		CreatedAt:        time.Now().UTC(),
		Entry:            2163.77,
		StopLoss:         0,
		TakeProfits:      nil,
		TakeProfitRatios: nil,
	})
	if err == nil {
		t.Fatal("expected validatePlan to reject incomplete valid llm plan")
	}
}

func TestValidatePlanAcceptsCompleteValidPlan(t *testing.T) {
	err := validatePlan(execution.ExecutionPlan{
		Symbol:           "ETHUSDT",
		Valid:            true,
		PlanSource:       execution.PlanSourceLLM,
		PositionID:       "ETHUSDT-1",
		CreatedAt:        time.Now().UTC(),
		Entry:            2163.77,
		StopLoss:         2120.0,
		TakeProfits:      []float64{2200, 2240},
		TakeProfitRatios: []float64{0.6, 0.4},
	})
	if err != nil {
		t.Fatalf("validatePlan returned error: %v", err)
	}
}

func TestFirstFailedSymbolResultReturnsFirstError(t *testing.T) {
	failure := errors.New("boom")
	res, ok := firstFailedSymbolResult([]SymbolResult{{Symbol: "BTCUSDT"}, {Symbol: "ETHUSDT", Err: failure}, {Symbol: "SOLUSDT", Err: errors.New("later")}})
	if !ok {
		t.Fatal("expected failed result")
	}
	if res.Symbol != "ETHUSDT" {
		t.Fatalf("symbol=%s want=ETHUSDT", res.Symbol)
	}
	if !errors.Is(res.Err, failure) {
		t.Fatalf("err=%v want=%v", res.Err, failure)
	}
}

func TestHandleSymbolErrorNotifiesWithoutPersistingGate(t *testing.T) {
	notifier := &countingNotifier{}
	gateStoreCalls := 0
	p := &Pipeline{
		Notifier: notifier,
		GateStore: func(context.Context, snapshot.MarketSnapshot, uint, string, fund.GateDecision, fund.ProviderBundle) error {
			gateStoreCalls++
			return nil
		},
	}
	boom := errors.New("boom")
	err := p.handleSymbolError(context.Background(), zap.NewNop(), SymbolResult{Symbol: "ETHUSDT", Err: boom})
	if !errors.Is(err, boom) {
		t.Fatalf("err=%v want=%v", err, boom)
	}
	if gateStoreCalls != 0 {
		t.Fatalf("gate store calls=%d want=0", gateStoreCalls)
	}
	if len(notifier.messages) != 1 {
		t.Fatalf("notifier messages=%v want 1 message", notifier.messages)
	}
	message := notifier.messages[0]
	if !strings.Contains(message, "decision pipeline failed") {
		t.Fatalf("message=%q missing header", message)
	}
	if !strings.Contains(message, "error=boom") {
		t.Fatalf("message=%q missing error body", message)
	}
}

func TestHandleSymbolErrorNotificationIncludesRoundID(t *testing.T) {
	notifier := &countingNotifier{}
	p := &Pipeline{Notifier: notifier}
	roundID, err := llm.NewRoundID("round-notify-1")
	if err != nil {
		t.Fatalf("round id: %v", err)
	}
	ctx := llm.WithSessionRoundID(context.Background(), roundID)
	ctx = llm.WithSessionFlow(ctx, llm.LLMFlowFlat)
	boom := errors.New("llm agent stage failed: symbol=ETHUSDT stage=structure: boom")
	if err := p.handleSymbolError(ctx, zap.NewNop(), SymbolResult{Symbol: "ETHUSDT", Err: boom}); !errors.Is(err, boom) {
		t.Fatalf("err=%v want=%v", err, boom)
	}
	if len(notifier.messages) != 1 {
		t.Fatalf("notifier messages=%v want 1 message", notifier.messages)
	}
	message := notifier.messages[0]
	if !strings.Contains(message, "round_id=round-notify-1") {
		t.Fatalf("message=%q missing round id", message)
	}
	if !strings.Contains(message, "flow=flat") {
		t.Fatalf("message=%q missing flow", message)
	}
	if !strings.Contains(message, "symbol=ETHUSDT stage=structure") {
		t.Fatalf("message=%q missing stage details", message)
	}
}
