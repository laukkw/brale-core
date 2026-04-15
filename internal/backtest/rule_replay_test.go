package backtest

import (
	"context"
	"testing"

	"brale-core/internal/config"
	"brale-core/internal/decision/agent"
	"brale-core/internal/decision/provider"
	"brale-core/internal/store"
	"brale-core/internal/strategy"
)

func TestRuleReplayRunReplaysFlatRounds(t *testing.T) {
	ctx := context.Background()
	st := newReplayTestStore(t)

	saveReplayFixture(t, st, store.AgentEventRecord{
		SnapshotID: 101,
		Symbol:     "BTCUSDT",
		Timestamp:  100,
		Stage:      "indicator",
		OutputJSON: mustJSON(t, agent.IndicatorSummary{
			Expansion:          agent.ExpansionExpanding,
			Alignment:          agent.AlignmentAligned,
			Noise:              agent.NoiseLow,
			MovementScore:      0.70,
			MovementConfidence: 0.80,
		}),
	}, store.AgentEventRecord{
		SnapshotID: 101,
		Symbol:     "BTCUSDT",
		Timestamp:  100,
		Stage:      "structure",
		OutputJSON: mustJSON(t, agent.StructureSummary{
			Regime:             agent.RegimeTrendUp,
			LastBreak:          agent.LastBreakBosUp,
			Quality:            agent.QualityClean,
			Pattern:            agent.PatternNone,
			MovementScore:      0.85,
			MovementConfidence: 0.90,
		}),
	}, store.AgentEventRecord{
		SnapshotID: 101,
		Symbol:     "BTCUSDT",
		Timestamp:  100,
		Stage:      "mechanics",
		OutputJSON: mustJSON(t, agent.MechanicsSummary{
			LeverageState:      agent.LeverageStateStable,
			Crowding:           agent.CrowdingBalanced,
			RiskLevel:          agent.RiskLevelLow,
			MovementScore:      0.10,
			MovementConfidence: 0.60,
		}),
	}, store.ProviderEventRecord{
		SnapshotID: 101,
		Symbol:     "BTCUSDT",
		Timestamp:  100,
		Role:       "indicator",
		OutputJSON: mustJSON(t, provider.IndicatorProviderOut{MomentumExpansion: true, Alignment: true, SignalTag: "trend_surge"}),
	}, store.ProviderEventRecord{
		SnapshotID: 101,
		Symbol:     "BTCUSDT",
		Timestamp:  100,
		Role:       "structure",
		OutputJSON: mustJSON(t, provider.StructureProviderOut{ClearStructure: true, Integrity: true, SignalTag: "support_retest"}),
	}, store.ProviderEventRecord{
		SnapshotID: 101,
		Symbol:     "BTCUSDT",
		Timestamp:  100,
		Role:       "mechanics",
		OutputJSON: mustJSON(t, provider.MechanicsProviderOut{
			LiquidationStress: provider.SemanticSignal{Value: false, Confidence: provider.ConfidenceLow, Reason: "stable"},
			SignalTag:         "neutral",
		}),
	}, store.GateEventRecord{
		SnapshotID:      101,
		Symbol:          "BTCUSDT",
		Timestamp:       100,
		GlobalTradeable: true,
		DecisionAction:  "ALLOW",
		GateReason:      "ALLOW",
		Direction:       "long",
		Grade:           3,
		DerivedJSON: mustJSON(t, map[string]any{
			"current_price": 100.0,
			"direction_consensus": map[string]any{
				"score":                0.82,
				"confidence":           0.74,
				"agreement":            0.90,
				"coverage":             0.81,
				"resonance_bonus":      0.05,
				"resonance_active":     true,
				"score_threshold":      0.35,
				"confidence_threshold": 0.52,
				"sources": map[string]any{
					"indicator": map[string]any{"confidence": 0.80, "raw_confidence": 0.80},
					"structure": map[string]any{"confidence": 0.90, "raw_confidence": 0.90},
					"mechanics": map[string]any{"confidence": 0.60, "raw_confidence": 0.60},
				},
			},
		}),
	})
	saveReplayFixture(t, st, store.GateEventRecord{
		SnapshotID:      102,
		Symbol:          "BTCUSDT",
		Timestamp:       200,
		GlobalTradeable: false,
		DecisionAction:  "WAIT",
		GateReason:      "QUALITY_TOO_LOW",
		Direction:       "long",
		DerivedJSON: mustJSON(t, map[string]any{
			"current_price": 108.0,
			"direction_consensus": map[string]any{
				"score":                0.20,
				"confidence":           0.40,
				"agreement":            0.40,
				"coverage":             0.50,
				"resonance_bonus":      0.00,
				"resonance_active":     false,
				"score_threshold":      0.35,
				"confidence_threshold": 0.52,
			},
		}),
	})

	replay := RuleReplay{
		Store:               st,
		Binding:             testReplayBinding(),
		ScoreThreshold:      0.35,
		ConfidenceThreshold: 0.52,
	}
	result, err := replay.Run(ctx, "btcusdt", TimeRange{StartUnix: 100, EndUnix: 200})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Rounds) != 2 {
		t.Fatalf("len(rounds)=%d want=2", len(result.Rounds))
	}
	if result.Rounds[0].Skipped {
		t.Fatalf("expected flat round to be replayed")
	}
	if result.Rounds[0].ReplayedGate.DecisionAction != "ALLOW" {
		t.Fatalf("replayed action=%s want ALLOW", result.Rounds[0].ReplayedGate.DecisionAction)
	}
	if result.Rounds[0].Changed {
		t.Fatalf("expected first round to remain unchanged")
	}
	if result.Rounds[0].PriceAtDecision != 100 || result.Rounds[0].PriceAfter != 108 {
		t.Fatalf("unexpected prices: %+v", result.Rounds[0])
	}
}

