// 本文件主要内容：流程测试的辅助实现与假对象。

package test

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/decision"
	"brale-core/internal/decision/agent"
	"brale-core/internal/decision/features"
	"brale-core/internal/decision/fsm"
	"brale-core/internal/decision/fund"
	"brale-core/internal/decision/provider"
	"brale-core/internal/decision/ruleflow"
	"brale-core/internal/execution"
	"brale-core/internal/market"
	"brale-core/internal/position"
	"brale-core/internal/prompt/positionprompt"
	"brale-core/internal/reconcile"
	"brale-core/internal/snapshot"
	"brale-core/internal/store"
	"brale-core/internal/strategy"
)

type fakeSnapshotter struct {
	snap snapshot.MarketSnapshot
}

func (f fakeSnapshotter) Fetch(_ context.Context, _ []string, _ []string, _ int) (snapshot.MarketSnapshot, error) {
	if f.snap.Timestamp.IsZero() {
		f.snap.Timestamp = time.UnixMilli(0)
	}
	return f.snap, nil
}

type fakeCompressor struct {
	comp features.CompressionResult
}

func (f fakeCompressor) Compress(_ context.Context, _ snapshot.MarketSnapshot) (features.CompressionResult, []features.FeatureError, error) {
	return f.comp, nil, nil
}

type fakeAgent struct {
	indicator agent.IndicatorSummary
	structure agent.StructureSummary
	mechanics agent.MechanicsSummary
}

func (f fakeAgent) Analyze(_ context.Context, _ string, _ features.CompressionResult, _ decision.AgentEnabled) (agent.IndicatorSummary, agent.StructureSummary, agent.MechanicsSummary, decision.AgentPromptSet, decision.AgentInputSet, error) {
	return f.indicator, f.structure, f.mechanics, decision.AgentPromptSet{}, decision.AgentInputSet{}, nil
}

type fakeProvider struct {
	indicator           provider.IndicatorProviderOut
	structure           provider.StructureProviderOut
	mechanics           provider.MechanicsProviderOut
	inPositionIndicator *provider.InPositionIndicatorOut
	inPositionStructure *provider.InPositionStructureOut
	inPositionMechanics *provider.InPositionMechanicsOut
}

func (f fakeProvider) Judge(_ context.Context, _ string, _ agent.IndicatorSummary, _ agent.StructureSummary, _ agent.MechanicsSummary, _ decision.AgentEnabled, _ decision.ProviderDataContext) (provider.IndicatorProviderOut, provider.StructureProviderOut, provider.MechanicsProviderOut, decision.ProviderPromptSet, error) {
	return f.indicator, f.structure, f.mechanics, decision.ProviderPromptSet{}, nil
}

func (f fakeProvider) JudgeInPosition(_ context.Context, _ string, _ agent.IndicatorSummary, _ agent.StructureSummary, _ agent.MechanicsSummary, _ positionprompt.Summary, _ decision.AgentEnabled, _ decision.ProviderDataContext) (provider.InPositionIndicatorOut, provider.InPositionStructureOut, provider.InPositionMechanicsOut, decision.ProviderPromptSet, error) {
	ind := provider.InPositionIndicatorOut{MomentumSustaining: true, DivergenceDetected: false, Reason: "ok"}
	st := provider.InPositionStructureOut{Integrity: true, ThreatLevel: provider.ThreatLevelNone, Reason: "ok"}
	mech := provider.InPositionMechanicsOut{AdverseLiquidation: false, CrowdingReversal: false, Reason: "ok"}
	if f.inPositionIndicator != nil {
		ind = *f.inPositionIndicator
	}
	if f.inPositionStructure != nil {
		st = *f.inPositionStructure
	}
	if f.inPositionMechanics != nil {
		mech = *f.inPositionMechanics
	}
	return ind, st, mech, decision.ProviderPromptSet{}, nil
}

type fakeExecutor struct {
	name       string
	nowMillis  int64
	nextID     int
	placeCalls int
	positions  map[string]execution.ExternalPosition
	bySymbol   map[string]string
	mu         sync.Mutex
}

