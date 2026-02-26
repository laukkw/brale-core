// 本文件主要内容：维护 Binance Mark Price WebSocket 订阅与本地缓存。
package binance

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"brale-core/internal/market"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/pkg/parseutil"

	"github.com/adshao/go-binance/v2/futures"
	"go.uber.org/zap"
)

type MarkPriceStreamOptions struct {
	Symbols []string
	Rate    time.Duration
}

// MarkPriceStream keeps a local cache of mark prices from Binance futures websocket.
type MarkPriceStream struct {
	symbols []string
	rate    time.Duration

	mu     sync.RWMutex
	quotes map[string]market.PriceQuote

	ctrlMu  sync.Mutex
	stopCh  chan struct{}
	cancel  context.CancelFunc
	running atomic.Bool
}

func NewMarkPriceStream(opts MarkPriceStreamOptions) *MarkPriceStream {
	symbols := normalizeSymbols(opts.Symbols)
	rate := opts.Rate
	if rate == 0 {
		rate = time.Second
	} else if rate != time.Second && rate != 3*time.Second {
		rate = time.Second
	}
	return &MarkPriceStream{
		symbols: symbols,
		rate:    rate,
		quotes:  make(map[string]market.PriceQuote),
		stopCh:  make(chan struct{}),
	}
}

func (s *MarkPriceStream) Start(ctx context.Context) error {
	if s == nil {
		return errors.New("mark price stream is nil")
	}
	if s.running.Swap(true) {
		return nil
	}
	baseCtx := ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	logger := logging.FromContext(baseCtx)
	streamCtx, cancel := context.WithCancel(baseCtx)
	streamCtx = logging.WithLogger(streamCtx, logger)

	s.ctrlMu.Lock()
	s.stopCh = make(chan struct{})
	s.cancel = cancel
	s.ctrlMu.Unlock()

	go s.run(streamCtx)
	return nil
}

func (s *MarkPriceStream) Close() {
	if s == nil {
		return
	}
	if !s.running.Swap(false) {
		return
	}
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

func (s *MarkPriceStream) MarkPrice(ctx context.Context, symbol string) (market.PriceQuote, error) {
	if s == nil {
		return market.PriceQuote{}, market.ErrPriceUnavailable
	}
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return market.PriceQuote{}, errors.New("symbol is required")
	}
	s.mu.RLock()
	quote, ok := s.quotes[symbol]
	s.mu.RUnlock()
	if !ok || quote.Price <= 0 {
		return market.PriceQuote{}, market.ErrPriceUnavailable
	}
	return quote, nil
}

func (s *MarkPriceStream) run(ctx context.Context) {
	logger := logging.FromContext(ctx).Named("market")
	backoff := time.Second
	connectedOnce := false
	retrying := false
	for {
		if !s.running.Load() {
			return
		}
		doneC, stopC, err := s.serve(ctx)
		if err != nil {
			logger.Warn("mark price ws start failed", zap.Error(err))
			if connectedOnce {
				retrying = true
			}
			if !s.waitRetry(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}
		if retrying {
			logger.Info("mark price ws reconnected")
		}
		connectedOnce = true
		retrying = false
		backoff = time.Second
		logger.Info("mark price ws connected")
		select {
		case <-ctx.Done():
			stopC <- struct{}{}
			return
		case <-s.stopCh:
			stopC <- struct{}{}
			return
		case <-doneC:
			if connectedOnce {
				retrying = true
			}
			if !s.waitRetry(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
		}
	}
}

func (s *MarkPriceStream) serve(ctx context.Context) (doneC, stopC chan struct{}, err error) {
	logger := logging.FromContext(ctx).Named("market")
	errHandler := func(err error) {
		if err != nil {
			logger.Warn("mark price ws error", zap.Error(err))
		}
	}
	handler := func(event *futures.WsMarkPriceEvent) {
		s.handleEvent(event)
	}
	if len(s.symbols) == 0 {
		allHandler := func(events futures.WsAllMarkPriceEvent) {
			for _, event := range events {
				s.handleEvent(event)
			}
		}
		return futures.WsAllMarkPriceServeWithRate(s.rate, allHandler, errHandler)
	}
	rates := make(map[string]time.Duration, len(s.symbols))
	for _, sym := range s.symbols {
		rates[sym] = s.rate
	}
	return futures.WsCombinedMarkPriceServeWithRate(rates, handler, errHandler)
}

func (s *MarkPriceStream) handleEvent(event *futures.WsMarkPriceEvent) {
	if event == nil {
		return
	}
	price, ok := parseutil.FloatStringOK(event.MarkPrice)
	if !ok || price <= 0 {
		return
	}
	symbol := strings.ToUpper(strings.TrimSpace(event.Symbol))
	if symbol == "" {
		return
	}
	quote := market.PriceQuote{
		Symbol:    symbol,
		Price:     price,
		Timestamp: event.Time,
		Source:    "binance_mark_ws",
	}
	s.mu.Lock()
	s.quotes[symbol] = quote
	s.mu.Unlock()
}

func (s *MarkPriceStream) waitRetry(ctx context.Context, backoff time.Duration) bool {
	timer := time.NewTimer(backoff)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-s.stopCh:
		return false
	case <-timer.C:
		return true
	}
}

func nextBackoff(backoff time.Duration) time.Duration {
	if backoff < 5*time.Second {
		return backoff + time.Second
	}
	return 5 * time.Second
}

func normalizeSymbols(symbols []string) []string {
	if len(symbols) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(symbols))
	out := make([]string, 0, len(symbols))
	for _, sym := range symbols {
		sym = strings.ToUpper(strings.TrimSpace(sym))
		if sym == "" {
			continue
		}
		if _, ok := seen[sym]; ok {
			continue
		}
		seen[sym] = struct{}{}
		out = append(out, sym)
	}
	return out
}
