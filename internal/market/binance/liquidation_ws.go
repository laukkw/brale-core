package binance

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"brale-core/internal/market"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/snapshot"

	"github.com/adshao/go-binance/v2/futures"
	"go.uber.org/zap"
)

const (
	liquidationSourceName        = "binance_force_order_snapshot_ws"
	liquidationCoverageName      = "largest_order_per_symbol_per_1000ms"
	liquidationStatusOK          = "ok"
	liquidationStatusWarmingUp   = "warming_up"
	liquidationStatusStale       = "stale"
	liquidationStatusUnavailable = "unavailable"
)

var (
	sdkLiquidationOrderServe    = futures.WsLiquidationOrderServe
	sdkAllLiquidationOrderServe = futures.WsAllLiquidationOrderServe
)

type LiquidationStreamOptions struct {
	Symbols []string
	Now     func() time.Time
}

type LiquidationStream struct {
	configuredSymbols []string
	symbols           map[string]*liquidationSymbolState

	windows         []string
	windowDurations map[string]time.Duration
	maxWindow       time.Duration
	priceBins       []int

	now func() time.Time

	mu sync.RWMutex

	ctrlMu sync.Mutex
	stopCh chan struct{}
	cancel context.CancelFunc

	runID     atomic.Uint64
	running   atomic.Bool
	connected atomic.Bool
}

type liquidationSymbolState struct {
	events          liquidationRingBuffer
	coverageStart   time.Time
	lastReconnectAt time.Time
	lastGapResetAt  time.Time
	lastEventAt     time.Time
}

type liquidationRingBuffer struct {
	events []liqEvent
	start  int
}

func NewLiquidationStream(opts LiquidationStreamOptions) (*LiquidationStream, error) {
	symbols := normalizeSymbols(opts.Symbols)
	windows, priceBins, durations, maxWindow, err := resolveLiquidationWindows()
	if err != nil {
		return nil, err
	}
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = func() time.Time { return time.Now().UTC() }
	}

	stream := &LiquidationStream{
		configuredSymbols: symbols,
		symbols:           make(map[string]*liquidationSymbolState, len(symbols)),
		windows:           append([]string(nil), windows...),
		windowDurations:   durations,
		maxWindow:         maxWindow,
		priceBins:         append([]int(nil), priceBins...),
		now:               nowFn,
		stopCh:            make(chan struct{}),
	}
	for _, symbol := range symbols {
		stream.symbols[symbol] = &liquidationSymbolState{}
	}
	return stream, nil
}

func (s *LiquidationStream) Start(ctx context.Context) error {
	if s == nil {
		return errors.New("liquidation stream is nil")
	}
	if len(s.configuredSymbols) == 0 {
		return nil
	}
	if s.running.Swap(true) {
		return nil
	}
	baseCtx := ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	logger := logging.FromContext(baseCtx)
	runCtx, cancel := context.WithCancel(baseCtx)
	runCtx = logging.WithLogger(runCtx, logger)
	stopCh := make(chan struct{})

	s.ctrlMu.Lock()
	s.stopCh = stopCh
	s.cancel = cancel
	s.ctrlMu.Unlock()

	runID := s.runID.Add(1)
	go s.run(runCtx, runID, stopCh)
	return nil
}

func (s *LiquidationStream) Close() {
	if s == nil {
		return
	}
	if !s.running.Swap(false) {
		return
	}
	s.connected.Store(false)
	s.ctrlMu.Lock()
	cancel := s.cancel
	s.cancel = nil
	stopCh := s.stopCh
	s.ctrlMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if stopCh == nil {
		return
	}
	select {
	case <-stopCh:
	default:
		close(stopCh)
	}
}

