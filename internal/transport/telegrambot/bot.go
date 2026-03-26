package telegrambot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"brale-core/internal/cardimage"
	"brale-core/internal/pkg/httpclient"
	symbolpkg "brale-core/internal/pkg/symbol"
	"brale-core/internal/transport/botruntime"

	"go.uber.org/zap"
)

const (
	defaultAPIBASE        = "https://api.telegram.org"
	defaultPollTimeout    = 30 * time.Second
	defaultUpdateLimit    = 50
	defaultSessionTTL     = 5 * time.Minute
	defaultRequestTimeout = 90 * time.Second
	defaultLockPath       = "/tmp/brale-core-telegrambot.lock"
	tradeHistoryMenuLimit = 10

	cbMenuMonitor   = "menu_monitor"
	cbMenuPositions = "menu_positions"
	cbMenuTrades    = "menu_trades"
	cbMenuObserve   = "menu_observe"
	cbMenuToggle    = "menu_toggle"
	cbMenuLatest    = "menu_latest"
	cbToggleOn      = "toggle_on"
	cbToggleOff     = "toggle_off"
	cbMenuCancel    = "menu_cancel"
	cbObservePrefix = "observe:"
	cbObserveManual = "observe_manual"
	cbLatestPrefix  = "latest:"
)

type Bot struct {
	apiBase        string
	token          string
	runtimeBase    string
	runtimeClient  *botruntime.Client
	client         *http.Client
	logger         *zap.Logger
	sessions       *sessionStore
	pollTimeout    time.Duration
	updateLimit    int
	requestTimeout time.Duration
	lockPath       string
	lockFile       *os.File
}

func New(cfg Config, logger *zap.Logger) (*Bot, error) {
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, errors.New("telegram token is required")
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	pollTimeout := cfg.PollTimeout
	if pollTimeout <= 0 {
		pollTimeout = defaultPollTimeout
	}
	updateLimit := cfg.UpdateLimit
	if updateLimit <= 0 {
		updateLimit = defaultUpdateLimit
	}
	requestTimeout := cfg.RequestTimeout
	if requestTimeout <= 0 {
		requestTimeout = defaultRequestTimeout
	}
	sessionTTL := cfg.SessionTTL
	if sessionTTL <= 0 {
		sessionTTL = defaultSessionTTL
	}
	runtimeBase := strings.TrimRight(strings.TrimSpace(cfg.RuntimeBaseURL), "/")
	if runtimeBase == "" {
		return nil, errors.New("runtime base url is required")
	}
	httpClient := &http.Client{Timeout: requestTimeout}
	runtimeClient, err := botruntime.NewClient(runtimeBase, httpClient)
	if err != nil {
		return nil, err
	}
	lockPath := strings.TrimSpace(cfg.LockPath)
	if lockPath == "" {
		lockPath = defaultLockPath
	}

	return &Bot{
		apiBase:        defaultAPIBASE,
		token:          strings.TrimSpace(cfg.Token),
		runtimeBase:    runtimeBase,
		runtimeClient:  runtimeClient,
		client:         httpClient,
		logger:         logger,
		sessions:       newSessionStore(sessionTTL),
		pollTimeout:    pollTimeout,
		updateLimit:    updateLimit,
		requestTimeout: requestTimeout,
		lockPath:       lockPath,
	}, nil
}

func (b *Bot) Run(ctx context.Context) error {
	if b == nil {
		return errors.New("bot is nil")
	}
	if err := b.acquireLock(); err != nil {
		return err
	}
	defer b.releaseLock()
	offset := 0
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		updates, err := b.getUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			b.logger.Warn("telegram getUpdates failed", zap.Error(err))
			time.Sleep(2 * time.Second)
			continue
		}
		for _, upd := range updates {
			if upd.UpdateID >= offset {
				offset = upd.UpdateID + 1
			}
			b.handleUpdate(ctx, upd)
		}
	}
}

func (b *Bot) acquireLock() error {
	if strings.TrimSpace(b.lockPath) == "" {
		return nil
	}
	file, err := os.OpenFile(b.lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return errors.New("telegram bot already running")
		}
		return err
	}
	b.lockFile = file
	return nil
}

