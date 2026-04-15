package backtest

import (
	"encoding/json"
	"testing"

	"brale-core/internal/decision/agent"
	"brale-core/internal/decision/provider"
	"brale-core/internal/store"
)

func TestBuildRoundsGroupsEventsAndInfersState(t *testing.T) {
	agents := []store.AgentEventRecord{
		{
			SnapshotID: 11,
			Symbol:     "BTCUSDT",
			Timestamp:  100,
			Stage:      "indicator",
			OutputJSON: mustJSON(t, agent.IndicatorSummary{
				Expansion:          agent.ExpansionExpanding,
				Alignment:          agent.AlignmentAligned,
				Noise:              agent.NoiseLow,
				MovementScore:      0.7,
				MovementConfidence: 0.8,
			}),
		},
		{
			SnapshotID: 11,
			Symbol:     "BTCUSDT",
			Timestamp:  100,
			Stage:      "structure",
			OutputJSON: mustJSON(t, agent.StructureSummary{
				Regime:             agent.RegimeTrendUp,
				LastBreak:          agent.LastBreakBosUp,
				Quality:            agent.QualityClean,
				Pattern:            agent.PatternNone,
				MovementScore:      0.8,
				MovementConfidence: 0.9,
			}),
		},
		{
			SnapshotID: 11,
			Symbol:     "BTCUSDT",
			Timestamp:  100,
			Stage:      "mechanics",
			OutputJSON: mustJSON(t, agent.MechanicsSummary{
				LeverageState:      agent.LeverageStateStable,
				Crowding:           agent.CrowdingBalanced,
				RiskLevel:          agent.RiskLevelLow,
				MovementScore:      0.2,
				MovementConfidence: 0.6,
			}),
		},
	}
	providers := []store.ProviderEventRecord{
		{
			SnapshotID: 11,
			Symbol:     "BTCUSDT",
			Timestamp:  100,
			Role:       "indicator",
			OutputJSON: mustJSON(t, provider.IndicatorProviderOut{MomentumExpansion: true, Alignment: true, SignalTag: "trend_surge"}),
		},
		{
			SnapshotID: 11,
			Symbol:     "BTCUSDT",
			Timestamp:  100,
			Role:       "structure",
			OutputJSON: mustJSON(t, provider.StructureProviderOut{ClearStructure: true, Integrity: true, SignalTag: "support_retest"}),
		},
		{
			SnapshotID: 11,
			Symbol:     "BTCUSDT",
			Timestamp:  100,
			Role:       "mechanics",
			OutputJSON: mustJSON(t, provider.MechanicsProviderOut{
				LiquidationStress: provider.SemanticSignal{Value: false, Confidence: provider.ConfidenceLow, Reason: "stable"},
				SignalTag:         "neutral",
			}),
		},
		{
			SnapshotID: 22,
			Symbol:     "BTCUSDT",
			Timestamp:  200,
			Role:       "indicator_in_position",
			OutputJSON: mustJSON(t, provider.InPositionIndicatorOut{MomentumSustaining: true, MonitorTag: "hold"}),
		},
		{
			SnapshotID: 22,
			Symbol:     "BTCUSDT",
			Timestamp:  200,
			Role:       "structure_in_position",
			OutputJSON: mustJSON(t, provider.InPositionStructureOut{Integrity: true, ThreatLevel: provider.ThreatLevelNone, MonitorTag: "hold"}),
		},
		{
			SnapshotID: 22,
			Symbol:     "BTCUSDT",
			Timestamp:  200,
			Role:       "mechanics_in_position",
			OutputJSON: mustJSON(t, provider.InPositionMechanicsOut{MonitorTag: "hold"}),
		},
	}
	gates := []store.GateEventRecord{
		{
			SnapshotID:      11,
			Symbol:          "BTCUSDT",
			Timestamp:       100,
			DecisionAction:  "ALLOW",
			GateReason:      "ALLOW",
			Direction:       "long",
			GlobalTradeable: true,
			DerivedJSON:     mustJSON(t, map[string]any{"current_price": 100.0}),
		},
		{
			SnapshotID:     22,
			Symbol:         "BTCUSDT",
			Timestamp:      200,
			DecisionAction: "TIGHTEN",
			GateReason:     "STRUCT_THREAT",
			DerivedJSON:    mustJSON(t, map[string]any{"current_price": 105.0}),
		},
	}

	rounds, err := BuildRounds(agents, providers, gates)
	if err != nil {
		t.Fatalf("BuildRounds: %v", err)
	}
	if len(rounds) != 2 {
		t.Fatalf("len(rounds)=%d want=2", len(rounds))
	}

	flat := rounds[0]
	if flat.SnapshotID != 11 {
		t.Fatalf("flat snapshot=%d want=11", flat.SnapshotID)
	}
	if flat.State != "FLAT" {
		t.Fatalf("flat state=%s want FLAT", flat.State)
	}
	if flat.PriceAtDecision != 100 {
		t.Fatalf("flat price=%v want 100", flat.PriceAtDecision)
	}
	if !flat.Providers.Bundle.Enabled.Indicator || !flat.Providers.Bundle.Enabled.Structure || !flat.Providers.Bundle.Enabled.Mechanics {
		t.Fatalf("expected flat provider stages to be enabled: %+v", flat.Providers.Bundle.Enabled)
	}
	if !flat.Agents.IndicatorSet || !flat.Agents.StructureSet || !flat.Agents.MechanicsSet {
		t.Fatalf("expected all flat agent stages to be present: %+v", flat.Agents)
	}

	inPosition := rounds[1]
	if inPosition.SnapshotID != 22 {
		t.Fatalf("inPosition snapshot=%d want=22", inPosition.SnapshotID)
	}
	if inPosition.State != "IN_POSITION" {
		t.Fatalf("state=%s want IN_POSITION", inPosition.State)
	}
	if !inPosition.Providers.InPosition.Ready {
		t.Fatalf("expected in-position outputs to be ready")
	}
	if inPosition.PriceAtDecision != 105 {
		t.Fatalf("inPosition price=%v want 105", inPosition.PriceAtDecision)
	}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return json.RawMessage(raw)
}
