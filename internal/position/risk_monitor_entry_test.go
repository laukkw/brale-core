package position

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"brale-core/internal/execution"
	"brale-core/internal/market"
	"brale-core/internal/store"
)

func TestRiskMonitorStopBeyondLiquidationLong(t *testing.T) {
	planCache := NewPlanCache()
	plan := execution.ExecutionPlan{
		Symbol:    "BTCUSDT",
		Direction: "long",
		Entry:     100,
		StopLoss:  120,
		RiskPct:   0.2,
		Leverage:  10,
		RiskAnnotations: execution.RiskAnnotations{
			RiskDistance: 1,
			MaxInvestPct: 0.1,
		},
	}
	planCache.ForceUpsert(plan.Symbol, plan)
	monitor := baseRiskMonitor(planCache, 99)

	if err := monitor.handlePlanEntry(context.Background(), plan.Symbol); err == nil || !strings.Contains(err.Error(), "stop loss beyond liquidation") {
		t.Fatalf("expected stop loss beyond liquidation error, got %v", err)
	}
}

func TestRiskMonitorStopBeyondLiquidationShort(t *testing.T) {
	planCache := NewPlanCache()
	plan := execution.ExecutionPlan{
		Symbol:    "BTCUSDT",
		Direction: "short",
		Entry:     100,
		StopLoss:  80,
		RiskPct:   0.2,
		Leverage:  10,
		RiskAnnotations: execution.RiskAnnotations{
			RiskDistance: 1,
			MaxInvestPct: 0.1,
		},
	}
	planCache.ForceUpsert(plan.Symbol, plan)
	monitor := baseRiskMonitor(planCache, 101)

	if err := monitor.handlePlanEntry(context.Background(), plan.Symbol); err == nil || !strings.Contains(err.Error(), "stop loss beyond liquidation") {
		t.Fatalf("expected stop loss beyond liquidation error, got %v", err)
	}
}

func TestRiskMonitorLeverageRoundingAndQtyReduction(t *testing.T) {
	planCache := NewPlanCache()
	plan := execution.ExecutionPlan{
		Symbol:    "BTCUSDT",
		Direction: "long",
		Entry:     100,
		StopLoss:  90,
		RiskPct:   0.1,
		Leverage:  5.7,
		RiskAnnotations: execution.RiskAnnotations{
			RiskDistance: 1,
			MaxInvestPct: 0.2,
			MaxLeverage:  5.7,
		},
	}
	planCache.ForceUpsert(plan.Symbol, plan)
	monitor := baseRiskMonitor(planCache, 99)

	if err := monitor.handlePlanEntry(context.Background(), plan.Symbol); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	entry, ok := planCache.GetEntry(plan.Symbol)
	if !ok || entry == nil {
		t.Fatalf("expected plan cache entry")
	}
	updated := entry.Plan
	if math.Abs(updated.Leverage-5) > 1e-9 {
		t.Fatalf("expected leverage 5, got %v", updated.Leverage)
	}
	maxInvestAmt := 1000.0 * 0.2
	expectedSize := (updated.Leverage * maxInvestAmt) / updated.Entry
	if math.Abs(updated.PositionSize-expectedSize) > 1e-9 {
		t.Fatalf("expected position_size %v, got %v", expectedSize, updated.PositionSize)
	}
	initialSize := (1000.0 * 0.1) / 1.0
	if updated.PositionSize >= initialSize {
		t.Fatalf("expected position_size reduced from %v, got %v", initialSize, updated.PositionSize)
	}
}

func baseRiskMonitor(planCache *PlanCache, markPrice float64) *RiskMonitor {
	return &RiskMonitor{
		PriceSource: stubPriceSource{price: markPrice},
		Positions: &PositionService{
			Store:     &stubStore{},
			Executor:  &stubExecutor{},
			PlanCache: planCache,
		},
		PlanCache: planCache,
		AccountFetcher: func(ctx context.Context, symbol string) (execution.AccountState, error) {
			return execution.AccountState{Available: 1000, Currency: "USDT"}, nil
		},
	}
}

