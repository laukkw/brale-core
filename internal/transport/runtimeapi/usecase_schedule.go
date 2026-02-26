package runtimeapi

import (
	"context"
	"fmt"
	"strings"

	"brale-core/internal/runtime"

	"go.uber.org/zap"
)

type scheduleUsecase struct {
	scheduler Scheduler
	portfolio portfolioUsecase
}

func newScheduleUsecase(s *Server) scheduleUsecase {
	if s == nil {
		return scheduleUsecase{}
	}
	return scheduleUsecase{scheduler: s.Scheduler, portfolio: newPortfolioUsecase(s)}
}

func (u scheduleUsecase) enable() scheduleResponse {
	u.scheduler.SetScheduledDecision(true)
	u.scheduler.ClearMonitorSymbols()
	status := u.scheduler.GetScheduleStatus()
	symbols := make([]string, 0, len(status.NextRuns))
	for _, item := range status.NextRuns {
		if item.Symbol != "" {
			symbols = append(symbols, item.Symbol)
		}
	}
	summary := "定时 LLM 已开启"
	if len(symbols) > 0 {
		summary = fmt.Sprintf("定时 LLM 已开启，监控币种: %s", strings.Join(symbols, ", "))
	}
	return scheduleResponse{
		Status:       "ok",
		LLMScheduled: true,
		Mode:         modeFromScheduled(true),
		NextRuns:     status.NextRuns,
		Summary:      summary,
	}
}

func (u scheduleUsecase) disable(ctx context.Context, logger *zap.Logger) scheduleResponse {
	u.scheduler.SetScheduledDecision(false)
	positions, posErr := u.portfolio.buildPositionStatus(ctx)
	monitorSymbols := make([]string, 0, len(positions))
	for _, pos := range positions {
		if pos.Symbol != "" {
			monitorSymbols = append(monitorSymbols, pos.Symbol)
		}
	}
	if len(monitorSymbols) > 0 {
		if err := u.scheduler.SetMonitorSymbols(monitorSymbols); err != nil {
			if logger != nil {
				logger.Warn("set monitor symbols failed", zap.Error(err))
			}
			u.scheduler.ClearMonitorSymbols()
		}
	} else {
		u.scheduler.ClearMonitorSymbols()
	}
	status := u.scheduler.GetScheduleStatus()
	summary := "定时 LLM 已关闭，观察接口仍可用"
	if len(monitorSymbols) > 0 {
		summary = "定时 LLM 已关闭，仍监控持仓止盈止损"
	} else if posErr != nil {
		summary = fmt.Sprintf("定时 LLM 已关闭，持仓查询失败: %s", posErr.Error())
	}
	return scheduleResponse{
		Status:       "ok",
		LLMScheduled: false,
		Mode:         modeFromScheduled(false),
		NextRuns:     status.NextRuns,
		Positions:    positions,
		Summary:      summary,
	}
}

func (u scheduleUsecase) status() scheduleResponse {
	status := u.scheduler.GetScheduleStatus()
	return scheduleResponse{
		Status:       "ok",
		LLMScheduled: status.IsScheduled,
		Mode:         modeFromScheduled(status.IsScheduled),
		NextRuns:     status.NextRuns,
		Summary:      status.Details,
	}
}

func (u scheduleUsecase) setSymbolMode(symbol string, enable bool) (scheduleResponse, error) {
	mode := runtime.SymbolModeTrade
	if !enable {
		mode = runtime.SymbolModeOff
	}
	if err := u.scheduler.SetSymbolMode(symbol, mode); err != nil {
		return scheduleResponse{}, err
	}
	status := u.scheduler.GetScheduleStatus()
	scheduled := u.scheduler.GetScheduledDecision()
	return scheduleResponse{
		Status:       "ok",
		LLMScheduled: scheduled,
		Mode:         modeFromScheduled(scheduled),
		NextRuns:     status.NextRuns,
		Summary:      fmt.Sprintf("%s 已设置为 %s", symbol, mode),
	}, nil
}