func (s *LiquidationStream) run(ctx context.Context, runID uint64, stopCh <-chan struct{}) {
	logger := logging.FromContext(ctx).Named("market")
	backoff := time.Second
	connectedOnce := false
	retrying := false
	for {
		if !s.running.Load() {
			s.setConnected(runID, false)
			return
		}
		doneC, sdkStopC, errC, err := s.serve()
		if err != nil {
			s.setConnected(runID, false)
			logger.Warn("liquidation ws start failed", zap.Error(err))
			if connectedOnce {
				retrying = true
			}
			if !waitRetry(ctx, stopCh, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}

		connectedAt := s.now()
		if retrying {
			logger.Info("liquidation ws reconnected")
		} else {
			logger.Info("liquidation ws connected")
		}
		s.setConnected(runID, true)
		s.markReconnect(connectedAt)
		connectedOnce = true
		retrying = false
		backoff = time.Second

		select {
		case <-ctx.Done():
			s.setConnected(runID, false)
			signalLiquidationStop(doneC, sdkStopC)
			return
		case <-stopCh:
			s.setConnected(runID, false)
			signalLiquidationStop(doneC, sdkStopC)
			return
		case <-doneC:
			s.setConnected(runID, false)
			if err := drainLiquidationErr(errC); err != nil && ctx.Err() == nil {
				logger.Warn("liquidation ws disconnected", zap.Error(err))
			}
			if !waitRetry(ctx, stopCh, backoff) {
				return
			}
			retrying = true
			backoff = nextBackoff(backoff)
		}
	}
}

func (s *LiquidationStream) serve() (doneC, stopC chan struct{}, errC <-chan error, err error) {
	errCh := make(chan error, 1)
	handler := func(event *futures.WsLiquidationOrderEvent) {
		item, ok, parseErr := parseLiquidationEvent(event)
		if parseErr != nil {
			logging.L().Named("market").Warn("liquidation ws decode failed", zap.Error(parseErr))
			return
		}
		if !ok {
			return
		}
		s.handleLiquidationEvent(item)
	}
	errHandler := func(err error) {
		if err == nil {
			return
		}
		select {
		case errCh <- err:
		default:
		}
	}
	if len(s.configuredSymbols) == 1 {
		doneC, stopC, err = sdkLiquidationOrderServe(s.configuredSymbols[0], handler, errHandler)
	} else {
		doneC, stopC, err = sdkAllLiquidationOrderServe(handler, errHandler)
	}
	if err != nil {
		return nil, nil, nil, err
	}
	return doneC, stopC, errCh, nil
}

func signalLiquidationStop(doneC, stopC chan struct{}) bool {
	if stopC == nil {
		return false
	}
	if doneC != nil {
		select {
		case <-doneC:
			return false
		default:
		}
	}
	close(stopC)
	return true
}

func drainLiquidationErr(errC <-chan error) error {
	if errC == nil {
		return nil
	}
	select {
	case err := <-errC:
		return err
	default:
		return nil
	}
}

func parseLiquidationEvent(event *futures.WsLiquidationOrderEvent) (liqEvent, bool, error) {
	if event == nil {
		return liqEvent{}, false, nil
	}
	order := event.LiquidationOrder
	price, err := parseLiquidationPrice(order)
	if err != nil {
		return liqEvent{}, false, err
	}
	qty, err := parseLiquidationQty(order)
	if err != nil {
		return liqEvent{}, false, err
	}
	if price <= 0 || qty <= 0 {
		return liqEvent{}, false, nil
	}
	isLong, ok := liquidationSideIsLong(order.Side)
	if !ok {
		return liqEvent{}, false, nil
	}
	symbol := strings.ToUpper(strings.TrimSpace(order.Symbol))
	if symbol == "" {
		return liqEvent{}, false, nil
	}
	timeMs := order.TradeTime
	if timeMs <= 0 {
		timeMs = event.Time
	}
	if timeMs <= 0 {
		timeMs = time.Now().UTC().UnixMilli()
	}
	return liqEvent{
		symbol:   symbol,
		timeMs:   timeMs,
		price:    price,
		qty:      qty,
		notional: price * qty,
		isLong:   isLong,
	}, true, nil
}

func parseLiquidationPrice(order futures.WsLiquidationOrder) (float64, error) {
	if price, err := parseFloat(order.AvgPrice); err == nil && price > 0 {
		return price, nil
	}
	return parseFloat(order.Price)
}

func parseLiquidationQty(order futures.WsLiquidationOrder) (float64, error) {
	for _, raw := range []string{order.AccumulatedFilledQty, order.LastFilledQty, order.OrigQuantity} {
		qty, err := parseFloat(raw)
		if err == nil && qty > 0 {
			return qty, nil
		}
	}
	return 0, nil
}

func (s *LiquidationStream) handleLiquidationEvent(event liqEvent) {
	if event.symbol == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.symbols[event.symbol]
	if state == nil {
		return
	}
	cutoff := s.now().Add(-s.maxWindow).UnixMilli()
	state.events.PruneBefore(cutoff)
	state.events.Push(event)
	eventTime := time.UnixMilli(event.timeMs).UTC()
	if !eventTime.IsZero() {
		state.lastEventAt = eventTime
	}
}

func (s *LiquidationStream) LiquidationsByWindow(_ context.Context, symbol string) (map[string]snapshot.LiqWindow, error) {
	if s == nil {
		return nil, fmt.Errorf("liquidation stream is nil")
	}
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	now := s.now()

	s.mu.Lock()
	state := s.symbols[symbol]
	if state == nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("liquidation symbol %s not configured", symbol)
	}
	state.events.PruneBefore(now.Add(-s.maxWindow).UnixMilli())
	events := state.events.Items()
	coverageStart := state.coverageStart
	s.mu.Unlock()

	results := aggregateLiquidationWindows(events, s.windows, s.windowDurations, now.UnixMilli(), s.priceBins)
	connected := s.connected.Load()
	coverageSec := secondsSince(coverageStart, now)
	for _, window := range s.windows {
		item := results[window]
		item.CoverageSec = minInt64(coverageSec, int64(s.windowDurations[window].Seconds()))
		item.Status, item.Complete = liquidationWindowStatus(connected, coverageStart, s.windowDurations[window], now)
		results[window] = item
	}
	return results, nil
}

