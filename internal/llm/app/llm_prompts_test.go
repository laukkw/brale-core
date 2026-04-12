package llmapp

import (
	"strings"
	"testing"

	"brale-core/internal/decision"
	"brale-core/internal/decision/agent"
	"brale-core/internal/decision/features"
)

func TestFlatRiskInitPromptIncludesAgentSummaryBlocks(t *testing.T) {
	builder := LLMPromptBuilder{RiskFlatInitSystem: "risk-system", UserFormat: UserPromptFormatBullet}
	_, user, err := builder.FlatRiskInitPrompt(FlatRiskPromptInput{
		Symbol:    "BTCUSDT",
		Direction: "long",
		Entry:     100,
		RiskPct:   0.01,
		PlanSummary: map[string]any{
			"atr":          2.0,
			"max_leverage": 5.0,
		},
		AgentIndicator: agent.IndicatorSummary{
			Expansion: agent.ExpansionExpanding,
			Alignment: agent.AlignmentAligned,
			Noise:     agent.NoiseLow,
		},
		AgentStructure: agent.StructureSummary{
			Regime:    agent.RegimeTrendUp,
			LastBreak: agent.LastBreakBosUp,
			Quality:   agent.QualityClean,
		},
		AgentMechanics: agent.MechanicsSummary{
			LeverageState: agent.LeverageStateStable,
			Crowding:      agent.CrowdingBalanced,
			RiskLevel:     agent.RiskLevelLow,
		},
		StructureAnchors: map[string]any{
			"nearest_below_entry": map[string]any{"price": 98.5},
		},
	})
	if err != nil {
		t.Fatalf("FlatRiskInitPrompt: %v", err)
	}
	if !strings.Contains(user, "计划摘要(必填):") {
		t.Fatalf("user prompt missing plan summary block: %s", user)
	}
	if !strings.Contains(user, "- atr: 2") {
		t.Fatalf("user prompt missing atr payload: %s", user)
	}
	if !strings.Contains(user, "Indicator Agent 摘要(必填):") {
		t.Fatalf("user prompt missing indicator agent block: %s", user)
	}
	if !strings.Contains(user, "Structure Agent 摘要(必填):") {
		t.Fatalf("user prompt missing structure agent block: %s", user)
	}
	if !strings.Contains(user, "Mechanics Agent 摘要(必填):") {
		t.Fatalf("user prompt missing mechanics agent block: %s", user)
	}
	if !strings.Contains(user, "结构锚点摘要(必填):") {
		t.Fatalf("user prompt missing structure anchor block: %s", user)
	}
	if strings.Contains(user, "共识摘要(必填):") || strings.Contains(user, "结构摘要(必填):") || strings.Contains(user, "其他 Provider 摘要(必填):") {
		t.Fatalf("user prompt should not contain legacy summary blocks: %s", user)
	}
}

func TestTightenRiskUpdatePromptIncludesAgentSummaryBlocks(t *testing.T) {
	builder := LLMPromptBuilder{RiskTightenSystem: "risk-tighten-system", UserFormat: UserPromptFormatBullet}
	_, user, err := builder.TightenRiskUpdatePrompt(TightenRiskPromptInput{
		Symbol:             "BTCUSDT",
		Direction:          "long",
		Entry:              100,
		MarkPrice:          105,
		ATR:                2,
		CurrentStopLoss:    95,
		CurrentTakeProfits: []float64{108, 112},
		UnrealizedPnlPct:   0.05,
		PositionAgeMin:     45,
		TP1Hit:             true,
		DistanceToLiqPct:   0.18,
		AgentIndicator: agent.IndicatorSummary{
			Expansion: agent.ExpansionExpanding,
			Alignment: agent.AlignmentAligned,
			Noise:     agent.NoiseLow,
		},
		AgentStructure: agent.StructureSummary{
			Regime:    agent.RegimeTrendUp,
			LastBreak: agent.LastBreakBosUp,
			Quality:   agent.QualityClean,
		},
		AgentMechanics: agent.MechanicsSummary{
			LeverageState: agent.LeverageStateStable,
			Crowding:      agent.CrowdingBalanced,
			RiskLevel:     agent.RiskLevelLow,
		},
		StructureAnchors: map[string]any{
			"nearest_above_entry": map[string]any{"price": 108.5},
		},
	})
	if err != nil {
		t.Fatalf("TightenRiskUpdatePrompt: %v", err)
	}
	if !strings.Contains(user, "Indicator Agent 摘要(必填):") || !strings.Contains(user, "Structure Agent 摘要(必填):") || !strings.Contains(user, "Mechanics Agent 摘要(必填):") {
		t.Fatalf("user prompt missing agent blocks: %s", user)
	}
	if !strings.Contains(user, "结构锚点摘要(必填):") {
		t.Fatalf("user prompt missing structure anchor block: %s", user)
	}
	if !strings.Contains(user, "- unrealized_pnl_pct: 0.05") {
		t.Fatalf("user prompt missing unrealized pnl metric: %s", user)
	}
	if !strings.Contains(user, "- position_age_minutes: 45") {
		t.Fatalf("user prompt missing position age metric: %s", user)
	}
	if !strings.Contains(user, "- tp1_hit: true") {
		t.Fatalf("user prompt missing tp1 hit metric: %s", user)
	}
	if !strings.Contains(user, "- distance_to_liq_pct: 0.18") {
		t.Fatalf("user prompt missing liq distance metric: %s", user)
	}
	if strings.Contains(user, "Gate 摘要(必填):") || strings.Contains(user, "In-position 评估(必填):") {
		t.Fatalf("user prompt should not contain legacy tighten blocks: %s", user)
	}
}

