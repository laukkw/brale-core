package features

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"brale-core/internal/market"
	"brale-core/internal/snapshot"
)

type MechanicsCompressOptions struct {
	Pretty               bool
	Metrics              *market.MetricsService
	Sentiment            *market.SentimentService
	FearGreed            *market.FearGreedService
	FeatureFlagsExplicit bool
	EnableOI             bool
	EnableFunding        bool
	EnableLongShort      bool
	EnableFearGreed      bool
	EnableLiquidations   bool
	EnableCVD            bool
	EnableSentiment      bool
	EnableFutSentiment   bool
}

type MechanicsCompressedInput struct {
	Symbol                 string                      `json:"symbol"`
	Timestamp              string                      `json:"timestamp"`
	OI                     *oiPayload                  `json:"oi,omitempty"`
	OIHistory              map[string]oiHistoryPayload `json:"oi_history,omitempty"`
	Funding                *fundingPayload             `json:"funding,omitempty"`
	LongShortByInterval    map[string]longShortPayload `json:"long_short_by_interval,omitempty"`
	CVDByInterval          map[string]cvdPayload       `json:"cvd_by_interval,omitempty"`
	SentimentByInterval    map[string]sentimentPayload `json:"sentiment_by_interval,omitempty"`
	FearGreedHistory       []fearGreedHistoryPoint     `json:"fear_greed_history,omitempty"`
	FearGreedNextUpdateSec int64                       `json:"fear_greed_next_update_sec,omitempty"`
	FearGreed              *fearGreedPayload           `json:"fear_greed,omitempty"`
	Liquidations           *liqPayload                 `json:"liquidations,omitempty"`
	LiquidationsByWindow   map[string]liqWindowPayload `json:"liquidations_by_window,omitempty"`
	LiquidationSource      *liqSourcePayload           `json:"liquidation_source,omitempty"`
	FuturesSentiment       *futuresSentimentPayload    `json:"futures_sentiment,omitempty"`
}

type oiPayload struct {
	Value          float64 `json:"value"`
	Timestamp      string  `json:"timestamp,omitempty"`
	Price          float64 `json:"price,omitempty"`
	PriceTimestamp string  `json:"price_timestamp,omitempty"`
}

type fundingPayload struct {
	Rate      float64 `json:"rate"`
	Timestamp string  `json:"timestamp,omitempty"`
}

type longShortPayload struct {
	Ratio     float64 `json:"ratio"`
	Timestamp string  `json:"timestamp,omitempty"`
}

type cvdPayload struct {
	Value      float64 `json:"value"`
	Momentum   float64 `json:"momentum"`
	Normalized float64 `json:"normalized"`
	Divergence string  `json:"divergence,omitempty"`
	PeakFlip   string  `json:"peak_flip,omitempty"`
	Timestamp  string  `json:"timestamp,omitempty"`
}

type oiHistoryPayload struct {
	Value          float64 `json:"value"`
	ChangePct      float64 `json:"change_pct,omitempty"`
	Price          float64 `json:"price,omitempty"`
	PriceChangePct float64 `json:"price_change_pct,omitempty"`
}

type sentimentPayload struct {
	Score int    `json:"score"`
	Tag   string `json:"tag,omitempty"`
}

type fearGreedHistoryPoint struct {
	Value          int    `json:"value"`
	Classification string `json:"classification,omitempty"`
	Timestamp      string `json:"timestamp,omitempty"`
}

type fearGreedPayload struct {
	Value     float64 `json:"value"`
	Timestamp string  `json:"timestamp,omitempty"`
}

type liqPayload struct {
	Volume    float64 `json:"volume"`
	Timestamp string  `json:"timestamp,omitempty"`
}

type liqRelPayload struct {
	VolOverOI     float64 `json:"vol_over_oi,omitempty"`
	VolOverVolume float64 `json:"vol_over_volume,omitempty"`
	ZScore        float64 `json:"z_score,omitempty"`
	Spike         bool    `json:"spike,omitempty"`
}

