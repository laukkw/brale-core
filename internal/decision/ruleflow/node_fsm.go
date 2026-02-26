package ruleflow

import (
	"encoding/json"
	"strings"

	"github.com/rulego/rulego/api/types"
)

const (
	monitorActionExit    = "EXIT"
	monitorActionTighten = "TIGHTEN"
	monitorActionKeep    = "KEEP"

	monitorPriorityKeep               = 1
	monitorPriorityExitConfirmPending = 2
	monitorPriorityTighten            = 3
	monitorPriorityExit               = 5
)

type FSMDecisionNode struct{}

func (n *FSMDecisionNode) Type() string {
	return "brale/fsm_decision"
}

func (n *FSMDecisionNode) New() types.Node {
	return &FSMDecisionNode{}
}

func (n *FSMDecisionNode) Init(types.Config, types.Configuration) error {
	return nil
}

func (n *FSMDecisionNode) Destroy() {}

func (n *FSMDecisionNode) OnMsg(ctx types.RuleContext, msg types.RuleMsg) {
	root, err := readRoot(msg)
	if err != nil {
		ctx.TellFailure(msg, err)
		return
	}
	state := strings.ToUpper(toString(root["state"]))
	gate := toMap(root["gate"])
	plan := toMap(root["plan"])
	planValid := toBool(plan["valid"])
	action := "noop"
	reason := "HOLD"
	next := state
	switch state {
	case "FLAT":
		if strings.EqualFold(toString(gate["action"]), "ALLOW") && planValid {
			action = "open_position"
			reason = "GATE_ALLOW"
			next = "IN_POSITION"
		}
	case "IN_POSITION":
		if strings.EqualFold(toString(gate["action"]), "EXIT") {
			action = "close_position"
			reason = "GATE_EXIT"
		} else if strings.EqualFold(toString(gate["action"]), "TIGHTEN") {
			action = "reduce_position"
			reason = "GATE_TIGHTEN"
		}
	}
	actions := []map[string]any{{"type": action, "reason": reason}}
	ruleHit := map[string]any{
		"name":     action,
		"priority": 1,
		"action":   action,
		"reason":   reason,
		"next":     next,
		"default":  false,
	}
	root["fsm"] = map[string]any{
		"next_state": next,
		"actions":    actions,
		"rule_hit":   ruleHit,
	}
	payload, err := json.Marshal(root)
	if err != nil {
		ctx.TellFailure(msg, err)
		return
	}
	msg.DataType = types.JSON
	msg.SetData(string(payload))
	ctx.TellSuccess(msg)
}
