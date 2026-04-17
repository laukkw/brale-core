// 本文件主要内容：实现通知管理器与发送流程。
package notify

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"brale-core/internal/cardimage"
	"brale-core/internal/decision/decisionfmt"
	"brale-core/internal/pkg/logging"

	"go.uber.org/zap"
)

type Manager struct {
	formatter decisionfmt.Formatter
	renderer  DecisionImageRenderer
	senders   []Sender
	dedupe    *dedupeGuard
	closeAgg  *closeNoticeAggregator
}

type DecisionImageRenderer interface {
	RenderDecision(ctx context.Context, input decisionfmt.DecisionInput, report decisionfmt.DecisionReport) (*cardimage.ImageAsset, error)
}

type dedupeGuard struct {
	mu          sync.Mutex
	ttl         time.Duration
	lastCleanup time.Time
	items       map[string]dedupeEntry
}

type dedupeEntry struct {
	at       time.Time
	inFlight bool
}

const defaultNotifyDedupeTTL = 90 * time.Second

func newDedupeGuard(ttl time.Duration) *dedupeGuard {
	if ttl <= 0 {
		ttl = defaultNotifyDedupeTTL
	}
	return &dedupeGuard{ttl: ttl, items: make(map[string]dedupeEntry)}
}

func (d *dedupeGuard) tryAcquire(key string, now time.Time) bool {
	if d == nil {
		return true
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return true
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cleanupLocked(now)
	if entry, ok := d.items[key]; ok {
		if entry.inFlight || now.Sub(entry.at) <= d.ttl {
			return false
		}
	}
	d.items[key] = dedupeEntry{at: now, inFlight: true}
	return true
}

func (d *dedupeGuard) commit(key string, now time.Time) {
	if d == nil {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.items[key] = dedupeEntry{at: now}
	d.cleanupLocked(now)
}

func (d *dedupeGuard) release(key string) {
	if d == nil {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.items, key)
}

func (d *dedupeGuard) cleanupLocked(now time.Time) {
	if d.lastCleanup.IsZero() || now.Sub(d.lastCleanup) >= d.ttl {
		expireBefore := now.Add(-d.ttl)
		for k, entry := range d.items {
			if entry.at.Before(expireBefore) {
				delete(d.items, k)
			}
		}
		d.lastCleanup = now
	}
}

type NopNotifier struct{}

func (NopNotifier) SendGate(ctx context.Context, input decisionfmt.DecisionInput, report decisionfmt.DecisionReport) error {
	return nil
}

func (NopNotifier) SendStartup(ctx context.Context, info StartupInfo) error {
	return nil
}

func (NopNotifier) SendShutdown(ctx context.Context, info ShutdownInfo) error {
	return nil
}

func (NopNotifier) SendPositionOpen(ctx context.Context, notice PositionOpenNotice) error {
	return nil
}

func (NopNotifier) SendPositionClose(ctx context.Context, notice PositionCloseNotice) error {
	return nil
}

func (NopNotifier) SendPositionCloseSummary(ctx context.Context, notice PositionCloseSummaryNotice) error {
	return nil
}

func (NopNotifier) SendRiskPlanUpdate(ctx context.Context, notice RiskPlanUpdateNotice) error {
	return nil
}

func (NopNotifier) SendTradeOpen(ctx context.Context, notice TradeOpenNotice) error {
	return nil
}

func (NopNotifier) SendTradePartialClose(ctx context.Context, notice TradePartialCloseNotice) error {
	return nil
}

func (NopNotifier) SendTradeCloseSummary(ctx context.Context, notice TradeCloseSummaryNotice) error {
	return nil
}

func (NopNotifier) SendError(ctx context.Context, notice ErrorNotice) error {
	return nil
}

func NewManager(cfg NotificationConfig, formatter decisionfmt.Formatter) (Notifier, error) {
	if !cfg.Enabled {
		return NopNotifier{}, nil
	}
	if formatter == nil {
		return nil, fmt.Errorf("formatter is required")
	}
	senders := make([]Sender, 0, 3)
	if cfg.Telegram.Enabled {
		sender, err := NewTelegramSender(cfg.Telegram)
		if err != nil {
			return nil, err
		}
		senders = append(senders, sender)
	}
	if cfg.Feishu.Enabled {
		sender, err := NewFeishuSender(cfg.Feishu)
		if err != nil {
			return nil, err
		}
		senders = append(senders, sender)
	}
	if cfg.Email.Enabled {
		sender, err := NewEmailSender(cfg.Email)
		if err != nil {
			return nil, err
		}
		senders = append(senders, sender)
	}
	if len(senders) == 0 {
		return nil, fmt.Errorf("notification enabled but no outbound sender configured")
	}
	mgr := Manager{
		formatter: formatter,
		renderer:  cardimage.NewOGRenderer(),
		senders:   senders,
		dedupe:    newDedupeGuard(defaultNotifyDedupeTTL),
	}
	mgr.closeAgg = newCloseNoticeAggregator(defaultCloseAggregationWindow, mgr.sendAggregatedClose)
	return mgr, nil
}

func NewTestManager(senders ...Sender) Manager {
	mgr := Manager{
		senders: append([]Sender(nil), senders...),
		dedupe:  newDedupeGuard(defaultNotifyDedupeTTL),
	}
	mgr.closeAgg = newCloseNoticeAggregator(25*time.Millisecond, mgr.sendAggregatedClose)
	return mgr
}

func (m Manager) SendGate(ctx context.Context, input decisionfmt.DecisionInput, report decisionfmt.DecisionReport) error {
	if m.renderer == nil {
		return fmt.Errorf("decision image renderer is required")
	}
	gateText := strings.TrimSpace(report.Gate.Overall.TradeableText)
	if gateText == "" {
		gateText = "-"
	}
	execTitle := decisionfmt.ResolveExecutionTitle(report)
	if strings.TrimSpace(execTitle) == "" {
		execTitle = fmt.Sprintf("Gate: %s", gateText)
	}
	title := fmt.Sprintf("[%s][snapshot:%d] %s", report.Symbol, report.SnapshotID, execTitle)
	rendered, err := m.renderer.RenderDecision(ctx, input, report)
	if err != nil {
		logging.FromContext(ctx).Named("notify").Error("render decision image failed",
			zap.String("symbol", strings.TrimSpace(report.Symbol)),
			zap.Uint("snapshot_id", report.SnapshotID),
			zap.Error(err),
		)
		return err
	}
	msg := Message{
		Title: title,
		Image: &ImageAsset{
			Data:        append([]byte(nil), rendered.Data...),
			Filename:    rendered.Filename,
			ContentType: rendered.ContentType,
			Caption:     rendered.Caption,
			AltText:     rendered.AltText,
		},
	}
	key := fmt.Sprintf("gate:%s:%d:%s:%s", strings.TrimSpace(report.Symbol), report.SnapshotID, strings.TrimSpace(report.Gate.Overall.DecisionAction), strings.TrimSpace(report.Gate.Overall.ReasonCode))
	return m.sendWithKey(ctx, msg, key)
}

func (m Manager) SendStartup(ctx context.Context, info StartupInfo) error {
	symbolList := strings.Join(info.Symbols, ", ")
	if symbolList == "" {
		symbolList = "-"
	}
	intervalList := strings.Join(info.Intervals, ", ")
	if intervalList == "" {
		intervalList = "-"
	}
	barIntervalText := strings.TrimSpace(info.BarInterval)
	if barIntervalText == "" {
		barIntervalText = "-"
	}
	balanceText := "-"
	if info.Balance > 0 {
		balanceText = formatFloat(info.Balance)
		if strings.TrimSpace(info.Currency) != "" {
			balanceText = balanceText + " " + strings.TrimSpace(info.Currency)
		}
	}
	scheduleText := strings.TrimSpace(info.ScheduleMode)
	if scheduleText == "" {
		scheduleText = "自动调度"
	}

	var sb strings.Builder
	sb.WriteString("🚀 Brale 已启动\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━\n")
	sb.WriteString(fmt.Sprintf("▸ %s：%s\n", Label("symbols"), symbolList))
	sb.WriteString(fmt.Sprintf("▸ %s：%s\n", Label("intervals"), intervalList))
	sb.WriteString(fmt.Sprintf("▸ %s：%s\n", Label("bar_interval"), barIntervalText))
	sb.WriteString(fmt.Sprintf("▸ %s：%s\n", Label("balance"), balanceText))
	sb.WriteString(fmt.Sprintf("▸ %s：%s\n", Label("schedule_mode"), scheduleText))
	for _, item := range info.SymbolStatuses {
		symbol := strings.TrimSpace(item.Symbol)
		if symbol == "" {
			continue
		}
		intervalText := strings.Join(item.Intervals, ", ")
		if intervalText == "" {
			intervalText = "-"
		}
		nextText := strings.TrimSpace(item.NextDecision)
		if nextText == "" {
			nextText = "—"
		}
		sb.WriteString(fmt.Sprintf("  · %s：周期 %s · 下次决策 %s\n", symbol, intervalText, nextText))
	}
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━")
	body := sb.String()

	msg := Message{Title: "Brale 已启动", Markdown: body, Plain: body}
	return m.sendWithKey(ctx, msg, "startup")
}

func (m Manager) SendShutdown(ctx context.Context, info ShutdownInfo) error {
	reason := strings.TrimSpace(info.Reason)
	if reason == "" {
		reason = "正常停止"
	}
	uptimeText := "-"
	if info.Uptime > 0 {
		uptimeText = info.Uptime.Truncate(time.Second).String()
	}

	var sb strings.Builder
	sb.WriteString("🛑 Brale 已停止\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━\n")
	sb.WriteString(fmt.Sprintf("▸ %s：%s\n", Label("reason"), reason))
	sb.WriteString(fmt.Sprintf("▸ %s：%s\n", Label("uptime"), uptimeText))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━")
	body := sb.String()

	msg := Message{Title: "Brale 已停止", Markdown: body, Plain: body}
	return m.sendWithKey(ctx, msg, "shutdown")
}

func (m Manager) SendPositionOpen(ctx context.Context, notice PositionOpenNotice) error {
	symbol := strings.TrimSpace(notice.Symbol)
	if symbol == "" {
		return fmt.Errorf("symbol is required")
	}
	direction := strings.TrimSpace(notice.Direction)
	if direction == "" {
		direction = "-"
	}
	qtyText := formatFloat(notice.Qty)
	entryText := formatFloat(notice.EntryPrice)
	stopText := "-"
	if notice.StopPrice > 0 {
		stopText = formatFloat(notice.StopPrice)
	}
	stopReasonText := strings.TrimSpace(notice.StopReason)
	if stopReasonText == "" {
		stopReasonText = "-"
	}
	tpText := "-"
	if len(notice.TakeProfits) > 0 {
		tpText = formatFloatSlice(notice.TakeProfits)
	}
	riskText := "-"
	if notice.RiskPct > 0 {
		riskText = formatPercent(notice.RiskPct)
	}
	leverageText := "-"
	if notice.Leverage > 0 {
		leverageText = formatFloat(notice.Leverage)
	}
	lines := []string{
		noticeLine("symbol", symbol),
		noticeLine("direction", direction),
		noticeLine("qty", qtyText),
		noticeLine("entry", entryText),
		noticeLine("stop", stopText),
		noticeLine("stop_reason", stopReasonText),
		noticeLine("take_profits", tpText),
		noticeLine("risk_pct", riskText),
		noticeLine("leverage", leverageText),
	}
	if strings.TrimSpace(notice.PositionID) != "" {
		lines = append(lines, noticeLine("position_id", strings.TrimSpace(notice.PositionID)))
	}
	title := fmt.Sprintf("[OPEN][%s] %s", symbol, strings.ToUpper(direction))
	body := buildNoticeBody("📈 仓位开启", lines)
	msg := Message{Title: title, Markdown: body, Plain: body}
	key := strings.TrimSpace(notice.PositionID)
	if key != "" {
		key = "position_open:" + key
	} else {
		key = fmt.Sprintf("position_open:%s:%s:%s:%s", symbol, strings.ToUpper(direction), entryText, qtyText)
	}
	return m.sendWithKey(ctx, msg, key)
}

func (m Manager) SendPositionClose(ctx context.Context, notice PositionCloseNotice) error {
	symbol := strings.TrimSpace(notice.Symbol)
	if symbol == "" {
		return fmt.Errorf("symbol is required")
	}
	direction := strings.TrimSpace(notice.Direction)
	if direction == "" {
		direction = "-"
	}
	key := closeAggregateKeyForPositionClose(notice)
	if m.closeAgg != nil {
		m.closeAgg.AddPositionClose(key, notice)
		return nil
	}
	posID := strings.TrimSpace(notice.PositionID)

	qtyText := formatFloat(notice.Qty)
	closeQtyText := "-"
	if notice.CloseQty > 0 {
		closeQtyText = formatFloat(notice.CloseQty)
	}
	entryText := "-"
	if notice.EntryPrice > 0 {
		entryText = formatFloat(notice.EntryPrice)
	}
	triggerText := "-"
	if notice.TriggerPrice > 0 {
		triggerText = formatFloat(notice.TriggerPrice)
	}
	stopText := "-"
	if notice.StopPrice > 0 {
		stopText = formatFloat(notice.StopPrice)
	}
	tpText := "-"
	if len(notice.TakeProfits) > 0 {
		tpText = formatFloatSlice(notice.TakeProfits)
	}
	reasonText := strings.TrimSpace(notice.Reason)
	if reasonText == "" {
		reasonText = "-"
	}
	riskText := "-"
	if notice.RiskPct > 0 {
		riskText = formatPercent(notice.RiskPct)
	}
	leverageText := "-"
	if notice.Leverage > 0 {
		leverageText = formatFloat(notice.Leverage)
	}
	lines := []string{
		noticeLine("symbol", symbol),
		noticeLine("direction", direction),
		noticeLine("qty", qtyText),
		noticeLine("close_qty", closeQtyText),
		noticeLine("entry", entryText),
		noticeLine("trigger_price", triggerText),
		noticeLine("stop", stopText),
		noticeLine("take_profits", tpText),
		noticeLine("reason", reasonText),
		noticeLine("risk_pct", riskText),
		noticeLine("leverage", leverageText),
	}
	if posID != "" {
		lines = append(lines, noticeLine("position_id", posID))
	}
	title := fmt.Sprintf("[CLOSE][%s] %s", symbol, strings.ToUpper(direction))
	body := buildNoticeBody("📉 仓位关闭", lines)
	msg := Message{Title: title, Markdown: body, Plain: body}
	return m.sendWithKey(ctx, msg, key)
}

func (m Manager) SendPositionCloseSummary(ctx context.Context, notice PositionCloseSummaryNotice) error {
	symbol := strings.TrimSpace(notice.Symbol)
	if symbol == "" {
		return fmt.Errorf("symbol is required")
	}
	direction := strings.TrimSpace(notice.Direction)
	if direction == "" {
		direction = "-"
	}
	key := closeAggregateKeyForPositionCloseSummary(notice)
	if m.closeAgg != nil {
		m.closeAgg.AddPositionCloseSummary(key, notice)
		return nil
	}
	posID := strings.TrimSpace(notice.PositionID)

	qtyText := formatFloat(notice.Qty)
	entryText := "-"
	if notice.EntryPrice > 0 {
		entryText = formatFloat(notice.EntryPrice)
	}
	exitText := "-"
	if notice.ExitPrice > 0 {
		exitText = formatFloat(notice.ExitPrice)
	}
	stopText := "-"
	if notice.StopPrice > 0 {
		stopText = formatFloat(notice.StopPrice)
	}
	tpText := "-"
	if len(notice.TakeProfits) > 0 {
		tpText = formatFloatSlice(notice.TakeProfits)
	}
	reasonText := strings.TrimSpace(notice.Reason)
	if reasonText == "" {
		reasonText = "-"
	}
	riskText := "-"
	if notice.RiskPct > 0 {
		riskText = formatPercent(notice.RiskPct)
	}
	leverageText := "-"
	if notice.Leverage > 0 {
		leverageText = formatFloat(notice.Leverage)
	}
	pnlText := formatFloat(notice.PnLAmount)
	pnlPctText := formatPercent(notice.PnLPct)
	lines := []string{
		noticeLine("symbol", symbol),
		noticeLine("direction", direction),
		noticeLine("qty", qtyText),
		noticeLine("entry", entryText),
		noticeLine("exit", exitText),
		noticeLine("pnl", pnlText),
		noticeLine("pnl_pct", pnlPctText),
		noticeLine("stop", stopText),
		noticeLine("take_profits", tpText),
		noticeLine("reason", reasonText),
		noticeLine("risk_pct", riskText),
		noticeLine("leverage", leverageText),
	}
	if posID != "" {
		lines = append(lines, noticeLine("position_id", posID))
	}
	title := fmt.Sprintf("[CLOSED][%s] %s", symbol, strings.ToUpper(direction))
	body := buildNoticeBody("📉 仓位全部平仓", lines)
	msg := Message{Title: title, Markdown: body, Plain: body}
	return m.sendWithKey(ctx, msg, key)
}

func (m Manager) SendRiskPlanUpdate(ctx context.Context, notice RiskPlanUpdateNotice) error {
	symbol := strings.TrimSpace(notice.Symbol)
	if symbol == "" {
		return fmt.Errorf("symbol is required")
	}
	direction := strings.TrimSpace(notice.Direction)
	if direction == "" {
		direction = "-"
	}
	entryText := "-"
	if notice.EntryPrice > 0 {
		entryText = formatFloat(notice.EntryPrice)
	}
	oldStopText := "-"
	if notice.OldStop > 0 {
		oldStopText = formatFloat(notice.OldStop)
	}
	newStopText := "-"
	if notice.NewStop > 0 {
		newStopText = formatFloat(notice.NewStop)
	}
	tpText := "-"
	if len(notice.TakeProfits) > 0 {
		tpText = formatFloatSlice(notice.TakeProfits)
	}
	sourceText := strings.TrimSpace(notice.Source)
	if sourceText == "" {
		sourceText = "-"
	}
	markText := "-"
	if notice.MarkPrice > 0 {
		markText = formatFloat(notice.MarkPrice)
	}
	stopReasonText := strings.TrimSpace(notice.StopReason)
	if stopReasonText == "" {
		stopReasonText = strings.TrimSpace(notice.Reason)
	}
	reasonText := strings.TrimSpace(notice.Reason)
	if reasonText == "" {
		reasonText = stopReasonText
	}
	if stopReasonText == "" {
		stopReasonText = "-"
	}
	if reasonText == "" {
		reasonText = "-"
	}
	gateText := fmt.Sprintf("%t", notice.GateSatisfied)
	scoreTotalText := "-"
	if notice.ScoreTotal != 0 {
		scoreTotalText = formatFloat(notice.ScoreTotal)
	}
	scoreThresholdText := "-"
	if notice.ScoreThreshold != 0 {
		scoreThresholdText = formatFloat(notice.ScoreThreshold)
	}
	tightenReasonText := strings.TrimSpace(notice.TightenReason)
	if tightenReasonText == "" {
		tightenReasonText = "-"
	}
	lines := []string{
		noticeLine("symbol", symbol),
		noticeLine("direction", direction),
		noticeLine("entry", entryText),
		fmt.Sprintf("▸ %s：%s → %s", Label("stop_prev"), oldStopText, newStopText),
	}
	if stopReasonText != "-" && stopReasonText != reasonText {
		lines = append(lines, noticeLine("stop_reason", stopReasonText))
	}
	if reasonText != "-" {
		lines = append(lines, noticeLine("reason", reasonText))
	}
	if tpText != "-" {
		lines = append(lines, noticeLine("take_profits", tpText))
	}
	lines = append(lines, noticeLine("source", sourceText))
	if markText != "-" {
		lines = append(lines, noticeLine("mark_price", markText))
	}
	if scoreTotalText != "-" || scoreThresholdText != "-" {
		lines = append(lines, fmt.Sprintf("▸ 评分：%s / %s · 通过：%s", scoreTotalText, scoreThresholdText, gateText))
	}
	if tightenReasonText != "-" {
		lines = append(lines, noticeLine("tighten_reason", tightenReasonText))
	}
	posID := strings.TrimSpace(notice.PositionID)
	if posID != "" {
		lines = append(lines, noticeLine("position_id", posID))
	}
	title := fmt.Sprintf("[RISK][%s] %s", symbol, strings.ToUpper(direction))
	body := buildNoticeBody("📋 风控计划更新", lines)
	msg := Message{Title: title, Markdown: body, Plain: body}
	key := posID
	if key != "" {
		key = "risk_plan_update:" + key + ":" + sourceText + ":" + newStopText
	} else {
		key = fmt.Sprintf("risk_plan_update:%s:%s:%s:%s", symbol, strings.ToUpper(direction), sourceText, newStopText)
	}
	return m.sendWithKey(ctx, msg, key)
}

func (m Manager) SendTradeOpen(ctx context.Context, notice TradeOpenNotice) error {
	pair := strings.TrimSpace(notice.Pair)
	if pair == "" {
		return fmt.Errorf("pair is required")
	}
	direction := "long"
	if notice.IsShort {
		direction = "short"
	}

	var sb strings.Builder
	sb.WriteString("📈 开仓通知\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━\n")
	sb.WriteString(fmt.Sprintf("▸ %s：%s\n", Label("pair"), pair))
	sb.WriteString(fmt.Sprintf("▸ %s：%s\n", Label("direction"), direction))
	sb.WriteString(fmt.Sprintf("▸ %s：%s\n", Label("open_rate"), formatFloat(notice.OpenRate)))
	sb.WriteString(fmt.Sprintf("▸ %s：%s\n", Label("amount"), formatFloat(notice.Amount)))
	sb.WriteString(fmt.Sprintf("▸ %s：%s\n", Label("stake_amount"), formatFloat(notice.StakeAmount)))
	sb.WriteString(fmt.Sprintf("▸ %s：%s\n", Label("leverage"), formatFloat(notice.Leverage)))
	if strings.TrimSpace(notice.EnterTag) != "" {
		sb.WriteString(fmt.Sprintf("▸ %s：%s\n", Label("enter_tag"), strings.TrimSpace(notice.EnterTag)))
	}
	sb.WriteString(fmt.Sprintf("▸ %s：%d\n", Label("trade_id"), notice.TradeID))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━")
	body := sb.String()

	title := fmt.Sprintf("[OPEN][%s] %s", pair, strings.ToUpper(direction))
	msg := Message{Title: title, Markdown: body, Plain: body}
	key := fmt.Sprintf("trade_open:%s:%d:%d", pair, notice.TradeID, notice.OpenTimestamp)
	return m.sendWithKey(ctx, msg, key)
}

func (m Manager) SendTradePartialClose(ctx context.Context, notice TradePartialCloseNotice) error {
	pair := strings.TrimSpace(notice.Pair)
	if pair == "" {
		return fmt.Errorf("pair is required")
	}
	direction := "long"
	if notice.IsShort {
		direction = "short"
	}
	exitReason := strings.TrimSpace(notice.ExitReason)
	if exitReason == "" {
		exitReason = "-"
	}
	exitType := strings.TrimSpace(notice.ExitType)
	if exitType == "" {
		exitType = "-"
	}
	lines := []string{
		noticeLine("pair", pair),
		noticeLine("direction", direction),
		fmt.Sprintf("▸ %s：%s → %s", Label("open_rate"), formatFloat(notice.OpenRate), formatFloat(notice.CloseRate)),
		noticeLine("amount", formatFloat(notice.Amount)),
		noticeLine("stake_amount", formatFloat(notice.StakeAmount)),
		fmt.Sprintf("▸ %s：%s (%s)", Label("realized_profit"), formatFloat(notice.RealizedProfit), formatPercent(notice.RealizedProfitRatio)),
		noticeLine("exit_reason", exitReason),
		noticeLine("exit_type", exitType),
	}
	title := fmt.Sprintf("[PARTIAL][%s] %s", pair, strings.ToUpper(direction))
	body := buildNoticeBody("🔄 部分平仓", lines)
	msg := Message{Title: title, Markdown: body, Plain: body}
	key := fmt.Sprintf("trade_partial_close:%s:%d:%s:%s:%s:%s", pair, notice.TradeID, exitReason, exitType, formatFloat(notice.CloseRate), formatFloat(notice.Amount))
	return m.sendWithKey(ctx, msg, key)
}

func (m Manager) SendTradeCloseSummary(ctx context.Context, notice TradeCloseSummaryNotice) error {
	pair := strings.TrimSpace(notice.Pair)
	if pair == "" {
		return fmt.Errorf("pair is required")
	}
	direction := "long"
	if notice.IsShort {
		direction = "short"
	}
	key := closeAggregateKeyForTradeCloseSummary(notice)
	if m.closeAgg != nil {
		m.closeAgg.AddTradeCloseSummary(key, notice)
		return nil
	}

	exitReason := strings.TrimSpace(notice.ExitReason)
	if exitReason == "" {
		exitReason = "-"
	}
	exitType := strings.TrimSpace(notice.ExitType)
	if exitType == "" {
		exitType = "-"
	}
	durationText := "-"
	if notice.TradeDurationS > 0 {
		durationText = formatDuration(notice.TradeDurationS)
	}
	lines := []string{
		noticeLine("pair", pair),
		noticeLine("direction", direction),
		fmt.Sprintf("▸ %s：%s → %s", Label("open_rate"), formatFloat(notice.OpenRate), formatFloat(notice.CloseRate)),
		noticeLine("amount", formatFloat(notice.Amount)),
		noticeLine("leverage", formatFloat(notice.Leverage)),
		fmt.Sprintf("▸ %s：%s (%s)", Label("profit_abs"), formatFloat(notice.ProfitAbs), formatPercent(notice.ProfitPct)),
		noticeLine("exit_reason", exitReason),
		noticeLine("exit_type", exitType),
		fmt.Sprintf("▸ 持仓时长：%s", durationText),
	}
	title := fmt.Sprintf("[CLOSED][%s] %s", pair, strings.ToUpper(direction))
	body := buildNoticeBody("📉 全部平仓完成", lines)
	msg := Message{Title: title, Markdown: body, Plain: body}
	return m.sendWithKey(ctx, msg, key)
}

func (m Manager) sendAggregatedClose(ctx context.Context, aggregated aggregatedCloseNotice) error {
	symbol := strings.TrimSpace(aggregated.Symbol)
	if symbol == "" {
		if aggregated.PositionClose != nil {
			symbol = strings.TrimSpace(aggregated.PositionClose.Symbol)
		} else if aggregated.CloseSummary != nil {
			symbol = strings.TrimSpace(aggregated.CloseSummary.Symbol)
		} else if aggregated.TradeClose != nil {
			symbol = normalizeCloseSymbol(aggregated.TradeClose.Pair)
		}
	}
	if symbol == "" {
		return fmt.Errorf("aggregated close symbol is required")
	}

	direction := strings.TrimSpace(aggregated.Direction)
	if direction == "" {
		switch {
		case aggregated.PositionClose != nil:
			direction = strings.TrimSpace(aggregated.PositionClose.Direction)
		case aggregated.CloseSummary != nil:
			direction = strings.TrimSpace(aggregated.CloseSummary.Direction)
		case aggregated.TradeClose != nil:
			direction = tradeDirection(aggregated.TradeClose.IsShort)
		}
	}
	if direction == "" {
		direction = "-"
	}

	entryPrice := 0.0
	if aggregated.TradeClose != nil && aggregated.TradeClose.OpenRate > 0 {
		entryPrice = aggregated.TradeClose.OpenRate
	} else if aggregated.CloseSummary != nil && aggregated.CloseSummary.EntryPrice > 0 {
		entryPrice = aggregated.CloseSummary.EntryPrice
	} else if aggregated.PositionClose != nil && aggregated.PositionClose.EntryPrice > 0 {
		entryPrice = aggregated.PositionClose.EntryPrice
	}

	exitPrice := 0.0
	if aggregated.TradeClose != nil && aggregated.TradeClose.CloseRate > 0 {
		exitPrice = aggregated.TradeClose.CloseRate
	} else if aggregated.CloseSummary != nil && aggregated.CloseSummary.ExitPrice > 0 {
		exitPrice = aggregated.CloseSummary.ExitPrice
	} else if aggregated.PositionClose != nil && aggregated.PositionClose.TriggerPrice > 0 {
		exitPrice = aggregated.PositionClose.TriggerPrice
	}

	qty := 0.0
	if aggregated.TradeClose != nil && aggregated.TradeClose.Amount > 0 {
		qty = aggregated.TradeClose.Amount
	} else if aggregated.CloseSummary != nil && aggregated.CloseSummary.Qty > 0 {
		qty = aggregated.CloseSummary.Qty
	} else if aggregated.PositionClose != nil && aggregated.PositionClose.Qty > 0 {
		qty = aggregated.PositionClose.Qty
	}

	pnlAmount := 0.0
	pnlPct := 0.0
	if aggregated.TradeClose != nil {
		pnlAmount = aggregated.TradeClose.ProfitAbs
		pnlPct = aggregated.TradeClose.ProfitPct
	} else if aggregated.CloseSummary != nil && (aggregated.CloseSummary.PnLAmount != 0 || aggregated.CloseSummary.PnLPct != 0) {
		pnlAmount = aggregated.CloseSummary.PnLAmount
		pnlPct = aggregated.CloseSummary.PnLPct
	}

	stopPrice := 0.0
	takeProfits := []float64(nil)
	if aggregated.CloseSummary != nil {
		stopPrice = aggregated.CloseSummary.StopPrice
		takeProfits = append(takeProfits, aggregated.CloseSummary.TakeProfits...)
	} else if aggregated.PositionClose != nil {
		stopPrice = aggregated.PositionClose.StopPrice
		takeProfits = append(takeProfits, aggregated.PositionClose.TakeProfits...)
	}

	reason := "-"
	if aggregated.PositionClose != nil && strings.TrimSpace(aggregated.PositionClose.Reason) != "" {
		reason = strings.TrimSpace(aggregated.PositionClose.Reason)
	} else if aggregated.CloseSummary != nil && strings.TrimSpace(aggregated.CloseSummary.Reason) != "" {
		reason = strings.TrimSpace(aggregated.CloseSummary.Reason)
	} else if aggregated.TradeClose != nil && strings.TrimSpace(aggregated.TradeClose.ExitReason) != "" {
		reason = strings.TrimSpace(aggregated.TradeClose.ExitReason)
	}

	exitType := "-"
	if aggregated.TradeClose != nil && strings.TrimSpace(aggregated.TradeClose.ExitType) != "" {
		exitType = strings.TrimSpace(aggregated.TradeClose.ExitType)
	}

	leverage := 0.0
	if aggregated.TradeClose != nil && aggregated.TradeClose.Leverage > 0 {
		leverage = aggregated.TradeClose.Leverage
	} else if aggregated.CloseSummary != nil && aggregated.CloseSummary.Leverage > 0 {
		leverage = aggregated.CloseSummary.Leverage
	} else if aggregated.PositionClose != nil && aggregated.PositionClose.Leverage > 0 {
		leverage = aggregated.PositionClose.Leverage
	}

	positionID := ""
	if aggregated.CloseSummary != nil {
		positionID = strings.TrimSpace(aggregated.CloseSummary.PositionID)
	}
	if positionID == "" && aggregated.PositionClose != nil {
		positionID = strings.TrimSpace(aggregated.PositionClose.PositionID)
	}

	tradeID := 0
	tradeDurationS := int64(0)
	if aggregated.TradeClose != nil {
		tradeID = aggregated.TradeClose.TradeID
		tradeDurationS = aggregated.TradeClose.TradeDurationS
	}
	lines := []string{
		noticeLine("symbol", symbol),
		noticeLine("direction", direction),
		noticeLine("qty", formatFloat(qty)),
		noticeLine("entry", formatFloat(entryPrice)),
		noticeLine("exit", formatFloat(exitPrice)),
		noticeLine("pnl", formatFloat(pnlAmount)),
		noticeLine("pnl_pct", formatPercent(pnlPct)),
		noticeLine("reason", reason),
	}
	if stopPrice > 0 {
		lines = append(lines, noticeLine("stop", formatFloat(stopPrice)))
	}
	if len(takeProfits) > 0 {
		lines = append(lines, noticeLine("take_profits", formatFloatSlice(takeProfits)))
	}
	if exitType != "-" {
		lines = append(lines, noticeLine("exit_type", exitType))
	}
	if tradeDurationS > 0 {
		lines = append(lines, fmt.Sprintf("▸ 持仓时长：%s", formatDuration(tradeDurationS)))
	}
	if leverage > 0 {
		lines = append(lines, noticeLine("leverage", formatFloat(leverage)))
	}
	if tradeID > 0 {
		lines = append(lines, fmt.Sprintf("▸ %s：%d", Label("trade_id"), tradeID))
	}
	if positionID != "" {
		lines = append(lines, noticeLine("position_id", positionID))
	}

	title := fmt.Sprintf("[CLOSED][%s] %s", symbol, strings.ToUpper(direction))
	body := buildNoticeBody("📉 仓位已关闭", lines)
	msg := Message{Title: title, Markdown: body, Plain: body}
	return m.sendWithKey(ctx, msg, aggregated.Key)
}

func (m Manager) SendError(ctx context.Context, notice ErrorNotice) error {
	msgText := strings.TrimSpace(notice.Message)
	if msgText == "" {
		return fmt.Errorf("error message is required")
	}
	severity := strings.TrimSpace(notice.Severity)
	if severity == "" {
		severity = "error"
	}
	component := strings.TrimSpace(notice.Component)
	symbol := strings.TrimSpace(notice.Symbol)

	title := "[ERROR]"
	if symbol != "" {
		title = fmt.Sprintf("[ERROR][%s]", symbol)
	}
	if component != "" {
		title = fmt.Sprintf("%s %s", title, component)
	}

	var sb strings.Builder
	sb.WriteString("⚠️ 错误提醒\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━\n")
	if severity != "" {
		sb.WriteString(fmt.Sprintf("▸ %s：%s\n", Label("severity"), severity))
	}
	if component != "" {
		sb.WriteString(fmt.Sprintf("▸ %s：%s\n", Label("component"), component))
	}
	if symbol != "" {
		sb.WriteString(fmt.Sprintf("▸ %s：%s\n", Label("symbol"), symbol))
	}
	sb.WriteString(fmt.Sprintf("▸ %s：%s\n", Label("detail"), msgText))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━")
	body := sb.String()

	msg := Message{Title: title, Markdown: body, Plain: body}
	return m.send(ctx, msg)
}

func buildNoticeBody(header string, lines []string) string {
	header = strings.TrimSpace(header)
	var sb strings.Builder
	if header != "" {
		sb.WriteString(header)
		sb.WriteString("\n")
	}
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		sb.WriteString("\n")
		sb.WriteString(strings.TrimRight(line, "\r\n"))
	}
	sb.WriteString("\n━━━━━━━━━━━━━━━━━━━━")
	return sb.String()
}

func noticeLine(key string, value string) string {
	return fmt.Sprintf("▸ %s：%s", Label(key), value)
}

func formatFloat(value float64) string {
	text := fmt.Sprintf("%.8f", value)
	text = strings.TrimRight(text, "0")
	text = strings.TrimRight(text, ".")
	if text == "" || text == "-0" {
		return "0"
	}
	return text
}

func formatFloatSlice(values []float64) string {
	if len(values) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(values))
	for _, val := range values {
		parts = append(parts, formatFloat(val))
	}
	return strings.Join(parts, ", ")
}

