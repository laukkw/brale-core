// 本文件主要内容：调度 Bar/Price/Sync/Webhook 任务并保证 per-symbol 串行执行，支持动态开关定时决策，提供符号运行时构建与配置加载辅助。
package runtime

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"brale-core/internal/execution"
	"brale-core/internal/pkg/errclass"
	"brale-core/internal/pkg/logging"

	"go.uber.org/zap"
)

type RuntimeScheduler struct {
	Symbols      map[string]SymbolRuntime
	Policy       SchedulePolicy
	TaskExecutor RuntimeTaskExecutor

	dispatcher    TaskDispatcher
	barWorkers    []*WorkerPool[RuntimeTask]
	symbolWorkers map[string]*WorkerPool[RuntimeTask]

	Reconciler  Reconciler
	RiskMonitor RiskMonitor

	SyncOrderInterval   time.Duration
	ReconcileInterval   time.Duration
	PriceTickInterval   time.Duration
	NewsOverlayInterval time.Duration
	cancel              context.CancelFunc
	streamCtx           context.Context
	started             bool
	startMu             sync.Mutex
	lifecycleMu         sync.RWMutex

	Logger      *zap.Logger
	PriceStream interface {
		Start(ctx context.Context) error
		Close()
	}

	AccountFetcher     func(ctx context.Context, symbol string) (execution.AccountState, error)
	NewsOverlayUpdater interface {
		RunOnce(ctx context.Context) error
	}

	EnableScheduledDecision bool
	mu                      sync.RWMutex

	symbolModes    map[string]SymbolMode
	monitorSymbols map[string]struct{}
}

func (s *RuntimeScheduler) SetScheduledDecision(enable bool) {
	var shouldStart bool
	var shouldStop bool
	var streamCtx context.Context

	s.mu.Lock()
	s.EnableScheduledDecision = enable
	if s.symbolModes == nil {
		s.symbolModes = make(map[string]SymbolMode, len(s.Symbols))
	}
	for symbol := range s.Symbols {
		mode, ok := s.symbolModes[symbol]
		if !ok {
			mode = SymbolModeTrade
		}
		if mode == SymbolModeOff {
			s.symbolModes[symbol] = mode
			continue
		}
		if enable {
			s.symbolModes[symbol] = SymbolModeTrade
			continue
		}
		s.symbolModes[symbol] = SymbolModeObserve
	}
	started, currentStreamCtx := s.lifecycleSnapshot()
	if started {
		if enable {
			if !s.allSymbolsIdleLocked() {
				shouldStart = true
				streamCtx = currentStreamCtx
			}
		} else if len(s.monitorSymbols) == 0 {
			shouldStop = true
		}
	}
	s.mu.Unlock()
	if shouldStart {
		s.startPriceStream(streamCtx)
	}
	if shouldStop {
		s.stopPriceStream()
	}
}

func (s *RuntimeScheduler) SetMonitorSymbols(symbols []string) error {
	if s == nil {
		return fmt.Errorf("scheduler is nil")
	}
	var shouldStart bool
	var streamCtx context.Context

	s.mu.Lock()
	if s.monitorSymbols == nil {
		s.monitorSymbols = make(map[string]struct{}, len(symbols))
	} else {
		for symbol := range s.monitorSymbols {
			delete(s.monitorSymbols, symbol)
		}
	}
	for _, symbol := range symbols {
		if symbol == "" {
			continue
		}
		if _, ok := s.Symbols[symbol]; !ok {
			s.mu.Unlock()
			return fmt.Errorf("symbol %s not found", symbol)
		}
		s.monitorSymbols[symbol] = struct{}{}
	}
	started, currentStreamCtx := s.lifecycleSnapshot()
	if started && len(s.monitorSymbols) > 0 {
		shouldStart = true
		streamCtx = currentStreamCtx
	}
	s.mu.Unlock()
	if shouldStart {
		s.startPriceStream(streamCtx)
	}
	return nil
}

func (s *RuntimeScheduler) ClearMonitorSymbols() {
	var shouldStop bool

	s.mu.Lock()
	if s.monitorSymbols == nil {
		s.mu.Unlock()
		return
	}
	for symbol := range s.monitorSymbols {
		delete(s.monitorSymbols, symbol)
	}
	started, _ := s.lifecycleSnapshot()
	if started && !s.EnableScheduledDecision {
		shouldStop = true
	}
	s.mu.Unlock()
	if shouldStop {
		s.stopPriceStream()
	}
}

