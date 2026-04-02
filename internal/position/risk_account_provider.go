package position

import (
	"context"
	"fmt"
	"strings"
	"time"

	"brale-core/internal/execution"

	"go.uber.org/zap"
)

const (
	riskMonitorPriceFetchTimeout  = 1200 * time.Millisecond
	riskMonitorStatusFetchTimeout = 1200 * time.Millisecond
	riskMonitorAccountTimeout     = 1200 * time.Millisecond
	riskMonitorAccountFreshTTL    = 3 * time.Second
	riskMonitorAccountStaleTTL    = 15 * time.Second
	riskMonitorStatusFreshTTL     = 3 * time.Second
	riskMonitorStatusStaleTTL     = 15 * time.Second
)

func riskMonitorChildTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

func (m *RiskMonitor) fetchAccountState(ctx context.Context, planSymbol string, logger *zap.Logger) (execution.AccountState, error) {
	if m.AccountFetcher == nil {
		return execution.AccountState{}, riskValidationErrorf("account_fetcher is required")
	}
	now := time.Now()
	if acct, ok := m.cachedFreshAccount(planSymbol, now); ok {
		return acct, nil
	}
	accountCtx, cancel := riskMonitorChildTimeout(ctx, riskMonitorAccountTimeout)
	acct, err := m.AccountFetcher(accountCtx, planSymbol)
	cancel()
	if err != nil {
		if stale, ok := m.cachedStaleAccount(planSymbol, now); ok {
			logger.Warn("account balance degraded, using stale cache", zap.Error(err), zap.String("symbol", planSymbol))
			return stale, nil
		}
		logger.Warn("account balance unavailable", zap.Error(err))
		return execution.AccountState{}, fmt.Errorf("fetch account state for %s: %w", planSymbol, err)
	}
	if acct.Available <= 0 {
		if stale, ok := m.cachedStaleAccount(planSymbol, now); ok {
			logger.Warn("account available invalid, using stale cache", zap.Float64("available", acct.Available), zap.String("currency", strings.TrimSpace(acct.Currency)))
			return stale, nil
		}
		logger.Warn("account available balance invalid", zap.Float64("available", acct.Available), zap.String("currency", strings.TrimSpace(acct.Currency)))
		return execution.AccountState{}, riskValidationErrorf("account available balance unavailable")
	}
	m.cacheAccount(planSymbol, acct, now)
	return acct, nil
}

func (m *RiskMonitor) cachedFreshAccount(symbol string, now time.Time) (execution.AccountState, bool) {
	return m.cachedAccount(symbol, now, riskMonitorAccountFreshTTL)
}

func (m *RiskMonitor) cachedStaleAccount(symbol string, now time.Time) (execution.AccountState, bool) {
	return m.cachedAccount(symbol, now, riskMonitorAccountStaleTTL)
}

func (m *RiskMonitor) cachedAccount(symbol string, now time.Time, ttl time.Duration) (execution.AccountState, bool) {
	if m == nil || ttl <= 0 {
		return execution.AccountState{}, false
	}
	key := strings.TrimSpace(symbol)
	if key == "" {
		return execution.AccountState{}, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.accountBySymbol[key]
	if !ok || entry.fetchedAt.IsZero() || now.Sub(entry.fetchedAt) > ttl {
		return execution.AccountState{}, false
	}
	return entry.state, true
}

func (m *RiskMonitor) cacheAccount(symbol string, acct execution.AccountState, now time.Time) {
	key := strings.TrimSpace(symbol)
	if m == nil || key == "" || now.IsZero() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.accountBySymbol == nil {
		m.accountBySymbol = make(map[string]cachedAccountState)
	}
	m.accountBySymbol[key] = cachedAccountState{state: acct, fetchedAt: now}
}
