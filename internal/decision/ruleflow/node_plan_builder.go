package ruleflow

import (
	"context"
	"strings"

	"brale-core/internal/pkg/numutil"
	riskcalc "brale-core/internal/risk"
	"brale-core/internal/risk/initexit"

	"github.com/rulego/rulego/api/types"
)

type PlanBuilderNode struct{}

func (n *PlanBuilderNode) Type() string {
	return "brale/plan_builder"
}

func (n *PlanBuilderNode) New() types.Node {
	return &PlanBuilderNode{}
}

func (n *PlanBuilderNode) Init(types.Config, types.Configuration) error {
	return nil
}

func (n *PlanBuilderNode) Destroy() {}

func (n *PlanBuilderNode) OnMsg(ctx types.RuleContext, msg types.RuleMsg) {
	root, err := readRoot(msg)
	if err != nil {
		ctx.TellFailure(msg, err)
		return
	}
	buildPlan := toBool(root["build_plan"])
	gate := toMap(root["gate"])
	action := strings.ToUpper(toString(gate["action"]))
	if !buildPlan || action != "ALLOW" {
		respondPlanInvalid(ctx, msg, root, "PLAN_CONDITION_FAIL")
		return
	}

	indicator := toMap(root["indicator"])
	binding := toMap(root["binding"])
	risk := toMap(root["risk"])
	riskMgmt := toMap(root["risk_management"])
	trend := toMap(root["trend"])

	direction := strings.ToLower(toString(gate["direction"]))
	closePrice := toFloat(indicator["close"])
	atr := toFloat(indicator["atr"])
	prevHigh, prevLow := resolvePrevHighLow(trend)
	entryOffsetATR := toFloat(riskMgmt["entry_offset_atr"])
	entryMode := strings.ToLower(strings.TrimSpace(toString(riskMgmt["entry_mode"])))
	orderbookDepth := resolveOrderbookDepth(toInt(riskMgmt["orderbook_depth"]))
	entry := resolveEntryPrice(direction, closePrice, atr, prevHigh, prevLow, entryOffsetATR, entryMode, toMap(root["orderbook"]), orderbookDepth)
	policyName, params := resolveInitialExitConfig(riskMgmt)
	initialOut, err := initexit.BuildInitial(context.Background(), policyName, initexit.BuildInput{
		Symbol:    toString(root["symbol"]),
		Direction: direction,
		Entry:     entry,
		ATR:       atr,
		Trend: initexit.TrendInput{
			StructureCandidates: toStructureCandidates(trend),
		},
		Params: params,
	})
	if err != nil {
		respondPlanInvalid(ctx, msg, root, "PLAN_INITIAL_EXIT_INVALID")
		return
	}
	stop := initialOut.StopLoss
	takeProfits := initialOut.TakeProfits
	takeProfitRatios := initialOut.TakeProfitRatios
	stopDist := numutil.AbsFloat(entry - stop)
	grade := toInt(gate["grade"])
	derived := toMap(gate["derived"])
	sieveSizeFactor := resolveSieveSizeFactor(derived)
	effectiveRiskPct := resolveEffectiveRiskPct(riskMgmt, risk, grade, sieveSizeFactor)
	positionSize := 0.0
	account := toMap(root["account"])
	equity := toFloat(account["equity"])
	available := toFloat(account["available"])
	baseBalance := resolveBalance(equity, available)
	riskAmount := baseBalance * effectiveRiskPct
	entryMultiplier := resolveEntryMultiplier(direction, toMap(root["news_overlay"]))
	if entryMultiplier > 0 {
		riskAmount = riskAmount * entryMultiplier
	}
	if stopDist > 0 && riskAmount > 0 {
		positionSize = riskAmount / stopDist
	}
	maxInvestPct := toFloat(riskMgmt["max_invest_pct"])
	maxInvestAmt := resolveMaxInvestAmount(equity, available, maxInvestPct)
	maxLeverage := toFloat(riskMgmt["max_leverage"])
	leverageResult := riskcalc.ResolveLeverageAndLiquidation(entry, positionSize, maxInvestAmt, maxLeverage, direction)
	positionSize = leverageResult.PositionSize
	leverage := leverageResult.Leverage
	liqPrice := leverageResult.LiquidationPrice
	mmr := leverageResult.MMR
	fee := leverageResult.Fee
	if entry <= 0 || stopDist <= 0 || positionSize <= 0 {
		respondPlanInvalid(ctx, msg, root, "PLAN_INPUT_INVALID")
		return
	}

	if riskcalc.IsStopBeyondLiquidation(direction, stop, liqPrice) {
		respondPlanInvalid(ctx, msg, root, "PLAN_STOP_BEYOND_LIQUIDATION")
		return
	}

	plan := map[string]any{
		"valid":                true,
		"invalid_reason":       "",
		"direction":            direction,
		"entry":                entry,
		"stop_loss":            stop,
		"risk_pct":             effectiveRiskPct,
		"position_size":        positionSize,
		"leverage":             leverage,
		"r_multiple":           1.0,
		"template":             "rulego_plan",
		"take_profits":         takeProfits,
		"take_profit_ratios":   takeProfitRatios,
		"position_id":          toString(root["position_id"]),
		"strategy_id":          toString(binding["strategy_id"]),
		"system_config_hash":   toString(binding["system_hash"]),
		"strategy_config_hash": toString(binding["strategy_hash"]),
		"symbol":               toString(root["symbol"]),
		"risk_annotations": map[string]any{
			"stop_source":       initialOut.StopSource,
			"stop_reason":       initialOut.StopReason,
			"risk_distance":     stopDist,
			"atr":               atr,
			"buffer_atr":        0.0,
			"sieve_size_factor": sieveSizeFactor,
			"sieve_reason":      toString(derived["sieve_reason"]),
			"sieve_action":      toString(derived["sieve_action"]),
			"sieve_policy_hash": toString(derived["sieve_policy_hash"]),
			"sieve_hit":         toBool(derived["sieve_hit"]),
			"max_invest_pct":    maxInvestPct,
			"max_invest_amt":    maxInvestAmt,
			"max_leverage":      maxLeverage,
			"liquidation_price": liqPrice,
			"mmr":               mmr,
			"fee":               fee,
			"entry_multiplier":  entryMultiplier,
		},
	}
	root["plan"] = plan
	respondRuleMsgJSON(ctx, msg, root)
}

