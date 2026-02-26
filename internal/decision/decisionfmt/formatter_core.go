// 本文件主要内容：决策报告与 LLM 输出格式化。

package decisionfmt

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	narrativeState    = "state"
	narrativeAction   = "action"
	narrativeConflict = "conflict"
	narrativeRisk     = "risk"
)

var narrativeKeyOrder = map[string][]string{
	narrativeState: {
		"regime",
		"pattern",
		"quality",
		"alignment",
		"noise",
		"expansion",
		"momentum_detail",
		"volume_action",
		"candle_reaction",
		"open_interest_context",
	},
	narrativeAction: {
		"last_break",
		"signal_tag",
		"monitor_tag",
		"momentum_expansion",
		"clear_structure",
		"integrity",
	},
	narrativeConflict: {
		"conflict_detail",
		"divergence_detected",
		"mean_rev_noise",
		"momentum_sustaining",
		"reason",
	},
	narrativeRisk: {
		"risk_level",
		"crowding",
		"leverage_state",
		"liquidation_stress",
		"adverse_liquidation",
		"crowding_reversal",
		"liq_stress",
		"threat_level",
	},
}

func buildNarrativeSummary(data map[string]any) string {
	if len(data) == 0 {
		return ""
	}
	normalized := make(map[string]any, len(data))
	notes := ""
	for k, v := range data {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "notes" {
			if raw, ok := v.(string); ok {
				notes = strings.TrimSpace(raw)
			}
			continue
		}
		normalized[key] = v
	}

	stateParts := collectNarrativeParts(narrativeState, normalized)
	actionParts := collectNarrativeParts(narrativeAction, normalized)
	conflictParts := collectNarrativeParts(narrativeConflict, normalized)
	riskParts := collectNarrativeParts(narrativeRisk, normalized)

	hasContent := len(stateParts)+len(actionParts)+len(conflictParts)+len(riskParts) > 0
	if !hasContent {
		return ""
	}

	lines := []string{
		formatNarrativeLine("状态", stateParts),
		formatNarrativeLine("动作", actionParts),
		formatNarrativeLine("冲突", conflictParts),
		formatNarrativeLine("风险", riskParts),
	}
	if notes != "" {
		lines = append(lines, fmt.Sprintf("补充：%s", notes))
	}
	return strings.Join(lines, "\n")
}

func collectNarrativeParts(category string, normalized map[string]any) []string {
	order := narrativeKeyOrder[category]
	if len(order) == 0 {
		return nil
	}
	parts := make([]string, 0, len(order))
	for _, key := range order {
		value, ok := normalized[key]
		if !ok {
			continue
		}
		if key == "threat_level" {
			if raw, ok := value.(string); ok && strings.TrimSpace(raw) == "" {
				continue
			}
		}
		entry := formatNarrativeEntry(key, value)
		if entry == "" {
			continue
		}
		parts = append(parts, entry)
	}
	return parts
}

func formatNarrativeEntry(key string, value any) string {
	if raw, ok := value.(bool); ok {
		if text, ok := formatBoolWithKey(key, raw); ok {
			return text
		}
	}
	text := formatLLMValue(key, value)
	if strings.TrimSpace(text) == "" {
		return ""
	}
	label := translateLLMKey(key)
	if strings.TrimSpace(label) == "" || label == key {
		return text
	}
	return fmt.Sprintf("%s=%s", label, text)
}

func formatNarrativeLine(label string, parts []string) string {
	if len(parts) == 0 {
		return fmt.Sprintf("%s：—", label)
	}
	return fmt.Sprintf("%s：%s", label, strings.Join(parts, "；"))
}

func New() Formatter {
	return DefaultFormatter{}
}

type DefaultFormatter struct{}

func (f DefaultFormatter) BuildGateReport(record GateEvent) (GateReport, error) {
	providers := []GateProviderStatus{}
	if !isHoldDecision(record.DecisionAction) {
		refs, err := parseGateRefs(record.ProviderRefsJSON)
		if err != nil {
			return GateReport{}, err
		}
		providers = buildGateProviders(refs)
	}
	ruleHit, err := parseGateRuleHit(record.RuleHitJSON)
	if err != nil {
		return GateReport{}, err
	}
	derived, err := parseDerived(record.DerivedJSON)
	if err != nil {
		return GateReport{}, err
	}
	decisionText := GateDecisionText(record.DecisionAction, record.GateReason)
	overall := GateOverall{
		Tradeable:      record.GlobalTradeable,
		TradeableText:  translateBoolAction(record.GlobalTradeable),
		DecisionAction: record.DecisionAction,
		DecisionText:   decisionText,
		Grade:          record.Grade,
		Reason:         translateGateReason(record.GateReason),
		ReasonCode:     record.GateReason,
		Direction:      translateDirection(record.Direction),
	}

	return GateReport{
		Overall:   overall,
		Providers: providers,
		RuleHit:   ruleHit,
		Derived:   derived,
	}, nil
}

