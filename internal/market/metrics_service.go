package market

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"brale-core/internal/interval"
	"brale-core/internal/pkg/logging"

	"go.uber.org/zap"
)

type DerivativesData struct {
	Symbol      string
	OI          float64
	OIHistory   map[string]float64
	FundingRate float64
	LastUpdate  time.Time
	Error       string
}

type MetricsService struct {
	source  Source
	cache   map[string]DerivativesData
	mu      sync.RWMutex
	symbols []string

	pollInterval           time.Duration
	baseOIHistoryPeriod    string
	oiHistoryLimit         int
	targetOIHistTimeframes []string
}

type oiPeriodProvider interface {
	SupportedOIPeriods() []string
}

func (s *MetricsService) GetTargetTimeframes() []string {
	return s.targetOIHistTimeframes
}

func (s *MetricsService) BaseOIHistoryPeriod() string {
	return s.baseOIHistoryPeriod
}

func (s *MetricsService) OIHistoryLimit() int {
	return s.oiHistoryLimit
}

func NewMetricsService(source Source, symbols []string, timeframes []string) (*MetricsService, error) {
	validSymbols := make([]string, 0, len(symbols))
	for _, s := range symbols {
		s = strings.ToUpper(strings.TrimSpace(s))
		if s != "" {
			validSymbols = append(validSymbols, s)
		}
	}
	if len(validSymbols) == 0 {
		return nil, nil
	}

	allTimeframes := normalizeTimeframes(timeframes)
	if len(allTimeframes) == 0 {
		return nil, fmt.Errorf("metrics configuration missing timeframes")
	}

	targetTimeframes := allTimeframes
	if provider, ok := source.(oiPeriodProvider); ok {
		supported := normalizeTimeframes(provider.SupportedOIPeriods())
		if len(supported) > 0 {
			targetTimeframes = intersectTimeframes(targetTimeframes, supported)
			if len(targetTimeframes) == 0 {
				logging.FromContext(context.Background()).Named("market").Warn("metrics OI periods unsupported",
					zap.Strings("requested", allTimeframes),
					zap.Strings("supported", supported),
				)
			}
		}
	}

	basePeriod, historyLimit := "", 0
	if len(targetTimeframes) > 0 {
		basePeriod, historyLimit, targetTimeframes = calculateOIHistoryParams(targetTimeframes)
		if basePeriod == "" {
			return nil, fmt.Errorf("failed to calculate valid OI history params from timeframes: %v", allTimeframes)
		}
	}

	pollInterval := 5 * time.Minute
	if dur, err := parseTimeframe(basePeriod); err == nil && dur > 0 {
		pollInterval = dur
	}

	return &MetricsService{
		source:                 source,
		cache:                  make(map[string]DerivativesData),
		symbols:                validSymbols,
		pollInterval:           pollInterval,
		baseOIHistoryPeriod:    basePeriod,
		oiHistoryLimit:         historyLimit,
		targetOIHistTimeframes: targetTimeframes,
	}, nil
}

func (s *MetricsService) Get(symbol string) (DerivativesData, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, ok := s.cache[strings.ToUpper(strings.TrimSpace(symbol))]
	return data, ok
}

func (s *MetricsService) OI(ctx context.Context, symbol string) (float64, error) {
	data, ok := s.Get(symbol)
	if !ok {
		return 0, fmt.Errorf("MetricsService: %s OI not found or not updated", symbol)
	}
	if data.Error != "" {
		return 0, fmt.Errorf("MetricsService: %s OI error: %s", symbol, data.Error)
	}
	return data.OI, nil
}

func (s *MetricsService) Funding(ctx context.Context, symbol string) (float64, error) {
	data, ok := s.Get(symbol)
	if !ok {
		return 0, fmt.Errorf("MetricsService: %s funding not found or not updated", symbol)
	}
	if data.Error != "" {
		return 0, fmt.Errorf("MetricsService: %s funding error: %s", symbol, data.Error)
	}
	return data.FundingRate, nil
}

func (s *MetricsService) RefreshSymbol(ctx context.Context, symbol string) {
	if s == nil {
		return
	}
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" || s.source == nil {
		return
	}
	s.updateSymbol(ctx, symbol)
}

