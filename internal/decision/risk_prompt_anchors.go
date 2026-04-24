package decision

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"brale-core/internal/decision/decisionutil"
	"brale-core/internal/decision/features"
)

type structureAnchorCandidate struct {
	Interval string
	Price    float64
	Source   string
	Type     string
}

type latestBreakAnchor struct {
	Interval   string
	Type       string
	Age        int
	LevelPrice *float64
}

func buildStructureAnchorSummary(comp features.CompressionResult, symbol string, entry, atr float64) (map[string]any, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	byInterval, ok := comp.Trends[symbol]
	if !ok || len(byInterval) == 0 {
		return nil, fmt.Errorf("trend inputs missing for symbol=%s", symbol)
	}
	keys := decisionutil.SortedTrendKeys(byInterval)
	if len(keys) == 0 {
		return nil, fmt.Errorf("trend inputs missing for symbol=%s", symbol)
	}

	order := make(map[string]int, len(keys))
	emaByInterval := make(map[string]any, len(keys))
	swingByInterval := make(map[string]any, len(keys))
	var candidates []structureAnchorCandidate
	var latestBreak *latestBreakAnchor
	var shortestSuperTrend *features.TrendSuperTrendSnapshot

	for i, key := range keys {
		order[key] = i
		block, err := parseTrendCompressedInput(byInterval[key].RawJSON)
		if err != nil {
			return nil, fmt.Errorf("parse trend input %s/%s: %w", symbol, key, err)
		}
		if emaMap := buildEMAMap(block.GlobalContext); len(emaMap) > 0 {
			emaByInterval[key] = emaMap
			for source, value := range emaMap {
				price, ok := value.(float64)
				if !ok || price <= 0 {
					continue
				}
				candidates = append(candidates, structureAnchorCandidate{
					Interval: key,
					Price:    price,
					Source:   source,
					Type:     "ema",
				})
			}
		}
		if swingMap := buildSwingLevelMap(block.KeyLevels); len(swingMap) > 0 {
			swingByInterval[key] = swingMap
			if high, ok := swingMap["last_swing_high_price"].(float64); ok && high > 0 {
				candidates = append(candidates, structureAnchorCandidate{
					Interval: key,
					Price:    high,
					Source:   "last_swing_high",
					Type:     "resistance",
				})
			}
			if low, ok := swingMap["last_swing_low_price"].(float64); ok && low > 0 {
				candidates = append(candidates, structureAnchorCandidate{
					Interval: key,
					Price:    low,
					Source:   "last_swing_low",
					Type:     "support",
				})
			}
		}
		candidates = append(candidates, candidatesFromStructure(key, block.StructureCandidates)...)
		if block.SMC != nil {
			candidates = append(candidates, candidatesFromOrderBlock(key, block.SMC.OrderBlock)...)
		}
		if shortestSuperTrend == nil && block.SuperTrend != nil {
			shortestSuperTrend = block.SuperTrend
		}
		if next := buildLatestBreakAnchor(key, block.BreakSummary); shouldReplaceLatestBreak(latestBreak, next, order) {
			latestBreak = next
		}
	}

	summary := map[string]any{
		"ema_by_interval":        emaByInterval,
		"last_swing_by_interval": swingByInterval,
	}
	if shortestSuperTrend != nil {
		summary["supertrend"] = map[string]any{
			"interval":     shortestSuperTrend.Interval,
			"state":        shortestSuperTrend.State,
			"level":        shortestSuperTrend.Level,
			"distance_pct": shortestSuperTrend.DistancePct,
		}
	}
	if below, ok := selectNearestAnchor(candidates, entry, atr, true, order); ok {
		summary["nearest_below_entry"] = below
	}
	if above, ok := selectNearestAnchor(candidates, entry, atr, false, order); ok {
		summary["nearest_above_entry"] = above
	}
	if latestBreak != nil {
		summary["latest_break"] = latestBreak.toMap()
	}
	if len(candidates) == 0 && latestBreak == nil {
		return nil, fmt.Errorf("structure anchors unavailable for symbol=%s", symbol)
	}
	return summary, nil
}

func parseTrendCompressedInput(raw []byte) (features.TrendCompressedInput, error) {
	var block features.TrendCompressedInput
	if err := json.Unmarshal(raw, &block); err != nil {
		return features.TrendCompressedInput{}, err
	}
	return block, nil
}

