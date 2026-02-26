package market

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"brale-core/internal/snapshot"

	"github.com/shopspring/decimal"
)

const (
	sentimentTTL         = 60 * time.Second
	sentimentRatioTTL    = 120 * time.Second
	sentimentOIHistTTL   = 120 * time.Second
	sentimentHistoryBars = 100
)

type SentimentFactors struct {
	OpenInterest  decimal.Decimal
	FundingRate   decimal.Decimal
	BigWhales     decimal.Decimal
	BigAccounts   decimal.Decimal
	RetailInverse decimal.Decimal
	VolumeEmotion decimal.Decimal
}

type SentimentData struct {
	Symbol    string
	Interval  string
	Score     int
	Tag       string
	Factors   SentimentFactors
	UpdatedAt time.Time
	Error     string
}

type sentimentCacheEntry struct {
	At   time.Time
	Data SentimentData
}

type ratioCacheEntry struct {
	At    time.Time
	Items []LongShortRatioPoint
}

type oiHistoryCacheEntry struct {
	At    time.Time
	Items []OpenInterestPoint
}

type SentimentService struct {
	store   KlineStore
	metrics *MetricsService
	source  Source
	ratios  LongShortRatioProvider

	mu            sync.RWMutex
	sentiment     map[string]sentimentCacheEntry
	ratioCache    map[string]ratioCacheEntry
	oiHistCache   map[string]oiHistoryCacheEntry
	lookbackByIV  map[string]int
	weightsByIV   map[string]sentimentWeights
	clock         func() time.Time
	volumeMaxBars int
}

type sentimentWeights struct {
	OI      decimal.Decimal
	Funding decimal.Decimal
	Big     decimal.Decimal
	BigAcc  decimal.Decimal
	Retail  decimal.Decimal
	Volume  decimal.Decimal
}

func NewSentimentService(store KlineStore, source Source, metrics *MetricsService) *SentimentService {
	svc := &SentimentService{
		store:         store,
		metrics:       metrics,
		source:        source,
		sentiment:     make(map[string]sentimentCacheEntry),
		ratioCache:    make(map[string]ratioCacheEntry),
		oiHistCache:   make(map[string]oiHistoryCacheEntry),
		clock:         time.Now,
		volumeMaxBars: sentimentHistoryBars,
		lookbackByIV: map[string]int{
			"5m":  20,
			"15m": 12,
			"1h":  5,
			"4h":  3,
			"1d":  2,
		},
		weightsByIV: map[string]sentimentWeights{
			"5m":  {OI: dec("0.1"), Funding: dec("0.2"), Big: dec("0.2"), BigAcc: dec("0.1"), Retail: dec("0.2"), Volume: dec("0.2")},
			"15m": {OI: dec("0.15"), Funding: dec("0.2"), Big: dec("0.25"), BigAcc: dec("0.1"), Retail: dec("0.2"), Volume: dec("0.1")},
			"1h":  {OI: dec("0.2"), Funding: dec("0.2"), Big: dec("0.25"), BigAcc: dec("0.1"), Retail: dec("0.15"), Volume: dec("0.1")},
			"4h":  {OI: dec("0.25"), Funding: dec("0.15"), Big: dec("0.3"), BigAcc: dec("0.1"), Retail: dec("0.2"), Volume: dec("0.0")},
			"1d":  {OI: dec("0.3"), Funding: dec("0.1"), Big: dec("0.35"), BigAcc: dec("0.1"), Retail: dec("0.2"), Volume: dec("0.0")},
		},
	}
	if ratioProvider, ok := source.(LongShortRatioProvider); ok {
		svc.ratios = ratioProvider
	}
	return svc
}

