package runtime

import (
	"errors"
	"sync"

	"brale-core/internal/config"
	"brale-core/internal/execution"
	"brale-core/internal/position"
	"brale-core/internal/transport/notify"
)

var ErrWebhookQueueFull = errors.New("webhook queue full")

type WebhookSyncService struct {
	Dispatcher  TaskDispatcher
	Queue       chan WebhookEvent
	WorkerCount int
	Now         func() int64
	AllowSymbol func(symbol string) bool
	ExecClient  webhookTradeClient
	Notifier    notify.Notifier
	PosCache    *position.PositionCache

	mu              sync.Mutex
	openNotified    map[int]int64
	lastExitOrderID map[int]exitNotifyState
	workerQueues    []chan WebhookEvent
}

type exitNotifyState struct {
	OrderID string
	At      int64
}

type webhookTradeClient interface {
	execution.OpenTradesReader
	execution.TradeFinder
}

type WebhookEvent struct {
	Type        string
	Symbol      string
	Timestamp   int64
	EnterTag    string
	TradeID     int
	Pair        string
	ExitReason  string
	CloseRate   float64
	Amount      float64
	StakeAmount float64
}

func NewWebhookSyncService(cfg config.WebhookConfig, dispatcher TaskDispatcher) *WebhookSyncService {
	queueSize := cfg.QueueSize
	if queueSize <= 0 {
		queueSize = 1024
	}
	workers := cfg.WorkerCount
	if workers <= 0 {
		workers = 4
	}
	return &WebhookSyncService{
		Dispatcher:      dispatcher,
		Queue:           make(chan WebhookEvent, queueSize),
		WorkerCount:     workers,
		openNotified:    make(map[int]int64),
		lastExitOrderID: make(map[int]exitNotifyState),
		workerQueues:    newWebhookWorkerQueues(workers, queueSize),
	}
}
