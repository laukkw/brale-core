package runtimeapi

const (
	dashboardOverviewPath        = "/api/runtime/dashboard/overview"
	dashboardAccountSummaryPath  = "/api/runtime/dashboard/account_summary"
	dashboardKlinePath           = "/api/runtime/dashboard/kline"
	dashboardDecisionFlowPath    = "/api/runtime/dashboard/decision_flow"
	dashboardDecisionHistoryPath = "/api/runtime/dashboard/decision_history"

	dashboardContractSummary = "contract_frozen"

	dashboardKlineDefaultLimit   = 120
	dashboardKlineMaxLimit       = 1000
	dashboardHistoryDefaultLimit = 20
	dashboardHistoryMaxLimit     = 200
)

type DashboardOverviewResponse struct {
	Status     string                    `json:"status"`
	Symbol     string                    `json:"symbol"`
	Symbols    []DashboardOverviewSymbol `json:"symbols"`
	AccountPnL *DashboardPnLCard         `json:"account_pnl,omitempty"`
	Summary    string                    `json:"summary"`
	RequestID  string                    `json:"request_id"`
}

type DashboardAccountSummaryResponse struct {
	Status    string                 `json:"status"`
	Balance   DashboardBalanceCard   `json:"balance"`
	Profit    DashboardProfitAllCard `json:"profit"`
	Summary   string                 `json:"summary"`
	RequestID string                 `json:"request_id"`
}

type DashboardBalanceCard struct {
	Currency       string   `json:"currency"`
	Total          float64  `json:"total"`
	Available      float64  `json:"available"`
	Used           float64  `json:"used"`
	Monitored      []string `json:"monitored_symbols"`
	MonitoredCount int      `json:"monitored_count"`
}

type DashboardProfitAllCard struct {
	ClosedProfit     float64 `json:"closed_profit"`
	AllProfit        float64 `json:"all_profit"`
	ClosedTradeCount int     `json:"closed_trade_count"`
	TradeCount       int     `json:"trade_count"`
}

type DashboardOverviewSymbol struct {
	Symbol         string                  `json:"symbol"`
	Position       DashboardPositionCard   `json:"position"`
	PnL            DashboardPnLCard        `json:"pnl"`
	Reconciliation DashboardReconciliation `json:"reconciliation"`
}

type DashboardPositionCard struct {
	Side             string                          `json:"side"`
	Amount           float64                         `json:"amount"`
	Leverage         float64                         `json:"leverage"`
	EntryPrice       float64                         `json:"entry_price"`
	CurrentPrice     float64                         `json:"current_price"`
	TakeProfits      []float64                       `json:"take_profits"`
	StopLoss         float64                         `json:"stop_loss"`
	RiskPlanTimeline []DashboardRiskPlanTimelineItem `json:"risk_plan_timeline,omitempty"`
}

type DashboardRiskPlanTimelineItem struct {
	Source              string    `json:"source"`
	Label               string    `json:"label"`
	CreatedAt           string    `json:"created_at"`
	StopLoss            float64   `json:"stop_loss"`
	TakeProfits         []float64 `json:"take_profits,omitempty"`
	PreviousStopLoss    float64   `json:"previous_stop_loss"`
	PreviousTakeProfits []float64 `json:"previous_take_profits,omitempty"`
}

type DashboardPnLCard struct {
	Realized   float64 `json:"realized"`
	Unrealized float64 `json:"unrealized"`
	Total      float64 `json:"total"`
}

type DashboardReconciliation struct {
	Status         string  `json:"status"`
	DriftAbs       float64 `json:"drift_abs"`
	DriftPct       float64 `json:"drift_pct"`
	DriftThreshold float64 `json:"drift_threshold"`
}

type DashboardKlineResponse struct {
	Status    string            `json:"status"`
	Symbol    string            `json:"symbol"`
	Interval  string            `json:"interval"`
	Limit     int               `json:"limit"`
	Candles   []DashboardCandle `json:"candles"`
	Summary   string            `json:"summary"`
	RequestID string            `json:"request_id"`
}

type DashboardCandle struct {
	OpenTime  int64   `json:"open_time"`
	CloseTime int64   `json:"close_time"`
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Close     float64 `json:"close"`
	Volume    float64 `json:"volume"`
}

