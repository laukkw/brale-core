package decisionview

import (
	"encoding/json"
	"fmt"
	"sort"
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

func addLLMRiskTraceNodes(acc *roundAccumulator, logger *zap.Logger, symbol string, providers []store.ProviderEventRecord, openPos store.PositionRecord, hasOpenPos bool) {
	if acc == nil {
		return
	}
	if !hasOpenPos || strings.TrimSpace(openPos.PositionID) == "" || len(openPos.RiskJSON) == 0 {
		return
	}
	traceBySnapshot := make(map[uint][]llmRiskTraceRecord)
	positionOpenTS := openPos.CreatedAt.Unix()
	hasPositionOpenTS := !openPos.CreatedAt.IsZero() && positionOpenTS > 0
	for _, rec := range providers {
		if !isInPositionProviderRole(rec.Role) {
			continue
		}
		if hasPositionOpenTS && rec.Timestamp < positionOpenTS {
			continue
		}
		if strings.TrimSpace(rec.SystemPrompt) == "" && strings.TrimSpace(rec.UserPrompt) == "" && len(rec.OutputJSON) == 0 {
			continue
		}
		traceBySnapshot[rec.SnapshotID] = append(traceBySnapshot[rec.SnapshotID], llmRiskTraceRecord{
			Role:         strings.TrimSpace(rec.Role),
			Timestamp:    rec.Timestamp,
			SystemPrompt: rec.SystemPrompt,
			UserPrompt:   rec.UserPrompt,
			Output:       decodeAnyJSON(rec.OutputJSON),
		})
	}
	if len(traceBySnapshot) == 0 {
		return
	}
	for snapshotID, traces := range traceBySnapshot {
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
		sort.Slice(traces, func(i, j int) bool {
			return traces[i].Timestamp < traces[j].Timestamp
		})
		latestTs := traces[len(traces)-1].Timestamp
		summary := fmt.Sprintf("%d 条止盈止损 LLM 记录", len(traces))
		rb.Nodes = append(rb.Nodes, nodeWithOrder{
			Node: Node{
				ID:       fmt.Sprintf("%s-%d-llm-risk", symbol, snapshotID),
				Title:    "LLM 止盈止损",
				Summary:  summary,
				Stage:    "llm_risk",
				Type:     "llm_risk",
				AgentKey: "llm_risk",
				Output: map[string]any{
					"position_id":     openPos.PositionID,
					"position_status": openPos.Status,
					"snapshot_id":     snapshotID,
					"records":         traces,
				},
				Meta: map[string]any{
					"timestamp": latestTs,
					"source":    "provider_in_position",
				},
			},
			Order:     stageOrder("llm_risk"),
			Timestamp: latestTs,
		})
		if logger != nil {
			logger.Debug("attached llm risk trace node",
				zap.Uint("snapshot_id", snapshotID),
				zap.Int("records", len(traces)),
			)
		}
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
