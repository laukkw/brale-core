package features

import (
	"math"
	"sort"
	"strings"

	"brale-core/internal/pkg/numutil"
	"brale-core/internal/snapshot"

	talib "github.com/markcheno/go-talib"
)

func selectStructurePoints(candles []snapshot.Candle, highs, lows, rsi, atr []float64, opts TrendCompressOptions) []TrendStructurePoint {
	n := len(candles)
	span := opts.FractalSpan
	if n < span*2+1 {
		return nil
	}
	selected := make([]TrendStructurePoint, 0, opts.MaxStructurePoints)
	for idx := n - span - 1; idx >= span; idx-- {
		if isFractalHigh(highs, idx, span) {
			p := TrendStructurePoint{Idx: idx, Type: "High", Price: roundFloat(highs[idx], 4)}
			if opts.IncludeStructureRSI && idx < len(rsi) {
				v := roundFloat(rsi[idx], 1)
				p.RSI = &v
			}
			selected = mergeStructurePoint(selected, p, atr, opts)
		}
		if isFractalLow(lows, idx, span) {
			p := TrendStructurePoint{Idx: idx, Type: "Low", Price: roundFloat(lows[idx], 4)}
			if opts.IncludeStructureRSI && idx < len(rsi) {
				v := roundFloat(rsi[idx], 1)
				p.RSI = &v
			}
			selected = mergeStructurePoint(selected, p, atr, opts)
		}
		if len(selected) >= opts.MaxStructurePoints {
			continue
		}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].Idx < selected[j].Idx })
	return selected
}

func mergeStructurePoint(existing []TrendStructurePoint, candidate TrendStructurePoint, atr []float64, opts TrendCompressOptions) []TrendStructurePoint {
	for i := range existing {
		other := existing[i]
		if other.Type != candidate.Type {
			continue
		}
		distance := numutil.AbsInt(other.Idx - candidate.Idx)
		if distance >= opts.DedupDistanceBars {
			continue
		}
		threshold := 0.0
		if candidate.Idx >= 0 && candidate.Idx < len(atr) {
			threshold = atr[candidate.Idx] * opts.DedupATRFactor
		}
		if threshold <= 0 && other.Idx >= 0 && other.Idx < len(atr) {
			threshold = atr[other.Idx] * opts.DedupATRFactor
		}
		if threshold <= 0 {
			continue
		}
		if math.Abs(other.Price-candidate.Price) >= threshold {
			continue
		}
		switch candidate.Type {
		case "High":
			if candidate.Price > other.Price {
				existing[i] = candidate
			}
		case "Low":
			if candidate.Price < other.Price {
				existing[i] = candidate
			}
		}
		return existing
	}
	if len(existing) >= opts.MaxStructurePoints {
		return existing
	}
	return append(existing, candidate)
}

func isFractalHigh(highs []float64, idx, span int) bool {
	v := highs[idx]
	for i := 1; i <= span; i++ {
		if v <= highs[idx-i] || v <= highs[idx+i] {
			return false
		}
	}
	return true
}

func isFractalLow(lows []float64, idx, span int) bool {
	v := lows[idx]
	for i := 1; i <= span; i++ {
		if v >= lows[idx-i] || v >= lows[idx+i] {
			return false
		}
	}
	return true
}

func linRegSlope(series []float64) float64 {
	n := len(series)
	if n == 0 {
		return 0
	}
	var sumX, sumY, sumXY, sumXX float64
	fn := float64(n)
	for i, y := range series {
		x := float64(i)
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}
	denom := fn*sumXX - sumX*sumX
	if denom == 0 {
		return 0
	}
	return (fn*sumXY - sumX*sumY) / denom
}

func volumeRatio(volumes []float64, lookback int) float64 {
	n := len(volumes)
	if n == 0 {
		return 0
	}
	if lookback <= 0 {
		lookback = 20
	}
	last := volumes[n-1]
	if n < 2 {
		return 0
	}
	count := lookback
	if count > n-1 {
		count = n - 1
	}
	if count <= 0 {
		return 0
	}
	sum := 0.0
	for i := n - 1 - count; i < n-1; i++ {
		sum += volumes[i]
	}
	avg := sum / float64(count)
	if avg == 0 {
		return 0
	}
	return last / avg
}

func lastNonZero(series []float64) float64 {
	for i := len(series) - 1; i >= 0; i-- {
		v := series[i]
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		if math.Abs(v) <= 1e-12 {
			continue
		}
		return v
	}
	return 0
}

