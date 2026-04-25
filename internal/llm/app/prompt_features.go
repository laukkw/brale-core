package llmapp

import (
	"encoding/json"
	"strings"

	"brale-core/internal/decision"
)

const (
	promptStageAgentIndicator    = "agent_indicator"
	promptStageAgentStructure    = "agent_structure"
	promptStageAgentMechanics    = "agent_mechanics"
	promptStageProviderMechanics = "provider_mechanics"
	promptStageInPosMechanics    = "in_position_mechanics"
	promptLocaleZH               = "zh"
	promptLocaleEN               = "en"
)

type promptLocalizer struct {
	flatRiskContextLabel       string
	tightenRiskContextLabel    string
	planSummaryLabel           string
	structureAnchorLabel       string
	indicatorAgentSummaryLabel string
	structureAgentSummaryLabel string
	mechanicsAgentSummaryLabel string
	inPosIndicatorSummaryLabel string
	inPosStructureSummaryLabel string
	inPosMechanicsSummaryLabel string
	outputRequirementLabel     string
	indicatorInputLabel        string
	trendInputLabel            string
	mechanicsInputLabel        string
	decisionWindowLabel        string
	summaryInputLabel          string
	positionSummaryLabel       string
	providerDataAnchorLabel    string
	constraintLabel            string
	outputExampleLabel         string
	providerConstraint         string
	inPosProviderConstraint    string
	examplePlaceholderReason   string
	featureHeader              string
}

func normalizePromptLocale(locale string) string {
	switch strings.ToLower(strings.TrimSpace(locale)) {
	case promptLocaleEN:
		return promptLocaleEN
	default:
		return promptLocaleZH
	}
}

func localizerFor(locale string) promptLocalizer {
	if normalizePromptLocale(locale) == promptLocaleEN {
		return promptLocalizer{
			flatRiskContextLabel:       "Trading Context (Required):",
			tightenRiskContextLabel:    "Position Risk Context (Required):",
			planSummaryLabel:           "Plan Summary (Required):",
			structureAnchorLabel:       "Structure Anchor Summary (Required):",
			indicatorAgentSummaryLabel: "Indicator Agent Summary (Required):",
			structureAgentSummaryLabel: "Structure Agent Summary (Required):",
			mechanicsAgentSummaryLabel: "Mechanics Agent Summary (Required):",
			inPosIndicatorSummaryLabel: "In-Position Indicator Provider Summary (Required):",
			inPosStructureSummaryLabel: "In-Position Structure Provider Summary (Required):",
			inPosMechanicsSummaryLabel: "In-Position Mechanics Provider Summary (Required):",
			outputRequirementLabel:     "Output Requirement:",
			indicatorInputLabel:        "Indicator Input",
			trendInputLabel:            "Trend Input",
			mechanicsInputLabel:        "Mechanics Input",
			decisionWindowLabel:        "Decision Window:",
			summaryInputLabel:          "Summary Input:",
			positionSummaryLabel:       "Position Summary:",
			providerDataAnchorLabel:    "Code-Computed Data Anchors (cross-check only):",
			constraintLabel:            "Constraints:",
			outputExampleLabel:         "Output Example (JSON):",
			providerConstraint:         "The example JSON only shows the fixed field layout, value types, and reference style. Do not copy or adapt its conclusions, reasons, tags, thresholds, booleans, confidence, or wording. The final output must be generated independently from the current input. Data anchors are only for cross-checking the Agent summary and are not an independent decision source.",
			inPosProviderConstraint:    "Output fixed-field JSON only. Do not invent fields or thresholds. You may quote existing input `field=value` pairs as audit evidence. Data anchors are only for cross-checking the Agent summary and are not an independent decision source. The final output must be generated independently from the current input.",
			examplePlaceholderReason:   "Cite the current input fields and explain the reasoning (example placeholder, do not copy).",
			featureHeader:              "Feature-Specific Guidance:",
		}
	}
	return promptLocalizer{
		flatRiskContextLabel:       "交易上下文(必填):",
		tightenRiskContextLabel:    "仓位风控上下文(必填):",
		planSummaryLabel:           "计划摘要(必填):",
		structureAnchorLabel:       "结构锚点摘要(必填):",
		indicatorAgentSummaryLabel: "Indicator Agent 摘要(必填):",
		structureAgentSummaryLabel: "Structure Agent 摘要(必填):",
		mechanicsAgentSummaryLabel: "Mechanics Agent 摘要(必填):",
		inPosIndicatorSummaryLabel: "持仓态 Indicator Provider 摘要(必填):",
		inPosStructureSummaryLabel: "持仓态 Structure Provider 摘要(必填):",
		inPosMechanicsSummaryLabel: "持仓态 Mechanics Provider 摘要(必填):",
		outputRequirementLabel:     "输出要求:",
		indicatorInputLabel:        "Indicator 输入",
		trendInputLabel:            "Trend 输入",
		mechanicsInputLabel:        "Mechanics 输入",
		decisionWindowLabel:        "决策窗口:",
		summaryInputLabel:          "摘要输入:",
		positionSummaryLabel:       "仓位摘要:",
		providerDataAnchorLabel:    "代码计算数据锚点(仅供交叉验证):",
		constraintLabel:            "约束:",
		outputExampleLabel:         "输出示例(JSON):",
		providerConstraint:         "输出示例 JSON 仅用于展示固定字段结构、字段类型与引用格式；禁止直接引用、复制、改写或沿用示例中的任何结论、reason、tag、阈值、布尔值、置信度或措辞。最终输出必须完全基于本轮输入独立生成。数据锚点仅用于交叉验证Agent摘要的一致性，不作为独立判断依据。",
		inPosProviderConstraint:    "仅输出固定字段 JSON；禁止编造/新增字段或阈值；允许原样引用输入中已有的 field=value 作为审计依据。数据锚点仅用于交叉验证Agent摘要的一致性，不作为独立判断依据。最终输出必须完全基于本轮输入独立生成。",
		examplePlaceholderReason:   "引用本轮输入中的关键字段作为依据并说明判断逻辑（示例占位，禁止直接引用）",
		featureHeader:              "当前启用且可用的特征说明:",
	}
}

