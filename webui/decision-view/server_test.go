package decisionview

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"brale-core/internal/config"
	"brale-core/internal/store"
)

type fakeStore struct {
	symbols   []string
	providers map[string][]store.ProviderEventRecord
	agents    map[string][]store.AgentEventRecord
	gates     map[string][]store.GateEventRecord
	positions map[string]store.PositionRecord
}

func (f *fakeStore) SaveAgentEvent(context.Context, *store.AgentEventRecord) error { return nil }

func (f *fakeStore) SaveProviderEvent(context.Context, *store.ProviderEventRecord) error { return nil }

func (f *fakeStore) SaveGateEvent(context.Context, *store.GateEventRecord) error { return nil }

func (f *fakeStore) SaveRiskPlanHistory(context.Context, *store.RiskPlanHistoryRecord) error {
	return nil
}

func (f *fakeStore) FindLatestRiskPlanHistory(context.Context, string) (store.RiskPlanHistoryRecord, bool, error) {
	return store.RiskPlanHistoryRecord{}, false, nil
}

func (f *fakeStore) ListRiskPlanHistory(context.Context, string, int) ([]store.RiskPlanHistoryRecord, error) {
	return nil, nil
}

func (f *fakeStore) SavePosition(context.Context, *store.PositionRecord) error { return nil }

func (f *fakeStore) UpdatePosition(context.Context, string, int, map[string]any) (bool, error) {
	return false, nil
}

func (f *fakeStore) FindPositionByID(context.Context, string) (store.PositionRecord, bool, error) {
	return store.PositionRecord{}, false, nil
}

func (f *fakeStore) FindPositionBySymbol(_ context.Context, symbol string, _ []string) (store.PositionRecord, bool, error) {
	if f.positions == nil {
		return store.PositionRecord{}, false, nil
	}
	pos, ok := f.positions[symbol]
	if !ok || pos.PositionID == "" {
		return store.PositionRecord{}, false, nil
	}
	return pos, true, nil
}

func (f *fakeStore) ListPositionsByStatus(context.Context, []string) ([]store.PositionRecord, error) {
	return nil, nil
}

func (f *fakeStore) ListSymbols(context.Context) ([]string, error) {
	return append([]string{}, f.symbols...), nil
}

func (f *fakeStore) ListProviderEvents(_ context.Context, symbol string, _ int) ([]store.ProviderEventRecord, error) {
	return append([]store.ProviderEventRecord{}, f.providers[symbol]...), nil
}

func (f *fakeStore) ListProviderEventsBySnapshot(context.Context, string, uint) ([]store.ProviderEventRecord, error) {
	return nil, nil
}

func (f *fakeStore) ListProviderEventsByTimeRange(context.Context, string, int64, int64) ([]store.ProviderEventRecord, error) {
	return nil, nil
}

func (f *fakeStore) ListAgentEvents(_ context.Context, symbol string, _ int) ([]store.AgentEventRecord, error) {
	return append([]store.AgentEventRecord{}, f.agents[symbol]...), nil
}

func (f *fakeStore) ListAgentEventsBySnapshot(context.Context, string, uint) ([]store.AgentEventRecord, error) {
	return nil, nil
}

func (f *fakeStore) ListAgentEventsByTimeRange(context.Context, string, int64, int64) ([]store.AgentEventRecord, error) {
	return nil, nil
}

func (f *fakeStore) ListGateEvents(_ context.Context, symbol string, _ int) ([]store.GateEventRecord, error) {
	return append([]store.GateEventRecord{}, f.gates[symbol]...), nil
}

func (f *fakeStore) ListGateEventsByTimeRange(context.Context, string, int64, int64) ([]store.GateEventRecord, error) {
	return nil, nil
}

func (f *fakeStore) FindGateEventBySnapshot(_ context.Context, symbol string, snapshotID uint) (store.GateEventRecord, bool, error) {
	for _, gate := range f.gates[symbol] {
		if gate.SnapshotID == snapshotID {
			return gate, true, nil
		}
	}
	return store.GateEventRecord{}, false, nil
}

