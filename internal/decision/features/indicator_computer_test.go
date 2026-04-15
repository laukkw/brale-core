package features

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"brale-core/internal/snapshot"
)

var _ IndicatorComputer = stubIndicatorComputer{}

type stubIndicatorComputer struct {
	ema       map[int][]float64
	rsi       []float64
	atr       []float64
	bbUpper   []float64
	bbMiddle  []float64
	bbLower   []float64
	obv       []float64
	stc       []float64
	chop      []float64
	stochRSI  []float64
	aroonUp   []float64
	aroonDown []float64
}

func (s stubIndicatorComputer) ComputeEMA(_ []float64, period int) ([]float64, error) {
	series, ok := s.ema[period]
	if !ok {
		return nil, fmt.Errorf("unexpected ema period %d", period)
	}
	return cloneSeries(series), nil
}

func (s stubIndicatorComputer) ComputeRSI(_ []float64, _ int) ([]float64, error) {
	return cloneSeries(s.rsi), nil
}

func (s stubIndicatorComputer) ComputeATR(_, _, _ []float64, _ int) ([]float64, error) {
	return cloneSeries(s.atr), nil
}

func (s stubIndicatorComputer) ComputeBB(_ []float64, _ int, _ float64, _ float64) ([]float64, []float64, []float64, error) {
	return cloneSeries(s.bbUpper), cloneSeries(s.bbMiddle), cloneSeries(s.bbLower), nil
}

func (s stubIndicatorComputer) ComputeOBV(_, _ []float64) ([]float64, error) {
	return cloneSeries(s.obv), nil
}

func (s stubIndicatorComputer) ComputeSTC(_ []float64, _, _, _, _ int) ([]float64, error) {
	return cloneSeries(s.stc), nil
}

func (s stubIndicatorComputer) ComputeCHOP(_, _, _ []float64, _ int) ([]float64, error) {
	return cloneSeries(s.chop), nil
}

func (s stubIndicatorComputer) ComputeAroon(_, _ []float64, _ int) ([]float64, []float64, error) {
	return cloneSeries(s.aroonUp), cloneSeries(s.aroonDown), nil
}

func (s stubIndicatorComputer) ComputeStochRSI(_ []float64, _ int) ([]float64, error) {
	return cloneSeries(s.stochRSI), nil
}

func TestBuildIndicatorCompressedInputWithComputerUsesInjectedSeries(t *testing.T) {
	candles := oscillatingTrendTestCandles(260)
	computer := newStubIndicatorComputer(len(candles))

	got, err := BuildIndicatorCompressedInputWithComputer("BTCUSDT", "1h", candles, DefaultIndicatorCompressOptions(), computer)
	if err != nil {
		t.Fatalf("BuildIndicatorCompressedInputWithComputer() error = %v", err)
	}

	if got.Data.EMAFast == nil || got.Data.EMAFast.Latest != roundFloat(computer.ema[21][len(candles)-1], 4) {
		t.Fatalf("EMAFast=%+v", got.Data.EMAFast)
	}
	if got.Data.EMAMid == nil || got.Data.EMAMid.Latest != roundFloat(computer.ema[50][len(candles)-1], 4) {
		t.Fatalf("EMAMid=%+v", got.Data.EMAMid)
	}
	if got.Data.EMASlow == nil || got.Data.EMASlow.Latest != roundFloat(computer.ema[200][len(candles)-1], 4) {
		t.Fatalf("EMASlow=%+v", got.Data.EMASlow)
	}
	if got.Data.RSI == nil || got.Data.RSI.Current != roundFloat(computer.rsi[len(candles)-1], 4) {
		t.Fatalf("RSI=%+v", got.Data.RSI)
	}
	if got.Data.ATR == nil || got.Data.ATR.Latest != roundFloat(computer.atr[len(candles)-1], 4) {
		t.Fatalf("ATR=%+v", got.Data.ATR)
	}
	if got.Data.OBV == nil || got.Data.OBV.Value != roundFloat(computer.obv[len(candles)-1], 4) {
		t.Fatalf("OBV=%+v", got.Data.OBV)
	}
	if got.Data.STC == nil || got.Data.STC.Current != roundFloat(computer.stc[len(candles)-1], 4) {
		t.Fatalf("STC=%+v", got.Data.STC)
	}
	if got.Data.BB == nil || got.Data.BB.Upper != roundFloat(computer.bbUpper[len(candles)-1], 4) || got.Data.BB.Middle != roundFloat(computer.bbMiddle[len(candles)-1], 4) || got.Data.BB.Lower != roundFloat(computer.bbLower[len(candles)-1], 4) {
		t.Fatalf("BB=%+v", got.Data.BB)
	}
	if got.Data.CHOP == nil || got.Data.CHOP.Value != roundFloat(computer.chop[len(candles)-1], 4) {
		t.Fatalf("CHOP=%+v", got.Data.CHOP)
	}
	if got.Data.StochRSI == nil || got.Data.StochRSI.Value != roundFloat(computer.stochRSI[len(candles)-1], 4) {
		t.Fatalf("StochRSI=%+v", got.Data.StochRSI)
	}
	if got.Data.Aroon == nil || got.Data.Aroon.Up != roundFloat(computer.aroonUp[len(candles)-1], 4) || got.Data.Aroon.Down != roundFloat(computer.aroonDown[len(candles)-1], 4) {
		t.Fatalf("Aroon=%+v", got.Data.Aroon)
	}
}

