package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"brale-core/internal/pkg/logging"

	"go.uber.org/zap"
)

func (s *WebhookSyncService) Start(ctx context.Context) {
	if s == nil {
		return
	}
	go s.dispatch(ctx)
	for i := 0; i < s.WorkerCount; i++ {
		go s.worker(ctx, s.workerQueue(i))
	}
}

func (s *WebhookSyncService) dispatch(ctx context.Context) {
	if s == nil || s.Queue == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-s.Queue:
			if !ok {
				return
			}
			queue := s.workerQueueForSymbol(evt.Symbol)
			if queue == nil {
				if err := s.process(ctx, evt); err != nil {
					logging.FromContext(ctx).Named("webhook").Warn("webhook event process failed", zap.Error(err))
				}
				continue
			}
			select {
			case <-ctx.Done():
				return
			case queue <- evt:
			}
		}
	}
}

func (s *WebhookSyncService) worker(ctx context.Context, queue <-chan WebhookEvent) {
	if s == nil || s.Dispatcher == nil || queue == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-queue:
			if !ok {
				return
			}
			if err := s.process(ctx, evt); err != nil {
				logging.FromContext(ctx).Named("webhook").Warn("webhook event process failed", zap.Error(err))
			}
		}
	}
}

func (s *WebhookSyncService) enqueue(evt WebhookEvent) error {
	select {
	case s.Queue <- evt:
		return nil
	default:
		return ErrWebhookQueueFull
	}
}

func (s *WebhookSyncService) process(ctx context.Context, evt WebhookEvent) error {
	if s.Dispatcher == nil {
		return fmt.Errorf("webhook dispatcher is required")
	}
	if strings.TrimSpace(evt.Symbol) == "" {
		return fmt.Errorf("symbol is required")
	}
	task := RuntimeTask{
		Type:              TaskWebhookEvent,
		Symbol:            evt.Symbol,
		EnqueuedAt:        time.Now(),
		WebhookEventType:  evt.Type,
		WebhookTradeID:    evt.TradeID,
		WebhookTimestamp:  evt.Timestamp,
		WebhookExitReason: evt.ExitReason,
	}
	if err := s.Dispatcher.Enqueue(task); err != nil {
		return err
	}
	s.notify(ctx, evt)
	return nil
}
