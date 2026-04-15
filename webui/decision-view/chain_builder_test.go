package decisionview

import (
	"context"
	"encoding/json"
	"testing"

	"brale-core/internal/store"
)

func TestBuildChainAddsLLMRiskNodeForEntryLLMPlan(t *testing.T) {
	t.Parallel()

	symbol := "STOUSDT"
	snapshotID := uint(1775232010)
	chain := buildChain(context.Background(), symbol, symbolEvents{
		gates: []store.GateEventRecord{
			{
				ID:               23,
				SnapshotID:       snapshotID,
				Symbol:           symbol,
				Timestamp:        int64(snapshotID),
				GlobalTradeable:  true,
				DecisionAction:   "ALLOW",
				GateReason:       "ALLOW",
				ProviderRefsJSON: json.RawMessage([]byte("{}")),
				DerivedJSON: mustJSON(t, map[string]any{"plan": map[string]any{
					"plan_source": "llm",
					"llm_trace": map[string]any{
						"stage":         "risk_flat_init",
						"flow":          "flat",
						"system_prompt": "risk-system",
						"user_prompt":   "risk-user",
						"raw_output":    "{\"stop_loss\":0.1}",
					},
				}}),
			},
		},
	}, store.PositionRecord{}, false, 10)

	llmNode, ok := findRoundNode(chain, snapshotID, "llm_risk")
	if !ok {
		t.Fatalf("expected llm_risk node for entry llm plan")
	}
	if got, want := llmNode.ID, "STOUSDT-1775232010-llm-risk"; got != want {
		t.Fatalf("unexpected llm_risk node id: got %q want %q", got, want)
	}
	if len(llmNode.Refs) != 1 || llmNode.Refs[0] != "STOUSDT-1775232010-gate-23" {
		t.Fatalf("expected llm_risk node to reference gate, got %#v", llmNode.Refs)
	}
	output, ok := llmNode.Output.(map[string]any)
	if !ok {
		t.Fatalf("expected llm_risk output map, got %#v", llmNode.Output)
	}
	trace, ok := output["trace"].(map[string]any)
	if !ok || trace["system_prompt"] != "risk-system" {
		t.Fatalf("expected llm trace payload, got %#v", output["trace"])
	}
}

func TestBuildChainSkipsLLMRiskNodeWithoutActualLLMTrace(t *testing.T) {
	t.Parallel()

	symbol := "STOUSDT"
	snapshotID := uint(1775233810)
	chain := buildChain(context.Background(), symbol, symbolEvents{
		gates: []store.GateEventRecord{
			{
				ID:               25,
				SnapshotID:       snapshotID,
				Symbol:           symbol,
				Timestamp:        int64(snapshotID),
				DecisionAction:   "TIGHTEN",
				GateReason:       "CRITICAL_TIGHTEN_ATTEMPT",
				ProviderRefsJSON: json.RawMessage([]byte("{}")),
				DerivedJSON: mustJSON(t, map[string]any{
					"execution": map[string]any{
						"action":      "tighten",
						"executed":    false,
						"plan_source": "llm",
					},
				}),
			},
		},
	}, store.PositionRecord{}, false, 10)

	if _, ok := findRoundNode(chain, snapshotID, "llm_risk"); ok {
		t.Fatalf("did not expect llm_risk node when actual llm trace is missing")
	}
}

func TestBuildChainSkipsLLMRiskNodeWhenGateBlocked(t *testing.T) {
	t.Parallel()

	symbol := "STOUSDT"
	snapshotID := uint(1775234010)
	chain := buildChain(context.Background(), symbol, symbolEvents{
		gates: []store.GateEventRecord{
			{
				ID:               26,
				SnapshotID:       snapshotID,
				Symbol:           symbol,
				Timestamp:        int64(snapshotID),
				GlobalTradeable:  false,
				DecisionAction:   "BLOCK",
				GateReason:       "MECHANICS_BLOCKED",
				ProviderRefsJSON: json.RawMessage([]byte("{}")),
				DerivedJSON: mustJSON(t, map[string]any{"plan": map[string]any{
					"plan_source": "llm",
					"llm_trace": map[string]any{
						"stage":         "risk_flat_init",
						"flow":          "flat",
						"system_prompt": "risk-system",
						"user_prompt":   "risk-user",
						"raw_output":    "{\"stop_loss\":0.1}",
					},
				}}),
			},
		},
	}, store.PositionRecord{}, false, 10)

	if _, ok := findRoundNode(chain, snapshotID, "llm_risk"); ok {
		t.Fatalf("did not expect llm_risk node when gate is blocked")
	}
}

func findRoundNode(chain SymbolChain, snapshotID uint, stage string) (Node, bool) {
	for _, round := range chain.Rounds {
		if round.SnapshotID != snapshotID {
			continue
		}
		for _, node := range round.Nodes {
			if node.Stage == stage {
				return node, true
			}
		}
	}
	return Node{}, false
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return json.RawMessage(raw)
}