type liqPriceBinPayload struct {
	Bps       int     `json:"bps"`
	LongVol   float64 `json:"long_vol"`
	ShortVol  float64 `json:"short_vol"`
	TotalVol  float64 `json:"total_vol"`
	Imbalance float64 `json:"imbalance"`
}

type liqWindowPayload struct {
	LongVol      float64              `json:"long_vol"`
	ShortVol     float64              `json:"short_vol"`
	TotalVol     float64              `json:"total_vol"`
	Imbalance    float64              `json:"imbalance"`
	SampleCount  int                  `json:"sample_count,omitempty"`
	CoverageSec  int64                `json:"coverage_sec,omitempty"`
	Status       string               `json:"status,omitempty"`
	Complete     bool                 `json:"complete,omitempty"`
	PriceBinsBps []int                `json:"price_bins_bps,omitempty"`
	Bins         []liqPriceBinPayload `json:"bins,omitempty"`
	Rel          *liqRelPayload       `json:"rel,omitempty"`
}

type liqSourcePayload struct {
	Source          string `json:"source,omitempty"`
	Coverage        string `json:"coverage,omitempty"`
	Status          string `json:"status,omitempty"`
	StreamConnected bool   `json:"stream_connected,omitempty"`
	CoverageSec     int64  `json:"coverage_sec,omitempty"`
	SampleCount     int    `json:"sample_count,omitempty"`
	LastEventAgeSec int64  `json:"last_event_age_sec,omitempty"`
	Complete        bool   `json:"complete,omitempty"`
}

type futuresSentimentPayload struct {
	TopTraderLSR           float64 `json:"top_trader_lsr,omitempty"`
	LSRatio                float64 `json:"ls_ratio,omitempty"`
	TakerLongShortVolRatio float64 `json:"taker_long_short_vol_ratio,omitempty"`
	Timestamp              string  `json:"timestamp,omitempty"`
}

const maxLiquidationBins = 20

