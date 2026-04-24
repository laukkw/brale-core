package binance

import (
	"context"
	"testing"
	"time"

	"brale-core/internal/snapshot"

	"github.com/adshao/go-binance/v2/futures"
)

func mustNewLiquidationStream(t *testing.T, opts LiquidationStreamOptions) *LiquidationStream {
	t.Helper()
	stream, err := NewLiquidationStream(opts)
	if err != nil {
		t.Fatalf("NewLiquidationStream() error = %v", err)
	}
	return stream
}

func TestParseLiquidationEventUsesFallbackFields(t *testing.T) {
	t.Parallel()

	event, ok, err := parseLiquidationEvent(&futures.WsLiquidationOrderEvent{
		Time: time.Unix(1_700_000_000, 0).UnixMilli(),
		LiquidationOrder: futures.WsLiquidationOrder{
			Symbol:               "BTCUSDT",
			Side:                 futures.SideTypeSell,
			Price:                "100",
			AvgPrice:             "",
			AccumulatedFilledQty: "",
			LastFilledQty:        "2",
			OrigQuantity:         "5",
			TradeTime:            time.Unix(1_700_000_001, 0).UnixMilli(),
		},
	})
	if err != nil {
		t.Fatalf("parseLiquidationEvent() error = %v", err)
	}
	if !ok {
		t.Fatal("parseLiquidationEvent() ok=false want true")
	}
	if event.symbol != "BTCUSDT" {
		t.Fatalf("symbol=%q want BTCUSDT", event.symbol)
	}
	if !event.isLong {
		t.Fatal("SELL liquidation should be long liquidation")
	}
	if event.notional != 200 {
		t.Fatalf("notional=%v want 200", event.notional)
	}
	if event.timeMs != time.Unix(1_700_000_001, 0).UnixMilli() {
		t.Fatalf("timeMs=%d want trade time", event.timeMs)
	}
}

func TestLiquidationStreamReportsOkWithZeroSamplesAfterWarmup(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()
	stream := mustNewLiquidationStream(t, LiquidationStreamOptions{
		Symbols: []string{"BTCUSDT"},
		Now:     func() time.Time { return now },
	})
	stream.mu.Lock()
	state := stream.symbols["BTCUSDT"]
	state.coverageStart = now.Add(-5 * time.Hour)
	stream.connected.Store(true)
	stream.mu.Unlock()

	windows, err := stream.LiquidationsByWindow(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("LiquidationsByWindow() error = %v", err)
	}
	source, err := stream.LiquidationSource(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("LiquidationSource() error = %v", err)
	}
	for _, key := range []string{snapshot.LiqWindow5m, snapshot.LiqWindow1h, snapshot.LiqWindow4h} {
		got := windows[key]
		if got.Status != liquidationStatusOK {
			t.Fatalf("%s status=%q want %q", key, got.Status, liquidationStatusOK)
		}
		if !got.Complete {
			t.Fatalf("%s complete=false want true", key)
		}
		if got.SampleCount != 0 {
			t.Fatalf("%s sample_count=%d want 0", key, got.SampleCount)
		}
	}
	if source.Status != liquidationStatusOK {
		t.Fatalf("source status=%q want ok", source.Status)
	}
	if !source.Complete {
		t.Fatal("source complete=false want true")
	}
}

