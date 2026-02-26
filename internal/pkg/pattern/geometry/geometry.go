// 本文件主要内容：按 patternpy 规则识别几何形态并输出证据结果。
package geometry

import (
	"math"

	"brale-core/internal/pkg/numutil"
	"brale-core/internal/pkg/pattern/patternutil"
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
	Window          int
	Lookback        int
	DoubleThreshold float64
	ChannelRange    float64
	MaxDetected     int
}

func DefaultOptions() Options {
	return Options{
		Window:          3,
		Lookback:        0,
		DoubleThreshold: 0.05,
		ChannelRange:    0.1,
		MaxDetected:     10,
	}
}

func Detect(candles []Candle, opts Options) Result {
	if len(candles) == 0 {
		return Result{}
	}
	opts = normalizeOptions(opts, len(candles))
	highs, lows, closes := extractSeries(candles)
	highRollMax, lowRollMin := rollingHighLow(highs, lows, opts.Window)
	trendHigh, trendLow := rollingTrend(highs, lows, opts.Window)

	headMask := detectHeadShoulders(highs, highRollMax)
	invHeadMask := detectInvHeadShoulders(lows, lowRollMin)
	doubleTopMask, doubleBottomMask := detectDoubleTopBottom(highs, lows, highRollMax, lowRollMin, opts.DoubleThreshold)
	triAscMask, triDescMask := detectTriangle(highs, lows, closes, highRollMax, lowRollMin)
	wedgeUpMask, wedgeDownMask := detectWedge(highs, lows, highRollMax, lowRollMin, trendHigh, trendLow)
	channelUpMask, channelDownMask := detectChannel(highs, lows, highRollMax, lowRollMin, trendHigh, trendLow, opts.ChannelRange)

	result := Result{}
	result.Detected = appendIfDetected(result.Detected, "head_shoulders", -150, headMask, opts.Lookback)
	result.Detected = appendIfDetected(result.Detected, "inv_head_shoulders", 150, invHeadMask, opts.Lookback)
	result.Detected = appendIfDetected(result.Detected, "double_top", -120, doubleTopMask, opts.Lookback)
	result.Detected = appendIfDetected(result.Detected, "double_bottom", 120, doubleBottomMask, opts.Lookback)
	result.Detected = appendIfDetected(result.Detected, "triangle_asc", 100, triAscMask, opts.Lookback)
	result.Detected = appendIfDetected(result.Detected, "triangle_desc", -100, triDescMask, opts.Lookback)
	result.Detected = appendIfDetected(result.Detected, "wedge_rising", -120, wedgeUpMask, opts.Lookback)
	result.Detected = appendIfDetected(result.Detected, "wedge_falling", 120, wedgeDownMask, opts.Lookback)
	result.Detected = appendIfDetected(result.Detected, "channel_up", 100, channelUpMask, opts.Lookback)
	result.Detected = appendIfDetected(result.Detected, "channel_down", -100, channelDownMask, opts.Lookback)

	if opts.MaxDetected > 0 && len(result.Detected) > opts.MaxDetected {
		patternutil.SortByScoreIndexName(result.Detected,
			func(item DetectedPattern) int { return item.Score },
			func(item DetectedPattern) int { return item.Index },
			func(item DetectedPattern) string { return item.Name },
		)
		result.Detected = result.Detected[:opts.MaxDetected]
	}
	if len(result.Detected) == 0 {
		return result
	}
	result.Primary, result.Strength = pickPrimary(result.Detected)
	return result
}

func normalizeOptions(opts Options, total int) Options {
	if opts.Window <= 0 {
		opts.Window = 3
	}
	if opts.Lookback <= 0 || opts.Lookback > total {
		opts.Lookback = total
	}
	if opts.DoubleThreshold <= 0 {
		opts.DoubleThreshold = 0.05
	}
	if opts.ChannelRange <= 0 {
		opts.ChannelRange = 0.1
	}
	return opts
}

