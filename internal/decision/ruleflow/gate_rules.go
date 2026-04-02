package ruleflow

type gateScriptCondition struct {
	MomentumExpansion *bool
	Alignment         *bool
	MeanRevNoise      *bool
}

type gateScriptRule struct {
	IndicatorTag string
	StructureTag string
	Script       string
	AllowOutcome gateDecisionOutcome
	Allow        gateScriptCondition
}

var gateScriptRules = []gateScriptRule{
	{
		IndicatorTag: "trend_surge",
		StructureTag: "structure_broken",
		Script:       "G",
		AllowOutcome: gateDecisionOutcome{
			Action:   "ALLOW",
			Reason:   "PASS_BREAK_CONTINUATION",
			Priority: gatePriorityAllow,
			StopStep: "gate_allow",
			Grade:    gateGradeMedium,
		},
		Allow: gateScriptCondition{
			MomentumExpansion: gateBoolPtr(true),
			Alignment:         gateBoolPtr(true),
			MeanRevNoise:      gateBoolPtr(false),
		},
	},
	{
		IndicatorTag: "trend_surge",
		StructureTag: "breakout_confirmed",
		Script:       "A",
		AllowOutcome: gateDecisionOutcome{
			Action:   "ALLOW",
			Reason:   "PASS_STRONG",
			Priority: gatePriorityAllow,
			StopStep: "gate_allow",
			Grade:    gateGradeHigh,
		},
		Allow: gateScriptCondition{
			MomentumExpansion: gateBoolPtr(true),
			Alignment:         gateBoolPtr(true),
			MeanRevNoise:      gateBoolPtr(false),
		},
	},
	{
		IndicatorTag: "pullback_entry",
		StructureTag: "support_retest",
		Script:       "B",
		AllowOutcome: gateDecisionOutcome{
			Action:   "ALLOW",
			Reason:   "PASS_STRONG",
			Priority: gatePriorityAllow,
			StopStep: "gate_allow",
			Grade:    gateGradeMedium,
		},
		Allow: gateScriptCondition{
			MomentumExpansion: gateBoolPtr(false),
			Alignment:         gateBoolPtr(true),
			MeanRevNoise:      gateBoolPtr(false),
		},
	},
	{
		IndicatorTag: "divergence_reversal",
		StructureTag: "support_retest",
		Script:       "C",
		AllowOutcome: gateDecisionOutcome{
			Action:   "ALLOW",
			Reason:   "PASS_STRONG",
			Priority: gatePriorityAllow,
			StopStep: "gate_allow",
			Grade:    gateGradeLow,
		},
		Allow: gateScriptCondition{
			Alignment:    gateBoolPtr(false),
			MeanRevNoise: gateBoolPtr(false),
		},
	},
	{
		IndicatorTag: "trend_surge",
		StructureTag: "support_retest",
		Script:       "D",
		AllowOutcome: gateDecisionOutcome{
			Action:   "ALLOW",
			Reason:   "PASS_STRONG",
			Priority: gatePriorityAllow,
			StopStep: "gate_allow",
			Grade:    gateGradeMedium,
		},
		Allow: gateScriptCondition{
			MomentumExpansion: gateBoolPtr(true),
			Alignment:         gateBoolPtr(true),
			MeanRevNoise:      gateBoolPtr(false),
		},
	},
	{
		IndicatorTag: "pullback_entry",
		StructureTag: "breakout_confirmed",
		Script:       "E",
		AllowOutcome: gateDecisionOutcome{
			Action:   "ALLOW",
			Reason:   "PASS_STRONG",
			Priority: gatePriorityAllow,
			StopStep: "gate_allow",
			Grade:    gateGradeLow,
		},
		Allow: gateScriptCondition{
			MomentumExpansion: gateBoolPtr(false),
			Alignment:         gateBoolPtr(true),
			MeanRevNoise:      gateBoolPtr(false),
		},
	},
	{
		IndicatorTag: "divergence_reversal",
		StructureTag: "breakout_confirmed",
		Script:       "F",
		AllowOutcome: gateDecisionOutcome{
			Action:   "ALLOW",
			Reason:   "PASS_STRONG",
			Priority: gatePriorityAllow,
			StopStep: "gate_allow",
			Grade:    gateGradeLow,
		},
		Allow: gateScriptCondition{
			Alignment:    gateBoolPtr(false),
			MeanRevNoise: gateBoolPtr(false),
		},
	},
}

func resolveEntryScript(indicatorTag, structureTag string) string {
	if rule, ok := findGateScriptRuleByTags(indicatorTag, structureTag); ok {
		return rule.Script
	}
	return ""
}

func isEntryScriptAllowed(script string, momentumExpansion, alignment, meanRevNoise bool) bool {
	rule, ok := findGateScriptRuleByScript(script)
	if !ok {
		return false
	}
	return rule.Allow.matches(momentumExpansion, alignment, meanRevNoise)
}

func resolveEntryGrade(script string) int {
	rule, ok := findGateScriptRuleByScript(script)
	if !ok {
		return gateGradeNone
	}
	return rule.AllowOutcome.Grade
}

func resolveEntryAllowOutcome(script string) (gateDecisionOutcome, bool) {
	rule, ok := findGateScriptRuleByScript(script)
	if !ok {
		return gateDecisionOutcome{}, false
	}
	return rule.AllowOutcome, true
}

func findGateScriptRuleByTags(indicatorTag, structureTag string) (gateScriptRule, bool) {
	for _, rule := range gateScriptRules {
		if rule.IndicatorTag == indicatorTag && rule.StructureTag == structureTag {
			return rule, true
		}
	}
	return gateScriptRule{}, false
}

func findGateScriptRuleByScript(script string) (gateScriptRule, bool) {
	for _, rule := range gateScriptRules {
		if rule.Script == script {
			return rule, true
		}
	}
	return gateScriptRule{}, false
}

func (c gateScriptCondition) matches(momentumExpansion, alignment, meanRevNoise bool) bool {
	if c.MomentumExpansion != nil && *c.MomentumExpansion != momentumExpansion {
		return false
	}
	if c.Alignment != nil && *c.Alignment != alignment {
		return false
	}
	if c.MeanRevNoise != nil && *c.MeanRevNoise != meanRevNoise {
		return false
	}
	return true
}

func gateBoolPtr(value bool) *bool {
	v := value
	return &v
}