func TestLiquidationStreamReconnectResetsCoverage(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()
	stream := mustNewLiquidationStream(t, LiquidationStreamOptions{
		Symbols: []string{"BTCUSDT"},
		Now:     func() time.Time { return now },
	})
	stream.mu.Lock()
	state := stream.symbols["BTCUSDT"]
	state.coverageStart = now.Add(-5 * time.Hour)
	state.lastGapResetAt = now.Add(-5 * time.Hour)
	state.events.Push(liqEvent{
		symbol:   "BTCUSDT",
		timeMs:   now.Add(-10 * time.Minute).UnixMilli(),
		price:    100,
		qty:      2,
		notional: 200,
		isLong:   true,
	})
	stream.mu.Unlock()

	reconnectedAt := now.Add(-30 * time.Minute)
	stream.markReconnect(reconnectedAt)
	stream.connected.Store(true)

	windows, err := stream.LiquidationsByWindow(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("LiquidationsByWindow() error = %v", err)
	}
	source, err := stream.LiquidationSource(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("LiquidationSource() error = %v", err)
	}
	if got := windows[snapshot.LiqWindow1h]; got.Status != liquidationStatusWarmingUp || got.Complete {
		t.Fatalf("1h window=%+v want warming_up incomplete", got)
	}
	if got := windows[snapshot.LiqWindow5m]; got.Status != liquidationStatusOK || !got.Complete {
		t.Fatalf("5m window=%+v want ok complete", got)
	}
	if source.LastReconnectTime != reconnectedAt.UnixMilli() {
		t.Fatalf("last_reconnect_time=%d want %d", source.LastReconnectTime, reconnectedAt.UnixMilli())
	}
	if source.LastGapResetTime != reconnectedAt.UnixMilli() {
		t.Fatalf("last_gap_reset_time=%d want %d", source.LastGapResetTime, reconnectedAt.UnixMilli())
	}
}

func TestLiquidationStreamPrunesEventsOutsideMaxWindow(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()
	stream := mustNewLiquidationStream(t, LiquidationStreamOptions{
		Symbols: []string{"BTCUSDT"},
		Now:     func() time.Time { return now },
	})
	stream.mu.Lock()
	state := stream.symbols["BTCUSDT"]
	state.coverageStart = now.Add(-5 * time.Hour)
	state.events.Push(liqEvent{
		symbol:   "BTCUSDT",
		timeMs:   now.Add(-5 * time.Hour).UnixMilli(),
		price:    100,
		qty:      1,
		notional: 100,
		isLong:   true,
	})
	state.events.Push(liqEvent{
		symbol:   "BTCUSDT",
		timeMs:   now.Add(-30 * time.Minute).UnixMilli(),
		price:    105,
		qty:      2,
		notional: 210,
		isLong:   false,
	})
	stream.connected.Store(true)
	stream.mu.Unlock()

	windows, err := stream.LiquidationsByWindow(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("LiquidationsByWindow() error = %v", err)
	}
	got := windows[snapshot.LiqWindow4h]
	if got.SampleCount != 1 {
		t.Fatalf("sample_count=%d want 1", got.SampleCount)
	}
	if got.TotalVol != 210 {
		t.Fatalf("total_vol=%v want 210", got.TotalVol)
	}
}

func TestLiquidationStreamServeUsesSingleSymbolSDK(t *testing.T) {
	doneC := make(chan struct{})
	stopC := make(chan struct{})
	t.Cleanup(func() { close(doneC) })
	now := time.Unix(1_700_000_000, 0).UTC()

	called := false
	origSingle := sdkLiquidationOrderServe
	origAll := sdkAllLiquidationOrderServe
	t.Cleanup(func() {
		sdkLiquidationOrderServe = origSingle
		sdkAllLiquidationOrderServe = origAll
	})
	sdkLiquidationOrderServe = func(symbol string, handler futures.WsLiquidationOrderHandler, errHandler futures.ErrHandler) (chan struct{}, chan struct{}, error) {
		called = true
		if symbol != "ETHUSDT" {
			t.Fatalf("symbol=%q want ETHUSDT", symbol)
		}
		handler(&futures.WsLiquidationOrderEvent{
			Time: time.Unix(1_700_000_000, 0).UnixMilli(),
			LiquidationOrder: futures.WsLiquidationOrder{
				Symbol:               "ETHUSDT",
				Side:                 futures.SideTypeBuy,
				AvgPrice:             "2000",
				AccumulatedFilledQty: "1.5",
			},
		})
		return doneC, stopC, nil
	}
	sdkAllLiquidationOrderServe = func(handler futures.WsLiquidationOrderHandler, errHandler futures.ErrHandler) (chan struct{}, chan struct{}, error) {
		t.Fatal("all-market serve should not be used for single-symbol streams")
		return nil, nil, nil
	}

	stream := mustNewLiquidationStream(t, LiquidationStreamOptions{
		Symbols: []string{"ETHUSDT"},
		Now:     func() time.Time { return now },
	})
	gotDoneC, gotStopC, _, err := stream.serve()
	if err != nil {
		t.Fatalf("serve() error = %v", err)
	}
	if !called {
		t.Fatal("single-symbol SDK serve was not called")
	}
	if gotDoneC != doneC || gotStopC != stopC {
		t.Fatal("serve() did not return SDK channels")
	}

	windows, err := stream.LiquidationsByWindow(context.Background(), "ETHUSDT")
	if err != nil {
		t.Fatalf("LiquidationsByWindow() error = %v", err)
	}
	if got := windows[snapshot.LiqWindow4h].SampleCount; got != 1 {
		t.Fatalf("sample_count=%d want 1", got)
	}
}

