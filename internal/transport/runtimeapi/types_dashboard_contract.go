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
	Side         string    `json:"side"`
	Amount       float64   `json:"amount"`
	EntryPrice   float64   `json:"entry_price"`
	CurrentPrice float64   `json:"current_price"`
	TakeProfits  []float64 `json:"take_profits"`
	StopLoss     float64   `json:"stop_loss"`
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
	Stage  string                    `json:"stage"`
	Mode   string                    `json:"mode,omitempty"`
	Source string                    `json:"source"`
	Values []DashboardFlowValueField `json:"values,omitempty"`
}

type DashboardFlowValueField struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	State string `json:"state,omitempty"`
}

type DashboardFlowInPosition struct {
	Active bool   `json:"active"`
	Side   string `json:"side,omitempty"`
}

type DashboardFlowGateTrace struct {
	Action    string                    `json:"action"`
	Tradeable bool                      `json:"tradeable"`
	Reason    string                    `json:"reason,omitempty"`
	Rules     []DashboardFlowValueField `json:"rules,omitempty"`
}

type DashboardFlowAnchor struct {
	Type       string `json:"type"`
	SnapshotID uint   `json:"snapshot_id"`
	Confidence string `json:"confidence"`
	Reason     string `json:"reason"`
}

type DashboardFlowNode struct {
	Stage   string `json:"stage"`
	Title   string `json:"title"`
	Outcome string `json:"outcome"`
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
	Detail    *DashboardDecisionDetail       `json:"detail,omitempty"`
	Message   string                         `json:"message,omitempty"`
	Summary   string                         `json:"summary"`
	RequestID string                         `json:"request_id"`
}

type DashboardDecisionHistoryItem struct {
	SnapshotID uint   `json:"snapshot_id"`
	Action     string `json:"action"`
	Reason     string `json:"reason"`
	At         string `json:"at"`
}

type DashboardDecisionDetail struct {
	SnapshotID      uint     `json:"snapshot_id"`
	Action          string   `json:"action"`
	Reason          string   `json:"reason"`
	Tradeable       bool     `json:"tradeable"`
	Providers       []string `json:"providers"`
	Agents          []string `json:"agents"`
	ReportMarkdown  string   `json:"report_markdown"`
	DecisionViewURL string   `json:"decision_view_url"`
}
