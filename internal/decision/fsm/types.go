package fsm

type PositionState string

const (
	StateFlat       PositionState = "FLAT"
	StateInPosition PositionState = "IN_POSITION"
)

type ActionType string

const (
	ActionOpen   ActionType = "open_position"
	ActionClose  ActionType = "close_position"
	ActionReduce ActionType = "reduce_position"
	ActionNoop   ActionType = "noop"
)

type ProviderState struct {
	IndicatorTradeable bool
	StructureTradeable bool
	MechanicsTradeable bool
	StructureIntegrity bool
}

type Event struct {
	Symbol     string
	PositionID string
	State      PositionState
	Providers  ProviderState
	ProfitR    float64
}

type RuleHit struct {
	Name     string
	Priority int
	Action   string
	Reason   string
	Next     string
	Default  bool
}

type Action struct {
	Type   ActionType
	Reason string
}
