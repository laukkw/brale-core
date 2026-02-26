// 本文件主要内容：定义风控计划结构、触发判断与辅助函数。

package risk

import (
	"math"
	"strconv"
	"strings"
)

type RiskPlanInput struct {
	Entry            float64
	StopLoss         float64
	PositionSize     float64
	TakeProfits      []float64
	TakeProfitRatios []float64
}

type RiskPlan struct {
	StopPrice     float64   `json:"stop_price"`
	TPLevels      []TPLevel `json:"tp_levels"`
	InitialQty    float64   `json:"initial_qty,omitempty"`
	HighWaterMark float64   `json:"high_water_mark,omitempty"`
	LowWaterMark  float64   `json:"low_water_mark,omitempty"`
}

type TPLevel struct {
	LevelID string  `json:"level_id"`
	Price   float64 `json:"price"`
	QtyPct  float64 `json:"qty_pct"`
	Hit     bool    `json:"hit"`
}

type RiskTrigger struct {
	Type    string
	Price   float64
	LevelID string
	QtyPct  float64
	Reason  string
}

func BuildRiskPlan(input RiskPlanInput) RiskPlan {
	levels := buildTPLevels(input)
	watermark := 0.0
	if input.Entry > 0 {
		watermark = input.Entry
	}
	return RiskPlan{
		StopPrice:     input.StopLoss,
		TPLevels:      levels,
		InitialQty:    input.PositionSize,
		HighWaterMark: watermark,
		LowWaterMark:  watermark,
	}
}

func CompactRiskPlan(plan RiskPlan) RiskPlan {
	return RiskPlan{
		StopPrice:     plan.StopPrice,
		TPLevels:      plan.TPLevels,
		InitialQty:    plan.InitialQty,
		HighWaterMark: plan.HighWaterMark,
		LowWaterMark:  plan.LowWaterMark,
	}
}

func EvaluateRisk(plan RiskPlan, side string, price float64) (RiskTrigger, bool) {
	if price == 0 {
		return RiskTrigger{}, false
	}
	side = strings.ToLower(strings.TrimSpace(side))
	if hitStop(plan, side, price) {
		return RiskTrigger{Type: "STOP_LOSS", Price: price}, true
	}
	level, ok := hitTP(plan, side, price)
	if ok {
		return RiskTrigger{Type: "TAKE_PROFIT", Price: price, LevelID: level.LevelID, QtyPct: level.QtyPct}, true
	}
	return RiskTrigger{}, false
}

const tpReasonPrefix = "take_profit_hit"

func FormatTPReason(levelID string) string {
	levelID = strings.TrimSpace(levelID)
	if levelID == "" {
		return tpReasonPrefix
	}
	return tpReasonPrefix + ":" + levelID
}

func ParseTPLevelID(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ""
	}
	lower := strings.ToLower(reason)
	if !strings.HasPrefix(lower, tpReasonPrefix) {
		return ""
	}
	rest := strings.TrimSpace(reason[len(tpReasonPrefix):])
	rest = strings.TrimLeft(rest, ":|# ")
	if rest == "" {
		return ""
	}
	parts := strings.FieldsFunc(rest, func(r rune) bool {
		return r == ':' || r == '|' || r == ',' || r == ' '
	})
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func MarkTPLevelHit(plan RiskPlan, levelID string) (RiskPlan, bool) {
	if levelID == "" {
		return plan, false
	}
	updated := false
	for i := range plan.TPLevels {
		if strings.EqualFold(plan.TPLevels[i].LevelID, levelID) {
			if !plan.TPLevels[i].Hit {
				plan.TPLevels[i].Hit = true
				updated = true
			}
			break
		}
	}
	return plan, updated
}

func HasTPHits(plan RiskPlan) bool {
	for _, lv := range plan.TPLevels {
		if lv.Hit {
			return true
		}
	}
	return false
}

func TightenTPLevels(plan RiskPlan, side string, entry float64, prevStop float64, nextStop float64) (RiskPlan, bool) {
	if len(plan.TPLevels) == 0 {
		return plan, false
	}
	if entry <= 0 || prevStop <= 0 || nextStop <= 0 {
		return plan, false
	}
	prevRisk := math.Abs(entry - prevStop)
	nextRisk := math.Abs(entry - nextStop)
	if prevRisk <= 0 || nextRisk <= 0 {
		return plan, false
	}
	updated := false
	for i := range plan.TPLevels {
		level := plan.TPLevels[i]
		if level.Price <= 0 {
			continue
		}
		ratio := math.Abs(level.Price-entry) / prevRisk
		if ratio <= 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
			continue
		}
		nextPrice := computeTPPrice(side, entry, nextRisk, ratio)
		if nextPrice <= 0 || math.IsNaN(nextPrice) || math.IsInf(nextPrice, 0) {
			continue
		}
		if strings.EqualFold(side, "short") {
			if nextPrice < level.Price {
				nextPrice = level.Price
			}
		} else if nextPrice > level.Price {
			nextPrice = level.Price
		}
		if nextPrice != level.Price {
			plan.TPLevels[i].Price = nextPrice
			updated = true
		}
	}
	return plan, updated
}

func computeTPPrice(side string, entry float64, risk float64, ratio float64) float64 {
	if entry <= 0 || risk <= 0 || ratio <= 0 {
		return 0
	}
	if strings.EqualFold(side, "short") {
		return entry - risk*ratio
	}
	return entry + risk*ratio
}

func buildTPLevels(input RiskPlanInput) []TPLevel {
	if len(input.TakeProfits) == 0 {
		return nil
	}
	out := make([]TPLevel, 0, len(input.TakeProfits))
	for i, tp := range input.TakeProfits {
		out = append(out, TPLevel{
			LevelID: "tp-" + strconv.Itoa(i+1),
			Price:   tp,
			QtyPct:  resolveTPPct(i, input),
			Hit:     false,
		})
	}
	if len(out) > 0 {
		out[len(out)-1].QtyPct = 1.0
	}
	return out
}

func resolveTPPct(idx int, input RiskPlanInput) float64 {
	if idx < 0 {
		return 1
	}
	if len(input.TakeProfitRatios) > idx {
		if input.TakeProfitRatios[idx] > 0 {
			return input.TakeProfitRatios[idx]
		}
		return 1
	}
	if len(input.TakeProfits) > 0 {
		return 1 / float64(len(input.TakeProfits))
	}
	return 1
}

func hitStop(plan RiskPlan, side string, price float64) bool {
	stop := plan.StopPrice
	if stop == 0 {
		return false
	}
	switch side {
	case "long":
		return price <= stop
	case "short":
		return price >= stop
	default:
		return false
	}
}

func hitTP(plan RiskPlan, side string, price float64) (TPLevel, bool) {
	for _, lv := range plan.TPLevels {
		if lv.Hit {
			continue
		}
		switch side {
		case "long":
			if price >= lv.Price {
				return lv, true
			}
		case "short":
			if price <= lv.Price {
				return lv, true
			}
		}
	}
	return TPLevel{}, false
}
