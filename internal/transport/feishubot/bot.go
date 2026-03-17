package feishubot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"brale-core/internal/pkg/symbol"
	"brale-core/internal/transport/botruntime"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/core/httpserverext"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	"go.uber.org/zap"
)

const (
	defaultSessionTTL     = 5 * time.Minute
	defaultIdempotencyTTL = 15 * time.Minute
	maxReplyChunk         = 3500
	tradeHistoryMenuLimit = 10
)

var (
	leadingAtTokenPattern = regexp.MustCompile(`^(?:@\S+\s+)+`)
	feishuAtTagPattern    = regexp.MustCompile(`(?is)<at[^>]*>.*?</at>`)
)

type RuntimeClient interface {
	FetchMonitorStatus(ctx context.Context) (botruntime.MonitorStatusResponse, error)
	FetchPositionStatus(ctx context.Context) (botruntime.PositionStatusResponse, error)
	FetchTradeHistory(ctx context.Context) (botruntime.TradeHistoryResponse, error)
	FetchDecisionLatest(ctx context.Context, symbol string) (botruntime.DecisionLatestResponse, error)
	FetchObserveReport(ctx context.Context, symbol string) (botruntime.ObserveResponse, error)
	PostScheduleToggle(ctx context.Context, enable bool) (botruntime.ScheduleResponse, error)
	RunObserve(ctx context.Context, req botruntime.ObserveRunRequest) (botruntime.ObserveResponse, error)
}

type Messenger interface {
	SendText(ctx context.Context, chatID string, text string) error
}

type Bot struct {
	logger      *zap.Logger
	runtime     RuntimeClient
	messenger   Messenger
	httpHandler http.HandlerFunc
	verifyToken string
	mode        string

	longConnStarter func(context.Context) error

	sessions *sessionStore

	idMu          sync.Mutex
	processedIDs  map[string]time.Time
	idempotencyTT time.Duration
}

func New(cfg Config, logger *zap.Logger) (*Bot, error) {
	if strings.TrimSpace(cfg.RuntimeBaseURL) == "" {
		return nil, errors.New("runtime base url is required")
	}
	if strings.TrimSpace(cfg.AppID) == "" {
		return nil, errors.New("feishu app_id is required")
	}
	if strings.TrimSpace(cfg.AppSecret) == "" {
		return nil, errors.New("feishu app_secret is required")
	}
	mode := resolveMode(cfg.Mode)
	if mode == ModeCallback && strings.TrimSpace(cfg.VerificationToken) == "" {
		return nil, errors.New("feishu verification token is required")
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	runtimeClient, err := botruntime.NewClient(strings.TrimSpace(cfg.RuntimeBaseURL), &http.Client{Timeout: 90 * time.Second})
	if err != nil {
		return nil, err
	}
	msgClient := lark.NewClient(strings.TrimSpace(cfg.AppID), strings.TrimSpace(cfg.AppSecret), lark.WithReqTimeout(15*time.Second))

	bot := &Bot{
		logger:        logger,
		runtime:       runtimeClient,
		messenger:     &feishuMessenger{client: msgClient},
		verifyToken:   strings.TrimSpace(cfg.VerificationToken),
		mode:          mode,
		sessions:      newSessionStore(resolveDuration(cfg.SessionTTL, defaultSessionTTL)),
		processedIDs:  make(map[string]time.Time),
		idempotencyTT: resolveDuration(cfg.IdempotencyTTL, defaultIdempotencyTTL),
	}

	d := dispatcher.NewEventDispatcher(strings.TrimSpace(cfg.VerificationToken), strings.TrimSpace(cfg.EncryptKey)).
		OnP2MessageReceiveV1(bot.handleMessageReceive)
	bot.httpHandler = httpserverext.NewEventHandlerFunc(d)
	if mode == ModeLongConnection {
		wsClient := larkws.NewClient(
			strings.TrimSpace(cfg.AppID),
			strings.TrimSpace(cfg.AppSecret),
			larkws.WithEventHandler(d),
			larkws.WithLogLevel(larkcore.LogLevelWarn),
		)
		bot.longConnStarter = wsClient.Start
	}
	return bot, nil
}

func NewWithDeps(cfg Config, logger *zap.Logger, runtimeClient RuntimeClient, messenger Messenger) (*Bot, error) {
	mode := resolveMode(cfg.Mode)
	if mode == ModeCallback && strings.TrimSpace(cfg.VerificationToken) == "" {
		return nil, errors.New("feishu verification token is required")
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	if runtimeClient == nil {
		return nil, errors.New("runtime client is required")
	}
	if messenger == nil {
		return nil, errors.New("messenger is required")
	}

	bot := &Bot{
		logger:        logger,
		runtime:       runtimeClient,
		messenger:     messenger,
		verifyToken:   strings.TrimSpace(cfg.VerificationToken),
		mode:          mode,
		sessions:      newSessionStore(resolveDuration(cfg.SessionTTL, defaultSessionTTL)),
		processedIDs:  make(map[string]time.Time),
		idempotencyTT: resolveDuration(cfg.IdempotencyTTL, defaultIdempotencyTTL),
	}
	d := dispatcher.NewEventDispatcher(strings.TrimSpace(cfg.VerificationToken), strings.TrimSpace(cfg.EncryptKey)).
		OnP2MessageReceiveV1(bot.handleMessageReceive)
	bot.httpHandler = httpserverext.NewEventHandlerFunc(d)
	return bot, nil
}

func (b *Bot) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.httpHandler(w, r)
	})
}

