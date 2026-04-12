// 本文件主要职责：构建 LLM 提示词，按 agent/provider 阶段使用配置内 system prompt 并拼装输入。
// 本文件主要内容：构建 LLM prompts 与示例输出。

package llmapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"brale-core/internal/decision"
	"brale-core/internal/decision/agent"
	"brale-core/internal/decision/features"
	"brale-core/internal/decision/provider"
	"brale-core/internal/prompt/positionprompt"
)

type LLMPromptBuilder struct {
	AgentIndicatorSystem      string
	AgentStructureSystem      string
	AgentMechanicsSystem      string
	ProviderIndicatorSystem   string
	ProviderStructureSystem   string
	ProviderMechanicsSystem   string
	ProviderInPosIndicatorSys string
	ProviderInPosStructureSys string
	ProviderInPosMechanicsSys string
	RiskFlatInitSystem        string
	RiskTightenSystem         string
	UserFormat                UserPromptFormat
}

type FlatRiskPromptInput struct {
	Symbol           string
	Direction        string
	Entry            float64
	RiskPct          float64
	PlanSummary      map[string]any
	StructureAnchors map[string]any
	AgentIndicator   agent.IndicatorSummary
	AgentStructure   agent.StructureSummary
	AgentMechanics   agent.MechanicsSummary
}

type TightenRiskPromptInput struct {
	Symbol             string
	Direction          string
	Entry              float64
	MarkPrice          float64
	ATR                float64
	UnrealizedPnlPct   float64
	PositionAgeMin     int64
	TP1Hit             bool
	DistanceToLiqPct   float64
	CurrentStopLoss    float64
	CurrentTakeProfits []float64
	StructureAnchors   map[string]any
	AgentIndicator     agent.IndicatorSummary
	AgentStructure     agent.StructureSummary
	AgentMechanics     agent.MechanicsSummary
}

func (b LLMPromptBuilder) FlatRiskInitPrompt(input FlatRiskPromptInput) (string, string, error) {
	system, err := requirePrompt("prompts.risk.flat_init", b.RiskFlatInitSystem)
	if err != nil {
		return "", "", err
	}
	if err := validateFlatRiskPromptInput(input); err != nil {
		return "", "", err
	}
	contextRaw, _ := json.Marshal(map[string]any{
		"symbol":    strings.ToUpper(strings.TrimSpace(input.Symbol)),
		"direction": strings.ToLower(strings.TrimSpace(input.Direction)),
		"entry":     input.Entry,
		"risk_pct":  input.RiskPct,
	})
	planRaw, _ := json.Marshal(input.PlanSummary)
	anchorsRaw, _ := json.Marshal(input.StructureAnchors)
	indicatorRaw, _ := json.Marshal(input.AgentIndicator)
	structureRaw, _ := json.Marshal(input.AgentStructure)
	mechanicsRaw, _ := json.Marshal(input.AgentMechanics)
	user := formatPayloads(
		b.UserFormat,
		payloadBlock{label: "交易上下文(必填):", payload: string(contextRaw)},
		payloadBlock{label: "计划摘要(必填):", payload: string(planRaw)},
		payloadBlock{label: "结构锚点摘要(必填):", payload: string(anchorsRaw)},
		payloadBlock{label: "Indicator Agent 摘要(必填):", payload: string(indicatorRaw)},
		payloadBlock{label: "Structure Agent 摘要(必填):", payload: string(structureRaw)},
		payloadBlock{label: "Mechanics Agent 摘要(必填):", payload: string(mechanicsRaw)},
	)
	return system, user, nil
}

