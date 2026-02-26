package features

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"brale-core/internal/snapshot"

	talib "github.com/markcheno/go-talib"
)

const indicatorCompressVersion = "indicator_compress_v1"

type IndicatorCompressOptions struct {
	EMAFast    int
	EMAMid     int
	EMASlow    int
	RSIPeriod  int
	ATRPeriod  int
	MACDFast   int
	MACDSlow   int
	MACDSignal int
	LastN      int
	Pretty     bool
	SkipEMA    bool
	SkipRSI    bool
	SkipMACD   bool
}

func DefaultIndicatorCompressOptions() IndicatorCompressOptions {
	return IndicatorCompressOptions{
		EMAFast:    21,
		EMAMid:     50,
		EMASlow:    200,
		RSIPeriod:  14,
		ATRPeriod:  14,
		MACDFast:   12,
		MACDSlow:   26,
		MACDSignal: 9,
		LastN:      3,
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
	PriceTimestamp string  `json:"price_timestamp"`
}

type indicatorData struct {
	EMAFast *emaSnapshot  `json:"ema_fast,omitempty"`
	EMAMid  *emaSnapshot  `json:"ema_mid,omitempty"`
	EMASlow *emaSnapshot  `json:"ema_slow,omitempty"`
	MACD    *macdSnapshot `json:"macd,omitempty"`
	RSI     *rsiSnapshot  `json:"rsi,omitempty"`
	ATR     *atrSnapshot  `json:"atr,omitempty"`
	OBV     *obvSnapshot  `json:"obv,omitempty"`
}

type emaSnapshot struct {
	Latest       float64   `json:"latest"`
	LastN        []float64 `json:"last_n,omitempty"`
	DeltaToPrice float64   `json:"delta_to_price"`
	DeltaPct     float64   `json:"delta_pct"`
}

type macdSnapshot struct {
	DIF             float64         `json:"dif"`
	DEA             float64         `json:"dea"`
	Histogram       *seriesSnapshot `json:"histogram,omitempty"`
	Slope           *float64        `json:"slope,omitempty"`
	NormalizedSlope *float64        `json:"normalized_slope,omitempty"`
	SlopeState      string          `json:"slope_state,omitempty"`
	Signal          string          `json:"signal,omitempty"`
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

type seriesSnapshot struct {
	Last []float64 `json:"last_n,omitempty"`
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
	market := buildIndicatorMarket(symbol, interval, last)
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

func buildIndicatorMarket(symbol, interval string, last snapshot.Candle) indicatorMarket {
	return indicatorMarket{
		Symbol:         strings.ToUpper(strings.TrimSpace(symbol)),
		Interval:       strings.ToLower(strings.TrimSpace(interval)),
		CurrentPrice:   roundFloat(last.Close, 4),
		PriceTimestamp: candleTimestamp(last),
	}
}

func buildIndicatorData(closes, highs, lows, volumes []float64, lastClose float64, opts IndicatorCompressOptions) indicatorData {
	data := indicatorData{}
	maxPeriod := maxIndicatorPeriod(len(closes))
	if !opts.SkipEMA && maxPeriod > 0 {
		if period := clampIndicatorPeriod(opts.EMAFast, maxPeriod); period > 0 {
			if ema := buildEMASnapshot(sanitizeSeries(talib.Ema(closes, period)), lastClose, opts.LastN); ema != nil {
				data.EMAFast = ema
			}
		}
		if period := clampIndicatorPeriod(opts.EMAMid, maxPeriod); period > 0 {
			if ema := buildEMASnapshot(sanitizeSeries(talib.Ema(closes, period)), lastClose, opts.LastN); ema != nil {
				data.EMAMid = ema
			}
		}
		if period := clampIndicatorPeriod(opts.EMASlow, maxPeriod); period > 0 {
			if ema := buildEMASnapshot(sanitizeSeries(talib.Ema(closes, period)), lastClose, opts.LastN); ema != nil {
				data.EMASlow = ema
			}
		}
	}
	if !opts.SkipMACD && maxPeriod > 0 {
		fast := clampIndicatorPeriod(opts.MACDFast, maxPeriod)
		slow := clampIndicatorPeriod(opts.MACDSlow, maxPeriod)
		signal := clampIndicatorPeriod(opts.MACDSignal, maxPeriod)
		if fast > 0 && slow > 0 && signal > 0 {
			if macd := buildMACDSnapshot(closes, fast, slow, signal, opts.LastN); macd != nil {
				data.MACD = macd
			}
		}
	}
	if !opts.SkipRSI && maxPeriod > 0 {
		if period := clampIndicatorPeriod(opts.RSIPeriod, maxPeriod); period > 0 {
			if rsi := buildRSISnapshot(sanitizeSeries(talib.Rsi(closes, period)), opts.LastN); rsi != nil {
				data.RSI = rsi
			}
		}
	}
	if maxPeriod > 0 {
		if period := clampIndicatorPeriod(opts.ATRPeriod, maxPeriod); period > 0 {
			if atr := buildATRSnapshot(sanitizeSeries(talib.Atr(highs, lows, closes, period)), opts.LastN); atr != nil {
				data.ATR = atr
			}
		}
	}
	if obv := buildOBVSnapshot(closes, volumes); obv != nil {
		data.OBV = obv
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
	if opts.MACDFast <= 0 {
		opts.MACDFast = def.MACDFast
	}
	if opts.MACDSlow <= 0 {
		opts.MACDSlow = def.MACDSlow
	}
	if opts.MACDSignal <= 0 {
		opts.MACDSignal = def.MACDSignal
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

func buildMACDSnapshot(closes []float64, fast, slow, signal, tail int) *macdSnapshot {
	if len(closes) == 0 {
		return nil
	}
	macdSeries, signalSeries, histSeries := talib.Macd(closes, fast, slow, signal)
	mSeries := sanitizeSeries(macdSeries)
	sSeries := sanitizeSeries(signalSeries)
	hSeries := sanitizeSeries(histSeries)
	if len(mSeries) == 0 || len(sSeries) == 0 || len(hSeries) == 0 {
		return nil
	}
	histLast := roundSeriesTail(hSeries, tail)
	var hist *seriesSnapshot
	if len(histLast) > 0 {
		hist = &seriesSnapshot{Last: histLast}
	}
	ms := &macdSnapshot{
		DIF:       roundFloat(mSeries[len(mSeries)-1], 4),
		DEA:       roundFloat(sSeries[len(sSeries)-1], 4),
		Histogram: hist,
	}
	if slope, norm := computeSlopeSeries(histLast); slope != nil {
		ms.Slope = slope
		ms.NormalizedSlope = norm
		ms.SlopeState = indicatorSlopeState(norm)
	}
	if sig := macdSignal(mSeries, sSeries, hSeries); sig != "" {
		ms.Signal = sig
	}
	return ms
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

func macdSignal(dif, dea, hist []float64) string {
	n := minInt(len(dif), minInt(len(dea), len(hist)))
	if n < 2 {
		return ""
	}
	d1, d0 := dif[n-1], dif[n-2]
	e1, e0 := dea[n-1], dea[n-2]
	h1, h0 := hist[n-1], hist[n-2]

	crossZeroUp := h0 <= 0 && h1 > 0
	crossZeroDown := h0 >= 0 && h1 < 0
	golden := d0 <= e0 && d1 > e1
	dead := d0 >= e0 && d1 < e1

	switch {
	case golden && crossZeroUp:
		return "golden_cross_zero_up"
	case dead && crossZeroDown:
		return "dead_cross_zero_down"
	case golden:
		return "golden_cross"
	case dead:
		return "dead_cross"
	case crossZeroUp:
		return "zero_up"
	case crossZeroDown:
		return "zero_down"
	default:
		return "flat"
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxIndicatorPeriod(length int) int {
	maxPeriod := length - 2
	if maxPeriod < 1 {
		return 0
	}
	return maxPeriod
}

func clampIndicatorPeriod(period, maxPeriod int) int {
	if period <= 0 || maxPeriod <= 0 {
		return 0
	}
	if period > maxPeriod {
		return maxPeriod
	}
	return period
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