func (b *Bot) RunLongConnection(ctx context.Context) error {
	if b.mode != ModeLongConnection {
		return errors.New("feishu bot mode is not long_connection")
	}
	if b.longConnStarter == nil {
		return errors.New("feishu long connection starter is not configured")
	}
	return b.longConnStarter(ctx)
}

func (b *Bot) handleMessageReceive(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
	if event == nil || event.Event == nil || event.Event.Message == nil {
		return nil
	}
	if b.verifyToken != "" && event.EventV2Base != nil && event.EventV2Base.Header != nil {
		token := strings.TrimSpace(event.EventV2Base.Header.Token)
		if token != "" && token != b.verifyToken {
			return errors.New("invalid feishu event token")
		}
	}
	msg := event.Event.Message
	messageID := ptrValue(msg.MessageId)
	if messageID != "" && b.isDuplicateEvent(messageID) {
		return nil
	}

	chatID := ptrValue(msg.ChatId)
	if chatID == "" {
		return nil
	}
	senderID := extractSenderID(event)
	text := extractMessageText(ptrValue(msg.Content))
	if strings.TrimSpace(text) == "" {
		return nil
	}
	b.logger.Info("feishu message received",
		zap.String("message_id", messageID),
		zap.String("chat_id", chatID),
		zap.String("sender_id", senderID),
		zap.String("text", strings.TrimSpace(text)),
	)

	go b.processMessage(context.Background(), senderID, chatID, text)
	return nil
}

func (b *Bot) processMessage(ctx context.Context, senderID, chatID, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	commandText := normalizeCommandInput(text)

	if sess, ok := b.sessions.get(senderID); ok {
		symbolText := normalizeSymbol(commandText)
		if !isValidSymbol(symbolText) {
			b.sendReply(ctx, chatID, "invalid symbol, please input like BTC or BTCUSDT")
			return
		}
		b.sessions.delete(senderID)
		switch sess.Step {
		case stepAwaitObserveSymbol:
			b.handleObserve(ctx, chatID, symbolText)
			return
		case stepAwaitLatestSymbol:
			b.handleLatest(ctx, chatID, symbolText)
			return
		}
	}

	action, arg := parseCommand(commandText)
	switch action {
	case "monitor":
		b.handleMonitor(ctx, chatID)
	case "positions":
		b.handlePositions(ctx, chatID)
	case "trades":
		b.handleTrades(ctx, chatID)
	case "observe":
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
	case "schedule_on":
		b.handleSchedule(ctx, chatID, true)
	case "schedule_off":
		b.handleSchedule(ctx, chatID, false)
	case "latest":
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
	default:
		b.sendReply(ctx, chatID, helpText())
	}
}

