// 本文件主要内容：定义持仓提示词摘要与构建接口。

package positionprompt

import (
	"fmt"
	"strings"

	"brale-core/internal/position"
)

type Summary struct {
	Symbol                string   `json:"symbol"`
	EntryPrice            float64  `json:"entry_price,omitempty"`
	Size                  float64  `json:"size,omitempty"`
	Leverage              *float64 `json:"leverage,omitempty"`
	Side                  string   `json:"side,omitempty"`
	Timeframe             string   `json:"timeframe,omitempty"`
	PositionStatus        string   `json:"position_status,omitempty"`
	UnrealizedRBucket     string   `json:"unrealized_R_bucket,omitempty"`
	PeakUnrealizedRBucket string   `json:"peak_unrealized_R_bucket,omitempty"`
	CurrentSLRelative     string   `json:"current_sl_relative,omitempty"`
	HasPartialTake        bool     `json:"has_partial_take,omitempty"`
	BarsInPosition        int      `json:"bars_in_position,omitempty"`
	TimeInPositionBucket  string   `json:"time_in_position_bucket,omitempty"`
}

type Builder interface {
	Build(symbol string, entryPrice, size float64, leverage *float64) (Summary, error)
	BuildWithRisk(symbol string, entryPrice, size float64, leverage *float64, riskSummary *position.PositionRiskSummary) (Summary, error)
}

type DefaultBuilder struct{}

func NewBuilder() Builder {
	return DefaultBuilder{}
}

func (DefaultBuilder) Build(symbol string, entryPrice, size float64, leverage *float64) (Summary, error) {
	return DefaultBuilder{}.BuildWithRisk(symbol, entryPrice, size, leverage, nil)
}

func (DefaultBuilder) BuildWithRisk(symbol string, entryPrice, size float64, leverage *float64, riskSummary *position.PositionRiskSummary) (Summary, error) {
	sym := strings.TrimSpace(symbol)
	if sym == "" {
		return Summary{}, fmt.Errorf("symbol is required")
	}
	if entryPrice <= 0 {
		return Summary{}, fmt.Errorf("entry_price is required")
	}
	if size <= 0 {
		return Summary{}, fmt.Errorf("size is required")
	}
	if leverage != nil && *leverage <= 0 {
		return Summary{}, fmt.Errorf("leverage must be > 0")
	}
	out := Summary{
		Symbol:     sym,
		EntryPrice: entryPrice,
		Size:       size,
		Leverage:   leverage,
	}
	if riskSummary == nil {
		return out, nil
	}
	out.Side = strings.ToLower(strings.TrimSpace(riskSummary.Side))
	out.Timeframe = strings.TrimSpace(riskSummary.Timeframe)
	out.PositionStatus = strings.ToLower(strings.TrimSpace(riskSummary.PositionStatus))
	out.UnrealizedRBucket = strings.TrimSpace(riskSummary.UnrealizedRBucket)
	out.PeakUnrealizedRBucket = strings.TrimSpace(riskSummary.PeakUnrealizedRBucket)
	out.CurrentSLRelative = strings.TrimSpace(riskSummary.CurrentSLRelative)
	out.HasPartialTake = riskSummary.HasPartialTake
	out.BarsInPosition = riskSummary.BarsInPosition
	out.TimeInPositionBucket = strings.TrimSpace(riskSummary.TimeInPositionBucket)
	return out, nil
}
