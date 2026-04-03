package llmapp

import (
	"context"
	"testing"

	"brale-core/internal/decision"
	"brale-core/internal/decision/fund"
	"brale-core/internal/decision/provider"
	"brale-core/internal/execution"
)

type stubRiskSessionProvider struct {
	callResp  string
	callErr   error
	callCount int
}

func (s *stubRiskSessionProvider) Call(_ context.Context, _, _ string) (string, error) {
	s.callCount++
	if s.callErr != nil {
		return "", s.callErr
	}
	return s.callResp, nil
}

func TestDecodeTightenRiskPatch(t *testing.T) {
	payload, err := decodeTightenRiskPatch(`{"stop_loss":99.1,"take_profits":[101.2,102.8]}`)
	if err != nil {
		t.Fatalf("decode tighten patch: %v", err)
	}
	if payload.StopLoss == nil || *payload.StopLoss != 99.1 {
		t.Fatalf("stop_loss=%v", payload.StopLoss)
	}
	if len(payload.TakeProfits) != 2 {
		t.Fatalf("take_profits=%v", payload.TakeProfits)
	}
}

func TestDecodeFlatRiskPatchRequiresEntry(t *testing.T) {
	if _, err := decodeFlatRiskPatch(`{"stop_loss":98.5,"take_profits":[102,104],"take_profit_ratios":[0.6,0.4],"reason":"依据entry与structure设置"}`); err == nil {
		t.Fatalf("decode flat patch should fail when entry is missing")
	}
	payload, err := decodeFlatRiskPatch(`{"entry":100.2,"stop_loss":98.5,"take_profits":[102,104],"take_profit_ratios":[0.6,0.4],"reason":"依据entry与direction设置"}`)
	if err != nil {
		t.Fatalf("decode flat patch: %v", err)
	}
	if payload.Entry == nil || *payload.Entry != 100.2 {
		t.Fatalf("entry=%v", payload.Entry)
	}
	if payload.Reason == nil || *payload.Reason == "" {
		t.Fatalf("reason=%v", payload.Reason)
	}
}

func TestDecodeFlatRiskPatchRequiresReason(t *testing.T) {
	if _, err := decodeFlatRiskPatch(`{"entry":100.2,"stop_loss":98.5,"take_profits":[102,104],"take_profit_ratios":[0.6,0.4]}`); err == nil {
		t.Fatalf("decode flat patch should fail when reason is missing")
	}
}

func TestDecodeFlatRiskPatchIgnoresNonWhitelistedFields(t *testing.T) {
	payload, err := decodeFlatRiskPatch(`{"entry":100.2,"stop_loss":98.5,"take_profits":[102,104],"take_profit_ratios":[0.6,0.4],"reason":"依据entry与direction设置","direction":"long","clear_structure":true,"symbol":"ETHUSDT"}`)
	if err != nil {
		t.Fatalf("decode flat patch with extra fields: %v", err)
	}
	if payload.Entry == nil || *payload.Entry != 100.2 {
		t.Fatalf("entry=%v", payload.Entry)
	}
	if payload.StopLoss == nil || *payload.StopLoss != 98.5 {
		t.Fatalf("stop_loss=%v", payload.StopLoss)
	}
	if len(payload.TakeProfits) != 2 {
		t.Fatalf("take_profits=%v", payload.TakeProfits)
	}
	if len(payload.TakeProfitRatios) != 2 {
		t.Fatalf("take_profit_ratios=%v", payload.TakeProfitRatios)
	}
	if payload.Reason == nil || *payload.Reason == "" {
		t.Fatalf("reason=%v", payload.Reason)
	}
}

func TestBuildFlatRiskPromptInputIncludesPlanSummary(t *testing.T) {
	input := decision.FlatRiskInitInput{
		Symbol: "btcusdt",
		Gate:   fundGateForRiskTests(),
		Plan:   executionPlanForRiskTests(),
	}
	got, err := buildFlatRiskPromptInput(input)
	if err != nil {
		t.Fatalf("build flat risk prompt input: %v", err)
	}
	if got.Symbol != "BTCUSDT" {
		t.Fatalf("symbol=%q, want BTCUSDT", got.Symbol)
	}
	if got.PlanSummary["atr"] != 2.0 {
		t.Fatalf("plan_summary.atr=%v", got.PlanSummary["atr"])
	}
	if got.PlanSummary["max_leverage"] != 5.0 {
		t.Fatalf("plan_summary.max_leverage=%v", got.PlanSummary["max_leverage"])
	}
	if got.PlanSummary["stop_loss"] != 0.0 {
		t.Fatalf("plan_summary.stop_loss=%v", got.PlanSummary["stop_loss"])
	}
}

