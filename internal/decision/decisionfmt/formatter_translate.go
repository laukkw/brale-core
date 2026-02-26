package decisionfmt

import (
	"fmt"
	"strings"
	"unicode"
)

func translateDecisionAction(action string) string {
	switch strings.ToUpper(strings.TrimSpace(action)) {
	case "ALLOW":
		return "允许"
	case "WAIT":
		return "观望"
	case "VETO":
		return "否决"
	case "EXIT":
		return "平仓"
	case "TIGHTEN":
		return "收紧风控"
	case "KEEP":
		return "保持"
	case "MISSING":
		return "未执行"
	case "":
		return ""
	default:
		return action
	}
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
	switch strings.ToUpper(strings.TrimSpace(action)) {
	case "EXIT":
		return "平仓"
	case "TIGHTEN":
		return "收紧风控"
	case "KEEP":
		return "保持"
	default:
		return ""
	}
}

func translateGateReason(reason string) string {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return "未知原因"
	}
	upper := strings.ToUpper(trimmed)
	switch {
	case upper == "PASS_STRONG":
		return "通过(强)"
	case upper == "PASS_WEAK":
		return "通过(弱)"
	case upper == "CONSENSUS_NOT_PASSED":
		return "三路共识未通过"
	case upper == "STRUCT_INVALID":
		return "结构无效"
	case upper == "STRUCT_NO_BIAS":
		return "结构无方向"
	case upper == "MECH_RISK":
		return "力学风险"
	case upper == "STRUCT_BREAK":
		return "结构破坏"
	case upper == "MOMENTUM_WEAK":
		return "动能走弱"
	case upper == "INDICATOR_NOISE":
		return "指标噪音"
	case upper == "INDICATOR_MIXED":
		return "指标分歧"
	case upper == "STRUCT_THREAT":
		return "结构威胁"
	case upper == "KEEP":
		return "保持"
	case upper == "TIGHTEN":
		return "收紧(减仓/收紧止损)"
	case upper == "ENTRY_COOLDOWN_ACTIVE":
		return "开仓冷却中"
	case upper == "GATE_MISSING":
		return "Gate 事件缺失"
	case upper == "BINDING_MISSING":
		return "策略绑定缺失"
	case upper == "ENABLED_MISSING":
		return "启用配置缺失"
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
	labels := map[string]string{
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
		"CROWD_ALIGN_LOW_BLOCK":        "同向拥挤/低置信/拦截",
		"CROWD_COUNTER_HIGH":           "反向拥挤/高置信",
		"CROWD_COUNTER_LOW":            "反向拥挤/低置信",
		"BLOCK_CROWD_LONG_ALIGN":       "同向拥挤(多)/拦截",
		"BLOCK_CROWD_SHORT_ALIGN":      "同向拥挤(空)/拦截",
		"BLOCK_LIQ_CASCADE":            "链式风险/拦截",
		"ALLOW_TREND_BREAKOUT_FUEL":    "趋势突破/燃料充分",
		"ALLOW_TREND_BREAKOUT_NEUTRAL": "趋势突破/中性",
		"ALLOW_PULLBACK_FUEL":          "回踩确认/燃料充分",
		"ALLOW_PULLBACK_NEUTRAL":       "回踩确认/中性",
		"ALLOW_DIV_REV_HIGH":           "背离反转/高置信",
	}
	if label, ok := labels[upper]; ok {
		return label
	}
	return translateGateReason(trimmed)
}