func TestRuleReplayRunSkipsInPositionRounds(t *testing.T) {
	ctx := context.Background()
	st := newReplayTestStore(t)

	saveReplayFixture(t, st, store.ProviderEventRecord{
		SnapshotID: 201,
		Symbol:     "ETHUSDT",
		Timestamp:  300,
		Role:       "indicator_in_position",
		OutputJSON: mustJSON(t, provider.InPositionIndicatorOut{MomentumSustaining: true}),
	}, store.ProviderEventRecord{
		SnapshotID: 201,
		Symbol:     "ETHUSDT",
		Timestamp:  300,
		Role:       "structure_in_position",
		OutputJSON: mustJSON(t, provider.InPositionStructureOut{Integrity: true, ThreatLevel: provider.ThreatLevelNone}),
	}, store.ProviderEventRecord{
		SnapshotID: 201,
		Symbol:     "ETHUSDT",
		Timestamp:  300,
		Role:       "mechanics_in_position",
		OutputJSON: mustJSON(t, provider.InPositionMechanicsOut{}),
	}, store.GateEventRecord{
		SnapshotID:      201,
		Symbol:          "ETHUSDT",
		Timestamp:       300,
		GlobalTradeable: false,
		DecisionAction:  "TIGHTEN",
		GateReason:      "STRUCT_THREAT",
		DerivedJSON:     mustJSON(t, map[string]any{"current_price": 2400.0}),
	})

	replay := RuleReplay{
		Store:               st,
		Binding:             testReplayBinding(),
		ScoreThreshold:      0.35,
		ConfidenceThreshold: 0.52,
	}
	result, err := replay.Run(ctx, "ETHUSDT", TimeRange{StartUnix: 300, EndUnix: 300})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Rounds) != 1 {
		t.Fatalf("len(rounds)=%d want=1", len(result.Rounds))
	}
	if !result.Rounds[0].Skipped {
		t.Fatalf("expected in-position round to be skipped")
	}
	if result.Rounds[0].SkipReason == "" {
		t.Fatalf("expected skip reason")
	}
}

