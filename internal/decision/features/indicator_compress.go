package features

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/snapshot"
)

const indicatorCompressVersion = "indicator_compress_v1"

// IndicatorCompressOptions controls indicator compression behavior.
// A fully zero-value struct means "use defaults"; partially specified options
// are validated and missing fields are not auto-filled.
type IndicatorCompressOptions struct {
	EMAFast          int
	EMAMid           int
	EMASlow          int
	RSIPeriod        int
	ATRPeriod        int
	STCFast          int
	STCSlow          int
	BBPeriod         int
	BBMultiplier     float64
	CHOPPeriod       int
	StochRSIPeriod   int
	AroonPeriod      int
	LastN            int
	Pretty           bool
	SkipEMA          bool
	SkipRSI          bool
	SkipATR          bool
	SkipOBV          bool
	SkipSTC          bool
	SkipBB           bool
	SkipCHOP         bool
	SkipStochRSI     bool
	SkipAroon        bool
	SkipTDSequential bool
}

func DefaultIndicatorCompressOptions() IndicatorCompressOptions {
	return IndicatorCompressOptions{
		EMAFast:        21,
		EMAMid:         50,
		EMASlow:        200,
		RSIPeriod:      14,
		ATRPeriod:      14,
		STCFast:        23,
		STCSlow:        50,
		BBPeriod:       20,
		BBMultiplier:   2.0,
		CHOPPeriod:     14,
		StochRSIPeriod: 14,
		AroonPeriod:    25,
		LastN:          3,
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
	EMAFast      *emaSnapshot          `json:"ema_fast,omitempty"`
	EMAMid       *emaSnapshot          `json:"ema_mid,omitempty"`
	EMASlow      *emaSnapshot          `json:"ema_slow,omitempty"`
	RSI          *rsiSnapshot          `json:"rsi,omitempty"`
	ATR          *atrSnapshot          `json:"atr,omitempty"`
	OBV          *obvSnapshot          `json:"obv,omitempty"`
	STC          *stcSnapshot          `json:"stc,omitempty"`
	BB           *bbSnapshot           `json:"bb,omitempty"`
	CHOP         *chopSnapshot         `json:"chop,omitempty"`
	StochRSI     *stochRSISnapshot     `json:"stoch_rsi,omitempty"`
	Aroon        *aroonSnapshot        `json:"aroon,omitempty"`
	TDSequential *tdSequentialSnapshot `json:"td_sequential,omitempty"`
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
	return BuildIndicatorCompressedJSONWithComputer(symbol, interval, candles, opts, nil)
}

func BuildIndicatorCompressedJSONWithComputer(symbol, interval string, candles []snapshot.Candle, opts IndicatorCompressOptions, computer IndicatorComputer) (string, error) {
	payload, err := BuildIndicatorCompressedInputWithComputer(symbol, interval, candles, opts, computer)
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

// BuildIndicatorCompressedInput converts candles into a structured indicator payload.
// Zero-value opts use DefaultIndicatorCompressOptions; partially specified opts
// must already be complete and valid.
func BuildIndicatorCompressedInput(symbol, interval string, candles []snapshot.Candle, opts IndicatorCompressOptions) (IndicatorCompressedInput, error) {
	return BuildIndicatorCompressedInputWithComputer(symbol, interval, candles, opts, nil)
}

func BuildIndicatorCompressedInputWithComputer(symbol, interval string, candles []snapshot.Candle, opts IndicatorCompressOptions, computer IndicatorComputer) (IndicatorCompressedInput, error) {
	if len(candles) == 0 {
		return IndicatorCompressedInput{}, fmt.Errorf("no candles")
	}
	var err error
	opts, err = resolveIndicatorCompressOptions(opts)
	if err != nil {
		return IndicatorCompressedInput{}, err
	}
	last, closes, highs, lows, volumes := buildIndicatorSeries(candles)
	meta := buildIndicatorMeta(last)
	market := buildIndicatorMarket(symbol, interval, candles)
	data, err := buildIndicatorData(closes, highs, lows, volumes, last.Close, opts, defaultIndicatorComputer(computer))
	if err != nil {
		return IndicatorCompressedInput{}, err
	}
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

func buildIndicatorData(closes, highs, lows, volumes []float64, lastClose float64, opts IndicatorCompressOptions, computer IndicatorComputer) (indicatorData, error) {
	data := indicatorData{}
	if !opts.SkipEMA {
		if opts.EMAFast > 0 && len(closes) >= config.EMARequiredBars(opts.EMAFast) {
			series, err := computer.ComputeEMA(closes, opts.EMAFast)
			if err != nil {
				return indicatorData{}, err
			}
			if ema := buildEMASnapshot(sanitizeSeries(series), lastClose, opts.LastN); ema != nil {
				data.EMAFast = ema
			}
		}
		if opts.EMAMid > 0 && len(closes) >= config.EMARequiredBars(opts.EMAMid) {
			series, err := computer.ComputeEMA(closes, opts.EMAMid)
			if err != nil {
				return indicatorData{}, err
			}
			if ema := buildEMASnapshot(sanitizeSeries(series), lastClose, opts.LastN); ema != nil {
				data.EMAMid = ema
			}
		}
		if opts.EMASlow > 0 && len(closes) >= config.EMARequiredBars(opts.EMASlow) {
			series, err := computer.ComputeEMA(closes, opts.EMASlow)
			if err != nil {
				return indicatorData{}, err
			}
			if ema := buildEMASnapshot(sanitizeSeries(series), lastClose, opts.LastN); ema != nil {
				data.EMASlow = ema
			}
		}
	}
	if !opts.SkipRSI {
		if opts.RSIPeriod > 0 && len(closes) >= config.RSIRequiredBars(opts.RSIPeriod) {
			rsiSeriesRaw, err := computer.ComputeRSI(closes, opts.RSIPeriod)
			if err != nil {
				return indicatorData{}, err
			}
			rsiSeries := sanitizeSeries(rsiSeriesRaw)
			if rsi := buildRSISnapshot(rsiSeries, opts.LastN); rsi != nil {
				data.RSI = rsi
			}
			// StochRSI is computed from the RSI series
			if !opts.SkipStochRSI && opts.StochRSIPeriod > 0 && len(rsiSeries) >= opts.StochRSIPeriod {
				stochSeries, err := computer.ComputeStochRSI(rsiSeries, opts.StochRSIPeriod)
				if err != nil {
					return indicatorData{}, err
				}
				if snap := buildStochRSISnapshot(sanitizeSeries(stochSeries)); snap != nil {
					data.StochRSI = snap
				}
			}
		}
	}
	if !opts.SkipATR && opts.ATRPeriod > 0 && len(closes) >= config.ATRRequiredBars(opts.ATRPeriod) {
		series, err := computer.ComputeATR(highs, lows, closes, opts.ATRPeriod)
		if err != nil {
			return indicatorData{}, err
		}
		if atr := buildATRSnapshot(sanitizeSeries(series), opts.LastN); atr != nil {
			data.ATR = atr
		}
	}
	if !opts.SkipOBV {
		obvSeries, err := computer.ComputeOBV(closes, volumes)
		if err != nil {
			return indicatorData{}, err
		}
		if obv := buildOBVSnapshot(obvSeries); obv != nil {
			data.OBV = obv
		}
	}
	if !opts.SkipSTC {
		requiredBars := config.STCRequiredBars(opts.STCFast, opts.STCSlow)
		if len(closes) >= requiredBars {
			stcSeries, err := computer.ComputeSTC(
				closes,
				opts.STCFast,
				opts.STCSlow,
				config.DefaultSTCKPeriod,
				config.DefaultSTCDPeriod,
			)
			if err != nil {
				return indicatorData{}, err
			}
			if stc := buildSTCSnapshot(sanitizeSeries(stcSeries), opts.LastN); stc != nil {
				data.STC = stc
			}
		}
	}
	// Bollinger Bands
	if !opts.SkipBB && opts.BBPeriod > 0 && len(closes) >= config.BBRequiredBars(opts.BBPeriod) {
		upper, middle, lower, err := computer.ComputeBB(closes, opts.BBPeriod, opts.BBMultiplier, opts.BBMultiplier)
		if err != nil {
			return indicatorData{}, err
		}
		if snap := buildBBSnapshot(upper, middle, lower, lastClose); snap != nil {
			data.BB = snap
		}
	}
	// Choppiness Index
	if !opts.SkipCHOP && opts.CHOPPeriod > 1 && len(closes) >= config.CHOPRequiredBars(opts.CHOPPeriod) {
		chopSeries, err := computer.ComputeCHOP(highs, lows, closes, opts.CHOPPeriod)
		if err != nil {
			return indicatorData{}, err
		}
		if snap := buildCHOPSnapshot(chopSeries); snap != nil {
			data.CHOP = snap
		}
	}
	// Aroon
	if !opts.SkipAroon && opts.AroonPeriod > 0 && len(highs) >= config.AroonRequiredBars(opts.AroonPeriod) {
		aroonUp, aroonDown, err := computer.ComputeAroon(highs, lows, opts.AroonPeriod)
		if err != nil {
			return indicatorData{}, err
		}
		if snap := buildAroonSnapshot(aroonUp, aroonDown); snap != nil {
			data.Aroon = snap
		}
	}
	// TD Sequential
	if !opts.SkipTDSequential && len(closes) > 4 {
		buySetup, sellSetup := computeTDSequential(closes)
		if snap := buildTDSequentialSnapshot(buySetup, sellSetup); snap != nil {
			data.TDSequential = snap
		}
	}
	return data, nil
}

func resolveIndicatorCompressOptions(opts IndicatorCompressOptions) (IndicatorCompressOptions, error) {
	if opts == (IndicatorCompressOptions{}) {
		return DefaultIndicatorCompressOptions(), nil
	}
	if opts.SkipRSI {
		opts.SkipStochRSI = true
	}
	if err := ValidateIndicatorCompressOptions(opts); err != nil {
		return IndicatorCompressOptions{}, err
	}
	return opts, nil
}

// ValidateIndicatorCompressOptions rejects partially invalid option sets.
func ValidateIndicatorCompressOptions(opts IndicatorCompressOptions) error {
	if !opts.SkipEMA {
		if opts.EMAFast <= 0 {
			return fmt.Errorf("indicator options ema_fast must be > 0 when EMA is enabled")
		}
		if opts.EMAMid <= 0 {
			return fmt.Errorf("indicator options ema_mid must be > 0 when EMA is enabled")
		}
		if opts.EMASlow <= 0 {
			return fmt.Errorf("indicator options ema_slow must be > 0 when EMA is enabled")
		}
		if !(opts.EMAFast < opts.EMAMid && opts.EMAMid < opts.EMASlow) {
			return fmt.Errorf("indicator options must satisfy ema_fast < ema_mid < ema_slow")
		}
	}
	if !opts.SkipRSI {
		if opts.RSIPeriod <= 0 {
			return fmt.Errorf("indicator options rsi_period must be > 0 when RSI is enabled")
		}
	}
	if opts.SkipRSI {
		opts.SkipStochRSI = true
	}
	if !opts.SkipStochRSI {
		if opts.SkipRSI {
			return fmt.Errorf("indicator options stoch_rsi requires RSI to be enabled")
		}
		if opts.StochRSIPeriod <= 0 {
			return fmt.Errorf("indicator options stoch_rsi_period must be > 0 when RSI is enabled")
		}
	}
	if !opts.SkipATR && opts.ATRPeriod <= 0 {
		return fmt.Errorf("indicator options atr_period must be > 0")
	}
	if !opts.SkipSTC {
		if opts.STCFast <= 0 {
			return fmt.Errorf("indicator options stc_fast must be > 0 when STC is enabled")
		}
		if opts.STCSlow <= 0 {
			return fmt.Errorf("indicator options stc_slow must be > 0 when STC is enabled")
		}
		if opts.STCFast >= opts.STCSlow {
			return fmt.Errorf("indicator options stc_fast must be < stc_slow")
		}
	}
	if !opts.SkipBB && opts.BBPeriod <= 1 {
		return fmt.Errorf("indicator options bb_period must be > 1")
	}
	if !opts.SkipBB && opts.BBMultiplier <= 0 {
		return fmt.Errorf("indicator options bb_multiplier must be > 0")
	}
	if !opts.SkipCHOP && opts.CHOPPeriod <= 1 {
		return fmt.Errorf("indicator options chop_period must be > 1")
	}
	if !opts.SkipAroon && opts.AroonPeriod <= 0 {
		return fmt.Errorf("indicator options aroon_period must be > 0")
	}
	if opts.LastN <= 0 {
		return fmt.Errorf("indicator options last_n must be > 0")
	}
	return nil
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

func computeOBVSeries(closes, volumes []float64) []float64 {
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
	return obv
}

func buildOBVSnapshot(series []float64) *obvSnapshot {
	if len(series) == 0 {
		return nil
	}
	last := series[len(series)-1]
	snap := &obvSnapshot{Value: roundFloat(last, 4)}
	window := len(series)
	if window > 30 {
		window = 30
	}
	if window > 1 {
		base := series[len(series)-window]
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
