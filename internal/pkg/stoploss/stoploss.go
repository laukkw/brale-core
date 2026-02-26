// 本文件主要内容：计算结构止损与 ATR 兜底止损。
package stoploss

import (
	"fmt"
	"sort"
)

type Direction string

type SwingType string

type StopSource string

const (
	DirectionLong  Direction = "long"
	DirectionShort Direction = "short"
)

const (
	SwingHigh SwingType = "high"
	SwingLow  SwingType = "low"
)

const (
	SourceStructure StopSource = "structure"
	SourceATR       StopSource = "atr"
)

type SwingPoint struct {
	Index int
	Price float64
	Type  SwingType
}

type Params struct {
	RedundantPoints int
	ATRMultiplier   float64
	BufferPct       float64
	MinTick         float64
}

type Result struct {
	Price  float64
	Source StopSource
	Reason string
}

func ComputeStopLoss(direction Direction, entry float64, atr float64, points []SwingPoint, params Params) (Result, error) {
	switch direction {
	case DirectionLong, DirectionShort:
	default:
		return Result{}, fmt.Errorf("direction is required")
	}
	if entry <= 0 {
		return Result{}, fmt.Errorf("entry must be positive")
	}
	if params.RedundantPoints <= 0 {
		return Result{}, fmt.Errorf("redundant_points must be positive")
	}
	buffer, err := computeBuffer(entry, atr, params)
	if err != nil {
		return Result{}, err
	}
	needType := SwingLow
	if direction == DirectionShort {
		needType = SwingHigh
	}
	selected := selectRecent(points, needType, params.RedundantPoints)
	if len(selected) < params.RedundantPoints {
		price := stopFromEntry(direction, entry, buffer)
		if price <= 0 {
			return Result{}, fmt.Errorf("stop price invalid")
		}
		return Result{Price: price, Source: SourceATR, Reason: "atr_fallback"}, nil
	}
	base := selected[0].Price
	for _, pt := range selected[1:] {
		if direction == DirectionLong {
			if pt.Price < base {
				base = pt.Price
			}
			continue
		}
		if pt.Price > base {
			base = pt.Price
		}
	}
	price := stopFromBase(direction, base, buffer)
	if price <= 0 {
		return Result{}, fmt.Errorf("stop price invalid")
	}
	return Result{Price: price, Source: SourceStructure, Reason: "structure_points"}, nil
}

func computeBuffer(entry float64, atr float64, params Params) (float64, error) {
	buffer := 0.0
	if params.ATRMultiplier > 0 && atr > 0 {
		buffer = atr * params.ATRMultiplier
	}
	if params.BufferPct > 0 {
		pct := entry * params.BufferPct
		if pct > buffer {
			buffer = pct
		}
	}
	if params.MinTick > 0 && params.MinTick > buffer {
		buffer = params.MinTick
	}
	if buffer <= 0 {
		return 0, fmt.Errorf("buffer must be positive")
	}
	return buffer, nil
}

func selectRecent(points []SwingPoint, target SwingType, count int) []SwingPoint {
	filtered := make([]SwingPoint, 0, len(points))
	for _, pt := range points {
		if pt.Type == target {
			filtered = append(filtered, pt)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Index > filtered[j].Index
	})
	if len(filtered) > count {
		filtered = filtered[:count]
	}
	return filtered
}

func stopFromEntry(direction Direction, entry float64, buffer float64) float64 {
	if direction == DirectionShort {
		return entry + buffer
	}
	return entry - buffer
}

func stopFromBase(direction Direction, base float64, buffer float64) float64 {
	if direction == DirectionShort {
		return base + buffer
	}
	return base - buffer
}
