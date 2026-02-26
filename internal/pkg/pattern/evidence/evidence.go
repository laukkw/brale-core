// 本文件主要内容：合并几何与蜡烛形态结果并输出统一证据。
package evidence

import (
	"math"

	"brale-core/internal/pkg/numutil"
	"brale-core/internal/pkg/pattern/cdl"
	"brale-core/internal/pkg/pattern/geometry"
	"brale-core/internal/pkg/pattern/patternutil"
)

type DetectedPattern struct {
	Name  string `json:"name"`
	Score int    `json:"score"`
	Bias  string `json:"bias"`
	Index int    `json:"idx"`
}

type Result struct {
	Detected []DetectedPattern `json:"detected,omitempty"`
	Primary  string            `json:"primary,omitempty"`
	Strength int               `json:"strength,omitempty"`
}

type Options struct {
	MinScore    int
	MaxDetected int
}

func DefaultOptions() Options {
	return Options{
		MinScore:    100,
		MaxDetected: 3,
	}
}

func Combine(geom geometry.Result, candle cdl.Result, opts Options) Result {
	result := Result{}
	if len(geom.Detected) == 0 && len(candle.Detected) == 0 {
		return result
	}
	opts = normalizeOptions(opts)
	detected := make([]DetectedPattern, 0, len(geom.Detected)+len(candle.Detected))
	for _, item := range geom.Detected {
		detected = append(detected, DetectedPattern{
			Name:  item.Name,
			Score: item.Score,
			Bias:  biasFromScore(item.Score),
			Index: item.Index,
		})
	}
	for _, item := range candle.Detected {
		detected = append(detected, DetectedPattern{
			Name:  item.Name,
			Score: item.Score,
			Bias:  biasFromScore(item.Score),
			Index: item.Index,
		})
	}
	filtered := filterByMinScore(detected, opts.MinScore)
	if len(filtered) == 0 {
		return result
	}
	patternutil.SortByScoreIndexName(filtered,
		func(item DetectedPattern) int { return item.Score },
		func(item DetectedPattern) int { return item.Index },
		func(item DetectedPattern) string { return item.Name },
	)
	if opts.MaxDetected > 0 && len(filtered) > opts.MaxDetected {
		filtered = filtered[:opts.MaxDetected]
	}
	result.Detected = filtered
	result.Strength = sumStrength(filtered)
	result.Primary = filtered[0].Name
	return result
}

func normalizeOptions(opts Options) Options {
	if opts.MinScore <= 0 {
		opts.MinScore = DefaultOptions().MinScore
	}
	if opts.MaxDetected <= 0 {
		opts.MaxDetected = DefaultOptions().MaxDetected
	}
	return opts
}

func filterByMinScore(items []DetectedPattern, minScore int) []DetectedPattern {
	if minScore <= 0 {
		return items
	}
	filtered := items[:0]
	for _, item := range items {
		if numutil.AbsInt(item.Score) < minScore {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func sumStrength(items []DetectedPattern) int {
	strength := 0
	for _, item := range items {
		strength += int(math.Abs(float64(item.Score)))
	}
	return strength
}

func biasFromScore(score int) string {
	if score > 0 {
		return "bullish"
	}
	if score < 0 {
		return "bearish"
	}
	return "neutral"
}
