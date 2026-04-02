package position

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"brale-core/internal/execution"
)

func fetchFreqtradeStatusAmount(ctx context.Context, executor execution.Executor, tradeID string) (float64, bool, error) {
	type openTradesExecutor interface {
		ListOpenTrades(ctx context.Context) ([]execution.Trade, error)
	}
	lister, ok := executor.(openTradesExecutor)
	if !ok || lister == nil {
		return 0, false, nil
	}
	tradeID = strings.TrimSpace(tradeID)
	if tradeID == "" {
		return 0, false, nil
	}
	parsedID, err := strconv.Atoi(tradeID)
	if err != nil || parsedID <= 0 {
		return 0, false, nil
	}
	trades, err := lister.ListOpenTrades(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("list open trades for trade_id %d: %w", parsedID, err)
	}
	for _, tr := range trades {
		if tr.ID == parsedID {
			amount := float64(tr.Amount)
			if amount > 0 {
				return amount, true, nil
			}
			return 0, false, nil
		}
	}
	return 0, false, nil
}

func (m *RiskMonitor) fetchStatusAmount(ctx context.Context, tradeID string) (float64, bool, error) {
	now := time.Now()
	if qty, ok := m.cachedFreshStatusAmount(tradeID, now); ok {
		return qty, true, nil
	}
	statusCtx, cancel := riskMonitorChildTimeout(ctx, riskMonitorStatusFetchTimeout)
	qty, ok, err := fetchFreqtradeStatusAmount(statusCtx, m.Positions.Executor, tradeID)
	cancel()
	if err != nil {
		if staleQty, staleOK := m.cachedStaleStatusAmount(tradeID, now); staleOK {
			return staleQty, true, nil
		}
		return 0, false, err
	}
	m.cacheStatusAmount(tradeID, qty, ok, now)
	return qty, ok, nil
}

func (m *RiskMonitor) cachedFreshStatusAmount(tradeID string, now time.Time) (float64, bool) {
	return m.cachedStatusAmount(tradeID, now, riskMonitorStatusFreshTTL)
}

func (m *RiskMonitor) cachedStaleStatusAmount(tradeID string, now time.Time) (float64, bool) {
	return m.cachedStatusAmount(tradeID, now, riskMonitorStatusStaleTTL)
}

func (m *RiskMonitor) cachedStatusAmount(tradeID string, now time.Time, ttl time.Duration) (float64, bool) {
	if m == nil || ttl <= 0 {
		return 0, false
	}
	key := strings.TrimSpace(tradeID)
	if key == "" {
		return 0, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.statusByTradeID[key]
	if !ok || entry.fetchedAt.IsZero() || now.Sub(entry.fetchedAt) > ttl || !entry.ok {
		return 0, false
	}
	return entry.qty, true
}

func (m *RiskMonitor) cacheStatusAmount(tradeID string, qty float64, ok bool, now time.Time) {
	key := strings.TrimSpace(tradeID)
	if m == nil || key == "" || now.IsZero() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.statusByTradeID == nil {
		m.statusByTradeID = make(map[string]cachedStatusAmount)
	}
	m.statusByTradeID[key] = cachedStatusAmount{qty: qty, ok: ok, fetchedAt: now}
}
