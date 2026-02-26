package binance

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"brale-core/internal/market"
	"brale-core/internal/pkg/parseutil"
	"brale-core/internal/snapshot"

	"github.com/adshao/go-binance/v2/futures"
)

// =====================
// Binance Futures 公共接口实现：K 线 / OI / Funding / 多空比。
// 示例：interval="1h"，period="5m"（LongShortRatio）。
// =====================

type FuturesMarket struct {
	Client         *futures.Client
	liqHistory     *liqHistoryStore
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
		liqHistory:     newLiqHistoryStore(liqHistorySize),
		RequestTimeout: 12 * time.Second,
	}
}

func (m *FuturesMarket) Klines(ctx context.Context, symbol, interval string, limit int) ([]snapshot.Candle, error) {
	if err := m.validateClient(); err != nil {
		return nil, err
	}
	ctx, cancel := m.requestContext(ctx)
	defer cancel()
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
	for _, k := range klines {
		open, err := parseFloat(k.Open)
		if err != nil {
			return nil, err
		}
		high, err := parseFloat(k.High)
		if err != nil {
			return nil, err
		}
		low, err := parseFloat(k.Low)
		if err != nil {
			return nil, err
		}
		closeVal, err := parseFloat(k.Close)
		if err != nil {
			return nil, err
		}
		vol, err := parseFloat(k.Volume)
		if err != nil {
			return nil, err
		}
		takerBuy, err := parseFloat(k.TakerBuyBaseAssetVolume)
		if err != nil {
			return nil, err
		}
		takerSell := vol - takerBuy
		if takerSell < 0 {
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
	timeMs   int64
	price    float64
	qty      float64
	notional float64
	isLong   bool
}

const (
	liqHistorySize    = 50
	liqSpikeThreshold = 2.0
)

type liqHistoryStore struct {
	mu      sync.Mutex
	entries map[string]*liqHistory
	size    int
}

type liqHistory struct {
	values []float64
	next   int
	count  int
	sum    float64
	sumsq  float64
}

func newLiqHistoryStore(size int) *liqHistoryStore {
	if size <= 0 {
		size = 1
	}
	return &liqHistoryStore{
		entries: make(map[string]*liqHistory),
		size:    size,
	}
}

func (s *liqHistoryStore) Observe(key string, value float64) float64 {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	hist := s.entries[key]
	if hist == nil {
		hist = newLiqHistory(s.size)
		s.entries[key] = hist
	}
	return hist.observe(value)
}

func newLiqHistory(size int) *liqHistory {
	if size <= 0 {
		size = 1
	}
	return &liqHistory{values: make([]float64, size)}
}

func (h *liqHistory) observe(value float64) float64 {
	mean, std, ok := h.meanStd()
	zscore := 0.0
	if ok && std > 0 {
		zscore = (value - mean) / std
	}
	h.add(value)
	return zscore
}

func (h *liqHistory) meanStd() (float64, float64, bool) {
	if h == nil || h.count < 2 {
		return 0, 0, false
	}
	mean := h.sum / float64(h.count)
	variance := (h.sumsq / float64(h.count)) - (mean * mean)
	if variance < 0 {
		variance = 0
	}
	return mean, math.Sqrt(variance), true
}

func (h *liqHistory) add(value float64) {
	if h == nil || len(h.values) == 0 {
		return
	}
	if h.count == len(h.values) {
		old := h.values[h.next]
		h.sum -= old
		h.sumsq -= old * old
	} else {
		h.count++
	}
	h.values[h.next] = value
	h.sum += value
	h.sumsq += value * value
	h.next = (h.next + 1) % len(h.values)
}

func (m *FuturesMarket) LiquidationsByWindow(ctx context.Context, symbol string) (map[string]snapshot.LiqWindow, error) {
	if err := m.validateClient(); err != nil {
		return nil, err
	}
	ctx, cancel := m.requestContext(ctx)
	defer cancel()
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return nil, market.ValidationErrorf("symbol is required")
	}
	windows, priceBins, windowDurations, maxWindow, err := resolveLiquidationWindows()
	if err != nil {
		return nil, err
	}
	endTime := time.Now().UTC()
	startMs, endMs := liquidationTimeRange(endTime, maxWindow)
	events, err := m.fetchLiquidationEvents(ctx, symbol, startMs, endMs)
	if err != nil {
		return nil, err
	}
	results := aggregateLiquidationWindows(events, windows, windowDurations, endMs, priceBins)
	m.enrichLiquidationWindowMetrics(ctx, symbol, windows, endTime, results)
	return results, nil
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

func liquidationTimeRange(endTime time.Time, maxWindow time.Duration) (int64, int64) {
	endMs := endTime.UnixMilli()
	startMs := endMs - maxWindow.Milliseconds()
	if startMs < 0 {
		startMs = 0
	}
	return startMs, endMs
}

func (m *FuturesMarket) fetchLiquidationEvents(ctx context.Context, symbol string, startMs, endMs int64) ([]liqEvent, error) {
	const forceOrderLimit = 1000
	orders, err := m.Client.NewListLiquidationOrdersService().Symbol(symbol).StartTime(startMs).EndTime(endMs).Limit(forceOrderLimit).Do(ctx)
	if err != nil {
		return nil, market.ExternalError(err, "binance_force_orders_failed")
	}
	events := make([]liqEvent, 0, len(orders))
	for _, order := range orders {
		if order == nil {
			continue
		}
		event, ok, err := liquidationOrderToEvent(order.Price, order.ExecutedQuantity, order.OrigQuantity, order.Side, order.Time)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		events = append(events, event)
	}
	return events, nil
}

func liquidationOrderToEvent(priceText, executedQtyText, origQtyText string, side futures.SideType, timeMs int64) (liqEvent, bool, error) {
	price, err := parseFloat(priceText)
	if err != nil {
		return liqEvent{}, false, err
	}
	qty, err := parseFloat(executedQtyText)
	if err != nil {
		return liqEvent{}, false, err
	}
	if qty == 0 {
		qty, err = parseFloat(origQtyText)
		if err != nil {
			return liqEvent{}, false, err
		}
	}
	if qty <= 0 || price <= 0 {
		return liqEvent{}, false, nil
	}
	isLong, ok := liquidationSideIsLong(side)
	if !ok {
		return liqEvent{}, false, nil
	}
	return liqEvent{
		timeMs:   timeMs,
		price:    price,
		qty:      qty,
		notional: price * qty,
		isLong:   isLong,
	}, true, nil
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

func (m *FuturesMarket) enrichLiquidationWindowMetrics(ctx context.Context, symbol string, windows []string, endTime time.Time, results map[string]snapshot.LiqWindow) {
	oiValue := m.loadOpenInterestValue(ctx, symbol)
	volumeByWindow := m.loadVolumeByWindow(ctx, symbol, windows, endTime)
	for _, window := range windows {
		win := results[window]
		if oiValue > 0 {
			win.Rel.VolOverOI = win.TotalVol / oiValue
		}
		volume := volumeByWindow[window]
		if volume > 0 {
			win.Rel.VolOverVolume = win.TotalVol / volume
		}
		if m != nil && m.liqHistory != nil {
			key := symbol + "|" + window
			win.Rel.ZScore = m.liqHistory.Observe(key, win.TotalVol)
		}
		win.Rel.Spike = win.Rel.ZScore >= liqSpikeThreshold
		results[window] = win
	}
}

func (m *FuturesMarket) loadOpenInterestValue(ctx context.Context, symbol string) float64 {
	if oi, err := m.OpenInterest(ctx, symbol); err == nil {
		return oi.Value
	}
	return 0
}

func (m *FuturesMarket) loadVolumeByWindow(ctx context.Context, symbol string, windows []string, endTime time.Time) map[string]float64 {
	volumeByWindow := make(map[string]float64, len(windows))
	for _, window := range windows {
		volume, err := m.liqWindowVolume(ctx, symbol, window, endTime)
		if err != nil {
			continue
		}
		volumeByWindow[window] = volume
	}
	return volumeByWindow
}

func (m *FuturesMarket) liqWindowVolume(ctx context.Context, symbol, window string, now time.Time) (float64, error) {
	klines, err := m.Klines(ctx, symbol, window, 2)
	if err != nil {
		return 0, err
	}
	if len(klines) == 0 {
		return 0, nil
	}
	duration, err := parseWindowDuration(window)
	if err != nil {
		return 0, err
	}
	return lastClosedVolume(klines, duration, now), nil
}

func lastClosedVolume(klines []snapshot.Candle, duration time.Duration, now time.Time) float64 {
	if len(klines) == 0 {
		return 0
	}
	last := klines[len(klines)-1]
	if candleClosed(last.OpenTime, duration, now) {
		return last.Volume
	}
	if len(klines) > 1 {
		return klines[len(klines)-2].Volume
	}
	return 0
}

func candleClosed(openTimeMs int64, duration time.Duration, now time.Time) bool {
	if openTimeMs <= 0 || duration <= 0 {
		return false
	}
	openTime := time.UnixMilli(openTimeMs).UTC()
	return !openTime.Add(duration).After(now)
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
	if err := m.validateClient(); err != nil {
		return 0, err
	}
	ctx, cancel := m.requestContext(ctx)
	defer cancel()
	if strings.TrimSpace(symbol) == "" {
		return 0, market.ValidationErrorf("symbol is required")
	}
	res, err := m.Client.NewPremiumIndexService().Symbol(symbol).Do(ctx)
	if err != nil {
		return 0, market.ExternalError(err, "binance_premium_index_failed")
	}
	for _, entry := range res {
		if entry == nil {
			continue
		}
		if strings.EqualFold(entry.Symbol, symbol) {
			val, err := parseFloat(entry.LastFundingRate)
			if err != nil {
				return 0, err
			}
			return val, nil
		}
	}
	if len(res) > 0 && res[0] != nil {
		val, err := parseFloat(res[0].LastFundingRate)
		if err != nil {
			return 0, err
		}
		return val, nil
	}
	return 0, market.UnavailableErrorf("funding rate not available for %s", symbol)
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
