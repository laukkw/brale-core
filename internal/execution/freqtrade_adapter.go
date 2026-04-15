// 本文件主要内容：实现基于 freqtrade forceenter/forceexit 的执行器适配。

package execution

import (
	"context"
	"strconv"
	"strings"
	"time"

	"brale-core/internal/pkg/logging"
	symbolpkg "brale-core/internal/pkg/symbol"

	"go.uber.org/zap"
)

type FreqtradeAdapter struct {
	Client *FreqtradeClient
}

func (a *FreqtradeAdapter) ListOpenTrades(ctx context.Context) ([]Trade, error) {
	if a == nil || a.Client == nil {
		return nil, execValidationErrorf("freqtrade client is required")
	}
	return a.Client.ListOpenTrades(ctx)
}

func (a *FreqtradeAdapter) Name() string {
	return "freqtrade"
}

func (a *FreqtradeAdapter) Ping(ctx context.Context) error {
	if a == nil || a.Client == nil {
		return execValidationErrorf("freqtrade client is required")
	}
	if err := a.Client.Ping(ctx); err != nil {
		return execExternalError(err, "freqtrade_ping_failed")
	}
	return nil
}

func (a *FreqtradeAdapter) NowMillis() int64 {
	return time.Now().UnixMilli()
}

func (a *FreqtradeAdapter) ResolveCloseQuantity(kind OrderKind, requestedQty float64) float64 {
	if kind == OrderClose {
		return 0
	}
	return requestedQty
}

func (a *FreqtradeAdapter) GetOpenPositions(ctx context.Context, symbol string) ([]ExternalPosition, error) {
	if a == nil || a.Client == nil {
		return nil, execValidationErrorf("freqtrade client is required")
	}
	trades, err := a.Client.ListOpenTrades(ctx)
	if err != nil {
		return nil, execExternalError(err, "freqtrade_list_open_trades_failed")
	}
	out := make([]ExternalPosition, 0, len(trades))
	filterSymbol := fromFreqtradePair(symbol)
	for _, tr := range trades {
		pair := strings.ToUpper(strings.TrimSpace(tr.Pair))
		internalSymbol := fromFreqtradePair(pair)
		if filterSymbol != "" && !strings.EqualFold(internalSymbol, filterSymbol) {
			continue
		}
		side := "long"
		if tr.IsShort || strings.EqualFold(tr.Side, "short") {
			side = "short"
		}
		out = append(out, ExternalPosition{
			PositionID:   strconv.Itoa(tr.ID),
			Symbol:       internalSymbol,
			Side:         side,
			Quantity:     float64(tr.Amount),
			AvgEntry:     float64(tr.OpenRate),
			InitialStake: pickInitialStake(tr),
			Status:       "open",
			OpenOrderID:  string(tr.OpenOrderID),
			CloseOrderID: string(tr.CloseOrderID),
			CurrentPrice: float64(tr.CurrentRate),
			UpdatedAt:    time.Now().UnixMilli(),
		})
	}
	return out, nil
}

func (a *FreqtradeAdapter) GetOpenOrders(ctx context.Context, symbol string) ([]ExternalOrder, error) {
	if a == nil || a.Client == nil {
		return nil, execValidationErrorf("freqtrade client is required")
	}
	trades, err := a.Client.ListOpenTrades(ctx)
	if err != nil {
		return nil, execExternalError(err, "freqtrade_list_open_trades_failed")
	}
	out := make([]ExternalOrder, 0)
	filterSymbol := fromFreqtradePair(symbol)
	for _, tr := range trades {
		pair := strings.ToUpper(strings.TrimSpace(tr.Pair))
		internalSymbol := fromFreqtradePair(pair)
		if filterSymbol != "" && !strings.EqualFold(internalSymbol, filterSymbol) {
			continue
		}
		for _, ord := range tr.Orders {
			out = append(out, ExternalOrder{
				OrderID:   ord.OrderID,
				Symbol:    internalSymbol,
				Side:      normalizeFTSide(string(ord.FTOrderSide), tr.IsShort),
				Quantity:  float64(ord.Amount),
				Price:     float64(ord.Price),
				Status:    string(ord.Status),
				FilledQty: float64(ord.Filled),
				OrderType: string(ord.OrderType),
				Tag:       string(ord.FTOrderTag),
				Timestamp: int64(ord.OrderTimestamp),
			})
		}
	}
	return out, nil
}

func (a *FreqtradeAdapter) GetOrder(ctx context.Context, id string) (ExternalOrder, error) {
	if a == nil || a.Client == nil {
		return ExternalOrder{}, execValidationErrorf("freqtrade client is required")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return ExternalOrder{}, execValidationErrorf("order id is required")
	}
	if tradeID, err := strconv.Atoi(id); err == nil && tradeID > 0 {
		trade, ok, findErr := a.Client.FindTradeByID(ctx, tradeID)
		if findErr != nil {
			return ExternalOrder{}, execExternalError(findErr, "freqtrade_find_trade_failed")
		}
		if ok {
			if ord, hasOrder := externalOrderFromTrade(trade, id); hasOrder {
				return ord, nil
			}
			return externalOrderFromTradeSummary(trade, id), nil
		}
	}
	orders, err := a.GetOpenOrders(ctx, "")
	if err != nil {
		return ExternalOrder{}, err
	}
	for _, ord := range orders {
		if ord.OrderID == id {
			return ord, nil
		}
	}
	return ExternalOrder{}, execNotFoundErrorf("order not found")
}

