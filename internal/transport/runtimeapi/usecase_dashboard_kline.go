package runtimeapi

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"brale-core/internal/interval"
	"brale-core/internal/runtime"
	"brale-core/internal/snapshot"
)

var dashboardKlineNow = time.Now

type dashboardKlineCacheEntry struct {
	createdAt time.Time
	payload   DashboardKlineResponse
}

type dashboardKlineUsecase struct {
	resolver      SymbolResolver
	allowSymbol   func(string) bool
	klineProvider snapshot.KlineProvider
	now           func() time.Time
	cacheMu       *sync.RWMutex
	cache         map[string]dashboardKlineCacheEntry
}

func newDashboardKlineUsecase(s *Server) dashboardKlineUsecase {
	if s == nil {
		return dashboardKlineUsecase{}
	}
	if s.klineCache == nil {
		s.klineCacheMu.Lock()
		if s.klineCache == nil {
			s.klineCache = make(map[string]dashboardKlineCacheEntry)
		}
		s.klineCacheMu.Unlock()
	}
	return dashboardKlineUsecase{
		resolver:      s.Resolver,
		allowSymbol:   s.AllowSymbol,
		klineProvider: s.KlineProvider,
		now:           dashboardKlineNow,
		cacheMu:       &s.klineCacheMu,
		cache:         s.klineCache,
	}
}

func (u dashboardKlineUsecase) build(ctx context.Context, rawSymbol, rawInterval string, requestedLimit int) (DashboardKlineResponse, *usecaseError) {
	if u.resolver == nil {
		return DashboardKlineResponse{}, &usecaseError{Status: 500, Code: "resolver_missing", Message: "symbol resolver 未初始化"}
	}
	if u.klineProvider == nil {
		return DashboardKlineResponse{}, &usecaseError{Status: 502, Code: "dashboard_kline_failed", Message: "dashboard K 线获取失败", Details: "kline provider missing"}
	}

	normalizedSymbol := runtime.NormalizeSymbol(strings.TrimSpace(rawSymbol))
	if normalizedSymbol == "" || !isValidDashboardSymbol(normalizedSymbol) {
		return DashboardKlineResponse{}, &usecaseError{Status: 400, Code: "invalid_symbol", Message: "symbol 非法", Details: rawSymbol}
	}
	if u.allowSymbol != nil && !u.allowSymbol(normalizedSymbol) {
		return DashboardKlineResponse{}, &usecaseError{Status: 400, Code: "symbol_not_allowed", Message: "symbol 不在允许列表", Details: normalizedSymbol}
	}

	resolved, err := u.resolver.Resolve(normalizedSymbol)
	if err != nil {
		return DashboardKlineResponse{}, &usecaseError{Status: 400, Code: "symbol_invalid", Message: "symbol 配置不可用", Details: err.Error()}
	}

	requestedInterval := strings.ToLower(strings.TrimSpace(rawInterval))
	if requestedInterval == "" {
		return DashboardKlineResponse{}, &usecaseError{Status: 400, Code: "interval_required", Message: "interval 不能为空"}
	}
	if !containsInterval(resolved.Intervals, requestedInterval) {
		return DashboardKlineResponse{}, &usecaseError{Status: 400, Code: "unsupported_interval", Message: "interval 不受支持", Details: map[string]any{"supported_intervals": normalizedIntervals(resolved.Intervals)}}
	}

	effectiveLimit := requestedLimit
	if resolved.KlineLimit > 0 && effectiveLimit > resolved.KlineLimit {
		effectiveLimit = resolved.KlineLimit
	}
	cacheKey := fmt.Sprintf("%s|%s|%d", resolved.Symbol, requestedInterval, effectiveLimit)
	if cached, ok := u.loadCache(cacheKey, requestedInterval); ok {
		return cached, nil
	}

	candles, err := u.klineProvider.Klines(ctx, resolved.Symbol, requestedInterval, effectiveLimit+1)
	if err != nil {
		return DashboardKlineResponse{}, &usecaseError{Status: 502, Code: "dashboard_kline_failed", Message: "dashboard K 线获取失败", Details: err.Error()}
	}

	dur, err := interval.ParseInterval(requestedInterval)
	if err != nil {
		return DashboardKlineResponse{}, &usecaseError{Status: 400, Code: "unsupported_interval", Message: "interval 不受支持", Details: rawInterval}
	}
	now := time.Now().UTC()
	if u.now != nil {
		now = u.now().UTC()
	}
	visible := filterVisibleCandles(candles, dur, now)
	if len(visible) > effectiveLimit {
		visible = visible[len(visible)-effectiveLimit:]
	}

	resp := DashboardKlineResponse{
		Status:   "ok",
		Symbol:   resolved.Symbol,
		Interval: requestedInterval,
		Limit:    effectiveLimit,
		Candles:  mapDashboardCandles(visible, dur),
		Summary:  dashboardContractSummary,
	}
	u.storeCache(cacheKey, resp)
	return resp, nil
}