func (b LLMPromptBuilder) TightenRiskUpdatePrompt(input TightenRiskPromptInput) (string, string, error) {
	system, err := requirePrompt("prompts.risk.tighten_update", b.RiskTightenSystem)
	if err != nil {
		return "", "", err
	}
	if err := validateTightenRiskPromptInput(input); err != nil {
		return "", "", err
	}
	contextRaw, _ := json.Marshal(map[string]any{
		"symbol":               strings.ToUpper(strings.TrimSpace(input.Symbol)),
		"direction":            strings.ToLower(strings.TrimSpace(input.Direction)),
		"entry":                input.Entry,
		"mark_price":           input.MarkPrice,
		"atr":                  input.ATR,
		"unrealized_pnl_pct":   input.UnrealizedPnlPct,
		"position_age_minutes": input.PositionAgeMin,
		"tp1_hit":              input.TP1Hit,
		"distance_to_liq_pct":  input.DistanceToLiqPct,
		"current_stop_loss":    input.CurrentStopLoss,
		"current_take_profits": append([]float64(nil), input.CurrentTakeProfits...),
	})
	anchorsRaw, _ := json.Marshal(input.StructureAnchors)
	indicatorRaw, _ := json.Marshal(input.AgentIndicator)
	structureRaw, _ := json.Marshal(input.AgentStructure)
	mechanicsRaw, _ := json.Marshal(input.AgentMechanics)
	user := formatPayloads(
		b.UserFormat,
		payloadBlock{label: "仓位风控上下文(必填):", payload: string(contextRaw)},
		payloadBlock{label: "结构锚点摘要(必填):", payload: string(anchorsRaw)},
		payloadBlock{label: "Indicator Agent 摘要(必填):", payload: string(indicatorRaw)},
		payloadBlock{label: "Structure Agent 摘要(必填):", payload: string(structureRaw)},
		payloadBlock{label: "Mechanics Agent 摘要(必填):", payload: string(mechanicsRaw)},
		payloadBlock{label: "输出要求:", payload: `{"stop_loss":0.0,"take_profits":[0.0]}。必须输出完整新止损与止盈列表，不允许省略任一字段。`},
	)
	return system, user, nil
}

func (b LLMPromptBuilder) AgentIndicatorPrompt(ind features.IndicatorJSON, decisionInterval string) (string, string, error) {
	system, err := requirePrompt("prompts.agent.indicator", b.AgentIndicatorSystem)
	if err != nil {
		return "", "", err
	}
	blocks := []payloadBlock{{label: "Indicator 输入", payload: string(ind.RawJSON)}}
	if interval := strings.TrimSpace(decisionInterval); interval != "" {
		blocks = append(blocks, payloadBlock{label: "决策窗口:", payload: interval})
	}
	user := formatPayloads(b.UserFormat, blocks...)
	return system, user, nil
}

func (b LLMPromptBuilder) AgentStructurePrompt(tr features.TrendJSON, decisionInterval string) (string, string, error) {
	system, err := requirePrompt("prompts.agent.structure", b.AgentStructureSystem)
	if err != nil {
		return "", "", err
	}
	blocks := []payloadBlock{{label: "Trend 输入", payload: string(tr.RawJSON)}}
	if interval := strings.TrimSpace(decisionInterval); interval != "" {
		blocks = append(blocks, payloadBlock{label: "决策窗口:", payload: interval})
	}
	user := formatPayloads(b.UserFormat, blocks...)
	return system, user, nil
}

func (b LLMPromptBuilder) AgentMechanicsPrompt(mech features.MechanicsSnapshot, decisionInterval string) (string, string, error) {
	system, err := requirePrompt("prompts.agent.mechanics", b.AgentMechanicsSystem)
	if err != nil {
		return "", "", err
	}
	blocks := []payloadBlock{{label: "Mechanics 输入", payload: string(mech.RawJSON)}}
	if interval := strings.TrimSpace(decisionInterval); interval != "" {
		blocks = append(blocks, payloadBlock{label: "决策窗口:", payload: interval})
	}
	user := formatPayloads(b.UserFormat, blocks...)
	return system, user, nil
}

type ProviderPromptSet struct {
	IndicatorSys  string
	StructureSys  string
	MechanicsSys  string
	IndicatorUser string
	StructureUser string
	MechanicsUser string
}