func (f DefaultFormatter) BuildMissingGateReport(snapshotID uint) GateReport {
	overall := GateOverall{
		Tradeable:      false,
		TradeableText:  translateBoolAction(false),
		DecisionAction: "MISSING",
		DecisionText:   "Gate 事件缺失",
		Grade:          0,
		Reason:         translateGateReason("GATE_MISSING"),
		ReasonCode:     "GATE_MISSING",
		Direction:      translateDirection("none"),
		ExpectedSnapID: snapshotID,
	}
	return GateReport{
		Overall: overall,
	}
}

func (f DefaultFormatter) BuildDecisionReport(input DecisionInput) (DecisionReport, error) {
	report := DecisionReport{
		Symbol:     input.Symbol,
		SnapshotID: input.SnapshotID,
	}
	if hasGateEvent(input.Gate) {
		gateReport, err := f.BuildGateReport(input.Gate)
		if err != nil {
			return DecisionReport{}, err
		}
		report.Gate = gateReport
	} else {
		report.Gate = f.BuildMissingGateReport(input.SnapshotID)
	}
	providers, err := f.buildProviderOutputs(input.Providers, input.SnapshotID)
	if err != nil {
		return report, err
	}
	agents, err := f.buildAgentOutputs(input.Agents, input.SnapshotID)
	if err != nil {
		return report, err
	}
	report.Providers = providers
	report.Agents = agents
	return report, nil
}

func hasGateEvent(gate GateEvent) bool {
	if gate.ID > 0 || gate.SnapshotID > 0 {
		return true
	}
	if strings.TrimSpace(gate.DecisionAction) != "" {
		return true
	}
	if strings.TrimSpace(gate.GateReason) != "" {
		return true
	}
	return strings.TrimSpace(gate.Direction) != ""
}

func (f DefaultFormatter) HumanizeLLMOutput(raw json.RawMessage) (any, any, error) {
	if len(raw) == 0 {
		return nil, nil, nil
	}
	var data any
	if err := json.Unmarshal(raw, &data); err != nil {
		return json.RawMessage(raw), json.RawMessage(raw), err
	}
	summary := formatLLMSummary(data)
	if summary == "" {
		return data, data, nil
	}
	return summary, data, nil
}

func (f DefaultFormatter) buildProviderOutputs(records []ProviderEvent, snapshotID uint) ([]StageOutput, error) {
	var errList []error
	outputs := make([]StageOutput, 0, len(records))
	for _, rec := range records {
		if snapshotID > 0 && rec.SnapshotID != snapshotID {
			continue
		}
		summary, _, err := f.HumanizeLLMOutput(json.RawMessage(rec.OutputJSON))
		if err != nil {
			errList = append(errList, err)
			continue
		}
		outputs = append(outputs, StageOutput{
			Role:    normalizeStageKey(rec.Role),
			Summary: normalizeSummary(summary),
		})
	}
	if len(errList) > 0 {
		return outputs, fmt.Errorf("llm output decode failed: %d", len(errList))
	}
	return outputs, nil
}

func (f DefaultFormatter) buildAgentOutputs(records []AgentEvent, snapshotID uint) ([]StageOutput, error) {
	var errList []error
	outputs := make([]StageOutput, 0, len(records))
	for _, rec := range records {
		if snapshotID > 0 && rec.SnapshotID != snapshotID {
			continue
		}
		summary, _, err := f.HumanizeLLMOutput(json.RawMessage(rec.OutputJSON))
		if err != nil {
			errList = append(errList, err)
			continue
		}
		outputs = append(outputs, StageOutput{
			Role:    normalizeStageKey(rec.Stage),
			Summary: normalizeSummary(summary),
		})
	}
	if len(errList) > 0 {
		return outputs, fmt.Errorf("llm output decode failed: %d", len(errList))
	}
	return outputs, nil
}