type stubPriceSource struct {
	price float64
}

func (s stubPriceSource) MarkPrice(ctx context.Context, symbol string) (market.PriceQuote, error) {
	return market.PriceQuote{Symbol: symbol, Price: s.price}, nil
}

type stubExecutor struct{}

func (s *stubExecutor) Name() string { return "stub" }
func (s *stubExecutor) GetOpenPositions(ctx context.Context, symbol string) ([]execution.ExternalPosition, error) {
	return nil, nil
}
func (s *stubExecutor) GetOpenOrders(ctx context.Context, symbol string) ([]execution.ExternalOrder, error) {
	return nil, nil
}
func (s *stubExecutor) GetOrder(ctx context.Context, id string) (execution.ExternalOrder, error) {
	return execution.ExternalOrder{}, nil
}
func (s *stubExecutor) GetRecentFills(ctx context.Context, symbol string, sinceTS int64) ([]execution.ExternalFill, error) {
	return nil, nil
}
func (s *stubExecutor) PlaceOrder(ctx context.Context, req execution.PlaceOrderReq) (execution.PlaceOrderResp, error) {
	return execution.PlaceOrderResp{ExternalID: "stub", Status: "submitted"}, nil
}
func (s *stubExecutor) CancelOrder(ctx context.Context, req execution.CancelOrderReq) (execution.CancelOrderResp, error) {
	return execution.CancelOrderResp{Status: "ok"}, nil
}
func (s *stubExecutor) Ping(ctx context.Context) error { return nil }
func (s *stubExecutor) NowMillis() int64               { return 0 }

type stubStore struct{}