func (s *RuntimeScheduler) GetScheduledDecision() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.EnableScheduledDecision
}

func (s *RuntimeScheduler) SetSymbolMode(symbol string, mode SymbolMode) error {
	if s == nil {
		return fmt.Errorf("scheduler is nil")
	}
	var shouldStart bool
	var shouldStop bool
	var streamCtx context.Context

	s.mu.Lock()
	if s.symbolModes == nil {
		s.symbolModes = make(map[string]SymbolMode)
	}
	if _, ok := s.Symbols[symbol]; !ok {
		s.mu.Unlock()
		return fmt.Errorf("symbol %s not found", symbol)
	}
	s.symbolModes[symbol] = mode
	started, currentStreamCtx := s.lifecycleSnapshot()
	if started {
		if mode == SymbolModeOff || mode == SymbolModeObserve {
			// If all symbols are non-trade, we can stop the price stream.
			if s.allSymbolsIdleLocked() && len(s.monitorSymbols) == 0 {
				shouldStop = true
			}
		} else if s.EnableScheduledDecision || len(s.monitorSymbols) > 0 {
			shouldStart = true
			streamCtx = currentStreamCtx
		}
	}
	s.mu.Unlock()
	if shouldStart {
		s.startPriceStream(streamCtx)
	}
	if shouldStop {
		s.stopPriceStream()
	}
	return nil
}

func (s *RuntimeScheduler) getSymbolMode(symbol string) SymbolMode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.symbolModes == nil {
		return SymbolModeTrade
	}
	mode, ok := s.symbolModes[symbol]
	if !ok {
		return SymbolModeTrade
	}
	return mode
}

func (s *RuntimeScheduler) isSymbolMonitored(symbol string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.monitorSymbols == nil {
		return false
	}
	_, ok := s.monitorSymbols[symbol]
	return ok
}

func (s *RuntimeScheduler) allSymbolsIdleLocked() bool {
	if s.symbolModes == nil {
		return false
	}
	for sym := range s.Symbols {
		mode := s.symbolModes[sym]
		if mode == SymbolModeTrade {
			return false
		}
	}
	return true
}

func (s *RuntimeScheduler) startPriceStream(ctx context.Context) {
	if s.PriceStream == nil || ctx == nil {
		return
	}
	if err := s.PriceStream.Start(ctx); err != nil && s.Logger != nil {
		s.Logger.Warn("price stream start failed", zap.Error(err))
	}
}

func (s *RuntimeScheduler) stopPriceStream() {
	if s.PriceStream == nil {
		return
	}
	s.PriceStream.Close()
}

