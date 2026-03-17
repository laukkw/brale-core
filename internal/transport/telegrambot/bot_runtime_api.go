package telegrambot

import "context"

func (b *Bot) fetchMonitorStatus(ctx context.Context) (MonitorStatusResponse, error) {
	return b.runtimeClient.FetchMonitorStatus(ctx)
}

func (b *Bot) fetchPositionStatus(ctx context.Context) (PositionStatusResponse, error) {
	return b.runtimeClient.FetchPositionStatus(ctx)
}

func (b *Bot) fetchTradeHistory(ctx context.Context) (TradeHistoryResponse, error) {
	return b.runtimeClient.FetchTradeHistory(ctx)
}

func (b *Bot) fetchDecisionLatest(ctx context.Context, symbol string) (DecisionLatestResponse, error) {
	return b.runtimeClient.FetchDecisionLatest(ctx, symbol)
}

func (b *Bot) fetchObserveReport(ctx context.Context, symbol string) (ObserveResponse, error) {
	return b.runtimeClient.FetchObserveReport(ctx, symbol)
}

func (b *Bot) postScheduleToggle(ctx context.Context, enable bool) (ScheduleResponse, error) {
	return b.runtimeClient.PostScheduleToggle(ctx, enable)
}
