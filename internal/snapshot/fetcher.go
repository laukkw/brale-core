package snapshot

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"brale-core/internal/interval"
	"brale-core/internal/pkg/longshort"
)

type Fetcher struct {
	Klines               KlineProvider
	OI                   OIProvider
	Funding              FundingProvider
	LongShort            LongShortProvider
	FearGreed            FearGreedProvider
	Liquidations         LiquidationProvider
	LiquidationsByWindow LiquidationWindowProvider
	LiquidationSource    LiquidationSourceProvider

	RequireOI           bool
	RequireFunding      bool
	RequireLongShort    bool
	RequireFearGreed    bool
	RequireLiquidations bool

	MinKlineBars int

	Now func() time.Time

	liqHistory *liqMetricHistory
	liqMu      sync.Mutex
}

func (f *Fetcher) Fetch(ctx context.Context, symbols, intervals []string, limit int) (MarketSnapshot, error) {
	if f == nil || f.Klines == nil {
		return MarketSnapshot{}, fmt.Errorf("klines provider is required")
	}
	if err := f.checkRequirements(); err != nil {
		return MarketSnapshot{}, err
	}
	ts := f.now()
	out := MarketSnapshot{
		Timestamp:            ts,
		DataAgeSec:           map[string]int64{},
		Klines:               map[string]map[string][]Candle{},
		OI:                   map[string]OIBlock{},
		Funding:              map[string]FundingBlock{},
		LongShort:            map[string]map[string]LSRBlock{},
		Liquidations:         map[string]LiqBlock{},
		LiquidationsByWindow: map[string]map[string]LiqWindow{},
		LiquidationSource:    map[string]LiqSource{},
	}
	if err := f.loadKlines(ctx, &out, symbols, intervals, limit); err != nil {
		return MarketSnapshot{}, err
	}
	if err := f.loadDerivatives(ctx, &out, symbols, intervals); err != nil {
		return MarketSnapshot{}, err
	}
	return out, nil
}

func (f *Fetcher) now() time.Time {
	if f != nil && f.Now != nil {
		return f.Now()
	}
	return time.Now().UTC()
}

func (f *Fetcher) checkRequirements() error {
	if f.RequireOI && f.OI == nil {
		return fmt.Errorf("oi provider is required")
	}
	if f.RequireFunding && f.Funding == nil {
		return fmt.Errorf("funding provider is required")
	}
	if f.RequireLongShort && f.LongShort == nil {
		return fmt.Errorf("long_short provider is required")
	}
	if f.RequireFearGreed && f.FearGreed == nil {
		return fmt.Errorf("fear_greed provider is required")
	}
	if f.RequireLiquidations {
		if f.Liquidations == nil {
			if f.resolveLiquidationWindowProvider() == nil {
				return fmt.Errorf("liquidations provider is required")
			}
			return nil
		}
		if f.resolveLiquidationWindowProvider() != nil {
			return nil
		}
	}
	return nil
}

func (f *Fetcher) loadKlines(ctx context.Context, out *MarketSnapshot, symbols, intervals []string, limit int) error {
	if len(symbols) == 0 || len(intervals) == 0 {
		return fmt.Errorf("symbols/intervals is required")
	}
	for _, sym := range symbols {
		if out.Klines[sym] == nil {
			out.Klines[sym] = map[string][]Candle{}
		}
		for _, iv := range intervals {
			fetchLimit := klineFetchLimit(limit, f.MinKlineBars)
			candles, err := f.Klines.Klines(ctx, sym, iv, fetchLimit)
			if err != nil {
				return fmt.Errorf("klines %s %s: %w", sym, iv, err)
			}
			candles, err = dropUnclosed(candles, iv, f.now())
			if err != nil {
				return fmt.Errorf("klines %s %s: %w", sym, iv, err)
			}
			candles = trimKlinesToLimit(candles, limit)
			if len(candles) == 0 {
				return fmt.Errorf("klines %s %s is empty", sym, iv)
			}
			if f.MinKlineBars > 0 && len(candles) < f.MinKlineBars {
				return fmt.Errorf("klines %s %s has %d closed candles, need at least %d", sym, iv, len(candles), f.MinKlineBars)
			}
			out.Klines[sym][iv] = candles
			setDataAge(out, "kline", sym, iv, candles[len(candles)-1].OpenTime)
		}
	}
	return nil
}

func klineFetchLimit(limit int, minBars int) int {
	if limit <= 0 || minBars <= 0 {
		return limit
	}
	return limit + 1
}

