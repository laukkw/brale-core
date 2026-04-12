package features

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"brale-core/internal/config"
	"brale-core/internal/pkg/pattern/cdl"
	"brale-core/internal/pkg/pattern/evidence"
	"brale-core/internal/pkg/pattern/geometry"
	"brale-core/internal/snapshot"

	talib "github.com/markcheno/go-talib"
)

// Trend compression payload migrated from brale/internal/decision/trend_compress.go.
// It preserves the original field names so prompts can reference structure_points,
// structure_candidates, recent_candles, and global_context.

type TrendCompressOptions struct {
	FractalSpan          int
	MaxStructurePoints   int
	DedupDistanceBars    int
	DedupATRFactor       float64
	SuperTrendPeriod     int
	SuperTrendMultiplier float64
	SkipSuperTrend       bool
	RSIPeriod            int
	ATRPeriod            int
	RecentCandles        int
	VolumeMAPeriod       int
	EMA20Period          int
	EMA50Period          int
	EMA200Period         int
	PatternMinScore      int
	PatternMaxDetected   int
	Pretty               bool
	IncludeCurrentRSI    bool
	IncludeStructureRSI  bool
}

const (
	structurePointHigh = "High"
	structurePointLow  = "Low"

	trendBiasBullish = "bullish"
	trendBiasBearish = "bearish"
	trendTypeNone    = "none"

	superTrendStateUp   = "UP"
	superTrendStateDown = "DOWN"
)

func DefaultTrendCompressOptions() TrendCompressOptions {
	return TrendCompressOptions{
		FractalSpan:          2,
		MaxStructurePoints:   8,
		DedupDistanceBars:    10,
		DedupATRFactor:       0.5,
		SuperTrendPeriod:     14,
		SuperTrendMultiplier: 2.5,
		RSIPeriod:            14,
		ATRPeriod:            14,
		RecentCandles:        5,
		VolumeMAPeriod:       20,
		EMA20Period:          20,
		EMA50Period:          50,
		EMA200Period:         200,
		PatternMinScore:      100,
		PatternMaxDetected:   3,
		Pretty:               false,
		IncludeCurrentRSI:    true,
		IncludeStructureRSI:  true,
	}
}

type TrendCompressedInput struct {
	Meta                TrendCompressedMeta       `json:"meta"`
	StructurePoints     []TrendStructurePoint     `json:"structure_points"`
	StructureCandidates []TrendStructureCandidate `json:"structure_candidates,omitempty"`
	RecentCandles       []TrendRecentCandle       `json:"recent_candles"`
	GlobalContext       TrendGlobalContext        `json:"global_context"`
	SuperTrend          *TrendSuperTrendSnapshot  `json:"supertrend,omitempty"`
	KeyLevels           *TrendKeyLevels           `json:"key_levels,omitempty"`
	BreakEvents         []TrendBreakEvent         `json:"break_events,omitempty"`
	BreakSummary        *TrendBreakSummary        `json:"break_summary,omitempty"`
	RawCandles          []TrendRawCandleOptional  `json:"raw_candles,omitempty"`
	Pattern             *evidence.Result          `json:"pattern,omitempty"`
	SMC                 TrendSMC                  `json:"smc"`
}

type TrendCompressedMeta struct {
	Symbol    string `json:"symbol"`
	Interval  string `json:"interval"`
	Timestamp string `json:"timestamp"`
}

type TrendStructurePoint struct {
	Idx   int      `json:"idx"`
	Type  string   `json:"type"`
	Price float64  `json:"price"`
	RSI   *float64 `json:"rsi,omitempty"`
}

type TrendRecentCandle struct {
	Idx int      `json:"idx"`
	O   float64  `json:"o"`
	H   float64  `json:"h"`
	L   float64  `json:"l"`
	C   float64  `json:"c"`
	V   float64  `json:"v"`
	RSI *float64 `json:"rsi,omitempty"`
}

type TrendGlobalContext struct {
	TrendSlope      float64  `json:"trend_slope"`
	NormalizedSlope float64  `json:"normalized_slope"`
	SlopeState      string   `json:"slope_state,omitempty"`
	Window          int      `json:"window,omitempty"`
	VolRatio        float64  `json:"vol_ratio"`
	EMA20           *float64 `json:"ema20,omitempty"`
	EMA50           *float64 `json:"ema50,omitempty"`
	EMA200          *float64 `json:"ema200,omitempty"`
}