func (f *fakeStore) ListDistinctSnapshotIDs(context.Context, string, int64, int64) ([]uint, error) {
	return nil, nil
}

func TestServerHandlerDecisionViewAPIs(t *testing.T) {
	st := &fakeStore{
		symbols: []string{"BTCUSDT"},
		positions: map[string]store.PositionRecord{
			"BTCUSDT": {
				PositionID: "pos-btc-1",
				Symbol:     "BTCUSDT",
				Status:     "open",
				RiskJSON:   json.RawMessage([]byte(`{"tp":[1,2],"sl":0.5}`)),
			},
		},
		providers: map[string][]store.ProviderEventRecord{
			"BTCUSDT": {
				{
					ID:            11,
					SnapshotID:    1001,
					Symbol:        "BTCUSDT",
					Timestamp:     1_730_000_000,
					Role:          "indicator",
					SystemPrompt:  "sys",
					UserPrompt:    "user",
					OutputJSON:    json.RawMessage([]byte(`{"ok":true}`)),
					Fingerprint:   "fp-prov",
					SourceVersion: "v1",
				},
				{
					ID:            12,
					SnapshotID:    1001,
					Symbol:        "BTCUSDT",
					Timestamp:     1_730_000_030,
					Role:          "mechanics_in_position",
					SystemPrompt:  "risk-sys",
					UserPrompt:    "risk-user",
					OutputJSON:    json.RawMessage([]byte(`{"tp":1.8,"sl":0.6}`)),
					Fingerprint:   "fp-risk",
					SourceVersion: "v1",
				},
			},
		},
		agents: map[string][]store.AgentEventRecord{
			"BTCUSDT": {
				{
					ID:            22,
					SnapshotID:    1001,
					Symbol:        "BTCUSDT",
					Timestamp:     1_730_000_020,
					Stage:         "indicator",
					SystemPrompt:  "sys-a",
					UserPrompt:    "user-a",
					OutputJSON:    json.RawMessage([]byte(`{"score":0.8}`)),
					Fingerprint:   "fp-agent",
					SourceVersion: "v1",
				},
			},
		},
		gates: map[string][]store.GateEventRecord{
			"BTCUSDT": {
				{
					ID:               33,
					SnapshotID:       1001,
					Symbol:           "BTCUSDT",
					Timestamp:        1_730_000_040,
					GlobalTradeable:  true,
					DecisionAction:   "enter",
					Grade:            2,
					GateReason:       "ok",
					Direction:        "long",
					ProviderRefsJSON: json.RawMessage([]byte(`{}`)),
					DerivedJSON:      json.RawMessage([]byte(`{"plan":{"plan_source":"llm","llm_trace":{"stage":"risk_flat_init","system_prompt":"risk-system","user_prompt":"risk-user","raw_output":"{\"stop_loss\":0.6}","parsed_output":{"stop_loss":0.6}}}}`)),
					Fingerprint:      "fp-gate",
					SourceVersion:    "v1",
				},
			},
		},
	}

	srv := Server{
		Store:      st,
		BasePath:   "/decision-view",
		RoundLimit: 10,
		SystemConfig: config.SystemConfig{
			Hash: "sys-hash",
		},
		SymbolConfigs: map[string]ConfigBundle{
			"BTCUSDT": {
				Symbol: config.SymbolConfig{
					Hash:       "sym-hash",
					Symbol:     "BTCUSDT",
					Intervals:  []string{"1h"},
					KlineLimit: 120,
				},
				Strategy: config.StrategyConfig{
					Hash:          "strat-hash",
					ID:            "strat-1",
					RuleChainPath: "rules.yaml",
				},
			},
		},
	}

	h, err := srv.Handler()
	if err != nil {
		t.Fatalf("handler init failed: %v", err)
	}

	t.Run("chains endpoint returns symbol rounds nodes", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/decision-view/api/chains?symbol=BTCUSDT&limit=2", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
		}

		var payload map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		symbols, ok := payload["symbols"].([]any)
		if !ok || len(symbols) != 1 {
			t.Fatalf("symbols invalid: %#v", payload["symbols"])
		}

		sym, ok := symbols[0].(map[string]any)
		if !ok {
			t.Fatalf("symbol item invalid: %#v", symbols[0])
		}
		if sym["id"] != "BTCUSDT" {
			t.Fatalf("unexpected symbol id: %#v", sym["id"])
		}

		rounds, ok := sym["rounds"].([]any)
		if !ok || len(rounds) == 0 {
			t.Fatalf("rounds invalid: %#v", sym["rounds"])
		}

		r0, ok := rounds[0].(map[string]any)
		if !ok {
			t.Fatalf("round invalid: %#v", rounds[0])
		}
		nodes, ok := r0["nodes"].([]any)
		if !ok || len(nodes) == 0 {
			t.Fatalf("nodes invalid: %#v", r0["nodes"])
		}
	})

	t.Run("chains endpoint includes llm risk node with prompts and output", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/decision-view/api/chains?symbol=BTCUSDT&limit=2", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
		}

		var payload map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		symbols, ok := payload["symbols"].([]any)
		if !ok || len(symbols) != 1 {
			t.Fatalf("symbols invalid: %#v", payload["symbols"])
		}
		sym, ok := symbols[0].(map[string]any)
		if !ok {
			t.Fatalf("symbol item invalid: %#v", symbols[0])
		}
		rounds, ok := sym["rounds"].([]any)
		if !ok || len(rounds) == 0 {
			t.Fatalf("rounds invalid: %#v", sym["rounds"])
		}
		r0, ok := rounds[0].(map[string]any)
		if !ok {
			t.Fatalf("round invalid: %#v", rounds[0])
		}
		nodes, ok := r0["nodes"].([]any)
		if !ok || len(nodes) == 0 {
			t.Fatalf("nodes invalid: %#v", r0["nodes"])
		}

		var found map[string]any
		for _, raw := range nodes {
			node, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if node["stage"] == "llm_risk" {
				found = node
				break
			}
		}
		if found == nil {
			t.Fatalf("llm_risk node not found: %#v", nodes)
		}

		output, ok := found["output"].(map[string]any)
		if !ok {
			t.Fatalf("llm_risk output invalid: %#v", found["output"])
		}
		trace, ok := output["trace"].(map[string]any)
		if !ok {
			t.Fatalf("llm_risk trace invalid: %#v", output["trace"])
		}
		if trace["system_prompt"] == nil || trace["user_prompt"] == nil || trace["raw_output"] == nil {
			t.Fatalf("llm_risk trace fields missing: %#v", trace)
		}
	})

	t.Run("config graph endpoint returns symbol config", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/decision-view/api/config-graph", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
		}

		var payload map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		symbols, ok := payload["symbols"].([]any)
		if !ok || len(symbols) == 0 {
			t.Fatalf("symbols invalid: %#v", payload["symbols"])
		}

		first, ok := symbols[0].(map[string]any)
		if !ok {
			t.Fatalf("symbol entry invalid: %#v", symbols[0])
		}
		if first["id"] != "BTCUSDT" {
			t.Fatalf("unexpected id: %#v", first["id"])
		}

		cfg, ok := first["config"].(map[string]any)
		if !ok {
			t.Fatalf("config invalid: %#v", first["config"])
		}
		if cfg["kline_limit"] == nil {
			t.Fatalf("kline_limit missing: %#v", cfg)
		}
	})
}

func TestServerHandlerRequiresStore(t *testing.T) {
	_, err := (Server{}).Handler()
	if err == nil {
		t.Fatal("expected error when store is nil")
	}
}
