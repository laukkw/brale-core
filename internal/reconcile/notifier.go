package reconcile

import (
	"context"

	"brale-core/internal/notifyport"
)

type Notifier interface {
	SendPositionOpen(ctx context.Context, notice PositionOpenNotice) error
	SendPositionCloseSummary(ctx context.Context, notice PositionCloseSummaryNotice) error
	SendError(ctx context.Context, message string) error
}

type PositionOpenNotice = notifyport.PositionOpenNotice

type PositionCloseSummaryNotice = notifyport.PositionCloseSummaryNotice

type NopNotifier struct{}

func (NopNotifier) SendPositionOpen(ctx context.Context, notice PositionOpenNotice) error {
	return nil
}

func (NopNotifier) SendPositionCloseSummary(ctx context.Context, notice PositionCloseSummaryNotice) error {
	return nil
}

func (NopNotifier) SendError(ctx context.Context, message string) error {
	return nil
}