func extractSeries(candles []Candle) ([]float64, []float64, []float64) {
	highs := make([]float64, len(candles))
	lows := make([]float64, len(candles))
	closes := make([]float64, len(candles))
	for i, c := range candles {
		highs[i] = c.High
		lows[i] = c.Low
		closes[i] = c.Close
	}
	return highs, lows, closes
}

func rollingHighLow(highs, lows []float64, window int) ([]float64, []float64) {
	n := len(highs)
	highRoll := make([]float64, n)
	lowRoll := make([]float64, n)
	for i := 0; i < n; i++ {
		highRoll[i] = math.NaN()
		lowRoll[i] = math.NaN()
		if i+1 < window {
			continue
		}
		start := i - window + 1
		maxVal := highs[start]
		minVal := lows[start]
		for j := start + 1; j <= i; j++ {
			if highs[j] > maxVal {
				maxVal = highs[j]
			}
			if lows[j] < minVal {
				minVal = lows[j]
			}
		}
		highRoll[i] = maxVal
		lowRoll[i] = minVal
	}
	return highRoll, lowRoll
}

func rollingTrend(highs, lows []float64, window int) ([]int, []int) {
	n := len(highs)
	trendHigh := make([]int, n)
	trendLow := make([]int, n)
	for i := 0; i < n; i++ {
		if i+1 < window {
			continue
		}
		start := i - window + 1
		trendHigh[i] = trendDirection(highs[start], highs[i])
		trendLow[i] = trendDirection(lows[start], lows[i])
	}
	return trendHigh, trendLow
}

func trendDirection(first, last float64) int {
	delta := last - first
	switch {
	case delta > 0:
		return 1
	case delta < 0:
		return -1
	default:
		return 0
	}
}

func detectHeadShoulders(highs, highRollMax []float64) []bool {
	n := len(highs)
	mask := make([]bool, n)
	for i := 1; i+1 < n; i++ {
		roll := highRollMax[i]
		if !isValid(roll) {
			continue
		}
		if roll > highs[i-1] && roll > highs[i+1] && highs[i] < highs[i-1] && highs[i] < highs[i+1] {
			mask[i] = true
		}
	}
	return mask
}

func detectInvHeadShoulders(lows, lowRollMin []float64) []bool {
	n := len(lows)
	mask := make([]bool, n)
	for i := 1; i+1 < n; i++ {
		roll := lowRollMin[i]
		if !isValid(roll) {
			continue
		}
		if roll < lows[i-1] && roll < lows[i+1] && lows[i] > lows[i-1] && lows[i] > lows[i+1] {
			mask[i] = true
		}
	}
	return mask
}

func detectDoubleTopBottom(highs, lows, highRollMax, lowRollMin []float64, threshold float64) ([]bool, []bool) {
	n := len(highs)
	top := make([]bool, n)
	bottom := make([]bool, n)
	for i := 1; i+1 < n; i++ {
		if isValid(highRollMax[i]) {
			if highRollMax[i] >= highs[i-1] && highRollMax[i] >= highs[i+1] && highs[i] < highs[i-1] && highs[i] < highs[i+1] {
				if rangeWithinThreshold(highs[i-1], lows[i-1], threshold) && rangeWithinThreshold(highs[i+1], lows[i+1], threshold) {
					top[i] = true
				}
			}
		}
		if isValid(lowRollMin[i]) {
			if lowRollMin[i] <= lows[i-1] && lowRollMin[i] <= lows[i+1] && lows[i] > lows[i-1] && lows[i] > lows[i+1] {
				if rangeWithinThreshold(highs[i-1], lows[i-1], threshold) && rangeWithinThreshold(highs[i+1], lows[i+1], threshold) {
					bottom[i] = true
				}
			}
		}
	}
	return top, bottom
}