func buildEMAMap(gc features.TrendGlobalContext) map[string]any {
	out := map[string]any{}
	if gc.EMA20 != nil && *gc.EMA20 > 0 {
		out["ema20"] = *gc.EMA20
	}
	if gc.EMA50 != nil && *gc.EMA50 > 0 {
		out["ema50"] = *gc.EMA50
	}
	if gc.EMA200 != nil && *gc.EMA200 > 0 {
		out["ema200"] = *gc.EMA200
	}
	return out
}

func buildSwingLevelMap(levels *features.TrendKeyLevels) map[string]any {
	if levels == nil {
		return nil
	}
	out := map[string]any{}
	if levels.LastSwingHigh != nil && levels.LastSwingHigh.Price > 0 {
		out["last_swing_high_price"] = levels.LastSwingHigh.Price
	}
	if levels.LastSwingLow != nil && levels.LastSwingLow.Price > 0 {
		out["last_swing_low_price"] = levels.LastSwingLow.Price
	}
	return out
}

func candidatesFromStructure(interval string, items []features.TrendStructureCandidate) []structureAnchorCandidate {
	out := make([]structureAnchorCandidate, 0, len(items))
	for _, item := range items {
		if item.Price <= 0 {
			continue
		}
		out = append(out, structureAnchorCandidate{
			Interval: interval,
			Price:    item.Price,
			Source:   item.Source,
			Type:     item.Type,
		})
	}
	return out
}

func candidatesFromOrderBlock(interval string, block features.TrendOrderBlock) []structureAnchorCandidate {
	out := make([]structureAnchorCandidate, 0, 2)
	if block.Lower != nil && *block.Lower > 0 {
		out = append(out, structureAnchorCandidate{
			Interval: interval,
			Price:    *block.Lower,
			Source:   "order_block_lower",
			Type:     block.Type,
		})
	}
	if block.Upper != nil && *block.Upper > 0 {
		out = append(out, structureAnchorCandidate{
			Interval: interval,
			Price:    *block.Upper,
			Source:   "order_block_upper",
			Type:     block.Type,
		})
	}
	return out
}

func buildLatestBreakAnchor(interval string, summary *features.TrendBreakSummary) *latestBreakAnchor {
	if summary == nil || summary.LatestEventAge == nil || summary.LatestEventType == "" || summary.LatestEventType == "none" {
		return nil
	}
	out := &latestBreakAnchor{
		Interval: interval,
		Type:     summary.LatestEventType,
		Age:      *summary.LatestEventAge,
	}
	if summary.LatestEventLevelPrice != nil && *summary.LatestEventLevelPrice > 0 {
		out.LevelPrice = summary.LatestEventLevelPrice
	}
	return out
}

func shouldReplaceLatestBreak(current, candidate *latestBreakAnchor, order map[string]int) bool {
	if candidate == nil {
		return false
	}
	if current == nil {
		return true
	}
	if candidate.Age < current.Age {
		return true
	}
	if candidate.Age > current.Age {
		return false
	}
	return order[candidate.Interval] < order[current.Interval]
}

func (b *latestBreakAnchor) toMap() map[string]any {
	out := map[string]any{
		"interval": b.Interval,
		"type":     b.Type,
		"bar_age":  b.Age,
	}
	if b.LevelPrice != nil {
		out["level_price"] = *b.LevelPrice
	}
	return out
}

func selectNearestAnchor(candidates []structureAnchorCandidate, entry, atr float64, below bool, order map[string]int) (map[string]any, bool) {
	var selected *structureAnchorCandidate
	bestDistance := 0.0
	for i := range candidates {
		candidate := candidates[i]
		if candidate.Price <= 0 {
			continue
		}
		if below {
			if candidate.Price >= entry {
				continue
			}
		} else if candidate.Price <= entry {
			continue
		}
		distance := math.Abs(entry - candidate.Price)
		if selected == nil || distance < bestDistance || (distance == bestDistance && order[candidate.Interval] < order[selected.Interval]) {
			selected = &candidate
			bestDistance = distance
		}
	}
	if selected == nil {
		return nil, false
	}
	out := map[string]any{
		"interval": selected.Interval,
		"price":    selected.Price,
		"source":   selected.Source,
		"type":     selected.Type,
	}
	if atr > 0 {
		out["distance_atr"] = math.Abs(entry-selected.Price) / atr
	}
	return out, true
}
