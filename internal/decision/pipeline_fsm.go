package decision

import (
	"context"
	"fmt"

	"brale-core/internal/decision/fsm"
	"brale-core/internal/decision/fund"
	"brale-core/internal/execution"

	"go.uber.org/zap"
)

func (p *Pipeline) evaluateFSM(ctx context.Context, res SymbolResult, gateDecision fund.GateDecision, plan *execution.ExecutionPlan, state fsm.PositionState, posID string, logger *zap.Logger) (fsm.PositionState, []fsm.Action, fsm.RuleHit, error) {
	if p == nil || p.Runner == nil {
		return state, []fsm.Action{{Type: fsm.ActionNoop, Reason: "FSM_RUNNER_MISSING"}}, fsm.RuleHit{}, fmt.Errorf("runner is required")
	}
	if res.RuleflowResult == nil {
		return state, []fsm.Action{{Type: fsm.ActionNoop, Reason: "RULEFLOW_MISSING"}}, fsm.RuleHit{}, fmt.Errorf("ruleflow result missing")
	}
	return res.FSMNext, res.FSMActions, res.FSMRuleHit, nil
}

func hasFSMAction(actions []fsm.Action, want fsm.ActionType) bool {
	for _, action := range actions {
		if action.Type == want {
			return true
		}
	}
	return false
}
