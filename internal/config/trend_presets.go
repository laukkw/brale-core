package config

import (
	"math"
	"sort"
	"time"

	"brale-core/internal/interval"
)

type TrendPreset struct {
	FractalSpan         int
	MaxStructurePoints  int
	DedupDistanceBars   int
	DedupATRFactor      float64
	RSIPeriod           int
	ATRPeriod           int
	RecentCandles       int
	VolumeMAPeriod      int
	EMA20Period         int
	EMA50Period         int
	EMA200Period        int
	PatternMinScore     int
	PatternMaxDetected  int
	Pretty              bool
	IncludeCurrentRSI   bool
	IncludeStructureRSI bool
}

const (
	trendRoleEntry   = "entry"
	trendRoleConfirm = "confirm"
	trendRoleBias    = "bias"
)

func DefaultTrendPreset() TrendPreset {
	return TrendPreset{
		FractalSpan:         2,
		MaxStructurePoints:  8,
		DedupDistanceBars:   10,
		DedupATRFactor:      0.5,
		RSIPeriod:           14,
		ATRPeriod:           14,
		RecentCandles:       7,
		VolumeMAPeriod:      20,
		EMA20Period:         20,
		EMA50Period:         50,
		EMA200Period:        200,
		PatternMinScore:     100,
		PatternMaxDetected:  3,
		Pretty:              false,
		IncludeCurrentRSI:   true,
		IncludeStructureRSI: true,
	}
}

func TrendPresetForIntervals(intervals []string) map[string]TrendPreset {
	roles := trendRoleMap(intervals)
	presets := make(map[string]TrendPreset, len(intervals))
	for _, iv := range intervals {
		role := roles[iv]
		minutes := intervalMinutes(iv)
		presets[iv] = trendPresetByRole(role, minutes)
	}
	return presets
}

func TrendPresetForInterval(interval string, intervals []string) TrendPreset {
	presets := TrendPresetForIntervals(intervals)
	if preset, ok := presets[interval]; ok {
		return preset
	}
	return DefaultTrendPreset()
}

func TrendPresetRequiredBars(intervals []string) int {
	maxRequired := 0
	presets := TrendPresetForIntervals(intervals)
	for _, preset := range presets {
		required := presetRequiredBars(preset)
		if required > maxRequired {
			maxRequired = required
		}
	}
	maxRequired = max(1, maxRequired)
	return maxRequired + 1
}

type intervalEntry struct {
	name string
	dur  time.Duration
}

func trendRoleMap(intervals []string) map[string]string {
	roles := make(map[string]string, len(intervals))
	if len(intervals) == 0 {
		return roles
	}
	entries := make([]intervalEntry, 0, len(intervals))
	for _, iv := range intervals {
		dur, err := interval.ParseInterval(iv)
		if err != nil {
			continue
		}
		entries = append(entries, intervalEntry{name: iv, dur: dur})
	}
	if len(entries) == 0 {
		for _, iv := range intervals {
			roles[iv] = trendRoleEntry
		}
		return roles
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].dur < entries[j].dur })
	for idx, entry := range entries {
		switch {
		case idx == 0:
			roles[entry.name] = trendRoleEntry
		case idx == len(entries)-1:
			roles[entry.name] = trendRoleBias
		default:
			roles[entry.name] = trendRoleConfirm
		}
	}
	for _, iv := range intervals {
		if _, ok := roles[iv]; !ok {
			roles[iv] = trendRoleEntry
		}
	}
	return roles
}

func trendPresetByRole(role string, intervalMins int) TrendPreset {
	preset := DefaultTrendPreset()
	switch role {
	case trendRoleEntry:
		preset.FractalSpan = 2
		preset.MaxStructurePoints = 12
		preset.RecentCandles = 10
		preset.DedupDistanceBars = dedupBars(120, intervalMins, 2)
		preset.DedupATRFactor = 0.5
	case trendRoleConfirm:
		preset.FractalSpan = 3
		preset.MaxStructurePoints = 10
		preset.RecentCandles = 9
		preset.DedupDistanceBars = dedupBars(240, intervalMins, 3)
		preset.DedupATRFactor = 0.7
	case trendRoleBias:
		preset.FractalSpan = 4
		preset.MaxStructurePoints = 8
		preset.RecentCandles = 7
		preset.DedupDistanceBars = dedupBars(480, intervalMins, 4)
		preset.DedupATRFactor = 1.0
	}
	return preset
}

func intervalMinutes(iv string) int {
	dur, err := interval.ParseInterval(iv)
	if err != nil {
		return 0
	}
	mins := int(math.Round(dur.Minutes()))
	if mins < 1 {
		return 0
	}
	return mins
}

func dedupBars(targetMinutes, intervalMinutes, minBars int) int {
	if intervalMinutes <= 0 || targetMinutes <= 0 {
		return minBars
	}
	bars := int(math.Round(float64(targetMinutes) / float64(intervalMinutes)))
	if bars < minBars {
		return minBars
	}
	return bars
}

func presetRequiredBars(preset TrendPreset) int {
	return maxInt(
		preset.RSIPeriod,
		preset.ATRPeriod,
		preset.VolumeMAPeriod,
		preset.EMA20Period,
		preset.EMA50Period,
		preset.EMA200Period,
		preset.RecentCandles,
		preset.FractalSpan*2+1,
	)
}