type TrendSuperTrendSnapshot struct {
	Interval    string  `json:"interval"`
	State       string  `json:"state"`
	Level       float64 `json:"level"`
	DistancePct float64 `json:"distance_pct"`
}

type TrendKeyLevels struct {
	LastSwingHigh *TrendKeyLevel `json:"last_swing_high,omitempty"`
	LastSwingLow  *TrendKeyLevel `json:"last_swing_low,omitempty"`
}

type TrendKeyLevel struct {
	Price float64 `json:"price"`
	Idx   int     `json:"idx"`
}

type TrendBreakEvent struct {
	Type       string  `json:"type"`
	LevelPrice float64 `json:"level_price"`
	LevelIdx   int     `json:"level_idx"`
	BarIdx     int     `json:"bar_idx"`
	BarAge     int     `json:"bar_age"`
	Confirm    string  `json:"confirm"`
}

type TrendBreakSummary struct {
	LatestEventType       string   `json:"latest_event_type,omitempty"`
	LatestEventAge        *int     `json:"latest_event_age,omitempty"`
	LatestEventBarIdx     *int     `json:"latest_event_bar_idx,omitempty"`
	LatestEventLevelPrice *float64 `json:"latest_event_level_price,omitempty"`
	LatestEventLevelIdx   *int     `json:"latest_event_level_idx,omitempty"`
}

type TrendRawCandleOptional struct {
	Idx int     `json:"idx"`
	O   float64 `json:"o"`
	H   float64 `json:"h"`
	L   float64 `json:"l"`
	C   float64 `json:"c"`
	V   float64 `json:"v"`
}

type TrendStructureCandidate struct {
	Price      float64 `json:"price"`
	Type       string  `json:"type"`
	Source     string  `json:"source"`
	AgeCandles int     `json:"age_candles"`
	Window     int     `json:"window,omitempty"`
}

type TrendSMC struct {
	OrderBlock TrendOrderBlock `json:"order_block"`
	FVG        TrendFVG        `json:"fvg"`
}

type TrendOrderBlock struct {
	Type  string   `json:"type"`
	Upper *float64 `json:"upper,omitempty"`
	Lower *float64 `json:"lower,omitempty"`
}

type TrendFVG struct {
	Type      string   `json:"type"`
	GapTop    *float64 `json:"gap_top,omitempty"`
	GapBottom *float64 `json:"gap_bottom,omitempty"`
}