func trimKlinesToLimit(candles []Candle, limit int) []Candle {
	if limit <= 0 || len(candles) <= limit {
		return candles
	}
	return candles[len(candles)-limit:]
}

func (f *Fetcher) loadDerivatives(ctx context.Context, out *MarketSnapshot, symbols []string, intervals []string) error {
	if err := f.loadFearGreed(ctx, out); err != nil {
		return err
	}
	for _, sym := range symbols {
		if err := f.loadSymbolDerivatives(ctx, out, sym, intervals); err != nil {
			return err
		}
	}
	return nil
}

func (f *Fetcher) loadFearGreed(ctx context.Context, out *MarketSnapshot) error {
	var fg FearGreedPoint
	loaded, err := runOptionalLoad(
		f.FearGreed != nil,
		f.RequireFearGreed,
		"FearGreed provider is required",
		"FearGreed fetch failed",
		func() error {
			var fetchErr error
			fg, fetchErr = f.FearGreed.FearGreed(ctx)
			return fetchErr
		},
	)
	if err != nil {
		return err
	}
	if !loaded {
		return nil
	}
	out.FearGreed = &fg
	setDataAge(out, "fear_greed", "", "", fg.Timestamp)
	return nil
}

func runOptionalLoad(hasProvider bool, required bool, missingMsg string, fetchMsg string, load func() error) (bool, error) {
	if !hasProvider {
		if required {
			return false, fmt.Errorf("%s", missingMsg)
		}
		return false, nil
	}
	if err := load(); err != nil {
		if required {
			return false, fmt.Errorf("%s: %w", fetchMsg, err)
		}
		return false, nil
	}
	return true, nil
}

func setDataAge(out *MarketSnapshot, kind, sym, iv string, ts int64) {
	out.DataAgeSec[keyForAge(kind, sym, iv)] = ageSec(out.Timestamp, ts)
}

func (f *Fetcher) loadSymbolDerivatives(ctx context.Context, out *MarketSnapshot, sym string, intervals []string) error {
	if err := f.loadOI(ctx, out, sym); err != nil {
		return err
	}
	if err := f.loadFunding(ctx, out, sym); err != nil {
		return err
	}
	if err := f.loadLongShort(ctx, out, sym, intervals); err != nil {
		return err
	}
	if err := f.loadLiquidations(ctx, out, sym); err != nil {
		return err
	}
	if err := f.loadLiquidationsByWindow(ctx, out, sym); err != nil {
		return err
	}
	return nil
}

func (f *Fetcher) loadOI(ctx context.Context, out *MarketSnapshot, sym string) error {
	var oi OIBlock
	loaded, err := runOptionalLoad(
		f.OI != nil,
		f.RequireOI,
		"OI provider is required",
		fmt.Sprintf("oi fetch failed %s", sym),
		func() error {
			var fetchErr error
			oi, fetchErr = f.OI.OpenInterest(ctx, sym)
			return fetchErr
		},
	)
	if err != nil {
		return err
	}
	if !loaded {
		return nil
	}
	out.OI[sym] = oi
	setDataAge(out, "oi", sym, "", oi.Timestamp)
	return nil
}

func (f *Fetcher) loadFunding(ctx context.Context, out *MarketSnapshot, sym string) error {
	var funding FundingBlock
	loaded, err := runOptionalLoad(
		f.Funding != nil,
		f.RequireFunding,
		"funding provider is required",
		fmt.Sprintf("funding fetch failed %s", sym),
		func() error {
			var fetchErr error
			funding, fetchErr = f.Funding.Funding(ctx, sym)
			return fetchErr
		},
	)
	if err != nil {
		return err
	}
	if !loaded {
		return nil
	}
	out.Funding[sym] = funding
	setDataAge(out, "funding", sym, "", funding.Timestamp)
	return nil
}

func (f *Fetcher) loadLongShort(ctx context.Context, out *MarketSnapshot, sym string, intervals []string) error {
	if f.LongShort == nil {
		if f.RequireLongShort {
			return fmt.Errorf("LongShort provider is required")
		}
		return nil
	}
	supported := longshort.FilterSupported(intervals)
	if len(supported) == 0 {
		if f.RequireLongShort {
			return fmt.Errorf("LongShort intervals unsupported for %s", sym)
		}
		return nil
	}
	if out.LongShort[sym] == nil {
		out.LongShort[sym] = make(map[string]LSRBlock)
	}
	for _, iv := range supported {
		lsr, err := f.LongShort.LongShortRatio(ctx, sym, iv)
		if err != nil {
			if f.RequireLongShort {
				return fmt.Errorf("LongShort fetch failed %s %s: %w", sym, iv, err)
			}
			continue
		}
		out.LongShort[sym][iv] = lsr
		setDataAge(out, "lsr", sym, iv, lsr.Timestamp)
	}
	if len(out.LongShort[sym]) == 0 && f.RequireLongShort {
		return fmt.Errorf("LongShort fetch returned empty for %s", sym)
	}
	return nil
}

