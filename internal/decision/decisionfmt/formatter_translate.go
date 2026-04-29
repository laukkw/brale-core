package decisionfmt

import (
	"fmt"
	"regexp"
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
	"PASS_STRONG":              "强通过",
	"PASS_WEAK":                "弱通过",
	"CONSENSUS_NOT_PASSED":     "三路共识未通过",
	"DIRECTION_UNCLEAR":        "方向不明确",
	"STRUCT_INVALID":           "结构无效",
	"STRUCT_NO_BIAS":           "结构无方向",
	"MECH_RISK":                "清算风险过高",
	"STRUCT_BREAK":             "结构失效",
	"STRUCT_HARD_INVALIDATION": "结构硬失效",
	"LIQUIDATION_CASCADE":      "连锁清算风险",
	"MOMENTUM_WEAK":            "动能走弱",
	"INDICATOR_NOISE":          "指标噪音",
	"INDICATOR_MIXED":          "指标分歧",
	"QUALITY_TOO_LOW":          "建仓质量不足",
	"EDGE_TOO_LOW":             "执行价值不足",
	"ALLOW":                    "允许",
	"STRUCT_THREAT":            "结构受威胁",
	"KEEP":                     "保持",
	"TIGHTEN":                  "收紧(减仓/收紧止损)",
	"ENTRY_COOLDOWN_ACTIVE":    "开仓冷却中",
	"GATE_MISSING":             "Gate 事件缺失",
	"BINDING_MISSING":          "策略绑定缺失",
	"ENABLED_MISSING":          "启用配置缺失",
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
	"bos_up":               "向上 BOS",
	"bos_down":             "向下 BOS",
	"breakout_confirmed":   "突破确认",
	"support_retest":       "回踩确认",
	"fakeout_rejection":    "假突破回落",
	"choch_up":             "向上 CHoCH",
	"choch_down":           "向下 CHoCH",
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
	"flat":                 "走平/减弱",
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
	// 指标引擎状态值 (indicator_state.go)
	"above":       "上方",
	"below":       "下方",
	"near":        "附近",
	"bull":        "多头排列",
	"bear":        "空头排列",
	"rising":      "上升",
	"falling":     "下降",
	"up":          "上行",
	"down":        "下行",
	"below_lower": "低于下轨",
	"near_lower":  "靠近下轨",
	"mid":         "中轨区间",
	"near_upper":  "靠近上轨",
	"above_upper": "突破上轨",
	"squeeze":     "挤压收窄",
	"normal":      "正常",
	"wide":        "宽幅",
	"trending":    "趋势行情",
	"choppy":      "震荡行情",
	"transition":  "过渡阶段",
	"oversold":    "超卖",
	"overbought":  "超买",
	"strong_up":   "强势上行",
	"strong_down": "强势下行",
	"crossover":   "交叉",
	"conflict":    "冲突/分歧",
	// 指标事件名
	"price_cross_ema_fast_up":   "价格上穿快线EMA",
	"price_cross_ema_fast_down": "价格下穿快线EMA",
	"price_cross_ema_mid_up":    "价格上穿中线EMA",
	"price_cross_ema_mid_down":  "价格下穿中线EMA",
	"ema_stack_bull_flip":       "EMA转为多头排列",
	"ema_stack_bear_flip":       "EMA转为空头排列",
	"aroon_strong_bullish":      "阿隆指标强势看多",
	"aroon_strong_bearish":      "阿隆指标强势看空",
	// OI-价格关系 (mechanics_state.go classifyOIPriceRelation)
	"price_up_oi_up":     "价格上涨/OI上升",
	"price_up_oi_down":   "价格上涨/OI下降",
	"price_down_oi_up":   "价格下跌/OI上升",
	"price_down_oi_down": "价格下跌/OI下降",
	// 情绪状态 (mechanics_state.go classifySentiment)
	"fear":          "恐惧",
	"greed":         "贪婪",
	"extreme_greed": "极度贪婪",
	// 资金费率热度 / 清算压力 (mechanics_state.go)
	"hot":      "过热",
	"elevated": "偏高",
	// 机制冲突 (mechanics_state.go detectMechanicsConflicts)
	"crowding_long_but_liq_stress_high":  "多头拥挤但清算压力高",
	"crowding_short_but_liq_stress_high": "空头拥挤但清算压力高",
	"funding_long_but_oi_falling":        "资金费率偏多但OI下降",
	"funding_short_but_oi_rising":        "资金费率偏空但OI上升",
	// 结构突破事件类型 (trend_compress.go)
	"break_up":   "向上突破",
	"break_down": "向下突破",
	// SuperTrend 状态 / 情绪标签
	"bullish":      "看多",
	"bearish":      "看空",
	"Strong Long":  "强烈看多",
	"Strong Short": "强烈看空",
	"Long Bias":    "偏多",
	"Short Bias":   "偏空",
	"strong long":  "强烈看多",
	"strong short": "强烈看空",
	"long bias":    "偏多",
	"short bias":   "偏空",
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
	"last_break":            "最近结构事件",
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
	"integrity":             "结构叙事有效性",
	"momentum_sustaining":   "动能维持",
	"divergence_detected":   "背离",
	"adverse_liquidation":   "反向清算风险",
	"crowding_reversal":     "拥挤反转",
	// 事件相关
	"events": "事件",
	// 跨周期汇总字段
	"cross_tf_summary":    "跨周期汇总",
	"decision_tf_bias":    "决策周期偏向",
	"lower_tf_agreement":  "低周期一致性",
	"higher_tf_agreement": "高周期一致性",
	"conflict_count":      "冲突计数",
	// Agent 方向打分
	"movement_score":      "方向分数",
	"movement_confidence": "方向置信度",
	"bias":                "偏向",
	// 多周期结构
	"multi_tf":          "多周期数据",
	"decision_interval": "决策间隔",
	// 指标细分字段
	"rsi_zone":              "RSI区间",
	"rsi_slope_state":       "RSI斜率",
	"stc_state":             "STC状态",
	"obv_slope_state":       "OBV斜率",
	"stoch_rsi_zone":        "随机RSI区间",
	"atr_expand_state":      "ATR扩张状态",
	"atr_change_pct":        "ATR变化率",
	"bb_zone":               "布林带区间",
	"bb_width_state":        "布林带宽度",
	"chop_regime":           "震荡指数状态",
	"ema_stack":             "EMA排列",
	"ema_distance_fast_atr": "快线EMA距离(ATR)",
	"ema_distance_mid_atr":  "中线EMA距离(ATR)",
	"ema_distance_slow_atr": "慢线EMA距离(ATR)",
	"price_vs_ema_fast":     "价格vs快线EMA",
	"price_vs_ema_mid":      "价格vs中线EMA",
	"price_vs_ema_slow":     "价格vs慢线EMA",
	"freshness_sec":         "数据新鲜度(秒)",
	// 机制状态字段 (mechanics_state.go)
	"oi_state":           "持仓量状态",
	"funding_state":      "资金费率状态",
	"crowding_state":     "拥挤度状态",
	"liquidation_state":  "清算状态",
	"sentiment_state":    "市场情绪",
	"mechanics_conflict": "机制冲突",
	"oi_price_relation":  "OI-价格关系",
	"change_state":       "变化状态",
	"fear_greed":         "恐贪指数",
	"top_trader_bias":    "大户偏向",
	"reversal_risk":      "反转风险",
	"stress":             "清算压力",
	"heat":               "资金费率热度",
	"ls_ratio":           "多空比",
	"taker_ratio":        "主动买卖比",
	"oi_change_pct":      "OI变化率",
	"price_change_pct":   "价格变化率",
	// 趋势结构字段 (trend_compress.go)
	"vol_ratio":                  "成交量比率",
	"level_price":                "关键价位",
	"order_block":                "订单块(Order Block)",
	"fvg":                        "公允价值缺口(FVG)",
	"slope_state":                "斜率状态",
	"trend_slope":                "趋势斜率",
	"break_events":               "结构突破事件",
	"break_summary":              "突破汇总",
	"supertrend":                 "SuperTrend指标",
	"tag":                        "情绪标签",
	"taker_long_short_vol_ratio": "主买/主卖成交量比",
	// EMA 子字段（用于 momentum_detail 等自由文本）
	"ema_fast":  "快线EMA",
	"ema_mid":   "中线EMA",
	"ema_slow":  "慢线EMA",
	"delta_pct": "变化率",
}

