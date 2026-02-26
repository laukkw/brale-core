// 本文件主要内容：构建有仓位时的摘要输入，用于 Provider 风控判断。

package position

import (
	"fmt"
	"math"
	"strings"
	"time"

	"brale-core/internal/risk"
	"brale-core/internal/store"
)

type PositionSummary struct {
	PositionStatus    string  `json:"position_status"`
	Side              string  `json:"side"`
	Timeframe         string  `json:"timeframe"`
	EntryFillPrice    float64 `json:"entry_fill_price"`
	CurrentPrice      float64 `json:"current_price"`
	UnrealizedR       float64 `json:"unrealized_R"`
	PeakUnrealizedR   float64 `json:"peak_unrealized_R"`
	CurrentSLRelative string  `json:"current_sl_relative"`
	HasPartialTake    bool    `json:"has_partial_take"`
	BarsInPosition    int     `json:"bars_in_position"`
}

type PositionRiskSummary struct {
	Side                  string `json:"side"`
	PositionStatus        string `json:"position_status"`
	Timeframe             string `json:"timeframe,omitempty"`
	UnrealizedRBucket     string `json:"unrealized_R_bucket"`
	PeakUnrealizedRBucket string `json:"peak_unrealized_R_bucket"`
	CurrentSLRelative     string `json:"current_sl_relative"`
	HasPartialTake        bool   `json:"has_partial_take"`
	BarsInPosition        int    `json:"bars_in_position"`
	TimeInPositionBucket  string `json:"time_in_position_bucket,omitempty"`
}

func BuildPositionSummary(pos store.PositionRecord, plan risk.RiskPlan, currentPrice float64, barInterval time.Duration) (PositionSummary, error) {
	if strings.TrimSpace(pos.PositionID) == "" {
		return PositionSummary{}, fmt.Errorf("position_id is required")
	}
	if currentPrice <= 0 {
		return PositionSummary{}, fmt.Errorf("current_price is required")
	}
	if pos.AvgEntry <= 0 {
		return PositionSummary{}, fmt.Errorf("entry_fill_price is required")
	}
	if plan.StopPrice <= 0 {
		return PositionSummary{}, fmt.Errorf("stop_price is required")
	}
	if barInterval <= 0 {
		return PositionSummary{}, fmt.Errorf("bar_interval is required")
	}
	if pos.CreatedAt.IsZero() {
		return PositionSummary{}, fmt.Errorf("position created_at is required")
	}
	side := strings.ToLower(strings.TrimSpace(pos.Side))
	riskDistance := math.Abs(pos.AvgEntry - plan.StopPrice)
	if riskDistance <= 0 {
		return PositionSummary{}, fmt.Errorf("risk distance is required")
	}
	var unrealizedR float64
	switch side {
	case "long":
		unrealizedR = (currentPrice - pos.AvgEntry) / riskDistance
	case "short":
		unrealizedR = (pos.AvgEntry - currentPrice) / riskDistance
	default:
		return PositionSummary{}, fmt.Errorf("side is required")
	}
	relative := (plan.StopPrice - pos.AvgEntry) / riskDistance
	if side == "short" {
		relative = (pos.AvgEntry - plan.StopPrice) / riskDistance
	}
	var bucket string
	switch {
	case relative <= -0.75:
		bucket = "-1R"
	case relative <= -0.25:
		bucket = "-0.5R"
	case relative < 0.25:
		bucket = "BE"
	case relative < 0.75:
		bucket = "+0.5R"
	default:
		bucket = "+1R"
	}
	peak := pos.PeakUnrealizedR
	if unrealizedR > peak {
		peak = unrealizedR
	}
	bars := int(time.Since(pos.CreatedAt) / barInterval)
	if bars < 0 {
		bars = 0
	}
	return PositionSummary{
		PositionStatus:    pos.Status,
		Side:              side,
		Timeframe:         formatTimeframe(barInterval),
		EntryFillPrice:    pos.AvgEntry,
		CurrentPrice:      currentPrice,
		UnrealizedR:       unrealizedR,
		PeakUnrealizedR:   peak,
		CurrentSLRelative: bucket,
		HasPartialTake:    risk.HasTPHits(plan),
		BarsInPosition:    bars,
	}, nil
}

func BuildPositionRiskSummary(summary PositionSummary) (PositionRiskSummary, error) {
	if strings.TrimSpace(summary.Side) == "" {
		return PositionRiskSummary{}, fmt.Errorf("side is required")
	}
	if strings.TrimSpace(summary.PositionStatus) == "" {
		return PositionRiskSummary{}, fmt.Errorf("position_status is required")
	}
	bucket := bucketUnrealizedR(summary.UnrealizedR)
	peakBucket := bucketUnrealizedR(summary.PeakUnrealizedR)
	timeBucket := bucketBarsInPosition(summary.BarsInPosition)
	status := strings.ToLower(strings.TrimSpace(summary.PositionStatus))
	return PositionRiskSummary{
		Side:                  summary.Side,
		PositionStatus:        status,
		Timeframe:             strings.TrimSpace(summary.Timeframe),
		UnrealizedRBucket:     bucket,
		PeakUnrealizedRBucket: peakBucket,
		CurrentSLRelative:     summary.CurrentSLRelative,
		HasPartialTake:        summary.HasPartialTake,
		BarsInPosition:        summary.BarsInPosition,
		TimeInPositionBucket:  timeBucket,
	}, nil
}

func bucketUnrealizedR(val float64) string {
	switch {
	case val <= -0.5:
		return "-1R_to_-0.5R"
	case val < 0:
		return "-0.5R_to_0"
	case val < 0.5:
		return "0_to_0.5R"
	case val < 1:
		return "0.5R_to_1R"
	case val < 1.5:
		return "1R_to_1.5R"
	default:
		return ">1.5R"
	}
}

func bucketBarsInPosition(bars int) string {
	switch {
	case bars < 10:
		return "short"
	case bars < 30:
		return "medium"
	default:
		return "long"
	}
}

func formatTimeframe(interval time.Duration) string {
	if interval%(24*time.Hour) == 0 {
		return fmt.Sprintf("%dd", int(interval/(24*time.Hour)))
	}
	if interval%time.Hour == 0 {
		return fmt.Sprintf("%dh", int(interval/time.Hour))
	}
	if interval%time.Minute == 0 {
		return fmt.Sprintf("%dm", int(interval/time.Minute))
	}
	if interval%time.Second == 0 {
		return fmt.Sprintf("%ds", int(interval/time.Second))
	}
	return interval.String()
}
