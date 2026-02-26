package runtime

import (
	"fmt"
	"time"
)

type SchedulePolicy interface {
	ShouldEnqueueBar(scheduled bool, mode SymbolMode) bool
	ShouldEnqueuePeriodic(scheduled bool, mode SymbolMode, monitored bool) bool
	DescribeSymbolStatus(scheduled bool, mode SymbolMode, monitored bool, now time.Time, interval time.Duration) (string, string, string)
	Summary(scheduled bool, symbolCount int, monitoredCount int) string
}

type defaultSchedulePolicy struct{}

func (defaultSchedulePolicy) ShouldEnqueueBar(scheduled bool, mode SymbolMode) bool {
	if !scheduled {
		return false
	}
	return mode == SymbolModeTrade
}

func (defaultSchedulePolicy) ShouldEnqueuePeriodic(scheduled bool, mode SymbolMode, monitored bool) bool {
	if mode == SymbolModeOff {
		return false
	}
	if scheduled {
		return mode != SymbolModeObserve
	}
	return monitored
}

func (defaultSchedulePolicy) DescribeSymbolStatus(scheduled bool, mode SymbolMode, monitored bool, now time.Time, interval time.Duration) (string, string, string) {
	nextExecution := ""
	waiting := "定时 LLM 已关闭，仅响应 API 请求"
	details := "定时 LLM 已关闭，仅响应 API 请求"
	if mode == SymbolModeOff {
		return nextExecution, "符号已关闭", "符号已关闭"
	}
	if mode == SymbolModeObserve {
		if monitored {
			return nextExecution, "观察模式，仅手动触发", "观察模式，仅手动触发"
		}
		return nextExecution, "观察模式，仅手动触发", "观察模式，仅手动触发"
	}
	if scheduled {
		nextTime := nextBarClose(now, interval)
		wait := nextTime.Sub(now)
		waiting = fmt.Sprintf("约 %v 后执行", wait.Round(time.Minute))
		details = "定时 LLM 已开启"
		nextExecution = nextTime.Format("2006-01-02 15:04")
		return nextExecution, waiting, details
	}
	if monitored {
		return nextExecution, "定时 LLM 已关闭，监控仍在运行", "监控模式：price tick + reconcile"
	}
	return nextExecution, waiting, details
}

func (defaultSchedulePolicy) Summary(scheduled bool, symbolCount int, monitoredCount int) string {
	if scheduled {
		return fmt.Sprintf("定时 LLM 已开启，共 %d 个币种正在监控", symbolCount)
	}
	if monitoredCount > 0 {
		return fmt.Sprintf("定时 LLM 已关闭，%d 个币种保持监控", monitoredCount)
	}
	return "定时 LLM 已关闭，仅响应 API 请求"
}

func (s *RuntimeScheduler) policy() SchedulePolicy {
	if s == nil || s.Policy == nil {
		return defaultSchedulePolicy{}
	}
	return s.Policy
}
