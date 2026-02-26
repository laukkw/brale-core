package reconcile

import (
	"context"

	"brale-core/internal/execution"
)

type OrderStatusFetcher interface {
	Fetch(ctx context.Context, externalOrderID string) (execution.OrderStatus, error)
}
