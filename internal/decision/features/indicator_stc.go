package features

import (
	"math"
)

type stcSnapshot struct {
	Current float64   `json:"current"`
	LastN   []float64 `json:"last_n,omitempty"`
	State   string    `json:"state,omitempty"`
}

func buildSTCSnapshot(series []float64, tail int) *stcSnapshot {
	const stcStateDelta = 2.0

	if len(series) == 0 {
		return nil
	}
	current := series[len(series)-1]
	snap := &stcSnapshot{
		Current: roundFloat(current, 4),
		LastN:   roundSeriesTail(series, tail),
	}
	if len(snap.LastN) < 2 {
		snap.State = "flat"
		return snap
	}
	prev := snap.LastN[len(snap.LastN)-2]
	switch {
	case current-prev > stcStateDelta:
		snap.State = "rising"
	case prev-current > stcStateDelta:
		snap.State = "falling"
	default:
		snap.State = "flat"
	}
	return snap
}

func rollingStochastic(series []float64, period int) []float64 {
	out := make([]float64, len(series))
	for i := range series {
		out[i] = math.NaN()
		if period <= 0 || i+1 < period {
			continue
		}
		start := i + 1 - period
		lo := series[start]
		hi := series[start]
		for j := start + 1; j <= i; j++ {
			if series[j] < lo {
				lo = series[j]
			}
			if series[j] > hi {
				hi = series[j]
			}
		}
		if math.Abs(hi-lo) <= 1e-12 {
			continue
		}
		out[i] = 100 * (series[i] - lo) / (hi - lo)
	}
	return out
}

func rollingSMA(series []float64, period int) []float64 {
	out := make([]float64, len(series))
	for i := range series {
		out[i] = math.NaN()
		if period <= 0 || i+1 < period {
			continue
		}
		sum := 0.0
		valid := true
		start := i + 1 - period
		for j := start; j <= i; j++ {
			v := series[j]
			if math.IsNaN(v) || math.IsInf(v, 0) {
				valid = false
				break
			}
			sum += v
		}
		if !valid {
			continue
		}
		out[i] = sum / float64(period)
	}
	return out
}

func clampFloat64(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