func formatPercent(value float64) string {
	if value == 0 {
		return "0%"
	}
	text := formatFloat(value * 100)
	return text + "%"
}

func formatDuration(seconds int64) string {
	if seconds <= 0 {
		return "-"
	}
	d := time.Duration(seconds) * time.Second
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func (m Manager) send(ctx context.Context, msg Message) error {
	return m.sendWithKey(ctx, msg, "")
}

func (m Manager) sendWithKey(ctx context.Context, msg Message, dedupeKey string) error {
	now := time.Now()
	logger := logging.FromContext(ctx).Named("notify")
	acquired := false
	if m.dedupe != nil {
		if !m.dedupe.tryAcquire(dedupeKey, now) {
			logger.Debug("notify skipped (dedupe)", zap.String("key", strings.TrimSpace(dedupeKey)), zap.String("title", strings.TrimSpace(msg.Title)))
			return nil
		}
		acquired = true
	}
	errDetails := make([]string, 0)
	successCount := 0
	for _, sender := range m.senders {
		if err := sender.Send(ctx, msg); err != nil {
			errDetails = append(errDetails, fmt.Sprintf("%T: %v", sender, err))
			continue
		}
		successCount++
	}
	if successCount > 0 {
		if acquired {
			m.dedupe.commit(dedupeKey, time.Now())
		}
		if len(errDetails) > 0 {
			logger.Warn("notify send partially failed",
				zap.Int("success_count", successCount),
				zap.Int("failed_count", len(errDetails)),
				zap.String("title", strings.TrimSpace(msg.Title)),
				zap.String("dedupe_key", strings.TrimSpace(dedupeKey)),
				zap.Strings("error_details", errDetails),
			)
			return nil
		}
		logger.Info("notify sent",
			zap.Int("channels", len(m.senders)),
			zap.String("title", strings.TrimSpace(msg.Title)),
		)
		return nil
	}
	if len(errDetails) > 0 {
		logger.Warn("notify send partially failed",
			zap.Int("success_count", successCount),
			zap.Int("failed_count", len(errDetails)),
			zap.String("title", strings.TrimSpace(msg.Title)),
			zap.String("dedupe_key", strings.TrimSpace(dedupeKey)),
			zap.Strings("error_details", errDetails),
		)
		if acquired {
			m.dedupe.release(dedupeKey)
		}
		return fmt.Errorf("notify send failed: %d (%s)", len(errDetails), strings.Join(errDetails, "; "))
	}
	if acquired {
		m.dedupe.release(dedupeKey)
	}
	logger.Info("notify sent",
		zap.Int("channels", len(m.senders)),
		zap.String("title", strings.TrimSpace(msg.Title)),
	)
	return nil
}
