package decisionfmt

import (
	"fmt"
	"strings"
)

func pickNarrativeSummary(report DecisionReport) string {
	if merged := mergeNarrativeSummary(report); strings.TrimSpace(merged) != "" {
		return merged
	}
	var fallback string
	for _, item := range append(report.Agents, report.Providers...) {
		summary := strings.TrimSpace(item.Summary)
		if summary == "" {
			continue
		}
		if fallback == "" {
			fallback = summary
		}
		if strings.Contains(summary, "状态：") || strings.Contains(summary, "动作：") || strings.Contains(summary, "冲突：") || strings.Contains(summary, "风险：") || strings.Contains(summary, "状态:") {
			return summary
		}
	}
	return fallback
}

func mergeNarrativeSummary(report DecisionReport) string {
	sections := map[string][]string{"状态": {}, "动作": {}, "冲突": {}, "风险": {}}
	seen := map[string]map[string]struct{}{"状态": {}, "动作": {}, "冲突": {}, "风险": {}}
	hasValue := false
	for _, stage := range orderedNarrativeStages(report) {
		summary := strings.TrimSpace(stage.Summary)
		if summary == "" {
			continue
		}
		if mergeNarrativeFromSummary(summary, sections, seen) {
			hasValue = true
		}
	}
	addStructureResolutionNote(sections, seen)
	if !hasValue {
		return ""
	}
	labels := []string{"状态", "动作", "冲突", "风险"}
	lines := make([]string, 0, len(labels))
	for _, label := range labels {
		values := sections[label]
		if len(values) == 0 {
			lines = append(lines, fmt.Sprintf("%s：—", label))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s：%s", label, strings.Join(values, "；")))
	}
	return strings.Join(lines, "\n")
}

func addStructureResolutionNote(sections map[string][]string, seen map[string]map[string]struct{}) {
	lastBreak := firstSectionValueWithPrefix(sections["动作"], "最近结构事件=")
	if lastBreak == "" {
		lastBreak = firstSectionValueWithPrefix(sections["动作"], "最近结构变化=")
	}
	if lastBreak == "" || !containsStructureInvalidation(sections) {
		return
	}
	note := fmt.Sprintf("结构复核=%s 未被确认，当前按%s处理", structureEventLabel(lastBreak), structureResolutionLabel(sections))
	if _, ok := seen["冲突"]; !ok {
		seen["冲突"] = map[string]struct{}{}
	}
	if _, exists := seen["冲突"][note]; exists {
		return
	}
	seen["冲突"][note] = struct{}{}
	sections["冲突"] = append(sections["冲突"], note)
}

func firstSectionValueWithPrefix(values []string, prefix string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if strings.HasPrefix(trimmed, prefix) {
			return trimmed
		}
	}
	return ""
}

func structureEventLabel(value string) string {
	trimmed := strings.TrimSpace(value)
	if after, ok := strings.CutPrefix(trimmed, "最近结构事件="); ok {
		trimmed = strings.TrimSpace(after)
	} else if after, ok := strings.CutPrefix(trimmed, "最近结构变化="); ok {
		trimmed = strings.TrimSpace(after)
	}
	if open := strings.Index(trimmed, "("); open >= 0 {
		if close := strings.LastIndex(trimmed, ")"); close > open {
			return strings.TrimSpace(trimmed[open+1 : close])
		}
	}
	switch trimmed {
	case "bos_up":
		return "向上 BOS"
	case "bos_down":
		return "向下 BOS"
	case "choch_up":
		return "向上 CHoCH"
	case "choch_down":
		return "向下 CHoCH"
	}
	return trimmed
}

func containsStructureInvalidation(sections map[string][]string) bool {
	for _, value := range sections["动作"] {
		if isStructureInvalidationValue(value) {
			return true
		}
	}
	for _, value := range sections["冲突"] {
		if isStructureInvalidationValue(value) {
			return true
		}
	}
	return false
}

