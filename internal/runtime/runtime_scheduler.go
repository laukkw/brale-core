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

	SyncOrderInterval time.Duration
	ReconcileInterval time.Duration
	PriceTickInterval time.Duration
	cancel            context.CancelFunc
	streamCtx         context.Context
	started           bool
	startMu           sync.Mutex
	lifecycleMu       sync.RWMutex

	Logger      *zap.Logger
	PriceStream interface {
		Start(ctx context.Context) error
		Close()
	}

	AccountFetcher func(ctx context.Context, symbol string) (execution.AccountState, error)

	EnableScheduledDecision bool
	mu                      sync.RWMutex

	symbolModes    map[string]SymbolMode
	monitorSymbols map[string]struct{}
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
	s.ensureSymbolModesLocked()
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
