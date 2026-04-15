package runtimeapi

import (
	"context"
	"encoding/json"
	"testing"

	"brale-core/internal/store"
)

type stubDashboardSnapshotStore struct {
	gate            store.GateEventRecord
	gateFound       bool
	gateErr         error
	providers       []store.ProviderEventRecord
	agents          []store.AgentEventRecord
	position        store.PositionRecord
	positionFound   bool
	positionErr     error
	listGateCalls   int
	findGateCalls   int
	providerListErr error
	agentListErr    error
}

func (s *stubDashboardSnapshotStore) ListGateEvents(context.Context, string, int) ([]store.GateEventRecord, error) {
	s.listGateCalls++
	return nil, nil
}

func (s *stubDashboardSnapshotStore) FindGateEventBySnapshot(context.Context, string, uint) (store.GateEventRecord, bool, error) {
	s.findGateCalls++
	if s.gateErr != nil {
		return store.GateEventRecord{}, false, s.gateErr
	}
	return s.gate, s.gateFound, nil
}

func (s *stubDashboardSnapshotStore) ListProviderEventsBySnapshot(context.Context, string, uint) ([]store.ProviderEventRecord, error) {
	if s.providerListErr != nil {
		return nil, s.providerListErr
	}
	return s.providers, nil
}

func (s *stubDashboardSnapshotStore) ListProviderEventsByTimeRange(context.Context, string, int64, int64) ([]store.ProviderEventRecord, error) {
	return nil, nil
}

func (s *stubDashboardSnapshotStore) ListAgentEventsBySnapshot(context.Context, string, uint) ([]store.AgentEventRecord, error) {
	if s.agentListErr != nil {
		return nil, s.agentListErr
	}
	return s.agents, nil
}

func (s *stubDashboardSnapshotStore) ListAgentEventsByTimeRange(context.Context, string, int64, int64) ([]store.AgentEventRecord, error) {
	return nil, nil
}

func (s *stubDashboardSnapshotStore) ListProviderEvents(context.Context, string, int) ([]store.ProviderEventRecord, error) {
	return nil, nil
}

func (s *stubDashboardSnapshotStore) ListAgentEvents(context.Context, string, int) ([]store.AgentEventRecord, error) {
	return nil, nil
}

func (s *stubDashboardSnapshotStore) ListGateEventsByTimeRange(context.Context, string, int64, int64) ([]store.GateEventRecord, error) {
	return nil, nil
}

func (s *stubDashboardSnapshotStore) ListDistinctSnapshotIDs(context.Context, string, int64, int64) ([]uint, error) {
	return nil, nil
}

func (s *stubDashboardSnapshotStore) FindPositionBySymbol(context.Context, string, []string) (store.PositionRecord, bool, error) {
	if s.positionErr != nil {
		return store.PositionRecord{}, false, s.positionErr
	}
	return s.position, s.positionFound, nil
}

func (s *stubDashboardSnapshotStore) FindPositionByID(context.Context, string) (store.PositionRecord, bool, error) {
	return store.PositionRecord{}, false, nil
}

func (s *stubDashboardSnapshotStore) ListPositionsByStatus(context.Context, []string) ([]store.PositionRecord, error) {
	return nil, nil
}

func (s *stubDashboardSnapshotStore) ListRiskPlanHistory(context.Context, string, int) ([]store.RiskPlanHistoryRecord, error) {
	return nil, nil
}

func (s *stubDashboardSnapshotStore) FindLatestRiskPlanHistory(context.Context, string) (store.RiskPlanHistoryRecord, bool, error) {
	return store.RiskPlanHistoryRecord{}, false, nil
}

func TestBuildDecisionDetailUsesDirectGateLookup(t *testing.T) {
	st := &stubDashboardSnapshotStore{
		gateFound: true,
		gate: store.GateEventRecord{
			ID:              7,
			SnapshotID:      9001,
			Symbol:          "BTCUSDT",
			DecisionAction:  "ALLOW",
			GlobalTradeable: true,
			DerivedJSON:     json.RawMessage([]byte(`{"direction_consensus":{"score":1}}`)),
		},
	}
	detail, useErr := buildDecisionDetail(context.Background(), st, nil, "BTCUSDT", 9001)
	if useErr != nil {
		t.Fatalf("buildDecisionDetail error: %v", useErr)
	}
	if detail == nil || detail.SnapshotID != 9001 {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	if st.findGateCalls != 1 {
		t.Fatalf("findGateCalls=%d want=1", st.findGateCalls)
	}
	if st.listGateCalls != 0 {
		t.Fatalf("listGateCalls=%d want=0", st.listGateCalls)
	}
}

func TestDashboardFlowUsesDirectGateLookupForSelectedSnapshot(t *testing.T) {
	st := &stubDashboardSnapshotStore{
		gateFound: true,
		gate: store.GateEventRecord{
			SnapshotID:      7001,
			Symbol:          "BTCUSDT",
			DecisionAction:  "ALLOW",
			GlobalTradeable: true,
		},
	}
	u := dashboardFlowUsecase{store: st, allowSymbol: func(string) bool { return true }}
	resp, useErr := u.build(context.Background(), "BTCUSDT", "7001")
	if useErr != nil {
		t.Fatalf("build error: %v", useErr)
	}
	if resp.Flow.Anchor.SnapshotID != 7001 {
		t.Fatalf("anchor=%+v", resp.Flow.Anchor)
	}
	if st.findGateCalls != 1 {
		t.Fatalf("findGateCalls=%d want=1", st.findGateCalls)
	}
	if st.listGateCalls != 0 {
		t.Fatalf("listGateCalls=%d want=0", st.listGateCalls)
	}
}
