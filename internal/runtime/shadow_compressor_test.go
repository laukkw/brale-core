package runtime

import (
	"context"
	"testing"
	"time"

	"brale-core/internal/decision"
	"brale-core/internal/decision/features"
	"brale-core/internal/snapshot"
)

func TestShadowComparingCompressorReportsDiffWithoutChangingPrimaryOutput(t *testing.T) {
	primary := &decision.FeatureCompressor{
		Indicators: decision.DefaultIndicatorBuilder{
			Options:  features.DefaultIndicatorCompressOptions(),
			Computer: decision.TalibComputer{},
		},
		Trends: features.DefaultTrendBuilder{
			Options:  diffTrendOptionsForShadow(),
			Computer: decision.TalibComputer{},
		},
		Mechanics: noopMechanicsBuilder{},
	}

	var reports []features.EngineDiffReport
	compressor := &shadowComparingCompressor{
		primary:            primary,
		primaryName:        "talib",
		shadowName:         "reference",
		primaryComputer:    decision.TalibComputer{},
		shadowComputer:     decision.ReferenceComputer{},
		indicatorOptions:   features.DefaultIndicatorCompressOptions(),
		trendOptionsByInt:  map[string]decision.TrendCompressOptions{"1h": diffTrendOptionsForShadow()},
		defaultTrendOption: diffTrendOptionsForShadow(),
		report: func(_ context.Context, report features.EngineDiffReport) {
			reports = append(reports, report)
		},
	}

	snap := snapshot.MarketSnapshot{
		Klines: map[string]map[string][]snapshot.Candle{
			"BTCUSDT": {"1h": shadowTestCandles(260)},
		},
	}

	out, errs, err := compressor.Compress(context.Background(), snap)
	if err != nil {
		t.Fatalf("Compress() error = %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("errs=%v want none", errs)
	}
	if len(out.Indicators["BTCUSDT"]) != 1 {
		t.Fatalf("indicator intervals=%d want 1", len(out.Indicators["BTCUSDT"]))
	}
	if len(out.Trends["BTCUSDT"]) != 1 {
		t.Fatalf("trend intervals=%d want 1", len(out.Trends["BTCUSDT"]))
	}
	if len(reports) != 1 {
		t.Fatalf("reports=%d want 1", len(reports))
	}
	if reports[0].Symbol != "BTCUSDT" {
		t.Fatalf("report symbol=%q want BTCUSDT", reports[0].Symbol)
	}
	if len(reports[0].Intervals) != 1 || reports[0].Intervals[0].Interval != "1h" {
		t.Fatalf("report intervals=%+v", reports[0].Intervals)
	}
}

func TestShadowComparingCompressorSurfacesDiffFailures(t *testing.T) {
	primary := &decision.FeatureCompressor{
		Indicators: decision.DefaultIndicatorBuilder{
			Options:  features.DefaultIndicatorCompressOptions(),
			Computer: decision.TalibComputer{},
		},
		Trends: features.DefaultTrendBuilder{
			Options:  diffTrendOptionsForShadow(),
			Computer: decision.TalibComputer{},
		},
		Mechanics: noopMechanicsBuilder{},
	}

	var reported []features.FeatureError
	compressor := &shadowComparingCompressor{
		primary:            primary,
		primaryName:        "talib",
		shadowName:         "reference",
		primaryComputer:    decision.TalibComputer{},
		indicatorOptions:   features.DefaultIndicatorCompressOptions(),
		trendOptionsByInt:  map[string]decision.TrendCompressOptions{"1h": diffTrendOptionsForShadow()},
		defaultTrendOption: diffTrendOptionsForShadow(),
		reportErr: func(_ context.Context, featureErr features.FeatureError) {
			reported = append(reported, featureErr)
		},
	}

	snap := snapshot.MarketSnapshot{
		Klines: map[string]map[string][]snapshot.Candle{
			"BTCUSDT": {"1h": shadowTestCandles(260)},
		},
	}

	out, errs, err := compressor.Compress(context.Background(), snap)
	if err != nil {
		t.Fatalf("Compress() error = %v", err)
	}
	if len(out.Indicators["BTCUSDT"]) != 1 {
		t.Fatalf("indicator intervals=%d want 1", len(out.Indicators["BTCUSDT"]))
	}
	if len(errs) != 1 {
		t.Fatalf("errs=%d want 1", len(errs))
	}
	if errs[0].Stage != "shadow_diff" {
		t.Fatalf("stage=%q want shadow_diff", errs[0].Stage)
	}
	if len(reported) != 1 {
		t.Fatalf("reported=%d want 1", len(reported))
	}
	if reported[0].Stage != "shadow_diff" {
		t.Fatalf("reported stage=%q want shadow_diff", reported[0].Stage)
	}
}

type noopMechanicsBuilder struct{}

func (noopMechanicsBuilder) BuildMechanics(context.Context, snapshot.MarketSnapshot, string) (features.MechanicsSnapshot, error) {
	return features.MechanicsSnapshot{Symbol: "BTCUSDT", RawJSON: []byte(`{}`)}, nil
}

func diffTrendOptionsForShadow() decision.TrendCompressOptions {
	opts := features.DefaultTrendCompressOptions()
	opts.SkipSuperTrend = true
	return opts
}

func shadowTestCandles(n int) []snapshot.Candle {
	candles := make([]snapshot.Candle, 0, n)
	baseTime := time.Unix(0, 0).UTC()
	price := 100.0
	for i := 0; i < n; i++ {
		wave := float64((i%7)-3) * 0.35
		open := price + wave
		close := open + 0.45
		high := close + 0.6
		low := open - 0.6
		candles = append(candles, snapshot.Candle{
			OpenTime: baseTime.Add(time.Duration(i) * time.Hour).UnixMilli(),
			Open:     open,
			High:     high,
			Low:      low,
			Close:    close,
			Volume:   1000 + float64(i*3),
		})
		price += 0.25
	}
	return candles
}
