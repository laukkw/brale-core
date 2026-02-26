// 本文件主要内容：定义执行器接口与外部订单/持仓结构体。

package execution

import "context"

type OrderKind string

type PlaceOrderReq struct {
	Kind          OrderKind
	Symbol        string
	Side          string
	Quantity      float64
	Price         float64
	ReduceOnly    bool
	PositionID    string
	ClientOrderID string
	Tag           string
	OrderType     string
	Leverage      float64
}

type PlaceOrderResp struct {
	ExternalID string
	Status     string
}

type CancelOrderReq struct {
	ExternalID    string
	ClientOrderID string
}

type CancelOrderResp struct {
	Status string
}

type ExternalPosition struct {
	PositionID   string
	Symbol       string
	Side         string
	Quantity     float64
	AvgEntry     float64
	InitialStake float64
	Status       string
	OpenOrderID  string
	CloseOrderID string
	EntryTag     string
	CurrentPrice float64
	UpdatedAt    int64
}

type ExternalOrder struct {
	OrderID       string
	ClientOrderID string
	Symbol        string
	Side          string
	Quantity      float64
	Price         float64
	Status        string
	FilledQty     float64
	OrderType     string
	Tag           string
	Timestamp     int64
}

type ExternalFill struct {
	OrderID   string
	Symbol    string
	Side      string
	Quantity  float64
	Price     float64
	Timestamp int64
}

type Executor interface {
	Name() string
	GetOpenPositions(ctx context.Context, symbol string) ([]ExternalPosition, error)
	GetOpenOrders(ctx context.Context, symbol string) ([]ExternalOrder, error)
	GetOrder(ctx context.Context, id string) (ExternalOrder, error)
	GetRecentFills(ctx context.Context, symbol string, sinceTS int64) ([]ExternalFill, error)
	PlaceOrder(ctx context.Context, req PlaceOrderReq) (PlaceOrderResp, error)
	CancelOrder(ctx context.Context, req CancelOrderReq) (CancelOrderResp, error)
	Ping(ctx context.Context) error
	NowMillis() int64
}

const (
	OrderOpen   OrderKind = "open"
	OrderClose  OrderKind = "close"
	OrderReduce OrderKind = "reduce"
)
