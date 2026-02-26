package notifyport

type PositionOpenNotice struct {
	Symbol      string
	Direction   string
	Qty         float64
	EntryPrice  float64
	StopPrice   float64
	TakeProfits []float64
	RiskPct     float64
	Leverage    float64
	PositionID  string
}

type PositionCloseNotice struct {
	Symbol       string
	Direction    string
	Qty          float64
	CloseQty     float64
	EntryPrice   float64
	TriggerPrice float64
	StopPrice    float64
	TakeProfits  []float64
	Reason       string
	RiskPct      float64
	Leverage     float64
	PositionID   string
}

type PositionCloseSummaryNotice struct {
	Symbol      string
	Direction   string
	Qty         float64
	EntryPrice  float64
	ExitPrice   float64
	StopPrice   float64
	TakeProfits []float64
	Reason      string
	RiskPct     float64
	Leverage    float64
	PnLAmount   float64
	PnLPct      float64
	PositionID  string
}

type RiskPlanUpdateScoreItem struct {
	Signal       string
	Weight       float64
	Value        string
	Contribution float64
}

type RiskPlanUpdateNotice struct {
	Symbol         string
	Direction      string
	EntryPrice     float64
	OldStop        float64
	NewStop        float64
	TakeProfits    []float64
	Source         string
	MarkPrice      float64
	ATR            float64
	Volatility     float64
	GateSatisfied  bool
	ScoreTotal     float64
	ScoreThreshold float64
	ScoreBreakdown []RiskPlanUpdateScoreItem
	ParseOK        bool
	TightenReason  string
	TPTightened    bool
	RiskPct        float64
	Leverage       float64
	PositionID     string
}