var providerRoleLabels = map[string]string{
	"indicator": "指标",
	"structure": "结构",
	"mechanics": "市场机制",
}

// phraseTranslations maps multi-word English phrases to Chinese.
// Applied before word-level replacements, longest first.
var phraseTranslations = [][2]string{
	{"OI increased", "持仓量上升"},
	{"OI decreased", "持仓量下降"},
	{"OI declined", "持仓量回落"},
	{"OI stable", "持仓量稳定"},
	{"funding rate negative", "资金费率为负"},
	{"funding rate positive", "资金费率为正"},
	{"funding rate", "资金费率"},
	{"negative funding", "负资金费率"},
	{"open interest", "持仓量"},
	{"long crowding", "多头拥挤"},
	{"short crowding", "空头拥挤"},
	{"in 15m", "在15分钟内"},
	{"in 1h", "在1小时内"},
	{"in 4h", "在4小时内"},
	{"over 4h", "4小时以上"},
	{"over 1h", "1小时以上"},
	{"Strong Long", "强烈看多"},
	{"Strong Short", "强烈看空"},
	{"Long Bias", "偏多"},
	{"Short Bias", "偏空"},
}

// wordTranslations maps single English words to Chinese using word-boundary matching.
var wordTranslations = map[string]string{
	"stable":        "稳定",
	"medium":        "中",
	"low":           "低",
	"high":          "高",
	"mixed":         "混合",
	"positive":      "正向",
	"negative":      "负向",
	"bullish":       "看多",
	"bearish":       "看空",
	"neutral":       "中性",
	"overbought":    "超买",
	"oversold":      "超卖",
	"slightly":      "小幅",
	"significantly": "大幅",
	"versus":        "对比",
	"but":           "但",
	"and":           "且",
	"increased":     "上升",
	"decreased":     "下降",
	"declined":      "回落",
	"expanding":     "扩张",
	"contracting":   "收缩",
	"moderate":      "温和",
	"steep":         "陡峭",
}

