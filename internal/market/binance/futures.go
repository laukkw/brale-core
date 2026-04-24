package binance

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"brale-core/internal/market"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/pkg/parseutil"
	"brale-core/internal/snapshot"

	"github.com/adshao/go-binance/v2/futures"
	"go.uber.org/zap"
)

// =====================
// Binance Futures 公共接口实现：K 线 / OI / Funding / 多空比。
// 示例：interval="1h"，period="5m"（LongShortRatio）。
// =====================

type FuturesMarket struct {
	Client         *futures.Client
	RequestTimeout time.Duration
}

type Orderbook struct {
	LastUpdateID int64
	Bids         []OrderbookLevel
	Asks         []OrderbookLevel
}

type OrderbookLevel struct {
	Price    float64
	Quantity float64
}

func NewFuturesMarket() *FuturesMarket {
	return &FuturesMarket{
		Client:         futures.NewClient("", ""),
		RequestTimeout: 12 * time.Second,
	}
}

func (m *FuturesMarket) Klines(ctx context.Context, symbol, interval string, limit int) ([]snapshot.Candle, error) {
	if err := m.validateClient(); err != nil {
		return nil, err
	}
	ctx, cancel := m.requestContext(ctx)
	defer cancel()
	logger := logging.FromContext(ctx).Named("market").With(zap.String("symbol", symbol), zap.String("interval", interval))
	svc := m.Client.NewKlinesService().Symbol(symbol).Interval(interval)
	if limit > 0 {
		svc.Limit(limit)
	}
	klines, err := svc.Do(ctx)
	if err != nil {
		return nil, market.ExternalError(err, "binance_klines_failed")
	}
	if len(klines) == 0 {
		return nil, market.UnavailableErrorf("empty klines")
	}
	out := make([]snapshot.Candle, 0, len(klines))
	var skipped int
	for i, k := range klines {
		open, err := parseFloat(k.Open)
		if err != nil {
			skipped++
			logKlineParseSkip("open", i, k.OpenTime, err)
			continue
		}
		high, err := parseFloat(k.High)
		if err != nil {
			skipped++
			logKlineParseSkip("high", i, k.OpenTime, err)
			continue
		}
		low, err := parseFloat(k.Low)
		if err != nil {
			skipped++
			logKlineParseSkip("low", i, k.OpenTime, err)
			continue
		}
		closeVal, err := parseFloat(k.Close)
		if err != nil {
			skipped++
			logKlineParseSkip("close", i, k.OpenTime, err)
			continue
		}
		vol, err := parseFloat(k.Volume)
		if err != nil {
			skipped++
			logKlineParseSkip("volume", i, k.OpenTime, err)
			continue
		}
		takerBuy, err := parseFloat(k.TakerBuyBaseAssetVolume)
		if err != nil {
			skipped++
			logKlineParseSkip("taker_buy", i, k.OpenTime, err)
			continue
		}
		takerSell := vol - takerBuy
		if takerSell < 0 {
			logger.Warn("binance taker sell volume negative, clamping to zero",
				zap.Int("index", i),
				zap.Int64("open_time", k.OpenTime),
				zap.Float64("volume", vol),
				zap.Float64("taker_buy_volume", takerBuy),
			)
			takerSell = 0
		}
		out = append(out, snapshot.Candle{
			OpenTime:        k.OpenTime,
			Open:            open,
			High:            high,
			Low:             low,
			Close:           closeVal,
			Volume:          vol,
			TakerBuyVolume:  takerBuy,
			TakerSellVolume: takerSell,
		})
	}
	if len(out) == 0 {
		return nil, market.UnavailableErrorf("all %d klines failed to parse", len(klines))
	}
	return out, nil
}

func (m *FuturesMarket) OpenInterest(ctx context.Context, symbol string) (snapshot.OIBlock, error) {
	if err := m.validateClient(); err != nil {
		return snapshot.OIBlock{}, err
	}
	ctx, cancel := m.requestContext(ctx)
	defer cancel()
	oi, err := m.Client.NewGetOpenInterestService().Symbol(symbol).Do(ctx)
	if err != nil {
		return snapshot.OIBlock{}, market.ExternalError(err, "binance_open_interest_failed")
	}
	val, err := parseFloat(oi.OpenInterest)
	if err != nil {
		return snapshot.OIBlock{}, err
	}
	return snapshot.OIBlock{Value: val, Timestamp: oi.Time}, nil
}

