package runtimeapi

import (
	"context"
	"math"
	"strings"
	"time"

	"brale-core/internal/execution"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/runtime"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type debugPlanUsecase struct {
	server *Server
}

func newDebugPlanUsecase(s *Server) debugPlanUsecase {
	return debugPlanUsecase{server: s}
}

func (u debugPlanUsecase) inject(ctx context.Context, req debugPlanInjectRequest) (debugPlanInjectResponse, *usecaseError) {
	if u.server == nil {
		return debugPlanInjectResponse{}, &usecaseError{Status: 500, Code: "server_missing", Message: "运行服务未初始化"}
	}
	if u.server.PlanCache == nil {
		return debugPlanInjectResponse{}, &usecaseError{Status: 500, Code: "plan_cache_missing", Message: "PlanCache 未配置"}
	}
	if u.server.PriceSource == nil {
		return debugPlanInjectResponse{}, &usecaseError{Status: 500, Code: "price_source_missing", Message: "PriceSource 未配置"}
	}
	symbol := runtime.NormalizeSymbol(req.Symbol)
	if symbol == "" {
		symbol = "ETHUSDT"
	}
	if u.server.AllowSymbol != nil && !u.server.AllowSymbol(symbol) {
		return debugPlanInjectResponse{}, &usecaseError{Status: 400, Code: "symbol_not_allowed", Message: "symbol 不在允许列表", Details: symbol}
	}
	direction := strings.ToLower(strings.TrimSpace(req.Direction))
	if direction != "long" && direction != "short" {
		return debugPlanInjectResponse{}, &usecaseError{Status: 400, Code: "direction_invalid", Message: "direction 必须为 long/short"}
	}
	if req.RiskPct <= 0 {
		return debugPlanInjectResponse{}, &usecaseError{Status: 400, Code: "risk_pct_invalid", Message: "risk_pct 必须大于 0"}
	}
	if req.LeverageCap < 0 {
		return debugPlanInjectResponse{}, &usecaseError{Status: 400, Code: "leverage_invalid", Message: "leverage_cap 不能为负数"}
	}
	if req.EntryOffsetPct < 0 || req.StopOffsetPct < 0 || req.TP1OffsetPct < 0 {
		return debugPlanInjectResponse{}, &usecaseError{Status: 400, Code: "offset_invalid", Message: "offset_pct 不能为负数"}
	}
	if req.ExpiresSec <= 0 {
		return debugPlanInjectResponse{}, &usecaseError{Status: 400, Code: "expires_invalid", Message: "expires_sec 必须大于 0"}
	}
	quote, err := u.server.PriceSource.MarkPrice(ctx, symbol)
	if err != nil {
		logging.FromContext(ctx).Named("runtime-api").Error("mark price fetch failed", zap.Error(err), zap.String("symbol", symbol))
		return debugPlanInjectResponse{}, &usecaseError{Status: 500, Code: "mark_price_failed", Message: "mark price 获取失败", Details: err.Error()}
	}
	if quote.Price <= 0 {
		return debugPlanInjectResponse{}, &usecaseError{Status: 500, Code: "mark_price_invalid", Message: "mark price 无效", Details: quote.Price}
	}
	var entry float64
	var stop float64
	var tp1 float64
	if direction == "long" {
		entry = quote.Price * (1 + req.EntryOffsetPct)
		stop = entry * (1 - req.StopOffsetPct)
		tp1 = entry * (1 + req.TP1OffsetPct)
	} else {
		entry = quote.Price * (1 - req.EntryOffsetPct)
		stop = entry * (1 + req.StopOffsetPct)
		tp1 = entry * (1 - req.TP1OffsetPct)
	}
	riskDistance := math.Abs(entry - stop)
	if entry <= 0 || stop <= 0 || tp1 <= 0 || riskDistance <= 0 {
		return debugPlanInjectResponse{}, &usecaseError{Status: 400, Code: "plan_invalid", Message: "计算后的价格无效", Details: map[string]float64{"entry": entry, "stop": stop, "tp1": tp1}}
	}
	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(req.ExpiresSec) * time.Second)
	plan := execution.ExecutionPlan{
		Symbol:      symbol,
		Valid:       true,
		Direction:   direction,
		Entry:       entry,
		StopLoss:    stop,
		TakeProfits: []float64{tp1},
		RiskPct:     req.RiskPct,
		Leverage:    req.LeverageCap,
		Template:    "debug_inject",
		PositionID:  uuid.NewString(),
		RiskAnnotations: execution.RiskAnnotations{
			RiskDistance: riskDistance,
		},
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}
	_, _ = u.server.PlanCache.ForceUpsert(symbol, plan)
	return debugPlanInjectResponse{
		Status:       "ok",
		Symbol:       symbol,
		Direction:    direction,
		MarkPrice:    quote.Price,
		Entry:        entry,
		StopLoss:     stop,
		TakeProfits:  []float64{tp1},
		RiskPct:      req.RiskPct,
		RiskDistance: riskDistance,
		PositionID:   plan.PositionID,
		ExpiresAt:    expiresAt,
	}, nil
}

