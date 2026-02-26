package ruleflow

import (
	"strings"

	"github.com/rulego/rulego/api/types"
)

type GateEntryNode struct{}

const (
	gateGradeNone   = 0
	gateGradeLow    = 1
	gateGradeMedium = 2
	gateGradeHigh   = 3

	gatePriorityConsensusFailed  = 0
	gatePriorityDataMissing      = 1
	gatePriorityStructBreak      = 1
	gatePriorityMechRisk         = 2
	gatePriorityIndicatorNoise   = 3
	gatePriorityIndicatorMixed   = 4
	gatePriorityTagInconsistent  = 5
	gatePriorityScriptMissing    = 6
	gatePriorityScriptNotAllowed = 7
	gatePrioritySieveOverride    = 8
	gatePriorityAllow            = 10
)

// GateDecisionNode exists for backward-compatible RuleGo chains.
// Some rule JSONs reference brale/gate_decision; we delegate to GateEntryNode.
type GateDecisionNode struct{}

func (n *GateDecisionNode) Type() string {
	return "brale/gate_decision"
}

func (n *GateDecisionNode) New() types.Node {
	return &GateDecisionNode{}
}

func (n *GateDecisionNode) Init(types.Config, types.Configuration) error {
	return nil
}

func (n *GateDecisionNode) Destroy() {}

func (n *GateDecisionNode) OnMsg(ctx types.RuleContext, msg types.RuleMsg) {
	(&GateEntryNode{}).OnMsg(ctx, msg)
}

func (n *GateEntryNode) Type() string {
	return "brale/gate_entry"
}

func (n *GateEntryNode) New() types.Node {
	return &GateEntryNode{}
}

func (n *GateEntryNode) Init(types.Config, types.Configuration) error {
	return nil
}

func (n *GateEntryNode) Destroy() {}

func (n *GateEntryNode) OnMsg(ctx types.RuleContext, msg types.RuleMsg) {
	root, err := readRoot(msg)
	if err != nil {
		ctx.TellFailure(msg, err)
		return
	}
	providers := toMap(root["providers"])
	providersEnabled := toMap(root["providers_enabled"])
	indicator := toMap(providers["indicator"])
	structure := toMap(providers["structure"])
	mechanics := toMap(providers["mechanics"])
	riskMgmt := toMap(root["risk_management"])
	binding := toMap(root["binding"])
	state := strings.ToUpper(strings.TrimSpace(toString(root["state"])))
	structureDirection := strings.ToLower(toString(toMap(root["structure"])["direction"]))

	indicatorTag := strings.ToLower(toString(indicator["signal_tag"]))
	structureTag := strings.ToLower(toString(structure["signal_tag"]))
	mechanicsTag := strings.ToLower(toString(mechanics["signal_tag"]))

	momentumExpansion := toBool(indicator["momentum_expansion"])
	alignment := toBool(indicator["alignment"])
	meanRevNoise := toBool(indicator["mean_rev_noise"])
	structureClear := toBool(structure["clear_structure"])
	structureIntegrity := toBool(structure["integrity"])
	liquidationStress := toBool(toMap(mechanics["liquidation_stress"])["value"])
	liqConfidence := strings.ToLower(toString(toMap(mechanics["liquidation_stress"])["confidence"]))
	crowdingAlign := (structureDirection == "long" && mechanicsTag == "crowded_long") || (structureDirection == "short" && mechanicsTag == "crowded_short")
	inputs := gateInputs{
		State:              state,
		StructureDirection: structureDirection,
		IndicatorTag:       indicatorTag,
		StructureTag:       structureTag,
		MechanicsTag:       mechanicsTag,
		MomentumExpansion:  momentumExpansion,
		Alignment:          alignment,
		MeanRevNoise:       meanRevNoise,
		StructureClear:     structureClear,
		StructureIntegrity: structureIntegrity,
		LiquidationStress:  liquidationStress,
		LiqConfidence:      liqConfidence,
	}
	missingProviders := resolveMissingProviders(providersEnabled, indicator, structure, mechanics)
	decision := evaluateGateDecision(inputs, missingProviders)

	gateActionBeforeSieve := decision.Action
	var sieveDecision sieveDecision
	if decision.Action == "ALLOW" {
		sieveDecision = resolveSieveDecision(riskMgmt, indicatorTag, structureTag, mechanicsTag, liqConfidence, decision.Direction, crowdingAlign)
		if sieveDecision.Action != "" && sieveDecision.Action != "ALLOW" {
			decision.Action = sieveDecision.Action
			decision.Reason = "SIEVE_POLICY"
			decision.Priority = gatePrioritySieveOverride
			decision.Grade = gateGradeNone
		}
	}

	ruleHit := map[string]any{
		"name":      decision.Reason,
		"priority":  decision.Priority,
		"action":    decision.Action,
		"reason":    decision.Reason,
		"grade":     decision.Grade,
		"direction": decision.Direction,
		"default":   false,
	}

	root["gate"] = map[string]any{
		"action":    decision.Action,
		"reason":    decision.Reason,
		"grade":     decision.Grade,
		"direction": decision.Direction,
		"tradeable": decision.Action == "ALLOW",
		"derived": map[string]any{
			"indicator_tag":             indicatorTag,
			"structure_tag":             structureTag,
			"mechanics_tag":             mechanicsTag,
			"crowding_align":            crowdingAlign,
			"gate_trace":                decision.GateTrace,
			"gate_stop_step":            decision.StopStep,
			"gate_stop_reason":          decision.StopReason,
			"gate_action_before_sieve":  gateActionBeforeSieve,
			"sieve_action":              sieveDecision.Action,
			"sieve_size_factor":         sieveDecision.SizeFactor,
			"sieve_reason":              sieveDecision.Reason,
			"sieve_hit":                 sieveDecision.Hit,
			"sieve_min_size_factor":     sieveDecision.MinSizeFactor,
			"sieve_default_action":      sieveDecision.DefaultAction,
			"sieve_default_size_factor": sieveDecision.DefaultSizeFactor,
			"sieve_policy_hash":         toString(binding["strategy_hash"]),
		},
		"rule_hit": ruleHit,
	}
	respondRuleMsgJSON(ctx, msg, root)
}

