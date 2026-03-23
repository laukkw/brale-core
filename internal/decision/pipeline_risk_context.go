package decision

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"brale-core/internal/decision/features"
	"brale-core/internal/market"
	"brale-core/internal/pkg/parseutil"
	"brale-core/internal/position"
	"brale-core/internal/risk"
	"brale-core/internal/store"
)

func deriveCurrentPrice(derived map[string]any) float64 {
	if len(derived) == 0 {
		return 0
	}
	value, ok := derived["current_price"]
	if !ok || value == nil {
		return 0
	}
	return parseutil.Float(value)
}

func (p *Pipeline) loadRiskPlanForUpdate(ctx context.Context, symbol string, posID string) (store.PositionRecord, risk.RiskPlan, bool, error) {
	pos, err := p.loadPositionRecord(ctx, symbol, posID)
	if err != nil {
		return store.PositionRecord{}, risk.RiskPlan{}, false, err
	}
	if p.Positioner != nil && p.Positioner.Cache != nil {
		pos = p.Positioner.Cache.HydratePosition(pos)
	}
	if len(pos.RiskJSON) == 0 {
		p.notifyMissingRiskPlan(ctx, pos)
		return store.PositionRecord{}, risk.RiskPlan{}, false, nil
	}
	plan, decodeErr := position.DecodeRiskPlan(pos.RiskJSON)
	if decodeErr != nil {
		return store.PositionRecord{}, risk.RiskPlan{}, false, fmt.Errorf("decode risk plan for position %s: %w", strings.TrimSpace(pos.PositionID), decodeErr)
	}
	return pos, plan, true, nil
}

func (p *Pipeline) isTightenDebounced(ctx context.Context, symbol string, pos store.PositionRecord) (bool, int64, int64) {
	if p == nil {
		return false, 0, 0
	}
	bind, err := p.getBinding(symbol)
	if err != nil {
		return false, 0, 0
	}
	minIntervalSec := bind.RiskManagement.TightenATR.MinUpdateIntervalSec
	if minIntervalSec <= 0 || p.Store == nil || strings.TrimSpace(pos.PositionID) == "" {
		return false, minIntervalSec, 0
	}
	latest, ok, err := p.Store.FindLatestRiskPlanHistory(ctx, pos.PositionID)
	if err != nil || !ok {
		return false, minIntervalSec, 0
	}
	source := strings.ToLower(strings.TrimSpace(latest.Source))
	if source != "monitor-tighten" && source != "monitor-breakeven" {
		return false, minIntervalSec, 0
	}
	if latest.CreatedAt.IsZero() {
		return false, minIntervalSec, 0
	}
	elapsedSec := int64(time.Since(latest.CreatedAt).Seconds())
	remainingSec := minIntervalSec - elapsedSec
	if remainingSec > 0 {
		return true, minIntervalSec, remainingSec
	}
	return false, minIntervalSec, 0
}

func (p *Pipeline) buildTightenContext(ctx context.Context, res SymbolResult, comp features.CompressionResult, exec tightenExecution) (tightenContext, string, error) {
	_, atr, err := pickIndicatorValues(comp, res.Symbol)
	if err != nil || atr <= 0 {
		return tightenContext{}, tightenBlockATRValueMissing, nil
	}
	markPrice := deriveCurrentPrice(res.Gate.Derived)
	if markPrice <= 0 {
		if p.PriceSource == nil {
			return tightenContext{}, tightenBlockPriceSourceMiss, nil
		}
		quote, err := p.PriceSource.MarkPrice(ctx, res.Symbol)
		if err != nil {
			if errors.Is(err, market.ErrPriceUnavailable) {
				return tightenContext{}, tightenBlockPriceUnavailable, nil
			}
			return tightenContext{}, tightenBlockPriceUnavailable, err
		}
		markPrice = quote.Price
		if markPrice <= 0 {
			return tightenContext{}, tightenBlockPriceUnavailable, nil
		}
	}
	bind, err := p.getBinding(res.Symbol)
	if err != nil {
		return tightenContext{}, tightenBlockBindingMissing, err
	}
	return tightenContext{
		Binding:        bind,
		Gate:           res.Gate,
		InPosIndicator: res.InPositionIndicator,
		InPosStructure: res.InPositionStructure,
		InPosMechanics: res.InPositionMechanics,
		MarkPrice:      markPrice,
		ATR:            atr,
		ATRChangePct:   exec.ATRChangePct,
		ATRChangePctOK: exec.ATRChangePctOK,
		GateSatisfied:  exec.GateSatisfied,
		ScoreBreakdown: exec.ScoreBreakdown,
		ScoreTotal:     exec.ScoreTotal,
		ScoreParseOK:   exec.ScoreParseOK,
		CriticalExit: strings.EqualFold(strings.TrimSpace(res.InPositionStructure.MonitorTag), "exit") &&
			strings.EqualFold(strings.TrimSpace(string(res.InPositionStructure.ThreatLevel)), "critical"),
	}, "", nil
}