// tdSequentialPattern matches td_buy_setup_N / td_sell_setup_N event keys.
var tdSequentialPattern = regexp.MustCompile(`\btd_(buy|sell)_setup_(\d+)\b`)

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

// TranslateValue translates a single enum/token value to Chinese.
// Unlike translateTerm, it returns Chinese-only output (no English suffix).
// Falls back to the original value if no translation exists.
func TranslateValue(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw
	}
	// Check action labels (uppercase)
	upper := strings.ToUpper(trimmed)
	if label, ok := decisionActionLabels[upper]; ok && label != "" {
		return label
	}
	// Check gate reason labels
	if label, ok := gateReasonLabels[upper]; ok {
		return label
	}
	// Check sieve reason labels
	if label, ok := sieveReasonLabels[upper]; ok {
		return label
	}
	// Check gate step labels (lowercase)
	lower := strings.ToLower(trimmed)
	if label := translateGateStep(lower); label != "" && label != lower {
		return label
	}
	// Check direction labels (lowercase)
	if label, ok := directionLabels[lower]; ok {
		return label
	}
	// Check translated terms (lowercase)
	if label, ok := translatedTerms[lower]; ok {
		return label
	}
	// Check original case in translatedTerms (for "Strong Long" etc.)
	if label, ok := translatedTerms[trimmed]; ok {
		return label
	}
	return trimmed
}

