// 本文件主要内容：基于 talib-cdl-go 计算核心蜡烛形态并输出最小证据结构。
package cdl

import (
	"math"
	"sort"

	talibcdl "github.com/iwat/talib-cdl-go"
)

type Candle struct {
	Open  float64
	High  float64
	Low   float64
	Close float64
}

type DetectedPattern struct {
	Name  string `json:"name"`
	Score int    `json:"score"`
	Index int    `json:"idx"`
}

type Result struct {
	Detected []DetectedPattern `json:"detected,omitempty"`
	Primary  string            `json:"primary,omitempty"`
	Strength int               `json:"strength,omitempty"`
}

type Options struct {
	Lookback    int
	Penetration float64
	MaxDetected int
}

func DefaultOptions() Options {
	return Options{
		Lookback:    5,
		Penetration: 0.3,
		MaxDetected: 10,
	}
}

type patternSpec struct {
	name string
	fn   func(talibcdl.Series) []int
}

type patternSpecWithPenetration struct {
	name string
	fn   func(talibcdl.Series, float64) []int
}

var patternSpecs = []patternSpec{
	{name: "doji", fn: talibcdl.Doji},
	{name: "doji_star", fn: talibcdl.DojiStar},
	{name: "piercing", fn: talibcdl.Piercing},
	{name: "three_inside", fn: talibcdl.ThreeInside},
	{name: "three_outside", fn: talibcdl.ThreeOutside},
	{name: "three_black_crows", fn: talibcdl.ThreeBlackCrows},
	{name: "three_white_soldiers", fn: talibcdl.ThreeWhiteSoldiers},
}

var patternSpecsWithPenetration = []patternSpecWithPenetration{
	{name: "evening_star", fn: talibcdl.EveningStar},
}

func Detect(candles []Candle, opts Options) Result {
	if len(candles) == 0 {
		return Result{}
	}
	opts = normalizeOptions(opts, len(candles))
	series := buildSeries(candles)
	result := detectPatterns(series, opts)
	if len(result.Detected) == 0 {
		return result
	}
	limitDetected(&result, opts.MaxDetected)
	resolvePrimary(&result, len(candles))
	return result
}

func normalizeOptions(opts Options, total int) Options {
	if opts.Lookback <= 0 || opts.Lookback > total {
		opts.Lookback = total
	}
	if opts.Penetration <= 0 {
		opts.Penetration = 0.3
	}
	return opts
}

func buildSeries(candles []Candle) talibcdl.SimpleSeries {
	opens := make([]float64, len(candles))
	highs := make([]float64, len(candles))
	lows := make([]float64, len(candles))
	closes := make([]float64, len(candles))
	for i, c := range candles {
		opens[i] = c.Open
		highs[i] = c.High
		lows[i] = c.Low
		closes[i] = c.Close
	}
	return talibcdl.SimpleSeries{
		Opens:  opens,
		Highs:  highs,
		Lows:   lows,
		Closes: closes,
	}
}

func detectPatterns(series talibcdl.SimpleSeries, opts Options) Result {
	result := Result{}
	for _, spec := range patternSpecs {
		if score, idx, ok := lastSignal(spec.fn(series), opts.Lookback); ok {
			result.Detected = append(result.Detected, DetectedPattern{
				Name:  spec.name,
				Score: score,
				Index: idx,
			})
		}
	}
	for _, spec := range patternSpecsWithPenetration {
		if score, idx, ok := lastSignal(spec.fn(series, opts.Penetration), opts.Lookback); ok {
			result.Detected = append(result.Detected, DetectedPattern{
				Name:  spec.name,
				Score: score,
				Index: idx,
			})
		}
	}
	return result
}

func limitDetected(result *Result, maxDetected int) {
	if result == nil || maxDetected <= 0 || len(result.Detected) <= maxDetected {
		return
	}
	sort.Slice(result.Detected, func(i, j int) bool {
		ai := math.Abs(float64(result.Detected[i].Score))
		aj := math.Abs(float64(result.Detected[j].Score))
		if ai != aj {
			return ai > aj
		}
		if result.Detected[i].Index != result.Detected[j].Index {
			return result.Detected[i].Index > result.Detected[j].Index
		}
		return result.Detected[i].Name < result.Detected[j].Name
	})
	result.Detected = result.Detected[:maxDetected]
}

func resolvePrimary(result *Result, total int) {
	if result == nil {
		return
	}
	bestScore := -1
	bestIndex := -total - 1
	for _, item := range result.Detected {
		absScore := item.Score
		if absScore < 0 {
			absScore = -absScore
		}
		result.Strength += absScore
		if absScore > bestScore {
			bestScore = absScore
			bestIndex = item.Index
			result.Primary = item.Name
			continue
		}
		if absScore == bestScore && item.Index > bestIndex {
			bestIndex = item.Index
			result.Primary = item.Name
		}
	}
}

func lastSignal(values []int, lookback int) (int, int, bool) {
	if len(values) == 0 {
		return 0, 0, false
	}
	if lookback <= 0 || lookback > len(values) {
		lookback = len(values)
	}
	for i := len(values) - 1; i >= len(values)-lookback; i-- {
		if values[i] != 0 {
			return values[i], i - len(values), true
		}
	}
	return 0, 0, false
}