func translateDirection(dir string) string {
	switch strings.ToLower(strings.TrimSpace(dir)) {
	case "long":
		return "多头"
	case "short":
		return "空头"
	case "conflict":
		return "信号冲突"
	case "none", "":
		return "无方向"
	default:
		return fmt.Sprintf("%s(英文)", dir)
	}
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
	dict := map[string]string{
		// --- 市场环境/质量 ---
		"mixed":   "信号混杂/分歧",
		"messy":   "结构杂乱/噪音较多",
		"clean":   "结构清晰",
		"noise":   "噪音/无效信号",
		"unclear": "不明确/难判断",
		"neutral": "中性/无明显倾向",

		// --- 趋势/方向（描述状态，不给动作） ---
		"trend_up":   "上行趋势",
		"trend_down": "下行趋势",
		"range":      "区间震荡",
		"long":       "多头方向",
		"short":      "空头方向",

		// --- 结构信号（SMC）---
		"bos_up":             "向上结构突破(BOS)",
		"bos_down":           "向下结构突破(BOS)",
		"breakout_confirmed": "突破确认",
		"support_retest":     "支撑回踩确认",
		"fakeout_rejection":  "假突破回落",
		"choch_up":           "向上结构转变(CHoCH)",
		"choch_down":         "向下结构转变(CHoCH)",
		"structure_broken":   "结构失效/破坏",

		// --- 拥挤/风险 ---
		"long_crowded":        "多头拥挤(追高风险)",
		"short_crowded":       "空头拥挤(轧空风险)",
		"crowded_long":        "多头拥挤(追高风险)",
		"crowded_short":       "空头拥挤(轧空风险)",
		"balanced":            "多空均衡",
		"liquidation_cascade": "清算级联风险",
		"critical":            "危急(极高风险)",
		"stable":              "稳定",

		// --- 动能/波动状态 ---
		"steep":               "涨跌过快(乖离大)",
		"flat":                "动能走平/减弱",
		"expanding":           "波动/动能扩张",
		"contracting":         "波动/动能收敛",
		"aligned":             "指标一致",
		"divergent":           "指标分歧/不一致",
		"divergence_reversal": "指标背离(反转风险)",
		"momentum":            "动能",
		"momentum_weak":       "动能偏弱",
		"pullback_entry":      "回踩信号(等待确认)",

		// --- 价格形态（尽量中性命名）---
		"double_top":         "双顶形态",
		"double_bottom":      "双底形态",
		"head_shoulders":     "头肩形态",
		"inv_head_shoulders": "反头肩形态",
		"wedge_rising":       "上升楔形",
		"wedge_falling":      "下降楔形",
		"triangle_sym":       "对称三角形",
		"triangle_asc":       "上升三角形",
		"triangle_desc":      "下降三角形",
		"flag":               "旗形整理",
		"pennant":            "三角旗形",
		"channel_up":         "上行通道",
		"channel_down":       "下行通道",
		"compression":        "波动挤压/收敛",

		// --- 基础描述 ---
		"low":         "低",
		"medium":      "中",
		"high":        "高",
		"increasing":  "上升",
		"overheated":  "过热",
		"none":        "无",
		"ok":          "正常",
		"unknown":     "无法判断",
		"keep":        "保持(不调整)",
		"exit":        "退出/平仓",
		"tighten":     "收紧(减仓/收紧止损)",
		"risk_off":    "风险偏好下降(偏防守)",
		"trend_surge": "趋势加速(动能增强)",
		"fuel_ready":  "条件具备(可推进)",

		// --- monitor_tag（补齐 ruleflow 会用到的值）---
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
	key := strings.ToLower(trimmed)
	if v, ok := dict[key]; ok {
		return fmt.Sprintf("%s(%s)", raw, v)
	}
	if containsHan(trimmed) || strings.ContainsAny(trimmed, " \t\n") {
		return raw
	}
	return fmt.Sprintf("%s(英文)", raw)
}

func translateLLMKey(key string) string {
	dict := map[string]string{
		// 通用
		"confidence": "置信度",
		"reason":     "原因",
		"value":      "结论",
		"notes":      "备注",

		// provider / gate
		"tradeable":          "可交易",
		"signal_tag":         "信号标签",
		"monitor_tag":        "监控标签",
		"threat_level":       "威胁等级",
		"liquidation_stress": "强平压力",
		"liq_stress":         "清算压力",

		// agent: indicator
		"expansion":       "扩张状态",
		"alignment":       "指标一致性",
		"noise":           "噪音水平",
		"momentum_detail": "动能细节",
		"conflict_detail": "冲突细节",

		// agent: structure
		"regime":          "结构状态",
		"last_break":      "最近结构变化",
		"quality":         "结构质量",
		"pattern":         "主导形态",
		"volume_action":   "量能表现",
		"candle_reaction": "K线反应",

		// agent: mechanics
		"leverage_state":        "杠杆状态",
		"crowding":              "拥挤度",
		"risk_level":            "风险等级",
		"open_interest_context": "持仓量背景",
		"anomaly_detail":        "异常说明",

		// provider booleans
		"momentum_expansion":  "动能扩张",
		"mean_rev_noise":      "均值回归噪音",
		"clear_structure":     "结构清晰",
		"integrity":           "结构完整性",
		"momentum_sustaining": "动能维持",
		"divergence_detected": "背离",
		"adverse_liquidation": "反向清算风险",
		"crowding_reversal":   "拥挤反转",
	}
	if v, ok := dict[key]; ok {
		return v
	}
	return key
}

func providerRoleLabel(role string) string {
	switch strings.ToLower(role) {
	case "indicator":
		return "指标"
	case "structure":
		return "结构"
	case "mechanics":
		return "力学/风险"
	default:
		return strings.TrimSpace(role)
	}
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