func newFakeExecutor(name string, startID int, nowMillis int64) *fakeExecutor {
	return &fakeExecutor{
		name:      name,
		nowMillis: nowMillis,
		nextID:    startID,
		positions: make(map[string]execution.ExternalPosition),
		bySymbol:  make(map[string]string),
	}
}

func (f *fakeExecutor) Name() string {
	return f.name
}

func (f *fakeExecutor) GetOpenPositions(_ context.Context, _ string) ([]execution.ExternalPosition, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]execution.ExternalPosition, 0, len(f.positions))
	for _, pos := range f.positions {
		if pos.Quantity > 0 {
			out = append(out, pos)
		}
	}
	return out, nil
}

func (f *fakeExecutor) GetOpenOrders(_ context.Context, _ string) ([]execution.ExternalOrder, error) {
	return nil, nil
}

func (f *fakeExecutor) GetOrder(_ context.Context, _ string) (execution.ExternalOrder, error) {
	return execution.ExternalOrder{}, nil
}

func (f *fakeExecutor) GetRecentFills(_ context.Context, _ string, _ int64) ([]execution.ExternalFill, error) {
	return nil, nil
}

func (f *fakeExecutor) PlaceOrder(_ context.Context, req execution.PlaceOrderReq) (execution.PlaceOrderResp, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.placeCalls++
	id := strconv.Itoa(f.nextID)
	f.nextID++
	switch req.Kind {
	case execution.OrderOpen:
		pos := execution.ExternalPosition{
			PositionID:   id,
			Symbol:       req.Symbol,
			Side:         req.Side,
			Quantity:     req.Quantity,
			AvgEntry:     req.Price,
			Status:       "open",
			CurrentPrice: req.Price,
			UpdatedAt:    f.nowMillis,
		}
		f.positions[id] = pos
		f.bySymbol[strings.ToUpper(strings.TrimSpace(req.Symbol))] = id
		return execution.PlaceOrderResp{ExternalID: id, Status: "submitted"}, nil
	case execution.OrderClose, execution.OrderReduce:
		if pos, ok := f.positions[req.PositionID]; ok {
			newQty := pos.Quantity - req.Quantity
			if req.Kind == execution.OrderClose || newQty <= 0 {
				delete(f.positions, req.PositionID)
				delete(f.bySymbol, strings.ToUpper(strings.TrimSpace(pos.Symbol)))
			} else {
				pos.Quantity = newQty
				pos.UpdatedAt = f.nowMillis
				f.positions[req.PositionID] = pos
			}
		}
		return execution.PlaceOrderResp{ExternalID: id, Status: "submitted"}, nil
	default:
		return execution.PlaceOrderResp{}, fmt.Errorf("unknown order kind")
	}
}

func (f *fakeExecutor) CancelOrder(_ context.Context, _ execution.CancelOrderReq) (execution.CancelOrderResp, error) {
	return execution.CancelOrderResp{Status: "canceled"}, nil
}

func (f *fakeExecutor) Ping(_ context.Context) error {
	return nil
}

func (f *fakeExecutor) NowMillis() int64 {
	if f.nowMillis > 0 {
		return f.nowMillis
	}
	return time.Now().UnixMilli()
}

func (f *fakeExecutor) PlaceCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.placeCalls
}

func (f *fakeExecutor) AddExternalPosition(pos execution.ExternalPosition) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.positions[pos.PositionID] = pos
	f.bySymbol[strings.ToUpper(strings.TrimSpace(pos.Symbol))] = pos.PositionID
}

type fakePriceSource struct {
	prices map[string]float64
}

func newFakePriceSource() *fakePriceSource {
	return &fakePriceSource{prices: make(map[string]float64)}
}

func (f *fakePriceSource) Set(symbol string, price float64) {
	if f.prices == nil {
		f.prices = make(map[string]float64)
	}
	key := strings.ToUpper(strings.TrimSpace(symbol))
	f.prices[key] = price
}