func BuildMechanicsCompressedJSON(ctx context.Context, symbol string, snap snapshot.MarketSnapshot, opts MechanicsCompressOptions) (string, error) {
	payload, err := BuildMechanicsCompressed(ctx, symbol, snap, opts)
	if err != nil {
		return "", err
	}
	if opts.Pretty {
		raw, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return "", err
		}
		return string(raw), nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func BuildMechanicsSnapshot(ctx context.Context, symbol string, snap snapshot.MarketSnapshot, opts MechanicsCompressOptions) (MechanicsSnapshot, error) {
	payload, err := BuildMechanicsCompressed(ctx, symbol, snap, opts)
	if err != nil {
		return MechanicsSnapshot{}, err
	}
	raw, err := buildMechanicsStateRaw(payload, opts.Pretty)
	if err != nil {
		return MechanicsSnapshot{}, err
	}
	return MechanicsSnapshot{Symbol: symbol, RawJSON: []byte(raw)}, nil
}

func BuildMechanicsCompressed(ctx context.Context, symbol string, snap snapshot.MarketSnapshot, opts MechanicsCompressOptions) (MechanicsCompressedInput, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	opts = normalizeMechanicsCompressOptions(opts)
	key := strings.ToUpper(strings.TrimSpace(symbol))
	out := MechanicsCompressedInput{
		Symbol:    key,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	applySnapshotFields(&out, snap, symbol, opts)
	applyMetricsSnapshot(&out, symbol, snap, opts)
	applyCurrentOIPrice(&out, snap, symbol)
	applyKlineDerived(ctx, &out, symbol, snap, opts)
	applyFearGreed(ctx, &out, snap, opts)
	applyFuturesSentiment(&out, snap, opts)
	if !hasMechanicsData(out) {
		return out, fmt.Errorf("mechanics: no data for symbol %s", symbol)
	}
	return out, nil
}

func normalizeMechanicsCompressOptions(opts MechanicsCompressOptions) MechanicsCompressOptions {
	if opts.FeatureFlagsExplicit || hasAnyMechanicsFeatureFlag(opts) {
		return opts
	}
	opts.EnableOI = true
	opts.EnableFunding = true
	opts.EnableLongShort = true
	opts.EnableFearGreed = true
	opts.EnableLiquidations = true
	opts.EnableCVD = true
	opts.EnableFutSentiment = true
	opts.EnableSentiment = opts.Sentiment != nil
	return opts
}

func hasAnyMechanicsFeatureFlag(opts MechanicsCompressOptions) bool {
	return opts.EnableOI ||
		opts.EnableFunding ||
		opts.EnableLongShort ||
		opts.EnableFearGreed ||
		opts.EnableLiquidations ||
		opts.EnableCVD ||
		opts.EnableSentiment ||
		opts.EnableFutSentiment
}

func applySnapshotFields(out *MechanicsCompressedInput, snap snapshot.MarketSnapshot, symbol string, opts MechanicsCompressOptions) {
	if out == nil {
		return
	}
	if opts.EnableOI && snap.OI != nil {
		if oi, ok := snap.OI[symbol]; ok {
			out.OI = &oiPayload{Value: roundFloat(oi.Value, 4), Timestamp: formatUnixTimestamp(oi.Timestamp)}
		}
	}
	if opts.EnableFunding && snap.Funding != nil {
		if f, ok := snap.Funding[symbol]; ok {
			out.Funding = &fundingPayload{Rate: roundFloat(f.Rate, 6), Timestamp: formatUnixTimestamp(f.Timestamp)}
		}
	}
	if opts.EnableLongShort && snap.LongShort != nil {
		if byInterval, ok := snap.LongShort[symbol]; ok {
			longShortByInterval := make(map[string]longShortPayload, len(byInterval))
			for iv, ls := range byInterval {
				longShortByInterval[iv] = longShortPayload{Ratio: roundFloat(ls.LongShortRatio, 4), Timestamp: formatUnixTimestamp(ls.Timestamp)}
			}
			if len(longShortByInterval) > 0 {
				out.LongShortByInterval = longShortByInterval
			}
		}
	}
	if opts.EnableLiquidations && snap.Liquidations != nil {
		if l, ok := snap.Liquidations[symbol]; ok {
			out.Liquidations = &liqPayload{
				Volume:    roundFloat(l.Volume, 4),
				Timestamp: formatUnixTimestamp(l.Timestamp),
			}
		}
	}
	if opts.EnableLiquidations && snap.LiquidationsByWindow != nil {
		if byWindow, ok := snap.LiquidationsByWindow[symbol]; ok {
			payload := buildLiquidationsByWindowPayloads(byWindow)
			if len(payload) > 0 {
				out.LiquidationsByWindow = payload
			}
		}
	}
	if opts.EnableLiquidations && snap.LiquidationSource != nil {
		if src, ok := snap.LiquidationSource[symbol]; ok {
			out.LiquidationSource = &liqSourcePayload{
				Source:          strings.TrimSpace(src.Source),
				Coverage:        strings.TrimSpace(src.Coverage),
				Status:          strings.TrimSpace(src.Status),
				StreamConnected: src.StreamConnected,
				CoverageSec:     src.CoverageSec,
				SampleCount:     src.SampleCount,
				LastEventAgeSec: src.LastEventAgeSec,
				Complete:        src.Complete,
			}
		}
	}
	if opts.EnableFearGreed && out.FearGreed == nil && snap.FearGreed != nil {
		out.FearGreed = &fearGreedPayload{
			Value:     snap.FearGreed.Value,
			Timestamp: formatUnixTimestamp(snap.FearGreed.Timestamp),
		}
	}
}

func applyMetricsSnapshot(out *MechanicsCompressedInput, symbol string, snap snapshot.MarketSnapshot, opts MechanicsCompressOptions) {
	if out == nil || opts.Metrics == nil || !opts.EnableOI {
		return
	}
	data, ok := opts.Metrics.Get(symbol)
	if !ok {
		return
	}
	if out.OI == nil && data.OI > 0 {
		out.OI = &oiPayload{Value: roundFloat(data.OI, 4), Timestamp: formatTimeRFC3339(data.LastUpdate)}
	}
	if len(data.OIHistory) == 0 {
		return
	}
	current := data.OI
	if out.OI != nil {
		current = out.OI.Value
	}
	byInterval := snap.Klines[symbol]
	oiHistory := make(map[string]oiHistoryPayload, len(data.OIHistory))
	for tf, val := range data.OIHistory {
		entry := oiHistoryPayload{Value: roundFloat(val, 4)}
		if current > 0 && val > 0 {
			entry.ChangePct = roundFloat((current-val)/val*100, 2)
		}
		if val > 0 {
			duration, ok := parseTimeframeDuration(tf)
			if ok {
				targetTime := data.LastUpdate.Add(-duration)
				if candles, exists := byInterval[tf]; exists && len(candles) > 0 {
					if price, _, ok := priceAtOrBefore(candles, targetTime); ok {
						entry.Price = roundFloat(price, 4)
						if currentPrice, _, ok := latestCandleClose(candles); ok && price > 0 {
							entry.PriceChangePct = roundFloat((currentPrice-price)/price*100, 2)
						}
					}
				}
			}
		}
		oiHistory[tf] = entry
	}
	out.OIHistory = oiHistory
}

func applyCurrentOIPrice(out *MechanicsCompressedInput, snap snapshot.MarketSnapshot, symbol string) {
	if out == nil || out.OI == nil {
		return
	}
	byInterval, ok := snap.Klines[symbol]
	if !ok {
		return
	}
	price, ts, ok := latestCloseAcrossIntervals(byInterval)
	if !ok || price <= 0 {
		return
	}
	out.OI.Price = roundFloat(price, 4)
	out.OI.PriceTimestamp = formatUnixTimestamp(ts)
}

func parseTimeframeDuration(tf string) (time.Duration, bool) {
	norm := strings.ToLower(strings.TrimSpace(tf))
	if norm == "" {
		return 0, false
	}
	switch {
	case strings.HasSuffix(norm, "m"):
		minutes, err := strconv.Atoi(strings.TrimSuffix(norm, "m"))
		if err != nil || minutes <= 0 {
			return 0, false
		}
		return time.Duration(minutes) * time.Minute, true
	case strings.HasSuffix(norm, "h"):
		hours, err := strconv.Atoi(strings.TrimSuffix(norm, "h"))
		if err != nil || hours <= 0 {
			return 0, false
		}
		return time.Duration(hours) * time.Hour, true
	case strings.HasSuffix(norm, "d"):
		days, err := strconv.Atoi(strings.TrimSuffix(norm, "d"))
		if err != nil || days <= 0 {
			return 0, false
		}
		return time.Duration(days) * 24 * time.Hour, true
	default:
		return 0, false
	}
}

func priceAtOrBefore(candles []snapshot.Candle, target time.Time) (float64, int64, bool) {
	if len(candles) == 0 {
		return 0, 0, false
	}
	targetTs := target.UnixMilli()
	for i := len(candles) - 1; i >= 0; i-- {
		point := candles[i]
		if point.OpenTime <= targetTs {
			return point.Close, point.OpenTime, true
		}
	}
	return 0, 0, false
}

func latestCandleClose(candles []snapshot.Candle) (float64, int64, bool) {
	if len(candles) == 0 {
		return 0, 0, false
	}
	last := candles[len(candles)-1]
	return last.Close, last.OpenTime, true
}

func latestCloseAcrossIntervals(byInterval map[string][]snapshot.Candle) (float64, int64, bool) {
	var best snapshot.Candle
	var bestTs int64
	for _, candles := range byInterval {
		if len(candles) == 0 {
			continue
		}
		last := candles[len(candles)-1]
		if last.OpenTime >= bestTs {
			best = last
			bestTs = last.OpenTime
		}
	}
	if bestTs == 0 {
		return 0, 0, false
	}
	return best.Close, best.OpenTime, true
}

func applyKlineDerived(ctx context.Context, out *MechanicsCompressedInput, symbol string, snap snapshot.MarketSnapshot, opts MechanicsCompressOptions) {
	if out == nil {
		return
	}
	byInterval, ok := snap.Klines[symbol]
	if !ok {
		return
	}
	if opts.EnableCVD {
		cvdByInterval := buildCVDByInterval(byInterval)
		if len(cvdByInterval) > 0 {
			out.CVDByInterval = cvdByInterval
		}
	}
	if !opts.EnableSentiment || opts.Sentiment == nil {
		return
	}
	sentimentByInterval := buildSentimentByInterval(ctx, symbol, byInterval, opts.Sentiment)
	if len(sentimentByInterval) > 0 {
		out.SentimentByInterval = sentimentByInterval
	}
}

func buildCVDByInterval(byInterval map[string][]snapshot.Candle) map[string]cvdPayload {
	cvdByInterval := make(map[string]cvdPayload)
	for iv, candles := range byInterval {
		if len(candles) == 0 {
			continue
		}
		metrics, ok := market.ComputeCVD(candles)
		if !ok {
			continue
		}
		last := candles[len(candles)-1]
		cvdByInterval[iv] = cvdPayload{
			Value:      roundFloat(metrics.Value, 4),
			Momentum:   roundFloat(metrics.Momentum, 4),
			Normalized: roundFloat(metrics.Normalized, 4),
			Divergence: strings.TrimSpace(metrics.Divergence),
			PeakFlip:   strings.TrimSpace(metrics.PeakFlip),
			Timestamp:  formatUnixTimestamp(last.OpenTime),
		}
	}
	if len(cvdByInterval) == 0 {
		return nil
	}
	return cvdByInterval
}

func buildSentimentByInterval(ctx context.Context, symbol string, byInterval map[string][]snapshot.Candle, sentiment *market.SentimentService) map[string]sentimentPayload {
	sentimentByInterval := make(map[string]sentimentPayload)
	for iv, candles := range byInterval {
		if len(candles) == 0 {
			continue
		}
		if data, ok := sentiment.Calculate(ctx, symbol, iv, candles); ok {
			sentimentByInterval[iv] = sentimentPayload{
				Score: data.Score,
				Tag:   strings.TrimSpace(data.Tag),
			}
		}
	}
	if len(sentimentByInterval) == 0 {
		return nil
	}
	return sentimentByInterval
}

func applyFearGreed(ctx context.Context, out *MechanicsCompressedInput, snap snapshot.MarketSnapshot, opts MechanicsCompressOptions) {
	if out == nil || !opts.EnableFearGreed || opts.FearGreed == nil {
		return
	}
	data, ok := opts.FearGreed.Get()
	if !ok {
		opts.FearGreed.RefreshIfStale(ctx)
		data, ok = opts.FearGreed.Get()
	}
	if !ok {
		return
	}
	if !data.Timestamp.IsZero() {
		out.FearGreed = &fearGreedPayload{
			Value:     float64(data.Value),
			Timestamp: formatTimeRFC3339(data.Timestamp),
		}
	}
	if len(data.History) > 0 {
		history := make([]fearGreedHistoryPoint, 0, len(data.History))
		for _, pt := range data.History {
			if pt.Timestamp.IsZero() {
				continue
			}
			history = append(history, fearGreedHistoryPoint{
				Value:          pt.Value,
				Classification: strings.TrimSpace(pt.Classification),
				Timestamp:      formatTimeRFC3339(pt.Timestamp),
			})
		}
		if len(history) > 0 {
			out.FearGreedHistory = history
		}
	}
	if data.TimeUntilUpdate > 0 {
		out.FearGreedNextUpdateSec = int64(data.TimeUntilUpdate.Seconds())
	}
	if out.FearGreed == nil && snap.FearGreed != nil {
		out.FearGreed = &fearGreedPayload{
			Value:     snap.FearGreed.Value,
			Timestamp: formatUnixTimestamp(snap.FearGreed.Timestamp),
		}
	}
}

func applyFuturesSentiment(out *MechanicsCompressedInput, snap snapshot.MarketSnapshot, opts MechanicsCompressOptions) {
	if out == nil || !opts.EnableFutSentiment {
		return
	}
	fs := futuresSentimentPayload{}
	if snap.LongShort != nil {
		if byInterval, ok := snap.LongShort[out.Symbol]; ok {
			if latest := pickLatestLSR(byInterval); latest != nil {
				fs.LSRatio = roundFloat(latest.LongShortRatio, 4)
				fs.Timestamp = formatUnixTimestamp(latest.Timestamp)
			}
		}
	}
	latestRatio := pickLatestTakerRatio(snap.Klines[out.Symbol])
	if latestRatio != nil {
		fs.TakerLongShortVolRatio = roundFloat(*latestRatio, 4)
	}
	if fs.LSRatio != 0 || fs.TakerLongShortVolRatio != 0 {
		out.FuturesSentiment = &fs
	}
}

func pickLatestLSR(byInterval map[string]snapshot.LSRBlock) *snapshot.LSRBlock {
	var latest *snapshot.LSRBlock
	var bestTs int64
	for _, lsr := range byInterval {
		if lsr.Timestamp >= bestTs {
			copy := lsr
			latest = &copy
			bestTs = lsr.Timestamp
		}
	}
	return latest
}

func pickLatestTakerRatio(byInterval map[string][]snapshot.Candle) *float64 {
	var (
		bestTs int64
		best   *float64
	)
	for _, candles := range byInterval {
		if len(candles) == 0 {
			continue
		}
		last := candles[len(candles)-1]
		buy := last.TakerBuyVolume
		sell := last.TakerSellVolume
		if sell == 0 {
			sell = last.Volume - buy
		}
		if sell <= 0 {
			continue
		}
		ratio := buy / sell
		if last.OpenTime >= bestTs {
			val := ratio
			best = &val
			bestTs = last.OpenTime
		}
	}
	return best
}

func buildLiquidationsByWindowPayloads(byWindow map[string]snapshot.LiqWindow) map[string]liqWindowPayload {
	if len(byWindow) == 0 {
		return nil
	}
	out := make(map[string]liqWindowPayload, len(byWindow))
	for window, win := range byWindow {
		payload := liqWindowPayload{
			LongVol:     roundFloat(win.LongVol, 4),
			ShortVol:    roundFloat(win.ShortVol, 4),
			TotalVol:    roundFloat(win.TotalVol, 4),
			Imbalance:   roundFloat(win.Imbalance, 4),
			SampleCount: win.SampleCount,
			CoverageSec: win.CoverageSec,
			Status:      strings.TrimSpace(win.Status),
			Complete:    win.Complete,
		}
		rel := liqRelPayload{
			VolOverOI:     roundFloat(win.Rel.VolOverOI, 4),
			VolOverVolume: roundFloat(win.Rel.VolOverVolume, 4),
			ZScore:        roundFloat(win.Rel.ZScore, 4),
			Spike:         win.Rel.Spike,
		}
		if rel.VolOverOI != 0 || rel.VolOverVolume != 0 || rel.ZScore != 0 || rel.Spike {
			payload.Rel = &rel
		}
		out[window] = payload
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func limitLiquidationBins(bins []snapshot.LiqPriceBin, bps []int, max int) ([]snapshot.LiqPriceBin, []int) {
	if len(bins) == 0 || len(bps) == 0 {
		return nil, nil
	}
	limit := len(bins)
	if len(bps) < limit {
		limit = len(bps)
	}
	if max > 0 && limit > max {
		limit = max
	}
	if limit <= 0 {
		return nil, nil
	}
	return bins[:limit], bps[:limit]
}

func hasMechanicsData(out MechanicsCompressedInput) bool {
	return out.OI != nil ||
		len(out.OIHistory) > 0 ||
		out.Funding != nil ||
		len(out.LongShortByInterval) > 0 ||
		len(out.CVDByInterval) > 0 ||
		len(out.SentimentByInterval) > 0 ||
		len(out.FearGreedHistory) > 0 ||
		out.FearGreed != nil ||
		out.Liquidations != nil ||
		len(out.LiquidationsByWindow) > 0 ||
		out.LiquidationSource != nil ||
		out.FuturesSentiment != nil
}

func formatUnixTimestamp(ts int64) string {
	if ts <= 0 {
		return ""
	}
	if ts >= 1_000_000_000_000 {
		return time.UnixMilli(ts).UTC().Format(time.RFC3339)
	}
	return time.Unix(ts, 0).UTC().Format(time.RFC3339)
}

func formatTimeRFC3339(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.UTC().Format(time.RFC3339)
}
