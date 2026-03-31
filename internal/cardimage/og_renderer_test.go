package cardimage

import (
	"bytes"
	"encoding/json"
	"testing"

	"brale-core/internal/decision/decisionfmt"
)

func TestBuildPayloadUsesRawAgentOutputs(t *testing.T) {
	indicatorRaw := mustJSON(t, map[string]any{
		"expansion":           "contracting",
		"alignment":           "mixed",
		"noise":               "low",
		"momentum_detail":     "fast positive",
		"conflict_detail":     "none",
		"movement_score":      0.12,
		"movement_confidence": 0.33,
	})
	mechanicsRaw := mustJSON(t, map[string]any{
		"leverage_state":        "stable",
		"crowding":              "long_crowded",
		"risk_level":            "medium",
		"open_interest_context": "oi mixed",
		"anomaly_detail":        "fear high",
		"movement_score":        0.25,
		"movement_confidence":   0.44,
	})
	structureRaw := mustJSON(t, map[string]any{
		"regime":              "mixed",
		"last_break":          "bos_down",
		"quality":             "messy",
		"pattern":             "head_shoulders",
		"volume_action":       "heavy",
		"candle_reaction":     "wick rejection",
		"movement_score":      0.05,
		"movement_confidence": 0.10,
	})
	input := decisionfmt.DecisionInput{
		Symbol:     "ETHUSDT",
		SnapshotID: 42,
		Agents: []decisionfmt.AgentEvent{
			{Stage: "indicator", OutputJSON: indicatorRaw},
			{Stage: "mechanics", OutputJSON: mechanicsRaw},
			{Stage: "structure", OutputJSON: structureRaw},
		},
	}
	report := decisionfmt.DecisionReport{Symbol: "ETHUSDT", SnapshotID: 42, Gate: decisionfmt.GateReport{Overall: decisionfmt.GateOverall{Tradeable: false, DecisionAction: "VETO", DecisionText: "否决", Direction: "none", Grade: 0, Reason: "三路共识未通过", ReasonCode: "CONSENSUS_NOT_PASSED"}, RuleHit: &decisionfmt.GateRuleHit{Name: "MECH_RISK"}, Derived: map[string]any{"gate_stop_step": "mech_risk", "gate_action_before_sieve": "ALLOW", "sieve_action": "VETO", "sieve_reason": "crowded_long", "gate_trace": []any{map[string]any{"step": "direction", "ok": true}, map[string]any{"step": "mech_risk", "ok": false, "reason": "MECH_RISK"}}, "direction_consensus": map[string]any{"score": -0.276, "confidence": 0.116, "score_threshold": 0.2, "confidence_threshold": 0.3, "score_passed": true, "confidence_passed": false, "passed": false}}}}
	payload, err := buildPayload(input, report)
	if err != nil {
		t.Fatalf("buildPayload failed: %v", err)
	}
	if payload.RawBlocks.Gate.DecisionAction != "VETO" {
		t.Fatalf("unexpected gate action: %s", payload.RawBlocks.Gate.DecisionAction)
	}
	if payload.RawBlocks.Agent.Indicator.Alignment != "mixed" {
		t.Fatalf("unexpected indicator alignment: %s", payload.RawBlocks.Agent.Indicator.Alignment)
	}
	if payload.RawBlocks.Agent.Mechanics.Crowding != "long_crowded" {
		t.Fatalf("unexpected mechanics crowding: %s", payload.RawBlocks.Agent.Mechanics.Crowding)
	}
	if payload.RawBlocks.Agent.Structure.Pattern != "head_shoulders" {
		t.Fatalf("unexpected structure pattern: %s", payload.RawBlocks.Agent.Structure.Pattern)
	}
	if payload.RawBlocks.Gate.Consensus == nil {
		t.Fatalf("expected consensus metrics in gate payload")
	}
	if payload.RawBlocks.Gate.Consensus.Score == nil || *payload.RawBlocks.Gate.Consensus.Score != -0.276 {
		t.Fatalf("unexpected consensus score: %+v", payload.RawBlocks.Gate.Consensus.Score)
	}
	if payload.RawBlocks.Gate.Consensus.ConfidenceThreshold == nil || *payload.RawBlocks.Gate.Consensus.ConfidenceThreshold != 0.3 {
		t.Fatalf("unexpected consensus confidence threshold: %+v", payload.RawBlocks.Gate.Consensus.ConfidenceThreshold)
	}
	if payload.RawBlocks.Gate.StopStep != "mech_risk" {
		t.Fatalf("unexpected stop step: %s", payload.RawBlocks.Gate.StopStep)
	}
	if payload.RawBlocks.Gate.RuleName != "MECH_RISK" {
		t.Fatalf("unexpected rule name: %s", payload.RawBlocks.Gate.RuleName)
	}
	if payload.RawBlocks.Gate.ActionBefore != "ALLOW" || payload.RawBlocks.Gate.SieveAction != "VETO" {
		t.Fatalf("unexpected sieve override: before=%s action=%s", payload.RawBlocks.Gate.ActionBefore, payload.RawBlocks.Gate.SieveAction)
	}
	report.Gate.Derived["execution"] = map[string]any{
		"action":       "tighten",
		"evaluated":    true,
		"executed":     false,
		"blocked_by":   []string{"score_threshold"},
		"stop_loss":    2415.5,
		"take_profits": []float64{2450.1, 2480.2},
	}
	payload, err = buildPayload(input, report)
	if err != nil {
		t.Fatalf("buildPayload with execution failed: %v", err)
	}
	if payload.RawBlocks.Gate.Execution == nil {
		t.Fatalf("expected execution payload in gate block")
	}
	if payload.RawBlocks.Gate.Execution["action"] != "tighten" {
		t.Fatalf("unexpected execution action: %+v", payload.RawBlocks.Gate.Execution)
	}
	if len(payload.RawBlocks.Gate.Trace) != 2 {
		t.Fatalf("unexpected gate trace length: %d", len(payload.RawBlocks.Gate.Trace))
	}
	if payload.RawBlocks.Gate.Trace[1].Step != "mech_risk" || payload.RawBlocks.Gate.Trace[1].Reason != "MECH_RISK" || payload.RawBlocks.Gate.Trace[1].OK {
		t.Fatalf("unexpected second trace step: %+v", payload.RawBlocks.Gate.Trace[1])
	}
}