func (f *fakePriceSource) MarkPrice(_ context.Context, symbol string) (market.PriceQuote, error) {
	key := strings.ToUpper(strings.TrimSpace(symbol))
	price, ok := f.prices[key]
	if !ok {
		return market.PriceQuote{}, market.ErrPriceUnavailable
	}
	return market.PriceQuote{
		Symbol:    symbol,
		Price:     price,
		Timestamp: time.Now().UnixMilli(),
		Source:    "mock",
	}, nil
}

type pipelineConfig struct {
	symbol             string
	positionID         string
	nowMillis          int64
	startID            int
	bind               strategy.StrategyBinding
	providers          fund.ProviderBundle
	structureRegime    agent.Regime
	structureLastBreak agent.LastBreak
	gateDecision       string
}

type pipelineEnv struct {
	pipeline    *decision.Pipeline
	executor    *fakeExecutor
	store       store.Store
	positioner  *position.PositionService
	reconcile   *reconcile.ReconcileService
	priceSource *fakePriceSource
}

var testPositionCache *position.PositionCache

type fakeRuleflow struct {
	gateDecision string
	positionID   string
	symbol       string
}

func (f fakeRuleflow) Evaluate(_ context.Context, _ string, input ruleflow.Input) (ruleflow.Result, error) {
	positionID := f.positionID
	if positionID == "" {
		positionID = "pos-test"
	}
	gateReason := f.gateDecision
	if gateReason == "" {
		gateReason = "PASS_STRONG"
	}
	tradeable := gateReason == "PASS_STRONG" || gateReason == "PASS_WEAK" || gateReason == "ALL_CLEAR_LONG"
	switch gateReason {
	case "MECH_RISK", "STRUCT_INVALID":
		tradeable = false
	}
	gate := fund.GateDecision{
		DecisionAction:  "ALLOW",
		GateReason:      gateReason,
		Direction:       "long",
		Grade:           1,
		GlobalTradeable: tradeable,
	}
	if input.State == fsm.StateInPosition && input.InPosition.Ready && gateReason == "PASS_STRONG" {
		indicatorTag := strings.ToLower(strings.TrimSpace(input.InPosition.Indicator.MonitorTag))
		structureTag := strings.ToLower(strings.TrimSpace(input.InPosition.Structure.MonitorTag))
		mechanicsTag := strings.ToLower(strings.TrimSpace(input.InPosition.Mechanics.MonitorTag))
		exitHit := indicatorTag == "exit" || structureTag == "exit" || mechanicsTag == "exit"
		tightenHit := indicatorTag == "tighten" || structureTag == "tighten" || mechanicsTag == "tighten"
		switch {
		case exitHit:
			gate.DecisionAction = "EXIT"
			gate.GateReason = "REVERSAL_CONFIRMED"
			gate.GlobalTradeable = false
		case tightenHit:
			gate.DecisionAction = "TIGHTEN"
			gate.GateReason = "TIGHTEN"
			gate.GlobalTradeable = false
		default:
			gate.DecisionAction = "KEEP"
			gate.GateReason = "KEEP"
			gate.GlobalTradeable = false
		}
	}
	switch gateReason {
	case "NO_TIMING":
		gate.DecisionAction = "WAIT"
	case "MECH_RISK", "STRUCT_INVALID":
		gate.DecisionAction = "VETO"
		gate.GlobalTradeable = false
	}
	if gateReason == "ALL_CLEAR_LONG" {
		gate.Direction = "long"
		gate.GlobalTradeable = true
		gate.Grade = 2
		tradeable = true
	}
	entry := 10000.0
	stopLoss := 9900.0
	stopSource := "atr_multiple"
	atr := 100.0
	if strings.HasPrefix(strings.ToUpper(f.symbol), "ETH/") {
		entry = 2000
		stopLoss = 2100
		atr = 50
	}
	if strings.HasPrefix(strings.ToUpper(f.symbol), "SOL/") {
		entry = 20
		stopLoss = 10
		atr = 1
	}
	direction := "long"
	if strings.HasPrefix(strings.ToUpper(f.symbol), "ETH/") {
		direction = "short"
	}
	tpLevel := entry + 205
	if direction == "short" {
		tpLevel = entry - 205
	}
	plan := &execution.ExecutionPlan{
		Symbol:       f.symbol,
		Valid:        true,
		Direction:    direction,
		Entry:        entry,
		StopLoss:     stopLoss,
		RiskPct:      1,
		PositionSize: 1,
		Leverage:     1,
		RMultiple:    1,
		Template:     "rulego_plan",
		PositionID:   positionID,
		CreatedAt:    time.Now().UTC(),
		ExpiresAt:    time.Now().UTC().Add(24 * time.Hour),
		TakeProfits:  []float64{tpLevel},
		TakeProfitRatios: []float64{1},
		RiskAnnotations: execution.RiskAnnotations{
			StopSource:   stopSource,
			StopReason:   "atr_multiple",
			RiskDistance: math.Abs(entry - stopLoss),
			ATR:          atr,
		},
	}
	if strings.HasPrefix(strings.ToUpper(f.symbol), "SOL/") {
		plan.PositionSize = 0.5
		plan.ExpiresAt = time.Now().UTC().Add(24 * time.Hour)
	}
	var planPtr *execution.ExecutionPlan
	if tradeable {
		planPtr = plan
	}
	fsmNext := fsm.StateFlat
	if tradeable {
		fsmNext = fsm.StateInPosition
	}
	return ruleflow.Result{Gate: gate, Plan: planPtr, FSMNext: fsmNext, FSMActions: []fsm.Action{{Type: fsm.ActionOpen, Reason: "test"}}}, nil

}

