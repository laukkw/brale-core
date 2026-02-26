package decisionfmt

import (
	"encoding/json"
	"fmt"
	"strings"

	"brale-core/internal/decision/gate"
)

type indicatorProviderOut struct {
	MomentumExpansion bool `json:"momentum_expansion"`
	Alignment         bool `json:"alignment"`
	MeanRevNoise      bool `json:"mean_rev_noise"`
}

type structureProviderOut struct {
	ClearStructure bool   `json:"clear_structure"`
	Integrity      bool   `json:"integrity"`
	Reason         string `json:"reason"`
}

type semanticSignal struct {
	Value      bool   `json:"value"`
	Confidence string `json:"confidence"`
	Reason     string `json:"reason"`
}

type mechanicsProviderOut struct {
	LiquidationStress semanticSignal `json:"liquidation_stress"`
}

type gateRefs struct {
	Indicator indicatorProviderOut `json:"indicator"`
	Structure structureProviderOut `json:"structure"`
	Mechanics mechanicsProviderOut `json:"mechanics"`
}

func parseGateRefs(data []byte) (gateRefs, error) {
	var refs gateRefs
	if len(data) == 0 {
		return refs, nil
	}
	if err := json.Unmarshal(data, &refs); err != nil {
		return gateRefs{}, err
	}
	return refs, nil
}

func parseGateRuleHit(data []byte) (*GateRuleHit, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var hit GateRuleHit
	if err := json.Unmarshal(data, &hit); err != nil {
		return nil, err
	}
	if strings.TrimSpace(hit.Name) == "" {
		return nil, nil
	}
	return &hit, nil
}

func parseDerived(data []byte) (map[string]any, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var derived map[string]any
	if err := json.Unmarshal(data, &derived); err != nil {
		return nil, err
	}
	if len(derived) == 0 {
		return nil, nil
	}
	return derived, nil
}

func formatDerivedSummary(data map[string]any) string {
	if len(data) == 0 {
		return ""
	}
	parts := make([]string, 0, 3)
	if val, ok := lookupBool(data, "indicator.tradeable"); ok {
		parts = append(parts, fmt.Sprintf("indicator=%s", translateBoolStatus(val)))
	}
	if val, ok := lookupBool(data, "structure.tradeable"); ok {
		parts = append(parts, fmt.Sprintf("structure=%s", translateBoolStatus(val)))
	}
	if val, ok := lookupBool(data, "mechanics.tradeable"); ok {
		parts = append(parts, fmt.Sprintf("mechanics=%s", translateBoolStatus(val)))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ", ")
}

func formatGateTrace(data map[string]any) string {
	if len(data) == 0 {
		return ""
	}
	stepsRaw, ok := data["gate_trace"].([]any)
	if !ok || len(stepsRaw) == 0 {
		return formatSieveSuffix("", data)
	}
	mode := strings.ToLower(strings.TrimSpace(fmt.Sprint(data["gate_trace_mode"])))
	if mode == "monitor" {
		return formatMonitorTrace(stepsRaw, data)
	}
	parts := make([]string, 0, len(stepsRaw))
	for _, raw := range stepsRaw {
		stepMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		stepKey := strings.TrimSpace(fmt.Sprint(stepMap["step"]))
		if stepKey == "" || stepKey == "<nil>" {
			continue
		}
		stepLabel := translateGateStep(stepKey)
		okVal := false
		switch v := stepMap["ok"].(type) {
		case bool:
			okVal = v
		case float64:
			okVal = v != 0
		case string:
			okVal = strings.ToLower(strings.TrimSpace(v)) == "true"
		}
		if okVal {
			parts = append(parts, fmt.Sprintf("%s=通过", stepLabel))
			continue
		}
		reasonKey := strings.TrimSpace(fmt.Sprint(stepMap["reason"]))
		reasonLabel := translateGateReasonCode(reasonKey)
		if reasonLabel != "" {
			parts = append(parts, fmt.Sprintf("%s=停止(%s)", stepLabel, reasonLabel))
		} else if reasonKey != "" && reasonKey != "<nil>" {
			parts = append(parts, fmt.Sprintf("%s=停止(%s)", stepLabel, reasonKey))
		} else {
			parts = append(parts, fmt.Sprintf("%s=停止", stepLabel))
		}
		break
	}
	trace := strings.Join(parts, " -> ")
	return formatSieveSuffix(trace, data)
}

