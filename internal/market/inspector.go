package market

import "time"

// StreamStatus holds the current state of a price stream for a symbol.
type StreamStatus struct {
	Symbol      string    `json:"symbol"`
	Source      string    `json:"source"`
	Connected   bool      `json:"ws_connected"`
	LastPrice   float64   `json:"last_mark_price"`
	LastPriceTS time.Time `json:"last_mark_ts"`
	AgeMs       int64     `json:"age_ms"`
	Fresh       bool      `json:"fresh"`
}

// PriceStreamInspector is an optional interface that a PriceSource may implement
// to expose internal stream health for observability and E2E testing.
type PriceStreamInspector interface {
	StreamStatus(symbol string) (StreamStatus, bool)
}

type LiquidationStreamStatus struct {
	Symbol          string `json:"symbol"`
	Source          string `json:"source"`
	Status          string `json:"status"`
	StreamConnected bool   `json:"stream_connected"`
	ShardCount      int    `json:"shard_count"`
	CoverageSec     int64  `json:"coverage_sec"`
	SampleCount     int    `json:"sample_count"`
	LastEventAgeSec int64  `json:"last_event_age_sec"`
	Complete        bool   `json:"complete"`
}

type LiquidationStreamInspector interface {
	LiquidationStreamStatus(symbol string) (LiquidationStreamStatus, bool)
}
