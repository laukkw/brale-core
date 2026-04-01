package decisionflow

import (
	"brale-core/internal/config"
	"brale-core/internal/readmodel/dashboard"
)

type SymbolConfig struct {
	AgentModels    map[string]string
	Intervals      []string
	StrategyHash   string
	RiskManagement RiskManagementConfig
}

type RiskManagementConfig struct {
	RiskPerTradePct float64
	MaxInvestPct    float64
	MaxLeverage     float64
	EntryOffsetATR  float64
	EntryMode       string
	InitialExit     string
	Sieve           config.RiskManagementSieveConfig
}

type FlowResult struct {
	Anchor    dashboard.FlowAnchor
	Nodes     []dashboard.FlowNode
	Intervals []string
	Trace     dashboard.FlowTrace
	Tighten   *dashboard.TightenInfo
}

type HistoryItem struct {
	SnapshotID          uint
	Action              string
	Reason              string
	At                  string
	ConsensusScore      *float64
	ConsensusConfidence *float64
}

type Detail struct {
	SnapshotID                   uint
	Action                       string
	Reason                       string
	Tradeable                    bool
	ConsensusScore               *float64
	ConsensusConfidence          *float64
	ConsensusScoreThreshold      *float64
	ConsensusConfidenceThreshold *float64
	ConsensusScorePassed         *bool
	ConsensusConfidencePassed    *bool
	ConsensusPassed              *bool
	Providers                    []string
	Agents                       []string
	Tighten                      *dashboard.DecisionTightenDetail
	PlanContext                  *PlanContext
	Plan                         *PlanSummary
	Sieve                        *SieveDetail
	ReportMarkdown               string
	DecisionViewURL              string
}

type PlanContext struct {
	RiskPerTradePct float64
	MaxInvestPct    float64
	MaxLeverage     float64
	EntryOffsetATR  float64
	EntryMode       string
	PlanSource      string
	InitialExit     string
}

type PlanSummary struct {
	Status           string
	Direction        string
	EntryPrice       float64
	StopLoss         float64
	TakeProfits      []float64
	TakeProfitLevels []PlanTPLevel
	PositionSize     float64
	InitialQty       float64
	RiskPct          float64
	Leverage         float64
	OpenedAt         string
}

type PlanTPLevel struct {
	LevelID string
	Price   float64
	QtyPct  float64
	Hit     bool
}

type SieveDetail struct {
	Action            string
	ReasonCode        string
	Hit               bool
	SizeFactor        float64
	MinSizeFactor     float64
	DefaultAction     string
	DefaultSizeFactor float64
	ActionBefore      string
	PolicyHash        string
	Rows              []SieveRow
}

type SieveRow struct {
	MechanicsTag  string
	LiqConfidence string
	CrowdingAlign *bool
	GateAction    string
	SizeFactor    float64
	ReasonCode    string
	Matched       bool
}