type gateInputs struct {
	State              string
	StructureDirection string
	IndicatorTag       string
	StructureTag       string
	MechanicsTag       string
	MomentumExpansion  bool
	Alignment          bool
	MeanRevNoise       bool
	StructureClear     bool
	StructureIntegrity bool
	LiquidationStress  bool
	LiqConfidence      string
}

type gateDecision struct {
	Action     string
	Reason     string
	Direction  string
	Grade      int
	Priority   int
	StopStep   string
	StopReason string
	GateTrace  []map[string]any
}

func resolveMissingProviders(providersEnabled, indicator, structure, mechanics map[string]any) bool {
	if len(providersEnabled) > 0 {
		return !toBool(providersEnabled["indicator"]) || !toBool(providersEnabled["structure"]) || !toBool(providersEnabled["mechanics"])
	}
	return len(indicator) == 0 || len(structure) == 0 || len(mechanics) == 0
}

func evaluateGateDecision(inputs gateInputs, missingProviders bool) gateDecision {
	eval := gateDecisionEvaluator{
		inputs:           inputs,
		missingProviders: missingProviders,
		decision:         gateDecision{Direction: inputs.StructureDirection, Grade: gateGradeNone},
	}
	eval.evaluate()
	eval.decision.GateTrace = eval.gateTrace
	return eval.decision
}

type gateDecisionEvaluator struct {
	inputs           gateInputs
	missingProviders bool
	decision         gateDecision
	gateTrace        []map[string]any
}

func (e *gateDecisionEvaluator) evaluate() {
	e.evalDirection()
	e.evalData()
	e.evalStructure()
	e.evalMechRisk()
	e.evalIndicatorNoise()
	e.evalStructureClear()
	e.evalTagConsistency()
	e.evalScript()
}

func (e *gateDecisionEvaluator) hasAction() bool {
	return strings.TrimSpace(e.decision.Action) != ""
}