func BuildTrendCompressedJSON(symbol, interval string, candles []snapshot.Candle, opts TrendCompressOptions) (string, error) {
	payload, err := BuildTrendCompressedInput(symbol, interval, candles, opts)
	if err != nil {
		return "", err
	}
	if opts.Pretty {
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func BuildTrendCompressedInput(symbol, interval string, candles []snapshot.Candle, opts TrendCompressOptions) (TrendCompressedInput, error) {
	if len(candles) == 0 {
		return TrendCompressedInput{}, fmt.Errorf("no candles")
	}
	opts = normalizeTrendCompressOptions(opts)
	n := len(candles)

	closes := make([]float64, n)
	highs := make([]float64, n)
	lows := make([]float64, n)
	volumes := make([]float64, n)
	patternCandles := make([]cdl.Candle, n)
	geometryCandles := make([]geometry.Candle, n)
	for i, c := range candles {
		closes[i] = c.Close
		highs[i] = c.High
		lows[i] = c.Low
		volumes[i] = c.Volume
		patternCandles[i] = cdl.Candle{
			Open:  c.Open,
			High:  c.High,
			Low:   c.Low,
			Close: c.Close,
		}
		geometryCandles[i] = geometry.Candle{
			Open:  c.Open,
			High:  c.High,
			Low:   c.Low,
			Close: c.Close,
		}
	}

	var rsiSeries []float64
	var atrSeries []float64
	if opts.RSIPeriod > 0 && len(closes) >= config.RSIRequiredBars(opts.RSIPeriod) {
		rsiSeries = talib.Rsi(closes, opts.RSIPeriod)
	}
	if opts.ATRPeriod > 0 && len(closes) >= config.ATRRequiredBars(opts.ATRPeriod) {
		atrSeries = talib.Atr(highs, lows, closes, opts.ATRPeriod)
	}

	meta := TrendCompressedMeta{
		Symbol:    strings.ToUpper(strings.TrimSpace(symbol)),
		Interval:  strings.ToLower(strings.TrimSpace(interval)),
		Timestamp: candleTimestamp(candles[n-1]),
	}

	gc := TrendGlobalContext{
		TrendSlope: roundFloat(linRegSlope(closes), 4),
		VolRatio:   roundFloat(volumeRatio(volumes, opts.VolumeMAPeriod), 3),
		Window:     n,
	}
	gc.NormalizedSlope = roundFloat(normalizedSlope(closes), 4)
	gc.SlopeState = trendSlopeState(gc.NormalizedSlope)
	if opts.EMA20Period > 0 && len(closes) >= config.EMARequiredBars(opts.EMA20Period) {
		if v := lastNonZero(talib.Ema(closes, opts.EMA20Period)); v > 0 {
			val := roundFloat(v, 4)
			gc.EMA20 = &val
		}
	}
	if opts.EMA50Period > 0 && len(closes) >= config.EMARequiredBars(opts.EMA50Period) {
		if v := lastNonZero(talib.Ema(closes, opts.EMA50Period)); v > 0 {
			val := roundFloat(v, 4)
			gc.EMA50 = &val
		}
	}
	if opts.EMA200Period > 0 && len(closes) >= config.EMARequiredBars(opts.EMA200Period) {
		if v := lastNonZero(talib.Ema(closes, opts.EMA200Period)); v > 0 {
			val := roundFloat(v, 4)
			gc.EMA200 = &val
		}
	}

	var superTrend *TrendSuperTrendSnapshot
	if !opts.SkipSuperTrend && opts.SuperTrendPeriod > 0 {
		requiredBars := config.SuperTrendRequiredBars(opts.SuperTrendPeriod, opts.SuperTrendMultiplier)
		if n >= requiredBars {
			stResult := computeSuperTrendSeries(highs, lows, closes, opts.SuperTrendPeriod, opts.SuperTrendMultiplier)
			superTrend = buildSuperTrendSnapshot(interval, stResult, closes)
		}
	}

	structurePoints := selectStructurePoints(candles, highs, lows, rsiSeries, atrSeries, opts)
	candidates := buildStructureCandidates(candles, highs, lows, atrSeries, gc, structurePoints, opts)
	recentCandles := buildRecentCandles(candles, rsiSeries, opts)

	patternEvidence := evidence.Combine(
		geometry.Detect(geometryCandles, geometry.DefaultOptions()),
		cdl.Detect(patternCandles, cdl.DefaultOptions()),
		evidence.Options{
			MinScore:    opts.PatternMinScore,
			MaxDetected: opts.PatternMaxDetected,
		},
	)
	var patternRef *evidence.Result
	if len(patternEvidence.Detected) > 0 {
		patternRef = &patternEvidence
	}
	smc := buildTrendSMC(candles, closes)
	keyLevels := buildTrendKeyLevels(structurePoints)
	breakEvents, breakSummary := buildTrendBreakSummary(closes, keyLevels)

	return TrendCompressedInput{
		Meta:                meta,
		StructurePoints:     structurePoints,
		StructureCandidates: candidates,
		RecentCandles:       recentCandles,
		GlobalContext:       gc,
		SuperTrend:          superTrend,
		KeyLevels:           keyLevels,
		BreakEvents:         breakEvents,
		BreakSummary:        breakSummary,
		Pattern:             patternRef,
		SMC:                 smc,
	}, nil
}

func normalizeTrendCompressOptions(opts TrendCompressOptions) TrendCompressOptions {
	def := DefaultTrendCompressOptions()
	if opts.FractalSpan <= 0 {
		opts.FractalSpan = def.FractalSpan
	}
	if opts.MaxStructurePoints <= 0 {
		opts.MaxStructurePoints = def.MaxStructurePoints
	}
	if opts.DedupDistanceBars <= 0 {
		opts.DedupDistanceBars = def.DedupDistanceBars
	}
	if opts.DedupATRFactor <= 0 {
		opts.DedupATRFactor = def.DedupATRFactor
	}
	if opts.SuperTrendPeriod <= 0 {
		opts.SuperTrendPeriod = def.SuperTrendPeriod
	}
	if opts.SuperTrendMultiplier <= 0 {
		opts.SuperTrendMultiplier = def.SuperTrendMultiplier
	}
	if opts.RSIPeriod <= 0 {
		opts.RSIPeriod = def.RSIPeriod
	}
	if opts.ATRPeriod <= 0 {
		opts.ATRPeriod = def.ATRPeriod
	}
	if opts.RecentCandles <= 0 {
		opts.RecentCandles = def.RecentCandles
	}
	if opts.VolumeMAPeriod <= 0 {
		opts.VolumeMAPeriod = def.VolumeMAPeriod
	}
	if opts.EMA20Period <= 0 {
		opts.EMA20Period = def.EMA20Period
	}
	if opts.EMA50Period <= 0 {
		opts.EMA50Period = def.EMA50Period
	}
	if opts.EMA200Period <= 0 {
		opts.EMA200Period = def.EMA200Period
	}
	if opts.PatternMinScore <= 0 {
		opts.PatternMinScore = def.PatternMinScore
	}
	if opts.PatternMaxDetected <= 0 {
		opts.PatternMaxDetected = def.PatternMaxDetected
	}
	if !opts.IncludeCurrentRSI && !opts.IncludeStructureRSI {
		opts.IncludeCurrentRSI = def.IncludeCurrentRSI
		opts.IncludeStructureRSI = def.IncludeStructureRSI
	}
	return opts
}

func buildSuperTrendSnapshot(interval string, stSeries, closes []float64) *TrendSuperTrendSnapshot {
	limit := len(stSeries)
	if len(closes) < limit {
		limit = len(closes)
	}
	for i := limit - 1; i >= 0; i-- {
		level := stSeries[i]
		close := closes[i]
		if math.IsNaN(level) || math.IsInf(level, 0) || math.Abs(level) <= 1e-12 {
			continue
		}
		if math.IsNaN(close) || math.IsInf(close, 0) || math.Abs(close) <= 1e-12 {
			continue
		}
		state := superTrendStateDown
		if close >= level {
			state = superTrendStateUp
		}
		return &TrendSuperTrendSnapshot{
			Interval:    strings.ToLower(strings.TrimSpace(interval)),
			State:       state,
			Level:       roundFloat(level, 4),
			DistancePct: roundFloat(math.Abs(close-level)/close*100, 4),
		}
	}
	return nil
}

func buildTrendSMC(candles []snapshot.Candle, closes []float64) TrendSMC {
	bias := trendBiasBearish
	if len(closes) > 0 {
		last := closes[len(closes)-1]
		if last >= emaSpan(closes, 34) {
			bias = trendBiasBullish
		}
	}
	return TrendSMC{
		OrderBlock: detectOrderBlock(candles, bias),
		FVG:        detectFVG(candles),
	}
}

func buildTrendKeyLevels(points []TrendStructurePoint) *TrendKeyLevels {
	if len(points) == 0 {
		return nil
	}
	var lastHigh *TrendKeyLevel
	var lastLow *TrendKeyLevel
	for i := len(points) - 1; i >= 0; i-- {
		p := points[i]
		switch p.Type {
		case structurePointHigh:
			if lastHigh == nil {
				lastHigh = &TrendKeyLevel{Price: p.Price, Idx: p.Idx}
			}
		case structurePointLow:
			if lastLow == nil {
				lastLow = &TrendKeyLevel{Price: p.Price, Idx: p.Idx}
			}
		}
		if lastHigh != nil && lastLow != nil {
			break
		}
	}
	if lastHigh == nil && lastLow == nil {
		return nil
	}
	return &TrendKeyLevels{LastSwingHigh: lastHigh, LastSwingLow: lastLow}
}

func buildTrendBreakSummary(closes []float64, levels *TrendKeyLevels) ([]TrendBreakEvent, *TrendBreakSummary) {
	event, ok := detectLatestBreakEvent(closes, levels)
	if !ok {
		return nil, nil
	}
	age := event.BarAge
	barIdx := event.BarIdx
	levelPrice := event.LevelPrice
	levelIdx := event.LevelIdx
	summary := &TrendBreakSummary{
		LatestEventType:       event.Type,
		LatestEventAge:        &age,
		LatestEventBarIdx:     &barIdx,
		LatestEventLevelPrice: &levelPrice,
		LatestEventLevelIdx:   &levelIdx,
	}
	return []TrendBreakEvent{event}, summary
}

func detectLatestBreakEvent(closes []float64, levels *TrendKeyLevels) (TrendBreakEvent, bool) {
	if len(closes) < 2 || levels == nil {
		return TrendBreakEvent{}, false
	}
	latestIdx := len(closes) - 1
	for i := latestIdx; i >= 1; i-- {
		prev := closes[i-1]
		curr := closes[i]
		if levels.LastSwingHigh != nil {
			level := levels.LastSwingHigh
			if prev <= level.Price && curr > level.Price {
				return TrendBreakEvent{
					Type:       "break_up",
					LevelPrice: level.Price,
					LevelIdx:   level.Idx,
					BarIdx:     i,
					BarAge:     latestIdx - i,
					Confirm:    "close",
				}, true
			}
		}
		if levels.LastSwingLow != nil {
			level := levels.LastSwingLow
			if prev >= level.Price && curr < level.Price {
				return TrendBreakEvent{
					Type:       "break_down",
					LevelPrice: level.Price,
					LevelIdx:   level.Idx,
					BarIdx:     i,
					BarAge:     latestIdx - i,
					Confirm:    "close",
				}, true
			}
		}
	}
	return TrendBreakEvent{}, false
}

func emaSpan(series []float64, span int) float64 {
	if len(series) == 0 || span <= 0 {
		return 0
	}
	alpha := 2.0 / float64(span+1)
	ema := series[0]
	for i := 1; i < len(series); i++ {
		ema = alpha*series[i] + (1-alpha)*ema
	}
	return ema
}

func detectOrderBlock(candles []snapshot.Candle, bias string) TrendOrderBlock {
	block := TrendOrderBlock{Type: trendTypeNone}
	if len(candles) == 0 {
		return block
	}
	recent := candles
	if len(recent) > 8 {
		recent = candles[len(candles)-8:]
	}
	if bias == trendBiasBullish {
		for i := len(recent) - 1; i >= 0; i-- {
			c := recent[i]
			if c.Close < c.Open {
				upper := roundFloat(math.Max(c.Open, c.Close), 4)
				lower := roundFloat(math.Min(c.Low, c.Open), 4)
				block.Type = trendBiasBullish
				block.Upper = &upper
				block.Lower = &lower
				return block
			}
		}
		return block
	}
	for i := len(recent) - 1; i >= 0; i-- {
		c := recent[i]
		if c.Close > c.Open {
			upper := roundFloat(math.Max(c.Open, c.High), 4)
			lower := roundFloat(math.Min(c.Open, c.Close), 4)
			block.Type = trendBiasBearish
			block.Upper = &upper
			block.Lower = &lower
			return block
		}
	}
	return block
}

func detectFVG(candles []snapshot.Candle) TrendFVG {
	fvg := TrendFVG{Type: trendTypeNone}
	if len(candles) < 5 {
		return fvg
	}
	n := len(candles)
	for offset := 2; offset <= 5; offset++ {
		idx := n - offset
		if idx-2 < 0 {
			break
		}
		highPrev := candles[idx-2].High
		lowPrev := candles[idx-2].Low
		highCurr := candles[idx].High
		lowCurr := candles[idx].Low
		if candles[idx-1].Low > highPrev && lowCurr > highPrev {
			gapTop := roundFloat(lowCurr, 4)
			gapBottom := roundFloat(highPrev, 4)
			fvg.Type = trendBiasBullish
			fvg.GapTop = &gapTop
			fvg.GapBottom = &gapBottom
			return fvg
		}
		if candles[idx-1].High < lowPrev && highCurr < lowPrev {
			gapTop := roundFloat(lowPrev, 4)
			gapBottom := roundFloat(highCurr, 4)
			fvg.Type = trendBiasBearish
			fvg.GapTop = &gapTop
			fvg.GapBottom = &gapBottom
			return fvg
		}
	}
	return fvg
}

func buildRecentCandles(candles []snapshot.Candle, rsi []float64, opts TrendCompressOptions) []TrendRecentCandle {
	n := len(candles)
	keep := opts.RecentCandles
	if keep > n {
		keep = n
	}
	start := n - keep
	out := make([]TrendRecentCandle, 0, keep)
	for idx := start; idx < n; idx++ {
		c := candles[idx]
		rc := TrendRecentCandle{
			Idx: idx,
			O:   roundFloat(c.Open, 4),
			H:   roundFloat(c.High, 4),
			L:   roundFloat(c.Low, 4),
			C:   roundFloat(c.Close, 4),
			V:   roundFloat(c.Volume, 4),
		}
		if opts.IncludeCurrentRSI && idx < len(rsi) {
			v := rsi[idx]
			if !math.IsNaN(v) && !math.IsInf(v, 0) {
				r := roundFloat(v, 1)
				rc.RSI = &r
			}
		}
		out = append(out, rc)
	}
	return out
}