func (b *Bot) releaseLock() {
	if b.lockFile == nil {
		return
	}
	_ = syscall.Flock(int(b.lockFile.Fd()), syscall.LOCK_UN)
	_ = b.lockFile.Close()
	b.lockFile = nil
}

func (b *Bot) handleUpdate(ctx context.Context, upd update) {
	if upd.Message != nil {
		b.handleMessage(ctx, upd.Message)
		return
	}
	if upd.CallbackQuery != nil {
		b.handleCallback(ctx, upd.CallbackQuery)
		return
	}
}

func (b *Bot) handleMessage(ctx context.Context, msg *message) {
	if msg == nil || msg.From == nil || msg.Chat == nil {
		return
	}
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}
	chatID := msg.Chat.ID
	userID := msg.From.ID

	if strings.HasPrefix(text, "/cancel") {
		b.sessions.delete(chatID, userID)
		b.sendText(ctx, chatID, "已取消当前会话。")
		b.sendMainMenu(ctx, chatID)
		return
	}
	if strings.HasPrefix(text, "/start") || strings.HasPrefix(text, "/menu") {
		b.sendMainMenu(ctx, chatID)
		return
	}

	sess, ok := b.sessions.get(chatID, userID)
	if ok && sess.Step == stepAwaitSymbol {
		symbol := normalizeSymbol(text)
		if symbol == "" {
			b.sendTextWithReply(ctx, chatID, "币种不能为空，请重新输入。")
			return
		}
		b.sessions.delete(chatID, userID)
		b.sendText(ctx, chatID, "开始执行单轮决策...")
		go b.executeObserveFlat(ctx, chatID, symbol)
		return
	}

	b.sendMainMenu(ctx, chatID)
}

func (b *Bot) handleCallback(ctx context.Context, cb *callbackQuery) {
	if cb == nil || cb.From == nil || cb.Message == nil || cb.Message.Chat == nil {
		return
	}
	chatID := cb.Message.Chat.ID
	userID := cb.From.ID
	data := strings.TrimSpace(cb.Data)
	b.answerCallback(ctx, cb.ID)

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
		sess := &session{ChatID: chatID, UserID: userID, Step: stepAwaitSymbol}
		b.sessions.save(sess)
		b.sendTextWithReply(ctx, chatID, "请输入币种（如 ETH 或 ETHUSDT）：")
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
		symbol := normalizeSymbol(strings.TrimPrefix(data, cbObservePrefix))
		if symbol == "" {
			b.sendText(ctx, chatID, "币种不能为空，请重新选择。")
			return
		}
		b.sessions.delete(chatID, userID)
		b.sendText(ctx, chatID, "开始执行单轮决策...")
		go b.executeObserveFlat(ctx, chatID, symbol)
	case strings.HasPrefix(data, cbLatestPrefix):
		symbol := normalizeSymbol(strings.TrimPrefix(data, cbLatestPrefix))
		if symbol == "" {
			b.sendText(ctx, chatID, "币种不能为空，请重新选择。")
			return
		}
		b.handleDecisionLatest(ctx, chatID, symbol)
	default:
		b.sendText(ctx, chatID, "未知操作，请使用菜单按钮。")
	}
}

func (b *Bot) runObserve(ctx context.Context, req ObserveRequest) (ObserveResponse, error) {
	out, err := b.runtimeClient.RunObserve(ctx, botruntime.ObserveRunRequest(req))
	if err != nil {
		return ObserveResponse{}, err
	}
	if hasObserveReport(out) && strings.EqualFold(strings.TrimSpace(out.Status), "ok") {
		return out, nil
	}
	if out.Symbol == "" {
		out.Symbol = req.Symbol
	}
	if fallback, ok := b.waitObserveReport(ctx, out.Symbol, out.RequestID); ok {
		return fallback, nil
	}
	return out, nil
}

func (b *Bot) sendText(ctx context.Context, chatID int64, text string) {
	_, _ = b.sendMessage(ctx, chatID, text, "", nil)
}

func (b *Bot) sendTextWithReply(ctx context.Context, chatID int64, text string) {
	markup := forceReply{ForceReply: true}
	_, _ = b.sendMessage(ctx, chatID, text, "", markup)
}

