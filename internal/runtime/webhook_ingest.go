package runtime

import (
	"context"
	"fmt"
	"strings"

	"brale-core/internal/transport/webhook"
)

func (s *WebhookSyncService) HandleWebhook(ctx context.Context, payload webhook.FreqtradeWebhookPayload) error {
	if s == nil {
		return fmt.Errorf("webhook service not ready")
	}
	if s.Dispatcher == nil {
		return fmt.Errorf("webhook dispatcher is required")
	}
	evtType := strings.ToLower(strings.TrimSpace(payload.Type))
	if evtType == "" {
		return fmt.Errorf("webhook type is required")
	}
	if !isKnownEvent(evtType) {
		return fmt.Errorf("unsupported webhook type=%s", evtType)
	}
	symbol := normalizeSymbol(payload.Pair)
	if symbol == "" {
		return fmt.Errorf("pair is required")
	}
	if s.AllowSymbol != nil && !s.AllowSymbol(symbol) {
		return nil
	}
	evt := WebhookEvent{
		Type:        evtType,
		Symbol:      symbol,
		Timestamp:   webhook.ParseWebhookTimestamp(payload, s.now),
		EnterTag:    strings.TrimSpace(payload.EnterTag),
		TradeID:     int(payload.TradeID),
		Pair:        strings.TrimSpace(payload.Pair),
		ExitReason:  strings.TrimSpace(payload.ExitReason),
		CloseRate:   float64(payload.CloseRate),
		Amount:      float64(payload.Amount),
		StakeAmount: float64(payload.StakeAmount),
	}
	return s.enqueue(evt)
}

func isKnownEvent(evtType string) bool {
	switch strings.ToLower(strings.TrimSpace(evtType)) {
	case "entry", "exit", "entry_fill", "exit_fill":
		return true
	default:
		return false
	}
}