func TestAgentPromptsIncludeDecisionIntervalWhenPresent(t *testing.T) {
	builder := LLMPromptBuilder{
		AgentIndicatorSystem: "indicator-system",
		AgentStructureSystem: "structure-system",
		AgentMechanicsSystem: "mechanics-system",
		UserFormat:           UserPromptFormatBullet,
	}

	tests := []struct {
		name string
		user string
	}{
		{
			name: "indicator",
			user: func() string {
				_, user, err := builder.AgentIndicatorPrompt(features.IndicatorJSON{RawJSON: []byte(`{"symbol":"BTCUSDT"}`)}, "1h")
				if err != nil {
					t.Fatalf("AgentIndicatorPrompt: %v", err)
				}
				return user
			}(),
		},
		{
			name: "structure",
			user: func() string {
				_, user, err := builder.AgentStructurePrompt(features.TrendJSON{RawJSON: []byte(`{"symbol":"BTCUSDT"}`)}, "4h")
				if err != nil {
					t.Fatalf("AgentStructurePrompt: %v", err)
				}
				return user
			}(),
		},
		{
			name: "mechanics",
			user: func() string {
				_, user, err := builder.AgentMechanicsPrompt(features.MechanicsSnapshot{RawJSON: []byte(`{"symbol":"BTCUSDT"}`)}, "15m")
				if err != nil {
					t.Fatalf("AgentMechanicsPrompt: %v", err)
				}
				return user
			}(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(tc.user, "决策窗口:") {
				t.Fatalf("user prompt missing decision interval block: %s", tc.user)
			}
		})
	}
}

func TestAgentPromptOmitsDecisionIntervalWhenEmpty(t *testing.T) {
	builder := LLMPromptBuilder{
		AgentIndicatorSystem: "indicator-system",
		UserFormat:           UserPromptFormatBullet,
	}

	_, user, err := builder.AgentIndicatorPrompt(features.IndicatorJSON{RawJSON: []byte(`{"symbol":"BTCUSDT"}`)}, "")
	if err != nil {
		t.Fatalf("AgentIndicatorPrompt: %v", err)
	}
	if strings.Contains(user, "决策窗口:") {
		t.Fatalf("user prompt should omit decision interval block: %s", user)
	}
}

func TestProviderExamplesUseSafeDefaultTags(t *testing.T) {
	if !strings.Contains(providerExampleIndicator(), `"signal_tag":"noise"`) {
		t.Fatalf("unexpected indicator example: %s", providerExampleIndicator())
	}
	if !strings.Contains(providerExampleStructure(), `"signal_tag":"support_retest"`) {
		t.Fatalf("unexpected structure example: %s", providerExampleStructure())
	}
	if !strings.Contains(providerExampleMechanics(), `"signal_tag":"neutral"`) {
		t.Fatalf("unexpected mechanics example: %s", providerExampleMechanics())
	}
	if !strings.Contains(providerExampleInPositionIndicator(), `"monitor_tag":"keep"`) {
		t.Fatalf("unexpected in-position indicator example: %s", providerExampleInPositionIndicator())
	}
	if !strings.Contains(providerExampleInPositionStructure(), `"monitor_tag":"keep"`) {
		t.Fatalf("unexpected in-position structure example: %s", providerExampleInPositionStructure())
	}
}

func TestProviderPromptsCarryMovementFieldsAndDataAnchors(t *testing.T) {
	builder := LLMPromptBuilder{
		ProviderIndicatorSystem: "provider-indicator-system",
		UserFormat:              UserPromptFormatBullet,
	}

	ind := agent.IndicatorSummary{
		Expansion:          agent.ExpansionExpanding,
		Alignment:          agent.AlignmentAligned,
		Noise:              agent.NoiseLow,
		MomentumDetail:     "rsi_slope_state=rising",
		ConflictDetail:     "未观察到明显冲突",
		MovementScore:      0.42,
		MovementConfidence: 0.73,
	}
	prompts, err := builder.ProviderPrompts(ind, agent.StructureSummary{}, agent.MechanicsSummary{}, decision.AgentEnabled{Indicator: true}, decision.ProviderDataContext{
		IndicatorCrossTF: &decision.IndicatorCrossTFContext{
			DecisionTFBias: "up",
			Alignment:      "aligned",
		},
	})
	if err != nil {
		t.Fatalf("ProviderPrompts: %v", err)
	}
	if !strings.Contains(prompts.IndicatorUser, `- movement_score: 0.42`) {
		t.Fatalf("provider user missing movement_score: %s", prompts.IndicatorUser)
	}
	if !strings.Contains(prompts.IndicatorUser, `- movement_confidence: 0.73`) {
		t.Fatalf("provider user missing movement_confidence: %s", prompts.IndicatorUser)
	}
	if !strings.Contains(prompts.IndicatorUser, `代码计算数据锚点(仅供交叉验证):`) {
		t.Fatalf("provider user missing data anchor block: %s", prompts.IndicatorUser)
	}
	if !strings.Contains(prompts.IndicatorUser, `- decision_tf_bias: "up"`) {
		t.Fatalf("provider user missing indicator anchor payload: %s", prompts.IndicatorUser)
	}
	if !strings.Contains(prompts.IndicatorUser, "约束:\n") {
		t.Fatalf("provider user missing constraint block label: %s", prompts.IndicatorUser)
	}
	if !strings.Contains(prompts.IndicatorUser, `最终输出必须完全基于本轮输入独立生成。`) {
		t.Fatalf("provider user missing shared constraint payload: %s", prompts.IndicatorUser)
	}
}
