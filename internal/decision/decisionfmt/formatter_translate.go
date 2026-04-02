package decisionfmt

import (
	"fmt"
	"strings"
	"unicode"
)

var decisionActionLabels = map[string]string{
	"ALLOW":   "允许",
	"WAIT":    "观望",
	"VETO":    "否决",
	"EXIT":    "平仓",
	"TIGHTEN": "收紧风控",
	"KEEP":    "保持",
	"MISSING": "未执行",
	"":        "",
}

var gateReasonLabels = map[string]string{
	"PASS_STRONG":           "强通过",
	"PASS_WEAK":             "弱通过",
	"CONSENSUS_NOT_PASSED":  "三路共识未通过",
	"STRUCT_INVALID":        "结构无效",
	"STRUCT_NO_BIAS":        "结构无方向",
	"MECH_RISK":             "清算风险过高",
	"STRUCT_BREAK":          "结构失效",
	"MOMENTUM_WEAK":         "动能走弱",
	"INDICATOR_NOISE":       "指标噪音",
	"INDICATOR_MIXED":       "指标分歧",
	"STRUCT_THREAT":         "结构受威胁",
	"KEEP":                  "保持",
	"TIGHTEN":               "收紧(减仓/收紧止损)",
	"ENTRY_COOLDOWN_ACTIVE": "开仓冷却中",
	"GATE_MISSING":          "Gate 事件缺失",
	"BINDING_MISSING":       "策略绑定缺失",
	"ENABLED_MISSING":       "启用配置缺失",
}

var sieveReasonLabels = map[string]string{
	"SIEVE_MATCH":                  "Sieve 命中",
	"SIEVE_DEFAULT":                "Sieve 默认",
	"FUEL_HIGH":                    "燃料充分/高置信",
	"FUEL_LOW":                     "燃料充分/低置信",
	"FUEL_HIGH_ALIGN":              "燃料充分/高置信/同向拥挤",
	"FUEL_LOW_ALIGN":               "燃料充分/低置信/同向拥挤",
	"NEUTRAL_HIGH":                 "中性/高置信",
	"NEUTRAL_LOW":                  "中性/低置信",
	"NEUTRAL_HIGH_ALIGN":           "中性/高置信/同向拥挤",
	"NEUTRAL_LOW_ALIGN":            "中性/低置信/同向拥挤",
	"CROWD_ALIGN_HIGH":             "同向拥挤/高置信",
	"CROWD_ALIGN_LOW":              "同向拥挤/低置信",
	"CROWD_ALIGN_LOW_BLOCK":        "同向拥挤/低置信/拦截",
	"CROWD_COUNTER_HIGH":           "反向拥挤/高置信",
	"CROWD_COUNTER_LOW":            "反向拥挤/低置信",
	"CROWD_LONG_ALIGN_HIGH":        "多头同向拥挤/高置信",
	"CROWD_LONG_ALIGN_LOW":         "多头同向拥挤/低置信",
	"CROWD_SHORT_ALIGN_HIGH":       "空头同向拥挤/高置信",
	"CROWD_SHORT_ALIGN_LOW":        "空头同向拥挤/低置信",
	"CROWD_LONG_COUNTER_HIGH":      "多头反向拥挤/高置信",
	"CROWD_LONG_COUNTER_LOW":       "多头反向拥挤/低置信",
	"CROWD_SHORT_COUNTER_HIGH":     "空头反向拥挤/高置信",
	"CROWD_SHORT_COUNTER_LOW":      "空头反向拥挤/低置信",
	"BLOCK_CROWD_LONG_ALIGN":       "同向拥挤(多)/拦截",
	"BLOCK_CROWD_SHORT_ALIGN":      "同向拥挤(空)/拦截",
	"BLOCK_LIQ_CASCADE":            "链式风险/拦截",
	"ALLOW_TREND_BREAKOUT_FUEL":    "趋势突破/燃料充分",
	"ALLOW_TREND_BREAKOUT_NEUTRAL": "趋势突破/中性",
	"ALLOW_PULLBACK_FUEL":          "回踩确认/燃料充分",
	"ALLOW_PULLBACK_NEUTRAL":       "回踩确认/中性",
	"ALLOW_DIV_REV_HIGH":           "背离反转/高置信",
}

var directionLabels = map[string]string{
	"long":     "多头",
	"short":    "空头",
	"conflict": "信号冲突",
	"none":     "无方向",
	"":         "无方向",
}

