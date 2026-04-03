package decisionview

import (
	"encoding/json"
	"fmt"
	"strings"

	"brale-core/internal/decision/decisionfmt"
	"brale-core/internal/store"

	"go.uber.org/zap"
)

func addProviderNodes(acc *roundAccumulator, df decisionfmt.Formatter, logger *zap.Logger, symbol string, providers []store.ProviderEventRecord) {
	for _, rec := range providers {
		displayOut, rawOut, err := df.HumanizeLLMOutput(json.RawMessage(rec.OutputJSON))
		if err != nil {
			logger.Warn("provider output decode failed", zap.Error(err), zap.Uint("provider_id", rec.ID))
		}
		meta := map[string]any{
			"fingerprint": rec.Fingerprint,
			"timestamp":   rec.Timestamp,
			"source":      rec.SourceVersion,
		}
		if rawOut != nil {
			meta["raw_output"] = rawOut
		}
		acc.add(rec.SnapshotID, rec.Timestamp, nodeWithOrder{
			Node: Node{
				ID:       fmt.Sprintf("%s-%d-provider-%d", symbol, rec.SnapshotID, rec.ID),
				Title:    fmt.Sprintf("Provider/%s", rec.Role),
				Stage:    "provider",
				Type:     "provider",
				AgentKey: rec.Role,
				Input: map[string]any{
					"system_prompt": rec.SystemPrompt,
					"user_prompt":   rec.UserPrompt,
				},
				Output: displayOut,
				Meta:   meta,
			},
			Order:     stageOrder("provider"),
			Timestamp: rec.Timestamp,
		})
	}
}

func addAgentNodes(acc *roundAccumulator, df decisionfmt.Formatter, logger *zap.Logger, symbol string, agents []store.AgentEventRecord) {
	for _, rec := range agents {
		displayOut, rawOut, err := df.HumanizeLLMOutput(json.RawMessage(rec.OutputJSON))
		if err != nil {
			logger.Warn("agent output decode failed", zap.Error(err), zap.Uint("agent_id", rec.ID))
		}
		meta := map[string]any{
			"fingerprint": rec.Fingerprint,
			"timestamp":   rec.Timestamp,
			"source":      rec.SourceVersion,
		}
		if rawOut != nil {
			meta["raw_output"] = rawOut
		}
		acc.add(rec.SnapshotID, rec.Timestamp, nodeWithOrder{
			Node: Node{
				ID:       fmt.Sprintf("%s-%d-agent-%s-%d", symbol, rec.SnapshotID, rec.Stage, rec.ID),
				Title:    fmt.Sprintf("Agent/%s", rec.Stage),
				Stage:    rec.Stage,
				Type:     "agent",
				AgentKey: rec.Stage,
				Input: map[string]any{
					"system_prompt": rec.SystemPrompt,
					"user_prompt":   rec.UserPrompt,
				},
				Output: displayOut,
				Meta:   meta,
			},
			Order:     stageOrder(rec.Stage),
			Timestamp: rec.Timestamp,
		})
	}
}

func addGateNodes(acc *roundAccumulator, df decisionfmt.Formatter, logger *zap.Logger, symbol string, gates []store.GateEventRecord) {
	for _, rec := range gates {
		gateEvent := decisionfmt.GateEvent{
			ID:               rec.ID,
			SnapshotID:       rec.SnapshotID,
			GlobalTradeable:  rec.GlobalTradeable,
			DecisionAction:   rec.DecisionAction,
			Grade:            rec.Grade,
			GateReason:       rec.GateReason,
			Direction:        rec.Direction,
			ProviderRefsJSON: json.RawMessage(rec.ProviderRefsJSON),
		}
		rpt, err := df.BuildGateReport(gateEvent)
		if err != nil {
			logger.Warn("gate refs parse failed", zap.Error(err), zap.Uint("gate_id", rec.ID))
			continue
		}
		gateOut := mapGateReportToDisplay(df, rpt)
		if derived := decodeMapJSON(rec.DerivedJSON); len(derived) > 0 {
			gateOut.Derived = derived
		}
		acc.add(rec.SnapshotID, rec.Timestamp, nodeWithOrder{
			Node: Node{
				ID:       fmt.Sprintf("%s-%d-gate-%d", symbol, rec.SnapshotID, rec.ID),
				Title:    "Gate",
				Stage:    "gate",
				Type:     "gate",
				AgentKey: "gate",
				Input:    nil,
				Output:   gateOut,
				Meta: map[string]any{
					"fingerprint": rec.Fingerprint,
					"timestamp":   rec.Timestamp,
					"source":      rec.SourceVersion,
				},
				Summary: rec.GateReason,
			},
			Order:     stageOrder("gate"),
			Timestamp: rec.Timestamp,
		})
	}
}

