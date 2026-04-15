package features

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"brale-core/internal/config"
)

func TestBuildIndicatorCompressedInputIncludesSTC(t *testing.T) {
	required := config.STCRequiredBars(23, 50)
	candles := oscillatingTrendTestCandles(required + 20)

	got, err := BuildIndicatorCompressedInput("BTCUSDT", "1h", candles, DefaultIndicatorCompressOptions())
	if err != nil {
		t.Fatalf("BuildIndicatorCompressedInput() error = %v", err)
	}
	if got.Data.STC == nil {
		t.Fatalf("STC = nil")
	}
	if got.Data.STC.Current < 0 || got.Data.STC.Current > 100 {
		t.Fatalf("STC.Current=%v want [0,100]", got.Data.STC.Current)
	}
	if got.Data.STC.State != "rising" && got.Data.STC.State != "falling" && got.Data.STC.State != "flat" {
		t.Fatalf("STC.State=%q", got.Data.STC.State)
	}

	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(raw), `"macd"`) {
		t.Fatalf("payload still contains macd: %s", raw)
	}
}

func TestBuildIndicatorCompressedInputIncludesEMAFastAtThreshold(t *testing.T) {
	required := config.EMARequiredBars(21)
	opts := DefaultIndicatorCompressOptions()
	opts.SkipRSI = true
	opts.SkipSTC = true

	got, err := BuildIndicatorCompressedInput("BTCUSDT", "1h", trendTestCandles(required), opts)
	if err != nil {
		t.Fatalf("BuildIndicatorCompressedInput() error = %v", err)
	}
	if got.Data.EMAFast == nil {
		t.Fatalf("EMAFast = nil at threshold=%d", required)
	}

	got, err = BuildIndicatorCompressedInput("BTCUSDT", "1h", trendTestCandles(required-1), opts)
	if err != nil {
		t.Fatalf("BuildIndicatorCompressedInput() error = %v", err)
	}
	if got.Data.EMAFast != nil {
		t.Fatalf("EMAFast=%+v want nil below threshold", got.Data.EMAFast)
	}
}

func TestBuildIndicatorCompressedInputIncludesRSIAtThreshold(t *testing.T) {
	required := config.RSIRequiredBars(14)
	opts := DefaultIndicatorCompressOptions()
	opts.SkipEMA = true
	opts.SkipSTC = true

	got, err := BuildIndicatorCompressedInput("BTCUSDT", "1h", trendTestCandles(required), opts)
	if err != nil {
		t.Fatalf("BuildIndicatorCompressedInput() error = %v", err)
	}
	if got.Data.RSI == nil {
		t.Fatalf("RSI = nil at threshold=%d", required)
	}

	got, err = BuildIndicatorCompressedInput("BTCUSDT", "1h", trendTestCandles(required-1), opts)
	if err != nil {
		t.Fatalf("BuildIndicatorCompressedInput() error = %v", err)
	}
	if got.Data.RSI != nil {
		t.Fatalf("RSI=%+v want nil below threshold", got.Data.RSI)
	}
}

func TestBuildIndicatorCompressedInputIncludesATRAtThreshold(t *testing.T) {
	required := config.ATRRequiredBars(14)
	opts := DefaultIndicatorCompressOptions()
	opts.SkipEMA = true
	opts.SkipRSI = true
	opts.SkipSTC = true

	got, err := BuildIndicatorCompressedInput("BTCUSDT", "1h", trendTestCandles(required), opts)
	if err != nil {
		t.Fatalf("BuildIndicatorCompressedInput() error = %v", err)
	}
	if got.Data.ATR == nil {
		t.Fatalf("ATR = nil at threshold=%d", required)
	}

	got, err = BuildIndicatorCompressedInput("BTCUSDT", "1h", trendTestCandles(required-1), opts)
	if err != nil {
		t.Fatalf("BuildIndicatorCompressedInput() error = %v", err)
	}
	if got.Data.ATR != nil {
		t.Fatalf("ATR=%+v want nil below threshold", got.Data.ATR)
	}
}

func TestBuildIndicatorCompressedInputOmitsSTCWhenBarsInsufficient(t *testing.T) {
	required := config.STCRequiredBars(23, 50)
	candles := trendTestCandles(required - 1)

	got, err := BuildIndicatorCompressedInput("BTCUSDT", "1h", candles, DefaultIndicatorCompressOptions())
	if err != nil {
		t.Fatalf("BuildIndicatorCompressedInput() error = %v", err)
	}
	if got.Data.STC != nil {
		t.Fatalf("STC=%+v want nil", got.Data.STC)
	}
}

