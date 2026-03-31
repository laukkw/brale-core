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
	"brale-core/internal/decision/fund"
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
	Symbol               string
	Direction            string
	Entry                float64
	RiskPct              float64
	PlanSummary          map[string]any
	Consensus            map[string]any
	Structure            map[string]any
	OtherProviderSummary map[string]any
}

type TightenRiskPromptInput struct {
	Symbol              string
	Direction           string
	Entry               float64
	MarkPrice           float64
	ATR                 float64
	CurrentStopLoss     float64
	CurrentTakeProfits  []float64
	Gate                fund.GateDecision
	InPositionIndicator provider.InPositionIndicatorOut
	InPositionStructure provider.InPositionStructureOut
	InPositionMechanics provider.InPositionMechanicsOut
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
	consensusRaw, _ := json.Marshal(input.Consensus)
	structureRaw, _ := json.Marshal(input.Structure)
	providerRaw, _ := json.Marshal(input.OtherProviderSummary)
	user := formatPayloads(
		b.UserFormat,
		payloadBlock{label: "交易上下文(必填):", payload: string(contextRaw)},
		payloadBlock{label: "计划摘要(必填):", payload: string(planRaw)},
		payloadBlock{label: "共识摘要(必填):", payload: string(consensusRaw)},
		payloadBlock{label: "结构摘要(必填):", payload: string(structureRaw)},
		payloadBlock{label: "其他 Provider 摘要(必填):", payload: string(providerRaw)},
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
		"current_stop_loss":    input.CurrentStopLoss,
		"current_take_profits": append([]float64(nil), input.CurrentTakeProfits...),
	})
	gateRaw, _ := json.Marshal(input.Gate)
	inPosRaw, _ := json.Marshal(map[string]any{
		"indicator": input.InPositionIndicator,
		"structure": input.InPositionStructure,
		"mechanics": input.InPositionMechanics,
	})
	user := formatPayloads(
		b.UserFormat,
		payloadBlock{label: "仓位风控上下文(必填):", payload: string(contextRaw)},
		payloadBlock{label: "Gate 摘要(必填):", payload: string(gateRaw)},
		payloadBlock{label: "In-position 评估(必填):", payload: string(inPosRaw)},
		payloadBlock{label: "输出要求:", payload: `{"stop_loss":0.0,"take_profits":[0.0]}。必须输出完整新止损与止盈列表，不允许省略任一字段。`},
	)
	return system, user, nil
}

func (b LLMPromptBuilder) AgentIndicatorPrompt(ind features.IndicatorJSON) (string, string, error) {
	system, err := requirePrompt("prompts.agent.indicator", b.AgentIndicatorSystem)
	if err != nil {
		return "", "", err
	}
	user := formatPayloads(b.UserFormat, payloadBlock{label: "Indicator 输入", payload: string(ind.RawJSON)})
	return system, user, nil
}

func (b LLMPromptBuilder) AgentStructurePrompt(tr features.TrendJSON) (string, string, error) {
	system, err := requirePrompt("prompts.agent.structure", b.AgentStructureSystem)
	if err != nil {
		return "", "", err
	}
	user := formatPayloads(
		b.UserFormat,
		payloadBlock{label: "Trend 输入", payload: string(tr.RawJSON)},
	)
	return system, user, nil
}