func (b LLMPromptBuilder) ProviderPrompts(ind agent.IndicatorSummary, st agent.StructureSummary, mech agent.MechanicsSummary, enabled decision.AgentEnabled, dataCtx decision.ProviderDataContext) (ProviderPromptSet, error) {
	var indicatorSys string
	var structureSys string
	var mechanicsSys string
	var err error
	if enabled.Indicator {
		indicatorSys, err = requirePrompt("prompts.provider.indicator", b.ProviderIndicatorSystem)
		if err != nil {
			return ProviderPromptSet{}, err
		}
	}
	if enabled.Structure {
		structureSys, err = requirePrompt("prompts.provider.structure", b.ProviderStructureSystem)
		if err != nil {
			return ProviderPromptSet{}, err
		}
	}
	if enabled.Mechanics {
		mechanicsSys, err = requirePrompt("prompts.provider.mechanics", b.ProviderMechanicsSystem)
		if err != nil {
			return ProviderPromptSet{}, err
		}
	}
	indicatorUser := ""
	structureUser := ""
	mechanicsUser := ""
	if enabled.Indicator {
		indSummary := toProviderIndicatorSummary(ind)
		indicatorUser = buildProviderUserWithData(b.UserFormat, providerSummary{Indicator: &indSummary}, dataCtx.IndicatorCrossTF, providerExampleIndicator())
	}
	if enabled.Structure {
		stSummary := toProviderStructureSummary(st)
		structureUser = buildProviderUserWithData(b.UserFormat, providerSummary{Structure: &stSummary}, dataCtx.StructureAnchorCtx, providerExampleStructure())
	}
	if enabled.Mechanics {
		mechSummary := toProviderMechanicsSummary(mech)
		mechanicsUser = buildProviderUserWithData(b.UserFormat, providerSummary{Mechanics: &mechSummary}, dataCtx.MechanicsCtx, providerExampleMechanics())
	}
	return ProviderPromptSet{
		IndicatorSys:  indicatorSys,
		StructureSys:  structureSys,
		MechanicsSys:  mechanicsSys,
		IndicatorUser: indicatorUser,
		StructureUser: structureUser,
		MechanicsUser: mechanicsUser,
	}, nil
}

type InPositionPromptSet struct {
	IndicatorSys  string
	StructureSys  string
	MechanicsSys  string
	IndicatorUser string
	StructureUser string
	MechanicsUser string
}

func (b LLMPromptBuilder) InPositionProviderPrompts(ind agent.IndicatorSummary, st agent.StructureSummary, mech agent.MechanicsSummary, summary positionprompt.Summary, enabled decision.AgentEnabled, dataCtx decision.ProviderDataContext) (InPositionPromptSet, error) {
	var indicatorSys string
	var structureSys string
	var mechanicsSys string
	var err error
	if enabled.Indicator {
		indicatorSys, err = requirePrompt("prompts.provider_in_position.indicator", b.ProviderInPosIndicatorSys)
		if err != nil {
			return InPositionPromptSet{}, err
		}
	}
	if enabled.Structure {
		structureSys, err = requirePrompt("prompts.provider_in_position.structure", b.ProviderInPosStructureSys)
		if err != nil {
			return InPositionPromptSet{}, err
		}
	}
	if enabled.Mechanics {
		mechanicsSys, err = requirePrompt("prompts.provider_in_position.mechanics", b.ProviderInPosMechanicsSys)
		if err != nil {
			return InPositionPromptSet{}, err
		}
	}
	indicatorUser := ""
	structureUser := ""
	mechanicsUser := ""
	if enabled.Indicator {
		indSummary := toProviderIndicatorSummary(ind)
		indicatorUser = buildInPositionProviderUserWithData(b.UserFormat, providerSummary{Indicator: &indSummary}, summary, dataCtx.IndicatorCrossTF, providerExampleInPositionIndicator())
	}
	if enabled.Structure {
		stSummary := toProviderStructureSummary(st)
		structureUser = buildInPositionProviderUserWithData(b.UserFormat, providerSummary{Structure: &stSummary}, summary, dataCtx.StructureAnchorCtx, providerExampleInPositionStructure())
	}
	if enabled.Mechanics {
		mechSummary := toProviderMechanicsSummary(mech)
		mechanicsUser = buildInPositionProviderUserWithData(b.UserFormat, providerSummary{Mechanics: &mechSummary}, summary, dataCtx.MechanicsCtx, providerExampleInPositionMechanics())
	}
	return InPositionPromptSet{
		IndicatorSys:  indicatorSys,
		StructureSys:  structureSys,
		MechanicsSys:  mechanicsSys,
		IndicatorUser: indicatorUser,
		StructureUser: structureUser,
		MechanicsUser: mechanicsUser,
	}, nil
}

func joinUser(lines ...string) string {
	return strings.Join(lines, "\n")
}

type providerSummary struct {
	Indicator *providerIndicatorSummary `json:"indicator,omitempty"`
	Structure *providerStructureSummary `json:"structure,omitempty"`
	Mechanics *providerMechanicsSummary `json:"mechanics,omitempty"`
}

type providerIndicatorSummary struct {
	Expansion          agent.Expansion `json:"expansion"`
	Alignment          agent.Alignment `json:"alignment"`
	Noise              agent.Noise     `json:"noise"`
	MomentumDetail     string          `json:"momentum_detail"`
	ConflictDetail     string          `json:"conflict_detail"`
	MovementScore      float64         `json:"movement_score"`
	MovementConfidence float64         `json:"movement_confidence"`
}

