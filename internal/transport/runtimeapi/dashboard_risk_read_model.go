package runtimeapi

import (
	"context"
	"fmt"
	"strings"
	"time"

	"brale-core/internal/position"
	"brale-core/internal/store"
)

type dashboardRiskState struct {
	StopLoss    float64
	TakeProfits []float64
	Timeline    []DashboardRiskPlanTimelineItem
}

func loadDashboardRiskState(ctx context.Context, st store.RiskPlanQueryStore, pos store.PositionRecord, timelineLimit int) dashboardRiskState {
	stopLoss, takeProfits, ok := decodeDashboardRiskLevels(pos.RiskJSON)
	if !ok && strings.TrimSpace(pos.PositionID) == "" {
		return dashboardRiskState{}
	}
	return dashboardRiskState{
		StopLoss:    stopLoss,
		TakeProfits: takeProfits,
		Timeline:    loadDashboardRiskPlanTimeline(ctx, st, pos, timelineLimit),
	}
}

func loadDashboardRiskPlanTimeline(ctx context.Context, st store.RiskPlanQueryStore, pos store.PositionRecord, limit int) []DashboardRiskPlanTimelineItem {
	if st == nil || strings.TrimSpace(pos.PositionID) == "" || limit <= 0 {
		return nil
	}
	rows, err := st.ListRiskPlanHistory(ctx, pos.PositionID, limit)
	if err != nil || len(rows) == 0 {
		return nil
	}
	type decodedRiskPlanHistory struct {
		row         store.RiskPlanHistoryRecord
		stopLoss    float64
		takeProfits []float64
	}
	decoded := make([]decodedRiskPlanHistory, 0, len(rows))
	for _, row := range rows {
		stopLoss, takeProfits, ok := decodeDashboardRiskLevels(row.PayloadJSON)
		if !ok {
			continue
		}
		decoded = append(decoded, decodedRiskPlanHistory{row: row, stopLoss: stopLoss, takeProfits: takeProfits})
	}
	items := make([]DashboardRiskPlanTimelineItem, 0, len(decoded))
	for idx, item := range decoded {
		createdAt := ""
		if !item.row.CreatedAt.IsZero() {
			createdAt = item.row.CreatedAt.UTC().Format(time.RFC3339)
		}
		prevStop := item.stopLoss
		prevTPs := append([]float64(nil), item.takeProfits...)
		if idx+1 < len(decoded) {
			prevStop = decoded[idx+1].stopLoss
			prevTPs = append([]float64(nil), decoded[idx+1].takeProfits...)
		}
		items = append(items, DashboardRiskPlanTimelineItem{
			Source:              strings.TrimSpace(item.row.Source),
			Label:               dashboardRiskPlanLabel(strings.TrimSpace(item.row.Source), item.row.Version),
			CreatedAt:           createdAt,
			StopLoss:            item.stopLoss,
			TakeProfits:         append([]float64(nil), item.takeProfits...),
			PreviousStopLoss:    prevStop,
			PreviousTakeProfits: prevTPs,
		})
	}
	return items
}

func dashboardRiskPlanLabel(source string, version int) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "monitor-tighten":
		return "收紧止损"
	case "monitor-breakeven":
		return "推保护本"
	case "entry-fill", "open-fill", "init", "init_from_plan":
		return "初始计划"
	default:
		if version > 0 {
			return fmt.Sprintf("计划 v%d", version)
		}
		if source != "" {
			return source
		}
		return "风险计划"
	}
}

func decodeDashboardRiskLevels(riskJSON []byte) (float64, []float64, bool) {
	if len(riskJSON) == 0 {
		return 0, nil, false
	}
	plan, err := position.DecodeRiskPlan(riskJSON)
	if err != nil {
		return 0, nil, false
	}
	takeProfits := make([]float64, 0, len(plan.TPLevels))
	for _, level := range plan.TPLevels {
		takeProfits = append(takeProfits, level.Price)
	}
	return plan.StopPrice, takeProfits, true
}

func isDashboardTightenSource(source string) bool {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "monitor-tighten", "monitor-breakeven":
		return true
	default:
		return false
	}
}