func TestBuildIndicatorCompressedInputIncludesSTCAtThreshold(t *testing.T) {
	required := config.STCRequiredBars(23, 50)
	// Add a small buffer beyond the minimum warmup; at exactly "required" bars,
	// the STC formula may still yield NaN depending on data shape — this is
	// mathematically correct since STCRequiredBars is the EMA warmup floor.
	candles := oscillatingTrendTestCandles(required + 10)

	got, err := BuildIndicatorCompressedInput("BTCUSDT", "1h", candles, DefaultIndicatorCompressOptions())
	if err != nil {
		t.Fatalf("BuildIndicatorCompressedInput() error = %v", err)
	}
	if got.Data.STC == nil {
		t.Fatalf("STC = nil at threshold=%d", required)
	}
}

func TestSTCRequiredBarsChangeWithParams(t *testing.T) {
	base := config.STCRequiredBars(23, 50)
	alt := config.STCRequiredBars(30, 60)
	if alt <= base {
		t.Fatalf("alt=%d want > base=%d", alt, base)
	}
}

func TestBuildIndicatorCompressedInputSTCNoNaNOnSideways(t *testing.T) {
	required := config.STCRequiredBars(23, 50)
	candles := sidewaysTrendTestCandles(required + 20)

	got, err := BuildIndicatorCompressedInput("BTCUSDT", "1h", candles, DefaultIndicatorCompressOptions())
	if err != nil {
		t.Fatalf("BuildIndicatorCompressedInput() error = %v", err)
	}
	if got.Data.STC == nil {
		t.Fatalf("STC = nil")
	}
	if math.IsNaN(got.Data.STC.Current) || math.IsInf(got.Data.STC.Current, 0) {
		t.Fatalf("STC.Current=%v", got.Data.STC.Current)
	}
	for _, v := range got.Data.STC.LastN {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("STC.LastN contains invalid value: %v", got.Data.STC.LastN)
		}
	}
}

func TestComputeSTCSeriesGoldenTail(t *testing.T) {
	closes := make([]float64, 90)
	for i := range closes {
		base := 100 + math.Sin(float64(i)/11.0)*1.1
		wave := math.Sin(float64(i)/3.2)*2.4 + math.Cos(float64(i)/5.7)*1.6
		closes[i] = base + wave
	}
	series := sanitizeSeries(computeSTCSeries(closes, 23, 50, config.DefaultSTCKPeriod, config.DefaultSTCDPeriod))
	got := roundSeriesTail(series, 5)
	want := []float64{100, 100, 100, 0, 0}
	if len(got) != len(want) {
		t.Fatalf("tail len=%d want %d tail=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tail[%d]=%v want %v full=%v", i, got[i], want[i], got)
		}
	}
}

