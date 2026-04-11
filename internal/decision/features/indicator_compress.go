package features

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/snapshot"

	talib "github.com/markcheno/go-talib"
)

const indicatorCompressVersion = "indicator_compress_v1"

type IndicatorCompressOptions struct {
	EMAFast   int
	EMAMid    int
	EMASlow   int
	RSIPeriod int
	ATRPeriod int
	STCFast   int
	STCSlow   int
	LastN     int
	Pretty    bool
	SkipEMA   bool
	SkipRSI   bool
	SkipSTC   bool
}

func DefaultIndicatorCompressOptions() IndicatorCompressOptions {
	return IndicatorCompressOptions{
		EMAFast:   21,
		EMAMid:    50,
		EMASlow:   200,
		RSIPeriod: 14,
		ATRPeriod: 14,
		STCFast:   23,
		STCSlow:   50,
		LastN:     3,
	}
}

type IndicatorCompressedInput struct {
	Meta   indicatorMeta   `json:"_meta"`
	Market indicatorMarket `json:"market"`
	Data   indicatorData   `json:"data"`
}

type indicatorMeta struct {
	SeriesOrder  string           `json:"series_order"`
	SampledAt    string           `json:"sampled_at"`
	Version      string           `json:"version"`
	TimestampNow string           `json:"timestamp_now_ts,omitempty"`
	DataAgeSec   map[string]int64 `json:"data_age_sec,omitempty"`
}

type indicatorMarket struct {
	Symbol         string  `json:"symbol"`
	Interval       string  `json:"interval"`
	CurrentPrice   float64 `json:"current_price"`
	PreviousPrice  float64 `json:"previous_price,omitempty"`
	PriceTimestamp string  `json:"price_timestamp"`
}

type indicatorData struct {
	EMAFast *emaSnapshot `json:"ema_fast,omitempty"`
	EMAMid  *emaSnapshot `json:"ema_mid,omitempty"`
	EMASlow *emaSnapshot `json:"ema_slow,omitempty"`
	RSI     *rsiSnapshot `json:"rsi,omitempty"`
	ATR     *atrSnapshot `json:"atr,omitempty"`
	OBV     *obvSnapshot `json:"obv,omitempty"`
	STC     *stcSnapshot `json:"stc,omitempty"`
}

type emaSnapshot struct {
	Latest       float64   `json:"latest"`
	LastN        []float64 `json:"last_n,omitempty"`
	DeltaToPrice float64   `json:"delta_to_price"`
	DeltaPct     float64   `json:"delta_pct"`
}

type rsiSnapshot struct {
	Current         float64   `json:"current"`
	LastN           []float64 `json:"last_n,omitempty"`
	DistanceToHigh  float64   `json:"distance_to_high"`
	DistanceToLow   float64   `json:"distance_to_low"`
	Slope           *float64  `json:"slope,omitempty"`
	NormalizedSlope *float64  `json:"normalized_slope,omitempty"`
	SlopeState      string    `json:"slope_state,omitempty"`
}

type atrSnapshot struct {
	Latest    float64   `json:"latest"`
	LastN     []float64 `json:"last_n,omitempty"`
	ChangePct *float64  `json:"change_pct,omitempty"`
}

type obvSnapshot struct {
	Value      float64  `json:"value"`
	ChangeRate *float64 `json:"change_rate,omitempty"`
}

