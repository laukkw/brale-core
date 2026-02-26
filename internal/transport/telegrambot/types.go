package telegrambot

import "time"

type Config struct {
	Token          string
	RuntimeBaseURL string
	PollTimeout    time.Duration
	UpdateLimit    int
	SessionTTL     time.Duration
	RequestTimeout time.Duration
	LockPath       string
}

type ObserveRequest struct {
	Symbol string `json:"symbol"`
}

type ObserveResponse struct {
	Symbol         string         `json:"symbol"`
	Status         string         `json:"status"`
	Agent          map[string]any `json:"agent,omitempty"`
	Provider       map[string]any `json:"provider,omitempty"`
	Gate           map[string]any `json:"gate"`
	InPosition     map[string]any `json:"in_position,omitempty"`
	Report         string         `json:"report,omitempty"`
	ReportMarkdown string         `json:"report_markdown,omitempty"`
	ReportHTML     string         `json:"report_html,omitempty"`
	Summary        string         `json:"summary"`
	RequestID      string         `json:"request_id"`
	SkippedExec    bool           `json:"skipped_execution"`
	TraceID        string         `json:"llm_trace_id,omitempty"`
}

type MonitorStatusResponse struct {
	Status    string                `json:"status"`
	Symbols   []MonitorSymbolConfig `json:"symbols"`
	Summary   string                `json:"summary"`
	RequestID string                `json:"request_id"`
}

type MonitorSymbolConfig struct {
	Symbol              string    `json:"symbol"`
	NextRun             time.Time `json:"next_run"`
	KlineInterval       string    `json:"kline_interval"`
	RiskPct             float64   `json:"risk_pct"`
	RiskAmount          float64   `json:"risk_amount"`
	MaxLeverage         float64   `json:"max_leverage"`
	TakeProfitMultiple  float64   `json:"take_profit_multiple"`
	InitialStopMultiple float64   `json:"initial_stop_multiple"`
	EntryPricingMode    string    `json:"entry_pricing_mode"`
}

type PositionStatusResponse struct {
	Status    string               `json:"status"`
	Positions []PositionStatusItem `json:"positions"`
	Summary   string               `json:"summary"`
	RequestID string               `json:"request_id"`
}

type PositionStatusItem struct {
	Symbol           string    `json:"symbol"`
	Amount           float64   `json:"amount"`
	AmountRequested  float64   `json:"amount_requested"`
	MarginAmount     float64   `json:"margin_amount"`
	EntryPrice       float64   `json:"entry_price"`
	CurrentPrice     float64   `json:"current_price"`
	Side             string    `json:"side"`
	ProfitTotal      float64   `json:"profit_total"`
	ProfitRealized   float64   `json:"profit_realized"`
	ProfitUnrealized float64   `json:"profit_unrealized"`
	OpenedAt         string    `json:"opened_at"`
	DurationMin      int64     `json:"duration_min"`
	DurationSec      int64     `json:"duration_sec"`
	TakeProfits      []float64 `json:"take_profits"`
	StopLoss         float64   `json:"stop_loss"`
}

type TradeHistoryResponse struct {
	Status    string             `json:"status"`
	Trades    []TradeHistoryItem `json:"trades"`
	Summary   string             `json:"summary"`
	RequestID string             `json:"request_id"`
}

type TradeHistoryItem struct {
	Symbol       string    `json:"symbol"`
	Side         string    `json:"side"`
	Amount       float64   `json:"amount"`
	MarginAmount float64   `json:"margin_amount"`
	OpenedAt     time.Time `json:"opened_at"`
	DurationSec  int64     `json:"duration_sec"`
	Profit       float64   `json:"profit"`
}

type DecisionLatestResponse struct {
	Status         string `json:"status"`
	Symbol         string `json:"symbol"`
	SnapshotID     uint   `json:"snapshot_id,omitempty"`
	Report         string `json:"report"`
	ReportMarkdown string `json:"report_markdown"`
	ReportHTML     string `json:"report_html"`
	Summary        string `json:"summary"`
	RequestID      string `json:"request_id"`
}

type NewsOverlayLatestResponse struct {
	Status         string `json:"status"`
	UpdatedAt      string `json:"updated_at,omitempty"`
	LLMDecisionRaw string `json:"llm_decision_raw,omitempty"`
	Stale          bool   `json:"stale"`
	StaleAfter     string `json:"stale_after,omitempty"`
	Summary        string `json:"summary"`
	RequestID      string `json:"request_id"`
}

type ScheduleResponse struct {
	Status       string               `json:"status"`
	LLMScheduled bool                 `json:"llm_scheduled"`
	Mode         string               `json:"mode"`
	NextRuns     []ScheduleNextRun    `json:"next_runs"`
	Positions    []PositionStatusItem `json:"positions,omitempty"`
	Summary      string               `json:"summary"`
	RequestID    string               `json:"request_id"`
}

type ScheduleNextRun struct {
	Symbol        string `json:"symbol"`
	BarInterval   string `json:"bar_interval"`
	NextExecution string `json:"next_execution"`
}