type DashboardDecisionFlowResponse struct {
	Status    string                `json:"status"`
	Symbol    string                `json:"symbol"`
	Flow      DashboardDecisionFlow `json:"flow"`
	Summary   string                `json:"summary"`
	RequestID string                `json:"request_id"`
}

type DashboardDecisionFlow struct {
	Anchor    DashboardFlowAnchor   `json:"anchor"`
	Nodes     []DashboardFlowNode   `json:"nodes"`
	Intervals []string              `json:"intervals,omitempty"`
	Trace     DashboardFlowTrace    `json:"trace"`
	Tighten   *DashboardTightenInfo `json:"tighten,omitempty"`
}

type DashboardFlowTrace struct {
	Agents     []DashboardFlowStageValues `json:"agents,omitempty"`
	Providers  []DashboardFlowStageValues `json:"providers,omitempty"`
	InPosition *DashboardFlowInPosition   `json:"in_position,omitempty"`
	Gate       *DashboardFlowGateTrace    `json:"gate,omitempty"`
}

type DashboardFlowStageValues struct {
	Stage   string                    `json:"stage"`
	Mode    string                    `json:"mode,omitempty"`
	Source  string                    `json:"source"`
	Model   string                    `json:"model,omitempty"`
	Status  string                    `json:"status,omitempty"`
	Reason  string                    `json:"reason,omitempty"`
	Summary string                    `json:"summary,omitempty"`
	Values  []DashboardFlowValueField `json:"values,omitempty"`
}

type DashboardFlowValueField struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	State string `json:"state,omitempty"`
}