func BuildIndicatorCompressedJSON(symbol, interval string, candles []snapshot.Candle, opts IndicatorCompressOptions) (string, error) {
	payload, err := BuildIndicatorCompressedInput(symbol, interval, candles, opts)
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

func BuildIndicatorCompressedInput(symbol, interval string, candles []snapshot.Candle, opts IndicatorCompressOptions) (IndicatorCompressedInput, error) {
	if len(candles) == 0 {
		return IndicatorCompressedInput{}, fmt.Errorf("no candles")
	}
	opts = normalizeIndicatorCompressOptions(opts)
	last, closes, highs, lows, volumes := buildIndicatorSeries(candles)
	meta := buildIndicatorMeta(last)
	market := buildIndicatorMarket(symbol, interval, candles)
	data := buildIndicatorData(closes, highs, lows, volumes, last.Close, opts)
	return IndicatorCompressedInput{Meta: meta, Market: market, Data: data}, nil
}

func buildIndicatorSeries(candles []snapshot.Candle) (snapshot.Candle, []float64, []float64, []float64, []float64) {
	last := candles[len(candles)-1]
	closes := make([]float64, len(candles))
	highs := make([]float64, len(candles))
	lows := make([]float64, len(candles))
	volumes := make([]float64, len(candles))
	for i, c := range candles {
		closes[i] = c.Close
		highs[i] = c.High
		lows[i] = c.Low
		volumes[i] = c.Volume
	}
	return last, closes, highs, lows, volumes
}

func buildIndicatorMeta(last snapshot.Candle) indicatorMeta {
	meta := indicatorMeta{
		SeriesOrder:  "oldest_to_latest",
		SampledAt:    candleTimestamp(last),
		Version:      indicatorCompressVersion,
		TimestampNow: time.Now().UTC().Format(time.RFC3339),
	}
	if last.OpenTime > 0 {
		age := int64(time.Since(time.UnixMilli(last.OpenTime)).Seconds())
		if age < 0 {
			age = 0
		}
		meta.DataAgeSec = map[string]int64{"indicator": age}
	}
	return meta
}

func buildIndicatorMarket(symbol, interval string, candles []snapshot.Candle) indicatorMarket {
	last := candles[len(candles)-1]
	market := indicatorMarket{
		Symbol:         strings.ToUpper(strings.TrimSpace(symbol)),
		Interval:       strings.ToLower(strings.TrimSpace(interval)),
		CurrentPrice:   roundFloat(last.Close, 4),
		PriceTimestamp: candleTimestamp(last),
	}
	if len(candles) > 1 {
		market.PreviousPrice = roundFloat(candles[len(candles)-2].Close, 4)
	}
	return market
}

func buildIndicatorData(closes, highs, lows, volumes []float64, lastClose float64, opts IndicatorCompressOptions) indicatorData {
	data := indicatorData{}
	if !opts.SkipEMA {
		if opts.EMAFast > 0 && len(closes) >= config.EMARequiredBars(opts.EMAFast) {
			if ema := buildEMASnapshot(sanitizeSeries(talib.Ema(closes, opts.EMAFast)), lastClose, opts.LastN); ema != nil {
				data.EMAFast = ema
			}
		}
		if opts.EMAMid > 0 && len(closes) >= config.EMARequiredBars(opts.EMAMid) {
			if ema := buildEMASnapshot(sanitizeSeries(talib.Ema(closes, opts.EMAMid)), lastClose, opts.LastN); ema != nil {
				data.EMAMid = ema
			}
		}
		if opts.EMASlow > 0 && len(closes) >= config.EMARequiredBars(opts.EMASlow) {
			if ema := buildEMASnapshot(sanitizeSeries(talib.Ema(closes, opts.EMASlow)), lastClose, opts.LastN); ema != nil {
				data.EMASlow = ema
			}
		}
	}
	if !opts.SkipRSI {
		if opts.RSIPeriod > 0 && len(closes) >= config.RSIRequiredBars(opts.RSIPeriod) {
			if rsi := buildRSISnapshot(sanitizeSeries(talib.Rsi(closes, opts.RSIPeriod)), opts.LastN); rsi != nil {
				data.RSI = rsi
			}
		}
	}
	if opts.ATRPeriod > 0 && len(closes) >= config.ATRRequiredBars(opts.ATRPeriod) {
		if atr := buildATRSnapshot(sanitizeSeries(talib.Atr(highs, lows, closes, opts.ATRPeriod)), opts.LastN); atr != nil {
			data.ATR = atr
		}
	}
	if obv := buildOBVSnapshot(closes, volumes); obv != nil {
		data.OBV = obv
	}
	if !opts.SkipSTC {
		requiredBars := config.STCRequiredBars(opts.STCFast, opts.STCSlow)
		if len(closes) >= requiredBars {
			stcSeries := computeSTCSeries(
				closes,
				opts.STCFast,
				opts.STCSlow,
				config.DefaultSTCKPeriod,
				config.DefaultSTCDPeriod,
			)
			if stc := buildSTCSnapshot(sanitizeSeries(stcSeries), opts.LastN); stc != nil {
				data.STC = stc
			}
		}
	}
	return data
}

func normalizeIndicatorCompressOptions(opts IndicatorCompressOptions) IndicatorCompressOptions {
	def := DefaultIndicatorCompressOptions()
	if opts.EMAFast <= 0 {
		opts.EMAFast = def.EMAFast
	}
	if opts.EMAMid <= 0 {
		opts.EMAMid = def.EMAMid
	}
	if opts.EMASlow <= 0 {
		opts.EMASlow = def.EMASlow
	}
	if opts.RSIPeriod <= 0 {
		opts.RSIPeriod = def.RSIPeriod
	}
	if opts.ATRPeriod <= 0 {
		opts.ATRPeriod = def.ATRPeriod
	}
	if opts.STCFast <= 0 {
		opts.STCFast = def.STCFast
	}
	if opts.STCSlow <= 0 {
		opts.STCSlow = def.STCSlow
	}
	if opts.LastN <= 0 {
		opts.LastN = def.LastN
	}
	return opts
}

func buildEMASnapshot(series []float64, price float64, tail int) *emaSnapshot {
	if len(series) == 0 {
		return nil
	}
	latest := series[len(series)-1]
	delta := price - latest
	deltaPct := 0.0
	if latest != 0 {
		deltaPct = (delta / latest) * 100
	}
	return &emaSnapshot{
		Latest:       roundFloat(latest, 4),
		LastN:        roundSeriesTail(series, tail),
		DeltaToPrice: roundFloat(delta, 4),
		DeltaPct:     roundFloat(deltaPct, 4),
	}
}

func buildRSISnapshot(series []float64, tail int) *rsiSnapshot {
	if len(series) == 0 {
		return nil
	}
	maxVal, minVal := seriesBounds(series)
	cur := series[len(series)-1]
	rs := &rsiSnapshot{
		Current:        roundFloat(cur, 4),
		LastN:          roundSeriesTail(series, tail),
		DistanceToHigh: roundFloat(maxVal-cur, 4),
		DistanceToLow:  roundFloat(cur-minVal, 4),
	}
	if slope, norm := computeSlopeSeries(rs.LastN); slope != nil {
		rs.Slope = slope
		rs.NormalizedSlope = norm
		rs.SlopeState = indicatorSlopeState(norm)
	}
	return rs
}

func buildATRSnapshot(series []float64, tail int) *atrSnapshot {
	if len(series) == 0 {
		return nil
	}
	snap := &atrSnapshot{
		Latest: roundFloat(series[len(series)-1], 4),
		LastN:  roundSeriesTail(series, tail),
	}
	snap.ChangePct = computeChangePct(series)
	return snap
}

func buildOBVSnapshot(closes, volumes []float64) *obvSnapshot {
	if len(closes) == 0 || len(volumes) != len(closes) {
		return nil
	}
	obv := make([]float64, len(closes))
	for i := range closes {
		if i == 0 {
			obv[i] = volumes[i]
			continue
		}
		dir := 0.0
		if closes[i] > closes[i-1] {
			dir = 1.0
		} else if closes[i] < closes[i-1] {
			dir = -1.0
		}
		obv[i] = obv[i-1] + dir*volumes[i]
	}
	last := obv[len(obv)-1]
	snap := &obvSnapshot{Value: roundFloat(last, 4)}
	window := len(obv)
	if window > 30 {
		window = 30
	}
	if window > 1 {
		base := obv[len(obv)-window]
		if base != 0 {
			change := (last - base) / base
			snap.ChangeRate = floatPtr(roundFloat(change, 4))
		}
	}
	return snap
}

func floatPtr(v float64) *float64 {
	return &v
}

func sanitizeSeries(series []float64) []float64 {
	if len(series) == 0 {
		return nil
	}
	out := make([]float64, 0, len(series))
	for _, v := range series {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		out = append(out, roundFloat(v, 4))
	}
	return out
}

func seriesBounds(series []float64) (max float64, min float64) {
	max = -math.MaxFloat64
	min = math.MaxFloat64
	for _, v := range series {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		if v > max {
			max = v
		}
		if v < min {
			min = v
		}
	}
	if max == -math.MaxFloat64 {
		max = 0
	}
	if min == math.MaxFloat64 {
		min = 0
	}
	return
}

func roundSeriesTail(series []float64, tail int) []float64 {
	if tail <= 0 || len(series) == 0 {
		return nil
	}
	if tail > len(series) {
		tail = len(series)
	}
	start := len(series) - tail
	out := make([]float64, 0, tail)
	for _, v := range series[start:] {
		out = append(out, roundFloat(v, 4))
	}
	return out
}

func computeSlopeSeries(series []float64) (*float64, *float64) {
	if len(series) < 2 {
		return nil, nil
	}
	start := 0
	if len(series) > 5 {
		start = len(series) - 5
	}
	first := series[start]
	last := series[len(series)-1]
	steps := float64(len(series) - start - 1)
	if steps <= 0 {
		return nil, nil
	}
	delta := last - first
	raw := roundFloat(delta/steps, 4)
	var norm *float64
	if math.Abs(first) > 1e-9 {
		v := roundFloat((delta/math.Abs(first))*100/steps, 4)
		norm = &v
	}
	return &raw, norm
}

func computeChangePct(series []float64) *float64 {
	if len(series) < 2 {
		return nil
	}
	last := series[len(series)-1]
	prev := series[len(series)-2]
	if math.Abs(prev) <= 1e-9 {
		return nil
	}
	v := roundFloat(((last-prev)/prev)*100, 4)
	return &v
}

func indicatorSlopeState(norm *float64) string {
	if norm == nil {
		return ""
	}
	abs := math.Abs(*norm)
	switch {
	case abs < 0.1:
		return "FLAT"
	case abs < 0.4:
		return "MODERATE"
	default:
		return "STEEP"
	}
}
