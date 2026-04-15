package notify

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"brale-core/internal/pkg/logging"

	"go.uber.org/zap"
)

const defaultCloseAggregationWindow = 2 * time.Second

type aggregatedCloseNotice struct {
	Key           string
	Symbol        string
	Direction     string
	PositionClose *PositionCloseNotice
	CloseSummary  *PositionCloseSummaryNotice
	TradeClose    *TradeCloseSummaryNotice
}

func (a aggregatedCloseNotice) isComplete() bool {
	return a.PositionClose != nil && a.CloseSummary != nil && a.TradeClose != nil
}

type closeNoticeAggregator struct {
	mu       sync.Mutex
	window   time.Duration
	pending  map[string]*aggregatedCloseNotice
	timers   map[string]*time.Timer
	callback func(context.Context, aggregatedCloseNotice) error
}

func newCloseNoticeAggregator(window time.Duration, callback func(context.Context, aggregatedCloseNotice) error) *closeNoticeAggregator {
	if window <= 0 {
		window = defaultCloseAggregationWindow
	}
	if callback == nil {
		return nil
	}
	return &closeNoticeAggregator{
		window:   window,
		pending:  make(map[string]*aggregatedCloseNotice),
		timers:   make(map[string]*time.Timer),
		callback: callback,
	}
}

func (a *closeNoticeAggregator) AddPositionClose(key string, notice PositionCloseNotice) {
	if a == nil || strings.TrimSpace(key) == "" {
		return
	}
	cloned := notice
	cloned.TakeProfits = slices.Clone(notice.TakeProfits)
	a.add(key, func(item *aggregatedCloseNotice) {
		item.PositionClose = &cloned
		fillAggregatedCloseMeta(item, cloned.Symbol, cloned.Direction)
	})
}

func (a *closeNoticeAggregator) AddPositionCloseSummary(key string, notice PositionCloseSummaryNotice) {
	if a == nil || strings.TrimSpace(key) == "" {
		return
	}
	cloned := notice
	cloned.TakeProfits = slices.Clone(notice.TakeProfits)
	a.add(key, func(item *aggregatedCloseNotice) {
		item.CloseSummary = &cloned
		fillAggregatedCloseMeta(item, cloned.Symbol, cloned.Direction)
	})
}

func (a *closeNoticeAggregator) AddTradeCloseSummary(key string, notice TradeCloseSummaryNotice) {
	if a == nil || strings.TrimSpace(key) == "" {
		return
	}
	cloned := notice
	a.add(key, func(item *aggregatedCloseNotice) {
		item.TradeClose = &cloned
		fillAggregatedCloseMeta(item, normalizeCloseSymbol(cloned.Pair), tradeDirection(cloned.IsShort))
	})
}

func (a *closeNoticeAggregator) add(key string, update func(*aggregatedCloseNotice)) {
	a.mu.Lock()
	item, ok := a.pending[key]
	if !ok {
		item = &aggregatedCloseNotice{Key: key}
		a.pending[key] = item
	}
	update(item)
	if item.isComplete() {
		timer := a.timers[key]
		delete(a.pending, key)
		delete(a.timers, key)
		a.mu.Unlock()
		if timer != nil {
			timer.Stop()
		}
		a.dispatch(*item)
		return
	}
	if _, ok := a.timers[key]; !ok {
		a.timers[key] = time.AfterFunc(a.window, func() {
			a.flush(key)
		})
	}
	a.mu.Unlock()
}

func (a *closeNoticeAggregator) flush(key string) {
	a.mu.Lock()
	item := a.pending[key]
	timer := a.timers[key]
	delete(a.pending, key)
	delete(a.timers, key)
	a.mu.Unlock()
	if timer != nil {
		timer.Stop()
	}
	if item == nil {
		return
	}
	a.dispatch(*item)
}

func (a *closeNoticeAggregator) dispatch(item aggregatedCloseNotice) {
	if a == nil || a.callback == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := a.callback(ctx, item); err != nil {
		logging.FromContext(ctx).Named("notify").Warn("aggregated close notify failed",
			zap.String("symbol", strings.TrimSpace(item.Symbol)),
			zap.String("direction", strings.TrimSpace(item.Direction)),
			zap.String("key", strings.TrimSpace(item.Key)),
			zap.Error(err),
		)
	}
}

func fillAggregatedCloseMeta(item *aggregatedCloseNotice, symbol string, direction string) {
	if item == nil {
		return
	}
	if strings.TrimSpace(item.Symbol) == "" {
		item.Symbol = strings.TrimSpace(symbol)
	}
	if strings.TrimSpace(item.Direction) == "" {
		item.Direction = strings.TrimSpace(direction)
	}
}

func normalizeCloseSymbol(pair string) string {
	pair = strings.TrimSpace(pair)
	if pair == "" {
		return ""
	}
	symbol := strings.ToUpper(pair)
	symbol = strings.TrimSuffix(symbol, ":USDT")
	symbol = strings.ReplaceAll(symbol, "/", "")
	if strings.TrimSpace(symbol) == "" {
		return pair
	}
	return symbol
}

func tradeDirection(isShort bool) string {
	if isShort {
		return "short"
	}
	return "long"
}

func closeAggregateKeyForPositionClose(notice PositionCloseNotice) string {
	return closeAggregateKey(notice.Symbol, notice.Direction, notice.ExecutorPositionID, notice.PositionID, 0, notice.EntryPrice, notice.TriggerPrice, notice.Qty)
}

func closeAggregateKeyForPositionCloseSummary(notice PositionCloseSummaryNotice) string {
	return closeAggregateKey(notice.Symbol, notice.Direction, notice.ExecutorPositionID, notice.PositionID, 0, notice.EntryPrice, notice.ExitPrice, notice.Qty)
}

func closeAggregateKeyForTradeCloseSummary(notice TradeCloseSummaryNotice) string {
	return closeAggregateKey(normalizeCloseSymbol(notice.Pair), tradeDirection(notice.IsShort), "", "", notice.TradeID, notice.OpenRate, notice.CloseRate, notice.Amount)
}

func closeAggregateKey(symbol string, direction string, executorPositionID string, positionID string, tradeID int, entry float64, exit float64, qty float64) string {
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		symbol = "unknown"
	}
	dir := strings.ToUpper(strings.TrimSpace(direction))
	if dir == "" {
		dir = "UNKNOWN"
	}
	if tradeID > 0 {
		return fmt.Sprintf("close_final:%s:%s:trade:%d", symbol, dir, tradeID)
	}
	if ext := strings.TrimSpace(executorPositionID); ext != "" {
		return fmt.Sprintf("close_final:%s:%s:trade:%s", symbol, dir, ext)
	}
	if pos := strings.TrimSpace(positionID); pos != "" {
		return fmt.Sprintf("close_final:%s:%s:position:%s", symbol, dir, pos)
	}
	return fmt.Sprintf("close_final:%s:%s:fallback:%s:%s:%s", symbol, dir, formatFloat(entry), formatFloat(exit), formatFloat(qty))
}