type DashboardFlowInPosition struct {
	Active bool   `json:"active"`
	Side   string `json:"side,omitempty"`
	Status string `json:"status,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type DashboardFlowGateTrace struct {
	Action    string                    `json:"action"`
	Tradeable bool                      `json:"tradeable"`
	Status    string                    `json:"status,omitempty"`
	Reason    string                    `json:"reason,omitempty"`
	Summary   string                    `json:"summary,omitempty"`
	Rules     []DashboardFlowValueField `json:"rules,omitempty"`
}

type DashboardFlowAnchor struct {
	Type       string `json:"type"`
	SnapshotID uint   `json:"snapshot_id"`
	Confidence string `json:"confidence"`
	Reason     string `json:"reason"`
}

type DashboardFlowNode struct {
	Stage   string                    `json:"stage"`
	Title   string                    `json:"title"`
	Outcome string                    `json:"outcome"`
	Status  string                    `json:"status,omitempty"`
	Reason  string                    `json:"reason,omitempty"`
	Values  []DashboardFlowValueField `json:"values,omitempty"`
}

type DashboardTightenInfo struct {
	Triggered bool   `json:"triggered"`
	Reason    string `json:"reason"`
}

type DashboardDecisionHistoryResponse struct {
	Status    string                         `json:"status"`
	Symbol    string                         `json:"symbol"`
	Limit     int                            `json:"limit"`
	Items     []DashboardDecisionHistoryItem `json:"items"`
	NextCursor string                        `json:"next_cursor,omitempty"`
	Detail    *DashboardDecisionDetail       `json:"detail,omitempty"`
	Message   string                         `json:"message,omitempty"`
	Summary   string                         `json:"summary"`
	RequestID string                         `json:"request_id"`
}

type DashboardDecisionHistoryItem struct {
	SnapshotID          uint     `json:"snapshot_id"`
	Action              string   `json:"action"`
	Reason              string   `json:"reason"`
	At                  string   `json:"at"`
	ConsensusScore      *float64 `json:"consensus_score,omitempty"`
	ConsensusConfidence *float64 `json:"consensus_confidence,omitempty"`
}

type DashboardDecisionDetail struct {
	SnapshotID                   uint                            `json:"snapshot_id"`
	Action                       string                          `json:"action"`
	Reason                       string                          `json:"reason"`
	Tradeable                    bool                            `json:"tradeable"`
	ConsensusScore               *float64                        `json:"consensus_score,omitempty"`
	ConsensusConfidence          *float64                        `json:"consensus_confidence,omitempty"`
	ConsensusScoreThreshold      *float64                        `json:"consensus_score_threshold,omitempty"`
	ConsensusConfidenceThreshold *float64                        `json:"consensus_confidence_threshold,omitempty"`
	ConsensusScorePassed         *bool                           `json:"consensus_score_passed,omitempty"`
	ConsensusConfidencePassed    *bool                           `json:"consensus_confidence_passed,omitempty"`
	ConsensusPassed              *bool                           `json:"consensus_passed,omitempty"`
	Providers                    []string                        `json:"providers"`
	Agents                       []string                        `json:"agents"`
	Tighten                      *DashboardDecisionTightenDetail `json:"tighten,omitempty"`
	PlanContext                  *DashboardDecisionPlanContext   `json:"plan_context,omitempty"`
	Plan                         *DashboardDecisionPlanSummary   `json:"plan,omitempty"`
	Sieve                        *DashboardDecisionSieveDetail   `json:"sieve,omitempty"`
	ReportMarkdown               string                          `json:"report_markdown"`
	DecisionViewURL              string                          `json:"decision_view_url"`
}

type DashboardDecisionTightenDetail struct {
	Action         string   `json:"action"`
	Evaluated      bool     `json:"evaluated"`
	Eligible       bool     `json:"eligible"`
	Executed       bool     `json:"executed"`
	TPTightened    bool     `json:"tp_tightened"`
	BlockedBy      []string `json:"blocked_by,omitempty"`
	Score          float64  `json:"score"`
	ScoreThreshold float64  `json:"score_threshold"`
	ScoreParseOK   bool     `json:"score_parse_ok"`
	DisplayReason  string   `json:"display_reason,omitempty"`
}

type DashboardDecisionPlanContext struct {
	RiskPerTradePct float64 `json:"risk_per_trade_pct"`
	MaxInvestPct    float64 `json:"max_invest_pct"`
	MaxLeverage     float64 `json:"max_leverage"`
	EntryOffsetATR  float64 `json:"entry_offset_atr"`
	EntryMode       string  `json:"entry_mode"`
	PlanSource      string  `json:"plan_source,omitempty"`
	InitialExit     string  `json:"initial_exit"`
}

type DashboardDecisionPlanSummary struct {
	Status           string                         `json:"status"`
	Direction        string                         `json:"direction"`
	EntryPrice       float64                        `json:"entry_price"`
	StopLoss         float64                        `json:"stop_loss"`
	TakeProfits      []float64                      `json:"take_profits,omitempty"`
	TakeProfitLevels []DashboardDecisionPlanTPLevel `json:"take_profit_levels,omitempty"`
	PositionSize     float64                        `json:"position_size"`
	RiskPct          float64                        `json:"risk_pct"`
	Leverage         float64                        `json:"leverage"`
	InitialQty       float64                        `json:"initial_qty"`
	OpenedAt         string                         `json:"opened_at,omitempty"`
}

type DashboardDecisionPlanTPLevel struct {
	LevelID string  `json:"level_id"`
	Price   float64 `json:"price"`
	QtyPct  float64 `json:"qty_pct"`
	Hit     bool    `json:"hit"`
}

type DashboardDecisionSieveDetail struct {
	Action            string                      `json:"action"`
	ReasonCode        string                      `json:"reason_code"`
	Hit               bool                        `json:"hit"`
	SizeFactor        float64                     `json:"size_factor"`
	MinSizeFactor     float64                     `json:"min_size_factor"`
	DefaultAction     string                      `json:"default_action"`
	DefaultSizeFactor float64                     `json:"default_size_factor"`
	ActionBefore      string                      `json:"action_before"`
	PolicyHash        string                      `json:"policy_hash"`
	Rows              []DashboardDecisionSieveRow `json:"rows,omitempty"`
}

type DashboardDecisionSieveRow struct {
	MechanicsTag  string  `json:"mechanics_tag"`
	LiqConfidence string  `json:"liq_confidence"`
	CrowdingAlign *bool   `json:"crowding_align,omitempty"`
	GateAction    string  `json:"gate_action"`
	SizeFactor    float64 `json:"size_factor"`
	ReasonCode    string  `json:"reason_code"`
	Matched       bool    `json:"matched"`
}