var translatedTerms = map[string]string{
	"mixed":                "信号混杂/分歧",
	"messy":                "结构杂乱/噪音较多",
	"clean":                "结构清晰",
	"noise":                "噪音/无效信号",
	"unclear":              "不明确/难判断",
	"neutral":              "中性/无明显倾向",
	"trend_up":             "上行趋势",
	"trend_down":           "下行趋势",
	"range":                "区间震荡",
	"long":                 "多头方向",
	"short":                "空头方向",
	"bos_up":               "向上结构突破(BOS)",
	"bos_down":             "向下结构突破(BOS)",
	"breakout_confirmed":   "突破确认",
	"support_retest":       "回踩确认",
	"fakeout_rejection":    "假突破回落",
	"choch_up":             "向上结构转变(CHoCH)",
	"choch_down":           "向下结构转变(CHoCH)",
	"structure_broken":     "结构失效",
	"long_crowded":         "多头拥挤",
	"short_crowded":        "空头拥挤",
	"crowded_long":         "多头拥挤",
	"crowded_short":        "空头拥挤",
	"balanced":             "多空均衡",
	"liquidation_cascade":  "连环清算风险",
	"critical":             "危急(极高风险)",
	"stable":               "稳定",
	"steep":                "涨跌过快(乖离大)",
	"flat":                 "动能走平/减弱",
	"expanding":            "波动/动能扩张",
	"contracting":          "波动/动能收敛",
	"aligned":              "指标一致",
	"divergent":            "指标分歧/不一致",
	"divergence_reversal":  "背离反转风险",
	"momentum":             "动能",
	"momentum_weak":        "动能偏弱",
	"pullback_entry":       "回踩入场",
	"double_top":           "双顶形态",
	"double_bottom":        "双底形态",
	"head_shoulders":       "头肩形态",
	"inv_head_shoulders":   "反头肩形态",
	"wedge_rising":         "上升楔形",
	"wedge_falling":        "下降楔形",
	"triangle_sym":         "对称三角形",
	"triangle_asc":         "上升三角形",
	"triangle_desc":        "下降三角形",
	"flag":                 "旗形整理",
	"pennant":              "三角旗形",
	"channel_up":           "上行通道",
	"channel_down":         "下行通道",
	"compression":          "波动挤压/收敛",
	"low":                  "低",
	"medium":               "中",
	"high":                 "高",
	"increasing":           "杠杆升温",
	"overheated":           "过热",
	"none":                 "无",
	"ok":                   "正常",
	"unknown":              "无法判断",
	"keep":                 "保持(不调整)",
	"exit":                 "退出/平仓",
	"tighten":              "收紧(减仓/收紧止损)",
	"risk_off":             "风险偏好下降(偏防守)",
	"trend_surge":          "趋势加速",
	"fuel_ready":           "条件具备",
	"reversal_confirmed":   "反转确认",
	"reversal_divergence":  "背离反转信号",
	"structure_threatened": "结构受威胁/临近失效",
	"support_test":         "支撑测试",
	"crowding_limit":       "拥挤度触发阈值",
	"momentum_stalling":    "动能停滞",
	"volatility_drop":      "波动下降",
	"volatility_squeeze":   "波动挤压",
	"dead_fish":            "低动能低波动",
	"exit_confirm_pending": "退出确认中",
}

var llmKeyLabels = map[string]string{
	"confidence":            "置信度",
	"reason":                "原因",
	"value":                 "结论",
	"notes":                 "备注",
	"tradeable":             "可交易",
	"signal_tag":            "信号标签",
	"monitor_tag":           "监控标签",
	"threat_level":          "威胁等级",
	"liquidation_stress":    "清算压力",
	"liq_stress":            "清算压力",
	"expansion":             "扩张状态",
	"alignment":             "指标一致性",
	"noise":                 "噪音水平",
	"momentum_detail":       "动能细节",
	"conflict_detail":       "冲突细节",
	"regime":                "结构状态",
	"last_break":            "最近结构变化",
	"quality":               "结构质量",
	"pattern":               "主导形态",
	"volume_action":         "量能表现",
	"candle_reaction":       "K线反应",
	"leverage_state":        "杠杆状态",
	"crowding":              "拥挤度",
	"risk_level":            "风险等级",
	"open_interest_context": "持仓量背景",
	"anomaly_detail":        "异常说明",
	"momentum_expansion":    "动能扩张",
	"mean_rev_noise":        "均值回归噪音",
	"clear_structure":       "结构清晰",
	"integrity":             "结构完整性",
	"momentum_sustaining":   "动能维持",
	"divergence_detected":   "背离",
	"adverse_liquidation":   "反向清算风险",
	"crowding_reversal":     "拥挤反转",
}

