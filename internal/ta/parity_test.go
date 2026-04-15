package ta_test

import (
	"math"
	"testing"

	"brale-core/internal/ta"

	talib "github.com/markcheno/go-talib"
)

const parityEpsilon = 1e-10

// compareSeries checks that two float64 slices are element-wise equal within ε,
// accounting for NaN/zero warmup padding differences between ta and go-talib.
// go-talib pads warmup positions with 0.0; our ta package uses NaN.
func compareSeries(t *testing.T, name string, got, want []float64, eps float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: len=%d want %d", name, len(got), len(want))
	}
	for i := range got {
		gNaN := math.IsNaN(got[i])
		wZero := want[i] == 0
		// Both in warmup zone: ta=NaN, talib=0 — treat as equal
		if gNaN && wZero {
			continue
		}
		wNaN := math.IsNaN(want[i])
		if gNaN && wNaN {
			continue
		}
		if gNaN != wNaN {
			t.Fatalf("%s[%d]: got NaN=%v want NaN=%v (got=%f want=%f)", name, i, gNaN, wNaN, got[i], want[i])
		}
		if math.Abs(got[i]-want[i]) > eps {
			t.Fatalf("%s[%d]: got=%f want=%f diff=%e", name, i, got[i], want[i], math.Abs(got[i]-want[i]))
		}
	}
}

// --- Parity: EMA ---

func TestParity_EMA(t *testing.T) {
	got, err := ta.EMA(testCloses, 10)
	if err != nil {
		t.Fatal(err)
	}
	want := talib.Ema(testCloses, 10)
	compareSeries(t, "EMA(10)", got, want, parityEpsilon)
}

// --- Parity: RSI ---

func TestParity_RSI(t *testing.T) {
	got, err := ta.RSI(testCloses, 14)
	if err != nil {
		t.Fatal(err)
	}
	want := talib.Rsi(testCloses, 14)
	compareSeries(t, "RSI(14)", got, want, parityEpsilon)
}

// --- Parity: ATR ---

func TestParity_ATR(t *testing.T) {
	got, err := ta.ATR(testHighs, testLows, testCloses, 14)
	if err != nil {
		t.Fatal(err)
	}
	want := talib.Atr(testHighs, testLows, testCloses, 14)
	compareSeries(t, "ATR(14)", got, want, parityEpsilon)
}

// --- Parity: Bollinger Bands ---

func TestParity_BollingerBands(t *testing.T) {
	gotU, gotM, gotL, err := ta.BollingerBands(testCloses, 20, 2.0, 2.0)
	if err != nil {
		t.Fatal(err)
	}
	wantU, wantM, wantL := talib.BBands(testCloses, 20, 2.0, 2.0, talib.SMA)
	compareSeries(t, "BB_Upper", gotU, wantU, parityEpsilon)
	compareSeries(t, "BB_Middle", gotM, wantM, parityEpsilon)
	compareSeries(t, "BB_Lower", gotL, wantL, parityEpsilon)
}

// --- Parity: Aroon ---

func TestParity_Aroon(t *testing.T) {
	gotUp, gotDown, err := ta.Aroon(testHighs, testLows, 14)
	if err != nil {
		t.Fatal(err)
	}
	wantDown, wantUp := talib.Aroon(testHighs, testLows, 14)
	compareSeries(t, "Aroon_Up", gotUp, wantUp, parityEpsilon)
	compareSeries(t, "Aroon_Down", gotDown, wantDown, parityEpsilon)
}
