// 本文件主要内容：定义价格源接口与价格引用结构。

package market

import "context"

type PriceQuote struct {
	Symbol    string
	Price     float64
	Timestamp int64
	Source    string
}

type PriceSource interface {
	MarkPrice(ctx context.Context, symbol string) (PriceQuote, error)
}
