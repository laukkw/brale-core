package features

import (
	"fmt"
	"math"
	"sort"

	"brale-core/internal/config"
	"brale-core/internal/snapshot"
)

type EngineDiffRequest struct {
	Symbol            string
	BaselineName      string
	Baseline          IndicatorComputer
	CandidateName     string
	Candidate         IndicatorComputer
	IndicatorOptions  IndicatorCompressOptions
	TrendOptionsByInt map[string]TrendCompressOptions
	DefaultTrendOpts  TrendCompressOptions
	CandlesByInterval map[string][]snapshot.Candle
}

type EngineDiffReport struct {
	Symbol    string               `json:"symbol"`
	Baseline  string               `json:"baseline"`
	Candidate string               `json:"candidate"`
	Intervals []EngineIntervalDiff `json:"intervals"`
}

type EngineIntervalDiff struct {
	Interval  string         `json:"interval"`
	Numeric   []SeriesDiff   `json:"numeric"`
	Semantics []SemanticDiff `json:"semantics"`
}

type SeriesDiff struct {
	Name             string  `json:"name"`
	ComparablePoints int     `json:"comparable_points"`
	MaxDiff          float64 `json:"max_diff"`
	AvgDiff          float64 `json:"avg_diff"`
	LatestDiff       float64 `json:"latest_diff"`
	LatestComparable bool    `json:"latest_comparable"`
	BaselineWarmup   int     `json:"baseline_warmup"`
	CandidateWarmup  int     `json:"candidate_warmup"`
	WarmupDelta      int     `json:"warmup_delta"`
}

type SemanticDiff struct {
	Name      string `json:"name"`
	Baseline  string `json:"baseline"`
	Candidate string `json:"candidate"`
	Match     bool   `json:"match"`
}

type seriesDiffInput struct {
	values     []float64
	warmupHint int
}

func RunEngineDiff(req EngineDiffRequest) (EngineDiffReport, error) {
	if req.Baseline == nil {
		return EngineDiffReport{}, fmt.Errorf("baseline computer is required")
	}
	if req.Candidate == nil {
		return EngineDiffReport{}, fmt.Errorf("candidate computer is required")
	}
	if len(req.CandlesByInterval) == 0 {
		return EngineDiffReport{}, fmt.Errorf("candles by interval is required")
	}
	defaultTrend := req.DefaultTrendOpts
	if defaultTrend == (TrendCompressOptions{}) {
		defaultTrend = DefaultTrendCompressOptions()
		defaultTrend.SkipSuperTrend = true
	}
	report := EngineDiffReport{
		Symbol:    req.Symbol,
		Baseline:  req.BaselineName,
		Candidate: req.CandidateName,
	}
	intervals := make([]string, 0, len(req.CandlesByInterval))
	for interval := range req.CandlesByInterval {
		intervals = append(intervals, interval)
	}
	sort.Strings(intervals)
	for _, interval := range intervals {
		candles := req.CandlesByInterval[interval]
		if len(candles) == 0 {
			return EngineDiffReport{}, fmt.Errorf("candles for %s are empty", interval)
		}
		trendOpts := defaultTrend
		if selected, ok := req.TrendOptionsByInt[interval]; ok {
			trendOpts = selected
		}
		numeric, err := buildEngineNumericDiffs(candles, req.IndicatorOptions, trendOpts, req.Baseline, req.Candidate)
		if err != nil {
			return EngineDiffReport{}, err
		}
		semantics, err := buildEngineSemanticDiffs(req.Symbol, interval, candles, req.IndicatorOptions, trendOpts, req.Baseline, req.Candidate)
		if err != nil {
			return EngineDiffReport{}, err
		}
		report.Intervals = append(report.Intervals, EngineIntervalDiff{
			Interval:  interval,
			Numeric:   numeric,
			Semantics: semantics,
		})
	}
	return report, nil
}

func buildEngineNumericDiffs(candles []snapshot.Candle, indicatorOpts IndicatorCompressOptions, trendOpts TrendCompressOptions, baseline, candidate IndicatorComputer) ([]SeriesDiff, error) {
	baselineSeries, err := buildEngineSeriesSet(candles, indicatorOpts, trendOpts, baseline)
	if err != nil {
		return nil, err
	}
	candidateSeries, err := buildEngineSeriesSet(candles, indicatorOpts, trendOpts, candidate)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(baselineSeries))
	for name := range baselineSeries {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]SeriesDiff, 0, len(names))
	for _, name := range names {
		out = append(out, compareSeriesDiff(name, baselineSeries[name], candidateSeries[name]))
	}
	return out, nil
}

