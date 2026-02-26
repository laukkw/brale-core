package decisionfmt

import (
	"fmt"
	"strings"
)

type executionSummary struct {
	Action           string
	Evaluated        bool
	Eligible         bool
	Executed         bool
	BlockedBy        []string
	ScoreTotal       float64
	ScoreThreshold   float64
	ScoreParseOK     bool
	ATRChangePct     float64
	ATRThreshold     float64
	ATRChangePctOK   bool
	MonitorGateHit   bool
	DebounceSec      float64
	DebounceRemain   float64
	NewsGateDecision string
	NewsGateReasonZH string
}

func ResolveExecutionTitle(report DecisionReport) string {
	return resolveExecutionTitle(report)
}

func resolveExecutionTitle(report DecisionReport) string {
	exec := parseExecutionSummary(report.Gate.Derived)
	if exec == nil || strings.TrimSpace(exec.Action) == "" {
		return ""
	}
	if !strings.EqualFold(exec.Action, "tighten") {
		return ""
	}
	if exec.Executed {
		return "执行收紧风控"
	}
	if blocked := formatExecutionBlockedReasons(exec.BlockedBy); blocked != "" {
		return fmt.Sprintf("继续持仓（收紧未执行：%s）", blocked)
	}
	if exec.Evaluated {
		return "继续持仓（收紧未触发）"
	}
	return "继续持仓"
}

func parseExecutionSummary(derived map[string]any) *executionSummary {
	if len(derived) == 0 {
		return nil
	}
	raw, ok := derived["execution"].(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	exec := &executionSummary{
		Action: parseStringValue(raw["action"]),
	}
	exec.Evaluated = parseBoolValue(raw["evaluated"])
	exec.Eligible = parseBoolValue(raw["eligible"])
	exec.Executed = parseBoolValue(raw["executed"])
	exec.BlockedBy = parseStringList(raw["blocked_by"])
	if gate := parseMapValue(raw["gate"]); gate != nil {
		exec.MonitorGateHit = parseBoolValue(gate["monitor_gate_hit"])
		exec.DebounceSec = parseFloatValue(gate["debounce_sec"])
		exec.DebounceRemain = parseFloatValue(gate["debounce_remaining_sec"])
		exec.ATRChangePct = parseFloatValue(gate["atr_change_pct"])
		exec.ATRThreshold = parseFloatValue(gate["atr_threshold"])
		exec.ATRChangePctOK = parseBoolValue(gate["atr_change_pct_ok"])
	}
	if score := parseMapValue(raw["score"]); score != nil {
		exec.ScoreTotal = parseFloatValue(score["total"])
		exec.ScoreThreshold = parseFloatValue(score["threshold"])
		exec.ScoreParseOK = parseBoolValue(score["parse_ok"])
	}
	if newsGate := parseMapValue(raw["news_gate"]); newsGate != nil {
		exec.NewsGateDecision = parseStringValue(newsGate["decision"])
		exec.NewsGateReasonZH = parseStringValue(newsGate["reason_zh"])
	}
	return exec
}

func formatExecutionSummary(exec executionSummary) string {
	if exec.Executed {
		return "执行收紧风控"
	}
	if blocked := formatExecutionBlockedReasons(exec.BlockedBy); blocked != "" {
		return fmt.Sprintf("继续持仓（收紧未执行：%s）", blocked)
	}
	if exec.Evaluated {
		return "继续持仓（收紧未触发）"
	}
	return "继续持仓（收紧未评估）"
}

func formatExecutionBlockedReasons(reasons []string) string {
	if len(reasons) == 0 {
		return ""
	}
	parts := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		label := translateExecutionBlockedReason(reason)
		if strings.TrimSpace(label) == "" {
			label = reason
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, " / ")
}

func translateExecutionBlockedReason(reason string) string {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "monitor_gate":
		return "收紧监控门槛未满足"
	case "atr_missing":
		return "ATR 数据缺失"
	case "atr_gate":
		return "ATR 门槛未满足"
	case "atr_value_missing":
		return "ATR 数值缺失"
	case "score_threshold":
		return "评分未达标"
	case "score_parse":
		return "评分解析失败"
	case "risk_plan_missing":
		return "风控计划缺失"
	case "risk_plan_disabled":
		return "风控更新未启用"
	case "price_unavailable":
		return "价格不可用"
	case "price_source_missing":
		return "价格源缺失"
	case "binding_missing":
		return "策略绑定缺失"
	case "no_tighten_needed":
		return "执行阶段未发现更优止损（继续持仓）"
	case "not_evaluated":
		return "未完成评估"
	case "tighten_debounce":
		return "收紧更新冷却中"
	case "news_gate":
		return "舆情门槛未通过"
	default:
		return ""
	}
}

func formatExecutionBlockedStages(reasons []string) string {
	if len(reasons) == 0 {
		return ""
	}
	seen := make(map[string]struct{}, len(reasons))
	stages := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		stage := translateExecutionBlockedStage(reason)
		if strings.TrimSpace(stage) == "" {
			continue
		}
		if _, ok := seen[stage]; ok {
			continue
		}
		seen[stage] = struct{}{}
		stages = append(stages, stage)
	}
	if len(stages) == 0 {
		return ""
	}
	return strings.Join(stages, " / ")
}

func translateExecutionBlockedStage(reason string) string {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "monitor_gate", "atr_missing", "atr_gate", "score_threshold", "score_parse", "not_evaluated":
		return "收紧判定阶段"
	case "tighten_debounce":
		return "防抖阶段"
	case "news_gate":
		return "舆情门槛阶段"
	case "risk_plan_missing", "risk_plan_disabled", "price_unavailable", "price_source_missing", "binding_missing", "atr_value_missing":
		return "收紧执行准备阶段"
	case "no_tighten_needed":
		return "收紧执行阶段"
	default:
		return ""
	}
}

func resolveHoldStatusLine(report DecisionReport) (string, string, bool) {
	action := strings.ToUpper(strings.TrimSpace(report.Gate.Overall.DecisionAction))
	if !isHoldDecision(action) {
		return "", "", false
	}
	if action == "EXIT" {
		return "持仓状态", "立即平仓", true
	}
	if action == "TIGHTEN" {
		exec := parseExecutionSummary(report.Gate.Derived)
		if exec != nil && strings.EqualFold(exec.Action, "tighten") && exec.Executed {
			return "持仓状态", "执行收紧风控", true
		}
		return "持仓状态", "继续持仓", true
	}
	return "持仓状态", "继续持仓", true
}

func formatExecutionFloat(value float64) string {
	text := fmt.Sprintf("%.4f", value)
	text = strings.TrimRight(text, "0")
	text = strings.TrimRight(text, ".")
	if text == "" || text == "-0" {
		return "0"
	}
	return text
}