// TranslateSentence translates a sentence containing mixed English/Chinese text.
// It applies translations in safe order: event keys → TD Sequential → phrases → field=value → words.
// This is the Go equivalent of render.mjs mapSentence().
func TranslateSentence(text string) string {
	output := strings.TrimSpace(text)
	if output == "" {
		return "—"
	}

	// Step 1: Replace complete event keys (exact match on token boundary).
	for _, key := range eventKeyOrder {
		label := translatedTerms[key]
		if label == "" {
			continue
		}
		re := regexp.MustCompile(`(?i)(?:^|[^a-zA-Z0-9_])` + regexp.QuoteMeta(key) + `(?:[^a-zA-Z0-9_]|$)`)
		output = re.ReplaceAllStringFunc(output, func(match string) string {
			prefix := ""
			suffix := ""
			for _, r := range match {
				if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
					break
				}
				prefix += string(r)
			}
			runes := []rune(match)
			for i := len(runes) - 1; i >= 0; i-- {
				r := runes[i]
				if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
					break
				}
				suffix = string(r) + suffix
			}
			return prefix + label + suffix
		})
	}

	// Step 1.5: TD Sequential dynamic event keys.
	output = tdSequentialPattern.ReplaceAllStringFunc(output, func(match string) string {
		parts := tdSequentialPattern.FindStringSubmatch(match)
		if len(parts) < 3 {
			return match
		}
		side := "买入"
		if parts[1] == "sell" {
			side = "卖出"
		}
		return "TD" + side + "序列" + parts[2]
	})

	// Step 2: Replace multi-word phrases (longest first, already ordered).
	for _, pair := range phraseTranslations {
		output = strings.ReplaceAll(output, pair[0], pair[1])
	}

	// Step 3: Replace field=value and standalone field-name references.
	output = TranslateLLMFieldRefs(output)

	// Step 4: Replace standalone English words with word-boundary safety.
	for word, label := range wordTranslations {
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(word) + `\b`)
		output = re.ReplaceAllString(output, label)
	}

	return output
}

// eventKeyOrder lists compound event keys that must be translated as atomic units.
// These must be checked before any word-level translation to prevent partial replacement.
var eventKeyOrder = []string{
	"price_cross_ema_fast_up",
	"price_cross_ema_fast_down",
	"price_cross_ema_mid_up",
	"price_cross_ema_mid_down",
	"ema_stack_bull_flip",
	"ema_stack_bear_flip",
	"aroon_strong_bullish",
	"aroon_strong_bearish",
	// Compound mechanics conflicts (must match before individual words)
	"crowding_long_but_liq_stress_high",
	"crowding_short_but_liq_stress_high",
	"funding_long_but_oi_falling",
	"funding_short_but_oi_rising",
	// Compound OI-price relations
	"price_up_oi_up",
	"price_up_oi_down",
	"price_down_oi_up",
	"price_down_oi_down",
}

// TranslateExecutionBlockedReason translates execution blocked reason codes to Chinese.
func TranslateExecutionBlockedReason(reason string) string {
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
		return "执行阶段未发现更优止损"
	case "not_evaluated":
		return "未完成评估"
	case "tighten_debounce":
		return "收紧更新冷却中"
	default:
		return strings.TrimSpace(reason)
	}
}

// TranslateLLMFieldRefs replaces English field paths (e.g. cross_tf_summary.alignment=mixed)
// in LLM free-text output with Chinese labels. Handles dotted paths, field=value patterns,
// standalone field names, and event list references (events=xxx, events含 xxx).
func TranslateLLMFieldRefs(text string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	// First pass: translate "events=..." and "events含 ..." patterns.
	result := FormatEventList(text)
	if !containsFieldRef(result) && containsHan(result) {
		return result
	}
	result = fieldRefPattern.ReplaceAllStringFunc(result, func(match string) string {
		return translateFieldRef(match)
	})
	return result
}

