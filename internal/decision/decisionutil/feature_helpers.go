package decisionutil

import (
	"encoding/json"
	"fmt"
	"sort"

	"brale-core/internal/decision/features"
	"brale-core/internal/interval"
)

type SimpleIndicator struct {
	Open          float64 `json:"open"`
	High          float64 `json:"high"`
	Low           float64 `json:"low"`
	Close         float64 `json:"close"`
	ATR           float64 `json:"atr"`
	RSI           float64 `json:"rsi"`
	RSIOK         bool    `json:"rsi_ok"`
	PctChange5m   float64 `json:"pct_change_5m"`
	PctChange5mOK bool    `json:"pct_change_5m_ok"`
}

func PickIndicator(data features.CompressionResult, symbol string) (SimpleIndicator, error) {
	if byInterval, ok := data.Indicators[symbol]; ok {
		keys := SortedIndicatorKeys(byInterval)
		if len(keys) == 0 {
			return SimpleIndicator{}, fmt.Errorf("no indicator intervals for symbol=%s", symbol)
		}
		iv := keys[0]
		var raw struct {
			Open   float64 `json:"open"`
			High   float64 `json:"high"`
			Low    float64 `json:"low"`
			Close  float64 `json:"close"`
			ATR    float64 `json:"atr"`
			Market struct {
				CurrentPrice float64 `json:"current_price"`
			} `json:"market"`
			Data struct {
				ATR struct {
					Latest float64 `json:"latest"`
				} `json:"atr"`
				RSI *struct {
					Current float64   `json:"current"`
					LastN   []float64 `json:"last_n"`
				} `json:"rsi"`
			} `json:"data"`
		}
		if err := json.Unmarshal(byInterval[iv].RawJSON, &raw); err != nil {
			return SimpleIndicator{}, fmt.Errorf("indicator json unmarshal failed for %s/%s: %w", symbol, iv, err)
		}
		closeVal := raw.Close
		if closeVal == 0 {
			closeVal = raw.Market.CurrentPrice
		}
		atrVal := raw.ATR
		if atrVal == 0 {
			atrVal = raw.Data.ATR.Latest
		}
		rsiVal, rsiOK := pickRSI(raw.Data.RSI)
		pct5m, pct5mOK := pickPctChange5m(data, symbol)
		return SimpleIndicator{
			Open:          raw.Open,
			High:          raw.High,
			Low:           raw.Low,
			Close:         closeVal,
			ATR:           atrVal,
			RSI:           rsiVal,
			RSIOK:         rsiOK,
			PctChange5m:   pct5m,
			PctChange5mOK: pct5mOK,
		}, nil
	}
	return SimpleIndicator{}, fmt.Errorf("indicator missing for symbol=%s", symbol)
}

func pickRSI(rsi *struct {
	Current float64   `json:"current"`
	LastN   []float64 `json:"last_n"`
}) (float64, bool) {
	if rsi == nil {
		return 0, false
	}
	if rsi.Current != 0 {
		return rsi.Current, true
	}
	if len(rsi.LastN) == 0 {
		return 0, true
	}
	return rsi.LastN[len(rsi.LastN)-1], true
}

func pickPctChange5m(data features.CompressionResult, symbol string) (float64, bool) {
	trendJSON, ok := PickTrendJSONByInterval(data, symbol, "5m")
	if !ok {
		return 0, false
	}
	var raw struct {
		RecentCandles []struct {
			Close float64 `json:"c"`
		} `json:"recent_candles"`
	}
	if err := json.Unmarshal(trendJSON.RawJSON, &raw); err != nil {
		return 0, false
	}
	if len(raw.RecentCandles) < 2 {
		return 0, false
	}
	prev := raw.RecentCandles[len(raw.RecentCandles)-2].Close
	last := raw.RecentCandles[len(raw.RecentCandles)-1].Close
	if prev <= 0 {
		return 0, false
	}
	return (last - prev) / prev * 100, true
}

func PickIndicatorJSON(data features.CompressionResult, symbol string) (features.IndicatorJSON, bool) {
	if byInterval, ok := data.Indicators[symbol]; ok {
		for _, iv := range SortedIndicatorKeys(byInterval) {
			return byInterval[iv], true
		}
	}
	return features.IndicatorJSON{}, false
}

func PickIndicatorJSONByInterval(data features.CompressionResult, symbol, interval string) (features.IndicatorJSON, bool) {
	if byInterval, ok := data.Indicators[symbol]; ok {
		if out, ok := byInterval[interval]; ok {
			return out, true
		}
	}
	return features.IndicatorJSON{}, false
}

func PickTrendJSON(data features.CompressionResult, symbol string) (features.TrendJSON, bool) {
	if byInterval, ok := data.Trends[symbol]; ok {
		for _, iv := range SortedTrendKeys(byInterval) {
			return byInterval[iv], true
		}
	}
	return features.TrendJSON{}, false
}

func PickTrendJSONByInterval(data features.CompressionResult, symbol, interval string) (features.TrendJSON, bool) {
	if byInterval, ok := data.Trends[symbol]; ok {
		if out, ok := byInterval[interval]; ok {
			return out, true
		}
	}
	return features.TrendJSON{}, false
}

func PickMechanicsJSON(data features.CompressionResult, symbol string) (features.MechanicsSnapshot, bool) {
	if mech, ok := data.Mechanics[symbol]; ok {
		return mech, true
	}
	return features.MechanicsSnapshot{}, false
}

func SortedIndicatorKeys(m map[string]features.IndicatorJSON) []string {
	return SortedIntervalKeys(m)
}

func SortedTrendKeys(m map[string]features.TrendJSON) []string {
	return SortedIntervalKeys(m)
}

func SortedIntervalKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		di, errI := interval.ParseInterval(keys[i])
		dj, errJ := interval.ParseInterval(keys[j])
		if errI != nil || errJ != nil {
			return keys[i] < keys[j]
		}
		return di < dj
	})
	return keys
}