func resolveEntryMultiplier(direction string, newsOverlay map[string]any) float64 {
	if len(newsOverlay) == 0 {
		return 1.0
	}
	switch strings.ToLower(strings.TrimSpace(direction)) {
	case "short":
		value := toFloat(newsOverlay["entry_multiplier_short"])
		if value <= 0 {
			return 1.0
		}
		return value
	default:
		value := toFloat(newsOverlay["entry_multiplier_long"])
		if value <= 0 {
			return 1.0
		}
		return value
	}
}

func respondPlanInvalid(ctx types.RuleContext, msg types.RuleMsg, root map[string]any, reason string) {
	root["plan"] = map[string]any{
		"valid":          false,
		"invalid_reason": reason,
	}
	respondRuleMsgJSON(ctx, msg, root)
}

func resolveOrderbookDepth(depth int) int {
	if depth <= 0 {
		return 5
	}
	return depth
}

func resolveEntryPrice(direction string, closePrice, atr, prevHigh, prevLow, entryOffsetATR float64, entryMode string, orderbook map[string]any, orderbookDepth int) float64 {
	atrEntry := func() float64 {
		entry := closePrice
		switch direction {
		case "long":
			base := closePrice
			if prevLow > 0 {
				base = numutil.MinFloat(prevLow, closePrice)
			}
			entry = base - (atr * entryOffsetATR)
		case "short":
			entry = numutil.MaxFloat(prevHigh, closePrice) + (atr * entryOffsetATR)
		}
		return entry
	}
	switch entryMode {
	case "orderbook":
		if price, ok := pickOrderbookEntry(direction, orderbook, orderbookDepth-1); ok {
			return price
		}
		return atrEntry()
	case "market":
		return closePrice
	default:
		return atrEntry()
	}
}

func pickOrderbookEntry(direction string, orderbook map[string]any, depthIndex int) (float64, bool) {
	if depthIndex < 0 {
		return 0, false
	}
	switch direction {
	case "long":
		if bids, ok := orderbook["bids"].([]any); ok && len(bids) > depthIndex {
			price := toFloat(toMap(bids[depthIndex])["price"])
			return price, price > 0
		}
	case "short":
		if asks, ok := orderbook["asks"].([]any); ok && len(asks) > depthIndex {
			price := toFloat(toMap(asks[depthIndex])["price"])
			return price, price > 0
		}
	}
	return 0, false
}

func resolveSieveSizeFactor(derived map[string]any) float64 {
	sieveSizeFactor := toFloat(derived["sieve_size_factor"])
	if sieveSizeFactor <= 0 {
		sieveSizeFactor = 1.0
	}
	sieveMinFactor := toFloat(derived["sieve_min_size_factor"])
	if sieveMinFactor > 0 && sieveSizeFactor < sieveMinFactor {
		sieveSizeFactor = sieveMinFactor
	}
	return sieveSizeFactor
}

func resolveEffectiveRiskPct(riskMgmt map[string]any, risk map[string]any, grade int, sieveSizeFactor float64) float64 {
	riskPerTrade := toFloat(riskMgmt["risk_per_trade_pct"])
	if riskPerTrade <= 0 {
		riskPerTrade = toFloat(risk["risk_per_trade_pct"])
	}
	gradeFactor := resolveGradeFactor(grade, riskMgmt)
	return riskPerTrade * gradeFactor * sieveSizeFactor
}

func resolveBalance(equity, available float64) float64 {
	if available > 0 {
		return available
	}
	return equity
}

func resolveMaxInvestAmount(equity, available, maxInvestPct float64) float64 {
	if maxInvestPct <= 0 {
		maxInvestPct = 1.0
	}
	base := resolveBalance(equity, available)
	maxInvestAmt := base * maxInvestPct
	if maxInvestAmt <= 0 {
		maxInvestAmt = base
	}
	return maxInvestAmt
}

func resolveInitialExitConfig(riskMgmt map[string]any) (string, map[string]any) {
	initialExit := toMap(riskMgmt["initial_exit"])
	policyName := strings.TrimSpace(toString(initialExit["policy"]))
	params := toMap(initialExit["params"])
	if len(params) == 0 {
		params = map[string]any{}
	}
	return policyName, params
}

func toStructureCandidates(trend map[string]any) []initexit.StructureCandidate {
	raw, ok := trend["structure_candidates"].([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make([]initexit.StructureCandidate, 0, len(raw))
	for _, item := range raw {
		m := toMap(item)
		price := toFloat(m["price"])
		if price <= 0 {
			continue
		}
		out = append(out, initexit.StructureCandidate{
			Price: price,
			Type:  toString(m["type"]),
		})
	}
	return out
}

func resolveGradeFactor(grade int, riskMgmt map[string]any) float64 {
	switch grade {
	case 1:
		return toFloat(riskMgmt["grade_1_factor"])
	case 2:
		return toFloat(riskMgmt["grade_2_factor"])
	case 3:
		return toFloat(riskMgmt["grade_3_factor"])
	default:
		return 0
	}
}