func TestLLMRiskServiceFlatRiskInitLLMUsesStatelessCall(t *testing.T) {
	providerStub := &stubRiskSessionProvider{callResp: `{"entry":100.2,"stop_loss":98.5,"take_profits":[102,104],"take_profit_ratios":[0.6,0.4],"reason":"依据entry与plan_summary设置"}`}
	svc := LLMRiskService{
		Provider: providerStub,
		Prompts:  LLMPromptBuilder{RiskFlatInitSystem: "risk-system", RiskTightenSystem: "risk-tighten-system"},
	}
	initFn := svc.FlatRiskInitLLM()
	_, err := initFn(context.Background(), decision.FlatRiskInitInput{Symbol: "BTCUSDT", Gate: fundGateForRiskTests(), Plan: executionPlanForRiskTests()})
	if err != nil {
		t.Fatalf("flat risk init: %v", err)
	}
	if providerStub.callCount != 1 {
		t.Fatalf("stateless call_count=%d, want 1", providerStub.callCount)
	}
	patch, err := initFn(context.Background(), decision.FlatRiskInitInput{Symbol: "BTCUSDT", Gate: fundGateForRiskTests(), Plan: executionPlanForRiskTests()})
	if err != nil {
		t.Fatalf("flat risk init: %v", err)
	}
	if patch.Trace == nil || patch.Trace.SystemPrompt != "risk-system" || patch.Trace.UserPrompt == "" || patch.Trace.RawOutput == "" {
		t.Fatalf("trace=%#v", patch.Trace)
	}
}

func TestBuildTightenRiskPromptInput(t *testing.T) {
	input := decision.TightenRiskUpdateInput{
		Symbol:             "btcusdt",
		Side:               "long",
		Entry:              100,
		MarkPrice:          105,
		ATR:                2,
		CurrentStopLoss:    95,
		CurrentTakeProfits: []float64{108, 112},
		InPositionIndicator: provider.InPositionIndicatorOut{
			MonitorTag: "tighten",
		},
	}
	got, err := buildTightenRiskPromptInput(input)
	if err != nil {
		t.Fatalf("build tighten prompt input: %v", err)
	}
	if got.Symbol != "BTCUSDT" {
		t.Fatalf("symbol=%q, want BTCUSDT", got.Symbol)
	}
	if got.Direction != "long" {
		t.Fatalf("direction=%q, want long", got.Direction)
	}
	if len(got.CurrentTakeProfits) != 2 {
		t.Fatalf("current_take_profits=%v", got.CurrentTakeProfits)
	}
}

func TestLLMRiskServiceTightenRiskLLMUsesStatelessCall(t *testing.T) {
	providerStub := &stubRiskSessionProvider{callResp: `{"stop_loss":101.2,"take_profits":[103.1,104.6]}`}
	svc := LLMRiskService{
		Provider: providerStub,
		Prompts:  LLMPromptBuilder{RiskFlatInitSystem: "risk-system", RiskTightenSystem: "risk-tighten-system"},
	}
	tightenFn := svc.TightenRiskLLM()
	if _, err := tightenFn(context.Background(), tightenInputForRiskTests()); err != nil {
		t.Fatalf("tighten risk llm: %v", err)
	}
	if providerStub.callCount != 1 {
		t.Fatalf("call_count=%d, want 1", providerStub.callCount)
	}
	patch, err := tightenFn(context.Background(), tightenInputForRiskTests())
	if err != nil {
		t.Fatalf("tighten risk llm: %v", err)
	}
	if patch.Trace == nil || patch.Trace.SystemPrompt != "risk-tighten-system" || patch.Trace.UserPrompt == "" || patch.Trace.RawOutput == "" {
		t.Fatalf("trace=%#v", patch.Trace)
	}
}

func executionPlanForRiskTests() execution.ExecutionPlan {
	return execution.ExecutionPlan{
		Direction: "long",
		Entry:     100,
		RiskPct:   0.01,
		RiskAnnotations: execution.RiskAnnotations{
			ATR:          2,
			MaxInvestPct: 0.5,
			MaxInvestAmt: 500,
			MaxLeverage:  5,
		},
	}
}

func fundGateForRiskTests() fund.GateDecision {
	return fund.GateDecision{
		Direction:      "long",
		Grade:          1,
		GateReason:     "PASS",
		DecisionAction: "ALLOW",
		Derived: map[string]any{
			"direction_consensus": map[string]any{"direction": "long", "confidence": 0.8},
			"provider_structure":  map[string]any{"integrity": true},
			"providers":           map[string]any{"indicator": "ok"},
		},
	}
}

func tightenInputForRiskTests() decision.TightenRiskUpdateInput {
	return decision.TightenRiskUpdateInput{
		Symbol:             "BTCUSDT",
		Side:               "long",
		Entry:              100,
		MarkPrice:          105,
		ATR:                2,
		CurrentStopLoss:    99,
		CurrentTakeProfits: []float64{108, 112},
		Gate:               fundGateForRiskTests(),
	}
}