func (e *gateDecisionEvaluator) appendGateTrace(step string, ok bool, code string) {
	entry := map[string]any{
		"step": step,
		"ok":   ok,
	}
	if strings.TrimSpace(code) != "" {
		entry["reason"] = code
	}
	e.gateTrace = append(e.gateTrace, entry)
}

func (e *gateDecisionEvaluator) setStop(step string, action string, code string, priority int) {
	e.decision.Action = action
	e.decision.Reason = code
	e.decision.Priority = priority
	e.decision.StopStep = step
	e.decision.StopReason = code
	e.appendGateTrace(step, false, code)
}

func (e *gateDecisionEvaluator) evalDirection() {
	if e.hasAction() {
		return
	}
	if e.inputs.StructureDirection == "" || e.inputs.StructureDirection == "none" {
		e.decision.Direction = "none"
		e.setStop("direction", "VETO", "CONSENSUS_NOT_PASSED", gatePriorityConsensusFailed)
		return
	}
	e.appendGateTrace("direction", true, "")
}

func (e *gateDecisionEvaluator) evalData() {
	if e.hasAction() {
		return
	}
	if !e.missingProviders || e.inputs.State == "IN_POSITION" {
		e.appendGateTrace("data", true, "")
		return
	}
	e.setStop("data", "VETO", "DATA_MISSING", gatePriorityDataMissing)
}

func (e *gateDecisionEvaluator) evalStructure() {
	if e.hasAction() {
		return
	}
	if !e.inputs.StructureIntegrity || e.inputs.StructureTag == "structure_broken" {
		e.setStop("structure", "VETO", "STRUCT_BREAK", gatePriorityStructBreak)
		return
	}
	e.appendGateTrace("structure", true, "")
}

func (e *gateDecisionEvaluator) evalMechRisk() {
	if e.hasAction() {
		return
	}
	if e.inputs.LiquidationStress || e.inputs.MechanicsTag == "liquidation_cascade" {
		e.setStop("mech_risk", "VETO", "MECH_RISK", gatePriorityMechRisk)
		return
	}
	e.appendGateTrace("mech_risk", true, "")
}

func (e *gateDecisionEvaluator) evalIndicatorNoise() {
	if e.hasAction() {
		return
	}
	if e.inputs.MeanRevNoise || e.inputs.IndicatorTag == "noise" {
		e.setStop("indicator_noise", "WAIT", "INDICATOR_NOISE", gatePriorityIndicatorNoise)
		return
	}
	e.appendGateTrace("indicator_noise", true, "")
}

func (e *gateDecisionEvaluator) evalStructureClear() {
	if e.hasAction() {
		return
	}
	if !e.inputs.StructureClear {
		e.setStop("structure_clear", "WAIT", "INDICATOR_MIXED", gatePriorityIndicatorMixed)
		return
	}
	e.appendGateTrace("structure_clear", true, "")
}

func (e *gateDecisionEvaluator) evalTagConsistency() {
	if e.hasAction() {
		return
	}
	if !resolveBoolTagConsistencyFromFlags(e.inputs.IndicatorTag, e.inputs.MomentumExpansion, e.inputs.Alignment, e.inputs.MeanRevNoise) {
		e.setStop("tag_consistency", "WAIT", "INDICATOR_MIXED", gatePriorityTagInconsistent)
		return
	}
	e.appendGateTrace("tag_consistency", true, "")
}

func (e *gateDecisionEvaluator) evalScript() {
	if e.hasAction() {
		return
	}
	script := resolveEntryScript(e.inputs.IndicatorTag, e.inputs.StructureTag)
	if script == "" {
		e.setStop("script_select", "WAIT", "INDICATOR_MIXED", gatePriorityScriptMissing)
		return
	}
	e.appendGateTrace("script_select", true, "")
	if !isEntryScriptAllowed(script, e.inputs.MomentumExpansion, e.inputs.Alignment, e.inputs.MeanRevNoise) {
		e.setStop("script_allowed", "WAIT", "INDICATOR_MIXED", gatePriorityScriptNotAllowed)
		return
	}
	e.appendGateTrace("script_allowed", true, "")
	e.decision.Action = "ALLOW"
	e.decision.Reason = "PASS_STRONG"
	e.decision.Grade = resolveEntryGrade(script)
	e.decision.Priority = gatePriorityAllow
	e.decision.StopStep = "gate_allow"
	e.decision.StopReason = e.decision.Reason
}