func (u debugPlanUsecase) status(ctx context.Context, symbol string) (debugPlanStatusResponse, *usecaseError) {
	if u.server == nil {
		return debugPlanStatusResponse{}, &usecaseError{Status: 500, Code: "server_missing", Message: "运行服务未初始化"}
	}
	if u.server.PlanCache == nil {
		return debugPlanStatusResponse{}, &usecaseError{Status: 500, Code: "plan_cache_missing", Message: "PlanCache 未配置"}
	}
	symbol = runtime.NormalizeSymbol(symbol)
	if symbol == "" {
		return debugPlanStatusResponse{}, &usecaseError{Status: 400, Code: "symbol_required", Message: "symbol 不能为空"}
	}
	if u.server.AllowSymbol != nil && !u.server.AllowSymbol(symbol) {
		return debugPlanStatusResponse{}, &usecaseError{Status: 400, Code: "symbol_not_allowed", Message: "symbol 不在允许列表", Details: symbol}
	}
	entry, ok := u.server.PlanCache.GetEntry(symbol)
	if !ok || entry == nil {
		return debugPlanStatusResponse{}, &usecaseError{Status: 404, Code: "plan_not_found", Message: "暂无该符号计划"}
	}
	planCopy := entry.Plan
	return debugPlanStatusResponse{
		Status:        "ok",
		Symbol:        symbol,
		Plan:          &planCopy,
		ExternalID:    entry.ExternalID,
		ClientOrderID: entry.ClientOrderID,
		SubmittedAt:   entry.SubmittedAt,
	}, nil
}

func (u debugPlanUsecase) clear(ctx context.Context, symbol string) (debugPlanClearResponse, *usecaseError) {
	if u.server == nil {
		return debugPlanClearResponse{}, &usecaseError{Status: 500, Code: "server_missing", Message: "运行服务未初始化"}
	}
	if u.server.PlanCache == nil {
		return debugPlanClearResponse{}, &usecaseError{Status: 500, Code: "plan_cache_missing", Message: "PlanCache 未配置"}
	}
	symbol = runtime.NormalizeSymbol(symbol)
	if symbol == "" {
		return debugPlanClearResponse{}, &usecaseError{Status: 400, Code: "symbol_required", Message: "symbol 不能为空"}
	}
	if u.server.AllowSymbol != nil && !u.server.AllowSymbol(symbol) {
		return debugPlanClearResponse{}, &usecaseError{Status: 400, Code: "symbol_not_allowed", Message: "symbol 不在允许列表", Details: symbol}
	}
	_, exists := u.server.PlanCache.GetEntry(symbol)
	u.server.PlanCache.Remove(symbol)
	return debugPlanClearResponse{Status: "ok", Symbol: symbol, Cleared: exists}, nil
}