func buildEngineSeriesSet(candles []snapshot.Candle, indicatorOpts IndicatorCompressOptions, trendOpts TrendCompressOptions, computer IndicatorComputer) (map[string]seriesDiffInput, error) {
	_, closes, highs, lows, volumes := buildIndicatorSeries(candles)
	out := make(map[string]seriesDiffInput)

	if !indicatorOpts.SkipEMA {
		if indicatorOpts.EMAFast > 0 && len(closes) >= config.EMARequiredBars(indicatorOpts.EMAFast) {
			series, err := computer.ComputeEMA(closes, indicatorOpts.EMAFast)
			if err != nil {
				return nil, err
			}
			out["indicator.ema_fast"] = newSeriesDiffInput(series, warmupFromRequired(config.EMARequiredBars(indicatorOpts.EMAFast)))
		}
		if indicatorOpts.EMAMid > 0 && len(closes) >= config.EMARequiredBars(indicatorOpts.EMAMid) {
			series, err := computer.ComputeEMA(closes, indicatorOpts.EMAMid)
			if err != nil {
				return nil, err
			}
			out["indicator.ema_mid"] = newSeriesDiffInput(series, warmupFromRequired(config.EMARequiredBars(indicatorOpts.EMAMid)))
		}
		if indicatorOpts.EMASlow > 0 && len(closes) >= config.EMARequiredBars(indicatorOpts.EMASlow) {
			series, err := computer.ComputeEMA(closes, indicatorOpts.EMASlow)
			if err != nil {
				return nil, err
			}
			out["indicator.ema_slow"] = newSeriesDiffInput(series, warmupFromRequired(config.EMARequiredBars(indicatorOpts.EMASlow)))
		}
	}

	if !indicatorOpts.SkipRSI && indicatorOpts.RSIPeriod > 0 && len(closes) >= config.RSIRequiredBars(indicatorOpts.RSIPeriod) {
		rsiSeries, err := computer.ComputeRSI(closes, indicatorOpts.RSIPeriod)
		if err != nil {
			return nil, err
		}
		out["indicator.rsi"] = newSeriesDiffInput(rsiSeries, warmupFromRequired(config.RSIRequiredBars(indicatorOpts.RSIPeriod)))
		if indicatorOpts.StochRSIPeriod > 0 && len(rsiSeries) >= indicatorOpts.StochRSIPeriod {
			stoch, err := computer.ComputeStochRSI(rsiSeries, indicatorOpts.StochRSIPeriod)
			if err != nil {
				return nil, err
			}
			out["indicator.stoch_rsi"] = newSeriesDiffInput(stoch, warmupFromRequired(config.StochRSIRequiredBars(indicatorOpts.RSIPeriod, indicatorOpts.StochRSIPeriod)))
		}
	}

	if indicatorOpts.ATRPeriod > 0 && len(closes) >= config.ATRRequiredBars(indicatorOpts.ATRPeriod) {
		atrSeries, err := computer.ComputeATR(highs, lows, closes, indicatorOpts.ATRPeriod)
		if err != nil {
			return nil, err
		}
		out["indicator.atr"] = newSeriesDiffInput(atrSeries, warmupFromRequired(config.ATRRequiredBars(indicatorOpts.ATRPeriod)))
	}

	obvSeries, err := computer.ComputeOBV(closes, volumes)
	if err != nil {
		return nil, err
	}
	out["indicator.obv"] = newSeriesDiffInput(obvSeries, 0)

	if !indicatorOpts.SkipSTC {
		required := config.STCRequiredBars(indicatorOpts.STCFast, indicatorOpts.STCSlow)
		if len(closes) >= required {
			stcSeries, err := computer.ComputeSTC(closes, indicatorOpts.STCFast, indicatorOpts.STCSlow, config.DefaultSTCKPeriod, config.DefaultSTCDPeriod)
			if err != nil {
				return nil, err
			}
			out["indicator.stc"] = newSeriesDiffInput(stcSeries, warmupFromRequired(required))
		}
	}

	if indicatorOpts.BBPeriod > 0 && len(closes) >= config.BBRequiredBars(indicatorOpts.BBPeriod) {
		upper, middle, lower, err := computer.ComputeBB(closes, indicatorOpts.BBPeriod, indicatorOpts.BBMultiplier, indicatorOpts.BBMultiplier)
		if err != nil {
			return nil, err
		}
		hint := warmupFromRequired(config.BBRequiredBars(indicatorOpts.BBPeriod))
		out["indicator.bb_upper"] = newSeriesDiffInput(upper, hint)
		out["indicator.bb_middle"] = newSeriesDiffInput(middle, hint)
		out["indicator.bb_lower"] = newSeriesDiffInput(lower, hint)
	}

	if indicatorOpts.CHOPPeriod > 1 && len(closes) >= config.CHOPRequiredBars(indicatorOpts.CHOPPeriod) {
		chopSeries, err := computer.ComputeCHOP(highs, lows, closes, indicatorOpts.CHOPPeriod)
		if err != nil {
			return nil, err
		}
		out["indicator.chop"] = newSeriesDiffInput(chopSeries, warmupFromRequired(config.CHOPRequiredBars(indicatorOpts.CHOPPeriod)))
	}

	if indicatorOpts.AroonPeriod > 0 && len(closes) >= config.AroonRequiredBars(indicatorOpts.AroonPeriod) {
		up, down, err := computer.ComputeAroon(highs, lows, indicatorOpts.AroonPeriod)
		if err != nil {
			return nil, err
		}
		hint := warmupFromRequired(config.AroonRequiredBars(indicatorOpts.AroonPeriod))
		out["indicator.aroon_up"] = newSeriesDiffInput(up, hint)
		out["indicator.aroon_down"] = newSeriesDiffInput(down, hint)
	}

	if trendOpts.RSIPeriod > 0 && len(closes) >= config.RSIRequiredBars(trendOpts.RSIPeriod) {
		rsiSeries, err := computer.ComputeRSI(closes, trendOpts.RSIPeriod)
		if err != nil {
			return nil, err
		}
		out["trend.rsi"] = newSeriesDiffInput(rsiSeries, warmupFromRequired(config.RSIRequiredBars(trendOpts.RSIPeriod)))
	}
	if trendOpts.ATRPeriod > 0 && len(closes) >= config.ATRRequiredBars(trendOpts.ATRPeriod) {
		atrSeries, err := computer.ComputeATR(highs, lows, closes, trendOpts.ATRPeriod)
		if err != nil {
			return nil, err
		}
		out["trend.atr"] = newSeriesDiffInput(atrSeries, warmupFromRequired(config.ATRRequiredBars(trendOpts.ATRPeriod)))
	}
	if trendOpts.EMA20Period > 0 && len(closes) >= config.EMARequiredBars(trendOpts.EMA20Period) {
		series, err := computer.ComputeEMA(closes, trendOpts.EMA20Period)
		if err != nil {
			return nil, err
		}
		out["trend.ema20"] = newSeriesDiffInput(series, warmupFromRequired(config.EMARequiredBars(trendOpts.EMA20Period)))
	}
	if trendOpts.EMA50Period > 0 && len(closes) >= config.EMARequiredBars(trendOpts.EMA50Period) {
		series, err := computer.ComputeEMA(closes, trendOpts.EMA50Period)
		if err != nil {
			return nil, err
		}
		out["trend.ema50"] = newSeriesDiffInput(series, warmupFromRequired(config.EMARequiredBars(trendOpts.EMA50Period)))
	}
	if trendOpts.EMA200Period > 0 && len(closes) >= config.EMARequiredBars(trendOpts.EMA200Period) {
		series, err := computer.ComputeEMA(closes, trendOpts.EMA200Period)
		if err != nil {
			return nil, err
		}
		out["trend.ema200"] = newSeriesDiffInput(series, warmupFromRequired(config.EMARequiredBars(trendOpts.EMA200Period)))
	}
	if trendOpts.VolumeMAPeriod > 0 && len(closes) >= trendOpts.VolumeMAPeriod {
		upper, _, lower, err := computer.ComputeBB(closes, trendOpts.VolumeMAPeriod, 2, 2)
		if err != nil {
			return nil, err
		}
		hint := warmupFromRequired(config.BBRequiredBars(trendOpts.VolumeMAPeriod))
		out["trend.bb_upper"] = newSeriesDiffInput(upper, hint)
		out["trend.bb_lower"] = newSeriesDiffInput(lower, hint)
	}

	return out, nil
}