func addLLMRiskTraceNodes(acc *roundAccumulator, logger *zap.Logger, symbol string, gates []store.GateEventRecord) {
	if acc == nil {
		return
	}
	for _, gate := range gates {
		trace := llmRiskTraceFromGate(gate)
		if trace == nil {
			continue
		}
		snapshotID := gate.SnapshotID
		rb := acc.rounds[snapshotID]
		if rb == nil {
			continue
		}
		if hasNodeStage(rb.Nodes, "llm_risk") {
			continue
		}
		if !roundHasGateNode(rb.Nodes) {
			continue
		}
		latestTs := gate.Timestamp
		title, summary := llmRiskNodePresentation(gate)
		rb.Nodes = append(rb.Nodes, nodeWithOrder{
			Node: Node{
				ID:       fmt.Sprintf("%s-%d-llm-risk", symbol, snapshotID),
				Title:    title,
				Summary:  summary,
				Stage:    "llm_risk",
				Type:     "llm_risk",
				AgentKey: "llm_risk",
				Output: map[string]any{
					"snapshot_id":     snapshotID,
					"decision_action": gate.DecisionAction,
					"plan_source":     "llm",
					"trace":           trace,
				},
				Meta: map[string]any{
					"timestamp": latestTs,
					"source":    "gate_llm_trace",
				},
			},
			Order:     stageOrder("llm_risk"),
			Timestamp: latestTs,
		})
		if logger != nil {
			logger.Debug("attached llm risk trace node",
				zap.Uint("snapshot_id", snapshotID),
				zap.String("action", strings.TrimSpace(gate.DecisionAction)),
			)
		}
	}
}

func llmRiskPlanSource(gate store.GateEventRecord) string {
	derived := decodeMapJSON(gate.DerivedJSON)
	if len(derived) == 0 {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(gate.DecisionAction), "tighten") {
		execRaw, ok := derived["execution"].(map[string]any)
		if !ok {
			return ""
		}
		return normalizePlanSourceValue(execRaw["plan_source"])
	}
	planRaw, ok := derived["plan"].(map[string]any)
	if !ok {
		return ""
	}
	return normalizePlanSourceValue(planRaw["plan_source"])
}

func llmRiskTraceFromGate(gate store.GateEventRecord) map[string]any {
	if llmRiskPlanSource(gate) != "llm" {
		return nil
	}
	derived := decodeMapJSON(gate.DerivedJSON)
	if len(derived) == 0 {
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(gate.DecisionAction), "tighten") {
		execRaw, ok := derived["execution"].(map[string]any)
		if !ok {
			return nil
		}
		trace, _ := execRaw["llm_trace"].(map[string]any)
		if len(trace) == 0 {
			return nil
		}
		return trace
	}
	planRaw, ok := derived["plan"].(map[string]any)
	if !ok {
		return nil
	}
	trace, _ := planRaw["llm_trace"].(map[string]any)
	if len(trace) == 0 {
		return nil
	}
	return trace
}

func normalizePlanSourceValue(value any) string {
	switch strings.ToLower(strings.TrimSpace(fmt.Sprint(value))) {
	case "llm":
		return "llm"
	case "go":
		return "go"
	default:
		return ""
	}
}

func llmRiskNodePresentation(gate store.GateEventRecord) (string, string) {
	action := strings.ToUpper(strings.TrimSpace(gate.DecisionAction))
	switch action {
	case "TIGHTEN":
		return "LLM 止盈止损", "LLM 持仓风控"
	default:
		return "LLM 开仓风控", "LLM 开仓风控计划"
	}
}

func mapGateReportToDisplay(df decisionfmt.Formatter, rpt decisionfmt.GateReport) gateDisplay {
	provs := make([]gateProviderStatus, len(rpt.Providers))
	for i, p := range rpt.Providers {
		factors := make([]gateFactor, len(p.Factors))
		for j, f := range p.Factors {
			factors[j] = gateFactor{
				Key:    f.Key,
				Label:  f.Label,
				Status: f.Status,
				Raw:    f.Raw,
			}
		}
		provs[i] = gateProviderStatus{
			Role:          p.Role,
			Tradeable:     p.Tradeable,
			TradeableText: p.TradeableText,
			Factors:       factors,
		}
	}
	return gateDisplay{
		Overall: gateOverall{
			Tradeable:      rpt.Overall.Tradeable,
			TradeableText:  rpt.Overall.TradeableText,
			DecisionAction: rpt.Overall.DecisionAction,
			DecisionText:   rpt.Overall.DecisionText,
			Grade:          rpt.Overall.Grade,
			Reason:         rpt.Overall.Reason,
			ReasonCode:     rpt.Overall.ReasonCode,
			Direction:      rpt.Overall.Direction,
			ExpectedSnapID: rpt.Overall.ExpectedSnapID,
		},
		Providers: provs,
		Derived:   nil,
		Report:    df.RenderGateText(rpt),
	}
}

func decodeMapJSON(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil || len(out) == 0 {
		return nil
	}
	return out
}

func decodeAnyJSON(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return string(raw)
	}
	return out
}

func isInPositionProviderRole(role string) bool {
	r := strings.ToLower(strings.TrimSpace(role))
	return strings.HasSuffix(r, "_in_position")
}