func (b LLMPromptBuilder) AgentMechanicsPrompt(mech features.MechanicsSnapshot) (string, string, error) {
	system, err := requirePrompt("prompts.agent.mechanics", b.AgentMechanicsSystem)
	if err != nil {
		return "", "", err
	}
	user := formatPayloads(b.UserFormat, payloadBlock{label: "Mechanics 输入", payload: string(mech.RawJSON)})
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

func (b LLMPromptBuilder) ProviderPrompts(ind agent.IndicatorSummary, st agent.StructureSummary, mech agent.MechanicsSummary, enabled decision.AgentEnabled) (ProviderPromptSet, error) {
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
		indicatorUser = buildProviderUser(b.UserFormat, providerSummary{Indicator: &indSummary}, providerExampleIndicator())
	}
	if enabled.Structure {
		stSummary := toProviderStructureSummary(st)
		structureUser = buildProviderUser(b.UserFormat, providerSummary{Structure: &stSummary}, providerExampleStructure())
	}
	if enabled.Mechanics {
		mechSummary := toProviderMechanicsSummary(mech)
		mechanicsUser = buildProviderUser(b.UserFormat, providerSummary{Mechanics: &mechSummary}, providerExampleMechanics())
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

func (b LLMPromptBuilder) InPositionProviderPrompts(ind agent.IndicatorSummary, st agent.StructureSummary, mech agent.MechanicsSummary, summary positionprompt.Summary, enabled decision.AgentEnabled) (InPositionPromptSet, error) {
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
		indicatorUser = buildInPositionProviderUser(b.UserFormat, providerSummary{Indicator: &indSummary}, summary, providerExampleInPositionIndicator())
	}
	if enabled.Structure {
		stSummary := toProviderStructureSummary(st)
		structureUser = buildInPositionProviderUser(b.UserFormat, providerSummary{Structure: &stSummary}, summary, providerExampleInPositionStructure())
	}
	if enabled.Mechanics {
		mechSummary := toProviderMechanicsSummary(mech)
		mechanicsUser = buildInPositionProviderUser(b.UserFormat, providerSummary{Mechanics: &mechSummary}, summary, providerExampleInPositionMechanics())
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
	Expansion      agent.Expansion `json:"expansion"`
	Alignment      agent.Alignment `json:"alignment"`
	Noise          agent.Noise     `json:"noise"`
	MomentumDetail string          `json:"momentum_detail"`
	ConflictDetail string          `json:"conflict_detail"`
}

type providerStructureSummary struct {
	Regime         agent.Regime    `json:"regime"`
	LastBreak      agent.LastBreak `json:"last_break"`
	Quality        agent.Quality   `json:"quality"`
	Pattern        agent.Pattern   `json:"pattern"`
	VolumeAction   string          `json:"volume_action"`
	CandleReaction string          `json:"candle_reaction"`
}

type providerMechanicsSummary struct {
	LeverageState       agent.LeverageState `json:"leverage_state"`
	Crowding            agent.Crowding      `json:"crowding"`
	RiskLevel           agent.RiskLevel     `json:"risk_level"`
	OpenInterestContext string              `json:"open_interest_context"`
	AnomalyDetail       string              `json:"anomaly_detail"`
}

func toProviderIndicatorSummary(ind agent.IndicatorSummary) providerIndicatorSummary {
	return providerIndicatorSummary{
		Expansion:      ind.Expansion,
		Alignment:      ind.Alignment,
		Noise:          ind.Noise,
		MomentumDetail: ind.MomentumDetail,
		ConflictDetail: ind.ConflictDetail,
	}
}

func toProviderStructureSummary(st agent.StructureSummary) providerStructureSummary {
	return providerStructureSummary{
		Regime:         st.Regime,
		LastBreak:      st.LastBreak,
		Quality:        st.Quality,
		Pattern:        st.Pattern,
		VolumeAction:   st.VolumeAction,
		CandleReaction: st.CandleReaction,
	}
}

func toProviderMechanicsSummary(mech agent.MechanicsSummary) providerMechanicsSummary {
	return providerMechanicsSummary{
		LeverageState:       mech.LeverageState,
		Crowding:            mech.Crowding,
		RiskLevel:           mech.RiskLevel,
		OpenInterestContext: mech.OpenInterestContext,
		AnomalyDetail:       mech.AnomalyDetail,
	}
}

func buildProviderUser(format UserPromptFormat, summary providerSummary, example string) string {
	raw, _ := json.Marshal(summary)
	return formatPayloads(
		format,
		payloadBlock{label: "摘要输入:", payload: string(raw)},
		payloadBlock{label: "输出示例(JSON):", payload: example},
	)
}

func buildInPositionProviderUser(format UserPromptFormat, summary providerSummary, pos positionprompt.Summary, example string) string {
	raw, _ := json.Marshal(summary)
	posRaw, _ := json.Marshal(pos)
	return formatPayloads(
		format,
		payloadBlock{label: "摘要输入:", payload: string(raw)},
		payloadBlock{label: "仓位摘要:", payload: string(posRaw)},
		payloadBlock{label: "约束: 仅输出固定字段 JSON；禁止编造/新增字段或阈值；允许原样引用输入中已有的 field=value 作为审计依据。", payload: ""},
		payloadBlock{label: "输出示例(JSON):", payload: example},
	)
}

func providerExampleIndicator() string {
	ex := provider.IndicatorProviderOut{
		MomentumExpansion: false,
		Alignment:         false,
		MeanRevNoise:      false,
		SignalTag:         "momentum_weak",
	}

	raw, _ := json.Marshal(ex)
	return string(raw)
}

func providerExampleStructure() string {
	ex := provider.StructureProviderOut{
		ClearStructure: true,
		Integrity:      true,
		Reason:         "regime=trend_up; quality=mixed; last_break=bos_up（示例仅示意字段结构与引用格式）",
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
			Reason:     "crowding.long_short_ratio=1.4; oi_history.4h.change_pct=3.2，更多像缩仓或等待确认而非直接回避（示例仅示意字段结构与引用格式）",
		},
		SignalTag: "crowded_long",
	}

	raw, _ := json.Marshal(ex)
	return string(raw)
}

func providerExampleInPositionIndicator() string {
	ex := provider.InPositionIndicatorOut{
		MomentumSustaining: true,
		DivergenceDetected: false,
		Reason:             "ok",
		MonitorTag:         "keep",
	}
	raw, _ := json.Marshal(ex)
	return string(raw)
}

func providerExampleInPositionStructure() string {
	ex := provider.InPositionStructureOut{
		Integrity:   true,
		ThreatLevel: provider.ThreatLevelNone,
		Reason:      "ok",
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
	if err := requireNonEmptyMap("flat_risk.consensus", input.Consensus); err != nil {
		return err
	}
	if err := requireNonEmptyMap("flat_risk.structure", input.Structure); err != nil {
		return err
	}
	if err := requireNonEmptyMap("flat_risk.other_provider_summary", input.OtherProviderSummary); err != nil {
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