var providerRoleLabels = map[string]string{
	"indicator": "指标",
	"structure": "结构",
	"mechanics": "市场机制",
}

func translateDecisionAction(action string) string {
	upper := strings.ToUpper(strings.TrimSpace(action))
	if label, ok := decisionActionLabels[upper]; ok {
		return label
	}
	return action
}

func GateDecisionText(action, reason string) string {
	if isHoldDecision(action) {
		return formatHoldAdvice(action, reason)
	}
	return translateDecisionAction(action)
}

func isHoldDecision(action string) bool {
	upper := strings.ToUpper(strings.TrimSpace(action))
	return upper == "EXIT" || upper == "TIGHTEN" || upper == "KEEP"
}

func formatHoldAdvice(action, reason string) string {
	act := translateHoldAction(action)
	reasonText := translateGateReason(reason)
	if strings.TrimSpace(reasonText) == "" {
		reasonText = reason
	}
	if strings.TrimSpace(act) == "" {
		act = action
	}
	return fmt.Sprintf("%s（原因：%s）", act, reasonText)
}

func translateHoldAction(action string) string {
	upper := strings.ToUpper(strings.TrimSpace(action))
	if upper == "EXIT" || upper == "TIGHTEN" || upper == "KEEP" {
		return translateDecisionAction(upper)
	}
	return ""
}

func translateGateReason(reason string) string {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return "未知原因"
	}
	upper := strings.ToUpper(trimmed)
	if label, ok := gateReasonLabels[upper]; ok {
		return label
	}
	switch {
	case strings.HasPrefix(upper, "AGENT_ERROR:"):
		return fmt.Sprintf("Agent 阶段异常：%s", strings.TrimPrefix(trimmed, "AGENT_ERROR:"))
	case strings.HasPrefix(upper, "PROVIDER_ERROR:"):
		return fmt.Sprintf("Provider 阶段异常：%s", strings.TrimPrefix(trimmed, "PROVIDER_ERROR:"))
	case strings.HasPrefix(upper, "GATE_ERROR:"):
		return fmt.Sprintf("Gate 阶段异常：%s", strings.TrimPrefix(trimmed, "GATE_ERROR:"))
	default:
		return fmt.Sprintf("%s(英文)", reason)
	}
}

func translateSieveReasonCode(code string) string {
	trimmed := strings.TrimSpace(code)
	if trimmed == "" {
		return ""
	}
	upper := strings.ToUpper(trimmed)
	if label, ok := sieveReasonLabels[upper]; ok {
		return label
	}
	return translateGateReason(trimmed)
}

func translateDirection(dir string) string {
	key := strings.ToLower(strings.TrimSpace(dir))
	if label, ok := directionLabels[key]; ok {
		return label
	}
	return fmt.Sprintf("%s(英文)", dir)
}

func translateBoolStatus(v bool) string {
	if v {
		return "是"
	}
	return "否"
}

func formatTextStatus(v string) string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return "—"
	}
	return trimmed
}

func formatConfidenceStatus(v string) string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return "—"
	}
	return translateTerm(trimmed)
}

func translateBoolAction(v bool) string {
	if v {
		return "🟢 可交易 (YES)"
	}
	return "🔴 不可交易 (NO)"
}

func translateTerm(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw
	}
	key := strings.ToLower(trimmed)
	if v, ok := translatedTerms[key]; ok {
		return fmt.Sprintf("%s(%s)", raw, v)
	}
	if containsHan(trimmed) || strings.ContainsAny(trimmed, " \t\n") {
		return raw
	}
	return fmt.Sprintf("%s(英文)", raw)
}

func translateLLMKey(key string) string {
	if v, ok := llmKeyLabels[key]; ok {
		return v
	}
	return key
}

func providerRoleLabel(role string) string {
	key := strings.ToLower(strings.TrimSpace(role))
	if label, ok := providerRoleLabels[key]; ok {
		return label
	}
	return strings.TrimSpace(role)
}

func normalizeStageKey(stage string) string {
	s := strings.ToLower(stage)
	switch {
	case strings.Contains(s, "provider"):
		return "provider"
	case strings.Contains(s, "indicator"):
		return "indicator"
	case strings.Contains(s, "structure"):
		return "structure"
	case strings.Contains(s, "mechanics"):
		return "mechanics"
	case strings.Contains(s, "gate"):
		return "gate"
	case strings.Contains(s, "exec"):
		return "execution"
	default:
		return s
	}
}

func containsHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}