type providerStructureSummary struct {
	Regime             agent.Regime    `json:"regime"`
	LastBreak          agent.LastBreak `json:"last_break"`
	Quality            agent.Quality   `json:"quality"`
	Pattern            agent.Pattern   `json:"pattern"`
	VolumeAction       string          `json:"volume_action"`
	CandleReaction     string          `json:"candle_reaction"`
	MovementScore      float64         `json:"movement_score"`
	MovementConfidence float64         `json:"movement_confidence"`
}

type providerMechanicsSummary struct {
	LeverageState       agent.LeverageState `json:"leverage_state"`
	Crowding            agent.Crowding      `json:"crowding"`
	RiskLevel           agent.RiskLevel     `json:"risk_level"`
	OpenInterestContext string              `json:"open_interest_context"`
	AnomalyDetail       string              `json:"anomaly_detail"`
	MovementScore       float64             `json:"movement_score"`
	MovementConfidence  float64             `json:"movement_confidence"`
}

func toProviderIndicatorSummary(ind agent.IndicatorSummary) providerIndicatorSummary {
	return providerIndicatorSummary{
		Expansion:          ind.Expansion,
		Alignment:          ind.Alignment,
		Noise:              ind.Noise,
		MomentumDetail:     ind.MomentumDetail,
		ConflictDetail:     ind.ConflictDetail,
		MovementScore:      ind.MovementScore,
		MovementConfidence: ind.MovementConfidence,
	}
}

func toProviderStructureSummary(st agent.StructureSummary) providerStructureSummary {
	return providerStructureSummary{
		Regime:             st.Regime,
		LastBreak:          st.LastBreak,
		Quality:            st.Quality,
		Pattern:            st.Pattern,
		VolumeAction:       st.VolumeAction,
		CandleReaction:     st.CandleReaction,
		MovementScore:      st.MovementScore,
		MovementConfidence: st.MovementConfidence,
	}
}

func toProviderMechanicsSummary(mech agent.MechanicsSummary) providerMechanicsSummary {
	return providerMechanicsSummary{
		LeverageState:       mech.LeverageState,
		Crowding:            mech.Crowding,
		RiskLevel:           mech.RiskLevel,
		OpenInterestContext: mech.OpenInterestContext,
		AnomalyDetail:       mech.AnomalyDetail,
		MovementScore:       mech.MovementScore,
		MovementConfidence:  mech.MovementConfidence,
	}
}

const providerDataAnchorLabel = "代码计算数据锚点(仅供交叉验证):"

func appendProviderDataAnchor(blocks []payloadBlock, dataCtx any) []payloadBlock {
	if dataCtx == nil {
		return blocks
	}
	dataRaw, _ := json.Marshal(dataCtx)
	if len(dataRaw) <= 2 { // not just "{}" or "null"
		return blocks
	}
	return append(blocks, payloadBlock{label: providerDataAnchorLabel, payload: string(dataRaw)})
}

func providerConstraintPayload(inPosition bool) string {
	if inPosition {
		return "仅输出固定字段 JSON；禁止编造/新增字段或阈值；允许原样引用输入中已有的 field=value 作为审计依据。数据锚点仅用于交叉验证Agent摘要的一致性，不作为独立判断依据。最终输出必须完全基于本轮输入独立生成。"
	}
	return "输出示例 JSON 仅用于展示固定字段结构、字段类型与引用格式；禁止直接引用、复制、改写或沿用示例中的任何结论、reason、tag、阈值、布尔值、置信度或措辞。最终输出必须完全基于本轮输入独立生成。数据锚点仅用于交叉验证Agent摘要的一致性，不作为独立判断依据。"
}

func buildProviderUserWithData(format UserPromptFormat, summary providerSummary, dataCtx any, example string) string {
	raw, _ := json.Marshal(summary)
	blocks := []payloadBlock{{label: "摘要输入:", payload: string(raw)}}
	blocks = appendProviderDataAnchor(blocks, dataCtx)
	blocks = append(blocks,
		payloadBlock{label: "约束:", payload: providerConstraintPayload(false)},
		payloadBlock{label: "输出示例(JSON):", payload: example},
	)
	return formatPayloads(format, blocks...)
}