func formatMonitorTrace(stepsRaw []any, data map[string]any) string {
	parts := make([]string, 0, len(stepsRaw))
	for _, raw := range stepsRaw {
		stepMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		stepKey := strings.TrimSpace(fmt.Sprint(stepMap["step"]))
		if stepKey == "" || stepKey == "<nil>" {
			continue
		}
		stepLabel := translateGateStep(stepKey)
		if strings.TrimSpace(stepLabel) == "" {
			stepLabel = stepKey
		}
		tag := strings.TrimSpace(fmt.Sprint(stepMap["tag"]))
		if tag == "<nil>" {
			tag = ""
		}
		tagLabel := strings.TrimSpace(translateDecisionAction(tag))
		if tagLabel == "" {
			tagLabel = tag
		}
		reason := strings.TrimSpace(fmt.Sprint(stepMap["reason"]))
		if reason == "<nil>" {
			reason = ""
		}
		entry := fmt.Sprintf("%s=%s", stepLabel, tagLabel)
		if reason != "" {
			entry = fmt.Sprintf("%s(%s)", entry, reason)
		}
		parts = append(parts, entry)
	}
	trace := strings.Join(parts, "；")
	return formatSieveSuffix(trace, data)
}

func formatSieveSuffix(trace string, data map[string]any) string {
	action := strings.TrimSpace(fmt.Sprint(data["sieve_action"]))
	reason := strings.TrimSpace(fmt.Sprint(data["sieve_reason"]))
	if action == "<nil>" {
		action = ""
	}
	if reason == "<nil>" {
		reason = ""
	}
	action = strings.ToUpper(action)
	if action == "" && reason == "" {
		return trace
	}
	label := "Sieve"
	parts := make([]string, 0, 2)
	if action != "" {
		actionLabel := translateDecisionAction(action)
		if strings.TrimSpace(actionLabel) == "" {
			actionLabel = action
		}
		parts = append(parts, fmt.Sprintf("action=%s", actionLabel))
	}
	if reason != "" {
		reasonLabel := translateSieveReasonCode(reason)
		parts = append(parts, fmt.Sprintf("reason=%s", reasonLabel))
	}
	suffix := fmt.Sprintf("%s(%s)", label, strings.Join(parts, ", "))
	if trace == "" {
		return suffix
	}
	return fmt.Sprintf("%s -> %s", trace, suffix)
}

func translateGateStep(step string) string {
	step = strings.ToLower(strings.TrimSpace(step))
	if step == "" {
		return ""
	}
	labels := map[string]string{
		"direction":       "方向",
		"data":            "数据完整性",
		"structure":       "结构完整性",
		"mech_risk":       "力学风险",
		"indicator_noise": "指标噪音",
		"structure_clear": "结构清晰度",
		"tag_consistency": "标签一致性",
		"script_select":   "脚本选择",
		"script_allowed":  "脚本条件",
		"gate_allow":      "Gate 放行",
		"indicator":       "指标",
		"mechanics":       "力学",
	}
	if label, ok := labels[step]; ok {
		return label
	}
	return step
}

func translateGateReasonCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	labels := map[string]string{
		"DIRECTION_MISSING":    "方向缺失",
		"CONSENSUS_NOT_PASSED": "三路共识未通过",
		"DATA_MISSING":         "数据不足",
		"STRUCT_BREAK":         "结构破坏",
		"MECH_RISK":            "力学风险",
		"INDICATOR_NOISE":      "指标噪音",
		"INDICATOR_MIXED":      "指标混乱",
		"PASS_STRONG":          "强通过",
		"SIEVE_POLICY":         "Sieve 策略",
		"GATE_MISSING":         "Gate 事件缺失",
	}
	if label, ok := labels[code]; ok {
		return label
	}
	return ""
}

