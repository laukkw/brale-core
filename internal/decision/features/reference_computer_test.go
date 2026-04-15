package features

import (
	"math"
	"testing"
)

var _ IndicatorComputer = ReferenceComputer{}

func TestReferenceComputerMatchesPureGoCompatibilityHelpers(t *testing.T) {
	candles := oscillatingTrendTestCandles(260)
	computer := ReferenceComputer{}
	_, closes, highs, lows, volumes := buildIndicatorSeries(candles)

	t.Run("bbands", func(t *testing.T) {
		gotUpper, gotMiddle, gotLower, err := computer.ComputeBB(closes, 20, 2, 2)
		if err != nil {
			t.Fatalf("ComputeBB() error = %v", err)
		}
		wantUpper, wantMiddle, wantLower := computeBollingerBands(closes, 20, 2)
		if !equalFloatSeries(gotUpper, wantUpper) || !equalFloatSeries(gotMiddle, wantMiddle) || !equalFloatSeries(gotLower, wantLower) {
			t.Fatalf("BBands mismatch")
		}
	})

	t.Run("obv", func(t *testing.T) {
		got, err := computer.ComputeOBV(closes, volumes)
		if err != nil {
			t.Fatalf("ComputeOBV() error = %v", err)
		}
		want := computeOBVSeries(closes, volumes)
		if !equalFloatSeries(got, want) {
			t.Fatalf("OBV mismatch")
		}
	})

	t.Run("chop", func(t *testing.T) {
		got, err := computer.ComputeCHOP(highs, lows, closes, 14)
		if err != nil {
			t.Fatalf("ComputeCHOP() error = %v", err)
		}
		want := computeCHOP(highs, lows, closes, 14)
		if !equalFloatSeries(got, want) {
			t.Fatalf("CHOP mismatch")
		}
	})

	t.Run("aroon", func(t *testing.T) {
		gotUp, gotDown, err := computer.ComputeAroon(highs, lows, 25)
		if err != nil {
			t.Fatalf("ComputeAroon() error = %v", err)
		}
		wantUp, wantDown := computeAroon(highs, lows, 25)
		if !equalFloatSeries(gotUp, wantUp) || !equalFloatSeries(gotDown, wantDown) {
			t.Fatalf("Aroon mismatch")
		}
	})

	t.Run("stoch_rsi", func(t *testing.T) {
		rsiSeries := testSeries(len(closes), 40, 0.05)
		got, err := computer.ComputeStochRSI(rsiSeries, 14)
		if err != nil {
			t.Fatalf("ComputeStochRSI() error = %v", err)
		}
		want := computeStochRSI(rsiSeries, 14)
		if !equalFloatSeries(got, want) {
			t.Fatalf("StochRSI mismatch")
		}
	})
}

func TestReferenceComputerCoreIndicatorsProduceUsableSeries(t *testing.T) {
	candles := trendTestCandles(260)
	computer := ReferenceComputer{}
	_, closes, highs, lows, _ := buildIndicatorSeries(candles)

	ema, err := computer.ComputeEMA(closes, 21)
	if err != nil {
		t.Fatalf("ComputeEMA() error = %v", err)
	}
	if len(ema) != len(closes) {
		t.Fatalf("EMA len=%d want %d", len(ema), len(closes))
	}
	if math.IsNaN(ema[len(ema)-1]) || ema[len(ema)-1] <= 0 {
		t.Fatalf("EMA latest=%v", ema[len(ema)-1])
	}

	rsi, err := computer.ComputeRSI(closes, 14)
	if err != nil {
		t.Fatalf("ComputeRSI() error = %v", err)
	}
	if len(rsi) != len(closes) {
		t.Fatalf("RSI len=%d want %d", len(rsi), len(closes))
	}
	if math.IsNaN(rsi[len(rsi)-1]) || rsi[len(rsi)-1] < 0 || rsi[len(rsi)-1] > 100 {
		t.Fatalf("RSI latest=%v", rsi[len(rsi)-1])
	}

	atr, err := computer.ComputeATR(highs, lows, closes, 14)
	if err != nil {
		t.Fatalf("ComputeATR() error = %v", err)
	}
	if len(atr) != len(closes) {
		t.Fatalf("ATR len=%d want %d", len(atr), len(closes))
	}
	if math.IsNaN(atr[len(atr)-1]) || atr[len(atr)-1] <= 0 {
		t.Fatalf("ATR latest=%v", atr[len(atr)-1])
	}
}

func TestReferenceComputerSTCStaysBounded(t *testing.T) {
	candles := oscillatingTrendTestCandles(260)
	computer := ReferenceComputer{}
	_, closes, _, _, _ := buildIndicatorSeries(candles)

	stc, err := computer.ComputeSTC(closes, 23, 50, 10, 3)
	if err != nil {
		t.Fatalf("ComputeSTC() error = %v", err)
	}
	series := sanitizeSeries(stc)
	if len(series) == 0 {
		t.Fatal("STC series = 0")
	}
	for _, value := range series {
		if value < 0 || value > 100 {
			t.Fatalf("STC value=%v want [0,100]", value)
		}
	}
}