func (s *stubStore) SaveAgentEvent(ctx context.Context, rec *store.AgentEventRecord) error {
	return nil
}
func (s *stubStore) SaveProviderEvent(ctx context.Context, rec *store.ProviderEventRecord) error {
	return nil
}
func (s *stubStore) SaveGateEvent(ctx context.Context, rec *store.GateEventRecord) error {
	return nil
}
func (s *stubStore) SaveRiskPlanHistory(ctx context.Context, rec *store.RiskPlanHistoryRecord) error {
	return nil
}
func (s *stubStore) FindLatestRiskPlanHistory(ctx context.Context, positionID string) (store.RiskPlanHistoryRecord, bool, error) {
	return store.RiskPlanHistoryRecord{}, false, nil
}
func (s *stubStore) ListRiskPlanHistory(ctx context.Context, positionID string, limit int) ([]store.RiskPlanHistoryRecord, error) {
	return nil, nil
}
func (s *stubStore) SavePosition(ctx context.Context, rec *store.PositionRecord) error { return nil }
func (s *stubStore) UpdatePosition(ctx context.Context, positionID string, expectedVersion int, updates map[string]any) (bool, error) {
	return false, nil
}
func (s *stubStore) UpdatePositionPatch(ctx context.Context, patch store.PositionPatch) (bool, error) {
	return false, nil
}
func (s *stubStore) FindPositionByID(ctx context.Context, positionID string) (store.PositionRecord, bool, error) {
	return store.PositionRecord{}, false, nil
}
func (s *stubStore) FindPositionBySymbol(ctx context.Context, symbol string, statuses []string) (store.PositionRecord, bool, error) {
	return store.PositionRecord{}, false, nil
}
func (s *stubStore) ListPositionsByStatus(ctx context.Context, statuses []string) ([]store.PositionRecord, error) {
	return nil, nil
}
func (s *stubStore) ListSymbols(ctx context.Context) ([]string, error) { return nil, nil }
func (s *stubStore) ListProviderEvents(ctx context.Context, symbol string, limit int) ([]store.ProviderEventRecord, error) {
	return nil, nil
}
func (s *stubStore) ListProviderEventsBySnapshot(ctx context.Context, symbol string, snapshotID uint) ([]store.ProviderEventRecord, error) {
	return nil, nil
}
func (s *stubStore) ListProviderEventsByTimeRange(ctx context.Context, symbol string, start, end int64) ([]store.ProviderEventRecord, error) {
	return nil, nil
}
func (s *stubStore) ListAgentEvents(ctx context.Context, symbol string, limit int) ([]store.AgentEventRecord, error) {
	return nil, nil
}
func (s *stubStore) ListAgentEventsBySnapshot(ctx context.Context, symbol string, snapshotID uint) ([]store.AgentEventRecord, error) {
	return nil, nil
}
func (s *stubStore) ListAgentEventsByTimeRange(ctx context.Context, symbol string, start, end int64) ([]store.AgentEventRecord, error) {
	return nil, nil
}
func (s *stubStore) ListGateEvents(ctx context.Context, symbol string, limit int) ([]store.GateEventRecord, error) {
	return nil, nil
}
func (s *stubStore) ListGateEventsByTimeRange(ctx context.Context, symbol string, start, end int64) ([]store.GateEventRecord, error) {
	return nil, nil
}
func (s *stubStore) FindGateEventBySnapshot(ctx context.Context, symbol string, snapshotID uint) (store.GateEventRecord, bool, error) {
	return store.GateEventRecord{}, false, nil
}
func (s *stubStore) ListDistinctSnapshotIDs(ctx context.Context, symbol string, start, end int64) ([]uint, error) {
	return nil, nil
}
func (s *stubStore) SaveEpisodicMemory(ctx context.Context, rec *store.EpisodicMemoryRecord) error {
	return nil
}
func (s *stubStore) ListEpisodicMemories(ctx context.Context, symbol string, limit int) ([]store.EpisodicMemoryRecord, error) {
	return nil, nil
}
func (s *stubStore) FindEpisodicMemoryByPosition(ctx context.Context, positionID string) (store.EpisodicMemoryRecord, bool, error) {
	return store.EpisodicMemoryRecord{}, false, nil
}
func (s *stubStore) DeleteEpisodicMemoriesOlderThan(ctx context.Context, symbol string, before time.Time) (int64, error) {
	return 0, nil
}
func (s *stubStore) SaveSemanticMemory(ctx context.Context, rec *store.SemanticMemoryRecord) error {
	return nil
}
func (s *stubStore) UpdateSemanticMemory(ctx context.Context, id uint, updates map[string]any) error {
	return nil
}
func (s *stubStore) DeleteSemanticMemory(ctx context.Context, id uint) error { return nil }
func (s *stubStore) ListSemanticMemories(ctx context.Context, symbol string, activeOnly bool, limit int) ([]store.SemanticMemoryRecord, error) {
	return nil, nil
}
func (s *stubStore) FindSemanticMemory(ctx context.Context, id uint) (store.SemanticMemoryRecord, bool, error) {
	return store.SemanticMemoryRecord{}, false, nil
}

// LLMRoundStore
func (s *stubStore) SaveLLMRound(ctx context.Context, rec *store.LLMRoundRecord) error { return nil }
func (s *stubStore) FindLLMRound(ctx context.Context, id string) (store.LLMRoundRecord, bool, error) {
	return store.LLMRoundRecord{}, false, nil
}
func (s *stubStore) ListLLMRounds(ctx context.Context, symbol string, limit int) ([]store.LLMRoundRecord, error) {
	return nil, nil
}

// PromptRegistryStore
func (s *stubStore) SavePromptEntry(ctx context.Context, rec *store.PromptRegistryEntry) error {
	return nil
}
func (s *stubStore) FindActivePrompt(ctx context.Context, role, stage string) (store.PromptRegistryEntry, bool, error) {
	return store.PromptRegistryEntry{}, false, nil
}
func (s *stubStore) ListPromptEntries(ctx context.Context, role string, activeOnly bool) ([]store.PromptRegistryEntry, error) {
	return nil, nil
}

var _ store.Store = (*stubStore)(nil)
var _ execution.Executor = (*stubExecutor)(nil)
