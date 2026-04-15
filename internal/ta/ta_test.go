package ta_test

import (
	"math"
	"testing"

	"brale-core/internal/ta"
)

// --- Test data ---

var testCloses = []float64{
	44.34, 44.09, 44.15, 43.61, 44.33, 44.83, 45.10, 45.42, 45.84, 46.08,
	45.89, 46.03, 45.61, 46.28, 46.28, 46.00, 46.03, 46.41, 46.22, 45.64,
	46.21, 46.25, 45.71, 46.45, 45.78, 45.35, 44.03, 44.18, 44.22, 44.57,
}

var testHighs = []float64{
	44.34, 44.15, 44.33, 44.10, 44.83, 45.10, 45.42, 45.84, 46.08, 46.20,
	46.03, 46.28, 46.28, 46.28, 46.41, 46.22, 46.21, 46.45, 46.45, 46.25,
	46.45, 46.25, 46.21, 46.45, 45.78, 45.35, 44.57, 44.57, 44.57, 44.57,
}

var testLows = []float64{
	44.09, 43.61, 44.09, 43.61, 44.15, 44.83, 45.10, 45.10, 45.42, 45.84,
	45.61, 45.89, 45.61, 46.00, 46.03, 45.64, 46.03, 46.03, 45.71, 45.35,
	46.21, 45.71, 45.35, 45.78, 44.03, 44.18, 44.03, 44.03, 44.18, 44.22,
}

var testVolumes = []float64{
	100, 200, 150, 300, 250, 100, 200, 150, 300, 250,
	100, 200, 150, 300, 250, 100, 200, 150, 300, 250,
	100, 200, 150, 300, 250, 100, 200, 150, 300, 250,
}

// --- Validation Tests ---

func TestValidation_EmptySeries(t *testing.T) {
	tests := []struct {
		name string
		fn   func() error
	}{
		{"EMA empty", func() error { _, e := ta.EMA(nil, 14); return e }},
		{"RSI empty", func() error { _, e := ta.RSI(nil, 14); return e }},
		{"ATR empty", func() error { _, e := ta.ATR(nil, nil, nil, 14); return e }},
		{"BB empty", func() error { _, _, _, e := ta.BollingerBands(nil, 20, 2, 2); return e }},
		{"OBV empty", func() error { _, e := ta.OBV(nil, nil); return e }},
		{"STC empty", func() error { _, e := ta.STC(nil, 23, 50, 10, 10); return e }},
		{"CHOP empty", func() error { _, e := ta.CHOP(nil, nil, nil, 14); return e }},
		{"Aroon empty", func() error { _, _, e := ta.Aroon(nil, nil, 25); return e }},
		{"StochRSI empty", func() error { _, e := ta.StochRSI(nil, 14); return e }},
		{"SuperTrend empty", func() error { _, e := ta.SuperTrend(nil, nil, nil, 10, 3); return e }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err == nil {
				t.Fatal("expected error for empty series")
			}
		})
	}
}

func TestValidation_ZeroPeriod(t *testing.T) {
	c := testCloses[:10]
	tests := []struct {
		name string
		fn   func() error
	}{
		{"EMA", func() error { _, e := ta.EMA(c, 0); return e }},
		{"RSI", func() error { _, e := ta.RSI(c, 0); return e }},
		{"ATR", func() error { _, e := ta.ATR(c, c, c, 0); return e }},
		{"BB", func() error { _, _, _, e := ta.BollingerBands(c, 0, 2, 2); return e }},
		{"STC", func() error { _, e := ta.STC(c, 0, 50, 10, 10); return e }},
		{"CHOP", func() error { _, e := ta.CHOP(c, c, c, 0); return e }},
		{"Aroon", func() error { _, _, e := ta.Aroon(c, c, 0); return e }},
		{"StochRSI", func() error { _, e := ta.StochRSI(c, 0); return e }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err == nil {
				t.Fatal("expected error for zero period")
			}
		})
	}
}

// --- EMA Tests ---