var fieldRefPattern = regexp.MustCompile(`[a-z][a-z0-9_]*(?:\.[a-z][a-z0-9_]*)*(?:=[^\s,;，；]+)?`)

func containsFieldRef(text string) bool {
	return fieldRefPattern.MatchString(text)
}

func translateFieldRef(ref string) string {
	eqIdx := strings.Index(ref, "=")
	if eqIdx < 0 {
		normalized := normalizeDirtyValue(strings.ToLower(ref))
		if normalized != strings.ToLower(ref) {
			if label, ok := translatedTerms[normalized]; ok {
				return label
			}
		}
		return translateFieldPath(ref)
	}
	fieldPart := ref[:eqIdx]
	valuePart := ref[eqIdx+1:]
	translatedField := translateFieldPath(fieldPart)
	translatedValue := translateFieldValue(valuePart)
	return translatedField + "=" + translatedValue
}

func translateFieldPath(path string) string {
	parts := strings.Split(path, ".")
	translated := make([]string, 0, len(parts))
	for _, part := range parts {
		if label, ok := llmKeyLabels[part]; ok {
			translated = append(translated, label)
		} else if label, ok := translatedTerms[strings.ToLower(part)]; ok {
			translated = append(translated, label)
		} else {
			translated = append(translated, part)
		}
	}
	return strings.Join(translated, ".")
}

func translateFieldValue(value string) string {
	key := strings.ToLower(strings.TrimSpace(value))
	key = normalizeDirtyValue(key)
	if key == "true" {
		return "是"
	}
	if key == "false" {
		return "否"
	}
	if label, ok := translatedTerms[key]; ok {
		return label
	}
	if label, ok := directionLabels[key]; ok {
		return label
	}
	return value
}

// normalizeDirtyValue fixes known misspellings and non-standard formats from LLM output.
var dirtyValueReplacements = map[string]string{
	"aaroon_strong_bullish": "aroon_strong_bullish",
	"aaroon_strong_bearish": "aroon_strong_bearish",
}

var dirtyValuePatterns = []struct {
	match   *regexp.Regexp
	replace string
}{
	// rsi_zone=45_55 → rsi_zone=45-55 (underscore separator in numeric ranges)
	{regexp.MustCompile(`^(\d+)_(\d+)$`), "${1}-${2}"},
}

func normalizeDirtyValue(value string) string {
	if replacement, ok := dirtyValueReplacements[value]; ok {
		return replacement
	}
	for _, p := range dirtyValuePatterns {
		if p.match.MatchString(value) {
			return p.match.ReplaceAllString(value, p.replace)
		}
	}
	return value
}

// FormatEventList translates an event list string like "events=price_cross_ema_fast_down"
// or "events含 aroon_strong_bearish" into fully translated Chinese text.
var eventsPattern = regexp.MustCompile(`(?i)(?:^|\b)(events)\s*([=含])\s*(.+)`)

func FormatEventList(text string) string {
	return eventsPattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := eventsPattern.FindStringSubmatch(match)
		if len(parts) < 4 {
			return match
		}
		sep := parts[2]
		if sep == "=" {
			sep = "="
		} else {
			sep = "含 "
		}
		eventKeys := strings.Split(parts[3], ",")
		translated := make([]string, 0, len(eventKeys))
		for _, ek := range eventKeys {
			ek = strings.TrimSpace(ek)
			normalized := normalizeDirtyValue(strings.ToLower(ek))
			if label, ok := translatedTerms[normalized]; ok {
				translated = append(translated, label)
			} else {
				translated = append(translated, ek)
			}
		}
		return "事件" + sep + strings.Join(translated, ", ")
	})
}