func (m *FuturesMarket) Funding(ctx context.Context, symbol string) (snapshot.FundingBlock, error) {
	if err := m.validateClient(); err != nil {
		return snapshot.FundingBlock{}, err
	}
	ctx, cancel := m.requestContext(ctx)
	defer cancel()
	list, err := m.Client.NewFundingRateService().Symbol(symbol).Limit(1).Do(ctx)
	if err != nil {
		return snapshot.FundingBlock{}, market.ExternalError(err, "binance_funding_failed")
	}
	if len(list) == 0 {
		return snapshot.FundingBlock{}, market.UnavailableErrorf("empty funding rate")
	}
	rate, err := parseFloat(list[0].FundingRate)
	if err != nil {
		return snapshot.FundingBlock{}, err
	}
	return snapshot.FundingBlock{Rate: rate, Timestamp: list[0].FundingTime}, nil
}

func (m *FuturesMarket) MarkPrice(ctx context.Context, symbol string) (market.PriceQuote, error) {
	if err := m.validateClient(); err != nil {
		return market.PriceQuote{}, err
	}
	ctx, cancel := m.requestContext(ctx)
	defer cancel()
	list, err := m.Client.NewPremiumIndexService().Symbol(symbol).Do(ctx)
	if err != nil {
		return market.PriceQuote{}, market.ExternalError(err, "binance_mark_price_failed")
	}
	if len(list) == 0 {
		return market.PriceQuote{}, market.UnavailableErrorf("empty mark price")
	}
	price, err := parseFloat(list[0].MarkPrice)
	if err != nil {
		return market.PriceQuote{}, err
	}
	return market.PriceQuote{
		Symbol:    symbol,
		Price:     price,
		Timestamp: list[0].Time,
		Source:    "binance_mark",
	}, nil
}

func (m *FuturesMarket) GetOrderbook(ctx context.Context, symbol string, limit int) (Orderbook, error) {
	if err := m.validateClient(); err != nil {
		return Orderbook{}, err
	}
	ctx, cancel := m.requestContext(ctx)
	defer cancel()
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return Orderbook{}, market.ValidationErrorf("symbol is required")
	}
	if limit <= 0 {
		limit = 5
	}
	switch limit {
	case 5, 10, 20, 50, 100, 500, 1000:
		// ok
	default:
		return Orderbook{}, market.ValidationErrorf("limit must be one of 5/10/20/50/100/500/1000")
	}

	depth, err := m.Client.NewDepthService().Symbol(symbol).Limit(limit).Do(ctx)
	if err != nil {
		return Orderbook{}, market.ExternalError(err, "binance_orderbook_failed")
	}
	if depth == nil {
		return Orderbook{}, market.UnavailableErrorf("orderbook not available")
	}
	if len(depth.Bids) == 0 || len(depth.Asks) == 0 {
		return Orderbook{}, market.UnavailableErrorf("empty orderbook")
	}

	out := Orderbook{LastUpdateID: depth.LastUpdateID}
	out.Bids = make([]OrderbookLevel, 0, len(depth.Bids))
	for _, bid := range depth.Bids {
		price, err := parseFloat(bid.Price)
		if err != nil {
			return Orderbook{}, err
		}
		qty, err := parseFloat(bid.Quantity)
		if err != nil {
			return Orderbook{}, err
		}
		out.Bids = append(out.Bids, OrderbookLevel{Price: price, Quantity: qty})
	}
	out.Asks = make([]OrderbookLevel, 0, len(depth.Asks))
	for _, ask := range depth.Asks {
		price, err := parseFloat(ask.Price)
		if err != nil {
			return Orderbook{}, err
		}
		qty, err := parseFloat(ask.Quantity)
		if err != nil {
			return Orderbook{}, err
		}
		out.Asks = append(out.Asks, OrderbookLevel{Price: price, Quantity: qty})
	}
	return out, nil
}

func (m *FuturesMarket) LongShortRatio(ctx context.Context, symbol, interval string) (snapshot.LSRBlock, error) {
	if err := m.validateClient(); err != nil {
		return snapshot.LSRBlock{}, err
	}
	ctx, cancel := m.requestContext(ctx)
	defer cancel()
	period := strings.TrimSpace(interval)
	if period == "" {
		period = "5m"
	}
	list, err := m.Client.NewLongShortRatioService().Symbol(symbol).Period(period).Limit(1).Do(ctx)
	if err != nil {
		return snapshot.LSRBlock{}, market.ExternalError(err, "binance_long_short_failed")
	}
	if len(list) == 0 {
		return snapshot.LSRBlock{}, market.UnavailableErrorf("empty long/short ratio")
	}
	val, err := parseFloat(list[0].LongShortRatio)
	if err != nil {
		return snapshot.LSRBlock{}, err
	}
	return snapshot.LSRBlock{LongShortRatio: val, Timestamp: list[0].Timestamp}, nil
}