// Calculate builds a sentiment score from derivatives + volume signals.
func (s *SentimentService) Calculate(ctx context.Context, symbol, interval string, candles []snapshot.Candle) (SentimentData, bool) {
	if s == nil || s.metrics == nil || s.source == nil {
		return SentimentData{}, false
	}
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	interval = strings.ToLower(strings.TrimSpace(interval))
	if symbol == "" || interval == "" {
		return SentimentData{}, false
	}
	now := s.now()
	cacheKey := symbol + "|" + interval
	if cached, ok := s.getSentiment(cacheKey, now, sentimentTTL); ok {
		return cached, true
	}

	volumeScore := decimal.Zero
	if len(candles) == 0 && s.store != nil {
		if stored, err := s.store.Get(ctx, symbol, interval); err == nil {
			candles = stored
		}
	}
	if len(candles) > 0 {
		if len(candles) > s.volumeMaxBars {
			candles = candles[len(candles)-s.volumeMaxBars:]
		}
		volumeScore = calcVolumeScore(candles)
	}

	oiScore := decimal.NewFromFloat(0.5)
	fundingScore := decimal.Zero
	if data, ok := s.metrics.Get(symbol); ok && data.Error == "" {
		curOI := decimal.NewFromFloat(data.OI)
		fundingScore = normalize(decFromFloat(data.FundingRate), dec("-0.02"), dec("0.02"))
		if hist := s.getOIHistory(ctx, symbol, "1h", 10, now); len(hist) > 0 {
			minOI, maxOI := minMaxOI(hist)
			if maxOI.Equal(minOI) {
				oiScore = decimal.NewFromFloat(0.5)
			} else {
				oiScore = normalize(curOI, minOI, maxOI)
			}
		}
	}

	lookback := s.lookbackByIV[interval]
	if lookback == 0 {
		lookback = 3
	}
	topPosVal := avgRatio(s.getRatio(ctx, "top_pos", symbol, interval, lookback, now))
	topAccVal := avgRatio(s.getRatio(ctx, "top_acc", symbol, interval, lookback, now))
	globalVal := avgRatio(s.getRatio(ctx, "global_acc", symbol, interval, lookback, now))

	bigPlayerScore := normalize(topPosVal, dec("0.9"), dec("2.0"))
	bigAccountScore := normalize(topAccVal, dec("0.9"), dec("2.0"))
	crowdInverseScore := normalizeInverse(globalVal, dec("0.8"), dec("1.2"))

	weights := s.weightsByIV[interval]
	if weights == (sentimentWeights{}) {
		weights = s.weightsByIV["1h"]
	}

	score := weights.OI.Mul(oiScore).
		Add(weights.Funding.Mul(fundingScore)).
		Add(weights.Big.Mul(bigPlayerScore)).
		Add(weights.BigAcc.Mul(bigAccountScore)).
		Add(weights.Retail.Mul(crowdInverseScore)).
		Add(weights.Volume.Mul(volumeScore))

	score100 := int(decimal.Max(decimal.Zero, decimal.Min(dec("100"), score.Mul(dec("100")))).IntPart())

	tag := sentimentTag(score100)
	data := SentimentData{
		Symbol:   symbol,
		Interval: interval,
		Score:    score100,
		Tag:      tag,
		Factors: SentimentFactors{
			OpenInterest:  round(oiScore, 3),
			FundingRate:   round(fundingScore, 3),
			BigWhales:     round(bigPlayerScore, 3),
			BigAccounts:   round(bigAccountScore, 3),
			RetailInverse: round(crowdInverseScore, 3),
			VolumeEmotion: round(volumeScore, 3),
		},
		UpdatedAt: now,
	}
	s.setSentiment(cacheKey, data)
	return data, true
}

func (s *SentimentService) now() time.Time {
	if s.clock != nil {
		return s.clock()
	}
	return time.Now()
}

func (s *SentimentService) getSentiment(key string, now time.Time, ttl time.Duration) (SentimentData, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.sentiment[key]
	if !ok || now.Sub(entry.At) > ttl {
		return SentimentData{}, false
	}
	return entry.Data, true
}

func (s *SentimentService) setSentiment(key string, data SentimentData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sentiment[key] = sentimentCacheEntry{At: s.now(), Data: data}
}

func (s *SentimentService) getRatio(ctx context.Context, group, symbol, interval string, limit int, now time.Time) []LongShortRatioPoint {
	if s.ratios == nil {
		return nil
	}
	key := fmt.Sprintf("%s:%s:%s:%d", group, symbol, interval, limit)
	s.mu.RLock()
	entry, ok := s.ratioCache[key]
	s.mu.RUnlock()
	if ok && now.Sub(entry.At) <= sentimentRatioTTL {
		return entry.Items
	}
	var (
		items []LongShortRatioPoint
		err   error
	)
	switch group {
	case "top_pos":
		items, err = s.ratios.TopPositionRatio(ctx, symbol, interval, limit)
	case "top_acc":
		items, err = s.ratios.TopAccountRatio(ctx, symbol, interval, limit)
	case "global_acc":
		items, err = s.ratios.GlobalAccountRatio(ctx, symbol, interval, limit)
	default:
		return nil
	}
	if err != nil {
		return nil
	}
	s.mu.Lock()
	s.ratioCache[key] = ratioCacheEntry{At: now, Items: items}
	s.mu.Unlock()
	return items
}