func (a *FreqtradeAdapter) GetRecentFills(ctx context.Context, symbol string, sinceTS int64) ([]ExternalFill, error) {
	return nil, execValidationErrorf("freqtrade recent fills not supported")
}

func (a *FreqtradeAdapter) PlaceOrder(ctx context.Context, req PlaceOrderReq) (PlaceOrderResp, error) {
	if a == nil || a.Client == nil {
		return PlaceOrderResp{}, execValidationErrorf("freqtrade client is required")
	}
	switch req.Kind {
	case OrderOpen:
		return a.placeOpen(ctx, req)
	case OrderClose, OrderReduce:
		return a.placeClose(ctx, req)
	default:
		return PlaceOrderResp{}, execValidationErrorf("unsupported order kind")
	}
}

func (a *FreqtradeAdapter) CancelOrder(ctx context.Context, req CancelOrderReq) (CancelOrderResp, error) {
	if a == nil || a.Client == nil {
		return CancelOrderResp{}, execValidationErrorf("freqtrade client is required")
	}
	tradeID, err := strconv.Atoi(strings.TrimSpace(req.ExternalID))
	if err != nil || tradeID <= 0 {
		return CancelOrderResp{}, execValidationErrorf("trade_id is required")
	}
	if err := a.Client.CancelOpenOrder(ctx, tradeID); err != nil {
		if isCancelOrderNotFound(err) {
			return CancelOrderResp{Status: "not_found"}, nil
		}
		logging.FromContext(ctx).Named("execution").Error("freqtrade cancel order failed",
			zap.Int("trade_id", tradeID),
			zap.Error(err),
		)
		return CancelOrderResp{}, execExternalError(err, "freqtrade_cancel_order_failed")
	}
	return CancelOrderResp{Status: "submitted"}, nil
}

func (a *FreqtradeAdapter) placeOpen(ctx context.Context, req PlaceOrderReq) (PlaceOrderResp, error) {
	pair := toFreqtradePair(req.Symbol)
	if pair == "" {
		return PlaceOrderResp{}, execValidationErrorf("symbol is required")
	}
	side := strings.ToLower(strings.TrimSpace(req.Side))
	if side != "long" && side != "short" {
		return PlaceOrderResp{}, execValidationErrorf("invalid side")
	}
	ftSide := side
	if req.Price <= 0 {
		return PlaceOrderResp{}, execValidationErrorf("price is required for stake calculation")
	}
	leverage := req.Leverage
	if leverage <= 0 {
		logging.FromContext(ctx).Named("execution").Warn("freqtrade leverage invalid, defaulting to 1", zap.Float64("leverage", leverage))
		leverage = 1
	}
	stake := (req.Price * req.Quantity) / leverage
	orderType := strings.TrimSpace(req.OrderType)
	orderTypeLower := strings.ToLower(orderType)
	payload := ForceEnterPayload{
		Pair:        pair,
		Side:        ftSide,
		StakeAmount: stake,
		OrderType:   orderType,
		EntryTag:    strings.TrimSpace(req.Tag),
		Leverage:    leverage,
	}
	logging.FromContext(ctx).Named("execution").Info("freqtrade place open", zap.String("symbol", req.Symbol), zap.String("side", side), zap.Float64("quantity", req.Quantity), zap.String("order_type", payload.OrderType), zap.String("tag", payload.EntryTag), zap.Float64("price", req.Price), zap.Float64("leverage", leverage))
	if req.Price > 0 && orderTypeLower != "market" {
		price := req.Price
		payload.Price = &price
	}
	resp, err := a.Client.ForceEnter(ctx, payload)
	if err != nil {
		logging.FromContext(ctx).Named("execution").Error("freqtrade force enter failed",
			zap.String("symbol", req.Symbol),
			zap.String("side", side),
			zap.Float64("stake", stake),
			zap.Error(err),
		)
		return PlaceOrderResp{}, execExternalError(err, "freqtrade_force_enter_failed")
	}
	return PlaceOrderResp{ExternalID: strconv.Itoa(resp.TradeID), Status: "submitted"}, nil
}