type liqEvent struct {
	symbol   string
	timeMs   int64
	price    float64
	qty      float64
	notional float64
	isLong   bool
}

func resolveLiquidationWindows() ([]string, []int, map[string]time.Duration, time.Duration, error) {
	windows := snapshot.DefaultLiqWindows
	priceBins := snapshot.DefaultLiqPriceBinsBps
	if len(windows) == 0 {
		return nil, nil, nil, 0, market.ValidationErrorf("liquidation windows are required")
	}
	windowDurations := make(map[string]time.Duration, len(windows))
	var maxWindow time.Duration
	for _, window := range windows {
		windowDur, err := parseWindowDuration(window)
		if err != nil {
			return nil, nil, nil, 0, err
		}
		windowDurations[window] = windowDur
		if windowDur > maxWindow {
			maxWindow = windowDur
		}
	}
	return windows, priceBins, windowDurations, maxWindow, nil
}

func liquidationSideIsLong(side futures.SideType) (bool, bool) {
	switch side {
	case futures.SideTypeSell:
		// Binance force orders report order side: SELL liquidates longs, BUY liquidates shorts.
		return true, true
	case futures.SideTypeBuy:
		return false, true
	default:
		return false, false
	}
}

func aggregateLiquidationWindows(events []liqEvent, windows []string, windowDurations map[string]time.Duration, endMs int64, priceBins []int) map[string]snapshot.LiqWindow {
	results := make(map[string]snapshot.LiqWindow, len(windows))
	for _, window := range windows {
		windowDur := windowDurations[window]
		sinceMs := endMs - windowDur.Milliseconds()
		if sinceMs < 0 {
			sinceMs = 0
		}
		results[window] = aggregateLiqWindow(events, sinceMs, priceBins)
	}
	return results
}

func aggregateLiqWindow(events []liqEvent, sinceMs int64, priceBins []int) snapshot.LiqWindow {
	window := snapshot.LiqWindow{
		PriceBinsBps: append([]int(nil), priceBins...),
		Bins:         make([]snapshot.LiqPriceBin, len(priceBins)),
		Rel:          snapshot.LiqRelMetrics{},
	}
	for i, bps := range priceBins {
		window.Bins[i] = snapshot.LiqPriceBin{Bps: bps}
	}
	var longVol float64
	var shortVol float64
	var totalVol float64
	var sumQty float64
	for _, event := range events {
		if event.timeMs < sinceMs {
			continue
		}
		if event.notional <= 0 || event.qty <= 0 {
			continue
		}
		window.SampleCount++
		if event.isLong {
			longVol += event.notional
		} else {
			shortVol += event.notional
		}
		totalVol += event.notional
		sumQty += event.qty
	}
	window.LongVol = longVol
	window.ShortVol = shortVol
	window.TotalVol = totalVol
	if totalVol > 0 {
		window.Imbalance = (longVol - shortVol) / totalVol
	}
	if len(window.Bins) == 0 || totalVol == 0 || sumQty <= 0 {
		return window
	}
	vwap := totalVol / sumQty
	if vwap <= 0 {
		return window
	}
	for _, event := range events {
		if event.timeMs < sinceMs {
			continue
		}
		deviationBps := math.Abs((event.price - vwap) / vwap * 10000)
		binIndex := liqBinIndex(deviationBps, priceBins)
		if binIndex < 0 || binIndex >= len(window.Bins) {
			continue
		}
		if event.isLong {
			window.Bins[binIndex].LongVol += event.notional
		} else {
			window.Bins[binIndex].ShortVol += event.notional
		}
		window.Bins[binIndex].TotalVol += event.notional
	}
	for i := range window.Bins {
		if window.Bins[i].TotalVol > 0 {
			window.Bins[i].Imbalance = (window.Bins[i].LongVol - window.Bins[i].ShortVol) / window.Bins[i].TotalVol
		}
	}
	return window
}

func liqBinIndex(bps float64, priceBins []int) int {
	if len(priceBins) == 0 {
		return -1
	}
	for i, binBps := range priceBins {
		if bps <= float64(binBps) {
			return i
		}
	}
	return len(priceBins) - 1
}

