package ruleflow

import (
	"strings"

	"brale-core/internal/config"

	"github.com/rulego/rulego/api/types"
)

type HardGuardNode struct{}

func (n *HardGuardNode) Type() string {
	return "brale/hard_guard"
}

func (n *HardGuardNode) New() types.Node {
	return &HardGuardNode{}
}

func (n *HardGuardNode) Init(types.Config, types.Configuration) error {
	return nil
}

func (n *HardGuardNode) Destroy() {}

func (n *HardGuardNode) OnMsg(ctx types.RuleContext, msg types.RuleMsg) {
	root, err := readRoot(msg)
	if err != nil {
		ctx.TellFailure(msg, err)
		return
	}
	position := toMap(root["position"])
	indicator := toMap(root["indicator"])
	structureDirection := strings.ToLower(toString(toMap(root["structure"])["direction"]))
	side := strings.ToLower(strings.TrimSpace(toString(position["side"])))
	if side == "" {
		side = structureDirection
	}
	mark := toFloat(position["mark_price"])
	markOK := toBool(position["mark_price_ok"])
	if !markOK {
		mark = toFloat(indicator["close"])
		markOK = mark > 0
	}
	stopLoss := toFloat(position["stop_loss"])
	stopLossOK := toBool(position["stop_loss_ok"])
	rsi := toFloat(indicator["rsi"])
	rsiOK := toBool(indicator["rsi_ok"])
	percent5m := toFloat(indicator["pct_change_5m"])
	percent5mOK := toBool(indicator["pct_change_5m_ok"])

	cfg := config.DefaultHardGuardConfig()
	action := monitorActionKeep
	reason := "HARD_GUARD_OK"
	priority := 1
	if hitStopLoss(side, mark, markOK, stopLoss, stopLossOK) {
		action = monitorActionExit
		reason = "STOP_LOSS"
		priority = 10
	} else if hitRSIExtreme(side, rsi, rsiOK, cfg) {
		action = monitorActionExit
		reason = "RSI_EXTREME"
		priority = 9
	} else if hitCircuitBreaker(side, percent5m, percent5mOK, cfg) {
		action = monitorActionExit
		reason = "CIRCUIT_BREAKER"
		priority = 8
	}

	gate := map[string]any{
		"action":    action,
		"reason":    reason,
		"grade":     0,
		"direction": structureDirection,
		"tradeable": false,
		"rule_hit": map[string]any{
			"name":     reason,
			"priority": priority,
			"action":   action,
			"reason":   reason,
			"default":  false,
		},
		"derived": map[string]any{
			"side":              side,
			"mark_price":        mark,
			"mark_price_ok":     markOK,
			"stop_loss":         stopLoss,
			"stop_loss_ok":      stopLossOK,
			"rsi":               rsi,
			"rsi_ok":            rsiOK,
			"pct_change_5m":     percent5m,
			"pct_change_5m_ok":  percent5mOK,
			"long_rsi_extreme":  cfg.LongRSIExtreme,
			"short_rsi_extreme": cfg.ShortRSIExtreme,
			"adverse_5m_pct":    cfg.FiveMinAdverseMovePct,
		},
	}
	root["gate"] = gate
	root["hard_guard"] = gate
	respondRuleMsgJSON(ctx, msg, root)
}

func hitStopLoss(side string, mark float64, markOK bool, stopLoss float64, stopLossOK bool) bool {
	if !markOK || !stopLossOK || mark <= 0 || stopLoss <= 0 {
		return false
	}
	if strings.EqualFold(side, "short") {
		return mark >= stopLoss
	}
	if strings.EqualFold(side, "long") {
		return mark <= stopLoss
	}
	return false
}

func hitRSIExtreme(side string, rsi float64, rsiOK bool, cfg config.HardGuardConfig) bool {
	if !rsiOK {
		return false
	}
	if strings.EqualFold(side, "short") {
		return rsi <= cfg.ShortRSIExtreme
	}
	if strings.EqualFold(side, "long") {
		return rsi >= cfg.LongRSIExtreme
	}
	return false
}

func hitCircuitBreaker(side string, percent5m float64, percent5mOK bool, cfg config.HardGuardConfig) bool {
	if !percent5mOK {
		return false
	}
	if strings.EqualFold(side, "short") {
		return percent5m >= cfg.FiveMinAdverseMovePct
	}
	if strings.EqualFold(side, "long") {
		return percent5m <= -cfg.FiveMinAdverseMovePct
	}
	return false
}
