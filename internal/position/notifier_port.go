package position

import (
	"context"

	"brale-core/internal/notifyport"
)

type Notifier interface {
	SendError(ctx context.Context, message string) error
	SendPositionClose(ctx context.Context, notice PositionCloseNotice) error
}

type PositionCloseNotice = notifyport.PositionCloseNotice