func buildEngineSemanticDiffs(symbol, interval string, candles []snapshot.Candle, indicatorOpts IndicatorCompressOptions, trendOpts TrendCompressOptions, baseline, candidate IndicatorComputer) ([]SemanticDiff, error) {
	baselineIndicator, err := BuildIndicatorCompressedInputWithComputer(symbol, interval, candles, indicatorOpts, baseline)
	if err != nil {
		return nil, err
	}
	candidateIndicator, err := BuildIndicatorCompressedInputWithComputer(symbol, interval, candles, indicatorOpts, candidate)
	if err != nil {
		return nil, err
	}
	baselineTrend, err := BuildTrendCompressedInputWithComputer(symbol, interval, candles, trendOpts, baseline)
	if err != nil {
		return nil, err
	}
	candidateTrend, err := BuildTrendCompressedInputWithComputer(symbol, interval, candles, trendOpts, candidate)
	if err != nil {
		return nil, err
	}
	diffs := []SemanticDiff{
		newSemanticDiff("indicator.rsi_zone", classifyRSIState(baselineIndicator.Data.RSI), classifyRSIState(candidateIndicator.Data.RSI)),
		newSemanticDiff("indicator.bb_zone", classifyBBState(baselineIndicator.Market.CurrentPrice, baselineIndicator.Data.BB), classifyBBState(candidateIndicator.Market.CurrentPrice, candidateIndicator.Data.BB)),
		newSemanticDiff("trend.ema_stack", classifyTrendEMAStack(baselineTrend.GlobalContext), classifyTrendEMAStack(candidateTrend.GlobalContext)),
		newSemanticDiff("trend.rsi_zone", classifyRecentRSIState(baselineTrend.RecentCandles), classifyRecentRSIState(candidateTrend.RecentCandles)),
	}
	return diffs, nil
}

