package runtime

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/execution"
	"brale-core/internal/pkg/logging"
	symbolpkg "brale-core/internal/pkg/symbol"
	"brale-core/internal/position"
	"brale-core/internal/transport/notify"
	"brale-core/internal/transport/webhook"

	"go.uber.org/zap"
)

var ErrWebhookQueueFull = errors.New("webhook queue full")

type WebhookSyncService struct {
	Dispatcher  TaskDispatcher
	Queue       chan WebhookEvent
	WorkerCount int
	Now         func() int64
	AllowSymbol func(symbol string) bool
	ExecClient  webhookTradeClient
	Notifier    notify.Notifier
	PosCache    *position.PositionCache

	mu              sync.Mutex
	openNotified    map[int]int64
	lastExitOrderID map[int]exitNotifyState
}

type exitNotifyState struct {
	OrderID string
	At      int64
}

type webhookTradeClient interface {
	execution.OpenTradesReader
	execution.TradeFinder
}

type WebhookEvent struct {
	Type        string
	Symbol      string
	Timestamp   int64
	EnterTag    string
	TradeID     int
	Pair        string
	ExitReason  string
	CloseRate   float64
	Amount      float64
	StakeAmount float64
}

func NewWebhookSyncService(cfg config.WebhookConfig, dispatcher TaskDispatcher) *WebhookSyncService {
	queueSize := cfg.QueueSize
	if queueSize <= 0 {
		queueSize = 1024
	}
	workers := cfg.WorkerCount
	if workers <= 0 {
		workers = 4
	}
	return &WebhookSyncService{
		Dispatcher:      dispatcher,
		Queue:           make(chan WebhookEvent, queueSize),
		WorkerCount:     workers,
		openNotified:    make(map[int]int64),
		lastExitOrderID: make(map[int]exitNotifyState),
	}
}

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

func (s *WebhookSyncService) Start(ctx context.Context) {
	if s == nil {
		return
	}
	for i := 0; i < s.WorkerCount; i++ {
		go s.worker(ctx)
	}
}

func (s *WebhookSyncService) worker(ctx context.Context) {
	if s == nil || s.Dispatcher == nil || s.Queue == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-s.Queue:
			if !ok {
				return
			}
			if err := s.process(ctx, evt); err != nil {
				logging.FromContext(ctx).Named("webhook").Warn("webhook event process failed", zap.Error(err))
			}
		}
	}
}

func (s *WebhookSyncService) enqueue(evt WebhookEvent) error {
	select {
	case s.Queue <- evt:
		return nil
	default:
		return ErrWebhookQueueFull
	}
}

func (s *WebhookSyncService) process(ctx context.Context, evt WebhookEvent) error {
	if s.Dispatcher == nil {
		return fmt.Errorf("webhook dispatcher is required")
	}
	if strings.TrimSpace(evt.Symbol) == "" {
		return fmt.Errorf("symbol is required")
	}
	s.notify(ctx, evt)
	if err := s.Dispatcher.Enqueue(RuntimeTask{Type: TaskWebhookEvent, Symbol: evt.Symbol}); err != nil {
		return err
	}
	return nil
}

func (s *WebhookSyncService) notify(ctx context.Context, evt WebhookEvent) {
	if s == nil || s.Notifier == nil || s.ExecClient == nil {
		return
	}
	tradeID := evt.TradeID
	if tradeID <= 0 {
		return
	}
	logger := logging.FromContext(ctx).Named("webhook")
	switch evt.Type {
	case "entry_fill":
		s.notifyEntry(ctx, logger, tradeID)
	case "exit_fill":
		s.notifyExit(ctx, logger, tradeID, evt)
	}
}

func (s *WebhookSyncService) notifyEntry(ctx context.Context, logger *zap.Logger, tradeID int) {
	if s.ExecClient == nil || s.Notifier == nil {
		return
	}
	if s.isOpenNotified(tradeID) {
		return
	}
	trade, ok, err := s.findOpenTrade(ctx, tradeID)
	if err != nil {
		logger.Warn("webhook entry trade lookup failed", zap.Error(err), zap.Int("trade_id", tradeID))
		return
	}
	if !ok {
		logger.Warn("webhook entry trade not found", zap.Int("trade_id", tradeID))
		return
	}
	notice := notify.TradeOpenNotice{
		TradeID:       trade.ID,
		Pair:          trade.Pair,
		Amount:        float64(trade.Amount),
		StakeAmount:   float64(trade.StakeAmount),
		IsShort:       trade.IsShort,
		OpenRate:      float64(trade.OpenRate),
		OpenTimestamp: int64(trade.OpenFillTimestamp),
		Leverage:      float64(trade.Leverage),
		EnterTag:      string(trade.EnterTag),
	}
	logger.Info("trade open notify",
		zap.Int("trade_id", notice.TradeID),
		zap.String("pair", notice.Pair),
		zap.Float64("amount", notice.Amount),
		zap.Float64("stake_amount", notice.StakeAmount),
		zap.Bool("is_short", notice.IsShort),
		zap.Float64("open_rate", notice.OpenRate),
		zap.Float64("leverage", notice.Leverage),
	)
	if err := s.Notifier.SendTradeOpen(ctx, notice); err != nil {
		logger.Warn("webhook entry notify failed", zap.Error(err), zap.Int("trade_id", tradeID))
		return
	}
	s.markOpenNotified(tradeID)
}