func isStructureInvalidationValue(value string) bool {
	normalized := strings.TrimSpace(value)
	return strings.Contains(normalized, "fakeout_rejection") ||
		strings.Contains(normalized, "structure_broken") ||
		strings.Contains(normalized, "信号标签=假突破回落") ||
		strings.Contains(normalized, "信号标签=结构失效") ||
		strings.Contains(normalized, "结构不清晰") ||
		strings.Contains(normalized, "结构叙事已失效")
}

func structureResolutionLabel(sections map[string][]string) string {
	for _, value := range sections["动作"] {
		normalized := strings.TrimSpace(value)
		if strings.Contains(normalized, "fakeout_rejection") ||
			strings.Contains(normalized, "信号标签=假突破回落") {
			return "假突破回落"
		}
		if strings.Contains(normalized, "structure_broken") ||
			strings.Contains(normalized, "信号标签=结构失效") {
			return "结构失效"
		}
	}
	return "结构失效/无效"
}

func orderedNarrativeStages(report DecisionReport) []StageOutput {
	stages := append([]StageOutput{}, report.Agents...)
	stages = append(stages, report.Providers...)
	ordered := make([]StageOutput, 0, len(stages))
	used := make([]bool, len(stages))
	roleOrder := []string{"structure", "indicator", "mechanics"}
	for _, role := range roleOrder {
		for idx, stage := range stages {
			if used[idx] {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(stage.Role), role) {
				ordered = append(ordered, stage)
				used[idx] = true
			}
		}
	}
	for idx, stage := range stages {
		if used[idx] {
			continue
		}
		ordered = append(ordered, stage)
	}
	return ordered
}

func mergeNarrativeFromSummary(summary string, sections map[string][]string, seen map[string]map[string]struct{}) bool {
	lines := strings.Split(summary, "\n")
	hasValue := false
	for _, line := range lines {
		label, values, ok := parseNarrativeLine(line)
		if !ok || len(values) == 0 {
			continue
		}
		if _, ok := seen[label]; !ok {
			seen[label] = map[string]struct{}{}
		}
		for _, value := range values {
			trimmed := strings.TrimSpace(value)
			if isPlaceholderValue(trimmed) {
				continue
			}
			if _, exists := seen[label][trimmed]; exists {
				continue
			}
			seen[label][trimmed] = struct{}{}
			sections[label] = append(sections[label], trimmed)
			hasValue = true
		}
	}
	return hasValue
}

func parseNarrativeLine(line string) (string, []string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", nil, false
	}
	labels := []string{"状态", "动作", "冲突", "风险"}
	for _, label := range labels {
		if strings.HasPrefix(trimmed, label+"：") {
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, label+"："))
			return label, splitNarrativeValues(value), true
		}
		if strings.HasPrefix(trimmed, label+":") {
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, label+":"))
			return label, splitNarrativeValues(value), true
		}
	}
	return "", nil, false
}

func splitNarrativeValues(value string) []string {
	trimmed := strings.TrimSpace(value)
	if isPlaceholderValue(trimmed) {
		return nil
	}
	parts := strings.FieldsFunc(trimmed, func(r rune) bool { return r == '；' || r == ';' })
	if len(parts) == 0 {
		return nil
	}
	return parts
}

func splitNarrativeSections(summary string) map[string]string {
	sections := map[string][]string{"状态": {}, "动作": {}, "冲突": {}, "风险": {}}
	matched := false
	for _, line := range strings.Split(summary, "\n") {
		label, values, ok := parseNarrativeLine(line)
		if !ok {
			continue
		}
		matched = true
		for _, value := range values {
			trimmed := strings.TrimSpace(value)
			if isPlaceholderValue(trimmed) {
				continue
			}
			sections[label] = append(sections[label], trimmed)
		}
	}
	output := make(map[string]string, 4)
	if !matched {
		output["状态"] = strings.TrimSpace(summary)
		return output
	}
	for label, values := range sections {
		output[label] = strings.Join(values, "；")
	}
	return output
}

func isPlaceholderValue(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed == "" || trimmed == "—" || trimmed == "-"
}
