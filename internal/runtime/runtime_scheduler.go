package runtime

import (
	"context"
	"fmt"
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

	SyncOrderInterval  time.Duration
	ReconcileInterval  time.Duration
	PriceTickInterval  time.Duration
	DisableTickerLoops bool
	cancel             context.CancelFunc
	streamCtx          context.Context
	started            bool
	startMu            sync.Mutex
	lifecycleMu        sync.RWMutex

	Logger      *zap.Logger
	PriceStream interface {
		Start(ctx context.Context) error
		Close()
	}
	LiquidationStream interface {
		Start(ctx context.Context) error
		Close()
	}

	AccountFetcher func(ctx context.Context, symbol string) (execution.AccountState, error)

	EnableScheduledDecision bool
	mu                      sync.RWMutex

	symbolModes    map[string]SymbolMode
	monitorSymbols map[string]struct{}

	taskStateMu         sync.Mutex
	pendingTaskKeys     map[string]struct{}
	droppedTaskCounts   map[string]int64
	coalescedTaskCounts map[string]int64
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
	s.pendingTaskKeys = make(map[string]struct{}, len(s.Symbols)*3)
	s.droppedTaskCounts = make(map[string]int64, len(s.Symbols)*3)
	s.coalescedTaskCounts = make(map[string]int64, len(s.Symbols)*3)
	s.mu.Lock()
	s.ensureSymbolModesLocked()
	shouldStartStream := s.shouldPriceStreamBeRunningLocked()
	s.mu.Unlock()
	for symbol := range s.Symbols {
		s.Symbols[symbol].StartServices(runCtx)
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
	if shouldStartStream {
		s.startPriceStream(runCtx)
		s.startLiquidationStream(runCtx)
	}
	if !s.DisableTickerLoops {
		s.startLoops(runCtx)
	}
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
	for _, rt := range s.Symbols {
		rt.StopServices()
	}
	s.stopPriceStream()
	s.stopLiquidationStream()
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

func (s *RuntimeScheduler) handleTask(ctx context.Context, task RuntimeTask) {
	defer s.clearPendingTask(task)
	defer func() {
		if r := recover(); r != nil {
			logging.FromContext(ctx).Named("scheduler").Error("task panic recovered",
				zap.String("symbol", task.Symbol),
				zap.String("type", string(task.Type)),
				zap.Any("panic", r),
				zap.Stack("stack"),
			)
		}
	}()
	s.taskExecutor().Execute(ctx, s, task)
}

func (s *RuntimeScheduler) shouldCoalesceTask(task RuntimeTask) bool {
	switch task.Type {
	case TaskBarDecide, TaskPriceTick, TaskReconcile:
		return true
	default:
		return false
	}
}

func (s *RuntimeScheduler) taskStateKey(task RuntimeTask) string {
	return task.Symbol + "|" + string(task.Type)
}

func (s *RuntimeScheduler) markTaskPending(task RuntimeTask) bool {
	if s == nil || !s.shouldCoalesceTask(task) {
		return true
	}
	key := s.taskStateKey(task)
	s.taskStateMu.Lock()
	defer s.taskStateMu.Unlock()
	if s.pendingTaskKeys == nil {
		s.pendingTaskKeys = make(map[string]struct{})
	}
	if _, exists := s.pendingTaskKeys[key]; exists {
		return false
	}
	s.pendingTaskKeys[key] = struct{}{}
	return true
}

func (s *RuntimeScheduler) clearPendingTask(task RuntimeTask) {
	if s == nil || !s.shouldCoalesceTask(task) {
		return
	}
	key := s.taskStateKey(task)
	s.taskStateMu.Lock()
	defer s.taskStateMu.Unlock()
	delete(s.pendingTaskKeys, key)
}

func (s *RuntimeScheduler) recordDroppedTask(task RuntimeTask) int64 {
	if s == nil {
		return 0
	}
	key := s.taskStateKey(task)
	s.taskStateMu.Lock()
	defer s.taskStateMu.Unlock()
	if s.droppedTaskCounts == nil {
		s.droppedTaskCounts = make(map[string]int64)
	}
	s.droppedTaskCounts[key]++
	return s.droppedTaskCounts[key]
}

func (s *RuntimeScheduler) resetDroppedTaskCount(task RuntimeTask) {
	if s == nil {
		return
	}
	key := s.taskStateKey(task)
	s.taskStateMu.Lock()
	defer s.taskStateMu.Unlock()
	delete(s.droppedTaskCounts, key)
}

func (s *RuntimeScheduler) recordCoalescedTask(task RuntimeTask) int64 {
	if s == nil {
		return 0
	}
	key := s.taskStateKey(task)
	s.taskStateMu.Lock()
	defer s.taskStateMu.Unlock()
	if s.coalescedTaskCounts == nil {
		s.coalescedTaskCounts = make(map[string]int64)
	}
	s.coalescedTaskCounts[key]++
	return s.coalescedTaskCounts[key]
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
