package decisionfmt

import (
	"strings"
	"testing"
)

func TestTranslateDecisionActionTableDriven(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "allow", want: "允许"},
		{in: " TIGHTEN ", want: "收紧风控"},
		{in: "", want: ""},
		{in: "custom", want: "custom"},
	}
	for _, tc := range tests {
		if got := translateDecisionAction(tc.in); got != tc.want {
			t.Fatalf("translateDecisionAction(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestTranslateGateReasonSpecialCases(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "PASS_STRONG", want: "强通过"},
		{in: "AGENT_ERROR:model timeout", want: "Agent 阶段异常：model timeout"},
		{in: "PROVIDER_ERROR:data stale", want: "Provider 阶段异常：data stale"},
		{in: "GATE_ERROR:consensus", want: "Gate 阶段异常：consensus"},
		{in: "weird_reason", want: "weird_reason(英文)"},
	}
	for _, tc := range tests {
		if got := translateGateReason(tc.in); got != tc.want {
			t.Fatalf("translateGateReason(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestTranslateSieveReasonCodeFallsBackToGateReason(t *testing.T) {
	if got := translateSieveReasonCode("CROWD_ALIGN_LOW"); got != "同向拥挤/低置信" {
		t.Fatalf("translateSieveReasonCode mapped=%q", got)
	}
	if got := translateSieveReasonCode("PASS_WEAK"); got != "弱通过" {
		t.Fatalf("translateSieveReasonCode fallback=%q", got)
	}
}

func TestTranslateTermFallbacks(t *testing.T) {
	if got := translateTerm("trend_surge"); got != "trend_surge(趋势加速)" {
		t.Fatalf("translateTerm mapped=%q", got)
	}
	if got := translateTerm("中文"); got != "中文" {
		t.Fatalf("translateTerm han=%q", got)
	}
	if got := translateTerm("unknown_token"); got != "unknown_token(英文)" {
		t.Fatalf("translateTerm unknown=%q", got)
	}
}

func TestTranslateLLMKeyAndProviderRole(t *testing.T) {
	if got := translateLLMKey("confidence"); got != "置信度" {
		t.Fatalf("translateLLMKey=%q", got)
	}
	if got := translateLLMKey("custom_key"); got != "custom_key" {
		t.Fatalf("translateLLMKey custom=%q", got)
	}
	if got := translateLLMKey("cross_tf_summary"); got != "跨周期汇总" {
		t.Fatalf("translateLLMKey cross_tf=%q", got)
	}
	if got := translateLLMKey("movement_score"); got != "方向分数" {
		t.Fatalf("translateLLMKey movement_score=%q", got)
	}
	if got := providerRoleLabel("mechanics"); got != "市场机制" {
		t.Fatalf("providerRoleLabel=%q", got)
	}
	if got := providerRoleLabel(" custom "); got != "custom" {
		t.Fatalf("providerRoleLabel custom=%q", got)
	}
}

func TestTranslateLLMFieldRefs(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty",
			in:   "",
			want: "",
		},
		{
			name: "pure chinese no refs",
			in:   "这是一段中文描述",
			want: "这是一段中文描述",
		},
		{
			name: "field=value",
			in:   "lower_tf_agreement=false",
			want: "低周期一致性=否",
		},
		{
			name: "dotted path=value",
			in:   "cross_tf_summary.alignment=mixed",
			want: "跨周期汇总.指标一致性=信号混杂/分歧",
		},
		{
			name: "mixed text with field ref",
			in:   "高 higher_tf_agreement=false，低 lower_tf_agreement=false",
			want: "高 高周期一致性=否，低 低周期一致性=否",
		},
		{
			name: "indicator state values",
			in:   "ema_stack=bull, bb_zone=near_upper",
			want: "EMA排列=多头排列, 布林带区间=靠近上轨",
		},
		{
			name: "standalone field path",
			in:   "movement_score 偏低",
			want: "方向分数 偏低",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := TranslateLLMFieldRefs(tc.in)
			if got != tc.want {
				t.Errorf("TranslateLLMFieldRefs(%q)\n  got  = %q\n  want = %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestIsLLMFreeTextField(t *testing.T) {
	if !isLLMFreeTextField("momentum_detail") {
		t.Fatal("expected momentum_detail to be free text field")
	}
	if !isLLMFreeTextField("conflict_detail") {
		t.Fatal("expected conflict_detail to be free text field")
	}
	if isLLMFreeTextField("tradeable") {
		t.Fatal("expected tradeable to NOT be free text field")
	}
}

func TestTranslateTermIndicatorStates(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "above", want: "above(上方)"},
		{in: "below", want: "below(下方)"},
		{in: "bull", want: "bull(多头排列)"},
		{in: "bear", want: "bear(空头排列)"},
		{in: "squeeze", want: "squeeze(挤压收窄)"},
		{in: "trending", want: "trending(趋势行情)"},
		{in: "choppy", want: "choppy(震荡行情)"},
		{in: "oversold", want: "oversold(超卖)"},
		{in: "overbought", want: "overbought(超买)"},
	}
	for _, tc := range tests {
		if got := translateTerm(tc.in); got != tc.want {
			t.Fatalf("translateTerm(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestTranslateTermMechanicsStates(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		// OI-价格关系
		{in: "price_up_oi_up", want: "price_up_oi_up(价格上涨/OI上升)"},
		{in: "price_up_oi_down", want: "price_up_oi_down(价格上涨/OI下降)"},
		{in: "price_down_oi_up", want: "price_down_oi_up(价格下跌/OI上升)"},
		{in: "price_down_oi_down", want: "price_down_oi_down(价格下跌/OI下降)"},
		// 情绪状态
		{in: "fear", want: "fear(恐惧)"},
		{in: "greed", want: "greed(贪婪)"},
		{in: "extreme_greed", want: "extreme_greed(极度贪婪)"},
		// 资金费率热度 / 清算压力
		{in: "hot", want: "hot(过热)"},
		{in: "elevated", want: "elevated(偏高)"},
		// 机制冲突
		{in: "crowding_long_but_liq_stress_high", want: "crowding_long_but_liq_stress_high(多头拥挤但清算压力高)"},
		{in: "crowding_short_but_liq_stress_high", want: "crowding_short_but_liq_stress_high(空头拥挤但清算压力高)"},
		{in: "funding_long_but_oi_falling", want: "funding_long_but_oi_falling(资金费率偏多但OI下降)"},
		{in: "funding_short_but_oi_rising", want: "funding_short_but_oi_rising(资金费率偏空但OI上升)"},
		// 趋势突破
		{in: "break_up", want: "break_up(向上突破)"},
		{in: "break_down", want: "break_down(向下突破)"},
		// SuperTrend / 情绪标签
		{in: "bullish", want: "bullish(看多)"},
		{in: "bearish", want: "bearish(看空)"},
		{in: "Strong Long", want: "Strong Long(强烈看多)"},
		{in: "Strong Short", want: "Strong Short(强烈看空)"},
		{in: "Long Bias", want: "Long Bias(偏多)"},
		{in: "Short Bias", want: "Short Bias(偏空)"},
	}
	for _, tc := range tests {
		if got := translateTerm(tc.in); got != tc.want {
			t.Fatalf("translateTerm(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestTranslateLLMKeyMechanicsAndTrendFields(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		// Mechanics state fields
		{in: "oi_state", want: "持仓量状态"},
		{in: "funding_state", want: "资金费率状态"},
		{in: "crowding_state", want: "拥挤度状态"},
		{in: "liquidation_state", want: "清算状态"},
		{in: "sentiment_state", want: "市场情绪"},
		{in: "mechanics_conflict", want: "机制冲突"},
		{in: "oi_price_relation", want: "OI-价格关系"},
		{in: "change_state", want: "变化状态"},
		{in: "fear_greed", want: "恐贪指数"},
		{in: "top_trader_bias", want: "大户偏向"},
		{in: "reversal_risk", want: "反转风险"},
		{in: "stress", want: "清算压力"},
		{in: "heat", want: "资金费率热度"},
		{in: "ls_ratio", want: "多空比"},
		{in: "taker_ratio", want: "主动买卖比"},
		{in: "oi_change_pct", want: "OI变化率"},
		{in: "price_change_pct", want: "价格变化率"},
		// Trend fields
		{in: "vol_ratio", want: "成交量比率"},
		{in: "level_price", want: "关键价位"},
		{in: "order_block", want: "订单块(Order Block)"},
		{in: "fvg", want: "公允价值缺口(FVG)"},
		{in: "slope_state", want: "斜率状态"},
		{in: "trend_slope", want: "趋势斜率"},
		{in: "break_events", want: "结构突破事件"},
		{in: "break_summary", want: "突破汇总"},
		{in: "supertrend", want: "SuperTrend指标"},
		{in: "tag", want: "情绪标签"},
		{in: "taker_long_short_vol_ratio", want: "主买/主卖成交量比"},
	}
	for _, tc := range tests {
		if got := translateLLMKey(tc.in); got != tc.want {
			t.Fatalf("translateLLMKey(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestTranslateLLMFieldRefsMechanics(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "oi_price_relation=price_down_oi_up",
			in:   "oi_price_relation=price_down_oi_up",
			want: "OI-价格关系=价格下跌/OI上升",
		},
		{
			name: "dotted mechanics path",
			in:   "oi_state.change_state=rising",
			want: "持仓量状态.变化状态=上升",
		},
		{
			name: "sentiment fear_greed=extreme_greed",
			in:   "sentiment_state.fear_greed=extreme_greed",
			want: "市场情绪.恐贪指数=极度贪婪",
		},
		{
			name: "conflict string standalone",
			in:   "crowding_long_but_liq_stress_high",
			want: "多头拥挤但清算压力高",
		},
		{
			name: "break event type",
			in:   "break_events含 break_up",
			want: "结构突破事件含 向上突破",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := TranslateLLMFieldRefs(tc.in)
			if got != tc.want {
				t.Errorf("TranslateLLMFieldRefs(%q)\n  got  = %q\n  want = %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestHalfTranslatedMechanicsRejected ensures conflict strings and compound values are
// translated as whole units, never partially.
func TestHalfTranslatedMechanicsRejected(t *testing.T) {
	badOutputs := []string{
		"crowding_多头方向_but",
		"funding_多头方向_but",
		"price_上行_oi",
		"break_上行",
	}
	inputs := []string{
		"crowding_long_but_liq_stress_high",
		"funding_long_but_oi_falling",
		"price_up_oi_down",
		"break_up",
	}
	for _, input := range inputs {
		got := TranslateLLMFieldRefs(input)
		for _, bad := range badOutputs {
			if strings.Contains(got, bad) {
				t.Errorf("TranslateLLMFieldRefs(%q) produced half-translated output containing %q: %q",
					input, bad, got)
			}
		}
	}
}

func TestFormatEventList(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "events= with event key",
			in:   "events=price_cross_ema_fast_down",
			want: "事件=价格下穿快线EMA",
		},
		{
			name: "events含 with event key",
			in:   "events含 aroon_strong_bearish",
			want: "事件含 阿隆指标强势看空",
		},
		{
			name: "events= with multiple keys",
			in:   "events=price_cross_ema_mid_down,aroon_strong_bullish",
			want: "事件=价格下穿中线EMA, 阿隆指标强势看多",
		},
		{
			name: "no events pattern",
			in:   "some other text",
			want: "some other text",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatEventList(tc.in)
			if got != tc.want {
				t.Errorf("FormatEventList(%q)\n  got  = %q\n  want = %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeDirtyValue(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "aaroon_strong_bullish", want: "aroon_strong_bullish"},
		{in: "aaroon_strong_bearish", want: "aroon_strong_bearish"},
		{in: "45_55", want: "45-55"},
		{in: "normal_value", want: "normal_value"},
		{in: "bull", want: "bull"},
	}
	for _, tc := range tests {
		if got := normalizeDirtyValue(tc.in); got != tc.want {
			t.Fatalf("normalizeDirtyValue(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

// TestHalfTranslatedStringsRejected verifies that known bad outputs never appear.
// These are the exact error forms reported by users.
func TestHalfTranslatedStringsRejected(t *testing.T) {
	badOutputs := []string{
		"price_cross_中线EMA_down",
		"price_cross_快线EMA_down",
		"aroon_strong_看空",
		"aroon_strong_看多",
	}

	// These inputs, when translated via TranslateLLMFieldRefs, must NOT produce
	// any of the bad outputs.
	inputs := []string{
		"events=price_cross_ema_mid_down",
		"events=price_cross_ema_fast_down",
		"events含 aroon_strong_bearish",
		"events含 aroon_strong_bullish",
		"price_cross_ema_mid_down",
		"price_cross_ema_fast_down",
		"aroon_strong_bearish",
		"aroon_strong_bullish",
	}
	for _, input := range inputs {
		got := TranslateLLMFieldRefs(input)
		for _, bad := range badOutputs {
			if strings.Contains(got, bad) {
				t.Errorf("TranslateLLMFieldRefs(%q) produced half-translated output containing %q: %q",
					input, bad, got)
			}
		}
	}
}

func TestTranslateLLMFieldRefsWithEvents(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "events=event_key",
			in:   "events=price_cross_ema_fast_down",
			want: "事件=价格下穿快线EMA",
		},
		{
			name: "events含 event_key",
			in:   "events含 aroon_strong_bearish",
			want: "事件含 阿隆指标强势看空",
		},
		{
			name: "dirty value aaroon",
			in:   "aaroon_strong_bullish",
			want: "阿隆指标强势看多",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := TranslateLLMFieldRefs(tc.in)
			if got != tc.want {
				t.Errorf("TranslateLLMFieldRefs(%q)\n  got  = %q\n  want = %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestTranslateValue(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"action ALLOW", "ALLOW", "允许"},
		{"action VETO", "VETO", "否决"},
		{"action WAIT", "WAIT", "观望"},
		{"direction long", "long", "多头"},
		{"direction short", "short", "空头"},
		{"enum mixed", "mixed", "信号混杂/分歧"},
		{"enum contracting", "contracting", "波动/动能收敛"},
		{"gate reason", "CONSENSUS_NOT_PASSED", "三路共识未通过"},
		{"sieve reason crowded_long", "crowded_long", "多头拥挤"},
		{"unknown passthrough", "unknown_xyz", "unknown_xyz"},
		{"empty", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := TranslateValue(tc.in)
			if got != tc.want {
				t.Errorf("TranslateValue(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestTranslateSentence(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			"empty becomes dash",
			"",
			"—",
		},
		{
			"already Chinese passthrough",
			"未观察到明显冲突",
			"未观察到明显冲突",
		},
		{
			"OI phrase translation",
			"OI increased slightly in 15m",
			"持仓量上升 小幅 在15分钟内",
		},
		{
			"funding rate phrase",
			"funding rate negative",
			"资金费率为负",
		},
		{
			"Chinese text with embedded event key preserves runes",
			"呈现价格上涨/OI上升的price_up_oi_up关系",
			"呈现价格上涨/OI上升的价格上涨/OI上升关系",
		},
		{
			"event key flanked by Chinese chars",
			"处于price_down_oi_down区间",
			"处于价格下跌/OI下降区间",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := TranslateSentence(tc.in)
			if got != tc.want {
				t.Errorf("TranslateSentence(%q)\n  got  = %q\n  want = %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestTranslateExecutionBlockedReason(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"monitor_gate", "收紧监控门槛未满足"},
		{"atr_missing", "ATR 数据缺失"},
		{"score_threshold", "评分未达标"},
		{"unknown_reason", "unknown_reason"},
		{"", ""},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got := TranslateExecutionBlockedReason(tc.in)
			if got != tc.want {
				t.Errorf("TranslateExecutionBlockedReason(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