func TestRuleReplayRunUsesDerivedConsensusWhenAgentsMissing(t *testing.T) {
	ctx := context.Background()
	st := newReplayTestStore(t)

	saveReplayFixture(t, st, store.ProviderEventRecord{
		SnapshotID: 301,
		Symbol:     "SOLUSDT",
		Timestamp:  400,
		Role:       "indicator",
		OutputJSON: mustJSON(t, provider.IndicatorProviderOut{MomentumExpansion: true, Alignment: true, SignalTag: "trend_surge"}),
	}, store.ProviderEventRecord{
		SnapshotID: 301,
		Symbol:     "SOLUSDT",
		Timestamp:  400,
		Role:       "structure",
		OutputJSON: mustJSON(t, provider.StructureProviderOut{ClearStructure: true, Integrity: true, SignalTag: "support_retest"}),
	}, store.ProviderEventRecord{
		SnapshotID: 301,
		Symbol:     "SOLUSDT",
		Timestamp:  400,
		Role:       "mechanics",
		OutputJSON: mustJSON(t, provider.MechanicsProviderOut{
			LiquidationStress: provider.SemanticSignal{Value: false, Confidence: provider.ConfidenceLow, Reason: "stable"},
			SignalTag:         "neutral",
		}),
	}, store.GateEventRecord{
		SnapshotID:      301,
		Symbol:          "SOLUSDT",
		Timestamp:       400,
		GlobalTradeable: true,
		DecisionAction:  "ALLOW",
		GateReason:      "ALLOW",
		Direction:       "long",
		Grade:           3,
		DerivedJSON: mustJSON(t, map[string]any{
			"current_price": 150.0,
			"direction_consensus": map[string]any{
				"score":                0.78,
				"confidence":           0.73,
				"agreement":            0.88,
				"coverage":             0.80,
				"resonance_bonus":      0.04,
				"resonance_active":     true,
				"score_threshold":      0.35,
				"confidence_threshold": 0.52,
				"sources": map[string]any{
					"indicator": map[string]any{"confidence": 0.75, "raw_confidence": 0.75},
					"structure": map[string]any{"confidence": 0.82, "raw_confidence": 0.82},
					"mechanics": map[string]any{"confidence": 0.55, "raw_confidence": 0.55},
				},
			},
		}),
	})

	replay := RuleReplay{
		Store:               st,
		Binding:             testReplayBinding(),
		ScoreThreshold:      0.35,
		ConfidenceThreshold: 0.52,
	}
	result, err := replay.Run(ctx, "SOLUSDT", TimeRange{StartUnix: 400, EndUnix: 400})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Rounds) != 1 {
		t.Fatalf("len(rounds)=%d want=1", len(result.Rounds))
	}
	if result.Rounds[0].ReplayedGate.DecisionAction != "ALLOW" {
		t.Fatalf("replayed action=%s want ALLOW", result.Rounds[0].ReplayedGate.DecisionAction)
	}
}

func newReplayTestStore(t *testing.T) store.Store {
	t.Helper()
	t.Skip("requires PostgreSQL")
	return nil
}

func testReplayBinding() strategy.StrategyBinding {
	return strategy.StrategyBinding{
		Symbol: "BTCUSDT",
		RiskManagement: config.RiskManagementConfig{
			Gate: config.GateConfig{
				QualityThreshold: 0.35,
				EdgeThreshold:    0.10,
			},
			Sieve: config.RiskManagementSieveConfig{
				DefaultGateAction: "ALLOW",
				DefaultSizeFactor: 1.0,
			},
		},
	}
}

func saveReplayFixture(t *testing.T, st store.Store, records ...any) {
	t.Helper()
	ctx := context.Background()
	for _, rec := range records {
		switch value := rec.(type) {
		case store.AgentEventRecord:
			item := value
			if err := st.SaveAgentEvent(ctx, &item); err != nil {
				t.Fatalf("save agent event: %v", err)
			}
		case store.ProviderEventRecord:
			item := value
			if err := st.SaveProviderEvent(ctx, &item); err != nil {
				t.Fatalf("save provider event: %v", err)
			}
		case store.GateEventRecord:
			item := value
			if err := st.SaveGateEvent(ctx, &item); err != nil {
				t.Fatalf("save gate event: %v", err)
			}
		default:
			t.Fatalf("unsupported fixture type %T", rec)
		}
	}
}
