// 本文件主要内容：从 K 线提取简单形态与趋势摘要，输出 Pattern JSON。

package features

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"brale-core/internal/pkg/pattern/cdl"
	"brale-core/internal/snapshot"
)

// Pattern compression payload migrated from brale/internal/analysis/pattern.
// It summarizes simple chart patterns without directional language.

type PatternCompressedInput struct {
	PatternSummary string      `json:"pattern_summary"`
	TrendSummary   string      `json:"trend_summary"`
	Bias           string      `json:"bias"`
	Signals        []string    `json:"signals"`
	Pattern        *cdl.Result `json:"pattern,omitempty"`
}

func BuildPatternCompressedJSON(symbol, interval string, candles []snapshot.Candle) (string, error) {
	payload, err := BuildPatternCompressedInput(candles)
	if err != nil {
		return "", err
	}
	_ = symbol
	_ = interval
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func BuildPatternCompressedInput(candles []snapshot.Candle) (PatternCompressedInput, error) {
	if len(candles) == 0 {
		return PatternCompressedInput{}, fmt.Errorf("no candles")
	}
	closes := make([]float64, len(candles))
	highs := make([]float64, len(candles))
	lows := make([]float64, len(candles))
	patternCandles := make([]cdl.Candle, len(candles))
	for i, c := range candles {
		closes[i] = c.Close
		highs[i] = c.High
		lows[i] = c.Low
		patternCandles[i] = cdl.Candle{
			Open:  c.Open,
			High:  c.High,
			Low:   c.Low,
			Close: c.Close,
		}
	}
	slope, intercept := fitLine(closes)
	bias := ""
	trend := describeTrend(slope, intercept, closes)

	signals := make([]string, 0, 4)
	if desc, ok := detectDoubleBottom(closes, lows); ok {
		signals = append(signals, desc)
	}
	if desc, ok := detectDoubleTop(highs); ok {
		signals = append(signals, desc)
	}
	if desc, ok := detectTriangle(highs, lows); ok {
		signals = append(signals, desc)
	}
	if desc, ok := detectCompression(highs, lows); ok {
		signals = append(signals, desc)
	}

	patternSummary := "未发现显著形态"
	if len(signals) > 0 {
		patternSummary = strings.Join(signals, "；")
	}
	pattern := cdl.Detect(patternCandles, cdl.DefaultOptions())
	var patternRef *cdl.Result
	if len(pattern.Detected) > 0 {
		patternRef = &pattern
	}
	return PatternCompressedInput{
		PatternSummary: patternSummary,
		TrendSummary:   trend,
		Bias:           bias,
		Signals:        signals,
		Pattern:        patternRef,
	}, nil
}

func fitLine(series []float64) (slope, intercept float64) {
	if len(series) == 0 {
		return 0, 0
	}
	var sumX, sumY, sumXY, sumXX float64
	n := float64(len(series))
	for i, y := range series {
		x := float64(i)
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}
	denom := n*sumXX - sumX*sumX
	if denom == 0 {
		return 0, series[len(series)-1]
	}
	slope = (n*sumXY - sumX*sumY) / denom
	intercept = (sumY - slope*sumX) / n
	return
}

func describeTrend(slope, intercept float64, closes []float64) string {
	last := closes[len(closes)-1]
	ref := intercept + slope*float64(len(closes)-1)
	angle := math.Atan(slope) * 180 / math.Pi
	return fmt.Sprintf("线性回归斜率=%.6f(%.2f°)，收盘价较基线偏移%.2f%%", slope, angle, (last-ref)/ref*100)
}

func detectDoubleBottom(closes, lows []float64) (string, bool) {
	if len(lows) < 20 {
		return "", false
	}
	window := lows[len(lows)/2:]
	min1, idx1 := minWithIndex(window)
	remove := append([]float64{}, window...)
	for i := idx1 - 2; i <= idx1+2 && i >= 0 && i < len(remove); i++ {
		remove[i] = math.MaxFloat64
	}
	min2, idx2 := minWithIndex(remove)
	diff := math.Abs(min1-min2) / math.Max(min1, 1)
	if diff <= 0.004 && idx2 >= 3 {
		desc := fmt.Sprintf("双底形成于最近区间，支撑约%.2f", (min1+min2)/2)
		return desc, true
	}
	return "", false
}

func detectDoubleTop(highs []float64) (string, bool) {
	if len(highs) < 20 {
		return "", false
	}
	window := highs[len(highs)/2:]
	max1, idx1 := maxWithIndex(window)
	remove := append([]float64{}, window...)
	for i := idx1 - 2; i <= idx1+2 && i >= 0 && i < len(remove); i++ {
		remove[i] = -math.MaxFloat64
	}
	max2, idx2 := maxWithIndex(remove)
	diff := math.Abs(max1-max2) / math.Max(max1, 1)
	if diff <= 0.004 && idx2 >= 3 {
		desc := fmt.Sprintf("双顶压制在%.2f附近", (max1+max2)/2)
		return desc, true
	}
	return "", false
}

func detectTriangle(highs, lows []float64) (string, bool) {
	if len(highs) < 30 {
		return "", false
	}
	firstHigh := max(highs[:len(highs)/2])
	lastHigh := max(highs[len(highs)/2:])
	firstLow := min(lows[:len(lows)/2])
	lastLow := min(lows[len(lows)/2:])
	if lastHigh < firstHigh && lastLow > firstLow {
		widthDelta := (firstHigh - firstLow) - (lastHigh - lastLow)
		if widthDelta/firstHigh > 0.05 {
			return "价格区间逐步收敛，疑似对称三角", true
		}
	}
	return "", false
}

func detectCompression(highs, lows []float64) (string, bool) {
	if len(highs) < 40 {
		return "", false
	}
	first := (max(highs[:len(highs)/2]) - min(lows[:len(lows)/2])) / max(highs[:len(highs)/2])
	second := (max(highs[len(highs)/2:]) - min(lows[len(lows)/2:])) / max(highs[len(highs)/2:])
	if second < first*0.65 {
		return "波动率快速收缩，关注突破方向", true
	}
	return "", false
}

func min(values []float64) float64 {
	m := math.MaxFloat64
	for _, v := range values {
		if v < m {
			m = v
		}
	}
	return m
}

func max(values []float64) float64 {
	m := -math.MaxFloat64
	for _, v := range values {
		if v > m {
			m = v
		}
	}
	return m
}

func minWithIndex(values []float64) (float64, int) {
	m := math.MaxFloat64
	idx := -1
	for i, v := range values {
		if v < m {
			m = v
			idx = i
		}
	}
	return m, idx
}

func maxWithIndex(values []float64) (float64, int) {
	m := -math.MaxFloat64
	idx := -1
	for i, v := range values {
		if v > m {
			m = v
			idx = i
		}
	}
	return m, idx
}