func dropUnclosed(candles []Candle, interval string, now time.Time) ([]Candle, error) {
	if len(candles) == 0 {
		return candles, nil
	}
	dur, err := parseInterval(interval)
	if err != nil {
		return nil, err
	}
	last := candles[len(candles)-1]
	openTime := toTime(last.OpenTime)
	if openTime.Add(dur).After(now) {
		return candles[:len(candles)-1], nil
	}
	return candles, nil
}

func parseInterval(iv string) (time.Duration, error) {
	return interval.ParseInterval(iv)
}

func toTime(ts int64) time.Time {
	if ts <= 0 {
		return time.Time{}
	}
	if ts >= 1_000_000_000_000 {
		return time.UnixMilli(ts).UTC()
	}
	return time.Unix(ts, 0).UTC()
}

func (f *Fetcher) loadLiquidations(ctx context.Context, out *MarketSnapshot, sym string) error {
	var liq LiqBlock
	requireRawLiquidations := f.RequireLiquidations && f.resolveLiquidationWindowProvider() == nil
	loaded, err := runOptionalLoad(
		f.Liquidations != nil,
		requireRawLiquidations,
		"liquidations provider is required",
		fmt.Sprintf("liquidations fetch failed %s", sym),
		func() error {
			var fetchErr error
			liq, fetchErr = f.Liquidations.Liquidations(ctx, sym)
			return fetchErr
		},
	)
	if err != nil {
		return err
	}
	if !loaded {
		return nil
	}
	out.Liquidations[sym] = liq
	setDataAge(out, "liq", sym, "", liq.Timestamp)
	return nil
}

func (f *Fetcher) loadLiquidationsByWindow(ctx context.Context, out *MarketSnapshot, sym string) error {
	provider := f.resolveLiquidationWindowProvider()
	if provider == nil {
		return nil
	}
	liq, err := provider.LiquidationsByWindow(ctx, sym)
	if err != nil {
		if f.RequireLiquidations {
			return fmt.Errorf("liquidations_by_window fetch failed %s: %w", sym, err)
		}
		return nil
	}
	source, err := f.loadLiquidationSource(ctx, sym)
	if err != nil {
		if f.RequireLiquidations {
			return fmt.Errorf("liquidation_source fetch failed %s: %w", sym, err)
		}
		source = LiqSource{}
	}
	f.enrichLiquidationWindows(out, sym, liq)
	if out.LiquidationsByWindow == nil {
		out.LiquidationsByWindow = map[string]map[string]LiqWindow{}
	}
	out.LiquidationsByWindow[sym] = liq
	if out.LiquidationSource == nil {
		out.LiquidationSource = map[string]LiqSource{}
	}
	if source.Source != "" || source.Status != "" || source.StreamConnected || source.CoverageSec > 0 || source.LastEventTime > 0 || source.LastGapResetTime > 0 {
		out.LiquidationSource[sym] = source
	}
	return nil
}

func (f *Fetcher) loadLiquidationSource(ctx context.Context, sym string) (LiqSource, error) {
	if f == nil || f.LiquidationSource == nil {
		return LiqSource{}, nil
	}
	return f.LiquidationSource.LiquidationSource(ctx, sym)
}

func (f *Fetcher) resolveLiquidationWindowProvider() LiquidationWindowProvider {
	if f == nil {
		return nil
	}
	if f.LiquidationsByWindow != nil {
		return f.LiquidationsByWindow
	}
	if typed, ok := f.Liquidations.(LiquidationWindowProvider); ok {
		return typed
	}
	return nil
}

