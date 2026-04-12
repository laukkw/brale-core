package features

import (
	"math"
	"testing"

	"brale-core/internal/config"
	"brale-core/internal/snapshot"
)

func TestBuildTrendCompressedInputIncludesSuperTrend(t *testing.T) {
	required := config.SuperTrendRequiredBars(14, 2.5)
	candles := trendTestCandles(required + 20)

	got, err := BuildTrendCompressedInput("BTCUSDT", "1h", candles, DefaultTrendCompressOptions())
	if err != nil {
		t.Fatalf("BuildTrendCompressedInput() error = %v", err)
	}
	if got.SuperTrend == nil {
		t.Fatalf("SuperTrend = nil")
	}
	if got.SuperTrend.Interval != "1h" {
		t.Fatalf("SuperTrend.Interval=%q want 1h", got.SuperTrend.Interval)
	}
	if got.SuperTrend.State != "UP" && got.SuperTrend.State != "DOWN" {
		t.Fatalf("SuperTrend.State=%q", got.SuperTrend.State)
	}
	if got.SuperTrend.Level <= 0 {
		t.Fatalf("SuperTrend.Level=%v", got.SuperTrend.Level)
	}
	if math.IsNaN(got.SuperTrend.DistancePct) || math.IsInf(got.SuperTrend.DistancePct, 0) {
		t.Fatalf("SuperTrend.DistancePct=%v", got.SuperTrend.DistancePct)
	}
}

func TestBuildTrendCompressedInputIncludesEMA20AtThreshold(t *testing.T) {
	required := config.EMARequiredBars(20)
	opts := DefaultTrendCompressOptions()
	opts.SkipSuperTrend = true

	got, err := BuildTrendCompressedInput("BTCUSDT", "1h", trendTestCandles(required), opts)
	if err != nil {
		t.Fatalf("BuildTrendCompressedInput() error = %v", err)
	}
	if got.GlobalContext.EMA20 == nil {
		t.Fatalf("EMA20 = nil at threshold=%d", required)
	}

	got, err = BuildTrendCompressedInput("BTCUSDT", "1h", trendTestCandles(required-1), opts)
	if err != nil {
		t.Fatalf("BuildTrendCompressedInput() error = %v", err)
	}
	if got.GlobalContext.EMA20 != nil {
		t.Fatalf("EMA20=%v want nil below threshold", *got.GlobalContext.EMA20)
	}
}

func TestBuildTrendCompressedInputOmitsSuperTrendWhenBarsInsufficient(t *testing.T) {
	required := config.SuperTrendRequiredBars(14, 2.5)
	candles := trendTestCandles(required - 1)

	got, err := BuildTrendCompressedInput("BTCUSDT", "1h", candles, DefaultTrendCompressOptions())
	if err != nil {
		t.Fatalf("BuildTrendCompressedInput() error = %v", err)
	}
	if got.SuperTrend != nil {
		t.Fatalf("SuperTrend=%+v want nil", got.SuperTrend)
	}
}

func TestBuildTrendCompressedInputIncludesSuperTrendAtThreshold(t *testing.T) {
	required := config.SuperTrendRequiredBars(14, 2.5)
	candles := trendTestCandles(required)

	got, err := BuildTrendCompressedInput("BTCUSDT", "1h", candles, DefaultTrendCompressOptions())
	if err != nil {
		t.Fatalf("BuildTrendCompressedInput() error = %v", err)
	}
	if got.SuperTrend == nil {
		t.Fatalf("SuperTrend = nil at threshold=%d", required)
	}
}

func TestSuperTrendRequiredBarsChangeWithParams(t *testing.T) {
	base := config.SuperTrendRequiredBars(14, 2.5)
	alt := config.SuperTrendRequiredBars(21, 3.0)
	if alt <= base {
		t.Fatalf("alt=%d want > base=%d", alt, base)
	}
}

func TestComputeSuperTrendSeriesGoldenTail(t *testing.T) {
	n := 80
	highs := make([]float64, n)
	lows := make([]float64, n)
	closes := make([]float64, n)
	for i := range n {
		base := 100 + math.Sin(float64(i)/9.0)*2.2 + float64(i)*0.18
		wave := math.Sin(float64(i)/3.5)*1.3 + math.Cos(float64(i)/6.5)*0.7
		close := base + wave
		highs[i] = close + 1.1 + math.Sin(float64(i)/5.0)*0.2
		lows[i] = close - 1.0 - math.Cos(float64(i)/4.0)*0.2
		closes[i] = close
	}
	got := roundSeriesTail(computeSuperTrendSeries(highs, lows, closes, 14, 2.5), 5)
	want := []float64{110.9938, 110.9938, 110.9938, 110.9938, 110.9938}
	if len(got) != len(want) {
		t.Fatalf("tail len=%d want %d tail=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tail[%d]=%v want %v full=%v", i, got[i], want[i], got)
		}
	}
}

