package features

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"brale-core/internal/config"

	talib "github.com/markcheno/go-talib"
)

func TestTalibComputerMatchesGoTalib(t *testing.T) {
	candles := oscillatingTrendTestCandles(260)
	computer := TalibComputer{}
	_, closes, highs, lows, _ := buildIndicatorSeries(candles)

	t.Run("ema", func(t *testing.T) {
		got, err := computer.ComputeEMA(closes, 21)
		if err != nil {
			t.Fatalf("ComputeEMA() error = %v", err)
		}
		want := talib.Ema(closes, 21)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("EMA mismatch")
		}
	})

	t.Run("rsi", func(t *testing.T) {
		got, err := computer.ComputeRSI(closes, 14)
		if err != nil {
			t.Fatalf("ComputeRSI() error = %v", err)
		}
		want := talib.Rsi(closes, 14)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("RSI mismatch")
		}
	})

	t.Run("atr", func(t *testing.T) {
		got, err := computer.ComputeATR(highs, lows, closes, 14)
		if err != nil {
			t.Fatalf("ComputeATR() error = %v", err)
		}
		want := talib.Atr(highs, lows, closes, 14)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("ATR mismatch")
		}
	})

	t.Run("bbands", func(t *testing.T) {
		gotUpper, gotMiddle, gotLower, err := computer.ComputeBB(closes, 20, 2, 2)
		if err != nil {
			t.Fatalf("ComputeBB() error = %v", err)
		}
		wantUpper, wantMiddle, wantLower := talib.BBands(closes, 20, 2, 2, talib.SMA)
		if !reflect.DeepEqual(gotUpper, wantUpper) || !reflect.DeepEqual(gotMiddle, wantMiddle) || !reflect.DeepEqual(gotLower, wantLower) {
			t.Fatalf("BBands mismatch")
		}
	})

	t.Run("aroon", func(t *testing.T) {
		gotUp, gotDown, err := computer.ComputeAroon(highs, lows, 25)
		if err != nil {
			t.Fatalf("ComputeAroon() error = %v", err)
		}
		wantUp, wantDown := talib.Aroon(highs, lows, 25)
		if !reflect.DeepEqual(gotUp, wantUp) || !reflect.DeepEqual(gotDown, wantDown) {
			t.Fatalf("Aroon mismatch")
		}
	})
}

func TestTalibComputerMatchesCurrentCompatibilityMath(t *testing.T) {
	candles := oscillatingTrendTestCandles(260)
	computer := TalibComputer{}
	_, closes, highs, lows, volumes := buildIndicatorSeries(candles)

	t.Run("stc", func(t *testing.T) {
		got, err := computer.ComputeSTC(closes, 23, 50, config.DefaultSTCKPeriod, config.DefaultSTCDPeriod)
		if err != nil {
			t.Fatalf("ComputeSTC() error = %v", err)
		}
		want := computeSTCSeries(closes, 23, 50, config.DefaultSTCKPeriod, config.DefaultSTCDPeriod)
		if !equalFloatSeries(got, want) {
			t.Fatalf("STC mismatch")
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

	t.Run("stoch_rsi", func(t *testing.T) {
		rsi := talib.Rsi(closes, 14)
		got, err := computer.ComputeStochRSI(rsi, 14)
		if err != nil {
			t.Fatalf("ComputeStochRSI() error = %v", err)
		}
		want := computeStochRSI(rsi, 14)
		if !equalFloatSeries(got, want) {
			t.Fatalf("StochRSI mismatch")
		}
	})

	t.Run("obv", func(t *testing.T) {
		got, err := computer.ComputeOBV(closes, volumes)
		if err != nil {
			t.Fatalf("ComputeOBV() error = %v", err)
		}
		want := computeOBVSeries(closes, volumes)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("OBV mismatch")
		}
	})
}

func TestTalibComputerRejectsInvalidInputs(t *testing.T) {
	computer := TalibComputer{}

	if _, err := computer.ComputeEMA(nil, 21); err == nil || !strings.Contains(err.Error(), "closes") {
		t.Fatalf("ComputeEMA() err = %v", err)
	}
	if _, err := computer.ComputeATR([]float64{1, 2}, []float64{1}, []float64{1, 2}, 14); err == nil || !strings.Contains(err.Error(), "same length") {
		t.Fatalf("ComputeATR() err = %v", err)
	}
	if _, _, err := computer.ComputeAroon([]float64{1, 2}, []float64{1}, 14); err == nil || !strings.Contains(err.Error(), "same length") {
		t.Fatalf("ComputeAroon() err = %v", err)
	}
}

func equalFloatSeries(left, right []float64) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if math.IsNaN(left[i]) && math.IsNaN(right[i]) {
			continue
		}
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