func (s *MetricsService) Start(ctx context.Context) {
	logger := logging.FromContext(ctx).Named("market")
	count := len(s.symbols)
	if count == 0 {
		logger.Warn("metrics service not started", zap.String("reason", "no symbols configured"))
		return
	}

	effectiveInterval := s.pollInterval
	if s.pollInterval > 10*time.Second {
		effectiveInterval = s.pollInterval - (s.pollInterval / 10)
		if effectiveInterval < 10*time.Second {
			effectiveInterval = 10 * time.Second
		}
	} else if s.pollInterval == 0 {
		effectiveInterval = 60 * time.Second
	}

	step := effectiveInterval / time.Duration(count)
	minStep := 1 * time.Second
	if step < minStep {
		step = minStep
		logger.Warn("metrics step too small", zap.Duration("step", step))
	}

	logger.Info("metrics service start",
		zap.Int("symbols", count),
		zap.Duration("interval", effectiveInterval),
		zap.Duration("step", step),
	)

	ticker := time.NewTicker(step)
	defer ticker.Stop()

	cursor := 0
	for {
		select {
		case <-ctx.Done():
			logger.Info("metrics service stopped")
			return
		case <-ticker.C:
			symbolsCopy := s.symbols
			if len(symbolsCopy) == 0 {
				continue
			}
			sym := symbolsCopy[cursor]
			cursor = (cursor + 1) % len(symbolsCopy)
			updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			go func(sCtx context.Context, symbol string) {
				defer cancel()
				s.updateSymbol(sCtx, symbol)
			}(updateCtx, sym)
		}
	}
}

func (s *MetricsService) updateSymbol(ctx context.Context, symbol string) {
	logger := logging.FromContext(ctx).Named("market").With(zap.String("symbol", symbol))
	type oiResult struct {
		hist []OpenInterestPoint
		err  error
	}
	type fundingResult struct {
		rate float64
		err  error
	}

	var oiCh chan oiResult
	if s.baseOIHistoryPeriod != "" && s.oiHistoryLimit > 0 {
		oiCh = make(chan oiResult, 1)
		go func() {
			hist, err := s.source.GetOpenInterestHistory(ctx, symbol, s.baseOIHistoryPeriod, s.oiHistoryLimit)
			if err != nil {
				logger.Warn("metrics OI history failed", zap.Error(err))
			}
			oiCh <- oiResult{hist: hist, err: err}
		}()
	}

	fundCh := make(chan fundingResult, 1)
	go func() {
		rate, err := s.source.GetFundingRate(ctx, symbol)
		if err != nil {
			logger.Warn("metrics funding failed", zap.Error(err))
		}
		fundCh <- fundingResult{rate: rate, err: err}
	}()

	var oiHist []OpenInterestPoint
	var errOI error
	if oiCh != nil {
		res := <-oiCh
		oiHist = res.hist
		errOI = res.err
	}

	fundRes := <-fundCh
	funding := fundRes.rate
	errFund := fundRes.err

	newData := DerivativesData{
		Symbol:     symbol,
		LastUpdate: time.Now(),
		OIHistory:  make(map[string]float64),
	}

	var allErrors strings.Builder

	if s.baseOIHistoryPeriod != "" && s.oiHistoryLimit > 0 {
		if errOI != nil {
			_, _ = fmt.Fprintf(&allErrors, "oi history error: %v; ", errOI)
		} else if len(oiHist) == 0 {
			allErrors.WriteString("oi history empty; ")
		} else {
			if len(oiHist) > 0 {
				newData.OI = oiHist[len(oiHist)-1].SumOpenInterest
			} else {
				allErrors.WriteString("oi history empty; ")
			}

			for _, tf := range s.targetOIHistTimeframes {
				duration, err := parseTimeframe(tf)
				if err != nil {
					logger.Warn("metrics invalid timeframe", zap.String("timeframe", tf), zap.Error(err))
					continue
				}

				targetTime := time.Now().Add(-duration)

				var oiAtPastTime float64
				found := false
				for i := len(oiHist) - 1; i >= 0; i-- {
					point := oiHist[i]
					pointTime := time.UnixMilli(point.Timestamp)
					if pointTime.Before(targetTime) || pointTime.Equal(targetTime) {
						oiAtPastTime = point.SumOpenInterest
						found = true
						break
					}
				}
				if found {
					newData.OIHistory[tf] = oiAtPastTime
				} else {
					newData.OIHistory[tf] = 0
					logger.Warn("metrics OI missing for timeframe", zap.String("timeframe", tf))
				}
			}
		}
	}

	if errFund != nil {
		_, _ = fmt.Fprintf(&allErrors, "funding error: %v", errFund)
	} else {
		newData.FundingRate = funding
	}

	newData.Error = allErrors.String()

	s.mu.Lock()
	s.cache[symbol] = newData
	s.mu.Unlock()
}

