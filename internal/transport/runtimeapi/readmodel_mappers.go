package runtimeapi

import (
	dashboard "brale-core/internal/readmodel/dashboard"
	decisionflowrm "brale-core/internal/readmodel/decisionflow"
	portfoliorm "brale-core/internal/readmodel/portfolio"
)

func mapDashboardTightenInfo(info *dashboard.TightenInfo) *DashboardTightenInfo {
	if info == nil {
		return nil
	}
	return &DashboardTightenInfo{Triggered: info.Triggered, Reason: info.Reason}
}

func mapDashboardDecisionHistoryItems(items []decisionflowrm.HistoryItem) []DashboardDecisionHistoryItem {
	if len(items) == 0 {
		return []DashboardDecisionHistoryItem{}
	}
	out := make([]DashboardDecisionHistoryItem, 0, len(items))
	for _, item := range items {
		out = append(out, DashboardDecisionHistoryItem{
			SnapshotID:          item.SnapshotID,
			Action:              item.Action,
			Reason:              item.Reason,
			At:                  item.At,
			ConsensusScore:      item.ConsensusScore,
			ConsensusConfidence: item.ConsensusConfidence,
		})
	}
	return out
}

func mapDashboardDecisionDetail(detail *decisionflowrm.Detail) *DashboardDecisionDetail {
	if detail == nil {
		return nil
	}
	return &DashboardDecisionDetail{
		SnapshotID:                   detail.SnapshotID,
		Action:                       detail.Action,
		Reason:                       detail.Reason,
		Tradeable:                    detail.Tradeable,
		ConsensusScore:               detail.ConsensusScore,
		ConsensusConfidence:          detail.ConsensusConfidence,
		ConsensusScoreThreshold:      detail.ConsensusScoreThreshold,
		ConsensusConfidenceThreshold: detail.ConsensusConfidenceThreshold,
		ConsensusScorePassed:         detail.ConsensusScorePassed,
		ConsensusConfidencePassed:    detail.ConsensusConfidencePassed,
		ConsensusPassed:              detail.ConsensusPassed,
		Providers:                    append([]string(nil), detail.Providers...),
		Agents:                       append([]string(nil), detail.Agents...),
		Tighten:                      mapDashboardDecisionTightenDetail(detail.Tighten),
		PlanContext:                  mapDashboardDecisionPlanContext(detail.PlanContext),
		Plan:                         mapDashboardDecisionPlanSummary(detail.Plan),
		Sieve:                        mapDashboardDecisionSieveDetail(detail.Sieve),
		ReportMarkdown:               detail.ReportMarkdown,
		DecisionViewURL:              detail.DecisionViewURL,
	}
}

func mapDashboardDecisionTightenDetail(detail *dashboard.DecisionTightenDetail) *DashboardDecisionTightenDetail {
	if detail == nil {
		return nil
	}
	return &DashboardDecisionTightenDetail{
		Action:         detail.Action,
		Evaluated:      detail.Evaluated,
		Eligible:       detail.Eligible,
		Executed:       detail.Executed,
		TPTightened:    detail.TPTightened,
		BlockedBy:      append([]string(nil), detail.BlockedBy...),
		Score:          detail.Score,
		ScoreThreshold: detail.ScoreThreshold,
		ScoreParseOK:   detail.ScoreParseOK,
		DisplayReason:  detail.DisplayReason,
	}
}

func mapDashboardDecisionPlanContext(ctx *decisionflowrm.PlanContext) *DashboardDecisionPlanContext {
	if ctx == nil {
		return nil
	}
	return &DashboardDecisionPlanContext{
		RiskPerTradePct: ctx.RiskPerTradePct,
		MaxInvestPct:    ctx.MaxInvestPct,
		MaxLeverage:     ctx.MaxLeverage,
		EntryOffsetATR:  ctx.EntryOffsetATR,
		EntryMode:       ctx.EntryMode,
		PlanSource:      ctx.PlanSource,
		InitialExit:     ctx.InitialExit,
	}
}

func mapDashboardDecisionPlanSummary(plan *decisionflowrm.PlanSummary) *DashboardDecisionPlanSummary {
	if plan == nil {
		return nil
	}
	levels := make([]DashboardDecisionPlanTPLevel, 0, len(plan.TakeProfitLevels))
	for _, level := range plan.TakeProfitLevels {
		levels = append(levels, DashboardDecisionPlanTPLevel{
			LevelID: level.LevelID,
			Price:   level.Price,
			QtyPct:  level.QtyPct,
			Hit:     level.Hit,
		})
	}
	return &DashboardDecisionPlanSummary{
		Status:           plan.Status,
		Direction:        plan.Direction,
		EntryPrice:       plan.EntryPrice,
		StopLoss:         plan.StopLoss,
		TakeProfits:      append([]float64(nil), plan.TakeProfits...),
		TakeProfitLevels: levels,
		PositionSize:     plan.PositionSize,
		InitialQty:       plan.InitialQty,
		RiskPct:          plan.RiskPct,
		Leverage:         plan.Leverage,
		OpenedAt:         plan.OpenedAt,
	}
}