func (s *RuntimeScheduler) Start(ctx context.Context) error {
	s.startMu.Lock()
	defer s.startMu.Unlock()
	if started, _ := s.lifecycleSnapshot(); started {
		return fmt.Errorf("scheduler already started")
	}
	if err := s.validate(); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if s.Logger == nil {
		s.Logger = logging.FromContext(ctx).Named("scheduler")
	}
	if s.TaskExecutor == nil {
		s.TaskExecutor = defaultRuntimeTaskExecutor{}
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.symbolWorkers = make(map[string]*WorkerPool[RuntimeTask], len(s.Symbols))
	s.barWorkers = make([]*WorkerPool[RuntimeTask], 0, len(s.Symbols))
	s.mu.Lock()
	if s.symbolModes == nil {
		s.symbolModes = make(map[string]SymbolMode, len(s.Symbols))
	}
	for symbol := range s.Symbols {
		if _, ok := s.symbolModes[symbol]; !ok {
			s.symbolModes[symbol] = SymbolModeTrade
		}
	}
	s.mu.Unlock()
	for symbol := range s.Symbols {
		worker := NewWorkerPool(1, 256, func(task RuntimeTask) {
			s.handleTask(runCtx, task)
		})
		s.symbolWorkers[symbol] = worker
		s.barWorkers = append(s.barWorkers, worker)
		worker.Start()
	}
	s.dispatcher = s
	if s.PriceTickInterval <= 0 {
		s.PriceTickInterval = time.Second
	}
	s.setLifecycle(true, runCtx, cancel)
	if s.EnableScheduledDecision {
		s.startPriceStream(runCtx)
	}
	s.startLoops(runCtx)
	return nil
}

func (s *RuntimeScheduler) Stop() {
	s.startMu.Lock()
	defer s.startMu.Unlock()
	started, _, cancel := s.lifecycleStopSnapshot()
	if !started {
		return
	}
	if cancel != nil {
		cancel()
	}
	s.stopPriceStream()
	for _, worker := range s.barWorkers {
		worker.Stop()
	}
}

func (s *RuntimeScheduler) Enqueue(task RuntimeTask) error {
	worker, ok := s.symbolWorkers[task.Symbol]
	if !ok {
		return fmt.Errorf("symbol worker not found")
	}
	return worker.Enqueue(task)
}

func (s *RuntimeScheduler) startLoops(ctx context.Context) {
	if s.NewsOverlayUpdater != nil && s.NewsOverlayInterval > 0 {
		go s.newsOverlayLoop(ctx)
	}
	for symbol, rt := range s.Symbols {
		interval := rt.BarInterval
		if interval <= 0 {
			continue
		}
		go s.barLoop(ctx, symbol, interval)
	}
	if s.PriceTickInterval > 0 {
		go s.priceTickLoop(ctx)
	}
	if s.ReconcileInterval > 0 {
		go s.reconcileLoop(ctx)
	}
}

func (s *RuntimeScheduler) newsOverlayLoop(ctx context.Context) {
	run := func() {
		if s.NewsOverlayUpdater == nil {
			return
		}
		if err := s.NewsOverlayUpdater.RunOnce(ctx); err != nil {
			if s.Logger != nil {
				s.Logger.Warn("news overlay update failed", zap.Error(err))
			}
			return
		}
		if s.Logger != nil {
			s.Logger.Debug("news overlay updated")
		}
	}
	run()
	ticker := time.NewTicker(s.NewsOverlayInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func (s *RuntimeScheduler) barLoop(ctx context.Context, symbol string, interval time.Duration) {
	policy := s.policy()
	for {
		next := nextBarClose(time.Now(), interval)
		wait := time.Until(next)
		if wait < 0 {
			wait = 0
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		scheduled := s.GetScheduledDecision()
		mode := s.getSymbolMode(symbol)
		if !policy.ShouldEnqueueBar(scheduled, mode) {
			continue
		}
		if err := s.Enqueue(RuntimeTask{Type: TaskBarDecide, Symbol: symbol, EnqueuedAt: time.Now()}); err != nil {
			if s.Logger != nil {
				s.Logger.Warn("enqueue bar task failed", zap.String("symbol", symbol), zap.Error(err))
			}
		}
	}
}

func (s *RuntimeScheduler) priceTickLoop(ctx context.Context) {
	if s.RiskMonitor == nil {
		return
	}
	policy := s.policy()
	ticker := time.NewTicker(s.PriceTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			scheduled := s.GetScheduledDecision()
			for symbol := range s.Symbols {
				mode := s.getSymbolMode(symbol)
				monitored := s.isSymbolMonitored(symbol)
				if !policy.ShouldEnqueuePeriodic(scheduled, mode, monitored) {
					continue
				}
				if err := s.Enqueue(RuntimeTask{Type: TaskPriceTick, Symbol: symbol, EnqueuedAt: time.Now()}); err != nil {
					if s.Logger != nil {
						s.Logger.Warn("enqueue price tick failed", zap.String("symbol", symbol), zap.Error(err))
					}
				}
			}
		}
	}
}

func (s *RuntimeScheduler) reconcileLoop(ctx context.Context) {
	if s.Reconciler == nil {
		return
	}
	policy := s.policy()
	ticker := time.NewTicker(s.ReconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			scheduled := s.GetScheduledDecision()
			for symbol := range s.Symbols {
				mode := s.getSymbolMode(symbol)
				monitored := s.isSymbolMonitored(symbol)
				if !policy.ShouldEnqueuePeriodic(scheduled, mode, monitored) {
					continue
				}
				if err := s.Enqueue(RuntimeTask{Type: TaskReconcile, Symbol: symbol, EnqueuedAt: time.Now()}); err != nil {
					if s.Logger != nil {
						s.Logger.Warn("enqueue reconcile task failed", zap.String("symbol", symbol), zap.Error(err))
					}
				}
			}
		}
	}
}

func (s *RuntimeScheduler) handleTask(ctx context.Context, task RuntimeTask) {
	s.taskExecutor().Execute(ctx, s, task)
}

func (s *RuntimeScheduler) validate() error {
	if s == nil {
		return runtimeValidationErrorf("scheduler is required")
	}
	if len(s.Symbols) == 0 {
		return runtimeValidationErrorf("symbols is required")
	}
	return nil
}

type runtimeValidationError struct {
	msg string
}

func (e runtimeValidationError) Error() string {
	return e.msg
}

func (e runtimeValidationError) Classification() errclass.Classification {
	return errclass.Classification{
		Kind:      "validation",
		Scope:     "runtime",
		Retryable: false,
		Action:    "abort",
		Reason:    "invalid_scheduler",
	}
}

func runtimeValidationErrorf(format string, args ...any) error {
	return runtimeValidationError{msg: fmt.Sprintf(format, args...)}
}

type ScheduleStatus struct {
	IsScheduled bool            `json:"is_scheduled"`
	NextRuns    []SymbolNextRun `json:"next_runs"`
	Details     string          `json:"details"`
}

type SymbolNextRun struct {
	Symbol        string `json:"symbol"`
	NextExecution string `json:"next_execution"`
	Waiting       string `json:"waiting"`
	BarInterval   string `json:"bar_interval"`
	LastBarTime   int64  `json:"last_bar_time"`
	Details       string `json:"details"`
	Mode          string `json:"mode"`
}

func nextBarClose(now time.Time, interval time.Duration) time.Time {
	next := now.Truncate(interval).Add(interval)
	return next.Add(10 * time.Second)
}

func (s *RuntimeScheduler) GetScheduleStatus() ScheduleStatus {
	s.mu.RLock()
	scheduled := s.EnableScheduledDecision
	s.mu.RUnlock()
	policy := s.policy()

	now := time.Now()
	keys := make([]string, 0, len(s.Symbols))
	for sym := range s.Symbols {
		keys = append(keys, sym)
	}
	sort.Strings(keys)
	results := make([]SymbolNextRun, 0, len(keys))
	for _, sym := range keys {
		rt := s.Symbols[sym]
		mode := s.getSymbolMode(sym)
		interval := rt.BarInterval
		if interval <= 0 {
			continue
		}
		monitored := s.isSymbolMonitored(sym)
		nextExecution, waiting, details := policy.DescribeSymbolStatus(scheduled, mode, monitored, now, interval)
		results = append(results, SymbolNextRun{
			Symbol:        sym,
			NextExecution: nextExecution,
			Waiting:       waiting,
			BarInterval:   interval.String(),
			LastBarTime:   0,
			Details:       details,
			Mode:          string(mode),
		})
	}

	monitoredCount := 0
	for _, sym := range keys {
		if s.isSymbolMonitored(sym) {
			monitoredCount++
		}
	}
	summary := policy.Summary(scheduled, len(results), monitoredCount)

	return ScheduleStatus{
		IsScheduled: scheduled,
		NextRuns:    results,
		Details:     summary,
	}
}

func (s *RuntimeScheduler) lifecycleSnapshot() (bool, context.Context) {
	s.lifecycleMu.RLock()
	defer s.lifecycleMu.RUnlock()
	return s.started, s.streamCtx
}

func (s *RuntimeScheduler) setLifecycle(started bool, streamCtx context.Context, cancel context.CancelFunc) {
	s.lifecycleMu.Lock()
	s.started = started
	s.streamCtx = streamCtx
	s.cancel = cancel
	s.lifecycleMu.Unlock()
}

func (s *RuntimeScheduler) lifecycleStopSnapshot() (bool, context.Context, context.CancelFunc) {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	started := s.started
	streamCtx := s.streamCtx
	cancel := s.cancel
	s.started = false
	s.streamCtx = nil
	s.cancel = nil
	return started, streamCtx, cancel
}