func (b *Bot) handleMonitor(ctx context.Context, chatID string) {
	resp, err := b.runtime.FetchMonitorStatus(ctx)
	if err != nil {
		b.sendReply(ctx, chatID, "monitor failed: "+err.Error())
		return
	}
	b.sendReply(ctx, chatID, renderMonitor(resp))
}

func (b *Bot) handlePositions(ctx context.Context, chatID string) {
	resp, err := b.runtime.FetchPositionStatus(ctx)
	if err != nil {
		b.sendReply(ctx, chatID, "positions failed: "+err.Error())
		return
	}
	b.sendReply(ctx, chatID, renderPositions(resp))
}

func (b *Bot) handleTrades(ctx context.Context, chatID string) {
	resp, err := b.runtime.FetchTradeHistory(ctx)
	if err != nil {
		b.sendReply(ctx, chatID, "trades failed: "+err.Error())
		return
	}
	b.sendReply(ctx, chatID, renderTrades(resp))
}

func (b *Bot) handleObserve(ctx context.Context, chatID, symbol string) {
	resp, err := b.runtime.RunObserve(ctx, botruntime.ObserveRunRequest{Symbol: symbol})
	if err != nil {
		b.sendReply(ctx, chatID, "observe failed: "+err.Error())
		return
	}
	text := strings.TrimSpace(resp.ReportMarkdown)
	if text == "" {
		text = strings.TrimSpace(resp.Report)
	}
	if text == "" {
		text = strings.TrimSpace(resp.Summary)
	}
	if text == "" {
		text = "observe completed"
	}
	b.sendChunked(ctx, chatID, text)
}

func (b *Bot) handleSchedule(ctx context.Context, chatID string, enable bool) {
	resp, err := b.runtime.PostScheduleToggle(ctx, enable)
	if err != nil {
		b.sendReply(ctx, chatID, "schedule toggle failed: "+err.Error())
		return
	}
	b.sendReply(ctx, chatID, renderSchedule(resp))
}

func (b *Bot) handleLatest(ctx context.Context, chatID, symbol string) {
	resp, err := b.runtime.FetchDecisionLatest(ctx, symbol)
	if err != nil {
		b.sendReply(ctx, chatID, "latest decision failed: "+err.Error())
		return
	}
	text := strings.TrimSpace(resp.ReportMarkdown)
	if text == "" {
		text = strings.TrimSpace(resp.Report)
	}
	if text == "" {
		text = strings.TrimSpace(resp.Summary)
	}
	if text == "" {
		text = "no decision found"
	}
	if b.sendDecisionCard(ctx, chatID, "🚦 决策报告", text) {
		return
	}
	b.sendChunked(ctx, chatID, text)
}
func (b *Bot) sendChunked(ctx context.Context, chatID, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	for _, chunk := range splitChunks(text, maxReplyChunk) {
		b.sendReply(ctx, chatID, chunk)
	}
}

func (b *Bot) sendReply(ctx context.Context, chatID, text string) {
	if err := b.messenger.SendText(ctx, chatID, text); err != nil {
		b.logger.Warn("feishu reply failed", zap.String("chat_id", chatID), zap.Error(err))
	}
}

func (b *Bot) sendDecisionCard(ctx context.Context, chatID, title, markdown string) bool {
	sender, ok := b.messenger.(interface {
		SendCard(ctx context.Context, chatID, title, markdown string) error
	})
	if !ok {
		return false
	}
	if err := sender.SendCard(ctx, chatID, title, markdown); err != nil {
		b.logger.Warn("feishu decision card failed, fallback to text", zap.String("chat_id", chatID), zap.Error(err))
		return false
	}
	return true
}

func (b *Bot) isDuplicateEvent(id string) bool {
	b.idMu.Lock()
	defer b.idMu.Unlock()
	now := time.Now()
	for key, ts := range b.processedIDs {
		if now.Sub(ts) > b.idempotencyTT {
			delete(b.processedIDs, key)
		}
	}
	if _, ok := b.processedIDs[id]; ok {
		return true
	}
	b.processedIDs[id] = now
	return false
}