func (s *WebhookSyncService) notifyExit(ctx context.Context, logger *zap.Logger, tradeID int, evt WebhookEvent) {
	if s.ExecClient == nil || s.Notifier == nil {
		return
	}
	trade, ok, err := s.ExecClient.FindTradeByID(ctx, tradeID)
	if err != nil {
		logger.Warn("webhook exit trade lookup failed", zap.Error(err), zap.Int("trade_id", tradeID))
		return
	}
	if !ok {
		logger.Warn("webhook exit trade not found", zap.Int("trade_id", tradeID))
		return
	}
	exitOrder, hasExitOrder := latestExitOrder(trade)
	if hasExitOrder {
		orderID := strings.TrimSpace(exitOrder.OrderID)
		if orderID != "" && s.isExitNotified(tradeID, orderID) {
			return
		}
		s.markExitNotified(tradeID, orderID)
	}
	internalReason := ""
	if s.PosCache != nil {
		if val, ok := s.PosCache.GetCloseReason(strconv.Itoa(tradeID)); ok {
			internalReason = val
		}
	}
	exitReason := firstNonEmpty(string(trade.ExitReason), evt.ExitReason)
	exitType := resolveExitType(internalReason, exitReason)
	if trade.IsOpen {
		closeRate := resolveCloseRate(trade, exitOrder, evt)
		amount := resolveExitAmount(exitOrder, evt)
		stake := resolveExitStake(exitOrder, trade)
		notice := notify.TradePartialCloseNotice{
			TradeID:             trade.ID,
			Pair:                trade.Pair,
			IsShort:             trade.IsShort,
			OpenRate:            float64(trade.OpenRate),
			CloseRate:           closeRate,
			Amount:              amount,
			StakeAmount:         stake,
			RealizedProfit:      float64(trade.RealizedProfit),
			RealizedProfitRatio: float64(trade.RealizedProfitRatio),
			ExitReason:          exitReason,
			ExitType:            exitType,
		}
		logger.Info("trade partial close notify",
			zap.Int("trade_id", notice.TradeID),
			zap.String("pair", notice.Pair),
			zap.Float64("amount", notice.Amount),
			zap.Float64("stake_amount", notice.StakeAmount),
			zap.Bool("is_short", notice.IsShort),
			zap.Float64("open_rate", notice.OpenRate),
			zap.Float64("close_rate", notice.CloseRate),
			zap.Float64("realized_profit", notice.RealizedProfit),
			zap.Float64("realized_profit_ratio", notice.RealizedProfitRatio),
			zap.String("exit_reason", notice.ExitReason),
			zap.String("exit_type", notice.ExitType),
		)
		if err := s.Notifier.SendTradePartialClose(ctx, notice); err != nil {
			logger.Warn("webhook partial close notify failed", zap.Error(err), zap.Int("trade_id", tradeID))
		}
		return
	}
	closeRate := float64(trade.CloseRate)
	if closeRate <= 0 {
		closeRate = resolveCloseRate(trade, exitOrder, evt)
	}
	notice := notify.TradeCloseSummaryNotice{
		TradeID:        trade.ID,
		Pair:           trade.Pair,
		IsShort:        trade.IsShort,
		OpenRate:       float64(trade.OpenRate),
		CloseRate:      closeRate,
		Amount:         float64(trade.Amount),
		StakeAmount:    float64(trade.StakeAmount),
		CloseProfitAbs: float64(trade.CloseProfitAbs),
		CloseProfitPct: float64(trade.CloseProfitPct),
		ProfitAbs:      float64(trade.ProfitAbs),
		ProfitPct:      float64(trade.ProfitPct),
		TradeDuration:  trade.TradeDuration,
		TradeDurationS: int64(trade.TradeDurationSeconds),
		ExitReason:     exitReason,
		ExitType:       exitType,
		Leverage:       float64(trade.Leverage),
	}
	logger.Info("trade close summary notify",
		zap.Int("trade_id", notice.TradeID),
		zap.String("pair", notice.Pair),
		zap.Float64("amount", notice.Amount),
		zap.Float64("stake_amount", notice.StakeAmount),
		zap.Bool("is_short", notice.IsShort),
		zap.Float64("open_rate", notice.OpenRate),
		zap.Float64("close_rate", notice.CloseRate),
		zap.Float64("close_profit_abs", notice.CloseProfitAbs),
		zap.Float64("close_profit_pct", notice.CloseProfitPct),
		zap.Float64("profit_abs", notice.ProfitAbs),
		zap.Float64("profit_pct", notice.ProfitPct),
		zap.String("exit_reason", notice.ExitReason),
		zap.String("exit_type", notice.ExitType),
	)
	if err := s.Notifier.SendTradeCloseSummary(ctx, notice); err != nil {
		logger.Warn("webhook close summary notify failed", zap.Error(err), zap.Int("trade_id", tradeID))
	}
}

