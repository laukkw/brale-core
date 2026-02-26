package decisionfmt

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type boolPhrase struct {
	True  string
	False string
}

var boolPhraseMap = map[string]boolPhrase{
	"integrity":           {True: "目前结构完整", False: "目前结构不完整"},
	"clear_structure":     {True: "结构清晰", False: "结构不清晰"},
	"momentum_expansion":  {True: "动能扩张", False: "动能未扩张"},
	"alignment":           {True: "指标一致", False: "指标不一致"},
	"mean_rev_noise":      {True: "均值回归噪音偏高", False: "均值回归噪音不显著"},
	"liquidation_stress":  {True: "强平压力上升", False: "强平压力不显著"},
	"adverse_liquidation": {True: "存在反向清算风险", False: "暂无反向清算风险"},
	"crowding_reversal":   {True: "拥挤度触发反转", False: "拥挤度未触发反转"},
	"momentum_sustaining": {True: "动能仍在维持", False: "动能未维持"},
	"divergence_detected": {True: "出现反向背离", False: "未见反向背离"},
	"leverage":            {True: "存在杠杆堆积", False: "未见杠杆堆积"},
	"crowding":            {True: "拥挤度偏高", False: "拥挤度不高"},
	"liq_stress":          {True: "存在清算压力", False: "清算压力不显著"},
}

func formatLLMSummary(v any) string {
	m, ok := v.(map[string]any)
	if !ok || len(m) == 0 {
		return ""
	}
	if summary := buildNarrativeSummary(m); summary != "" {
		return summary
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	notes := ""
	for _, k := range keys {
		if strings.EqualFold(k, "notes") {
			if raw, ok := m[k].(string); ok {
				notes = strings.TrimSpace(raw)
			}
			continue
		}
		if strings.EqualFold(k, "threat_level") {
			if raw, ok := m[k].(string); ok && strings.TrimSpace(raw) == "" {
				continue
			}
		}
		label := translateLLMKey(k)
		if label != k {
			label = fmt.Sprintf("%s(%s)", label, k)
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", label, formatLLMValue(k, m[k])))
	}
	if strings.TrimSpace(notes) != "" {
		lines = append(lines, fmt.Sprintf("备注: %s", notes))
	}
	return strings.Join(lines, "\n")
}

func formatLLMValue(key string, v any) string {
	if strings.EqualFold(strings.TrimSpace(key), "liquidation_stress") {
		if text, ok := formatLiquidationStress(v); ok {
			return text
		}
	}
	switch val := v.(type) {
	case bool:
		if text, ok := formatBoolWithKey(key, val); ok {
			return text
		}
		if val {
			return "是"
		}
		return "否"
	case string:
		return translateTerm(val)
	case []string:
		return formatTextList(val)
	case []any:
		return formatAnyList(val)
	case float64, float32, int, int64, uint64, int32, uint32:
		return fmt.Sprintf("%v", val)
	default:
		out, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(out)
	}
}

func formatLiquidationStress(v any) (string, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		if raw, okString := v.(string); okString && strings.HasPrefix(strings.TrimSpace(raw), "{") {
			var parsed map[string]any
			if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
				m = parsed
				ok = true
			}
		}
	}
	if !ok || len(m) == 0 {
		return "", false
	}
	parts := make([]string, 0, 3)
	if raw, exists := m["value"]; exists {
		switch v := raw.(type) {
		case bool:
			if text, ok := formatBoolWithKey("liquidation_stress", v); ok {
				parts = append(parts, text)
			} else {
				parts = append(parts, fmt.Sprintf("结论:%s", translateBoolStatus(v)))
			}
		default:
			parts = append(parts, fmt.Sprintf("结论:%v", raw))
		}
	}
	if raw, exists := m["confidence"]; exists {
		switch v := raw.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				parts = append(parts, fmt.Sprintf("置信度:%s", formatConfidenceStatus(v)))
			}
		default:
			parts = append(parts, fmt.Sprintf("置信度:%v", raw))
		}
	}
	if raw, exists := m["reason"]; exists {
		switch v := raw.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				parts = append(parts, fmt.Sprintf("原因:%s", v))
			}
		default:
			parts = append(parts, fmt.Sprintf("原因:%v", raw))
		}
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, "; "), true
}

func formatBoolWithKey(key string, val bool) (string, bool) {
	phrase, ok := boolPhraseMap[strings.ToLower(strings.TrimSpace(key))]
	if !ok {
		return "", false
	}
	if val {
		return phrase.True, true
	}
	return phrase.False, true
}

func normalizeSummary(summary any) string {
	switch v := summary.(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

func formatTextList(values []string) string {
	parts := make([]string, 0, len(values))
	for _, item := range values {
		text := strings.TrimSpace(item)
		if text == "" {
			continue
		}
		parts = append(parts, translateTerm(text))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "、")
}

func formatAnyList(values []any) string {
	parts := make([]string, 0, len(values))
	for _, item := range values {
		switch v := item.(type) {
		case string:
			text := strings.TrimSpace(v)
			if text == "" {
				continue
			}
			parts = append(parts, translateTerm(text))
		case float64, float32, int, int64, uint64, int32, uint32:
			parts = append(parts, fmt.Sprintf("%v", v))
		case bool:
			parts = append(parts, translateBoolStatus(v))
		default:
			parts = append(parts, fmt.Sprintf("%v", v))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "、")
}