func (b *Bot) sendInline(ctx context.Context, chatID int64, text string, keyboard inlineKeyboard) {
	markup := keyboard.toMarkup()
	_, _ = b.sendMessage(ctx, chatID, text, "", markup)
}

func (b *Bot) sendMessage(ctx context.Context, chatID int64, text string, parseMode string, replyMarkup any) (int, error) {
	payload := sendMessageRequest{ChatID: chatID, Text: text, ParseMode: parseMode, ReplyMarkup: replyMarkup}
	var resp sendMessageResponse
	if err := b.doTelegramRequest(ctx, http.MethodPost, "sendMessage", payload, &resp); err != nil {
		return 0, err
	}
	if !resp.OK {
		return 0, fmt.Errorf("telegram send failed: %s", resp.Description)
	}
	if resp.Result == nil {
		return 0, nil
	}
	return resp.Result.MessageID, nil
}

func (b *Bot) sendImage(ctx context.Context, chatID int64, asset *cardimage.ImageAsset) error {
	if asset == nil || len(asset.Data) == 0 {
		return errors.New("telegram bot image payload is empty")
	}
	endpoint := fmt.Sprintf("%s/bot%s/sendDocument", b.apiBase, b.token)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("chat_id", strconv.FormatInt(chatID, 10)); err != nil {
		return err
	}
	name := strings.TrimSpace(asset.Filename)
	if name == "" {
		name = "decision.png"
	}
	part, err := writer.CreateFormFile("document", filepath.Base(name))
	if err != nil {
		return err
	}
	if _, err := part.Write(asset.Data); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			return
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := httpclient.ReadLimitedBody(resp.Body, 2048)
		return fmt.Errorf("telegram image send failed: %s", strings.TrimSpace(string(bodyBytes)))
	}
	return nil
}

func (b *Bot) sendMainMenu(ctx context.Context, chatID int64) {
	keyboard := inlineKeyboard{Buttons: [][]inlineButton{
		{
			{Text: "监控列表", CallbackData: cbMenuMonitor},
			{Text: "当前持仓", CallbackData: cbMenuPositions},
		},
		{
			{Text: "历史仓位", CallbackData: cbMenuTrades},
			{Text: "观察分析", CallbackData: cbMenuObserve},
		},
		{
			{Text: "决策开关", CallbackData: cbMenuToggle},
			{Text: "最近决策", CallbackData: cbMenuLatest},
		},
	}}
	b.sendInline(ctx, chatID, "请选择功能：", keyboard)
}

func (b *Bot) sendToggleMenu(ctx context.Context, chatID int64) {
	b.sendInline(ctx, chatID, "请选择操作：", inlineKeyboard{Buttons: [][]inlineButton{{
		{Text: "开启定时", CallbackData: cbToggleOn},
		{Text: "关闭定时", CallbackData: cbToggleOff},
	}}})
}

func (b *Bot) handleMonitorMenu(ctx context.Context, chatID int64) {
	resp, err := b.fetchMonitorStatus(ctx)
	if err != nil {
		b.sendText(ctx, chatID, fmt.Sprintf("监控列表获取失败：%s", err.Error()))
		return
	}
	b.sendText(ctx, chatID, formatMonitorStatus(resp))
}

func (b *Bot) handleObserveMenu(ctx context.Context, chatID int64) {
	resp, err := b.fetchMonitorStatus(ctx)
	if err != nil {
		b.sendText(ctx, chatID, fmt.Sprintf("观察分析币种获取失败：%s", err.Error()))
		return
	}
	b.sendObserveOptionsFromMonitor(ctx, chatID, resp.Symbols)
}

func (b *Bot) sendObserveOptionsFromMonitor(ctx context.Context, chatID int64, symbols []MonitorSymbolConfig) {
	if len(symbols) == 0 {
		return
	}
	buttons := make([][]inlineButton, 0, len(symbols)/2+2)
	row := make([]inlineButton, 0, 2)
	added := 0
	for _, item := range symbols {
		symbol := normalizeSymbol(item.Symbol)
		if symbol == "" {
			continue
		}
		row = append(row, inlineButton{Text: "观察 " + symbol, CallbackData: cbObservePrefix + symbol})
		added++
		if added%2 == 0 {
			buttons = append(buttons, row)
			row = make([]inlineButton, 0, 2)
		}
	}
	if len(row) > 0 {
		buttons = append(buttons, row)
	}
	buttons = append(buttons, []inlineButton{{Text: "手动输入币种", CallbackData: cbObserveManual}})
	b.sendInline(ctx, chatID, "请选择观察分析币种：", inlineKeyboard{Buttons: buttons})
}

