package telegrambot

import (
	"time"

	"brale-core/internal/transport/botruntime"
)

type Config struct {
	Token          string
	RuntimeBaseURL string
	PollTimeout    time.Duration
	UpdateLimit    int
	SessionTTL     time.Duration
	RequestTimeout time.Duration
	LockPath       string
}

type ObserveRequest = botruntime.ObserveRunRequest
type ObserveResponse = botruntime.ObserveResponse
type MonitorStatusResponse = botruntime.MonitorStatusResponse
type MonitorSymbolConfig = botruntime.MonitorSymbolConfig
type PositionStatusResponse = botruntime.PositionStatusResponse
type PositionStatusItem = botruntime.PositionStatusItem
type TradeHistoryResponse = botruntime.TradeHistoryResponse
type TradeHistoryItem = botruntime.TradeHistoryItem
type DecisionLatestResponse = botruntime.DecisionLatestResponse
type ScheduleResponse = botruntime.ScheduleResponse
type ScheduleNextRun = botruntime.ScheduleNextRun