func (s *SentimentService) getOIHistory(ctx context.Context, symbol, period string, limit int, now time.Time) []OpenInterestPoint {
	key := fmt.Sprintf("%s:%s:%d", symbol, period, limit)
	s.mu.RLock()
	entry, ok := s.oiHistCache[key]
	s.mu.RUnlock()
	if ok && now.Sub(entry.At) <= sentimentOIHistTTL {
		return entry.Items
	}
	items, err := s.source.GetOpenInterestHistory(ctx, symbol, period, limit)
	if err != nil {
		return nil
	}
	s.mu.Lock()
	s.oiHistCache[key] = oiHistoryCacheEntry{At: now, Items: items}
	s.mu.Unlock()
	return items
}

func calcVolumeScore(candles []snapshot.Candle) decimal.Decimal {
	if len(candles) == 0 {
		return decimal.Zero
	}
	sum := decimal.Zero
	for _, c := range candles {
		sum = sum.Add(decimal.NewFromFloat(c.Volume))
	}
	avg := decimal.Zero
	if len(candles) > 0 {
		avg = sum.Div(decimal.NewFromInt(int64(len(candles))))
	}
	cur := decimal.NewFromFloat(candles[len(candles)-1].Volume)
	if avg.IsZero() {
		return decimal.Zero
	}
	ratio := cur.Div(avg)
	return normalize(ratio, dec("0.5"), dec("3.0"))
}

func minMaxOI(hist []OpenInterestPoint) (decimal.Decimal, decimal.Decimal) {
	if len(hist) == 0 {
		return decimal.Zero, decimal.Zero
	}
	minVal := decimal.NewFromFloat(hist[0].SumOpenInterest)
	maxVal := minVal
	for _, p := range hist[1:] {
		val := decimal.NewFromFloat(p.SumOpenInterest)
		if val.LessThan(minVal) {
			minVal = val
		}
		if val.GreaterThan(maxVal) {
			maxVal = val
		}
	}
	return minVal, maxVal
}

func avgRatio(items []LongShortRatioPoint) decimal.Decimal {
	if len(items) == 0 {
		return decimal.Zero
	}
	sum := decimal.Zero
	for _, v := range items {
		sum = sum.Add(decimal.NewFromFloat(v.Ratio))
	}
	return sum.Div(decimal.NewFromInt(int64(len(items))))
}

func normalize(value, minV, maxV decimal.Decimal) decimal.Decimal {
	if value.Equal(decimal.Zero) && minV.Equal(decimal.Zero) && maxV.Equal(decimal.Zero) {
		return decimal.Zero
	}
	if maxV.Equal(minV) {
		return decimal.Zero
	}
	val := value.Sub(minV).Div(maxV.Sub(minV))
	if val.LessThan(decimal.Zero) {
		return decimal.Zero
	}
	if val.GreaterThan(decimal.NewFromFloat(1)) {
		return decimal.NewFromFloat(1)
	}
	return val
}

func normalizeInverse(value, minV, maxV decimal.Decimal) decimal.Decimal {
	if maxV.Equal(minV) {
		return decimal.Zero
	}
	val := maxV.Sub(value).Div(maxV.Sub(minV))
	if val.LessThan(decimal.Zero) {
		return decimal.Zero
	}
	if val.GreaterThan(decimal.NewFromFloat(1)) {
		return decimal.NewFromFloat(1)
	}
	return val
}

func sentimentTag(score int) string {
	switch {
	case score >= 85:
		return "Strong Long"
	case score >= 65:
		return "Long Bias"
	case score >= 45:
		return "Neutral"
	case score >= 25:
		return "Short Bias"
	default:
		return "Strong Short"
	}
}

func round(val decimal.Decimal, places int32) decimal.Decimal {
	return val.Round(places)
}

func dec(value string) decimal.Decimal {
	out, err := decimal.NewFromString(value)
	if err != nil {
		return decimal.Zero
	}
	return out
}

func decFromFloat(val float64) decimal.Decimal {
	return decimal.NewFromFloat(val)
}