func (b *Bot) handlePositionsMenu(ctx context.Context, chatID int64) {
	resp, err := b.fetchPositionStatus(ctx)
	if err != nil {
		b.sendText(ctx, chatID, fmt.Sprintf("持仓获取失败：%s", err.Error()))
		return
	}
	b.sendText(ctx, chatID, formatPositions(resp.Positions))
}

func (b *Bot) handleTradesMenu(ctx context.Context, chatID int64) {
	resp, err := b.fetchTradeHistory(ctx)
	if err != nil {
		b.sendText(ctx, chatID, fmt.Sprintf("历史仓位获取失败：%s", err.Error()))
		return
	}
	b.sendText(ctx, chatID, formatTradeHistory(latestTradeHistory(resp.Trades, tradeHistoryMenuLimit)))
}

func (b *Bot) handleLatestMenu(ctx context.Context, chatID int64) {
	resp, err := b.fetchMonitorStatus(ctx)
	if err != nil {
		b.sendText(ctx, chatID, fmt.Sprintf("监控币种获取失败：%s", err.Error()))
		return
	}
	if len(resp.Symbols) == 0 {
		b.sendText(ctx, chatID, "暂无监控币种。")
		return
	}
	buttons := make([][]inlineButton, 0, len(resp.Symbols))
	row := make([]inlineButton, 0, 3)
	for i, sym := range resp.Symbols {
		row = append(row, inlineButton{Text: sym.Symbol, CallbackData: cbLatestPrefix + sym.Symbol})
		if (i+1)%3 == 0 {
			buttons = append(buttons, row)
			row = make([]inlineButton, 0, 3)
		}
	}
	if len(row) > 0 {
		buttons = append(buttons, row)
	}
	b.sendInline(ctx, chatID, "请选择币种：", inlineKeyboard{Buttons: buttons})
}
func (b *Bot) handleDecisionLatest(ctx context.Context, chatID int64, symbol string) {
	b.sendText(ctx, chatID, "正在查询...")
	resp, err := b.fetchDecisionLatest(ctx, symbol)
	if err != nil {
		b.sendText(ctx, chatID, fmt.Sprintf("最近决策获取失败：%s", err.Error()))
		return
	}
	if strings.TrimSpace(resp.Summary) == "查询不存在" && len(resp.Agent) == 0 && len(resp.Gate) == 0 {
		b.sendText(ctx, chatID, "查询不存在")
		return
	}
	asset, err := cardimage.NewOGRenderer().RenderRuntimePayload(ctx, resp.Symbol, resp.SnapshotID, resp.Gate, resp.Agent, "Decision Snapshot")
	if err != nil {
		b.logger.Warn("telegram latest image render failed", zap.String("symbol", symbol), zap.Error(err))
		b.sendText(ctx, chatID, fmt.Sprintf("最近决策图片生成失败：%s", err.Error()))
		return
	}
	if err := b.sendImage(ctx, chatID, asset); err != nil {
		b.logger.Warn("telegram latest image send failed", zap.String("symbol", symbol), zap.Error(err))
		b.sendText(ctx, chatID, fmt.Sprintf("最近决策图片发送失败：%s", err.Error()))
		return
	}
}

func (b *Bot) toggleSchedule(ctx context.Context, chatID int64, enable bool) {
	resp, err := b.postScheduleToggle(ctx, enable)
	if err != nil {
		b.sendText(ctx, chatID, fmt.Sprintf("定时切换失败：%s", err.Error()))
		return
	}
	b.sendText(ctx, chatID, formatScheduleResponse(resp))
}

func (b *Bot) executeObserveFlat(parent context.Context, chatID int64, symbol string) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, b.requestTimeout)
	defer cancel()
	req := ObserveRequest{Symbol: symbol}
	resp, err := b.runObserve(ctx, req)
	if err != nil {
		b.sendText(ctx, chatID, fmt.Sprintf("单轮决策失败：%s", err.Error()))
		return
	}
	b.sendObserveResponse(ctx, chatID, resp)
}