func externalOrderFromTrade(tr Trade, fallbackID string) (ExternalOrder, bool) {
	bestIdx := -1
	bestTS := int64(0)
	preferEntry := false
	for i, ord := range tr.Orders {
		ts := int64(ord.OrderFilledAt)
		if ts <= 0 {
			ts = int64(ord.OrderTimestamp)
		}
		if ts <= 0 {
			ts = int64(tr.OpenFillTimestamp)
		}
		isEntry := ord.FTIsEntry
		if bestIdx == -1 {
			bestIdx = i
			bestTS = ts
			preferEntry = isEntry
			continue
		}
		if isEntry && !preferEntry {
			bestIdx = i
			bestTS = ts
			preferEntry = true
			continue
		}
		if isEntry == preferEntry && ts >= bestTS {
			bestIdx = i
			bestTS = ts
		}
	}
	if bestIdx == -1 {
		return ExternalOrder{}, false
	}
	ord := tr.Orders[bestIdx]
	orderID := strings.TrimSpace(ord.OrderID)
	if orderID == "" {
		orderID = fallbackID
	}
	return ExternalOrder{
		OrderID:   orderID,
		Symbol:    fromFreqtradePair(tr.Pair),
		Side:      normalizeFTSide(string(ord.FTOrderSide), tr.IsShort),
		Quantity:  float64(ord.Amount),
		Price:     float64(ord.Price),
		Status:    string(ord.Status),
		FilledQty: float64(ord.Filled),
		OrderType: string(ord.OrderType),
		Tag:       string(ord.FTOrderTag),
		Timestamp: bestTS,
	}, true
}

func externalOrderFromTradeSummary(tr Trade, fallbackID string) ExternalOrder {
	qty := float64(tr.AmountRequested)
	if qty <= 0 {
		qty = float64(tr.Amount)
	}
	price := float64(tr.OpenRateRequested)
	if price <= 0 {
		price = float64(tr.OpenRate)
	}
	status := "closed"
	if tr.IsOpen {
		status = "open"
	}
	ts := int64(tr.OpenFillTimestamp)
	if ts <= 0 {
		ts = int64(tr.CloseTimestamp)
	}
	return ExternalOrder{
		OrderID:   fallbackID,
		Symbol:    fromFreqtradePair(tr.Pair),
		Side:      normalizeFTSide(tr.Side, tr.IsShort),
		Quantity:  qty,
		Price:     price,
		Status:    status,
		FilledQty: float64(tr.Amount),
		Tag:       string(tr.EnterTag),
		Timestamp: ts,
	}
}

func (a *FreqtradeAdapter) placeClose(ctx context.Context, req PlaceOrderReq) (PlaceOrderResp, error) {
	logger := logging.FromContext(ctx).Named("execution")
	tradeID := strings.TrimSpace(req.PositionID)
	if tradeID == "" {
		return PlaceOrderResp{}, execValidationErrorf("position_id is required")
	}
	payload := ForceExitPayload{TradeID: tradeID, OrderType: strings.TrimSpace(req.OrderType)}
	amountRequested := req.Quantity > 0
	if amountRequested {
		payload.Amount = req.Quantity
	}
	logger.Info("freqtrade place close",
		zap.String("symbol", req.Symbol),
		zap.String("trade_id", tradeID),
		zap.Float64("quantity", req.Quantity),
		zap.String("order_type", payload.OrderType),
		zap.String("kind", string(req.Kind)),
	)
	start := time.Now()
	if err := a.Client.ForceExit(ctx, payload); err != nil {
		if amountRequested && isFreqtradeDustRemainingError(err) {
			logger.Info("freqtrade dust remaining, retrying full close", zap.String("trade_id", tradeID))
			retryPayload := ForceExitPayload{TradeID: tradeID, OrderType: payload.OrderType}
			if retryErr := a.Client.ForceExit(ctx, retryPayload); retryErr != nil {
				logger.Error("freqtrade force exit retry failed",
					zap.String("symbol", req.Symbol),
					zap.String("trade_id", tradeID),
					zap.Duration("latency", time.Since(start)),
					zap.Error(retryErr),
				)
				return PlaceOrderResp{}, execExternalError(retryErr, "freqtrade_force_exit_failed")
			}
			return PlaceOrderResp{ExternalID: tradeID, Status: "submitted"}, nil
		}
		logger.Error("freqtrade force exit failed",
			zap.String("symbol", req.Symbol),
			zap.String("trade_id", tradeID),
			zap.Duration("latency", time.Since(start)),
			zap.Error(err),
		)
		return PlaceOrderResp{}, execExternalError(err, "freqtrade_force_exit_failed")
	}
	return PlaceOrderResp{ExternalID: tradeID, Status: "submitted"}, nil
}

func normalizeFTSide(side string, isShort bool) string {
	s := strings.ToLower(strings.TrimSpace(side))
	if s != "" {
		return s
	}
	if isShort {
		return "sell"
	}
	return "buy"
}

func pickInitialStake(tr Trade) float64 {
	if tr.MaxStakeAmount > 0 {
		return float64(tr.MaxStakeAmount)
	}
	if tr.OpenTradeValue > 0 {
		return float64(tr.OpenTradeValue)
	}
	if tr.StakeAmount > 0 {
		return float64(tr.StakeAmount)
	}
	return 0
}

func isCancelOrderNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no open order") ||
		strings.Contains(msg, "invalid trade_id") ||
		strings.Contains(msg, "trade not found")
}

func isFreqtradeDustRemainingError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "remaining amount")
}

func toFreqtradePair(symbol string) string {
	return symbolpkg.ToFreqtradePair(symbol)
}

func fromFreqtradePair(pair string) string {
	return symbolpkg.FromFreqtradePair(pair)
}
