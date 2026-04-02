package runtimeapi

import (
	"context"

	readmodel "brale-core/internal/readmodel/dashboard"
	"brale-core/internal/store"
)

type dashboardRiskState struct {
	StopLoss    float64
	TakeProfits []float64
	Timeline    []DashboardRiskPlanTimelineItem
}

func loadDashboardRiskState(ctx context.Context, st store.RiskPlanQueryStore, pos store.PositionRecord, timelineLimit int) dashboardRiskState {
	rm := readmodel.LoadRiskState(ctx, st, pos, timelineLimit)
	return dashboardRiskState{
		StopLoss:    rm.StopLoss,
		TakeProfits: rm.TakeProfits,
		Timeline:    mapDashboardRiskPlanTimeline(rm.Timeline),
	}
}

func loadDashboardRiskPlanTimeline(ctx context.Context, st store.RiskPlanQueryStore, pos store.PositionRecord, limit int) []DashboardRiskPlanTimelineItem {
	return mapDashboardRiskPlanTimeline(readmodel.LoadRiskPlanTimeline(ctx, st, pos, limit))
}

func dashboardRiskPlanLabel(source string, version int) string {
	return readmodel.RiskPlanLabel(source, version)
}

func decodeDashboardRiskLevels(riskJSON []byte) (float64, []float64, bool) {
	return readmodel.DecodeRiskLevels(riskJSON)
}

func isDashboardTightenSource(source string) bool {
	return readmodel.IsTightenSource(source)
}

func mapDashboardRiskPlanTimeline(items []readmodel.RiskPlanTimelineItem) []DashboardRiskPlanTimelineItem {
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