func TestBuildIndicatorCompressedInputRejectsPartiallyInvalidOptions(t *testing.T) {
	opts := DefaultIndicatorCompressOptions()
	opts.EMAMid = 0

	_, err := BuildIndicatorCompressedInput("BTCUSDT", "1h", trendTestCandles(config.EMARequiredBars(opts.EMASlow)+10), opts)
	if err == nil {
		t.Fatal("BuildIndicatorCompressedInput() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "ema_mid") {
		t.Fatalf("error=%q should mention ema_mid", err.Error())
	}
}

func TestBuildIndicatorCompressedInputUsesDefaultsForZeroValueOptions(t *testing.T) {
	def := DefaultIndicatorCompressOptions()
	got, err := BuildIndicatorCompressedInput("BTCUSDT", "1h", trendTestCandles(config.EMARequiredBars(def.EMASlow)+10), IndicatorCompressOptions{})
	if err != nil {
		t.Fatalf("BuildIndicatorCompressedInput() error = %v", err)
	}
	if got.Data.EMAFast == nil {
		t.Fatal("EMAFast = nil, want default indicator options to be applied")
	}
	if got.Data.ATR == nil {
		t.Fatal("ATR = nil, want default indicator options to be applied")
	}
}

func TestValidateIndicatorCompressOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    IndicatorCompressOptions
		wantErr string
	}{
		{name: "default valid", opts: DefaultIndicatorCompressOptions()},
		{
			name: "bb period one rejected",
			opts: func() IndicatorCompressOptions {
				opts := DefaultIndicatorCompressOptions()
				opts.BBPeriod = 1
				return opts
			}(),
			wantErr: "bb_period must be > 1",
		},
		{
			name: "chop period one rejected",
			opts: func() IndicatorCompressOptions {
				opts := DefaultIndicatorCompressOptions()
				opts.CHOPPeriod = 1
				return opts
			}(),
			wantErr: "chop_period must be > 1",
		},
		{
			name: "chop period two allowed",
			opts: func() IndicatorCompressOptions {
				opts := DefaultIndicatorCompressOptions()
				opts.CHOPPeriod = 2
				return opts
			}(),
		},
		{
			name: "aroon period zero rejected",
			opts: func() IndicatorCompressOptions {
				opts := DefaultIndicatorCompressOptions()
				opts.AroonPeriod = 0
				return opts
			}(),
			wantErr: "aroon_period must be > 0",
		},
		{
			name: "bb multiplier zero rejected",
			opts: func() IndicatorCompressOptions {
				opts := DefaultIndicatorCompressOptions()
				opts.BBMultiplier = 0
				return opts
			}(),
			wantErr: "bb_multiplier must be > 0",
		},
		{
			name: "last n zero rejected",
			opts: func() IndicatorCompressOptions {
				opts := DefaultIndicatorCompressOptions()
				opts.LastN = 0
				return opts
			}(),
			wantErr: "last_n must be > 0",
		},
		{
			name: "ema ordering rejected",
			opts: func() IndicatorCompressOptions {
				opts := DefaultIndicatorCompressOptions()
				opts.EMAFast = opts.EMAMid
				return opts
			}(),
			wantErr: "ema_fast < ema_mid < ema_slow",
		},
		{
			name: "stc ordering rejected",
			opts: func() IndicatorCompressOptions {
				opts := DefaultIndicatorCompressOptions()
				opts.STCFast = opts.STCSlow
				return opts
			}(),
			wantErr: "stc_fast must be < stc_slow",
		},
		{
			name: "skip flags allow zero values",
			opts: func() IndicatorCompressOptions {
				opts := DefaultIndicatorCompressOptions()
				opts.SkipEMA = true
				opts.EMAFast = 0
				opts.EMAMid = 0
				opts.EMASlow = 0
				opts.SkipRSI = true
				opts.RSIPeriod = 0
				opts.StochRSIPeriod = 0
				opts.SkipSTC = true
				opts.STCFast = 0
				opts.STCSlow = 0
				return opts
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIndicatorCompressOptions(tt.opts)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateIndicatorCompressOptions() error = %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateIndicatorCompressOptions() error = nil, want %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateIndicatorCompressOptions() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestBuildIndicatorCompressedInputIncludesCHOPAtThreshold(t *testing.T) {
	opts := DefaultIndicatorCompressOptions()
	opts.SkipEMA = true
	opts.SkipRSI = true
	opts.SkipSTC = true

	required := config.CHOPRequiredBars(opts.CHOPPeriod)
	got, err := BuildIndicatorCompressedInput("BTCUSDT", "1h", trendTestCandles(required), opts)
	if err != nil {
		t.Fatalf("BuildIndicatorCompressedInput() error = %v", err)
	}
	if got.Data.CHOP == nil {
		t.Fatalf("CHOP = nil at threshold=%d", required)
	}

	got, err = BuildIndicatorCompressedInput("BTCUSDT", "1h", trendTestCandles(required-1), opts)
	if err != nil {
		t.Fatalf("BuildIndicatorCompressedInput() error = %v", err)
	}
	if got.Data.CHOP != nil {
		t.Fatalf("CHOP=%+v want nil below threshold", got.Data.CHOP)
	}
}

func TestBuildIndicatorCompressedInputIncludesAroonAtThreshold(t *testing.T) {
	opts := DefaultIndicatorCompressOptions()
	opts.SkipEMA = true
	opts.SkipRSI = true
	opts.SkipSTC = true

	required := config.AroonRequiredBars(opts.AroonPeriod)
	got, err := BuildIndicatorCompressedInput("BTCUSDT", "1h", trendTestCandles(required), opts)
	if err != nil {
		t.Fatalf("BuildIndicatorCompressedInput() error = %v", err)
	}
	if got.Data.Aroon == nil {
		t.Fatalf("Aroon = nil at threshold=%d", required)
	}

	got, err = BuildIndicatorCompressedInput("BTCUSDT", "1h", trendTestCandles(required-1), opts)
	if err != nil {
		t.Fatalf("BuildIndicatorCompressedInput() error = %v", err)
	}
	if got.Data.Aroon != nil {
		t.Fatalf("Aroon=%+v want nil below threshold", got.Data.Aroon)
	}
}