func (s *WebhookSyncService) findOpenTrade(ctx context.Context, tradeID int) (execution.Trade, bool, error) {
	trades, err := s.ExecClient.ListOpenTrades(ctx)
	if err != nil {
		return execution.Trade{}, false, err
	}
	for _, tr := range trades {
		if tr.ID == tradeID {
			return tr, true, nil
		}
	}
	return execution.Trade{}, false, nil
}

func (s *WebhookSyncService) isOpenNotified(tradeID int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupNotifyCachesLocked(s.now())
	_, ok := s.openNotified[tradeID]
	return ok
}

func (s *WebhookSyncService) markOpenNotified(tradeID int) {
	s.mu.Lock()
	now := s.now()
	s.cleanupNotifyCachesLocked(now)
	s.openNotified[tradeID] = now
	s.mu.Unlock()
}

func (s *WebhookSyncService) isExitNotified(tradeID int, orderID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupNotifyCachesLocked(s.now())
	if orderID == "" {
		return false
	}
	state, ok := s.lastExitOrderID[tradeID]
	return ok && state.OrderID == orderID
}

func (s *WebhookSyncService) markExitNotified(tradeID int, orderID string) {
	if orderID == "" {
		return
	}
	s.mu.Lock()
	now := s.now()
	s.cleanupNotifyCachesLocked(now)
	s.lastExitOrderID[tradeID] = exitNotifyState{OrderID: orderID, At: now}
	s.mu.Unlock()
}

const webhookNotifyCacheTTL = int64((24 * time.Hour) / time.Millisecond)

func (s *WebhookSyncService) cleanupNotifyCachesLocked(now int64) {
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	if len(s.openNotified) > 0 {
		for tradeID, at := range s.openNotified {
			if at <= 0 || now-at >= webhookNotifyCacheTTL {
				delete(s.openNotified, tradeID)
			}
		}
	}
	if len(s.lastExitOrderID) > 0 {
		for tradeID, state := range s.lastExitOrderID {
			if state.At <= 0 || now-state.At >= webhookNotifyCacheTTL {
				delete(s.lastExitOrderID, tradeID)
			}
		}
	}
}

func latestExitOrder(trade execution.Trade) (execution.TradeOrder, bool) {
	var chosen execution.TradeOrder
	var found bool
	var latest int64
	for _, ord := range trade.Orders {
		if ord.FTIsEntry {
			continue
		}
		filledAt := int64(ord.OrderFilledAt)
		if filledAt <= 0 {
			filledAt = int64(ord.OrderTimestamp)
		}
		if !found || filledAt > latest {
			chosen = ord
			latest = filledAt
			found = true
		}
	}
	return chosen, found
}

func resolveCloseRate(trade execution.Trade, ord execution.TradeOrder, evt WebhookEvent) float64 {
	if ord.Average > 0 {
		return float64(ord.Average)
	}
	if ord.SafePrice > 0 {
		return float64(ord.SafePrice)
	}
	if trade.CloseRate > 0 {
		return float64(trade.CloseRate)
	}
	if evt.CloseRate > 0 {
		return evt.CloseRate
	}
	return 0
}

func resolveExitAmount(ord execution.TradeOrder, evt WebhookEvent) float64 {
	if ord.Filled > 0 {
		return float64(ord.Filled)
	}
	if ord.Amount > 0 {
		return float64(ord.Amount)
	}
	if evt.Amount > 0 {
		return evt.Amount
	}
	return 0
}

func resolveExitStake(ord execution.TradeOrder, trade execution.Trade) float64 {
	if ord.OrderCost > 0 {
		return float64(ord.OrderCost)
	}
	if trade.StakeAmount > 0 {
		return float64(trade.StakeAmount)
	}
	return 0
}

func resolveExitType(internalReason string, exitReason string) string {
	internal := strings.ToLower(strings.TrimSpace(internalReason))
	if strings.Contains(internal, "stop_loss_hit") {
		return "stop_loss"
	}
	if strings.Contains(internal, "take_profit_hit") {
		return "take_profit"
	}
	reason := strings.ToLower(strings.TrimSpace(exitReason))
	if strings.Contains(reason, "stop") {
		return "stop_loss"
	}
	if strings.Contains(reason, "roi") || strings.Contains(reason, "profit") || strings.Contains(reason, "take") || strings.Contains(reason, "tp") {
		return "take_profit"
	}
	return "external"
}

func firstNonEmpty(values ...string) string {
	for _, val := range values {
		if strings.TrimSpace(val) != "" {
			return val
		}
	}
	return ""
}

func isKnownEvent(evtType string) bool {
	switch strings.ToLower(strings.TrimSpace(evtType)) {
	case "entry", "exit", "entry_fill", "exit_fill":
		return true
	default:
		return false
	}
}

func normalizeSymbol(raw string) string {
	return symbolpkg.Normalize(raw)
}

func (s *WebhookSyncService) now() int64 {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UnixMilli()
}
