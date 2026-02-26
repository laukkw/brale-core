// 本文件主要内容：提供运行时 API 的路由与响应构建。

package runtimeapi

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"brale-core/internal/execution"
	"brale-core/internal/market"
	"brale-core/internal/pkg/asyncjob"
	"brale-core/internal/position"
	"brale-core/internal/prompt/positionprompt"
	"brale-core/internal/runtime"
	"brale-core/internal/store"
)

const (
	requestIDHeader = "X-Request-Id"
)

type requestIDKey struct{}

type Scheduler interface {
	SetScheduledDecision(enable bool)
	GetScheduledDecision() bool
	GetScheduleStatus() runtime.ScheduleStatus
	SetSymbolMode(symbol string, mode runtime.SymbolMode) error
	SetMonitorSymbols(symbols []string) error
	ClearMonitorSymbols()
}

type ObserveRunner interface {
	RunOnceObserveWithResults(ctx context.Context, symbols []string, intervals []string, limit int, acct execution.AccountState, risk execution.RiskParams) ([]ObserveSymbolResult, error)
	RunOnceObserveWithInjectedPosition(ctx context.Context, symbol string, intervals []string, limit int, acct execution.AccountState, risk execution.RiskParams, pos positionprompt.Summary) (ObserveSymbolResult, error)
	RunOnceObserveAsFlat(ctx context.Context, symbols []string, intervals []string, limit int, acct execution.AccountState, risk execution.RiskParams) ([]ObserveSymbolResult, error)
}

type SymbolResolver interface {
	Resolve(symbol string) (ResolvedSymbol, error)
}

type ResolvedSymbol struct {
	Symbol      string
	Intervals   []string
	KlineLimit  int
	Pipeline    ObserveRunner
	RiskPercent float64
}

type runtimeExecClient interface {
	execution.BalanceReader
	execution.OpenTradesReader
	execution.TradesReader
}

type Server struct {
	Scheduler             Scheduler
	Resolver              SymbolResolver
	SymbolConfigs         map[string]ConfigBundle
	ObserveJobs           *asyncjob.Manager[observeResponse]
	Store                 store.Store
	ExecClient            runtimeExecClient
	PositionCache         *position.PositionCache
	PlanCache             *position.PlanCache
	PriceSource           market.PriceSource
	AllowSymbol           func(symbol string) bool
	NewsOverlayStaleAfter time.Duration
	lastMu                sync.RWMutex
	lastRun               map[string]lastObserve
}

func (s *Server) Handler() (http.Handler, error) {
	if s.Scheduler == nil {
		return nil, fmt.Errorf("scheduler is required")
	}
	if s.Resolver == nil {
		return nil, fmt.Errorf("resolver is required")
	}
	s.ensureObserveJobs()
	mux := http.NewServeMux()
	mux.Handle("/api/runtime/schedule/enable", http.HandlerFunc(s.handleScheduleEnable))
	mux.Handle("/api/runtime/schedule/disable", http.HandlerFunc(s.handleScheduleDisable))
	mux.Handle("/api/runtime/schedule/symbol", http.HandlerFunc(s.handleScheduleSymbol))
	mux.Handle("/api/runtime/schedule/status", http.HandlerFunc(s.handleScheduleStatus))
	mux.Handle("/api/runtime/monitor/status", http.HandlerFunc(s.handleMonitorStatus))
	mux.Handle("/api/runtime/position/status", http.HandlerFunc(s.handlePositionStatus))
	mux.Handle("/api/runtime/position/history", http.HandlerFunc(s.handleTradeHistory))
	mux.Handle("/api/runtime/decision/latest", http.HandlerFunc(s.handleDecisionLatest))
	mux.Handle("/api/runtime/news_overlay/latest", http.HandlerFunc(s.handleNewsOverlayLatest))
	mux.Handle("/api/observe/run", http.HandlerFunc(s.handleObserveRun))
	mux.Handle("/api/observe/report", http.HandlerFunc(s.handleObserveReport))
	mux.Handle("/api/debug/plan/inject", http.HandlerFunc(s.handleDebugPlanInject))
	mux.Handle("/api/debug/plan/status", http.HandlerFunc(s.handleDebugPlanStatus))
	mux.Handle("/api/debug/plan/clear", http.HandlerFunc(s.handleDebugPlanClear))
	return withCORS(withRequestID(mux)), nil
}

func (s *Server) ensureObserveJobs() {
	if s.ObserveJobs == nil {
		s.ObserveJobs = asyncjob.NewManager[observeResponse]()
	}
}