func buildInPositionProviderUserWithData(format UserPromptFormat, summary providerSummary, pos positionprompt.Summary, dataCtx any, example string) string {
	raw, _ := json.Marshal(summary)
	posRaw, _ := json.Marshal(pos)
	blocks := []payloadBlock{
		{label: "摘要输入:", payload: string(raw)},
		{label: "仓位摘要:", payload: string(posRaw)},
	}
	blocks = appendProviderDataAnchor(blocks, dataCtx)
	blocks = append(blocks,
		payloadBlock{label: "约束:", payload: providerConstraintPayload(true)},
		payloadBlock{label: "输出示例(JSON):", payload: example},
	)
	return formatPayloads(format, blocks...)
}

func providerExampleIndicator() string {
	ex := provider.IndicatorProviderOut{
		MomentumExpansion: false,
		Alignment:         false,
		MeanRevNoise:      false,
		SignalTag:         "noise",
	}

	raw, _ := json.Marshal(ex)
	return string(raw)
}

func providerExampleStructure() string {
	ex := provider.StructureProviderOut{
		ClearStructure: true,
		Integrity:      true,
		Reason:         "引用本轮输入中的关键字段作为依据并说明判断逻辑（示例占位，禁止直接引用）",
		SignalTag:      "support_retest",
	}

	raw, _ := json.Marshal(ex)
	return string(raw)
}

func providerExampleMechanics() string {
	ex := provider.MechanicsProviderOut{
		LiquidationStress: provider.SemanticSignal{
			Value:      true,
			Confidence: provider.ConfidenceLow,
			Reason:     "引用本轮输入中的关键字段作为依据并说明判断逻辑（示例占位，禁止直接引用）",
		},
		SignalTag: "neutral",
	}

	raw, _ := json.Marshal(ex)
	return string(raw)
}

func providerExampleInPositionIndicator() string {
	ex := provider.InPositionIndicatorOut{
		MomentumSustaining: true,
		DivergenceDetected: false,
		Reason:             "引用本轮输入中的关键字段作为依据并说明判断逻辑（示例占位，禁止直接引用）",
		MonitorTag:         "keep",
	}
	raw, _ := json.Marshal(ex)
	return string(raw)
}

func providerExampleInPositionStructure() string {
	ex := provider.InPositionStructureOut{
		Integrity:   true,
		ThreatLevel: provider.ThreatLevelNone,
		Reason:      "引用本轮输入中的关键字段作为依据并说明判断逻辑（示例占位，禁止直接引用）",
		MonitorTag:  "keep",
	}
	raw, _ := json.Marshal(ex)
	return string(raw)
}

func providerExampleInPositionMechanics() string {
	ex := provider.InPositionMechanicsOut{
		AdverseLiquidation: false,
		CrowdingReversal:   false,
		Reason:             "ok",
		MonitorTag:         "keep",
	}
	raw, _ := json.Marshal(ex)
	return string(raw)
}

func requirePrompt(name string, value string) (string, error) {
	prompt := strings.TrimSpace(value)
	if prompt == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return prompt, nil
}

func validateFlatRiskPromptInput(input FlatRiskPromptInput) error {
	if strings.TrimSpace(input.Symbol) == "" {
		return fmt.Errorf("flat_risk.symbol is required")
	}
	direction := strings.ToLower(strings.TrimSpace(input.Direction))
	if direction != "long" && direction != "short" {
		return fmt.Errorf("flat_risk.direction must be long/short")
	}
	if input.Entry <= 0 {
		return fmt.Errorf("flat_risk.entry is required")
	}
	if input.RiskPct <= 0 {
		return fmt.Errorf("flat_risk.risk_pct is required")
	}
	if err := requireNonEmptyMap("flat_risk.plan_summary", input.PlanSummary); err != nil {
		return err
	}
	if err := requireNonEmptyMap("flat_risk.structure_anchors", input.StructureAnchors); err != nil {
		return err
	}
	return nil
}

func validateTightenRiskPromptInput(input TightenRiskPromptInput) error {
	if strings.TrimSpace(input.Symbol) == "" {
		return fmt.Errorf("tighten_risk.symbol is required")
	}
	direction := strings.ToLower(strings.TrimSpace(input.Direction))
	if direction != "long" && direction != "short" {
		return fmt.Errorf("tighten_risk.direction must be long/short")
	}
	if input.Entry <= 0 {
		return fmt.Errorf("tighten_risk.entry is required")
	}
	if input.MarkPrice <= 0 {
		return fmt.Errorf("tighten_risk.mark_price is required")
	}
	if input.ATR <= 0 {
		return fmt.Errorf("tighten_risk.atr is required")
	}
	if input.CurrentStopLoss <= 0 {
		return fmt.Errorf("tighten_risk.current_stop_loss is required")
	}
	if len(input.CurrentTakeProfits) == 0 {
		return fmt.Errorf("tighten_risk.current_take_profits is required")
	}
	if err := requireNonEmptyMap("tighten_risk.structure_anchors", input.StructureAnchors); err != nil {
		return err
	}
	return nil
}

