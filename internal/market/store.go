package market

import (
	"context"

	"brale-core/internal/snapshot"
)

type KlineStore interface {
	Get(ctx context.Context, symbol, interval string) ([]snapshot.Candle, error)
	Set(ctx context.Context, symbol, interval string, klines []snapshot.Candle) error
	Put(ctx context.Context, symbol, interval string, klines []snapshot.Candle, max int) error
}