func resolveBoolTagConsistencyFromFlags(indicatorTag string, momentumExpansion, alignment, meanRevNoise bool) bool {
	switch indicatorTag {
	case "trend_surge":
		return momentumExpansion && alignment && !meanRevNoise
	case "pullback_entry":
		return !momentumExpansion && alignment && !meanRevNoise
	case "divergence_reversal":
		return !alignment && !meanRevNoise
	case "noise":
		return meanRevNoise
	default:
		return true
	}
}

func resolveEntryScript(indicatorTag, structureTag string) string {
	switch {
	case indicatorTag == "trend_surge" && structureTag == "breakout_confirmed":
		return "A"
	case indicatorTag == "pullback_entry" && structureTag == "support_retest":
		return "B"
	case indicatorTag == "divergence_reversal" && structureTag == "support_retest":
		return "C"
	case indicatorTag == "trend_surge" && structureTag == "support_retest":
		return "D"
	case indicatorTag == "pullback_entry" && structureTag == "breakout_confirmed":
		return "E"
	case indicatorTag == "divergence_reversal" && structureTag == "breakout_confirmed":
		return "F"
	default:
		return ""
	}
}

func isEntryScriptAllowed(script string, momentumExpansion, alignment, meanRevNoise bool) bool {
	switch script {
	case "A":
		return momentumExpansion && alignment && !meanRevNoise
	case "B":
		return !momentumExpansion && alignment && !meanRevNoise
	case "C":
		return !alignment && !meanRevNoise
	case "D":
		return momentumExpansion && alignment && !meanRevNoise
	case "E":
		return !momentumExpansion && alignment && !meanRevNoise
	case "F":
		return !alignment && !meanRevNoise
	default:
		return false
	}
}

func resolveEntryGrade(script string) int {
	switch script {
	case "A":
		return gateGradeHigh
	case "B":
		return gateGradeMedium
	case "C":
		return gateGradeLow
	case "D":
		return gateGradeMedium
	case "E":
		return gateGradeLow
	case "F":
		return gateGradeLow
	}
	return gateGradeNone
}

type sieveDecision struct {
	Action            string
	SizeFactor        float64
	Reason            string
	Hit               bool
	MinSizeFactor     float64
	DefaultAction     string
	DefaultSizeFactor float64
}

func resolveSieveDecision(riskMgmt map[string]any, indicatorTag, structureTag, mechanicsTag, liqConfidence, direction string, crowdingAlign bool) sieveDecision {
	result := sieveDecision{
		Action:            "ALLOW",
		SizeFactor:        1.0,
		Reason:            "SIEVE_DEFAULT",
		Hit:               false,
		MinSizeFactor:     0.0,
		DefaultAction:     "ALLOW",
		DefaultSizeFactor: 1.0,
	}
	sieve := toMap(riskMgmt["sieve"])
	if len(sieve) == 0 {
		return result
	}
	if v := toFloat(sieve["min_size_factor"]); v >= 0 {
		result.MinSizeFactor = v
	}
	if v := strings.ToUpper(strings.TrimSpace(toString(sieve["default_gate_action"]))); v != "" {
		result.DefaultAction = v
	}
	if v := toFloat(sieve["default_size_factor"]); v >= 0 {
		result.DefaultSizeFactor = v
	}
	rows, ok := sieve["rows"].([]any)
	if !ok || len(rows) == 0 {
		result.Action = result.DefaultAction
		result.SizeFactor = result.DefaultSizeFactor
		return result
	}
	inputs := normalizeSieveInputs(indicatorTag, structureTag, mechanicsTag, liqConfidence, direction, crowdingAlign)
	bestRow := selectBestSieveRow(rows, inputs)
	if bestRow != nil {
		result.Action = strings.ToUpper(strings.TrimSpace(toString(bestRow["gate_action"])))
		result.SizeFactor = toFloat(bestRow["size_factor"])
		result.Reason = strings.TrimSpace(toString(bestRow["reason_code"]))
		result.Hit = true
	}
	if !result.Hit {
		result.Action = result.DefaultAction
		result.SizeFactor = result.DefaultSizeFactor
	}
	if result.Action == "" {
		result.Action = result.DefaultAction
	}
	if result.Action == "ALLOW" {
		if result.SizeFactor <= 0 {
			result.SizeFactor = result.DefaultSizeFactor
		}
		if result.MinSizeFactor > 0 && result.SizeFactor < result.MinSizeFactor {
			result.SizeFactor = result.MinSizeFactor
		}
	} else {
		result.SizeFactor = 0
	}
	if result.Reason == "" {
		if result.Hit {
			result.Reason = "SIEVE_MATCH"
		} else {
			result.Reason = "SIEVE_DEFAULT"
		}
	}
	return result
}