func compareSeriesDiff(name string, baseline, candidate seriesDiffInput) SeriesDiff {
	diff := SeriesDiff{
		Name:            name,
		BaselineWarmup:  firstValidIndex(baseline.values, baseline.warmupHint),
		CandidateWarmup: firstValidIndex(candidate.values, candidate.warmupHint),
	}
	diff.WarmupDelta = diff.CandidateWarmup - diff.BaselineWarmup
	comparable := 0
	sum := 0.0
	limit := len(baseline.values)
	if len(candidate.values) < limit {
		limit = len(candidate.values)
	}
	start := diff.BaselineWarmup
	if diff.CandidateWarmup > start {
		start = diff.CandidateWarmup
	}
	if start < 0 {
		start = 0
	}
	for i := start; i < limit; i++ {
		if invalidDiffValue(baseline.values[i]) || invalidDiffValue(candidate.values[i]) {
			continue
		}
		value := math.Abs(baseline.values[i] - candidate.values[i])
		if value > diff.MaxDiff {
			diff.MaxDiff = value
		}
		sum += value
		comparable++
	}
	diff.ComparablePoints = comparable
	if comparable > 0 {
		diff.AvgDiff = roundFloat(sum/float64(comparable), 6)
		diff.MaxDiff = roundFloat(diff.MaxDiff, 6)
	}
	if limit > 0 && start <= limit-1 && !invalidDiffValue(baseline.values[limit-1]) && !invalidDiffValue(candidate.values[limit-1]) {
		diff.LatestComparable = true
		diff.LatestDiff = roundFloat(math.Abs(baseline.values[limit-1]-candidate.values[limit-1]), 6)
	}
	return diff
}

func firstValidIndex(series []float64, start int) int {
	if start < 0 {
		start = 0
	}
	for i := start; i < len(series); i++ {
		value := series[i]
		if invalidDiffValue(value) {
			continue
		}
		return i
	}
	return -1
}

func invalidDiffValue(value float64) bool {
	return math.IsNaN(value) || math.IsInf(value, 0)
}

func newSeriesDiffInput(values []float64, warmupHint int) seriesDiffInput {
	return seriesDiffInput{values: values, warmupHint: warmupHint}
}

func warmupFromRequired(required int) int {
	if required <= 0 {
		return 0
	}
	return required - 1
}

func newSemanticDiff(name, baseline, candidate string) SemanticDiff {
	return SemanticDiff{
		Name:      name,
		Baseline:  baseline,
		Candidate: candidate,
		Match:     baseline == candidate,
	}
}

func classifyRSIState(snap *rsiSnapshot) string {
	if snap == nil {
		return "missing"
	}
	switch {
	case snap.Current <= 30:
		return "oversold"
	case snap.Current >= 70:
		return "overbought"
	default:
		return "neutral"
	}
}

func classifyBBState(price float64, snap *bbSnapshot) string {
	if snap == nil {
		return "missing"
	}
	switch {
	case price > snap.Upper:
		return "above_upper"
	case price < snap.Lower:
		return "below_lower"
	case price >= snap.Middle:
		return "inside_upper_half"
	default:
		return "inside_lower_half"
	}
}

func classifyTrendEMAStack(gc TrendGlobalContext) string {
	if gc.EMA20 == nil || gc.EMA50 == nil || gc.EMA200 == nil {
		return "missing"
	}
	switch {
	case *gc.EMA20 > *gc.EMA50 && *gc.EMA50 > *gc.EMA200:
		return "bullish"
	case *gc.EMA20 < *gc.EMA50 && *gc.EMA50 < *gc.EMA200:
		return "bearish"
	default:
		return "mixed"
	}
}

func classifyRecentRSIState(candles []TrendRecentCandle) string {
	if len(candles) == 0 || candles[len(candles)-1].RSI == nil {
		return "missing"
	}
	value := *candles[len(candles)-1].RSI
	switch {
	case value <= 30:
		return "oversold"
	case value >= 70:
		return "overbought"
	default:
		return "neutral"
	}
}