func mapDashboardDecisionSieveDetail(detail *decisionflowrm.SieveDetail) *DashboardDecisionSieveDetail {
	if detail == nil {
		return nil
	}
	rows := make([]DashboardDecisionSieveRow, 0, len(detail.Rows))
	for _, row := range detail.Rows {
		rows = append(rows, DashboardDecisionSieveRow{
			MechanicsTag:  row.MechanicsTag,
			LiqConfidence: row.LiqConfidence,
			CrowdingAlign: row.CrowdingAlign,
			GateAction:    row.GateAction,
			SizeFactor:    row.SizeFactor,
			ReasonCode:    row.ReasonCode,
			Matched:       row.Matched,
		})
	}
	return &DashboardDecisionSieveDetail{
		Action:            detail.Action,
		ReasonCode:        detail.ReasonCode,
		Hit:               detail.Hit,
		SizeFactor:        detail.SizeFactor,
		MinSizeFactor:     detail.MinSizeFactor,
		DefaultAction:     detail.DefaultAction,
		DefaultSizeFactor: detail.DefaultSizeFactor,
		ActionBefore:      detail.ActionBefore,
		PolicyHash:        detail.PolicyHash,
		Rows:              rows,
	}
}

func mapPositionStatusItems(items []portfoliorm.PositionStatusItem) []PositionStatusItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]PositionStatusItem, 0, len(items))
	for _, item := range items {
		out = append(out, PositionStatusItem{
			Symbol:           item.Symbol,
			Amount:           item.Amount,
			AmountRequested:  item.AmountRequested,
			MarginAmount:     item.MarginAmount,
			EntryPrice:       item.EntryPrice,
			CurrentPrice:     item.CurrentPrice,
			Side:             item.Side,
			ProfitTotal:      item.ProfitTotal,
			ProfitRealized:   item.ProfitRealized,
			ProfitUnrealized: item.ProfitUnrealized,
			OpenedAt:         item.OpenedAt,
			DurationMin:      item.DurationMin,
			DurationSec:      item.DurationSec,
			TakeProfits:      append([]float64(nil), item.TakeProfits...),
			StopLoss:         item.StopLoss,
		})
	}
	return out
}

func mapDashboardOverviewSymbols(items []portfoliorm.OverviewSymbol) []DashboardOverviewSymbol {
	if len(items) == 0 {
		return []DashboardOverviewSymbol{}
	}
	out := make([]DashboardOverviewSymbol, 0, len(items))
	for _, item := range items {
		out = append(out, DashboardOverviewSymbol{
			Symbol: item.Symbol,
			Position: DashboardPositionCard{
				Side:             item.Position.Side,
				Amount:           item.Position.Amount,
				Leverage:         item.Position.Leverage,
				EntryPrice:       item.Position.EntryPrice,
				CurrentPrice:     item.Position.CurrentPrice,
				TakeProfits:      append([]float64(nil), item.Position.TakeProfits...),
				StopLoss:         item.Position.StopLoss,
				RiskPlanTimeline: mapDashboardRiskPlanTimelineItems(item.Position.RiskPlanTimeline),
			},
			PnL: DashboardPnLCard{
				Realized:   item.PnL.Realized,
				Unrealized: item.PnL.Unrealized,
				Total:      item.PnL.Total,
			},
			Reconciliation: DashboardReconciliation{
				Status:         item.Reconciliation.Status,
				DriftAbs:       item.Reconciliation.DriftAbs,
				DriftPct:       item.Reconciliation.DriftPct,
				DriftThreshold: item.Reconciliation.DriftThreshold,
			},
		})
	}
	return out
}

func mapTradeHistoryItems(items []portfoliorm.TradeHistoryItem) []TradeHistoryItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]TradeHistoryItem, 0, len(items))
	for _, item := range items {
		out = append(out, TradeHistoryItem{
			Symbol:       item.Symbol,
			Side:         item.Side,
			Amount:       item.Amount,
			MarginAmount: item.MarginAmount,
			OpenedAt:     item.OpenedAt,
			ClosedAt:     item.ClosedAt,
			DurationSec:  item.DurationSec,
			Profit:       item.Profit,
			StopLoss:     item.StopLoss,
			TakeProfits:  append([]float64(nil), item.TakeProfits...),
			Timeline:     mapDashboardRiskPlanTimelineItems(item.Timeline),
		})
	}
	return out
}

func mapDashboardRiskPlanTimelineItems(items []dashboard.RiskPlanTimelineItem) []DashboardRiskPlanTimelineItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]DashboardRiskPlanTimelineItem, 0, len(items))
	for _, item := range items {
		out = append(out, DashboardRiskPlanTimelineItem{
			Source:              item.Source,
			Label:               item.Label,
			CreatedAt:           item.CreatedAt,
			StopLoss:            item.StopLoss,
			TakeProfits:         append([]float64(nil), item.TakeProfits...),
			PreviousStopLoss:    item.PreviousStopLoss,
			PreviousTakeProfits: append([]float64(nil), item.PreviousTakeProfits...),
		})
	}
	return out
}
