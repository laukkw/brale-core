package telegrambot

import (
	"context"
	"strings"
)

func (b *Bot) routeCallback(ctx context.Context, chatID, userID int64, data string) {
	switch {
	case data == cbMenuMonitor:
		b.handleMonitorMenu(ctx, chatID)
	case data == cbMenuPositions:
		b.handlePositionsMenu(ctx, chatID)
	case data == cbMenuTrades:
		b.handleTradesMenu(ctx, chatID)
	case data == cbMenuObserve:
		b.handleObserveMenu(ctx, chatID)
	case data == cbObserveManual:
		b.startManualObserveInput(ctx, chatID, userID)
	case data == cbMenuToggle:
		b.sendToggleMenu(ctx, chatID)
	case data == cbToggleOn:
		b.toggleSchedule(ctx, chatID, true)
	case data == cbToggleOff:
		b.toggleSchedule(ctx, chatID, false)
	case data == cbMenuLatest:
		b.handleLatestMenu(ctx, chatID)
	case data == cbMenuCancel:
		b.sessions.delete(chatID, userID)
		b.sendText(ctx, chatID, "已取消当前会话。")
	case strings.HasPrefix(data, cbObservePrefix):
		b.handleObserveCallback(ctx, chatID, userID, strings.TrimPrefix(data, cbObservePrefix))
	case strings.HasPrefix(data, cbLatestPrefix):
		b.handleLatestCallback(ctx, chatID, strings.TrimPrefix(data, cbLatestPrefix))
	default:
		b.sendText(ctx, chatID, "未知操作，请使用菜单按钮。")
	}
}

func (b *Bot) startManualObserveInput(ctx context.Context, chatID, userID int64) {
	sess := &session{ChatID: chatID, UserID: userID, Step: stepAwaitSymbol}
	b.sessions.save(sess)
	b.sendTextWithReply(ctx, chatID, "请输入币种（如 ETH 或 ETHUSDT）：")
}

func (b *Bot) handleObserveCallback(ctx context.Context, chatID, userID int64, rawSymbol string) {
	symbol := normalizeSymbol(rawSymbol)
	if symbol == "" {
		b.sendText(ctx, chatID, "币种不能为空，请重新选择。")
		return
	}
	b.sessions.delete(chatID, userID)
	b.sendText(ctx, chatID, "开始执行单轮决策...")
	go b.executeObserveFlat(ctx, chatID, symbol)
}

func (b *Bot) handleLatestCallback(ctx context.Context, chatID int64, rawSymbol string) {
	symbol := normalizeSymbol(rawSymbol)
	if symbol == "" {
		b.sendText(ctx, chatID, "币种不能为空，请重新选择。")
		return
	}
	b.handleDecisionLatest(ctx, chatID, symbol)
}
