// 本文件主要内容：实现通知管理器与发送流程。
package notify

import (
	"context"
	"fmt"
	"strings"

	"brale-core/internal/decision/decisionfmt"
	"brale-core/internal/pkg/logging"

	"go.uber.org/zap"
)

type Manager struct {
	formatter decisionfmt.Formatter
	senders   []Sender
}

type NopNotifier struct{}

func (NopNotifier) SendGate(ctx context.Context, report decisionfmt.DecisionReport) error {
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

func (NopNotifier) SendError(ctx context.Context, message string) error {
	return nil
}

func NewManager(cfg NotificationConfig, formatter decisionfmt.Formatter) (Notifier, error) {
	if !cfg.Enabled {
		return NopNotifier{}, nil
	}
	if formatter == nil {
		return nil, fmt.Errorf("formatter is required")
	}
	senders := make([]Sender, 0, 2)
	if cfg.Telegram.Enabled {
		sender, err := NewTelegramSender(cfg.Telegram)
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
		return nil, fmt.Errorf("notification enabled but no channel configured")
	}
	return Manager{formatter: formatter, senders: senders}, nil
}

func (m Manager) SendGate(ctx context.Context, report decisionfmt.DecisionReport) error {
	if m.formatter == nil {
		return fmt.Errorf("formatter is required")
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
	header := "🚦 决策报告"
	markdown := prependNoticeHeader(header, m.formatter.RenderDecisionMarkdown(report))
	html := prependNoticeHeader(header, m.formatter.RenderDecisionHTML(report))
	plain := prependNoticeHeader(header, title)
	msg := Message{
		Title:    title,
		Markdown: markdown,
		HTML:     html,
		Plain:    plain,
	}
	return m.send(ctx, msg)
}

func (m Manager) SendPositionOpen(ctx context.Context, notice PositionOpenNotice) error {
	symbol := strings.TrimSpace(notice.Symbol)
	if symbol == "" {
		return fmt.Errorf("symbol is required")
	}
	qtyText := formatFloat(notice.Qty)
	entryText := formatFloat(notice.EntryPrice)
	stopText := "-"
	if notice.StopPrice > 0 {
		stopText = formatFloat(notice.StopPrice)
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
	direction := strings.TrimSpace(notice.Direction)
	if direction == "" {
		direction = "-"
	}
	lines := []string{
		fmt.Sprintf("- %s: %s", Label("symbol"), symbol),
		fmt.Sprintf("- %s: %s", Label("direction"), direction),
		fmt.Sprintf("- %s: %s", Label("qty"), qtyText),
		fmt.Sprintf("- %s: %s", Label("entry"), entryText),
		fmt.Sprintf("- %s: %s", Label("stop"), stopText),
		fmt.Sprintf("- %s: %s", Label("take_profits"), tpText),
		fmt.Sprintf("- %s: %s", Label("risk_pct"), riskText),
		fmt.Sprintf("- %s: %s", Label("leverage"), leverageText),
	}
	if strings.TrimSpace(notice.PositionID) != "" {
		lines = append(lines, fmt.Sprintf("- %s: %s", Label("position_id"), strings.TrimSpace(notice.PositionID)))
	}
	title := fmt.Sprintf("[OPEN][%s] %s", symbol, strings.ToUpper(direction))
	body := strings.Join(lines, "\n")
	body = prependNoticeHeader("📈 仓位开启", body)
	msg := Message{
		Title:    title,
		Markdown: body,
		Plain:    body,
	}
	return m.send(ctx, msg)
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
		fmt.Sprintf("- %s: %s", Label("symbol"), symbol),
		fmt.Sprintf("- %s: %s", Label("direction"), direction),
		fmt.Sprintf("- %s: %s", Label("qty"), qtyText),
		fmt.Sprintf("- %s: %s", Label("close_qty"), closeQtyText),
		fmt.Sprintf("- %s: %s", Label("entry"), entryText),
		fmt.Sprintf("- %s: %s", Label("trigger_price"), triggerText),
		fmt.Sprintf("- %s: %s", Label("stop"), stopText),
		fmt.Sprintf("- %s: %s", Label("take_profits"), tpText),
		fmt.Sprintf("- %s: %s", Label("reason"), reasonText),
		fmt.Sprintf("- %s: %s", Label("risk_pct"), riskText),
		fmt.Sprintf("- %s: %s", Label("leverage"), leverageText),
	}
	if strings.TrimSpace(notice.PositionID) != "" {
		lines = append(lines, fmt.Sprintf("- %s: %s", Label("position_id"), strings.TrimSpace(notice.PositionID)))
	}
	title := fmt.Sprintf("[CLOSE][%s] %s", symbol, strings.ToUpper(direction))
	body := strings.Join(lines, "\n")
	body = prependNoticeHeader("📉 仓位关闭", body)
	msg := Message{
		Title:    title,
		Markdown: body,
		Plain:    body,
	}
	return m.send(ctx, msg)
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
		fmt.Sprintf("- %s: %s", Label("symbol"), symbol),
		fmt.Sprintf("- %s: %s", Label("direction"), direction),
		fmt.Sprintf("- %s: %s", Label("qty"), qtyText),
		fmt.Sprintf("- %s: %s", Label("entry"), entryText),
		fmt.Sprintf("- %s: %s", Label("exit"), exitText),
		fmt.Sprintf("- %s: %s", Label("pnl"), pnlText),
		fmt.Sprintf("- %s: %s", Label("pnl_pct"), pnlPctText),
		fmt.Sprintf("- %s: %s", Label("stop"), stopText),
		fmt.Sprintf("- %s: %s", Label("take_profits"), tpText),
		fmt.Sprintf("- %s: %s", Label("reason"), reasonText),
		fmt.Sprintf("- %s: %s", Label("risk_pct"), riskText),
		fmt.Sprintf("- %s: %s", Label("leverage"), leverageText),
	}
	if strings.TrimSpace(notice.PositionID) != "" {
		lines = append(lines, fmt.Sprintf("- %s: %s", Label("position_id"), strings.TrimSpace(notice.PositionID)))
	}
	title := fmt.Sprintf("[CLOSED][%s] %s", symbol, strings.ToUpper(direction))
	body := strings.Join(lines, "\n")
	body = prependNoticeHeader("📉 仓位全部平仓", body)
	msg := Message{
		Title:    title,
		Markdown: body,
		Plain:    body,
	}
	return m.send(ctx, msg)
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
	atrText := "-"
	if notice.ATR > 0 {
		atrText = formatFloat(notice.ATR)
	}
	volatilityText := "-"
	if notice.Volatility != 0 {
		volatilityText = formatFloat(notice.Volatility)
	}
	riskText := "-"
	if notice.RiskPct > 0 {
		riskText = formatPercent(notice.RiskPct)
	}
	leverageText := "-"
	if notice.Leverage > 0 {
		leverageText = formatFloat(notice.Leverage)
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
	breakdownText := "-"
	if len(notice.ScoreBreakdown) > 0 {
		parts := make([]string, 0, len(notice.ScoreBreakdown))
		for _, item := range notice.ScoreBreakdown {
			valueText := strings.TrimSpace(item.Value)
			if valueText == "" {
				valueText = "-"
			}
			parts = append(parts, fmt.Sprintf("%s=%s (w=%s, c=%s)", strings.TrimSpace(item.Signal), valueText, formatFloat(item.Weight), formatFloat(item.Contribution)))
		}
		breakdownText = strings.Join(parts, "; ")
	}
	parseText := fmt.Sprintf("%t", notice.ParseOK)
	tightenReasonText := strings.TrimSpace(notice.TightenReason)
	if tightenReasonText == "" {
		tightenReasonText = "-"
	}
	tpTightenedText := fmt.Sprintf("%t", notice.TPTightened)
	lines := []string{
		fmt.Sprintf("- %s: %s", Label("symbol"), symbol),
		fmt.Sprintf("- %s: %s", Label("direction"), direction),
		fmt.Sprintf("- %s: %s", Label("entry"), entryText),
		fmt.Sprintf("- %s: %s", Label("stop_prev"), oldStopText),
		fmt.Sprintf("- %s: %s", Label("stop_new"), newStopText),
		fmt.Sprintf("- %s: %s", Label("take_profits"), tpText),
		fmt.Sprintf("- %s: %s", Label("source"), sourceText),
		fmt.Sprintf("- %s: %s", Label("mark_price"), markText),
		fmt.Sprintf("- %s: %s", Label("atr"), atrText),
		fmt.Sprintf("- %s: %s", Label("volatility"), volatilityText),
		fmt.Sprintf("- %s: %s", Label("gate_satisfied"), gateText),
		fmt.Sprintf("- %s: %s", Label("score_total"), scoreTotalText),
		fmt.Sprintf("- %s: %s", Label("score_threshold"), scoreThresholdText),
		fmt.Sprintf("- %s: %s", Label("score_breakdown"), breakdownText),
		fmt.Sprintf("- %s: %s", Label("parse_ok"), parseText),
		fmt.Sprintf("- %s: %s", Label("tighten_reason"), tightenReasonText),
		fmt.Sprintf("- %s: %s", Label("tp_tightened"), tpTightenedText),
		fmt.Sprintf("- %s: %s", Label("risk_pct"), riskText),
		fmt.Sprintf("- %s: %s", Label("leverage"), leverageText),
	}
	if strings.TrimSpace(notice.PositionID) != "" {
		lines = append(lines, fmt.Sprintf("- %s: %s", Label("position_id"), strings.TrimSpace(notice.PositionID)))
	}
	title := fmt.Sprintf("[RISK][%s] %s", symbol, strings.ToUpper(direction))
	body := strings.Join(lines, "\n")
	body = prependNoticeHeader("📋 风控计划更新", body)
	msg := Message{
		Title:    title,
		Markdown: body,
		Plain:    body,
	}
	return m.send(ctx, msg)
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
	lines := []string{
		fmt.Sprintf("- %s: %s", Label("pair"), pair),
		fmt.Sprintf("- %s: %s", Label("amount"), formatFloat(notice.Amount)),
		fmt.Sprintf("- %s: %s", Label("stake_amount"), formatFloat(notice.StakeAmount)),
		fmt.Sprintf("- %s: %t", Label("is_short"), notice.IsShort),
		fmt.Sprintf("- %s: %s", Label("open_rate"), formatFloat(notice.OpenRate)),
		fmt.Sprintf("- %s: %s", Label("leverage"), formatFloat(notice.Leverage)),
		fmt.Sprintf("- %s: %d", Label("trade_id"), notice.TradeID),
	}
	if strings.TrimSpace(notice.EnterTag) != "" {
		lines = append(lines, fmt.Sprintf("- %s: %s", Label("enter_tag"), strings.TrimSpace(notice.EnterTag)))
	}
	if notice.OpenTimestamp > 0 {
		lines = append(lines, fmt.Sprintf("- %s: %d", Label("open_ts"), notice.OpenTimestamp))
	}
	title := fmt.Sprintf("[OPEN][%s] %s", pair, strings.ToUpper(direction))
	body := strings.Join(lines, "\n")
	body = prependNoticeHeader("📈 开仓通知", body)
	msg := Message{Title: title, Markdown: body, Plain: body}
	return m.send(ctx, msg)
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
		fmt.Sprintf("- %s: %s", Label("pair"), pair),
		fmt.Sprintf("- %s: %t", Label("is_short"), notice.IsShort),
		fmt.Sprintf("- %s: %s", Label("open_rate"), formatFloat(notice.OpenRate)),
		fmt.Sprintf("- %s: %s", Label("close_rate"), formatFloat(notice.CloseRate)),
		fmt.Sprintf("- %s: %s", Label("amount"), formatFloat(notice.Amount)),
		fmt.Sprintf("- %s: %s", Label("stake_amount"), formatFloat(notice.StakeAmount)),
		fmt.Sprintf("- %s: %s", Label("realized_profit"), formatFloat(notice.RealizedProfit)),
		fmt.Sprintf("- %s: %s", Label("realized_profit_ratio"), formatFloat(notice.RealizedProfitRatio)),
		fmt.Sprintf("- %s: %s", Label("exit_reason"), exitReason),
		fmt.Sprintf("- %s: %s", Label("exit_type"), exitType),
		fmt.Sprintf("- %s: %d", Label("trade_id"), notice.TradeID),
	}
	title := fmt.Sprintf("[PARTIAL][%s] %s", pair, strings.ToUpper(direction))
	body := strings.Join(lines, "\n")
	body = prependNoticeHeader("🔄 部分平仓", body)
	msg := Message{Title: title, Markdown: body, Plain: body}
	return m.send(ctx, msg)
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
	exitReason := strings.TrimSpace(notice.ExitReason)
	if exitReason == "" {
		exitReason = "-"
	}
	exitType := strings.TrimSpace(notice.ExitType)
	if exitType == "" {
		exitType = "-"
	}
	lines := []string{
		fmt.Sprintf("- %s: %s", Label("pair"), pair),
		fmt.Sprintf("- %s: %t", Label("is_short"), notice.IsShort),
		fmt.Sprintf("- %s: %s", Label("open_rate"), formatFloat(notice.OpenRate)),
		fmt.Sprintf("- %s: %s", Label("close_rate"), formatFloat(notice.CloseRate)),
		fmt.Sprintf("- %s: %s", Label("amount"), formatFloat(notice.Amount)),
		fmt.Sprintf("- %s: %s", Label("stake_amount"), formatFloat(notice.StakeAmount)),
		fmt.Sprintf("- %s: %s", Label("close_profit_abs"), formatFloat(notice.CloseProfitAbs)),
		fmt.Sprintf("- %s: %s", Label("close_profit_pct"), formatFloat(notice.CloseProfitPct)),
		fmt.Sprintf("- %s: %s", Label("profit_abs"), formatFloat(notice.ProfitAbs)),
		fmt.Sprintf("- %s: %s", Label("profit_pct"), formatFloat(notice.ProfitPct)),
		fmt.Sprintf("- %s: %d", Label("trade_duration_s"), notice.TradeDurationS),
		fmt.Sprintf("- %s: %d", Label("trade_duration"), notice.TradeDuration),
		fmt.Sprintf("- %s: %s", Label("exit_reason"), exitReason),
		fmt.Sprintf("- %s: %s", Label("exit_type"), exitType),
		fmt.Sprintf("- %s: %s", Label("leverage"), formatFloat(notice.Leverage)),
		fmt.Sprintf("- %s: %d", Label("trade_id"), notice.TradeID),
	}
	title := fmt.Sprintf("[CLOSED][%s] %s", pair, strings.ToUpper(direction))
	body := strings.Join(lines, "\n")
	body = prependNoticeHeader("📉 全部平仓完成", body)
	msg := Message{Title: title, Markdown: body, Plain: body}
	return m.send(ctx, msg)
}

func (m Manager) SendError(ctx context.Context, message string) error {
	msgText := strings.TrimSpace(message)
	if msgText == "" {
		return fmt.Errorf("error message is required")
	}
	msgText = prependNoticeHeader("⚠️ 错误提醒", msgText)
	msg := Message{
		Title:    "[ERROR]",
		Markdown: msgText,
		Plain:    msgText,
	}
	return m.send(ctx, msg)
}

func prependNoticeHeader(header string, body string) string {
	header = strings.TrimSpace(header)
	body = strings.TrimSpace(body)
	if header == "" {
		return body
	}
	if body == "" {
		return header
	}
	return header + "\n" + body
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

func (m Manager) send(ctx context.Context, msg Message) error {
	var errCount int
	for _, sender := range m.senders {
		if err := sender.Send(ctx, msg); err != nil {
			errCount++
		}
	}
	if errCount > 0 {
		return fmt.Errorf("notify send failed: %d", errCount)
	}
	logging.FromContext(ctx).Named("notify").Info("notify sent",
		zap.Int("channels", len(m.senders)),
		zap.String("title", strings.TrimSpace(msg.Title)),
	)
	return nil
}