type sieveMatchInputs struct {
	IndicatorTag  string
	StructureTag  string
	MechanicsTag  string
	LiqConfidence string
	Direction     string
	CrowdingAlign bool
}

func normalizeSieveInputs(indicatorTag, structureTag, mechanicsTag, liqConfidence, direction string, crowdingAlign bool) sieveMatchInputs {
	return sieveMatchInputs{
		IndicatorTag:  strings.ToLower(strings.TrimSpace(indicatorTag)),
		StructureTag:  strings.ToLower(strings.TrimSpace(structureTag)),
		MechanicsTag:  strings.ToLower(strings.TrimSpace(mechanicsTag)),
		LiqConfidence: strings.ToLower(strings.TrimSpace(liqConfidence)),
		Direction:     strings.ToLower(strings.TrimSpace(direction)),
		CrowdingAlign: crowdingAlign,
	}
}

func selectBestSieveRow(rows []any, inputs sieveMatchInputs) map[string]any {
	bestPriority := -1 << 30
	bestMatchCount := -1
	var bestRow map[string]any
	for _, rowRaw := range rows {
		row := toMap(rowRaw)
		rowPriority := toInt(row["priority"])
		matchCount, ok := matchSieveRow(row, inputs)
		if !ok {
			continue
		}
		if rowPriority < bestPriority {
			continue
		}
		if rowPriority == bestPriority && matchCount <= bestMatchCount {
			continue
		}
		bestPriority = rowPriority
		bestMatchCount = matchCount
		bestRow = row
	}
	return bestRow
}

func matchSieveRow(row map[string]any, inputs sieveMatchInputs) (int, bool) {
	matchCount := 0
	rowInd := strings.ToLower(strings.TrimSpace(toString(row["indicator_tag"])))
	rowStruct := strings.ToLower(strings.TrimSpace(toString(row["structure_tag"])))
	rowTag := strings.ToLower(strings.TrimSpace(toString(row["mechanics_tag"])))
	rowConf := strings.ToLower(strings.TrimSpace(toString(row["liq_confidence"])))
	rowDir := strings.ToLower(strings.TrimSpace(toString(row["direction"])))
	rowAlign, hasAlign := row["crowding_align"]
	if rowInd != "" {
		if rowInd != inputs.IndicatorTag {
			return 0, false
		}
		matchCount++
	}
	if rowStruct != "" {
		if rowStruct != inputs.StructureTag {
			return 0, false
		}
		matchCount++
	}
	if rowTag != "" && rowTag != inputs.MechanicsTag {
		return 0, false
	}
	if rowTag != "" {
		matchCount++
	}
	if rowConf != "" && rowConf != inputs.LiqConfidence {
		return 0, false
	}
	if rowConf != "" {
		matchCount++
	}
	if rowDir != "" {
		if rowDir != inputs.Direction {
			return 0, false
		}
		matchCount++
	}
	if hasAlign {
		if toBool(rowAlign) != inputs.CrowdingAlign {
			return 0, false
		}
		matchCount++
	}
	return matchCount, true
}
