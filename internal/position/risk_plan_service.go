// 本文件主要内容：风险计划写入与历史记录。
package position

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"brale-core/internal/execution"
	"brale-core/internal/risk"
	"brale-core/internal/store"
)

type RiskPlanService struct {
	Store store.Store
}

func (s *RiskPlanService) InitFromPlan(ctx context.Context, positionID string, plan execution.ExecutionPlan, source string) (store.RiskPlanHistoryRecord, error) {
	payload := risk.BuildRiskPlan(risk.RiskPlanInput{
		Entry:            plan.Entry,
		StopLoss:         plan.StopLoss,
		PositionSize:     plan.PositionSize,
		TakeProfits:      plan.TakeProfits,
		TakeProfitRatios: plan.TakeProfitRatios,
	})
	return s.ApplyUpdate(ctx, positionID, payload, source)
}

func (s *RiskPlanService) ApplyUpdate(ctx context.Context, positionID string, payload risk.RiskPlan, source string) (store.RiskPlanHistoryRecord, error) {
	if s.Store == nil {
		return store.RiskPlanHistoryRecord{}, fmt.Errorf("store is required")
	}
	if positionID == "" {
		return store.RiskPlanHistoryRecord{}, fmt.Errorf("position_id is required")
	}
	pos, ok, err := s.Store.FindPositionByID(ctx, positionID)
	if err != nil || !ok {
		return store.RiskPlanHistoryRecord{}, err
	}
	raw, err := encodeRiskPlan(payload)
	if err != nil {
		return store.RiskPlanHistoryRecord{}, err
	}
	updates := map[string]any{
		"risk_json": raw,
		"version":   pos.Version + 1,
	}
	if _, err := s.Store.UpdatePosition(ctx, pos.PositionID, pos.Version, updates); err != nil {
		return store.RiskPlanHistoryRecord{}, err
	}
	return s.saveHistory(ctx, positionID, raw, source)
}

func (s *RiskPlanService) SaveHistory(ctx context.Context, positionID string, payload risk.RiskPlan, source string) (store.RiskPlanHistoryRecord, error) {
	if s.Store == nil {
		return store.RiskPlanHistoryRecord{}, fmt.Errorf("store is required")
	}
	if positionID == "" {
		return store.RiskPlanHistoryRecord{}, fmt.Errorf("position_id is required")
	}
	raw, err := encodeRiskPlan(payload)
	if err != nil {
		return store.RiskPlanHistoryRecord{}, err
	}
	return s.saveHistory(ctx, positionID, raw, source)
}

func (s *RiskPlanService) saveHistory(ctx context.Context, positionID string, raw []byte, source string) (store.RiskPlanHistoryRecord, error) {
	latest, ok, err := s.Store.FindLatestRiskPlanHistory(ctx, positionID)
	if err != nil {
		return store.RiskPlanHistoryRecord{}, err
	}
	version := 1
	if ok && latest.Version > 0 {
		version = latest.Version + 1
	}
	rec := store.RiskPlanHistoryRecord{
		PositionID:  positionID,
		Version:     version,
		Source:      source,
		PayloadJSON: raw,
		CreatedAt:   time.Now().UTC(),
	}
	if err := s.Store.SaveRiskPlanHistory(ctx, &rec); err != nil {
		return store.RiskPlanHistoryRecord{}, err
	}
	return rec, nil
}

func encodeRiskPlan(payload risk.RiskPlan) ([]byte, error) {
	compact := risk.CompactRiskPlan(payload)
	raw, err := json.Marshal(compact)
	if err != nil {
		return nil, err
	}
	return raw, nil
}