func newPipelineEnv(t *testing.T, cfg pipelineConfig) pipelineEnv {
	t.Helper()
	t.Skip("requires PostgreSQL")
	return pipelineEnv{}
}

func newTestStore(t *testing.T) store.Store {
	t.Helper()
	t.Skip("requires PostgreSQL")
	return nil
}

func defaultBinding(symbol string) strategy.StrategyBinding {
	return strategy.StrategyBinding{
		Symbol:         symbol,
		StrategyID:     "strategy-1",
		SystemHash:     "sys-1",
		StrategyHash:   "strat-1",
		RiskManagement: defaultRiskManagementConfig(),
	}
}

func defaultRiskManagementConfig() config.RiskManagementConfig {
	return config.RiskManagementConfig{
		RiskPerTradePct:   0.01,
		MaxInvestPct:      1.0,
		MaxLeverage:       3.0,
		Grade1Factor:      0.3,
		Grade2Factor:      0.6,
		Grade3Factor:      1.0,
		EntryOffsetATR:    0.1,
		BreakevenFeePct:   0.0,
		SlippageBufferPct: 0.0,
		InitialExit: config.InitialExitConfig{
			Policy:            "atr_structure_v1",
			StructureInterval: "auto",
			Params: map[string]any{
				"stop_atr_multiplier":   2.0,
				"stop_min_distance_pct": 0.005,
				"take_profit_rr":        []float64{1.5, 3.0},
			},
		},
		TightenATR: config.TightenATRConfig{
			StructureThreatened:  0.5,
			TP1ATR:               0.8,
			TP2ATR:               1.2,
			MinTPDistancePct:     0.006,
			MinTPGapPct:          0.005,
			MinUpdateIntervalSec: 0,
		},
	}
}

func runPipelineOnce(t *testing.T, env pipelineEnv, symbol string, equity, riskPct float64) decision.PersistResult {
	t.Helper()
	results, err := env.pipeline.RunOnce(context.Background(), []string{symbol}, []string{"1h"}, 10, execution.AccountState{Equity: equity}, execution.RiskParams{RiskPerTradePct: riskPct})
	if err != nil {
		t.Fatalf("pipeline run: %v", err)
	}
	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("unexpected pipeline result: %+v", results)
	}
	return results[0]
}

func loadPosition(t *testing.T, s store.Store, positionID string) store.PositionRecord {
	t.Helper()
	rec, ok, err := s.FindPositionByID(context.Background(), positionID)
	if err != nil || !ok {
		t.Fatalf("find position: %v ok=%v", err, ok)
	}
	return rec
}
