package ruleflow

import (
	"strings"

	"github.com/rulego/rulego/api/types"
)

type GateEntryNode struct{}

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
	consensus := toMap(root["consensus"])
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
		State:               state,
		StructureDirection:  structureDirection,
		IndicatorTag:        indicatorTag,
		StructureTag:        structureTag,
		MechanicsTag:        mechanicsTag,
		MomentumExpansion:   momentumExpansion,
		Alignment:           alignment,
		MeanRevNoise:        meanRevNoise,
		StructureClear:      structureClear,
		StructureIntegrity:  structureIntegrity,
		LiquidationStress:   liquidationStress,
		LiqConfidence:       liqConfidence,
		ConsensusScore:      toFloat(consensus["score"]),
		ConsensusConfidence: toFloat(consensus["confidence"]),
		ConsensusAgreement:  toFloat(consensus["agreement"]),
		ConsensusResonance:  toFloat(consensus["resonance_bonus"]),
		ConsensusResonant:   toBool(consensus["resonance_active"]),
		ScoreThreshold:      toFloat(consensus["score_threshold"]),
		ConfidenceThreshold: toFloat(consensus["confidence_threshold"]),
	}
	missingProviders := resolveMissingProviders(providersEnabled, indicator, structure, mechanics)
	decision := evaluateGateDecision(inputs, missingProviders)

	gateActionBeforeSieve := decision.Action
	var sieveDecision sieveDecision
	if decision.Action == "ALLOW" {
		sieveDecision = resolveSieveDecision(riskMgmt, mechanicsTag, liqConfidence, crowdingAlign)
		if sieveDecision.Hit {
			decision.Reason = "SIEVE_POLICY"
			decision.Priority = gatePrioritySieveOverride
		}
		if sieveDecision.Action != "" && sieveDecision.Action != "ALLOW" {
			decision.Action = sieveDecision.Action
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
	derived := cloneMap(root["derived"])
	if len(derived) == 0 {
		derived = map[string]any{}
	}
	derived["indicator_tag"] = indicatorTag
	derived["structure_tag"] = structureTag
	derived["mechanics_tag"] = mechanicsTag
	derived["crowding_align"] = crowdingAlign
	derived["consensus_score"] = inputs.ConsensusScore
	derived["consensus_confidence"] = inputs.ConsensusConfidence
	derived["consensus_agreement"] = inputs.ConsensusAgreement
	derived["consensus_resonance_bonus"] = inputs.ConsensusResonance
	derived["consensus_resonant"] = inputs.ConsensusResonant
	derived["consensus_score_threshold"] = inputs.ScoreThreshold
	derived["consensus_confidence_threshold"] = inputs.ConfidenceThreshold
	derived["gate_trace"] = decision.GateTrace
	derived["gate_stop_step"] = decision.StopStep
	derived["gate_stop_reason"] = decision.StopReason
	derived["gate_action_before_sieve"] = gateActionBeforeSieve
	derived["sieve_action"] = sieveDecision.Action
	derived["sieve_size_factor"] = sieveDecision.SizeFactor
	derived["sieve_reason"] = sieveDecision.Reason
	derived["sieve_hit"] = sieveDecision.Hit
	derived["sieve_min_size_factor"] = sieveDecision.MinSizeFactor
	derived["sieve_default_action"] = sieveDecision.DefaultAction
	derived["sieve_default_size_factor"] = sieveDecision.DefaultSizeFactor
	derived["sieve_policy_hash"] = toString(binding["strategy_hash"])

	root["gate"] = map[string]any{
		"action":    decision.Action,
		"reason":    decision.Reason,
		"grade":     decision.Grade,
		"direction": decision.Direction,
		"tradeable": decision.Action == "ALLOW",
		"derived":   derived,
		"rule_hit":  ruleHit,
	}
	respondRuleMsgJSON(ctx, msg, root)
}

func resolveMissingProviders(providersEnabled, indicator, structure, mechanics map[string]any) bool {
	if len(providersEnabled) > 0 {
		return !toBool(providersEnabled["indicator"]) || !toBool(providersEnabled["structure"]) || !toBool(providersEnabled["mechanics"])
	}
	return len(indicator) == 0 || len(structure) == 0 || len(mechanics) == 0
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

type sieveDecision struct {
	Action            string
	SizeFactor        float64
	Reason            string
	Hit               bool
	MinSizeFactor     float64
	DefaultAction     string
	DefaultSizeFactor float64
}

func resolveSieveDecision(riskMgmt map[string]any, mechanicsTag, liqConfidence string, crowdingAlign bool) sieveDecision {
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
	inputs := normalizeSieveInputs(mechanicsTag, liqConfidence, crowdingAlign)
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
	MechanicsTag  string
	LiqConfidence string
	CrowdingAlign bool
}

func normalizeSieveInputs(mechanicsTag, liqConfidence string, crowdingAlign bool) sieveMatchInputs {
	return sieveMatchInputs{
		MechanicsTag:  strings.ToLower(strings.TrimSpace(mechanicsTag)),
		LiqConfidence: strings.ToLower(strings.TrimSpace(liqConfidence)),
		CrowdingAlign: crowdingAlign,
	}
}

func selectBestSieveRow(rows []any, inputs sieveMatchInputs) map[string]any {
	bestMatchCount := -1
	var bestRow map[string]any
	for _, rowRaw := range rows {
		row := toMap(rowRaw)
		matchCount, ok := matchSieveRow(row, inputs)
		if !ok {
			continue
		}
		if matchCount <= bestMatchCount {
			continue
		}
		bestMatchCount = matchCount
		bestRow = row
	}
	return bestRow
}

func matchSieveRow(row map[string]any, inputs sieveMatchInputs) (int, bool) {
	matchCount := 0
	rowTag := strings.ToLower(strings.TrimSpace(toString(row["mechanics_tag"])))
	rowConf := strings.ToLower(strings.TrimSpace(toString(row["liq_confidence"])))
	rowAlign, hasAlign := row["crowding_align"]
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
	if hasAlign {
		if toBool(rowAlign) != inputs.CrowdingAlign {
			return 0, false
		}
		matchCount++
	}
	return matchCount, true
}