func requireNonEmptyMap(name string, value map[string]any) error {
	if len(value) == 0 {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}

type UserPromptFormat string

const (
	UserPromptFormatJSON     UserPromptFormat = "json"
	UserPromptFormatMarkdown UserPromptFormat = "markdown"
	UserPromptFormatText     UserPromptFormat = "text"
	UserPromptFormatBullet   UserPromptFormat = "bullet"
)

type payloadBlock struct {
	label   string
	payload string
}

func formatPayloads(format UserPromptFormat, blocks ...payloadBlock) string {
	if len(blocks) == 0 {
		return ""
	}
	f := format
	if f == "" {
		f = UserPromptFormatJSON
	}
	var parts []string
	for _, block := range blocks {
		content := strings.TrimSpace(block.payload)
		label := strings.TrimSpace(block.label)
		switch f {
		case UserPromptFormatBullet:
			if content == "" {
				parts = append(parts, label)
				continue
			}
			markdown, ok := renderAsMarkdownList(content)
			if !ok {
				parts = append(parts, fmt.Sprintf("%s\n%s", label, content))
				continue
			}
			parts = append(parts, fmt.Sprintf("%s\n%s", label, markdown))
		case UserPromptFormatMarkdown:
			if content == "" {
				parts = append(parts, label)
				continue
			}
			content = indentJSONIfPossible(content)
			parts = append(parts, fmt.Sprintf("%s\n```json\n%s\n```", label, content))
		case UserPromptFormatText:
			if content == "" {
				parts = append(parts, label)
				continue
			}
			if content != "" {
				content = indentJSONIfPossible(content)
			}
			parts = append(parts, fmt.Sprintf("%s\n%s", label, content))
		default:
			if content == "" {
				parts = append(parts, label)
				continue
			}
			if content != "" {
				content = indentJSONIfPossible(content)
			}
			parts = append(parts, joinUser(label, content))
		}
	}
	return strings.Join(parts, "\n")
}

func indentJSONIfPossible(payload string) string {
	if payload == "" {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(payload), "", "  "); err != nil {
		return payload
	}
	return buf.String()
}

func renderAsMarkdownList(payload string) (string, bool) {
	if payload == "" {
		return "", false
	}
	var v any
	if err := json.Unmarshal([]byte(payload), &v); err != nil {
		return "", false
	}
	var b strings.Builder
	renderValue(&b, v, 0, false)
	return strings.TrimSpace(b.String()), true
}

func renderValue(b *strings.Builder, v any, indent int, useIndex bool) {
	switch val := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			child := val[k]
			writeIndent(b, indent)
			b.WriteString("- ")
			b.WriteString(k)
			switch child.(type) {
			case map[string]any, []any:
				b.WriteString(":\n")
				renderValue(b, child, indent+2, false)
			default:
				b.WriteString(": ")
				b.WriteString(formatScalar(child))
				b.WriteString("\n")
			}
		}
	case []any:
		for i, child := range val {
			writeIndent(b, indent)
			if useIndex {
				fmt.Fprintf(b, "- [%d] ", i)
			} else {
				b.WriteString("- ")
			}
			switch child.(type) {
			case map[string]any, []any:
				fmt.Fprintf(b, "[%d]:\n", i)
				renderValue(b, child, indent+2, true)
			default:
				if useIndex {
					b.WriteString(formatScalar(child))
				} else {
					fmt.Fprintf(b, "[%d] %s", i, formatScalar(child))
				}
				b.WriteString("\n")
			}
		}
	default:
		writeIndent(b, indent)
		b.WriteString("- ")
		b.WriteString(formatScalar(val))
		b.WriteString("\n")
	}
}

func writeIndent(b *strings.Builder, indent int) {
	for range indent {
		b.WriteByte(' ')
	}
}

func formatScalar(v any) string {
	switch val := v.(type) {
	case string:
		return strconv.Quote(val)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(val)
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%v", val)
	}
}
