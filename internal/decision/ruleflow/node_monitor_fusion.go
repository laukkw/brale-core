package ruleflow

import (
	"encoding/json"
	"strings"

	"github.com/rulego/rulego/api/types"
)

type MonitorFusionNode struct{}

func (n *MonitorFusionNode) Type() string {
	return "brale/monitor_fusion"
}

func (n *MonitorFusionNode) New() types.Node {
	return &MonitorFusionNode{}
}

func (n *MonitorFusionNode) Init(types.Config, types.Configuration) error {
	return nil
}

func (n *MonitorFusionNode) Destroy() {}

func (n *MonitorFusionNode) OnMsg(ctx types.RuleContext, msg types.RuleMsg) {
	root, err := readRoot(msg)
	if err != nil {
		ctx.TellFailure(msg, err)
		return
	}
	hardGuard := toMap(root["hard_guard"])
	if strings.EqualFold(toString(hardGuard["action"]), monitorActionExit) {
		root["gate"] = hardGuard
		payload, err := json.Marshal(root)
		if err != nil {
			ctx.TellFailure(msg, err)
			return
		}
		msg.DataType = types.JSON
		msg.SetData(string(payload))
		ctx.TellSuccess(msg)
		return
	}
	inPos := toMap(root["in_position"])
	ind := toMap(inPos["indicator"])
	st := toMap(inPos["structure"])
	mech := toMap(inPos["mechanics"])

	action := monitorActionKeep
	reason := "KEEP"
	priority := monitorPriorityKeep

	structureTag := strings.ToLower(toString(st["monitor_tag"]))
	structureThreat := strings.ToLower(toString(st["threat_level"]))
	indicatorTag := strings.ToLower(toString(ind["monitor_tag"]))
	mechanicsTag := strings.ToLower(toString(mech["monitor_tag"]))
	indicatorReason := strings.TrimSpace(toString(ind["reason"]))
	structureReason := strings.TrimSpace(toString(st["reason"]))
	mechanicsReason := strings.TrimSpace(toString(mech["reason"]))

	reasonSource := ""
	criticalExit := structureTag == "exit" && structureThreat == "critical"

	exitHit := indicatorTag == "exit" || structureTag == "exit" || mechanicsTag == "exit"
	tightenHit := indicatorTag == "tighten" || structureTag == "tighten" || mechanicsTag == "tighten"

	if criticalExit {
		action = monitorActionTighten
		reason = "CRITICAL_TIGHTEN_ATTEMPT"
		priority = monitorPriorityExitConfirmPending
		reasonSource = "P6"
	} else if exitHit {
		action = monitorActionExit
		reason = "REVERSAL_CONFIRMED"
		priority = monitorPriorityExit
		reasonSource = "P1"
	} else if tightenHit {
		action = monitorActionTighten
		reason = "TIGHTEN"
		priority = monitorPriorityTighten
		reasonSource = "P2"
	}

	switch action {
	case monitorActionExit:
		confirmCount := toInt(root["exit_confirm_count"])
		confirmCount++
		root["exit_confirm_count"] = confirmCount
		if confirmCount < 2 {
			action = monitorActionTighten
			reason = "EXIT_CONFIRM_PENDING"
			priority = monitorPriorityExitConfirmPending
			reasonSource = "P5"
		}
	case monitorActionKeep:
		root["exit_confirm_count"] = 0
	}
	stopStep := ""
	if strings.TrimSpace(stopStep) == "" {
		stopStep = strings.ToLower(reason)
	}
	gateTrace := []map[string]any{
		{
			"step":   "indicator",
			"tag":    indicatorTag,
			"reason": indicatorReason,
		},
		{
			"step":   "structure",
			"tag":    structureTag,
			"reason": structureReason,
		},
		{
			"step":   "mechanics",
			"tag":    mechanicsTag,
			"reason": mechanicsReason,
		},
	}

	gate := map[string]any{
		"action":    action,
		"reason":    reason,
		"grade":     0,
		"direction": strings.ToLower(toString(toMap(root["structure"])["direction"])),
		"tradeable": false,
		"rule_hit": map[string]any{
			"name":     reason,
			"priority": priority,
			"action":   action,
			"reason":   reason,
			"default":  false,
			"source":   reasonSource,
		},
		"derived": map[string]any{
			"indicator_tag":    indicatorTag,
			"structure_tag":    structureTag,
			"mechanics_tag":    mechanicsTag,
			"gate_trace":       gateTrace,
			"gate_trace_mode":  "monitor",
			"gate_stop_step":   stopStep,
			"gate_stop_reason": reason,
		},
	}
	root["gate"] = gate
	payload, err := json.Marshal(root)
	if err != nil {
		ctx.TellFailure(msg, err)
		return
	}
	msg.DataType = types.JSON
	msg.SetData(string(payload))
	ctx.TellSuccess(msg)
}