func detectTriangle(highs, lows, closes, highRollMax, lowRollMin []float64) ([]bool, []bool) {
	n := len(highs)
	asc := make([]bool, n)
	desc := make([]bool, n)
	for i := 1; i < n; i++ {
		if !isValid(highRollMax[i]) || !isValid(lowRollMin[i]) {
			continue
		}
		if highRollMax[i] >= highs[i-1] && lowRollMin[i] <= lows[i-1] && closes[i] > closes[i-1] {
			asc[i] = true
		}
		if highRollMax[i] <= highs[i-1] && lowRollMin[i] >= lows[i-1] && closes[i] < closes[i-1] {
			desc[i] = true
		}
	}
	return asc, desc
}

func detectWedge(highs, lows, highRollMax, lowRollMin []float64, trendHigh, trendLow []int) ([]bool, []bool) {
	n := len(highs)
	up := make([]bool, n)
	down := make([]bool, n)
	for i := 1; i < n; i++ {
		if !isValid(highRollMax[i]) || !isValid(lowRollMin[i]) {
			continue
		}
		if highRollMax[i] >= highs[i-1] && lowRollMin[i] <= lows[i-1] && trendHigh[i] == 1 && trendLow[i] == 1 {
			up[i] = true
		}
		if highRollMax[i] <= highs[i-1] && lowRollMin[i] >= lows[i-1] && trendHigh[i] == -1 && trendLow[i] == -1 {
			down[i] = true
		}
	}
	return up, down
}

func detectChannel(highs, lows, highRollMax, lowRollMin []float64, trendHigh, trendLow []int, channelRange float64) ([]bool, []bool) {
	n := len(highs)
	up := make([]bool, n)
	down := make([]bool, n)
	for i := 1; i < n; i++ {
		if !isValid(highRollMax[i]) || !isValid(lowRollMin[i]) {
			continue
		}
		if channelWithinRange(highRollMax[i], lowRollMin[i], channelRange) {
			if highRollMax[i] >= highs[i-1] && lowRollMin[i] <= lows[i-1] && trendHigh[i] == 1 && trendLow[i] == 1 {
				up[i] = true
			}
			if highRollMax[i] <= highs[i-1] && lowRollMin[i] >= lows[i-1] && trendHigh[i] == -1 && trendLow[i] == -1 {
				down[i] = true
			}
		}
	}
	return up, down
}

func appendIfDetected(list []DetectedPattern, name string, score int, mask []bool, lookback int) []DetectedPattern {
	idx, ok := lastMatch(mask, lookback)
	if !ok {
		return list
	}
	return append(list, DetectedPattern{
		Name:  name,
		Score: score,
		Index: idx - len(mask),
	})
}

func lastMatch(mask []bool, lookback int) (int, bool) {
	if len(mask) == 0 {
		return 0, false
	}
	start := len(mask) - 1
	end := len(mask) - lookback
	if end < 0 {
		end = 0
	}
	for i := start; i >= end; i-- {
		if mask[i] {
			return i, true
		}
	}
	return 0, false
}

func pickPrimary(items []DetectedPattern) (string, int) {
	bestScore := -1
	bestIndex := -1
	strength := 0
	primary := ""
	for _, item := range items {
		absScore := numutil.AbsInt(item.Score)
		strength += absScore
		if absScore > bestScore {
			bestScore = absScore
			bestIndex = item.Index
			primary = item.Name
			continue
		}
		if absScore == bestScore && item.Index > bestIndex {
			bestIndex = item.Index
			primary = item.Name
		}
	}
	return primary, strength
}

func isValid(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func rangeWithinThreshold(high, low, threshold float64) bool {
	avg := (high + low) / 2
	if avg == 0 {
		return false
	}
	return (high - low) <= threshold*avg
}

func channelWithinRange(high, low, channelRange float64) bool {
	avg := (high + low) / 2
	if avg == 0 {
		return false
	}
	return (high - low) <= channelRange*avg
}
