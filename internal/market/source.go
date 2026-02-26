package market

import "context"

type OpenInterestPoint struct {
	Symbol               string  `json:"symbol"`
	SumOpenInterest      float64 `json:"sumOpenInterest"`
	SumOpenInterestValue float64 `json:"sumOpenInterestValue"`
	Timestamp            int64   `json:"timestamp"`
}

type LongShortRatioPoint struct {
	Timestamp int64
	Ratio     float64
	Long      float64
	Short     float64
}

type LongShortRatioProvider interface {
	TopPositionRatio(ctx context.Context, symbol, period string, limit int) ([]LongShortRatioPoint, error)
	TopAccountRatio(ctx context.Context, symbol, period string, limit int) ([]LongShortRatioPoint, error)
	GlobalAccountRatio(ctx context.Context, symbol, period string, limit int) ([]LongShortRatioPoint, error)
}

type Source interface {
	GetFundingRate(ctx context.Context, symbol string) (float64, error)
	GetOpenInterestHistory(ctx context.Context, symbol, period string, limit int) ([]OpenInterestPoint, error)
}