func parseCommand(input string) (string, string) {
	normalized := strings.TrimSpace(strings.ToLower(input))
	normalized = strings.TrimPrefix(normalized, "/")
	normalized = strings.TrimSpace(normalized)
	parts := strings.Fields(normalized)
	if len(parts) == 0 {
		return "", ""
	}
	cmd := parts[0]
	arg := ""
	if len(parts) > 1 {
		arg = parts[1]
	}
	switch cmd {
	case "monitor":
		return "monitor", ""
	case "positions":
		return "positions", ""
	case "trades", "history":
		return "trades", ""
	case "observe":
		return "observe", arg
	case "toggle", "schedule":
		if arg == "on" || arg == "enable" {
			return "schedule_on", ""
		}
		if arg == "off" || arg == "disable" {
			return "schedule_off", ""
		}
		return "", ""
	case "latest", "decision":
		return "latest", arg
	default:
		return "", ""
	}
}

func normalizeCommandInput(input string) string {
	normalized := strings.TrimSpace(input)
	if normalized == "" {
		return ""
	}
	normalized = strings.ReplaceAll(normalized, "\u00a0", " ")
	normalized = feishuAtTagPattern.ReplaceAllString(normalized, " ")
	normalized = strings.TrimSpace(leadingAtTokenPattern.ReplaceAllString(normalized, ""))
	normalized = strings.TrimPrefix(normalized, "/")
	return strings.TrimSpace(normalized)
}

func normalizeSymbol(raw string) string {
	return symbol.Normalize(raw)
}

func isValidSymbol(value string) bool {
	if len(value) < 2 || len(value) > 20 {
		return false
	}
	for _, ch := range value {
		if (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') {
			return false
		}
	}
	return true
}

func extractSenderID(event *larkim.P2MessageReceiveV1) string {
	if event == nil || event.Event == nil || event.Event.Sender == nil || event.Event.Sender.SenderId == nil {
		return ""
	}
	if event.Event.Sender.SenderId.OpenId != nil {
		return strings.TrimSpace(*event.Event.Sender.SenderId.OpenId)
	}
	if event.Event.Sender.SenderId.UserId != nil {
		return strings.TrimSpace(*event.Event.Sender.SenderId.UserId)
	}
	if event.Event.Sender.SenderId.UnionId != nil {
		return strings.TrimSpace(*event.Event.Sender.SenderId.UnionId)
	}
	return ""
}

type contentText struct {
	Text string `json:"text"`
}

type cardContent struct {
	Config   cardConfig    `json:"config"`
	Header   cardHeader    `json:"header"`
	Elements []cardElement `json:"elements"`
}

type cardConfig struct {
	WideScreenMode bool `json:"wide_screen_mode"`
}

type cardHeader struct {
	Title    cardTitle `json:"title"`
	Template string    `json:"template,omitempty"`
}

type cardTitle struct {
	Tag     string `json:"tag"`
	Content string `json:"content"`
}

type cardElement struct {
	Tag  string    `json:"tag"`
	Text *cardText `json:"text,omitempty"`
}

type cardText struct {
	Tag     string `json:"tag"`
	Content string `json:"content"`
}

func extractMessageText(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	var parsed contentText
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Text)
}

func splitChunks(text string, size int) []string {
	if len(text) <= size {
		return []string{text}
	}
	parts := make([]string, 0, len(text)/size+1)
	remaining := text
	for len(remaining) > size {
		parts = append(parts, remaining[:size])
		remaining = remaining[size:]
	}
	if remaining != "" {
		parts = append(parts, remaining)
	}
	return parts
}