func TestBuildTrendCompressedInputSuperTrendNoNaNOnSideways(t *testing.T) {
	required := config.SuperTrendRequiredBars(14, 2.5)
	candles := sidewaysTrendTestCandles(required + 10)

	got, err := BuildTrendCompressedInput("BTCUSDT", "1h", candles, DefaultTrendCompressOptions())
	if err != nil {
		t.Fatalf("BuildTrendCompressedInput() error = %v", err)
	}
	if got.SuperTrend == nil {
		t.Fatalf("SuperTrend = nil")
	}
	if math.IsNaN(got.SuperTrend.Level) || math.IsInf(got.SuperTrend.Level, 0) {
		t.Fatalf("SuperTrend.Level=%v", got.SuperTrend.Level)
	}
	if math.IsNaN(got.SuperTrend.DistancePct) || math.IsInf(got.SuperTrend.DistancePct, 0) {
		t.Fatalf("SuperTrend.DistancePct=%v", got.SuperTrend.DistancePct)
	}
}

func TestDefaultTrendCompressOptionsUseFiveRecentCandles(t *testing.T) {
	if got := DefaultTrendCompressOptions().RecentCandles; got != 5 {
		t.Fatalf("RecentCandles=%d want 5", got)
	}
}

func TestBuildTrendCompressedInputLimitsStructureCandidatesPerSide(t *testing.T) {
	opts := DefaultTrendCompressOptions()
	opts.SkipSuperTrend = true

	got, err := BuildTrendCompressedInput("BTCUSDT", "1h", oscillatingTrendTestCandles(260), opts)
	if err != nil {
		t.Fatalf("BuildTrendCompressedInput() error = %v", err)
	}
	if len(got.StructureCandidates) > 6 {
		t.Fatalf("structure_candidates len=%d want <= 6", len(got.StructureCandidates))
	}
	current := got.RecentCandles[len(got.RecentCandles)-1].C
	below := 0
	above := 0
	for _, candidate := range got.StructureCandidates {
		if candidate.Price <= current {
			below++
		} else {
			above++
		}
	}
	if below > 3 {
		t.Fatalf("below candidates=%d want <= 3", below)
	}
	if above > 3 {
		t.Fatalf("above candidates=%d want <= 3", above)
	}
}

func trendTestCandles(n int) []snapshot.Candle {
	if n < 1 {
		return nil
	}
	candles := make([]snapshot.Candle, n)
	for i := range n {
		base := 100.0 + float64(i)*0.8
		candles[i] = snapshot.Candle{
			OpenTime: int64(i) * 60_000,
			Open:     base,
			High:     base + 1.2,
			Low:      base - 1.1,
			Close:    base + 0.6,
			Volume:   1000 + float64(i)*5,
		}
	}
	return candles
}

func sidewaysTrendTestCandles(n int) []snapshot.Candle {
	if n < 1 {
		return nil
	}
	candles := make([]snapshot.Candle, n)
	for i := range n {
		offset := 0.4
		if i%2 == 1 {
			offset = -0.4
		}
		base := 100.0 + offset
		candles[i] = snapshot.Candle{
			OpenTime: int64(i) * 60_000,
			Open:     base - 0.2,
			High:     base + 0.8,
			Low:      base - 0.8,
			Close:    base + 0.1,
			Volume:   1000,
		}
	}
	return candles
}

func oscillatingTrendTestCandles(n int) []snapshot.Candle {
	if n < 1 {
		return nil
	}
	candles := make([]snapshot.Candle, n)
	for i := range n {
		base := 100.0 + math.Sin(float64(i)/7.0)*6 + math.Cos(float64(i)/13.0)*2
		candles[i] = snapshot.Candle{
			OpenTime: int64(i) * 60_000,
			Open:     base - 0.6,
			High:     base + 1.4,
			Low:      base - 1.5,
			Close:    base + math.Sin(float64(i)/5.0)*0.5,
			Volume:   1000 + math.Cos(float64(i)/11.0)*40,
		}
	}
	return candles
}