func normalizedSlope(series []float64) float64 {
	if len(series) < 2 {
		return 0
	}
	first := series[0]
	last := series[len(series)-1]
	if math.Abs(first) < 1e-9 {
		return 0
	}
	return (last - first) / math.Abs(first) * 100 / float64(len(series)-1)
}

func trendSlopeState(norm float64) string {
	abs := math.Abs(norm)
	switch {
	case abs < 0.1:
		return "FLAT"
	case abs < 0.4:
		return "MODERATE"
	default:
		return "STEEP"
	}
}

func buildStructureCandidates(candles []snapshot.Candle, highs, lows, atr []float64, gc TrendGlobalContext, points []TrendStructurePoint, opts TrendCompressOptions) []TrendStructureCandidate {
	n := len(candles)
	if n == 0 {
		return nil
	}
	cands := make([]TrendStructureCandidate, 0, 12)
	atrLatest := 0.0
	if len(atr) > 0 {
		atrLatest = atr[len(atr)-1]
	}

	for _, p := range points {
		age := n - 1 - p.Idx
		source := "fractal_low"
		typ := "support"
		if strings.EqualFold(p.Type, "High") {
			source = "fractal_high"
			typ = "resistance"
		}
		cands = append(cands, TrendStructureCandidate{
			Price:      p.Price,
			Type:       typ,
			Source:     source,
			AgeCandles: age,
		})
	}

	addEMA := func(val *float64, source string, window int) {
		if val == nil {
			return
		}
		cands = append(cands, TrendStructureCandidate{
			Price:  roundFloat(*val, 4),
			Type:   "ema",
			Source: source,
			Window: window,
		})
	}
	addEMA(gc.EMA20, "ema20", opts.EMA20Period)
	addEMA(gc.EMA50, "ema50", opts.EMA50Period)
	addEMA(gc.EMA200, "ema200", opts.EMA200Period)

	if opts.VolumeMAPeriod > 0 && n >= opts.VolumeMAPeriod {
		upper, _, lower := talib.BBands(extractCloses(candles), opts.VolumeMAPeriod, 2, 2, talib.SMA)
		if u := lastNonZero(upper); u > 0 {
			cands = append(cands, TrendStructureCandidate{
				Price:  roundFloat(u, 4),
				Type:   "band_upper",
				Source: "bollinger_upper",
				Window: opts.VolumeMAPeriod,
			})
		}
		if l := lastNonZero(lower); l > 0 {
			cands = append(cands, TrendStructureCandidate{
				Price:  roundFloat(l, 4),
				Type:   "band_lower",
				Source: "bollinger_lower",
				Window: opts.VolumeMAPeriod,
			})
		}
	}

	rangeWin := 30
	if rangeWin > n {
		rangeWin = n
	}
	if rangeWin > 0 {
		hi := maxFloat(highs[n-rangeWin:])
		lo := minFloat(lows[n-rangeWin:])
		cands = append(cands, TrendStructureCandidate{
			Price:  roundFloat(hi, 4),
			Type:   "range_high",
			Source: "range_high",
			Window: rangeWin,
		})
		cands = append(cands, TrendStructureCandidate{
			Price:  roundFloat(lo, 4),
			Type:   "range_low",
			Source: "range_low",
			Window: rangeWin,
		})
	}

	return dedupCandidates(cands, atrLatest, opts)
}

func extractCloses(candles []snapshot.Candle) []float64 {
	out := make([]float64, 0, len(candles))
	for _, c := range candles {
		out = append(out, c.Close)
	}
	return out
}

func dedupCandidates(in []TrendStructureCandidate, atr float64, opts TrendCompressOptions) []TrendStructureCandidate {
	if len(in) == 0 {
		return nil
	}
	threshold := atr * opts.DedupATRFactor
	if threshold <= 0 {
		threshold = 0
	}
	out := make([]TrendStructureCandidate, 0, len(in))
	for _, c := range in {
		merged := false
		for i := range out {
			if out[i].Type != c.Type {
				continue
			}
			if threshold > 0 && math.Abs(out[i].Price-c.Price) <= threshold {
				if c.AgeCandles < out[i].AgeCandles || out[i].AgeCandles == 0 {
					out[i] = c
				}
				merged = true
				break
			}
		}
		if !merged {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AgeCandles != out[j].AgeCandles {
			return out[i].AgeCandles < out[j].AgeCandles
		}
		return out[i].Price < out[j].Price
	})
	return out
}