func (s *LiquidationStream) LiquidationSource(ctx context.Context, symbol string) (snapshot.LiqSource, error) {
	windows, err := s.LiquidationsByWindow(ctx, symbol)
	if err != nil {
		return snapshot.LiqSource{}, err
	}
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	s.mu.RLock()
	state := s.symbols[symbol]
	var coverageStart time.Time
	var lastEventAt time.Time
	var lastReconnectAt time.Time
	var lastGapResetAt time.Time
	if state != nil {
		coverageStart = state.coverageStart
		lastEventAt = state.lastEventAt
		lastReconnectAt = state.lastReconnectAt
		lastGapResetAt = state.lastGapResetAt
	}
	s.mu.RUnlock()
	if state == nil {
		return snapshot.LiqSource{}, fmt.Errorf("liquidation symbol %s not configured", symbol)
	}
	primary := s.primaryWindow(windows)
	now := s.now()
	return snapshot.LiqSource{
		Source:            liquidationSourceName,
		Coverage:          liquidationCoverageName,
		Status:            primary.Status,
		StreamConnected:   s.connected.Load(),
		CoverageSec:       secondsSince(coverageStart, now),
		SampleCount:       primary.SampleCount,
		LastEventAgeSec:   secondsSince(lastEventAt, now),
		Complete:          primary.Complete,
		LastReconnectTime: lastReconnectAt.UnixMilli(),
		LastEventTime:     lastEventAt.UnixMilli(),
		LastGapResetTime:  lastGapResetAt.UnixMilli(),
	}, nil
}

func (s *LiquidationStream) LiquidationStreamStatus(symbol string) (market.LiquidationStreamStatus, bool) {
	source, err := s.LiquidationSource(context.Background(), symbol)
	if err != nil {
		return market.LiquidationStreamStatus{}, false
	}
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	return market.LiquidationStreamStatus{
		Symbol:          symbol,
		Source:          source.Source,
		Status:          source.Status,
		StreamConnected: source.StreamConnected,
		ShardCount:      s.connectionCount(),
		CoverageSec:     source.CoverageSec,
		SampleCount:     source.SampleCount,
		LastEventAgeSec: source.LastEventAgeSec,
		Complete:        source.Complete,
	}, true
}

func (s *LiquidationStream) connectionCount() int {
	if s == nil || len(s.configuredSymbols) == 0 {
		return 0
	}
	return 1
}

func (s *LiquidationStream) primaryWindow(windows map[string]snapshot.LiqWindow) snapshot.LiqWindow {
	best := snapshot.LiqWindow{Status: liquidationStatusUnavailable}
	var bestDur time.Duration
	for name, item := range windows {
		if dur := s.windowDurations[name]; dur >= bestDur {
			best = item
			bestDur = dur
		}
	}
	return best
}

func (s *LiquidationStream) markReconnect(ts time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, state := range s.symbols {
		if state == nil {
			continue
		}
		state.coverageStart = ts
		state.lastGapResetAt = ts
		state.lastReconnectAt = ts
	}
}

func (s *LiquidationStream) setConnected(runID uint64, connected bool) {
	if s == nil || runID == 0 || s.runID.Load() != runID {
		return
	}
	s.connected.Store(connected)
}

func (b *liquidationRingBuffer) Push(event liqEvent) {
	b.events = append(b.events, event)
}

func (b *liquidationRingBuffer) PruneBefore(cutoff int64) {
	for b.start < len(b.events) && b.events[b.start].timeMs < cutoff {
		b.start++
	}
	if b.start > 0 && b.start*2 >= len(b.events) {
		b.events = append([]liqEvent(nil), b.events[b.start:]...)
		b.start = 0
	}
}

func (b *liquidationRingBuffer) Items() []liqEvent {
	if b.start >= len(b.events) {
		return nil
	}
	return append([]liqEvent(nil), b.events[b.start:]...)
}

func liquidationWindowStatus(connected bool, coverageStart time.Time, windowDur time.Duration, now time.Time) (string, bool) {
	if coverageStart.IsZero() {
		return liquidationStatusUnavailable, false
	}
	if !connected {
		return liquidationStatusStale, false
	}
	if secondsSince(coverageStart, now) < int64(windowDur.Seconds()) {
		return liquidationStatusWarmingUp, false
	}
	return liquidationStatusOK, true
}

func secondsSince(ref time.Time, now time.Time) int64 {
	if ref.IsZero() {
		return 0
	}
	age := int64(now.Sub(ref).Seconds())
	if age < 0 {
		return 0
	}
	return age
}

func minInt64(a, b int64) int64 {
	if a == 0 || a < b {
		return a
	}
	return b
}

func waitRetry(ctx context.Context, stopCh <-chan struct{}, backoff time.Duration) bool {
	timer := time.NewTimer(backoff)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-stopCh:
		return false
	case <-timer.C:
		return true
	}
}
