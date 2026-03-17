package decision

import (
	"strings"

	"brale-core/internal/decision/features"
	"brale-core/internal/decision/fsm"
	"brale-core/internal/decision/ruleflow"
	"brale-core/internal/execution"
	"brale-core/internal/strategy"
)

func buildRuleflowInput(symbol string, res SymbolResult, bind strategy.StrategyBinding, state fsm.PositionState, positionID string, exitConfirmCount int, buildPlan bool, comp features.CompressionResult, acct execution.AccountState, risk execution.RiskParams, inPos ruleflow.InPositionOutputs, position ruleflow.HardGuardPosition) ruleflow.Input {
	structureDirection := strings.ToLower(strings.TrimSpace(res.ConsensusDirection))
	return ruleflow.Input{
		Symbol:             symbol,
		Providers:          res.Providers,
		AgentStructure:     res.AgentStructure,
		InPosition:         inPos,
		Position:           position,
		State:              state,
		PositionID:         positionID,
		ExitConfirmCount:   exitConfirmCount,
		BuildPlan:          buildPlan,
		Compression:        comp,
		Account:            acct,
		Risk:               risk,
		Binding:            bind,
		StructureDirection: structureDirection,
	}
}