func (m *FuturesMarket) GetFundingRate(ctx context.Context, symbol string) (float64, error) {
	raw, err := m.GetFundingRateRaw(ctx, symbol)
	if err != nil {
		return 0, err
	}
	return parseFloat(raw)
}

func (m *FuturesMarket) GetFundingRateRaw(ctx context.Context, symbol string) (string, error) {
	if err := m.validateClient(); err != nil {
		return "", err
	}
	ctx, cancel := m.requestContext(ctx)
	defer cancel()
	if strings.TrimSpace(symbol) == "" {
		return "", market.ValidationErrorf("symbol is required")
	}
	res, err := m.Client.NewPremiumIndexService().Symbol(symbol).Do(ctx)
	if err != nil {
		return "", market.ExternalError(err, "binance_premium_index_failed")
	}
	for _, entry := range res {
		if entry == nil {
			continue
		}
		if strings.EqualFold(entry.Symbol, symbol) {
			return strings.TrimSpace(entry.LastFundingRate), nil
		}
	}
	if len(res) > 0 && res[0] != nil {
		return strings.TrimSpace(res[0].LastFundingRate), nil
	}
	return "", market.UnavailableErrorf("funding rate not available for %s", symbol)
}

func (m *FuturesMarket) GetOpenInterestHistory(ctx context.Context, symbol, period string, limit int) ([]market.OpenInterestPoint, error) {
	if err := m.validateClient(); err != nil {
		return nil, err
	}
	ctx, cancel := m.requestContext(ctx)
	defer cancel()
	if strings.TrimSpace(symbol) == "" || strings.TrimSpace(period) == "" {
		return nil, market.ValidationErrorf("symbol and period are required")
	}
	if limit <= 0 {
		limit = 30
	}
	if limit > 500 {
		limit = 500
	}
	period = strings.ToLower(strings.TrimSpace(period))
	svc := m.Client.NewOpenInterestStatisticsService().Symbol(symbol).Period(period).Limit(limit)
	stats, err := svc.Do(ctx)
	if err != nil {
		return nil, market.ExternalError(err, "binance_oi_history_failed")
	}
	points := make([]market.OpenInterestPoint, 0, len(stats))
	for _, item := range stats {
		if item == nil {
			continue
		}
		oiVal, err := parseFloat(item.SumOpenInterest)
		if err != nil {
			return nil, err
		}
		oiValue, err := parseFloat(item.SumOpenInterestValue)
		if err != nil {
			return nil, err
		}
		points = append(points, market.OpenInterestPoint{
			Symbol:               item.Symbol,
			SumOpenInterest:      oiVal,
			SumOpenInterestValue: oiValue,
			Timestamp:            item.Timestamp,
		})
	}
	return points, nil
}

func (m *FuturesMarket) TopPositionRatio(ctx context.Context, symbol, period string, limit int) ([]market.LongShortRatioPoint, error) {
	if err := m.validateClient(); err != nil {
		return nil, err
	}
	ctx, cancel := m.requestContext(ctx)
	defer cancel()
	if strings.TrimSpace(symbol) == "" || strings.TrimSpace(period) == "" {
		return nil, market.ValidationErrorf("symbol and period are required")
	}
	if limit <= 0 {
		limit = 30
	}
	if limit > 500 {
		limit = 500
	}
	period = strings.ToLower(strings.TrimSpace(period))
	svc := m.Client.NewTopLongShortPositionRatioService().Symbol(symbol).Period(period).Limit(uint32(limit))
	raw, err := svc.Do(ctx)
	if err != nil {
		return nil, market.ExternalError(err, "binance_top_position_ratio_failed")
	}
	points := make([]market.LongShortRatioPoint, 0, len(raw))
	for _, item := range raw {
		if item == nil {
			continue
		}
		ratio, err := parseFloat(item.LongShortRatio)
		if err != nil {
			return nil, err
		}
		longVal, err := parseFloat(item.LongAccount)
		if err != nil {
			return nil, err
		}
		shortVal, err := parseFloat(item.ShortAccount)
		if err != nil {
			return nil, err
		}
		points = append(points, market.LongShortRatioPoint{
			Timestamp: int64(item.Timestamp),
			Ratio:     ratio,
			Long:      longVal,
			Short:     shortVal,
		})
	}
	return points, nil
}

