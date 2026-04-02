package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"brale-core/internal/pkg/parseutil"
	"brale-core/internal/store"
)

func ResolveTightenInfo(gate *store.GateEventRecord) *TightenInfo {
	if gate == nil || len(gate.DerivedJSON) == 0 {
		return nil
	}
	var derived map[string]any
	if err := json.Unmarshal(gate.DerivedJSON, &derived); err != nil {
		return nil
	}
	executionRaw, ok := derived["execution"].(map[string]any)
	if !ok {
		return nil
	}
	action, _ := executionRaw["action"].(string)
	if !strings.EqualFold(strings.TrimSpace(action), "tighten") {
		return nil
	}
	executed := false
	if value, ok := executionRaw["executed"].(bool); ok {
		executed = value
	}
	reason := "tighten_metadata_present"
	if blockedBy, ok := executionRaw["blocked_by"].([]any); ok && len(blockedBy) > 0 {
		if first, ok := blockedBy[0].(string); ok && strings.TrimSpace(first) != "" {
			reason = strings.TrimSpace(first)
		}
	}
	if executed {
		reason = "executed"
	}
	return &TightenInfo{Triggered: executed, Reason: reason}
}

func ResolveTightenFromRiskHistory(ctx context.Context, st store.RiskPlanQueryStore, pos store.PositionRecord, timelineLimit int) *TightenInfo {
	if st == nil || strings.TrimSpace(pos.PositionID) == "" {
		return nil
	}
	timeline := LoadRiskPlanTimeline(ctx, st, pos, timelineLimit)
	if len(timeline) == 0 {
		return nil
	}
	for _, item := range timeline {
		if IsTightenSource(item.Source) {
			return &TightenInfo{Triggered: true, Reason: strings.ToLower(strings.TrimSpace(item.Source))}
		}
	}
	return nil
}

func BuildDecisionTightenDetail(raw json.RawMessage) *DecisionTightenDetail {
	if len(raw) == 0 {
		return nil
	}
	var derived map[string]any
	if err := json.Unmarshal(raw, &derived); err != nil {
		return nil
	}
	execRaw, ok := derived["execution"].(map[string]any)
	if !ok || len(execRaw) == 0 {
		return nil
	}
	action := strings.TrimSpace(fmt.Sprint(execRaw["action"]))
	if !strings.EqualFold(action, "tighten") {
		return nil
	}
	detail := &DecisionTightenDetail{
		Action:      strings.ToUpper(action),
		Evaluated:   parseDecisionBool(execRaw["evaluated"]),
		Eligible:    parseDecisionBool(execRaw["eligible"]),
		Executed:    parseDecisionBool(execRaw["executed"]),
		TPTightened: parseDecisionBool(execRaw["tp_tightened"]),
	}
	if blockedBy, ok := execRaw["blocked_by"].([]any); ok {
		list := make([]string, 0, len(blockedBy))
		for _, item := range blockedBy {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" {
				list = append(list, text)
			}
		}
		detail.BlockedBy = list
	}
	if score, ok := execRaw["score"].(map[string]any); ok {
		detail.Score = parseDecisionFloat(score["total"])
		detail.ScoreThreshold = parseDecisionFloat(score["threshold"])
		detail.ScoreParseOK = parseDecisionBool(score["parse_ok"])
	}
	detail.DisplayReason = tightenDisplayReason(detail)
	return detail
}

func ExtractConsensusMetrics(raw json.RawMessage) ConsensusMetrics {
	if len(raw) == 0 {
		return ConsensusMetrics{}
	}
	var derived map[string]any
	if err := json.Unmarshal(raw, &derived); err != nil {
		return ConsensusMetrics{}
	}
	consensusRaw, ok := derived["direction_consensus"]
	if !ok {
		return ConsensusMetrics{}
	}
	consensus, ok := consensusRaw.(map[string]any)
	if !ok || len(consensus) == 0 {
		return ConsensusMetrics{}
	}
	out := ConsensusMetrics{}
	if score, ok := parseutil.FloatOK(consensus["score"]); ok {
		out.Score = &score
	}
	if confidence, ok := parseutil.FloatOK(consensus["confidence"]); ok {
		out.Confidence = &confidence
	}
	if scoreThreshold, ok := parseutil.FloatOK(consensus["score_threshold"]); ok {
		out.ScoreThreshold = &scoreThreshold
	}
	if confidenceThreshold, ok := parseutil.FloatOK(consensus["confidence_threshold"]); ok {
		out.ConfidenceThreshold = &confidenceThreshold
	}
	if scorePassed, ok := parseConsensusBool(consensus["score_passed"]); ok {
		out.ScorePassed = boolPtr(scorePassed)
	} else if out.Score != nil && out.ScoreThreshold != nil {
		out.ScorePassed = boolPtr(absFloat(*out.Score) >= *out.ScoreThreshold)
	}
	if confidencePassed, ok := parseConsensusBool(consensus["confidence_passed"]); ok {
		out.ConfidencePassed = boolPtr(confidencePassed)
	} else if out.Confidence != nil && out.ConfidenceThreshold != nil {
		out.ConfidencePassed = boolPtr(*out.Confidence >= *out.ConfidenceThreshold)
	}
	if passed, ok := parseConsensusBool(consensus["passed"]); ok {
		out.Passed = boolPtr(passed)
	} else if out.ScorePassed != nil && out.ConfidencePassed != nil {
		out.Passed = boolPtr(*out.ScorePassed && *out.ConfidencePassed)
	}
	return out
}

func tightenDisplayReason(detail *DecisionTightenDetail) string {
	if detail == nil {
		return ""
	}
	if detail.Executed {
		if detail.TPTightened {
			return "已执行收紧，并同步收紧止盈"
		}
		return "已执行持仓收紧"
	}
	if len(detail.BlockedBy) > 0 {
		return "收紧未执行: " + detail.BlockedBy[0]
	}
	if detail.Eligible {
		return "满足收紧条件，等待执行"
	}
	if detail.Evaluated {
		return "已评估持仓收紧，但未触发"
	}
	return "持仓收紧未评估"
}

func parseDecisionFloat(value any) float64 {
	if parsed, ok := parseutil.FloatOK(value); ok {
		return parsed
	}
	return 0
}

func parseDecisionBool(value any) bool {
	parsed, ok := parseConsensusBool(value)
	return ok && parsed
}

func parseConsensusBool(value any) (bool, bool) {
	switch raw := value.(type) {
	case bool:
		return raw, true
	case string:
		trimmed := strings.TrimSpace(strings.ToLower(raw))
		if trimmed == "true" {
			return true, true
		}
		if trimmed == "false" {
			return false, true
		}
		return false, false
	case float64:
		return raw != 0, true
	case float32:
		return raw != 0, true
	case int:
		return raw != 0, true
	case int64:
		return raw != 0, true
	case uint64:
		return raw != 0, true
	default:
		return false, false
	}
}

func boolPtr(value bool) *bool {
	out := value
	return &out
}

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}