func TestBuildTrendCompressedInputWithComputerUsesInjectedSeries(t *testing.T) {
	candles := oscillatingTrendTestCandles(260)
	computer := newStubIndicatorComputer(len(candles))
	opts := DefaultTrendCompressOptions()
	opts.SkipSuperTrend = true

	got, err := BuildTrendCompressedInputWithComputer("BTCUSDT", "1h", candles, opts, computer)
	if err != nil {
		t.Fatalf("BuildTrendCompressedInputWithComputer() error = %v", err)
	}

	if got.GlobalContext.EMA20 == nil || *got.GlobalContext.EMA20 != roundFloat(computer.ema[20][len(candles)-1], 4) {
		t.Fatalf("EMA20=%v", got.GlobalContext.EMA20)
	}
	if got.GlobalContext.EMA50 == nil || *got.GlobalContext.EMA50 != roundFloat(computer.ema[50][len(candles)-1], 4) {
		t.Fatalf("EMA50=%v", got.GlobalContext.EMA50)
	}
	if got.GlobalContext.EMA200 == nil || *got.GlobalContext.EMA200 != roundFloat(computer.ema[200][len(candles)-1], 4) {
		t.Fatalf("EMA200=%v", got.GlobalContext.EMA200)
	}
	if len(got.RecentCandles) == 0 || got.RecentCandles[len(got.RecentCandles)-1].RSI == nil {
		t.Fatalf("recent candle RSI missing: %+v", got.RecentCandles)
	}
	if *got.RecentCandles[len(got.RecentCandles)-1].RSI != roundFloat(computer.rsi[len(candles)-1], 1) {
		t.Fatalf("recent candle RSI=%v want %v", *got.RecentCandles[len(got.RecentCandles)-1].RSI, roundFloat(computer.rsi[len(candles)-1], 1))
	}
	if len(got.StructurePoints) == 0 {
		t.Fatal("structure_points = 0, want RSI-bearing points from injected RSI series")
	}
	hasRSI := false
	for _, point := range got.StructurePoints {
		if point.RSI != nil {
			hasRSI = true
			break
		}
	}
	if !hasRSI {
		t.Fatalf("structure points missing RSI annotations: %+v", got.StructurePoints)
	}
	upper, ok := findStructureCandidate(got.StructureCandidates, "bollinger_upper")
	if !ok || upper.Price != roundFloat(computer.bbUpper[len(candles)-1], 4) {
		t.Fatalf("upper bollinger candidate=%+v", upper)
	}
	lower, ok := findStructureCandidate(got.StructureCandidates, "bollinger_lower")
	if !ok || lower.Price != roundFloat(computer.bbLower[len(candles)-1], 4) {
		t.Fatalf("lower bollinger candidate=%+v", lower)
	}
}

