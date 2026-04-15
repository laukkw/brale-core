package position

import (
	"math"
	"strings"

	"brale-core/internal/pkg/errclass"
	"brale-core/internal/store"
)

func resolveCloseIntentKind(positionQty float64, closeQty float64) string {
	if closeQty > 0 && positionQty > 0 && !shouldCloseEntirePosition(closeQty, positionQty) {
		return "REDUCE"
	}
	return "CLOSE"
}

func resolveClosePositionQty(pos store.PositionRecord, positionQty float64) float64 {
	if positionQty > 0 {
		return positionQty
	}
	if pos.Qty > 0 {
		return pos.Qty
	}
	return 0
}

func shouldCloseEntirePosition(closeQty, positionQty float64) bool {
	if closeQty <= 0 || positionQty <= 0 {
		return false
	}
	if closeQty >= positionQty {
		return true
	}
	dust := math.Max(positionQty*DustThresholdRatio, closeQtyPrecision)
	return positionQty-closeQty <= dust
}

func isUniqueConstraint(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "constraint")
}

func shouldResetOpenPlanOrder(err error) bool {
	class := errclass.ClassifyError(err)
	if string(class.Scope) != "execution" {
		return false
	}
	return string(class.Kind) == "validation"
}

func isCloseInFlightStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case PositionCloseArmed, PositionCloseSubmitting, PositionClosePending:
		return true
	default:
		return false
	}
}
