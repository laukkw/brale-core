package ruleflow

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

func (e *gateDecisionEvaluator) evalDirection() {
	if e.hasAction() {
		return
	}
	if rule, ok := findFirstGateDecisionRule(gateDirectionRules, e.context()); ok {
		e.decision.Direction = "none"
		e.applyOutcome(rule.Outcome)
		return
	}
	e.appendGateTrace("direction", true, "")
}

func (e *gateDecisionEvaluator) evalData() {
	if e.hasAction() {
		return
	}
	if rule, ok := findFirstGateDecisionRule(gateDataRules, e.context()); ok {
		e.applyOutcome(rule.Outcome)
		return
	}
	e.appendGateTrace("data", true, "")
}

func (e *gateDecisionEvaluator) evalStructure() {
	if e.hasAction() {
		return
	}
	if rule, ok := findFirstGateDecisionRule(gateStructureStopRules, e.context()); ok {
		e.applyOutcome(rule.Outcome)
		return
	}
	e.appendGateTrace("structure", true, "")
}

func (e *gateDecisionEvaluator) evalMechRisk() {
	if e.hasAction() {
		return
	}
	if rule, ok := findFirstGateDecisionRule(gateMechanicsStopRules, e.context()); ok {
		e.applyOutcome(rule.Outcome)
		return
	}
	e.appendGateTrace("mech_risk", true, "")
}

func (e *gateDecisionEvaluator) evalIndicatorNoise() {
	if e.hasAction() {
		return
	}
	if rule, ok := findFirstGateDecisionRule(gateNoiseStopRules[:1], e.context()); ok {
		e.applyOutcome(rule.Outcome)
		return
	}
	e.appendGateTrace("indicator_noise", true, "")
}

func (e *gateDecisionEvaluator) evalStructureClear() {
	if e.hasAction() {
		return
	}
	if rule, ok := findFirstGateDecisionRule(gateNoiseStopRules[1:2], e.context()); ok {
		e.applyOutcome(rule.Outcome)
		return
	}
	e.appendGateTrace("structure_clear", true, "")
}

func (e *gateDecisionEvaluator) evalTagConsistency() {
	if e.hasAction() {
		return
	}
	if rule, ok := findFirstGateDecisionRule(gateNoiseStopRules[2:], e.context()); ok {
		e.applyOutcome(rule.Outcome)
		return
	}
	e.appendGateTrace("tag_consistency", true, "")
}

func (e *gateDecisionEvaluator) evalScript() {
	if e.hasAction() {
		return
	}
	ctx := e.context()
	ctx.Script = resolveEntryScript(e.inputs.IndicatorTag, e.inputs.StructureTag)
	if rule, ok := findFirstGateDecisionRule(gateScriptStopRules[:1], ctx); ok {
		e.applyOutcome(rule.Outcome)
		return
	}
	e.appendGateTrace("script_select", true, "")
	if rule, ok := findFirstGateDecisionRule(gateScriptStopRules[1:], ctx); ok {
		e.applyOutcome(rule.Outcome)
		return
	}
	e.appendGateTrace("script_allowed", true, "")
	outcome, ok := resolveEntryAllowOutcome(ctx.Script)
	if !ok {
		return
	}
	e.decision.Action = outcome.Action
	e.decision.Reason = outcome.Reason
	e.decision.Grade = outcome.Grade
	e.decision.Priority = outcome.Priority
	e.decision.StopStep = outcome.StopStep
	e.decision.StopReason = outcome.Reason
}
