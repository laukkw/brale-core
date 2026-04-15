package position

import (
	"math"
	"strings"

	"brale-core/internal/risk"
	"brale-core/internal/store"
)

const closeQtyPrecision = 1e-8

func resolveCloseQty(pos store.PositionRecord, plan risk.RiskPlan, trigger risk.RiskTrigger, statusQty float64) (float64, float64, string) {
	positionQty := resolveBaseCloseQty(pos, statusQty)
	closeQty := positionQty
	reason := resolveCloseReason(trigger)
	if isPartialTakeProfit(trigger) {
		baseQty := resolvePartialTPBaseQty(plan)
		if baseQty > 0 {
			closeQty = baseQty * trigger.QtyPct
		} else {
			closeQty = 0
		}
	}
	closeQty = floorCloseQty(closeQty)
	closeQty = clipCloseQty(closeQty, positionQty)
	closeQty = cleanupDustCloseQty(closeQty, positionQty)
	return closeQty, positionQty, reason
}

func resolveBaseCloseQty(pos store.PositionRecord, statusQty float64) float64 {
	if statusQty > 0 {
		return statusQty
	}
	if pos.Qty > 0 {
		return pos.Qty
	}
	return 0
}

func resolveCloseReason(trigger risk.RiskTrigger) string {
	switch trigger.Type {
	case "FORCE_EXIT":
		reason := strings.TrimSpace(trigger.Reason)
		if reason == "" {
			return "force_exit"
		}
		return reason
	case "TAKE_PROFIT":
		return risk.FormatTPReason(trigger.LevelID)
	default:
		return "stop_loss_hit"
	}
}

func isPartialTakeProfit(trigger risk.RiskTrigger) bool {
	return trigger.Type == "TAKE_PROFIT" && trigger.QtyPct > 0 && trigger.QtyPct < 1
}

func isFinalTakeProfit(trigger risk.RiskTrigger) bool {
	return trigger.Type == "TAKE_PROFIT" && trigger.QtyPct == 1.0
}

func resolvePartialTPBaseQty(plan risk.RiskPlan) float64 {
	if plan.InitialQty > 0 {
		return plan.InitialQty
	}
	return 0
}

func cleanupDustCloseQty(closeQty, limitQty float64) float64 {
	if closeQty <= 0 || limitQty <= 0 || closeQty >= limitQty {
		return closeQty
	}
	dust := math.Max(limitQty*DustThresholdRatio, closeQtyPrecision)
	if limitQty-closeQty <= dust {
		return limitQty
	}
	return closeQty
}

func clipCloseQty(closeQty, limitQty float64) float64 {
	if limitQty > 0 && closeQty > limitQty {
		return limitQty
	}
	return closeQty
}

func floorCloseQty(value float64) float64 {
	if value <= 0 || closeQtyPrecision <= 0 {
		return value
	}
	return math.Floor(value/closeQtyPrecision) * closeQtyPrecision
}

func shouldFetchStatusAmount(pos store.PositionRecord, trigger risk.RiskTrigger) bool {
	if pos.Qty <= 0 {
		return true
	}
	return isPartialTakeProfit(trigger)
}

func clampMinOneFloat(value float64) float64 {
	return math.Max(value, 1)
}