func (f *Fetcher) enrichLiquidationWindows(out *MarketSnapshot, sym string, windows map[string]LiqWindow) {
	if f == nil || out == nil || len(windows) == 0 {
		return
	}
	now := out.Timestamp
	oiValue := 0.0
	if out.OI != nil {
		oiValue = out.OI[sym].Value
	}
	byInterval := out.Klines[sym]
	for window, item := range windows {
		if oiValue > 0 {
			item.Rel.VolOverOI = item.TotalVol / oiValue
		}
		if volume := closedVolumeForWindow(byInterval, window, now); volume > 0 {
			item.Rel.VolOverVolume = item.TotalVol / volume
		}
		if item.Complete {
			observationID := now.UTC().Unix()
			item.Rel.ZScore = f.observeLiquidationWindow(sym, window, observationID, item.TotalVol)
			item.Rel.Spike = item.Rel.ZScore >= liqSpikeThreshold
		}
		windows[window] = item
	}
}

const liqSpikeThreshold = 2.0

func closedVolumeForWindow(byInterval map[string][]Candle, window string, now time.Time) float64 {
	if len(byInterval) == 0 {
		return 0
	}
	candles := byInterval[window]
	if len(candles) == 0 {
		return 0
	}
	duration, err := parseLiquidationWindowDuration(window)
	if err != nil {
		return 0
	}
	return lastClosedWindowVolume(candles, duration, now)
}

func lastClosedWindowVolume(candles []Candle, duration time.Duration, now time.Time) float64 {
	if len(candles) == 0 {
		return 0
	}
	last := candles[len(candles)-1]
	if windowCandleClosed(last.OpenTime, duration, now) {
		return last.Volume
	}
	if len(candles) > 1 {
		return candles[len(candles)-2].Volume
	}
	return 0
}

func windowCandleClosed(openTimeMs int64, duration time.Duration, now time.Time) bool {
	if openTimeMs <= 0 || duration <= 0 {
		return false
	}
	openTime := time.UnixMilli(openTimeMs).UTC()
	return !openTime.Add(duration).After(now)
}

func (f *Fetcher) observeLiquidationWindow(symbol, window string, observationID int64, total float64) float64 {
	if observationID <= 0 {
		return 0
	}
	return f.ensureLiqHistory().Observe(symbol+"|"+window, observationID, total)
}

func (f *Fetcher) ensureLiqHistory() *liqMetricHistory {
	if f == nil {
		return newLiqMetricHistory(50)
	}
	f.liqMu.Lock()
	defer f.liqMu.Unlock()
	if f.liqHistory == nil {
		f.liqHistory = newLiqMetricHistory(50)
	}
	return f.liqHistory
}

type liqMetricHistory struct {
	mu      sync.Mutex
	entries map[string]*liqMetricHistoryEntry
	size    int
}

type liqMetricHistoryEntry struct {
	history       *liqRollingHistory
	lastBucketEnd int64
	lastZScore    float64
}

type liqRollingHistory struct {
	values []float64
	next   int
	count  int
	sum    float64
	sumsq  float64
}

func newLiqMetricHistory(size int) *liqMetricHistory {
	if size <= 0 {
		size = 1
	}
	return &liqMetricHistory{entries: make(map[string]*liqMetricHistoryEntry), size: size}
}

func (h *liqMetricHistory) Observe(key string, observationID int64, value float64) float64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	entry := h.entries[key]
	if entry == nil {
		entry = &liqMetricHistoryEntry{history: newLiqRollingHistory(h.size)}
		h.entries[key] = entry
	}
	if entry.lastBucketEnd == observationID {
		return entry.lastZScore
	}
	z := entry.history.Observe(value)
	entry.lastBucketEnd = observationID
	entry.lastZScore = z
	return z
}

func newLiqRollingHistory(size int) *liqRollingHistory {
	if size <= 0 {
		size = 1
	}
	return &liqRollingHistory{values: make([]float64, size)}
}

func (h *liqRollingHistory) Observe(value float64) float64 {
	mean, std, ok := h.meanStd()
	z := 0.0
	if ok && std > 0 {
		z = (value - mean) / std
	}
	h.add(value)
	return z
}

func (h *liqRollingHistory) meanStd() (float64, float64, bool) {
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

func (h *liqRollingHistory) add(value float64) {
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

func parseLiquidationWindowDuration(raw string) (time.Duration, error) {
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

func keyForAge(kind, sym, iv string) string {
	if sym == "" && iv == "" {
		return kind
	}
	if iv == "" {
		return kind + ":" + sym
	}
	return kind + ":" + sym + ":" + iv
}

func ageSec(now time.Time, ts int64) int64 {
	if ts <= 0 {
		return 0
	}
	ref := toTime(ts)
	if ref.IsZero() {
		return 0
	}
	d := int64(now.Sub(ref).Seconds())
	if d < 0 {
		return 0
	}
	return d
}