func (m *FuturesMarket) TopAccountRatio(ctx context.Context, symbol, period string, limit int) ([]market.LongShortRatioPoint, error) {
	if err := m.validateClient(); err != nil {
		return nil, err
	}
	ctx, cancel := m.requestContext(ctx)
	defer cancel()
	if strings.TrimSpace(symbol) == "" || strings.TrimSpace(period) == "" {
		return nil, market.ValidationErrorf("symbol and period are required")
	}
	if limit <= 0 {
		limit = 30
	}
	if limit > 500 {
		limit = 500
	}
	period = strings.ToLower(strings.TrimSpace(period))
	svc := m.Client.NewTopLongShortAccountRatioService().Symbol(symbol).Period(period).Limit(uint32(limit))
	raw, err := svc.Do(ctx)
	if err != nil {
		return nil, market.ExternalError(err, "binance_top_account_ratio_failed")
	}
	points := make([]market.LongShortRatioPoint, 0, len(raw))
	for _, item := range raw {
		if item == nil {
			continue
		}
		ratio, err := parseFloat(item.LongShortRatio)
		if err != nil {
			return nil, err
		}
		longVal, err := parseFloat(item.LongAccount)
		if err != nil {
			return nil, err
		}
		shortVal, err := parseFloat(item.ShortAccount)
		if err != nil {
			return nil, err
		}
		points = append(points, market.LongShortRatioPoint{
			Timestamp: int64(item.Timestamp),
			Ratio:     ratio,
			Long:      longVal,
			Short:     shortVal,
		})
	}
	return points, nil
}

func (m *FuturesMarket) GlobalAccountRatio(ctx context.Context, symbol, period string, limit int) ([]market.LongShortRatioPoint, error) {
	if err := m.validateClient(); err != nil {
		return nil, err
	}
	ctx, cancel := m.requestContext(ctx)
	defer cancel()
	if strings.TrimSpace(symbol) == "" || strings.TrimSpace(period) == "" {
		return nil, market.ValidationErrorf("symbol and period are required")
	}
	if limit <= 0 {
		limit = 30
	}
	if limit > 500 {
		limit = 500
	}
	period = strings.ToLower(strings.TrimSpace(period))
	svc := m.Client.NewLongShortRatioService().Symbol(symbol).Period(period).Limit(limit)
	raw, err := svc.Do(ctx)
	if err != nil {
		return nil, market.ExternalError(err, "binance_global_ratio_failed")
	}
	points := make([]market.LongShortRatioPoint, 0, len(raw))
	for _, item := range raw {
		if item == nil {
			continue
		}
		ratio, err := parseFloat(item.LongShortRatio)
		if err != nil {
			return nil, err
		}
		points = append(points, market.LongShortRatioPoint{
			Timestamp: int64(item.Timestamp),
			Ratio:     ratio,
			Long:      0,
			Short:     0,
		})
	}
	return points, nil
}

func (m *FuturesMarket) SupportedOIPeriods() []string {
	return []string{"5m", "15m", "30m", "1h", "2h", "4h", "6h", "12h", "1d"}
}

func (m *FuturesMarket) validateClient() error {
	if m == nil || m.Client == nil {
		return market.ValidationErrorf("binance client is required")
	}
	return nil
}

func (m *FuturesMarket) requestContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	timeout := m.RequestTimeout
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	return context.WithTimeout(ctx, timeout)
}

func parseWindowDuration(raw string) (time.Duration, error) {
	value := strings.TrimSpace(strings.ToLower(raw))
	if len(value) < 2 {
		return 0, fmt.Errorf("invalid window=%s", raw)
	}
	unit := value[len(value)-1]
	amountRaw := value[:len(value)-1]
	amount, err := strconv.Atoi(amountRaw)
	if err != nil || amount <= 0 {
		return 0, fmt.Errorf("invalid window=%s", raw)
	}
	switch unit {
	case 'm':
		return time.Duration(amount) * time.Minute, nil
	case 'h':
		return time.Duration(amount) * time.Hour, nil
	case 'd':
		return time.Duration(amount) * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unsupported window=%s", raw)
	}
}

func parseFloat(raw string) (float64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, market.ExternalError(fmt.Errorf("empty number"), "binance_parse_error")
	}
	val, err := parseutil.ParseFloatString(raw)
	if err != nil {
		return 0, market.ExternalError(fmt.Errorf("invalid number: %w", err), "binance_parse_error")
	}
	return val, nil
}

func logKlineParseSkip(field string, index int, openTime int64, err error) {
	logging.L().Warn("kline parse skip",
		zap.String("field", field),
		zap.Int("index", index),
		zap.Int64("open_time", openTime),
		zap.Error(err),
	)
}