func assemblePromptWithFeatures(core, locale, stage string, features []string) string {
	core = strings.TrimSpace(core)
	if core == "" || len(features) == 0 {
		return core
	}
	loc := localizerFor(locale)
	return core + "\n\n" + loc.featureHeader + "\n" + strings.Join(features, "\n")
}

func indicatorFeatureFragments(raw []byte, locale string) []string {
	var payload struct {
		MultiTF []struct {
			Trend struct {
				PriceVsEMAFast string `json:"price_vs_ema_fast"`
				PriceVsEMAMid  string `json:"price_vs_ema_mid"`
				PriceVsEMASlow string `json:"price_vs_ema_slow"`
				EMAStack       string `json:"ema_stack"`
			} `json:"trend"`
			Momentum struct {
				RSIZone      string `json:"rsi_zone"`
				RSISlope     string `json:"rsi_slope_state"`
				STCState     string `json:"stc_state"`
				OBVSlope     string `json:"obv_slope_state"`
				StochRSIZone string `json:"stoch_rsi_zone"`
			} `json:"momentum"`
			Volatility struct {
				ATRExpand string `json:"atr_expand_state"`
				BBZone    string `json:"bb_zone"`
				BBWidth   string `json:"bb_width_state"`
				CHOP      string `json:"chop_regime"`
			} `json:"volatility"`
			Events []string `json:"events"`
		} `json:"multi_tf"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	has := map[string]bool{}
	for _, tf := range payload.MultiTF {
		if tf.Trend.PriceVsEMAFast != "" || tf.Trend.PriceVsEMAMid != "" || tf.Trend.PriceVsEMASlow != "" || tf.Trend.EMAStack != "" {
			has["ema"] = has["ema"] || tf.Trend.EMAStack != "unknown"
		}
		has["rsi"] = has["rsi"] || (tf.Momentum.RSIZone != "" && tf.Momentum.RSIZone != "unknown")
		has["atr"] = has["atr"] || (tf.Volatility.ATRExpand != "" && tf.Volatility.ATRExpand != "unknown")
		has["obv"] = has["obv"] || (tf.Momentum.OBVSlope != "" && tf.Momentum.OBVSlope != "unknown")
		has["stc"] = has["stc"] || (tf.Momentum.STCState != "" && tf.Momentum.STCState != "unknown")
		has["bb"] = has["bb"] || tf.Volatility.BBZone != "" || tf.Volatility.BBWidth != ""
		has["chop"] = has["chop"] || tf.Volatility.CHOP != ""
		has["stoch_rsi"] = has["stoch_rsi"] || tf.Momentum.StochRSIZone != ""
		for _, event := range tf.Events {
			if strings.HasPrefix(event, "aroon_") {
				has["aroon"] = true
			}
			if strings.HasPrefix(event, "td_") {
				has["td"] = true
			}
		}
	}
	order := []string{"ema", "rsi", "atr", "obv", "stc", "bb", "chop", "stoch_rsi", "aroon", "td"}
	return orderedFragments(locale, promptStageAgentIndicator, order, has)
}

func structureFeatureFragments(raw []byte, locale string) []string {
	var payload struct {
		Blocks []struct {
			GlobalContext struct {
				EMA20  *float64 `json:"ema20"`
				EMA50  *float64 `json:"ema50"`
				EMA200 *float64 `json:"ema200"`
			} `json:"global_context"`
			SuperTrend *struct{} `json:"supertrend"`
			Recent     []struct {
				RSI *float64 `json:"rsi"`
			} `json:"recent_candles"`
			Points []struct {
				RSI *float64 `json:"rsi"`
			} `json:"structure_points"`
			Pattern json.RawMessage `json:"pattern"`
			SMC     json.RawMessage `json:"smc"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	has := map[string]bool{}
	for _, block := range payload.Blocks {
		if block.SuperTrend != nil {
			has["supertrend"] = true
		}
		if block.GlobalContext.EMA20 != nil || block.GlobalContext.EMA50 != nil || block.GlobalContext.EMA200 != nil {
			has["ema_context"] = true
		}
		for _, candle := range block.Recent {
			if candle.RSI != nil {
				has["rsi_context"] = true
				break
			}
		}
		if !has["rsi_context"] {
			for _, point := range block.Points {
				if point.RSI != nil {
					has["rsi_context"] = true
					break
				}
			}
		}
		if len(strings.TrimSpace(string(block.Pattern))) > 0 && string(block.Pattern) != "null" {
			has["patterns"] = true
		}
		if len(strings.TrimSpace(string(block.SMC))) > 0 && string(block.SMC) != "null" {
			has["smc"] = true
		}
	}
	order := []string{"supertrend", "ema_context", "rsi_context", "patterns", "smc"}
	return orderedFragments(locale, promptStageAgentStructure, order, has)
}

func mechanicsFeatureFragments(raw []byte, locale string) []string {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	has := map[string]bool{
		"oi":                hasJSONValue(payload["oi"]) || hasJSONValue(payload["oi_state"]) || hasJSONValue(payload["oi_history"]),
		"funding":           hasJSONValue(payload["funding"]) || hasJSONValue(payload["funding_state"]),
		"long_short":        hasJSONValue(payload["long_short_by_interval"]) || hasJSONValue(payload["crowding_state"]),
		"fear_greed":        hasJSONValue(payload["fear_greed"]) || hasJSONValue(payload["sentiment_state"]),
		"liquidations":      hasJSONValue(payload["liquidations"]) || hasJSONValue(payload["liquidations_by_window"]) || hasJSONValue(payload["liquidation_source"]) || hasJSONValue(payload["liquidation_state"]),
		"cvd":               hasJSONValue(payload["cvd_by_interval"]),
		"sentiment":         hasJSONValue(payload["sentiment_by_interval"]),
		"futures_sentiment": hasJSONValue(payload["futures_sentiment"]),
	}
	order := []string{"oi", "funding", "long_short", "fear_greed", "liquidations", "cvd", "sentiment", "futures_sentiment"}
	return orderedFragments(locale, promptStageAgentMechanics, order, has)
}

func mechanicsProviderFragments(dataCtx *decision.MechanicsDataContext, locale string, inPosition bool) []string {
	if dataCtx == nil {
		return nil
	}
	has := map[string]bool{
		"liquidations": dataCtx.LiquidationState != nil || dataCtx.LiquidationSource != nil,
	}
	stage := promptStageProviderMechanics
	if inPosition {
		stage = promptStageInPosMechanics
	}
	return orderedFragments(locale, stage, []string{"liquidations"}, has)
}

func orderedFragments(locale, stage string, order []string, present map[string]bool) []string {
	out := make([]string, 0, len(order))
	for _, key := range order {
		if !present[key] {
			continue
		}
		if fragment := featureFragment(locale, stage, key); fragment != "" {
			out = append(out, fragment)
		}
	}
	return out
}

func featureFragment(locale, stage, key string) string {
	locale = normalizePromptLocale(locale)
	if locale == promptLocaleEN {
		return featureFragmentEN(stage, key)
	}
	return featureFragmentZH(stage, key)
}

func featureFragmentZH(stage, key string) string {
	switch stage + "/" + key {
	case promptStageAgentIndicator + "/ema":
		return "- EMA 已可用时，优先读取 `price_vs_ema_*` 与 `ema_stack`，判断价格相对均线位置、均线顺序和趋势一致性。"
	case promptStageAgentIndicator + "/rsi":
		return "- RSI 已可用时，结合 `rsi_zone` 与 `rsi_slope_state` 判断动量强弱、回落还是加速。"
	case promptStageAgentIndicator + "/atr":
		return "- ATR 已可用时，参考 `atr_expand_state` 与 `atr_change_pct` 判断波动率是在扩张、收缩还是稳定。"
	case promptStageAgentIndicator + "/obv":
		return "- OBV 已可用时，参考 `obv_slope_state` 判断量价是否同向支持当前动量。"
	case promptStageAgentIndicator + "/stc":
		return "- STC 已可用时，参考 `stc_state` 判断趋势节奏是否仍在强化或已经转弱。"
	case promptStageAgentIndicator + "/bb":
		return "- 布林带已可用时，结合 `bb_zone` 与 `bb_width_state` 判断价格所处带内位置与压缩/扩张状态。"
	case promptStageAgentIndicator + "/chop":
		return "- CHOP 已可用时，使用 `chop_regime` 区分趋势环境、震荡环境或过渡阶段。"
	case promptStageAgentIndicator + "/stoch_rsi":
		return "- StochRSI 已可用时，参考 `stoch_rsi_zone` 判断短线是否过热、过冷或中性。"
	case promptStageAgentIndicator + "/aroon":
		return "- Aroon 已可用时，优先留意 `events` 中的 `aroon_*` 事件，它表示趋势强化或强反转线索。"
	case promptStageAgentIndicator + "/td":
		return "- TD Sequential 已可用时，优先留意 `events` 中的 `td_*` 事件，把它作为潜在衰竭/延续提示而不是单独结论。"
	case promptStageAgentStructure + "/supertrend":
		return "- SuperTrend 已可用时，参考 `supertrend.state/level/distance_pct` 判断趋势方向与失效距离。"
	case promptStageAgentStructure + "/ema_context":
		return "- 结构中的 EMA 上下文已可用时，结合 `global_context.ema20/ema50/ema200` 判断结构方向是否有中长期均线支撑。"
	case promptStageAgentStructure + "/rsi_context":
		return "- 结构中的 RSI 上下文已可用时，可交叉读取 `recent_candles[].rsi` 与 `structure_points[].rsi`，判断突破或回踩时动量是否配合。"
	case promptStageAgentStructure + "/patterns":
		return "- 形态识别已可用时，仅在 `pattern` 明确存在时引用，并结合 `quality` 与最近突破反应判断其可靠度。"
	case promptStageAgentStructure + "/smc":
		return "- SMC 信息已可用时，参考 `smc.order_block` 与 `smc.fvg` 判断关键供需区是否仍然有效。"
	case promptStageAgentMechanics + "/oi":
		return "- OI 已可用时，优先结合 `oi_state` 与 `oi_history` 判断杠杆是在堆积、释放还是无明显变化。"
	case promptStageAgentMechanics + "/funding":
		return "- 资金费率已可用时，参考 `funding` 与 `funding_state` 判断方向偏置和过热程度。"
	case promptStageAgentMechanics + "/long_short":
		return "- 多空比已可用时，结合 `long_short_by_interval` 与 `crowding_state` 判断拥挤方向与反转风险。"
	case promptStageAgentMechanics + "/fear_greed":
		return "- 情绪指数已可用时，参考 `fear_greed` 与 `sentiment_state` 判断情绪极端是否在放大机制风险。"
	case promptStageAgentMechanics + "/liquidations", promptStageProviderMechanics + "/liquidations", promptStageInPosMechanics + "/liquidations":
		return "- 清算数据已可用时，重点读取 `liquidation_source.coverage/status` 与 `liquidation_source.sample_count/coverage_sec/complete`，以及 `liquidations_by_window.*.sample_count/coverage_sec/complete`；当 `liquidation_source.coverage=largest_order_per_symbol_per_1000ms` 时，`sample_count` 只表示“每个 symbol 每 1000ms 保留的最大一笔清算样本数”，不是完整逐笔市场总量；`warming_up/stale/unavailable` 只能代表样本质量不足，不能直接解释为低风险。"
	case promptStageAgentMechanics + "/cvd":
		return "- CVD 已可用时，结合 `cvd_by_interval` 判断主动买卖量是否支持当前方向。"
	case promptStageAgentMechanics + "/sentiment":
		return "- 派生情绪分数已可用时，参考 `sentiment_by_interval` 的分数和标签，但不要单独把它当成结论。"
	case promptStageAgentMechanics + "/futures_sentiment":
		return "- 期货情绪已可用时，参考 `futures_sentiment` 判断顶级交易者与 taker 流向是否同向强化。"
	default:
		return ""
	}
}

func featureFragmentEN(stage, key string) string {
	switch stage + "/" + key {
	case promptStageAgentIndicator + "/ema":
		return "- When EMA context is available, read `price_vs_ema_*` and `ema_stack` first to judge price-vs-EMA position, stack order, and directional agreement."
	case promptStageAgentIndicator + "/rsi":
		return "- When RSI is available, use `rsi_zone` and `rsi_slope_state` together to judge momentum strength, pullback, or acceleration."
	case promptStageAgentIndicator + "/atr":
		return "- When ATR is available, use `atr_expand_state` and `atr_change_pct` to decide whether volatility is expanding, contracting, or stable."
	case promptStageAgentIndicator + "/obv":
		return "- When OBV is available, use `obv_slope_state` to judge whether volume flow confirms or weakens current momentum."
	case promptStageAgentIndicator + "/stc":
		return "- When STC is available, use `stc_state` to judge whether trend cadence is strengthening or fading."
	case promptStageAgentIndicator + "/bb":
		return "- When Bollinger Bands are available, combine `bb_zone` and `bb_width_state` to judge price location inside the band and squeeze vs expansion."
	case promptStageAgentIndicator + "/chop":
		return "- When CHOP is available, use `chop_regime` to separate trending, choppy, and transition regimes."
	case promptStageAgentIndicator + "/stoch_rsi":
		return "- When StochRSI is available, use `stoch_rsi_zone` to judge short-term overbought, oversold, or neutral conditions."
	case promptStageAgentIndicator + "/aroon":
		return "- When Aroon is available, pay special attention to `aroon_*` items inside `events`; they indicate trend-strength or reversal clues."
	case promptStageAgentIndicator + "/td":
		return "- When TD Sequential is available, treat `td_*` items inside `events` as exhaustion or continuation clues, not as standalone conclusions."
	case promptStageAgentStructure + "/supertrend":
		return "- When SuperTrend is available, use `supertrend.state`, `level`, and `distance_pct` to judge trend direction and invalidation distance."
	case promptStageAgentStructure + "/ema_context":
		return "- When structure EMA context is available, use `global_context.ema20/ema50/ema200` to judge whether medium and higher-timeframe averages support the structure read."
	case promptStageAgentStructure + "/rsi_context":
		return "- When structure RSI context is available, cross-read `recent_candles[].rsi` and `structure_points[].rsi` to judge whether momentum confirms breaks, retests, or rejection."
	case promptStageAgentStructure + "/patterns":
		return "- When pattern evidence is available, cite `pattern` only when it is clearly present and cross-check it against structure quality and the latest break reaction."
	case promptStageAgentStructure + "/smc":
		return "- When SMC data is available, use `smc.order_block` and `smc.fvg` to judge whether key supply-demand zones remain valid."
	case promptStageAgentMechanics + "/oi":
		return "- When OI is available, prioritize `oi_state` and `oi_history` to judge whether leverage is building, unwinding, or flat."
	case promptStageAgentMechanics + "/funding":
		return "- When funding is available, use `funding` and `funding_state` to judge directional bias and overheating."
	case promptStageAgentMechanics + "/long_short":
		return "- When long-short data is available, combine `long_short_by_interval` and `crowding_state` to judge crowding direction and reversal risk."
	case promptStageAgentMechanics + "/fear_greed":
		return "- When fear-greed data is available, use `fear_greed` and `sentiment_state` to judge whether sentiment extremes are amplifying mechanics risk."
	case promptStageAgentMechanics + "/liquidations", promptStageProviderMechanics + "/liquidations", promptStageInPosMechanics + "/liquidations":
		return "- When liquidation data is available, focus on `liquidation_source.coverage/status`, `liquidation_source.sample_count/coverage_sec/complete`, and `liquidations_by_window.*.sample_count/coverage_sec/complete`; when `liquidation_source.coverage=largest_order_per_symbol_per_1000ms`, `sample_count` means the count of the largest sampled liquidation per symbol per 1000ms, not the complete tick-by-tick market total; `warming_up/stale/unavailable` means the sample quality is incomplete, not that risk is low."
	case promptStageAgentMechanics + "/cvd":
		return "- When CVD is available, use `cvd_by_interval` to judge whether aggressive flow confirms the current direction."
	case promptStageAgentMechanics + "/sentiment":
		return "- When derivatives sentiment is available, use `sentiment_by_interval` as supporting context only, not as a standalone conclusion."
	case promptStageAgentMechanics + "/futures_sentiment":
		return "- When futures sentiment is available, use `futures_sentiment` to judge whether top-trader positioning and taker flow reinforce each other."
	default:
		return ""
	}
}

func hasJSONValue(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null" && trimmed != "{}" && trimmed != "[]"
}