func TestDefaultIndicatorBuilderUsesConfiguredComputer(t *testing.T) {
	candles := oscillatingTrendTestCandles(260)
	computer := newStubIndicatorComputer(len(candles))
	builder := DefaultIndicatorBuilder{
		Options:  DefaultIndicatorCompressOptions(),
		Computer: computer,
	}

	got, err := builder.BuildIndicator(context.Background(), marketSnapshotForTest("BTCUSDT", "1h", candles), "BTCUSDT", "1h")
	if err != nil {
		t.Fatalf("BuildIndicator() error = %v", err)
	}

	var payload IndicatorCompressedInput
	if err := json.Unmarshal(got.RawJSON, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Data.EMAFast == nil || payload.Data.EMAFast.Latest != roundFloat(computer.ema[21][len(candles)-1], 4) {
		t.Fatalf("payload EMAFast=%+v", payload.Data.EMAFast)
	}
}

func TestIntervalTrendBuilderUsesConfiguredComputer(t *testing.T) {
	candles := oscillatingTrendTestCandles(260)
	computer := newStubIndicatorComputer(len(candles))
	builder := IntervalTrendBuilder{
		DefaultOptions: DefaultTrendCompressOptions(),
		Computer:       computer,
	}
	builder.DefaultOptions.SkipSuperTrend = true

	got, err := builder.BuildTrend(context.Background(), marketSnapshotForTest("BTCUSDT", "1h", candles), "BTCUSDT", "1h")
	if err != nil {
		t.Fatalf("BuildTrend() error = %v", err)
	}

	var payload TrendCompressedInput
	if err := json.Unmarshal(got.RawJSON, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.GlobalContext.EMA20 == nil || *payload.GlobalContext.EMA20 != roundFloat(computer.ema[20][len(candles)-1], 4) {
		t.Fatalf("payload EMA20=%v", payload.GlobalContext.EMA20)
	}
}

func marketSnapshotForTest(symbol, interval string, candles []snapshot.Candle) snapshot.MarketSnapshot {
	return snapshot.MarketSnapshot{
		Klines: map[string]map[string][]snapshot.Candle{
			symbol: {interval: candles},
		},
	}
}

func newStubIndicatorComputer(n int) stubIndicatorComputer {
	return stubIndicatorComputer{
		ema: map[int][]float64{
			20:  testSeries(n, 120.1, 0.01),
			21:  testSeries(n, 121.1, 0.01),
			50:  testSeries(n, 150.1, 0.02),
			200: testSeries(n, 200.1, 0.03),
		},
		rsi:       testSeries(n, 40.0, 0.08),
		atr:       testSeries(n, 1.0, 0.005),
		bbUpper:   testConstantSeries(n, 105.0),
		bbMiddle:  testConstantSeries(n, 100.0),
		bbLower:   testConstantSeries(n, 95.0),
		obv:       testSeries(n, 1000.0, 7.5),
		stc:       testSeries(n, 25.0, 0.1),
		chop:      testSeries(n, 55.0, 0.02),
		stochRSI:  testSeries(n, 0.3, 0.001),
		aroonUp:   testSeries(n, 60.0, 0.05),
		aroonDown: testSeries(n, 30.0, 0.03),
	}
}

func testSeries(n int, base, step float64) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = base + float64(i)*step
	}
	return out
}

func testConstantSeries(n int, value float64) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = value
	}
	return out
}

func cloneSeries(in []float64) []float64 {
	if in == nil {
		return nil
	}
	out := make([]float64, len(in))
	copy(out, in)
	return out
}

func findStructureCandidate(candidates []TrendStructureCandidate, source string) (TrendStructureCandidate, bool) {
	for _, candidate := range candidates {
		if candidate.Source == source {
			return candidate, true
		}
	}
	return TrendStructureCandidate{}, false
}