func (u dashboardKlineUsecase) loadCache(key, intervalValue string) (DashboardKlineResponse, bool) {
	if u.cacheMu == nil || u.cache == nil {
		return DashboardKlineResponse{}, false
	}
	u.cacheMu.RLock()
	entry, ok := u.cache[key]
	u.cacheMu.RUnlock()
	if !ok {
		return DashboardKlineResponse{}, false
	}
	now := time.Now().UTC()
	if u.now != nil {
		now = u.now().UTC()
	}
	if now.Sub(entry.createdAt) > klineCacheTTL(intervalValue) {
		u.cacheMu.Lock()
		delete(u.cache, key)
		u.cacheMu.Unlock()
		return DashboardKlineResponse{}, false
	}
	return entry.payload, true
}

func (u dashboardKlineUsecase) storeCache(key string, payload DashboardKlineResponse) {
	if u.cacheMu == nil || u.cache == nil {
		return
	}
	now := time.Now().UTC()
	if u.now != nil {
		now = u.now().UTC()
	}
	u.cacheMu.Lock()
	u.cache[key] = dashboardKlineCacheEntry{createdAt: now, payload: payload}
	u.cacheMu.Unlock()
}

func klineCacheTTL(intervalValue string) time.Duration {
	dur, err := interval.ParseInterval(strings.ToLower(strings.TrimSpace(intervalValue)))
	if err != nil || dur <= 0 {
		return 5 * time.Second
	}
	ttl := dur / 10
	if ttl < 10*time.Second {
		ttl = 10 * time.Second
	}
	if ttl > 60*time.Second {
		ttl = 60 * time.Second
	}
	return ttl
}

func containsInterval(intervals []string, target string) bool {
	for _, intervalValue := range intervals {
		if strings.EqualFold(strings.TrimSpace(intervalValue), target) {
			return true
		}
	}
	return false
}

func normalizedIntervals(intervals []string) []string {
	out := make([]string, 0, len(intervals))
	for _, intervalValue := range intervals {
		value := strings.ToLower(strings.TrimSpace(intervalValue))
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func filterVisibleCandles(candles []snapshot.Candle, dur time.Duration, now time.Time) []snapshot.Candle {
	visible := make([]snapshot.Candle, 0, len(candles))
	for _, candle := range candles {
		if candle.OpenTime <= 0 {
			continue
		}
		openAt := time.UnixMilli(candle.OpenTime).UTC()
		if openAt.After(now.Add(dur)) {
			continue
		}
		visible = append(visible, candle)
	}
	return visible
}

func mapDashboardCandles(candles []snapshot.Candle, dur time.Duration) []DashboardCandle {
	out := make([]DashboardCandle, 0, len(candles))
	for _, candle := range candles {
		out = append(out, DashboardCandle{
			OpenTime:  candle.OpenTime,
			CloseTime: candle.OpenTime + dur.Milliseconds(),
			Open:      candle.Open,
			High:      candle.High,
			Low:       candle.Low,
			Close:     candle.Close,
			Volume:    candle.Volume,
		})
	}
	return out
}
