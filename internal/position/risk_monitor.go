// 本文件主要内容：基于 mark price 触发止盈止损并下发平仓意图。
package position

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"brale-core/internal/execution"
	"brale-core/internal/market"
	"brale-core/internal/pkg/errclass"
	"brale-core/internal/store"
)

type RiskMonitor struct {
	Store          store.Store
	PriceSource    market.PriceSource
	Positions      *PositionService
	PlanCache      *PlanCache
	AccountFetcher func(ctx context.Context, symbol string) (execution.AccountState, error)

	mu              sync.RWMutex
	accountBySymbol map[string]cachedAccountState
	statusByTradeID map[string]cachedStatusAmount
}

type cachedAccountState struct {
	state     execution.AccountState
	fetchedAt time.Time
}

type cachedStatusAmount struct {
	qty       float64
	ok        bool
	fetchedAt time.Time
}

type riskMonitorOpError struct {
	Op     string
	Symbol string
	Err    error
}

func (e *riskMonitorOpError) Error() string {
	if e == nil {
		return "<nil>"
	}
	op := strings.TrimSpace(e.Op)
	symbol := strings.ToUpper(strings.TrimSpace(e.Symbol))
	if symbol != "" {
		return fmt.Sprintf("risk monitor %s: symbol=%s: %v", op, symbol, e.Err)
	}
	return fmt.Sprintf("risk monitor %s: %v", op, e.Err)
}

func (e *riskMonitorOpError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (m *RiskMonitor) RunOnce(ctx context.Context, symbol string) error {
	if err := m.validate(); err != nil {
		return &riskMonitorOpError{Op: "validate", Symbol: symbol, Err: err}
	}
	if err := m.handleEntryArmed(ctx, symbol); err != nil {
		return &riskMonitorOpError{Op: "handle entry armed", Symbol: symbol, Err: err}
	}
	positions, err := m.Store.ListPositionsByStatus(ctx, []string{PositionOpenActive})
	if err != nil {
		return &riskMonitorOpError{Op: "list open positions", Symbol: symbol, Err: err}
	}
	for _, pos := range positions {
		if symbol != "" && !strings.EqualFold(pos.Symbol, symbol) {
			continue
		}
		if err := m.handleActivePosition(ctx, pos); err != nil {
			return &riskMonitorOpError{Op: "handle active position", Symbol: pos.Symbol, Err: err}
		}
	}
	return nil
}

func (m *RiskMonitor) handleEntryArmed(ctx context.Context, symbol string) error {
	if m.PlanCache == nil {
		return nil
	}
	if strings.TrimSpace(symbol) != "" {
		sym := strings.TrimSpace(symbol)
		if err := m.handlePlanEntry(ctx, sym); err != nil {
			return &riskMonitorOpError{Op: "handle plan entry", Symbol: sym, Err: err}
		}
		return nil
	}
	entries := m.PlanCache.ListEntries()
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		sym := strings.TrimSpace(entry.Plan.Symbol)
		if sym == "" {
			continue
		}
		if _, ok := seen[sym]; ok {
			continue
		}
		seen[sym] = struct{}{}
		if err := m.handlePlanEntry(ctx, sym); err != nil {
			return &riskMonitorOpError{Op: "handle plan entry", Symbol: sym, Err: err}
		}
	}
	return nil
}

func (m *RiskMonitor) validate() error {
	if m.Store == nil || m.PriceSource == nil || m.Positions == nil || m.PlanCache == nil {
		return riskValidationErrorf("store/price_source/positions/plan_cache is required")
	}
	return nil
}

type riskValidationError struct {
	msg string
}

func (e riskValidationError) Error() string {
	return e.msg
}

func (e riskValidationError) Classification() errclass.Classification {
	return errclass.Classification{
		Kind:      "validation",
		Scope:     "risk",
		Retryable: false,
		Action:    "abort",
		Reason:    "invalid_risk_monitor",
	}
}

func riskValidationErrorf(format string, args ...any) error {
	return riskValidationError{msg: fmt.Sprintf(format, args...)}
}