func TestOGRendererRenderDecision(t *testing.T) {
	indicatorRaw := mustJSON(t, map[string]any{
		"expansion":           "contracting",
		"alignment":           "mixed",
		"noise":               "low",
		"momentum_detail":     "fast positive",
		"conflict_detail":     "none",
		"movement_score":      0.12,
		"movement_confidence": 0.33,
	})
	mechanicsRaw := mustJSON(t, map[string]any{
		"leverage_state":        "stable",
		"crowding":              "long_crowded",
		"risk_level":            "medium",
		"open_interest_context": "oi mixed",
		"anomaly_detail":        "fear high",
		"movement_score":        0.25,
		"movement_confidence":   0.44,
	})
	structureRaw := mustJSON(t, map[string]any{
		"regime":              "mixed",
		"last_break":          "bos_down",
		"quality":             "messy",
		"pattern":             "head_shoulders",
		"volume_action":       "heavy",
		"candle_reaction":     "wick rejection",
		"movement_score":      0.05,
		"movement_confidence": 0.10,
	})
	input := decisionfmt.DecisionInput{
		Symbol:     "ETHUSDT",
		SnapshotID: 77,
		Agents: []decisionfmt.AgentEvent{
			{Stage: "indicator", OutputJSON: indicatorRaw},
			{Stage: "mechanics", OutputJSON: mechanicsRaw},
			{Stage: "structure", OutputJSON: structureRaw},
		},
	}
	report := decisionfmt.DecisionReport{Symbol: "ETHUSDT", SnapshotID: 77, Gate: decisionfmt.GateReport{Overall: decisionfmt.GateOverall{Tradeable: false, DecisionAction: "VETO", DecisionText: "否决", Direction: "none", Grade: 0, Reason: "三路共识未通过", ReasonCode: "CONSENSUS_NOT_PASSED"}}}
	renderer := NewOGRenderer()
	asset, err := renderer.RenderDecision(t.Context(), input, report)
	if err != nil {
		t.Fatalf("RenderDecision failed: %v", err)
	}
	if asset == nil || len(asset.Data) == 0 {
		t.Fatalf("expected rendered image bytes")
	}
	if !bytes.HasPrefix(asset.Data, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("rendered data is not png")
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json marshal failed: %v", err)
	}
	return raw
}
