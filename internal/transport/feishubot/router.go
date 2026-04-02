package feishubot

import "context"

type commandRoute struct {
	Action string
	Arg    string
}

func (b *Bot) dispatchSessionInput(ctx context.Context, senderID, chatID, commandText string) bool {
	sess, ok := b.sessions.get(senderID)
	if !ok {
		return false
	}
	symbolText := normalizeSymbol(commandText)
	if !isValidSymbol(symbolText) {
		b.sendReply(ctx, chatID, "invalid symbol, please input like BTC or BTCUSDT")
		return true
	}
	b.sessions.delete(senderID)
	switch sess.Step {
	case stepAwaitObserveSymbol:
		b.handleObserve(ctx, chatID, symbolText)
	case stepAwaitLatestSymbol:
		b.handleLatest(ctx, chatID, symbolText)
	}
	return true
}

func (b *Bot) dispatchCommand(ctx context.Context, senderID, chatID string, route commandRoute) {
	switch route.Action {
	case "monitor":
		b.handleMonitor(ctx, chatID)
	case "positions":
		b.handlePositions(ctx, chatID)
	case "trades":
		b.handleTrades(ctx, chatID)
	case "observe":
		b.dispatchObserveCommand(ctx, senderID, chatID, route.Arg)
	case "schedule_on":
		b.handleSchedule(ctx, chatID, true)
	case "schedule_off":
		b.handleSchedule(ctx, chatID, false)
	case "latest":
		b.dispatchLatestCommand(ctx, senderID, chatID, route.Arg)
	default:
		b.sendReply(ctx, chatID, helpText())
	}
}

func (b *Bot) dispatchObserveCommand(ctx context.Context, senderID, chatID, arg string) {
	if arg == "" {
		b.sessions.save(&session{SenderID: senderID, ChatID: chatID, Step: stepAwaitObserveSymbol})
		b.sendReply(ctx, chatID, "please input symbol for observe")
		return
	}
	symbolText := normalizeSymbol(arg)
	if !isValidSymbol(symbolText) {
		b.sendReply(ctx, chatID, "invalid symbol")
		return
	}
	b.handleObserve(ctx, chatID, symbolText)
}

func (b *Bot) dispatchLatestCommand(ctx context.Context, senderID, chatID, arg string) {
	if arg == "" {
		b.sessions.save(&session{SenderID: senderID, ChatID: chatID, Step: stepAwaitLatestSymbol})
		b.sendReply(ctx, chatID, "please input symbol for latest decision")
		return
	}
	symbolText := normalizeSymbol(arg)
	if !isValidSymbol(symbolText) {
		b.sendReply(ctx, chatID, "invalid symbol")
		return
	}
	b.handleLatest(ctx, chatID, symbolText)
}

func parseCommandRoute(input string) commandRoute {
	action, arg := parseCommand(input)
	return commandRoute{Action: action, Arg: arg}
}