func renderMonitor(resp botruntime.MonitorStatusResponse) string {
	if len(resp.Symbols) == 0 {
		return "暂无监控币种。"
	}
	b := &strings.Builder{}
	for _, sym := range resp.Symbols {
		b.WriteString("【")
		b.WriteString(sym.Symbol)
		b.WriteString("】\n")
		b.WriteString("下一次运行: ")
		b.WriteString(formatTime(sym.NextRun))
		b.WriteString("\n")
		b.WriteString("K线周期: ")
		b.WriteString(sym.KlineInterval)
		b.WriteString("\n")
		fmt.Fprintf(b, "单笔风险: %.4f (≈ %.2f USDT)\n", sym.RiskPct, sym.RiskAmount)
		fmt.Fprintf(b, "最大杠杆: %.2f\n", sym.MaxLeverage)
		fmt.Fprintf(b, "止盈倍数: %.2f\n", sym.TakeProfitMultiple)
		fmt.Fprintf(b, "初始止损倍数: %.2f\n", sym.InitialStopMultiple)
		b.WriteString("入场定价: ")
		b.WriteString(sym.EntryPricingMode)
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}

func renderPositions(resp botruntime.PositionStatusResponse) string {
	if len(resp.Positions) == 0 {
		return "暂无持仓。"
	}
	b := &strings.Builder{}
	for _, pos := range resp.Positions {
		b.WriteString("【")
		b.WriteString(pos.Symbol)
		b.WriteString("】\n")
		fmt.Fprintf(b, "方向: %s\n", pos.Side)
		fmt.Fprintf(b, "数量: %.6f (原始 %.6f)\n", pos.Amount, pos.AmountRequested)
		fmt.Fprintf(b, "保证金: %.4f\n", pos.MarginAmount)
		fmt.Fprintf(b, "开仓价: %.4f\n", pos.EntryPrice)
		fmt.Fprintf(b, "当前价: %.4f\n", pos.CurrentPrice)
		fmt.Fprintf(b, "收益: %.4f (已实现 %.4f / 未实现 %.4f)\n", pos.ProfitTotal, pos.ProfitRealized, pos.ProfitUnrealized)
		b.WriteString("开仓时间: ")
		b.WriteString(formatOpenedAt(pos.OpenedAt))
		b.WriteString("\n")
		fmt.Fprintf(b, "持仓时长: %dm\n", pos.DurationMin)
		if len(pos.TakeProfits) > 0 {
			b.WriteString("止盈: ")
			b.WriteString(formatFloatList(pos.TakeProfits))
			b.WriteString("\n")
		} else {
			b.WriteString("止盈: —\n")
		}
		if pos.StopLoss > 0 {
			fmt.Fprintf(b, "止损: %.4f\n", pos.StopLoss)
		} else {
			b.WriteString("止损: —\n")
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func renderTrades(resp botruntime.TradeHistoryResponse) string {
	items := latestTradeHistory(resp.Trades, tradeHistoryMenuLimit)
	if len(items) == 0 {
		return "暂无历史仓位。"
	}
	b := &strings.Builder{}
	for _, tr := range items {
		b.WriteString("【")
		b.WriteString(tr.Symbol)
		b.WriteString("】\n")
		fmt.Fprintf(b, "方向: %s\n", tr.Side)
		fmt.Fprintf(b, "数量: %.6f\n", tr.Amount)
		fmt.Fprintf(b, "保证金: %.4f\n", tr.MarginAmount)
		b.WriteString("开仓时间: ")
		b.WriteString(formatTime(tr.OpenedAt))
		b.WriteString("\n")
		fmt.Fprintf(b, "持仓时长: %ds\n", tr.DurationSec)
		fmt.Fprintf(b, "收益: %.4f\n\n", tr.Profit)
	}
	return strings.TrimSpace(b.String())
}

func renderSchedule(resp botruntime.ScheduleResponse) string {
	b := &strings.Builder{}
	if strings.TrimSpace(resp.Summary) != "" {
		b.WriteString(resp.Summary)
		b.WriteString("\n")
	}
	if resp.LLMScheduled {
		if len(resp.NextRuns) > 0 {
			b.WriteString("下一次运行：\n")
			for _, item := range resp.NextRuns {
				b.WriteString("- ")
				b.WriteString(item.Symbol)
				b.WriteString(" ")
				b.WriteString(item.NextExecution)
				if item.BarInterval != "" {
					b.WriteString(" (")
					b.WriteString(item.BarInterval)
					b.WriteString(")")
				}
				b.WriteString("\n")
			}
		}
		return strings.TrimSpace(b.String())
	}
	if len(resp.Positions) > 0 {
		b.WriteString("\n")
		b.WriteString(renderPositions(botruntime.PositionStatusResponse{Positions: resp.Positions}))
	}
	return strings.TrimSpace(b.String())
}
func latestTradeHistory(items []botruntime.TradeHistoryItem, limit int) []botruntime.TradeHistoryItem {
	if len(items) == 0 || limit <= 0 {
		return nil
	}
	sorted := append([]botruntime.TradeHistoryItem(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left := sorted[i].OpenedAt
		right := sorted[j].OpenedAt
		if left.Equal(right) {
			return sorted[i].Symbol < sorted[j].Symbol
		}
		if left.IsZero() {
			return false
		}
		if right.IsZero() {
			return true
		}
		return left.After(right)
	})
	if len(sorted) > limit {
		sorted = sorted[:limit]
	}
	return sorted
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2006-01-02 15:04:05")
}

func formatOpenedAt(value string) string {
	if strings.TrimSpace(value) == "" {
		return "—"
	}
	return value
}

func formatFloatList(items []float64) string {
	if len(items) == 0 {
		return "—"
	}
	parts := make([]string, 0, len(items))
	for _, v := range items {
		parts = append(parts, fmt.Sprintf("%.4f", v))
	}
	return strings.Join(parts, ", ")
}

func helpText() string {
	return strings.Join([]string{
		"available commands:",
		"monitor",
		"positions",
		"trades",
		"observe <symbol>",
		"schedule on|off",
		"latest <symbol>",
		"news",
	}, "\n")
}

func ptrValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func resolveDuration(value, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return value
}

func resolveMode(mode string) string {
	v := strings.TrimSpace(strings.ToLower(mode))
	if v == "" {
		return ModeLongConnection
	}
	return v
}

type feishuMessenger struct {
	client *lark.Client
}

func (m *feishuMessenger) SendText(ctx context.Context, chatID string, text string) error {
	content, err := buildMessageTextContent(text)
	if err != nil {
		return err
	}
	resp, err := m.client.Im.Message.Create(
		ctx,
		larkim.NewCreateMessageReqBuilder().
			ReceiveIdType(larkim.ReceiveIdTypeChatId).
			Body(larkim.NewCreateMessageReqBodyBuilder().
				ReceiveId(chatID).
				MsgType(larkim.MsgTypeText).
				Content(content).
				Build()).
			Build(),
	)
	if err != nil {
		return err
	}
	if resp == nil {
		return errors.New("feishu messenger empty response")
	}
	if !resp.Success() {
		return fmt.Errorf("feishu messenger failed: code=%d msg=%s", resp.Code, strings.TrimSpace(resp.Msg))
	}
	return nil
}

func (m *feishuMessenger) SendCard(ctx context.Context, chatID, title, markdown string) error {
	content, err := buildMessageCardContent(title, markdown)
	if err != nil {
		return err
	}
	resp, err := m.client.Im.Message.Create(
		ctx,
		larkim.NewCreateMessageReqBuilder().
			ReceiveIdType(larkim.ReceiveIdTypeChatId).
			Body(larkim.NewCreateMessageReqBodyBuilder().
				ReceiveId(chatID).
				MsgType("interactive").
				Content(content).
				Build()).
			Build(),
	)
	if err != nil {
		return err
	}
	if resp == nil {
		return errors.New("feishu messenger empty response")
	}
	if !resp.Success() {
		return fmt.Errorf("feishu messenger failed: code=%d msg=%s", resp.Code, strings.TrimSpace(resp.Msg))
	}
	return nil
}

func buildMessageTextContent(text string) (string, error) {
	raw, err := json.Marshal(contentText{Text: text})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func buildMessageCardContent(title, markdown string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "通知"
	}
	markdown = strings.TrimSpace(markdown)
	if markdown == "" {
		markdown = "通知"
	}
	card := cardContent{
		Config: cardConfig{WideScreenMode: true},
		Header: cardHeader{Title: cardTitle{Tag: "plain_text", Content: title}, Template: "blue"},
		Elements: []cardElement{{
			Tag:  "div",
			Text: &cardText{Tag: "lark_md", Content: markdown},
		}},
	}
	raw, err := json.Marshal(card)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