func displayGateReasonCode(code string) string {
	trimmed := strings.TrimSpace(code)
	if trimmed == "" {
		return ""
	}
	if label := translateGateReasonCode(trimmed); strings.TrimSpace(label) != "" {
		return label
	}
	if label := translateGateReason(trimmed); strings.TrimSpace(label) != "" {
		return label
	}
	return trimmed
}

func displayGateStep(step string) string {
	trimmed := strings.TrimSpace(step)
	if trimmed == "" {
		return ""
	}
	if label := translateGateStep(trimmed); strings.TrimSpace(label) != "" {
		return label
	}
	return trimmed
}

func lookupBool(data map[string]any, path string) (bool, bool) {
	if len(data) == 0 {
		return false, false
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return false, false
	}
	parts := strings.Split(path, ".")
	var current any = data
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return false, false
		}
		obj, ok := current.(map[string]any)
		if !ok {
			return false, false
		}
		val, ok := obj[part]
		if !ok {
			return false, false
		}
		current = val
	}
	val, ok := current.(bool)
	return val, ok
}

func buildGateProviders(refs gateRefs) []GateProviderStatus {
	indicatorTradeable := gate.IndicatorTradeable(gate.IndicatorAtomic{
		MomentumExpansion: refs.Indicator.MomentumExpansion,
		Alignment:         refs.Indicator.Alignment,
		MeanRevNoise:      refs.Indicator.MeanRevNoise,
	})
	structureTradeable := gate.StructureTradeable(gate.StructureAtomic{
		ClearStructure: refs.Structure.ClearStructure,
		Integrity:      refs.Structure.Integrity,
	})
	mechanicsTradeable := gate.MechanicsTradeable(gate.MechanicsAtomic{
		LiquidationStress: refs.Mechanics.LiquidationStress.Value,
	})
	out := []GateProviderStatus{
		{
			Role:          "indicator",
			Tradeable:     indicatorTradeable,
			TradeableText: translateBoolAction(indicatorTradeable),
			Factors: []GateFactor{
				{Key: "momentum_expansion", Label: translateLLMKey("momentum_expansion"), Status: translateBoolStatus(refs.Indicator.MomentumExpansion), Raw: refs.Indicator.MomentumExpansion},
				{Key: "alignment", Label: translateLLMKey("alignment"), Status: translateBoolStatus(refs.Indicator.Alignment), Raw: refs.Indicator.Alignment},
				{Key: "mean_rev_noise", Label: translateLLMKey("mean_rev_noise"), Status: translateBoolStatus(refs.Indicator.MeanRevNoise), Raw: refs.Indicator.MeanRevNoise},
			},
		},
		{
			Role:          "structure",
			Tradeable:     structureTradeable,
			TradeableText: translateBoolAction(structureTradeable),
			Factors: []GateFactor{
				{Key: "clear_structure", Label: translateLLMKey("clear_structure"), Status: translateBoolStatus(refs.Structure.ClearStructure), Raw: refs.Structure.ClearStructure},
				{Key: "integrity", Label: translateLLMKey("integrity"), Status: translateBoolStatus(refs.Structure.Integrity), Raw: refs.Structure.Integrity},
				{Key: "reason", Label: translateLLMKey("reason"), Status: formatTextStatus(refs.Structure.Reason), Raw: refs.Structure.Reason},
			},
		},
		{
			Role:          "mechanics",
			Tradeable:     mechanicsTradeable,
			TradeableText: translateBoolAction(mechanicsTradeable),
			Factors: []GateFactor{
				{Key: "liquidation_stress", Label: translateLLMKey("liquidation_stress"), Status: translateBoolStatus(refs.Mechanics.LiquidationStress.Value), Raw: refs.Mechanics.LiquidationStress.Value},
				{Key: "confidence", Label: translateLLMKey("confidence"), Status: formatConfidenceStatus(refs.Mechanics.LiquidationStress.Confidence), Raw: refs.Mechanics.LiquidationStress.Confidence},
				{Key: "reason", Label: translateLLMKey("reason"), Status: formatTextStatus(refs.Mechanics.LiquidationStress.Reason), Raw: refs.Mechanics.LiquidationStress.Reason},
			},
		},
	}
	return out
}
