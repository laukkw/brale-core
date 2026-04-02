package ruleflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"brale-core/internal/decision/decisionutil"
	"brale-core/internal/decision/features"
	binancemarket "brale-core/internal/market/binance"
	"brale-core/internal/pkg/logging"

	"go.uber.org/zap"
)

func buildInputPayload(ctx context.Context, input Input) (string, error) {
	timestamp := time.Now().UTC().Format(time.RFC3339)
	indicator, indicatorErr := decisionutil.PickIndicator(input.Compression, input.Symbol)
	preferredTrendInterval := strings.TrimSpace(input.Binding.RiskManagement.InitialExit.StructureInterval)
	trend, trendErr := loadTrend(input.Compression, input.Symbol, preferredTrendInterval)
	riskPerTrade := input.Binding.RiskManagement.RiskPerTradePct
	if input.Risk.RiskPerTradePct > 0 {
		riskPerTrade = input.Risk.RiskPerTradePct
	}
	sieveRows := make([]any, 0, len(input.Binding.RiskManagement.Sieve.Rows))
	for _, row := range input.Binding.RiskManagement.Sieve.Rows {
		rowMap := map[string]any{
			"mechanics_tag":  row.MechanicsTag,
			"liq_confidence": row.LiqConfidence,
			"gate_action":    row.GateAction,
			"size_factor":    row.SizeFactor,
			"reason_code":    row.ReasonCode,
		}
		if row.CrowdingAlign != nil {
			rowMap["crowding_align"] = *row.CrowdingAlign
		}
		sieveRows = append(sieveRows, rowMap)
	}
	payload := map[string]any{
		"symbol":             input.Symbol,
		"timestamp":          timestamp,
		"state":              string(input.State),
		"position_id":        input.PositionID,
		"exit_confirm_count": input.ExitConfirmCount,
		"build_plan":         input.BuildPlan,
		"providers_enabled": map[string]any{
			"indicator": input.Providers.Enabled.Indicator,
			"structure": input.Providers.Enabled.Structure,
			"mechanics": input.Providers.Enabled.Mechanics,
		},
		"providers": map[string]any{
			"indicator": input.Providers.Indicator,
			"structure": input.Providers.Structure,
			"mechanics": input.Providers.Mechanics,
		},
		"structure_direction": input.StructureDirection,
		"consensus": map[string]any{
			"score":                input.ConsensusScore,
			"confidence":           input.ConsensusConfidence,
			"agreement":            input.ConsensusAgreement,
			"resonance_bonus":      input.ConsensusResonance,
			"resonance_active":     input.ConsensusResonant,
			"score_threshold":      input.ScoreThreshold,
			"confidence_threshold": input.ConfidenceThreshold,
		},
		"account": map[string]any{
			"equity":    input.Account.Equity,
			"available": input.Account.Available,
			"currency":  input.Account.Currency,
		},
		"risk": map[string]any{
			"risk_per_trade_pct": riskPerTrade,
		},
		"risk_management": map[string]any{
			"risk_per_trade_pct": input.Binding.RiskManagement.RiskPerTradePct,
			"risk_strategy_mode": resolvePayloadRiskStrategyMode(input.Binding.RiskManagement.RiskStrategy.Mode),
			"max_invest_pct":     input.Binding.RiskManagement.MaxInvestPct,
			"max_leverage":       input.Binding.RiskManagement.MaxLeverage,
			"grade_1_factor":     input.Binding.RiskManagement.Grade1Factor,
			"grade_2_factor":     input.Binding.RiskManagement.Grade2Factor,
			"grade_3_factor":     input.Binding.RiskManagement.Grade3Factor,
			"entry_offset_atr":   input.Binding.RiskManagement.EntryOffsetATR,
			"entry_mode":         input.Binding.RiskManagement.EntryMode,
			"orderbook_depth":    input.Binding.RiskManagement.OrderbookDepth,
			"breakeven_fee_pct":  input.Binding.RiskManagement.BreakevenFeePct,
			"initial_exit": map[string]any{
				"policy":             input.Binding.RiskManagement.InitialExit.Policy,
				"structure_interval": input.Binding.RiskManagement.InitialExit.StructureInterval,
				"params":             input.Binding.RiskManagement.InitialExit.Params,
			},
			"sieve": map[string]any{
				"min_size_factor":     input.Binding.RiskManagement.Sieve.MinSizeFactor,
				"default_gate_action": input.Binding.RiskManagement.Sieve.DefaultGateAction,
				"default_size_factor": input.Binding.RiskManagement.Sieve.DefaultSizeFactor,
				"rows":                sieveRows,
			},
		},
		"binding": map[string]any{
			"strategy_id":   input.Binding.StrategyID,
			"strategy_hash": input.Binding.StrategyHash,
			"system_hash":   input.Binding.SystemHash,
		},
	}
	if input.Position.Side != "" || input.Position.MarkPriceOK || input.Position.StopLossOK {
		payload["position"] = map[string]any{
			"side":          input.Position.Side,
			"mark_price":    input.Position.MarkPrice,
			"mark_price_ok": input.Position.MarkPriceOK,
			"stop_loss":     input.Position.StopLoss,
			"stop_loss_ok":  input.Position.StopLossOK,
		}
	}

	entryMode := strings.ToLower(strings.TrimSpace(input.Binding.RiskManagement.EntryMode))
	if entryMode == "orderbook" {
		depth := input.Binding.RiskManagement.OrderbookDepth
		if depth <= 0 {
			depth = 5
		}
		market := binancemarket.NewFuturesMarket()
		ob, err := market.GetOrderbook(ctx, input.Symbol, depth)
		if err == nil {
			bids := make([]any, 0, len(ob.Bids))
			for _, lvl := range ob.Bids {
				bids = append(bids, map[string]any{"price": lvl.Price, "quantity": lvl.Quantity})
			}
			asks := make([]any, 0, len(ob.Asks))
			for _, lvl := range ob.Asks {
				asks = append(asks, map[string]any{"price": lvl.Price, "quantity": lvl.Quantity})
			}
			payload["orderbook"] = map[string]any{
				"last_update_id": ob.LastUpdateID,
				"bids":           bids,
				"asks":           asks,
			}
		} else {
			logging.L().Named("ruleflow").Debug("orderbook fetch failed; falling back to atr entry",
				zap.String("symbol", input.Symbol),
				zap.Error(err),
			)
		}
	}

	if indicatorErr == nil {
		payload["indicator"] = indicator
	}
	if trendErr == nil {
		payload["trend"] = trend
	}
	if input.InPosition.Ready {
		payload["in_position"] = map[string]any{
			"indicator": input.InPosition.Indicator,
			"structure": input.InPosition.Structure,
			"mechanics": input.InPosition.Mechanics,
		}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func loadTrend(comp features.CompressionResult, symbol, preferredInterval string) (features.TrendCompressedInput, error) {
	preferred := strings.ToLower(strings.TrimSpace(preferredInterval))
	trendJSON, ok := decisionutil.PickTrendJSON(comp, symbol)
	if preferred != "" && preferred != "auto" {
		if specific, specificOK := decisionutil.PickTrendJSONByInterval(comp, symbol, preferred); specificOK {
			trendJSON = specific
			ok = true
		}
	}
	if !ok {
		return features.TrendCompressedInput{}, fmt.Errorf("trend missing")
	}
	var trend features.TrendCompressedInput
	if err := json.Unmarshal(trendJSON.RawJSON, &trend); err != nil {
		return features.TrendCompressedInput{}, fmt.Errorf("trend json unmarshal failed: %w", err)
	}
	return trend, nil
}

func resolvePayloadRiskStrategyMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "llm":
		return "llm"
	default:
		return "native"
	}
}
