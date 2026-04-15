package features

import (
	"testing"

	"brale-core/internal/snapshot"
)

func TestRunEngineDiffTalibVsTalibIsZero(t *testing.T) {
	report, err := RunEngineDiff(EngineDiffRequest{
		Symbol:            "BTCUSDT",
		BaselineName:      "talib",
		Baseline:          TalibComputer{},
		CandidateName:     "talib",
		Candidate:         TalibComputer{},
		IndicatorOptions:  DefaultIndicatorCompressOptions(),
		TrendOptionsByInt: map[string]TrendCompressOptions{"1h": diffTrendOptions()},
		DefaultTrendOpts:  diffTrendOptions(),
		CandlesByInterval: map[string][]snapshot.Candle{"1h": oscillatingTrendTestCandles(260)},
	})
	if err != nil {
		t.Fatalf("RunEngineDiff() error = %v", err)
	}
	if report.Symbol != "BTCUSDT" {
		t.Fatalf("Symbol=%q want BTCUSDT", report.Symbol)
	}
	if len(report.Intervals) != 1 {
		t.Fatalf("intervals=%d want 1", len(report.Intervals))
	}
	for _, diff := range report.Intervals[0].Numeric {
		if diff.MaxDiff != 0 || diff.AvgDiff != 0 || diff.LatestDiff != 0 {
			t.Fatalf("numeric diff should be zero: %+v", diff)
		}
	}
	for _, semantic := range report.Intervals[0].Semantics {
		if !semantic.Match {
			t.Fatalf("semantic diff should match: %+v", semantic)
		}
	}
}

func TestRunEngineDiffTalibVsReferenceIncludesNumericAndSemanticChecks(t *testing.T) {
	report, err := RunEngineDiff(EngineDiffRequest{
		Symbol:            "BTCUSDT",
		BaselineName:      "talib",
		Baseline:          TalibComputer{},
		CandidateName:     "reference",
		Candidate:         ReferenceComputer{},
		IndicatorOptions:  DefaultIndicatorCompressOptions(),
		TrendOptionsByInt: map[string]TrendCompressOptions{"1h": diffTrendOptions()},
		DefaultTrendOpts:  diffTrendOptions(),
		CandlesByInterval: map[string][]snapshot.Candle{"1h": oscillatingTrendTestCandles(260)},
	})
	if err != nil {
		t.Fatalf("RunEngineDiff() error = %v", err)
	}
	interval := report.Intervals[0]
	if _, ok := findNumericDiff(interval.Numeric, "indicator.ema_fast"); !ok {
		t.Fatalf("missing indicator.ema_fast diff: %+v", interval.Numeric)
	}
	if _, ok := findNumericDiff(interval.Numeric, "trend.ema20"); !ok {
		t.Fatalf("missing trend.ema20 diff: %+v", interval.Numeric)
	}
	if _, ok := findSemanticDiff(interval.Semantics, "indicator.rsi_zone"); !ok {
		t.Fatalf("missing indicator.rsi_zone semantic: %+v", interval.Semantics)
	}
	if _, ok := findSemanticDiff(interval.Semantics, "trend.ema_stack"); !ok {
		t.Fatalf("missing trend.ema_stack semantic: %+v", interval.Semantics)
	}
}

func diffTrendOptions() TrendCompressOptions {
	opts := DefaultTrendCompressOptions()
	opts.SkipSuperTrend = true
	return opts
}

func findNumericDiff(diffs []SeriesDiff, name string) (SeriesDiff, bool) {
	for _, diff := range diffs {
		if diff.Name == name {
			return diff, true
		}
	}
	return SeriesDiff{}, false
}

func findSemanticDiff(diffs []SemanticDiff, name string) (SemanticDiff, bool) {
	for _, diff := range diffs {
		if diff.Name == name {
			return diff, true
		}
	}
	return SemanticDiff{}, false
}
