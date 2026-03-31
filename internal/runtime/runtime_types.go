package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"

	"brale-core/internal/decision"
	"brale-core/internal/execution"
	llmapp "brale-core/internal/llm/app"
)

type TaskType string

const (
	TaskBarDecide    TaskType = "BAR_DECIDE"
	TaskPriceTick    TaskType = "PRICE_TICK"
	TaskReconcile    TaskType = "RECONCILE"
	TaskWebhookEvent TaskType = "WEBHOOK_EVENT"
)

type RuntimeTask struct {
	Type              TaskType
	Symbol            string
	EnqueuedAt        time.Time
	WebhookEventType  string
	WebhookTradeID    int
	WebhookTimestamp  int64
	WebhookExitReason string
}

type TaskDispatcher interface {
	Enqueue(task RuntimeTask) error
}

type AccountStateProvider interface {
	AccountState(ctx context.Context, symbol string) (execution.AccountState, error)
}

type Reconciler interface {
	RunOnce(ctx context.Context, symbol string) error
}

type RiskMonitor interface {
	RunOnce(ctx context.Context, symbol string) error
}

type WorkerPool[T any] struct {
	queue   chan T
	workers int
	handler func(T)
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func NewWorkerPool[T any](workers int, queueSize int, handler func(T)) *WorkerPool[T] {
	ctx, cancel := context.WithCancel(context.Background())
	return &WorkerPool[T]{
		queue:   make(chan T, queueSize),
		workers: workers,
		handler: handler,
		ctx:     ctx,
		cancel:  cancel,
	}
}

func (wp *WorkerPool[T]) Start() {
	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)
		go wp.worker()
	}
}

func (wp *WorkerPool[T]) Stop() {
	wp.cancel()
	wp.wg.Wait()
}

func (wp *WorkerPool[T]) Enqueue(task T) error {
	select {
	case wp.queue <- task:
		return nil
	default:
		return fmt.Errorf("worker pool queue full")
	}
}

func (wp *WorkerPool[T]) worker() {
	defer wp.wg.Done()
	for {
		select {
		case <-wp.ctx.Done():
			return
		case task := <-wp.queue:
			wp.handler(task)
		}
	}
}

type SymbolRuntime struct {
	Symbol          string
	Intervals       []string
	KlineLimit      int
	BarInterval     time.Duration
	RiskPerTradePct float64
	Enabled         decision.AgentEnabled
	LLMTracker      *llmapp.LLMRunTracker
	Pipeline        *decision.Pipeline
	Services        []RuntimeService
}

func (rt SymbolRuntime) StartServices(ctx context.Context) {
	for _, service := range rt.Services {
		if service == nil {
			continue
		}
		service.Start(ctx)
	}
}

func (rt SymbolRuntime) StopServices() {
	for _, service := range rt.Services {
		if service == nil {
			continue
		}
		service.Stop()
	}
}

type SymbolMode string

const (
	SymbolModeTrade   SymbolMode = "trade"
	SymbolModeObserve SymbolMode = "observe"
	SymbolModeOff     SymbolMode = "off"
)