func TestLiquidationStreamServeUsesAllMarketSDKForMultipleSymbols(t *testing.T) {
	doneC := make(chan struct{})
	stopC := make(chan struct{})
	t.Cleanup(func() { close(doneC) })
	now := time.Unix(1_700_000_000, 0).UTC()

	called := false
	origSingle := sdkLiquidationOrderServe
	origAll := sdkAllLiquidationOrderServe
	t.Cleanup(func() {
		sdkLiquidationOrderServe = origSingle
		sdkAllLiquidationOrderServe = origAll
	})
	sdkLiquidationOrderServe = func(symbol string, handler futures.WsLiquidationOrderHandler, errHandler futures.ErrHandler) (chan struct{}, chan struct{}, error) {
		t.Fatal("single-symbol serve should not be used for multi-symbol streams")
		return nil, nil, nil
	}
	sdkAllLiquidationOrderServe = func(handler futures.WsLiquidationOrderHandler, errHandler futures.ErrHandler) (chan struct{}, chan struct{}, error) {
		called = true
		handler(&futures.WsLiquidationOrderEvent{
			Time: time.Unix(1_700_000_000, 0).UnixMilli(),
			LiquidationOrder: futures.WsLiquidationOrder{
				Symbol:               "ETHUSDT",
				Side:                 futures.SideTypeBuy,
				AvgPrice:             "2000",
				AccumulatedFilledQty: "1.5",
			},
		})
		handler(&futures.WsLiquidationOrderEvent{
			Time: time.Unix(1_700_000_000, 0).UnixMilli(),
			LiquidationOrder: futures.WsLiquidationOrder{
				Symbol:               "XRPUSDT",
				Side:                 futures.SideTypeSell,
				AvgPrice:             "0.5",
				AccumulatedFilledQty: "100",
			},
		})
		return doneC, stopC, nil
	}

	stream := mustNewLiquidationStream(t, LiquidationStreamOptions{
		Symbols: []string{"BTCUSDT", "ETHUSDT"},
		Now:     func() time.Time { return now },
	})
	gotDoneC, gotStopC, _, err := stream.serve()
	if err != nil {
		t.Fatalf("serve() error = %v", err)
	}
	if !called {
		t.Fatal("all-market SDK serve was not called")
	}
	if gotDoneC != doneC || gotStopC != stopC {
		t.Fatal("serve() did not return SDK channels")
	}

	ethWindows, err := stream.LiquidationsByWindow(context.Background(), "ETHUSDT")
	if err != nil {
		t.Fatalf("LiquidationsByWindow(ETHUSDT) error = %v", err)
	}
	if got := ethWindows[snapshot.LiqWindow4h].SampleCount; got != 1 {
		t.Fatalf("ETH sample_count=%d want 1", got)
	}
	btcWindows, err := stream.LiquidationsByWindow(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("LiquidationsByWindow(BTCUSDT) error = %v", err)
	}
	if got := btcWindows[snapshot.LiqWindow4h].SampleCount; got != 0 {
		t.Fatalf("BTC sample_count=%d want 0", got)
	}
}