func TestEMA_Basic(t *testing.T) {
	result, err := ta.EMA(testCloses, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != len(testCloses) {
		t.Fatalf("len=%d want %d", len(result), len(testCloses))
	}
	for i := 0; i < 9; i++ {
		if !math.IsNaN(result[i]) {
			t.Fatalf("result[%d]=%f want NaN", i, result[i])
		}
	}
	if math.IsNaN(result[9]) {
		t.Fatal("result[9] should not be NaN")
	}
	// EMA(10) seed is SMA of first 10 values
	expectedSeed := 0.0
	for i := 0; i < 10; i++ {
		expectedSeed += testCloses[i]
	}
	expectedSeed /= 10
	if math.Abs(result[9]-expectedSeed) > 1e-10 {
		t.Fatalf("seed=%f want %f", result[9], expectedSeed)
	}
}

func TestEMA_SingleValue(t *testing.T) {
	result, err := ta.EMA([]float64{42.0}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result[0] != 42.0 {
		t.Fatalf("got %f want 42.0", result[0])
	}
}

// --- RSI Tests ---

func TestRSI_Basic(t *testing.T) {
	result, err := ta.RSI(testCloses, 14)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != len(testCloses) {
		t.Fatalf("len=%d want %d", len(result), len(testCloses))
	}
	for i := 0; i <= 13; i++ {
		if !math.IsNaN(result[i]) {
			t.Fatalf("result[%d]=%f want NaN", i, result[i])
		}
	}
	rsi14 := result[14]
	if rsi14 < 0 || rsi14 > 100 {
		t.Fatalf("RSI out of range: %f", rsi14)
	}
}

func TestRSI_AllUp(t *testing.T) {
	closes := make([]float64, 20)
	for i := range closes {
		closes[i] = float64(i + 1)
	}
	result, err := ta.RSI(closes, 14)
	if err != nil {
		t.Fatal(err)
	}
	// All gains, no losses → RSI should be 100
	for i := 14; i < len(result); i++ {
		if result[i] != 100 {
			t.Fatalf("RSI[%d]=%f want 100", i, result[i])
		}
	}
}

func TestRSI_Flat(t *testing.T) {
	closes := make([]float64, 20)
	for i := range closes {
		closes[i] = 50.0
	}
	result, err := ta.RSI(closes, 14)
	if err != nil {
		t.Fatal(err)
	}
	for i := 14; i < len(result); i++ {
		if result[i] != 50 {
			t.Fatalf("RSI[%d]=%f want 50", i, result[i])
		}
	}
}

// --- ATR Tests ---

func TestATR_Basic(t *testing.T) {
	result, err := ta.ATR(testHighs, testLows, testCloses, 14)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != len(testCloses) {
		t.Fatalf("len=%d want %d", len(result), len(testCloses))
	}
	for i := 0; i <= 13; i++ {
		if !math.IsNaN(result[i]) {
			t.Fatalf("result[%d]=%f want NaN", i, result[i])
		}
	}
	atr14 := result[14]
	if atr14 <= 0 {
		t.Fatalf("ATR should be positive, got %f", atr14)
	}
}

// --- Bollinger Bands Tests ---

func TestBB_Basic(t *testing.T) {
	upper, middle, lower, err := ta.BollingerBands(testCloses, 20, 2.0, 2.0)
	if err != nil {
		t.Fatal(err)
	}
	if len(upper) != len(testCloses) {
		t.Fatal("length mismatch")
	}
	for i := 19; i < len(testCloses); i++ {
		if math.IsNaN(upper[i]) || math.IsNaN(middle[i]) || math.IsNaN(lower[i]) {
			t.Fatalf("NaN at index %d", i)
		}
		if upper[i] < middle[i] || middle[i] < lower[i] {
			t.Fatalf("band ordering violated at %d: U=%f M=%f L=%f", i, upper[i], middle[i], lower[i])
		}
	}
}

func TestBB_AsymmetricMultipliers(t *testing.T) {
	upper, middle, lower, err := ta.BollingerBands(testCloses, 20, 2.5, 1.5)
	if err != nil {
		t.Fatal(err)
	}
	for i := 19; i < len(testCloses); i++ {
		if math.IsNaN(upper[i]) {
			continue
		}
		upperDist := upper[i] - middle[i]
		lowerDist := middle[i] - lower[i]
		if upperDist <= lowerDist {
			t.Fatalf("asymmetric multipliers should produce wider upper band at %d", i)
		}
	}
}

func TestBB_InvalidMultiplier(t *testing.T) {
	_, _, _, err := ta.BollingerBands(testCloses, 20, 0, 2)
	if err == nil {
		t.Fatal("expected error for zero multiplier")
	}
}

// --- OBV Tests ---

func TestOBV_Basic(t *testing.T) {
	result, err := ta.OBV(testCloses, testVolumes)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != len(testCloses) {
		t.Fatal("length mismatch")
	}
	if result[0] != 0 {
		t.Fatalf("OBV[0]=%f want 0", result[0])
	}
}

func TestOBV_LengthMismatch(t *testing.T) {
	_, err := ta.OBV(testCloses[:5], testVolumes[:3])
	if err == nil {
		t.Fatal("expected error for mismatched lengths")
	}
}

// --- STC Tests ---

func TestSTC_Basic(t *testing.T) {
	// STC needs enough data: slow=26 EMA + k=10 stochastic + d=10 SMA ≈ 46+ bars
	closes := make([]float64, 60)
	for i := range closes {
		closes[i] = 44.0 + float64(i%10)*0.3 + float64(i)*0.05
	}
	result, err := ta.STC(closes, 12, 26, 10, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != len(closes) {
		t.Fatalf("len=%d want %d", len(result), len(closes))
	}
	hasValue := false
	for _, v := range result {
		if !math.IsNaN(v) {
			hasValue = true
			if v < 0 || v > 100 {
				t.Fatalf("STC out of [0,100]: %f", v)
			}
		}
	}
	if !hasValue {
		t.Fatal("all STC values are NaN")
	}
}

// --- CHOP Tests ---

func TestCHOP_Basic(t *testing.T) {
	result, err := ta.CHOP(testHighs, testLows, testCloses, 14)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != len(testCloses) {
		t.Fatal("length mismatch")
	}
	for i := 14; i < len(result); i++ {
		if math.IsNaN(result[i]) {
			continue
		}
		if result[i] < 0 || result[i] > 100 {
			t.Fatalf("CHOP[%d]=%f out of [0,100]", i, result[i])
		}
	}
}

// --- Aroon Tests ---

func TestAroon_Basic(t *testing.T) {
	up, down, err := ta.Aroon(testHighs, testLows, 14)
	if err != nil {
		t.Fatal(err)
	}
	if len(up) != len(testHighs) || len(down) != len(testHighs) {
		t.Fatal("length mismatch")
	}
	for i := 14; i < len(up); i++ {
		if math.IsNaN(up[i]) || math.IsNaN(down[i]) {
			continue
		}
		if up[i] < 0 || up[i] > 100 || down[i] < 0 || down[i] > 100 {
			t.Fatalf("Aroon[%d] out of range: up=%f down=%f", i, up[i], down[i])
		}
	}
}

// --- StochRSI Tests ---

func TestStochRSI_Basic(t *testing.T) {
	rsi, err := ta.RSI(testCloses, 14)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ta.StochRSI(rsi, 14)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != len(testCloses) {
		t.Fatal("length mismatch")
	}
	for _, v := range result {
		if math.IsNaN(v) {
			continue
		}
		if v < 0 || v > 1.001 {
			t.Fatalf("StochRSI=%f out of [0,1]", v)
		}
	}
}

// --- TD Sequential Tests ---

func TestTDSequential_Basic(t *testing.T) {
	buy, sell := ta.TDSequential(testCloses)
	if len(buy) != len(testCloses) || len(sell) != len(testCloses) {
		t.Fatal("length mismatch")
	}
	for i := 0; i < 4; i++ {
		if buy[i] != 0 || sell[i] != 0 {
			t.Fatalf("first 4 values should be 0")
		}
	}
}

func TestTDSequential_Short(t *testing.T) {
	buy, sell := ta.TDSequential([]float64{1, 2, 3})
	if len(buy) != 3 || len(sell) != 3 {
		t.Fatal("should return zero slices for short input")
	}
}

// --- SuperTrend Tests ---

func TestSuperTrend_Basic(t *testing.T) {
	result, err := ta.SuperTrend(testHighs, testLows, testCloses, 10, 3.0)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	for _, v := range result {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("SuperTrend contains NaN/Inf: %f", v)
		}
	}
}

func TestSuperTrend_InvalidMultiplier(t *testing.T) {
	_, err := ta.SuperTrend(testHighs, testLows, testCloses, 10, 0)
	if err == nil {
		t.Fatal("expected error for zero multiplier")
	}
}

// --- Helpers Tests ---

func TestSMA_Basic(t *testing.T) {
	result := ta.SMA(testCloses, 5)
	if len(result) != len(testCloses) {
		t.Fatal("length mismatch")
	}
	for i := 0; i < 4; i++ {
		if !math.IsNaN(result[i]) {
			t.Fatalf("SMA[%d] should be NaN", i)
		}
	}
	expected := (testCloses[0] + testCloses[1] + testCloses[2] + testCloses[3] + testCloses[4]) / 5
	if math.Abs(result[4]-expected) > 1e-10 {
		t.Fatalf("SMA[4]=%f want %f", result[4], expected)
	}
}

func TestTrueRange(t *testing.T) {
	tr := ta.TrueRange(testHighs, testLows, testCloses)
	if len(tr) != len(testCloses) {
		t.Fatal("length mismatch")
	}
	// TR[0] = High[0] - Low[0]
	expected := testHighs[0] - testLows[0]
	if math.Abs(tr[0]-expected) > 1e-10 {
		t.Fatalf("TR[0]=%f want %f", tr[0], expected)
	}
	for _, v := range tr {
		if v < 0 {
			t.Fatalf("TrueRange should be non-negative, got %f", v)
		}
	}
}

func TestWMA(t *testing.T) {
	result := ta.WMA(testCloses, 5)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result) != len(testCloses)-4 {
		t.Fatalf("len=%d want %d", len(result), len(testCloses)-4)
	}
}

func TestHMA(t *testing.T) {
	result := ta.HMA(testCloses, 10)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result) == 0 {
		t.Fatal("expected non-empty result")
	}
}
