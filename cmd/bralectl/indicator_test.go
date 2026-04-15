package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"brale-core/internal/snapshot"
)

func TestIndicatorDiffCommandWritesJSON(t *testing.T) {
	_, indexPath := writeBacktestConfigTree(t)
	prev := newIndicatorDiffFetcher
	newIndicatorDiffFetcher = func() *snapshot.Fetcher {
		return &snapshot.Fetcher{
			Klines: diffTestKlineProvider{
				byInterval: map[string][]snapshot.Candle{
					"15m": diffTestCandles(260, 15*time.Minute),
					"1h":  diffTestCandles(260, time.Hour),
					"4h":  diffTestCandles(260, 4*time.Hour),
				},
			},
			Now: func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		}
	}
	defer func() { newIndicatorDiffFetcher = prev }()

	out, errOut, err := executeRootCommand(
		t,
		"indicator", "diff",
		"--symbol", "BTCUSDT",
		"--index", indexPath,
		"--baseline", "talib",
		"--candidate", "talib",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("executeRootCommand() error = %v, stderr=%s", err, errOut)
	}
	if strings.TrimSpace(errOut) != "" {
		t.Fatalf("stderr=%q want empty", errOut)
	}
	if !strings.Contains(out, `"baseline": "talib"`) || !strings.Contains(out, `"candidate": "talib"`) {
		t.Fatalf("stdout=%s", out)
	}
	if !strings.Contains(out, `"indicator.ema_fast"`) {
		t.Fatalf("stdout missing numeric diff: %s", out)
	}
}

func TestIndicatorDiffCommandRejectsUnknownEngine(t *testing.T) {
	_, indexPath := writeBacktestConfigTree(t)
	_, errOut, err := executeRootCommand(
		t,
		"indicator", "diff",
		"--symbol", "BTCUSDT",
		"--index", indexPath,
		"--candidate", "mystery",
	)
	if err == nil {
		t.Fatal("executeRootCommand() error = nil, want unsupported engine")
	}
	if !strings.Contains(err.Error(), "unsupported engine") && !strings.Contains(errOut, "unsupported engine") {
		t.Fatalf("err=%v stderr=%s", err, errOut)
	}
}

type diffTestKlineProvider struct {
	byInterval map[string][]snapshot.Candle
}

func (p diffTestKlineProvider) Klines(_ context.Context, _ string, interval string, _ int) ([]snapshot.Candle, error) {
	return append([]snapshot.Candle(nil), p.byInterval[interval]...), nil
}

func diffTestCandles(n int, step time.Duration) []snapshot.Candle {
	candles := make([]snapshot.Candle, n)
	baseTime := time.Unix(1_600_000_000, 0).UTC()
	for i := range candles {
		base := 100.0 + float64(i)*0.25
		candles[i] = snapshot.Candle{
			OpenTime: baseTime.Add(time.Duration(i) * step).UnixMilli(),
			Open:     base - 0.4,
			High:     base + 1.2,
			Low:      base - 1.1,
			Close:    base + 0.3,
			Volume:   1000 + float64(i)*3,
		}
	}
	return candles
}
