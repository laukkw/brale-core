package runtime

import (
	"hash/fnv"
	"strings"
	"time"

	symbolpkg "brale-core/internal/pkg/symbol"
)

func newWebhookWorkerQueues(workers int, queueSize int) []chan WebhookEvent {
	if workers <= 0 {
		workers = 1
	}
	queues := make([]chan WebhookEvent, 0, workers)
	for i := 0; i < workers; i++ {
		queues = append(queues, make(chan WebhookEvent, queueSize))
	}
	return queues
}

func (s *WebhookSyncService) workerQueue(workerIdx int) chan WebhookEvent {
	if s == nil || len(s.workerQueues) == 0 {
		return nil
	}
	if workerIdx < 0 || workerIdx >= len(s.workerQueues) {
		return nil
	}
	return s.workerQueues[workerIdx]
}

func (s *WebhookSyncService) workerQueueForSymbol(symbol string) chan WebhookEvent {
	if s == nil || len(s.workerQueues) == 0 {
		return nil
	}
	if len(s.workerQueues) == 1 {
		return s.workerQueues[0]
	}
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(strings.ToUpper(strings.TrimSpace(symbol))))
	idx := int(hasher.Sum32() % uint32(len(s.workerQueues)))
	return s.workerQueues[idx]
}

func normalizeSymbol(raw string) string {
	return symbolpkg.Normalize(raw)
}

func (s *WebhookSyncService) now() int64 {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UnixMilli()
}