func normalizeTimeframes(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(items))
	for _, iv := range items {
		norm := strings.ToLower(strings.TrimSpace(iv))
		if norm == "" {
			continue
		}
		set[norm] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for tf := range set {
		out = append(out, tf)
	}
	return out
}

func intersectTimeframes(targets, supported []string) []string {
	if len(targets) == 0 || len(supported) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(supported))
	for _, tf := range supported {
		set[strings.ToLower(strings.TrimSpace(tf))] = struct{}{}
	}
	var out []string
	for _, tf := range targets {
		if _, ok := set[tf]; ok {
			out = append(out, tf)
		}
	}
	return normalizeTimeframes(out)
}

func calculateOIHistoryParams(horizonTimeframes []string) (basePeriod string, limit int, targetTimeframes []string) {
	if len(horizonTimeframes) == 0 {
		return "", 0, nil
	}

	minDuration := time.Hour * 24 * 365
	maxDuration := time.Duration(0)

	parsedDurations := make(map[string]time.Duration)
	var allIntervals []string

	for _, tf := range horizonTimeframes {
		d, err := parseTimeframe(tf)
		if err != nil {
			logging.FromContext(context.Background()).Named("market").Warn("metrics invalid timeframe", zap.String("timeframe", tf), zap.Error(err))
			continue
		}
		parsedDurations[tf] = d
		allIntervals = append(allIntervals, tf)

		if d < minDuration {
			minDuration = d
			basePeriod = tf
		}
		if d > maxDuration {
			maxDuration = d
		}
	}

	if basePeriod == "" || minDuration == 0 {
		return "", 0, nil
	}

	binanceOIPeriods := map[string]time.Duration{
		"5m":  5 * time.Minute,
		"15m": 15 * time.Minute,
		"30m": 30 * time.Minute,
		"1h":  1 * time.Hour,
		"2h":  2 * time.Hour,
		"4h":  4 * time.Hour,
		"6h":  6 * time.Hour,
		"12h": 12 * time.Hour,
		"1d":  24 * time.Hour,
	}

	actualBasePeriod := "30m"
	actualBaseDuration := 30 * time.Minute
	for p, d := range binanceOIPeriods {
		if d <= minDuration && d > actualBaseDuration {
			actualBaseDuration = d
			actualBasePeriod = p
		}
	}
	basePeriod = actualBasePeriod

	limit = int(maxDuration/actualBaseDuration) + 5
	if limit < 10 {
		limit = 10
	}
	if limit > 500 {
		limit = 500
	}

	targetTimeframes = allIntervals
	return basePeriod, limit, targetTimeframes
}

func parseTimeframe(tf string) (time.Duration, error) {
	if strings.TrimSpace(tf) != tf {
		return 0, fmt.Errorf("invalid timeframe: %s", tf)
	}
	if tf != strings.ToLower(tf) {
		return 0, fmt.Errorf("invalid timeframe: %s", tf)
	}
	if strings.HasSuffix(tf, "s") {
		return 0, fmt.Errorf("invalid timeframe: %s", tf)
	}
	value, err := interval.ParseInterval(tf)
	if err != nil {
		return 0, fmt.Errorf("invalid timeframe: %s", tf)
	}
	return value, nil
}