func (b *Bot) sendObserveResponse(ctx context.Context, chatID int64, resp ObserveResponse) {
	if len(resp.Agent) == 0 || len(resp.Gate) == 0 {
		text := strings.TrimSpace(resp.Summary)
		if text == "" {
			text = "观察结果缺少渲染数据"
		}
		b.sendText(ctx, chatID, text)
		return
	}
	asset, err := cardimage.NewOGRenderer().RenderRuntimePayload(ctx, resp.Symbol, 0, resp.Gate, resp.Agent, "Observe Snapshot")
	if err != nil {
		b.logger.Warn("telegram observe image render failed", zap.String("symbol", resp.Symbol), zap.Error(err))
		b.sendText(ctx, chatID, fmt.Sprintf("观察图片生成失败：%s", err.Error()))
		return
	}
	if err := b.sendImage(ctx, chatID, asset); err != nil {
		b.logger.Warn("telegram observe image send failed", zap.String("symbol", resp.Symbol), zap.Error(err))
		b.sendText(ctx, chatID, fmt.Sprintf("观察图片发送失败：%s", err.Error()))
		return
	}
}

func (b *Bot) waitObserveReport(ctx context.Context, symbol, requestID string) (ObserveResponse, bool) {
	if strings.TrimSpace(symbol) == "" {
		return ObserveResponse{}, false
	}
	if ctx == nil {
		return ObserveResponse{}, false
	}
	deadline, hasDeadline := ctx.Deadline()
	interval := 2 * time.Second
	if interval <= 0 {
		interval = time.Second
	}
	for {
		if hasDeadline && time.Now().After(deadline) {
			return ObserveResponse{}, false
		}
		select {
		case <-ctx.Done():
			return ObserveResponse{}, false
		case <-time.After(interval):
		}
		resp, err := b.fetchObserveReport(ctx, symbol)
		if err != nil {
			continue
		}
		if requestID != "" && resp.RequestID != "" && resp.RequestID != requestID {
			continue
		}
		if hasObserveReport(resp) || strings.EqualFold(strings.TrimSpace(resp.Status), "ok") {
			return resp, true
		}
	}
}

func hasObserveReport(resp ObserveResponse) bool {
	return len(resp.Agent) > 0 || len(resp.Gate) > 0 || strings.TrimSpace(resp.ReportHTML) != "" || strings.TrimSpace(resp.ReportMarkdown) != "" || strings.TrimSpace(resp.Report) != ""
}

func (b *Bot) answerCallback(ctx context.Context, id string) {
	if strings.TrimSpace(id) == "" {
		return
	}
	payload := answerCallbackRequest{CallbackQueryID: id}
	var resp baseResponse
	_ = b.doTelegramRequest(ctx, http.MethodPost, "answerCallbackQuery", payload, &resp)
}

func (b *Bot) getUpdates(ctx context.Context, offset int) ([]update, error) {
	vals := url.Values{}
	vals.Set("timeout", strconv.Itoa(int(b.pollTimeout.Seconds())))
	vals.Set("offset", strconv.Itoa(offset))
	vals.Set("limit", strconv.Itoa(b.updateLimit))
	path := "getUpdates?" + vals.Encode()
	var resp updatesResponse
	if err := b.doTelegramRequest(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("telegram getUpdates failed: %s", resp.Description)
	}
	return resp.Result, nil
}

func (b *Bot) doTelegramRequest(ctx context.Context, method, path string, payload any, out any) error {
	url := fmt.Sprintf("%s/bot%s/%s", b.apiBase, b.token, path)
	req, err := httpclient.NewJSONRequest(ctx, method, url, payload)
	if err != nil {
		return err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			return
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := httpclient.ReadLimitedBody(resp.Body, 2048)
		bodyText := strings.TrimSpace(string(bodyBytes))
		if bodyText == "" {
			return fmt.Errorf("telegram status %s", resp.Status)
		}
		return fmt.Errorf("telegram status %s: %s", resp.Status, bodyText)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func normalizeSymbol(symbol string) string {
	return symbolpkg.Normalize(symbol)
}
