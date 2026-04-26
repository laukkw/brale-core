package notify

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"brale-core/internal/cardimage"
	"brale-core/internal/decision/decisionfmt"

	"go.uber.org/zap"
)

type countSender struct {
	mu      sync.Mutex
	calls   int
	lastMsg Message
	msgs    []Message
}

func (s *countSender) Send(ctx context.Context, msg Message) error {
	s.mu.Lock()
	s.calls++
	s.lastMsg = msg
	s.msgs = append(s.msgs, msg)
	s.mu.Unlock()
	return nil
}

func (s *countSender) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *countSender) lastMessage() Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastMsg
}

func (s *countSender) messages() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Message, len(s.msgs))
	copy(out, s.msgs)
	return out
}

type flakySender struct {
	mu    sync.Mutex
	calls int
}

func (s *flakySender) Send(ctx context.Context, msg Message) error {
	s.mu.Lock()
	s.calls++
	calls := s.calls
	s.mu.Unlock()
	if calls == 1 {
		return context.DeadlineExceeded
	}
	return nil
}

func (s *flakySender) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

type staticRenderer struct{}

func (staticRenderer) RenderDecision(ctx context.Context, input decisionfmt.DecisionInput, report decisionfmt.DecisionReport) (*cardimage.ImageAsset, error) {
	return &cardimage.ImageAsset{Data: []byte("png"), Filename: "test.png", ContentType: "image/png", Caption: report.Symbol}, nil
}

type blockingSender struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
}

func (s *blockingSender) Send(ctx context.Context, msg Message) error {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	select {
	case s.started <- struct{}{}:
	default:
	}
	<-s.release
	return nil
}

func (s *blockingSender) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

type alwaysFailSender struct {
	calls int
}

func (s *alwaysFailSender) Send(ctx context.Context, msg Message) error {
	s.calls++
	return errors.New("channel down")
}

type namedCountSender struct {
	mu      sync.Mutex
	channel string
	calls   int
	lastMsg Message
}

func (s *namedCountSender) Channel() string { return s.channel }

func (s *namedCountSender) Send(_ context.Context, msg Message) error {
	s.mu.Lock()
	s.calls++
	s.lastMsg = msg
	s.mu.Unlock()
	return nil
}

func (s *namedCountSender) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *namedCountSender) lastMessage() Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastMsg
}

type asyncFallbackNotifier struct {
	errors int
}

func (n *asyncFallbackNotifier) SendGate(context.Context, decisionfmt.DecisionInput, decisionfmt.DecisionReport) error {
	return nil
}
func (n *asyncFallbackNotifier) SendStartup(context.Context, StartupInfo) error   { return nil }
func (n *asyncFallbackNotifier) SendShutdown(context.Context, ShutdownInfo) error { return nil }
func (n *asyncFallbackNotifier) SendError(context.Context, ErrorNotice) error {
	n.errors++
	return nil
}
func (n *asyncFallbackNotifier) SendPositionOpen(context.Context, PositionOpenNotice) error {
	return nil
}
func (n *asyncFallbackNotifier) SendPositionClose(context.Context, PositionCloseNotice) error {
	return nil
}
func (n *asyncFallbackNotifier) SendPositionCloseSummary(context.Context, PositionCloseSummaryNotice) error {
	return nil
}
func (n *asyncFallbackNotifier) SendRiskPlanUpdate(context.Context, RiskPlanUpdateNotice) error {
	return nil
}
func (n *asyncFallbackNotifier) SendTradeOpen(context.Context, TradeOpenNotice) error { return nil }
func (n *asyncFallbackNotifier) SendTradePartialClose(context.Context, TradePartialCloseNotice) error {
	return nil
}
func (n *asyncFallbackNotifier) SendTradeCloseSummary(context.Context, TradeCloseSummaryNotice) error {
	return nil
}

type countingRenderer struct {
	mu    sync.Mutex
	calls int
}

func (r *countingRenderer) RenderDecision(ctx context.Context, input decisionfmt.DecisionInput, report decisionfmt.DecisionReport) (*cardimage.ImageAsset, error) {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	return &cardimage.ImageAsset{Data: []byte("png"), Filename: "decision.png", ContentType: "image/png", Caption: report.Symbol}, nil
}

func (r *countingRenderer) renderCallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func waitForCondition(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met within %v", timeout)
}

func TestSendGate_DedupBySnapshot(t *testing.T) {
	sender := &countSender{}
	mgr := Manager{renderer: staticRenderer{}, senders: []Sender{sender}, dedupe: newDedupeGuard(2 * time.Minute)}
	report := decisionfmt.DecisionReport{
		Symbol:     "ETHUSDT",
		SnapshotID: 123456,
		Gate: decisionfmt.GateReport{Overall: decisionfmt.GateOverall{
			DecisionAction: "VETO",
			ReasonCode:     "CONSENSUS_NOT_PASSED",
		}},
	}

	if err := mgr.SendGate(context.Background(), decisionfmt.DecisionInput{Symbol: report.Symbol, SnapshotID: report.SnapshotID}, report); err != nil {
		t.Fatalf("first send failed: %v", err)
	}
	if err := mgr.SendGate(context.Background(), decisionfmt.DecisionInput{Symbol: report.Symbol, SnapshotID: report.SnapshotID}, report); err != nil {
		t.Fatalf("second send failed: %v", err)
	}
	if sender.callCount() != 1 {
		t.Fatalf("expected deduped single send, got %d", sender.callCount())
	}
}

func TestSendGate_DifferentSnapshotNotDeduped(t *testing.T) {
	sender := &countSender{}
	mgr := Manager{renderer: staticRenderer{}, senders: []Sender{sender}, dedupe: newDedupeGuard(2 * time.Minute)}
	reportA := decisionfmt.DecisionReport{
		Symbol:     "ETHUSDT",
		SnapshotID: 100,
		Gate: decisionfmt.GateReport{Overall: decisionfmt.GateOverall{
			DecisionAction: "VETO",
			ReasonCode:     "CONSENSUS_NOT_PASSED",
		}},
	}
	reportB := reportA
	reportB.SnapshotID = 101

	if err := mgr.SendGate(context.Background(), decisionfmt.DecisionInput{Symbol: reportA.Symbol, SnapshotID: reportA.SnapshotID}, reportA); err != nil {
		t.Fatalf("send A failed: %v", err)
	}
	if err := mgr.SendGate(context.Background(), decisionfmt.DecisionInput{Symbol: reportB.Symbol, SnapshotID: reportB.SnapshotID}, reportB); err != nil {
		t.Fatalf("send B failed: %v", err)
	}
	if sender.callCount() != 2 {
		t.Fatalf("expected two sends for different snapshots, got %d", sender.callCount())
	}
}

func TestSendGate_AllowPlanSendsPlanTextAndDedupes(t *testing.T) {
	sender := &countSender{}
	mgr := Manager{renderer: staticRenderer{}, senders: []Sender{sender}, dedupe: newDedupeGuard(2 * time.Minute)}
	report := decisionfmt.DecisionReport{
		Symbol:     "SOLUSDT",
		SnapshotID: 1776513610,
		Gate: decisionfmt.GateReport{
			Overall: decisionfmt.GateOverall{
				DecisionAction: "ALLOW",
				Direction:      "short",
			},
			Derived: map[string]any{
				"current_price": 86.74,
				"plan": map[string]any{
					"direction":    "short",
					"entry":        86.50,
					"stop_loss":    87.30,
					"take_profits": []any{85.80, 85.10, 84.40},
				},
			},
		},
	}
	input := decisionfmt.DecisionInput{Symbol: report.Symbol, SnapshotID: report.SnapshotID}

	if err := mgr.SendGate(context.Background(), input, report); err != nil {
		t.Fatalf("first send failed: %v", err)
	}
	if err := mgr.SendGate(context.Background(), input, report); err != nil {
		t.Fatalf("second send failed: %v", err)
	}
	if sender.callCount() != 2 {
		t.Fatalf("expected one image send and one plan text send, got %d", sender.callCount())
	}

	msgs := sender.messages()
	if len(msgs) != 2 {
		t.Fatalf("expected two outbound messages, got %d", len(msgs))
	}
	if msgs[0].Image == nil {
		t.Fatal("expected first gate notification to be the image card")
	}
	if msgs[1].Image != nil {
		t.Fatal("expected second gate notification to be text only")
	}
	body := msgs[1].Markdown
	if !strings.Contains(body, "📋 开仓计划已生成") {
		t.Fatalf("expected plan header, got %q", body)
	}
	if !strings.Contains(body, "▸ 币种：SOLUSDT") {
		t.Fatalf("expected symbol line, got %q", body)
	}
	if !strings.Contains(body, noticeLine("direction", "SHORT")) {
		t.Fatalf("expected direction line, got %q", body)
	}
	if !strings.Contains(body, "▸ 当前价格：86.74") {
		t.Fatalf("expected current price line, got %q", body)
	}
	if !strings.Contains(body, "▸ 开仓价：86.5") {
		t.Fatalf("expected entry line, got %q", body)
	}
	if !strings.Contains(body, noticeLine("stop", "87.3")) {
		t.Fatalf("expected stop line, got %q", body)
	}
	if !strings.Contains(body, noticeLine("take_profits", "TP1 85.8 / TP2 85.1 / TP3 84.4")) {
		t.Fatalf("expected take profits line, got %q", body)
	}
	if !strings.Contains(body, "▸ 说明：当前仅为计划，不代表已经成交") {
		t.Fatalf("expected plan disclaimer, got %q", body)
	}
	if !strings.Contains(body, "▸ 触发条件：只有当价格到达开仓触发点后，系统才会提交开仓动作") {
		t.Fatalf("expected trigger condition line, got %q", body)
	}
}

func TestSendGate_AllowWithoutPlanOnlySendsGateCard(t *testing.T) {
	sender := &countSender{}
	mgr := Manager{renderer: staticRenderer{}, senders: []Sender{sender}, dedupe: newDedupeGuard(2 * time.Minute)}
	report := decisionfmt.DecisionReport{
		Symbol:     "BTCUSDT",
		SnapshotID: 20260418,
		Gate: decisionfmt.GateReport{
			Overall: decisionfmt.GateOverall{
				DecisionAction: "ALLOW",
				Direction:      "long",
			},
			Derived: map[string]any{
				"current_price": 85000.0,
			},
		},
	}

	if err := mgr.SendGate(context.Background(), decisionfmt.DecisionInput{Symbol: report.Symbol, SnapshotID: report.SnapshotID}, report); err != nil {
		t.Fatalf("send gate failed: %v", err)
	}
	if sender.callCount() != 1 {
		t.Fatalf("expected only the gate card when plan is missing, got %d sends", sender.callCount())
	}
	if sender.lastMessage().Image == nil {
		t.Fatal("expected gate card image when plan text is skipped")
	}
}

func TestSendGate_FirstFailureDoesNotPoisonDedupe(t *testing.T) {
	sender := &flakySender{}
	mgr := Manager{renderer: staticRenderer{}, senders: []Sender{sender}, dedupe: newDedupeGuard(2 * time.Minute)}
	report := decisionfmt.DecisionReport{
		Symbol:     "ETHUSDT",
		SnapshotID: 2026,
		Gate: decisionfmt.GateReport{Overall: decisionfmt.GateOverall{
			DecisionAction: "VETO",
			ReasonCode:     "CONSENSUS_NOT_PASSED",
		}},
	}

	if err := mgr.SendGate(context.Background(), decisionfmt.DecisionInput{Symbol: report.Symbol, SnapshotID: report.SnapshotID}, report); err == nil {
		t.Fatalf("expected first send error")
	}
	if err := mgr.SendGate(context.Background(), decisionfmt.DecisionInput{Symbol: report.Symbol, SnapshotID: report.SnapshotID}, report); err != nil {
		t.Fatalf("expected second send success, got %v", err)
	}
	if sender.callCount() != 2 {
		t.Fatalf("expected retry to send again, got %d calls", sender.callCount())
	}
}

func TestSendWithKey_ConcurrentDuplicateOnlySendsOnce(t *testing.T) {
	sender := &blockingSender{
		started: make(chan struct{}, 2),
		release: make(chan struct{}),
	}
	mgr := Manager{senders: []Sender{sender}, dedupe: newDedupeGuard(2 * time.Minute)}

	start := make(chan struct{})
	errCh := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			errCh <- mgr.sendWithKey(context.Background(), Message{Title: "dup"}, "same-key")
		}()
	}
	close(start)

	select {
	case <-sender.started:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected first sender call to start")
	}
	time.Sleep(50 * time.Millisecond)
	if got := sender.callCount(); got != 1 {
		t.Fatalf("expected one in-flight send, got %d", got)
	}

	close(sender.release)
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("unexpected send error: %v", err)
		}
	}
}

func TestSendWithKey_PartialFailureCommitsDedupeAfterSuccess(t *testing.T) {
	okSender := &countSender{}
	failSender := &alwaysFailSender{}
	mgr := Manager{senders: []Sender{okSender, failSender}, dedupe: newDedupeGuard(2 * time.Minute)}

	if err := mgr.sendWithKey(context.Background(), Message{Title: "partial"}, "partial-key"); err != nil {
		t.Fatalf("expected partial success to return nil, got %v", err)
	}
	if err := mgr.sendWithKey(context.Background(), Message{Title: "partial"}, "partial-key"); err != nil {
		t.Fatalf("expected deduped second send to be skipped, got %v", err)
	}
	if okSender.callCount() != 1 {
		t.Fatalf("expected successful sender not to be retried, got %d calls", okSender.callCount())
	}
	if failSender.calls != 1 {
		t.Fatalf("expected failed sender not to be retried after dedupe commit, got %d calls", failSender.calls)
	}
}

func TestBuildNotifyDeliverArgsSplitsByChannel(t *testing.T) {
	t.Parallel()

	rendered := json.RawMessage(`{"message":"ok"}`)
	args := buildNotifyDeliverArgs("error", "ethusdt", rendered, []string{"telegram", "feishu"})
	if len(args) != 2 {
		t.Fatalf("args len=%d want 2", len(args))
	}
	for _, arg := range args {
		if arg.Channel == "" {
			t.Fatalf("channel is empty: %#v", arg)
		}
		if !strings.HasSuffix(arg.DedupeKey, ":"+arg.Channel) {
			t.Fatalf("dedupe key %q does not include channel %q", arg.DedupeKey, arg.Channel)
		}
	}
	if args[0].DedupeKey == args[1].DedupeKey {
		t.Fatalf("dedupe keys should differ per channel: %q", args[0].DedupeKey)
	}
}

func TestAsyncManagerFallsBackToSyncBeforeRiverClient(t *testing.T) {
	t.Parallel()

	syncNotifier := &asyncFallbackNotifier{}
	manager := NewAsyncManager(nil, syncNotifier, zap.NewNop())

	err := manager.SendError(context.Background(), ErrorNotice{
		Severity:  "error",
		Component: "reconcile",
		Symbol:    "ETHUSDT",
		Message:   "startup reconcile alert",
	})
	if err != nil {
		t.Fatalf("SendError() error=%v", err)
	}
	if syncNotifier.errors != 1 {
		t.Fatalf("sync errors=%d want 1", syncNotifier.errors)
	}
}

func TestNotifyRenderInsertOptsDelaysLifecycleEvents(t *testing.T) {
	t.Parallel()

	before := time.Now()
	opts := notifyRenderInsertOpts(asyncEventPositionOpen)
	if opts == nil {
		t.Fatalf("expected lifecycle event to have insert opts")
	}
	if opts.ScheduledAt.Before(before.Add(tradeLifecycleNotifyDelay - time.Second)) {
		t.Fatalf("scheduled_at=%s, want delayed by about %s", opts.ScheduledAt, tradeLifecycleNotifyDelay)
	}
	if opts.ScheduledAt.After(before.Add(tradeLifecycleNotifyDelay + time.Second)) {
		t.Fatalf("scheduled_at=%s, want delayed by about %s", opts.ScheduledAt, tradeLifecycleNotifyDelay)
	}
	if opts.UniqueOpts.ByPeriod != 2*time.Minute || !opts.UniqueOpts.ByArgs {
		t.Fatalf("unique opts not preserved: %#v", opts.UniqueOpts)
	}
}

func TestNotifyRenderInsertOptsDoesNotDelayGate(t *testing.T) {
	t.Parallel()

	if opts := notifyRenderInsertOpts(asyncEventGate); opts != nil {
		t.Fatalf("gate opts=%#v, want nil", opts)
	}
}

func TestAsyncDeliverRoutesToSingleChannel(t *testing.T) {
	t.Parallel()

	telegram := &namedCountSender{channel: "telegram"}
	feishu := &namedCountSender{channel: "feishu"}
	syncManager := Manager{senders: []Sender{telegram, feishu}, dedupe: newDedupeGuard(defaultNotifyDedupeTTL)}
	manager := NewAsyncManager(nil, &syncManager, zap.NewNop())
	rendered, err := json.Marshal(ErrorNotice{
		Severity:  "error",
		Component: "decision",
		Symbol:    "ETHUSDT",
		Message:   "channel scoped",
	})
	if err != nil {
		t.Fatalf("marshal notice: %v", err)
	}

	err = manager.Deliver(context.Background(), asyncEventError, "ETHUSDT", "telegram", rendered)
	if err != nil {
		t.Fatalf("Deliver() error=%v", err)
	}
	if telegram.callCount() != 1 {
		t.Fatalf("telegram calls=%d want 1", telegram.callCount())
	}
	if feishu.callCount() != 0 {
		t.Fatalf("feishu calls=%d want 0", feishu.callCount())
	}
}

func TestAsyncDeliverAggregatesCloseNoticesPerChannel(t *testing.T) {
	t.Parallel()

	telegram := &namedCountSender{channel: "telegram"}
	feishu := &namedCountSender{channel: "feishu"}
	syncManager := NewTestManager(telegram, feishu)
	manager := NewAsyncManager(nil, &syncManager, zap.NewNop())

	mustJSON := func(v any) json.RawMessage {
		t.Helper()
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		return data
	}

	if err := manager.Deliver(context.Background(), asyncEventPositionClose, "ETHUSDT", "telegram", mustJSON(PositionCloseNotice{
		Symbol:             "ETHUSDT",
		Direction:          "short",
		Qty:                1.619,
		CloseQty:           1.619,
		IntentKind:         "CLOSE",
		EntryPrice:         2305.53,
		TriggerPrice:       2308.94,
		StopPrice:          2317.88,
		TakeProfits:        []float64{2293.10, 2283.31, 2280.82},
		Reason:             "REVERSAL_CONFIRMED",
		Leverage:           5,
		PositionID:         "ETHUSDT-1777003785119948977",
		ExecutorPositionID: "1",
	})); err != nil {
		t.Fatalf("deliver position close: %v", err)
	}
	if err := manager.Deliver(context.Background(), asyncEventPositionCloseSummary, "ETHUSDT", "telegram", mustJSON(PositionCloseSummaryNotice{
		Symbol:             "ETHUSDT",
		Direction:          "short",
		Qty:                1.619,
		EntryPrice:         2305.53,
		ExitPrice:          2308.94,
		StopPrice:          2317.88,
		TakeProfits:        []float64{2293.10, 2283.31, 2280.82},
		Reason:             "external_missing",
		Leverage:           5,
		PnLAmount:          -5.52079,
		PnLPct:             -0.0014790525388955486,
		PositionID:         "ETHUSDT-1777003785119948977",
		ExecutorPositionID: "1",
	})); err != nil {
		t.Fatalf("deliver position close summary: %v", err)
	}
	if err := manager.Deliver(context.Background(), asyncEventTradeCloseSummary, "ETHUSDT", "telegram", mustJSON(TradeCloseSummaryNotice{
		TradeID:        1,
		Pair:           "ETH/USDT:USDT",
		IsShort:        true,
		OpenRate:       2305.53,
		CloseRate:      2308.95,
		Amount:         1.619,
		StakeAmount:    746.530614,
		CloseProfitAbs: -9.27240156,
		CloseProfitPct: -0.0124,
		ProfitAbs:      -9.27240156,
		ProfitPct:      -0.0124,
		TradeDurationS: 8606,
		ExitReason:     "force_exit",
		Leverage:       5,
	})); err != nil {
		t.Fatalf("deliver trade close summary: %v", err)
	}

	waitForCondition(t, 300*time.Millisecond, func() bool {
		return telegram.callCount() == 1
	})

	if got := feishu.callCount(); got != 0 {
		t.Fatalf("feishu calls=%d want 0", got)
	}
	msg := telegram.lastMessage()
	if !strings.Contains(msg.Markdown, "📉 仓位已关闭") {
		t.Fatalf("expected aggregated close header, got %q", msg.Markdown)
	}
	if !strings.Contains(msg.Markdown, noticeLine("pnl", formatFloat(-9.27240156))) {
		t.Fatalf("expected freqtrade pnl in text body, got %q", msg.Markdown)
	}
	if strings.Contains(msg.Markdown, "gross_pnl") {
		t.Fatalf("expected aggregated message to avoid gross pnl fallback, got %q", msg.Markdown)
	}
	if !strings.Contains(msg.Markdown, noticeLine("reason", "REVERSAL_CONFIRMED")) {
		t.Fatalf("expected decision close reason in text body, got %q", msg.Markdown)
	}
	if strings.Contains(msg.Markdown, "force_exit") {
		t.Fatalf("expected aggregated message to hide generic freqtrade reason, got %q", msg.Markdown)
	}
	if strings.Contains(msg.Markdown, "退出类型") {
		t.Fatalf("expected aggregated message to avoid exit type, got %q", msg.Markdown)
	}
	if !strings.Contains(msg.Markdown, noticeLine("close_rate", formatFloat(2308.95))) {
		t.Fatalf("expected freqtrade close rate in text body, got %q", msg.Markdown)
	}
	if !strings.Contains(msg.Markdown, noticeLine("planned_stop", formatFloat(2317.88))) {
		t.Fatalf("expected planned stop metadata in text body, got %q", msg.Markdown)
	}
	if !strings.Contains(msg.Markdown, "▸ 交易ID：1") {
		t.Fatalf("expected trade id in text body, got %q", msg.Markdown)
	}
}

func TestSendError_DedupesSameNotice(t *testing.T) {
	sender := &countSender{}
	mgr := Manager{senders: []Sender{sender}, dedupe: newDedupeGuard(2 * time.Minute)}
	notice := ErrorNotice{
		Severity:  "error",
		Component: "decision",
		Symbol:    "ETHUSDT",
		Message:   "klines ETHUSDT 1h: EOF",
	}

	if err := mgr.SendError(context.Background(), notice); err != nil {
		t.Fatalf("first send error: %v", err)
	}
	if err := mgr.SendError(context.Background(), notice); err != nil {
		t.Fatalf("second send error: %v", err)
	}
	if sender.callCount() != 1 {
		t.Fatalf("expected deduped error send, got %d", sender.callCount())
	}
}

func TestSendError_DifferentMessagesNotDeduped(t *testing.T) {
	sender := &countSender{}
	mgr := Manager{senders: []Sender{sender}, dedupe: newDedupeGuard(2 * time.Minute)}
	base := ErrorNotice{
		Severity:  "error",
		Component: "decision",
		Symbol:    "ETHUSDT",
		Message:   "klines ETHUSDT 1h: EOF",
	}
	next := base
	next.Message = "llm round save failed"

	if err := mgr.SendError(context.Background(), base); err != nil {
		t.Fatalf("first send error: %v", err)
	}
	if err := mgr.SendError(context.Background(), next); err != nil {
		t.Fatalf("second send error: %v", err)
	}
	if sender.callCount() != 2 {
		t.Fatalf("expected different error messages to send, got %d", sender.callCount())
	}
}

func TestSendErrorAndPositionOpenDedupeScopesAreIndependent(t *testing.T) {
	sender := &countSender{}
	mgr := Manager{senders: []Sender{sender}, dedupe: newDedupeGuard(2 * time.Minute)}
	errorNotice := ErrorNotice{
		Severity:  "error",
		Component: "decision",
		Symbol:    "ETHUSDT",
		Message:   "klines ETHUSDT 1h: EOF",
	}
	openNotice := PositionOpenNotice{
		Symbol:      "ETHUSDT",
		Direction:   "long",
		Qty:         0.5,
		EntryPrice:  3200,
		StopPrice:   3120,
		TakeProfits: []float64{3300, 3380},
		StopReason:  "structure_low",
		PositionID:  "pos-1",
	}

	if err := mgr.SendError(context.Background(), errorNotice); err != nil {
		t.Fatalf("send error notice: %v", err)
	}
	if err := mgr.SendPositionOpen(context.Background(), openNotice); err != nil {
		t.Fatalf("send position open: %v", err)
	}
	if err := mgr.SendError(context.Background(), errorNotice); err != nil {
		t.Fatalf("send duplicate error notice: %v", err)
	}
	if err := mgr.SendPositionOpen(context.Background(), openNotice); err != nil {
		t.Fatalf("send duplicate position open: %v", err)
	}

	msgs := sender.messages()
	if len(msgs) != 2 {
		t.Fatalf("expected one error and one position open send, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Markdown, "错误提醒") {
		t.Fatalf("expected first message to be error notice, got %q", msgs[0].Markdown)
	}
	if !strings.Contains(msgs[1].Markdown, "仓位开启") {
		t.Fatalf("expected second message to be position open notice, got %q", msgs[1].Markdown)
	}
}

func TestPositionOpenDedupesSamePositionIDAndAllowsDistinctPositions(t *testing.T) {
	sender := &countSender{}
	mgr := Manager{senders: []Sender{sender}, dedupe: newDedupeGuard(2 * time.Minute)}
	notice := PositionOpenNotice{
		Symbol:      "ETHUSDT",
		Direction:   "long",
		Qty:         0.5,
		EntryPrice:  3200,
		StopPrice:   3120,
		TakeProfits: []float64{3300, 3380},
		StopReason:  "structure_low",
		PositionID:  "pos-1",
	}

	if err := mgr.SendPositionOpen(context.Background(), notice); err != nil {
		t.Fatalf("send first position open: %v", err)
	}
	if err := mgr.SendPositionOpen(context.Background(), notice); err != nil {
		t.Fatalf("send duplicate position open: %v", err)
	}
	next := notice
	next.PositionID = "pos-2"
	if err := mgr.SendPositionOpen(context.Background(), next); err != nil {
		t.Fatalf("send next position open: %v", err)
	}

	msgs := sender.messages()
	if len(msgs) != 2 {
		t.Fatalf("expected duplicate position id to be skipped and distinct id to send, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Markdown, noticeLine("position_id", "pos-1")) {
		t.Fatalf("expected first position id in body, got %q", msgs[0].Markdown)
	}
	if !strings.Contains(msgs[1].Markdown, noticeLine("position_id", "pos-2")) {
		t.Fatalf("expected second position id in body, got %q", msgs[1].Markdown)
	}
}

func TestTelegramSenderSanitizeRequestErrorRedactsToken(t *testing.T) {
	sender := &TelegramSender{token: "secret-token"}
	err := sender.sanitizeRequestError(
		"image send",
		errors.New(`Post "https://api.telegram.org/botsecret-token/sendDocument": context deadline exceeded`),
	)
	if err == nil {
		t.Fatal("expected sanitized error")
	}
	got := err.Error()
	if strings.Contains(got, "secret-token") {
		t.Fatalf("sanitized error leaked token: %q", got)
	}
	if !strings.Contains(got, "<redacted>") {
		t.Fatalf("sanitized error missing redaction marker: %q", got)
	}
	if !strings.Contains(got, "telegram image send request failed") {
		t.Fatalf("sanitized error missing action context: %q", got)
	}
}

func TestPositionOpen_SendsTextEvenWhenRendererConfigured(t *testing.T) {
	sender := &countSender{}
	renderer := &countingRenderer{}
	mgr := Manager{
		renderer: renderer,
		senders:  []Sender{sender},
		dedupe:   newDedupeGuard(2 * time.Minute),
	}

	if err := mgr.SendPositionOpen(context.Background(), PositionOpenNotice{
		Symbol:      "ETHUSDT",
		Direction:   "long",
		Qty:         0.5,
		EntryPrice:  3200,
		StopPrice:   3120,
		TakeProfits: []float64{3300, 3380},
		StopReason:  "structure_low",
		PositionID:  "pos-1",
	}); err != nil {
		t.Fatalf("send position open: %v", err)
	}

	if got := renderer.renderCallCount(); got != 0 {
		t.Fatalf("renderer should not be called, got %d", got)
	}
	msg := sender.lastMessage()
	if msg.Image != nil {
		t.Fatal("position open should send text only")
	}
	if !strings.Contains(msg.Markdown, "📈 仓位开启") {
		t.Fatalf("expected text header, got %q", msg.Markdown)
	}
	if !strings.Contains(msg.Markdown, noticeLine("stop_reason", "structure_low")) {
		t.Fatalf("expected stop reason in text body, got %q", msg.Markdown)
	}
}

func TestRiskPlanUpdate_SendsTextEvenWhenRendererConfigured(t *testing.T) {
	sender := &countSender{}
	renderer := &countingRenderer{}
	mgr := Manager{
		renderer: renderer,
		senders:  []Sender{sender},
		dedupe:   newDedupeGuard(2 * time.Minute),
	}

	if err := mgr.SendRiskPlanUpdate(context.Background(), RiskPlanUpdateNotice{
		Symbol:         "BTCUSDT",
		Direction:      "long",
		EntryPrice:     100,
		OldStop:        90,
		NewStop:        95,
		StopReason:     "llm-flat",
		Reason:         "atr_tighten",
		TakeProfits:    []float64{110, 120},
		Source:         "monitor-tighten",
		MarkPrice:      102,
		GateSatisfied:  true,
		ScoreTotal:     3.5,
		ScoreThreshold: 3,
		TightenReason:  "volatility_drop",
		PositionID:     "risk-1",
	}); err != nil {
		t.Fatalf("send risk update: %v", err)
	}

	if got := renderer.renderCallCount(); got != 0 {
		t.Fatalf("renderer should not be called, got %d", got)
	}
	msg := sender.lastMessage()
	if msg.Image != nil {
		t.Fatal("risk update should send text only")
	}
	if !strings.Contains(msg.Markdown, "📋 风控计划更新") {
		t.Fatalf("expected risk header, got %q", msg.Markdown)
	}
	if !strings.Contains(msg.Markdown, "▸ 原止损：90 → 95") {
		t.Fatalf("expected stop update in text body, got %q", msg.Markdown)
	}
	if !strings.Contains(msg.Markdown, noticeLine("position_id", "risk-1")) {
		t.Fatalf("expected position id in text body, got %q", msg.Markdown)
	}
}

func TestCloseAggregation_MergesCloseNoticesByExecutorTradeID(t *testing.T) {
	sender := &countSender{}
	renderer := &countingRenderer{}
	mgr := Manager{
		renderer: renderer,
		senders:  []Sender{sender},
		dedupe:   newDedupeGuard(2 * time.Minute),
	}
	mgr.closeAgg = newCloseNoticeAggregator(20*time.Millisecond, mgr.sendAggregatedClose)

	if err := mgr.SendPositionClose(context.Background(), PositionCloseNotice{
		Symbol:             "ETHUSDT",
		Direction:          "long",
		Qty:                0.5,
		CloseQty:           0.5,
		EntryPrice:         3200,
		TriggerPrice:       3300,
		Reason:             "tp1_hit",
		PositionID:         "pos-1",
		ExecutorPositionID: "42",
	}); err != nil {
		t.Fatalf("send position close: %v", err)
	}
	if err := mgr.SendPositionCloseSummary(context.Background(), PositionCloseSummaryNotice{
		Symbol:             "ETHUSDT",
		Direction:          "long",
		Qty:                0.5,
		EntryPrice:         3200,
		ExitPrice:          3310,
		PnLAmount:          55,
		PnLPct:             0.017,
		PositionID:         "pos-1",
		ExecutorPositionID: "42",
	}); err != nil {
		t.Fatalf("send close summary: %v", err)
	}
	if err := mgr.SendTradeCloseSummary(context.Background(), TradeCloseSummaryNotice{
		TradeID:        42,
		Pair:           "ETH/USDT:USDT",
		OpenRate:       3200,
		CloseRate:      3312,
		Amount:         0.5,
		ProfitAbs:      56,
		ProfitPct:      0.0175,
		TradeDurationS: 3600,
		ExitReason:     "roi",
	}); err != nil {
		t.Fatalf("send trade close summary: %v", err)
	}

	waitForCondition(t, 300*time.Millisecond, func() bool {
		return sender.callCount() == 1
	})

	if got := renderer.renderCallCount(); got != 0 {
		t.Fatalf("renderer should not be called, got %d", got)
	}
	msg := sender.lastMessage()
	if msg.Image != nil {
		t.Fatal("aggregated close should send text only")
	}
	if !strings.Contains(msg.Markdown, "📉 仓位已关闭") {
		t.Fatalf("expected close header, got %q", msg.Markdown)
	}
	if !strings.Contains(msg.Markdown, noticeLine("position_id", "pos-1")) {
		t.Fatalf("expected position id in text body, got %q", msg.Markdown)
	}
	if !strings.Contains(msg.Markdown, "▸ 持仓时长：1h0m") {
		t.Fatalf("expected formatted duration in text body, got %q", msg.Markdown)
	}
	if !strings.Contains(msg.Markdown, "▸ 交易ID：42") {
		t.Fatalf("expected trade id in text body, got %q", msg.Markdown)
	}
}

func TestSendPositionCloseIncludesResidualAuditFields(t *testing.T) {
	sender := &countSender{}
	mgr := Manager{
		senders: []Sender{sender},
		dedupe:  newDedupeGuard(2 * time.Minute),
	}

	if err := mgr.SendPositionClose(context.Background(), PositionCloseNotice{
		Symbol:             "ETHUSDT",
		Direction:          "long",
		Qty:                100,
		CloseQty:           100,
		RequestedCloseQty:  98,
		ForcedFullClose:    true,
		IntentKind:         "CLOSE",
		EntryPrice:         3200,
		TriggerPrice:       3300,
		Reason:             "tp1_hit",
		PositionID:         "pos-1",
		ExecutorPositionID: "42",
	}); err != nil {
		t.Fatalf("send position close: %v", err)
	}

	msg := sender.lastMessage()
	if !strings.Contains(msg.Markdown, noticeLine("requested_close_qty", "98")) {
		t.Fatalf("expected requested close qty in text body, got %q", msg.Markdown)
	}
	if !strings.Contains(msg.Markdown, noticeLine("forced_full_close", "yes")) {
		t.Fatalf("expected forced full close flag in text body, got %q", msg.Markdown)
	}
	if !strings.Contains(msg.Markdown, noticeLine("intent_kind", "CLOSE")) {
		t.Fatalf("expected intent kind in text body, got %q", msg.Markdown)
	}
}

func TestCloseAggregation_PrefersTradeExecutionMetrics(t *testing.T) {
	sender := &countSender{}
	mgr := Manager{
		senders: []Sender{sender},
		dedupe:  newDedupeGuard(2 * time.Minute),
	}
	mgr.closeAgg = newCloseNoticeAggregator(20*time.Millisecond, mgr.sendAggregatedClose)

	if err := mgr.SendPositionCloseSummary(context.Background(), PositionCloseSummaryNotice{
		Symbol:             "ETHUSDT",
		Direction:          "long",
		Qty:                0.5,
		EntryPrice:         3200,
		ExitPrice:          3310,
		StopPrice:          3100,
		TakeProfits:        []float64{3400},
		Reason:             "external_missing",
		PnLAmount:          0,
		PnLPct:             0,
		PositionID:         "pos-1",
		ExecutorPositionID: "42",
	}); err != nil {
		t.Fatalf("send close summary: %v", err)
	}
	if err := mgr.SendTradeCloseSummary(context.Background(), TradeCloseSummaryNotice{
		TradeID:        42,
		Pair:           "ETH/USDT:USDT",
		OpenRate:       3200,
		CloseRate:      3312,
		Amount:         0.45,
		ProfitAbs:      -1.25,
		ProfitPct:      -0.0014,
		TradeDurationS: 75,
		ExitReason:     "force_exit",
		Leverage:       2,
	}); err != nil {
		t.Fatalf("send trade close summary: %v", err)
	}

	waitForCondition(t, 300*time.Millisecond, func() bool {
		return sender.callCount() == 1
	})

	msg := sender.lastMessage()
	if !strings.Contains(msg.Markdown, noticeLine("close_rate", formatFloat(3312))) {
		t.Fatalf("expected trade close rate, got %q", msg.Markdown)
	}
	if !strings.Contains(msg.Markdown, noticeLine("qty", formatFloat(0.45))) {
		t.Fatalf("expected trade close qty, got %q", msg.Markdown)
	}
	if !strings.Contains(msg.Markdown, noticeLine("pnl", formatFloat(-1.25))) {
		t.Fatalf("expected trade close pnl, got %q", msg.Markdown)
	}
	if !strings.Contains(msg.Markdown, noticeLine("pnl_pct", formatPercent(-0.0014))) {
		t.Fatalf("expected normalized pnl pct, got %q", msg.Markdown)
	}
	if !strings.Contains(msg.Markdown, noticeLine("planned_stop", formatFloat(3100))) {
		t.Fatalf("expected close summary planned stop metadata, got %q", msg.Markdown)
	}
	if strings.Contains(msg.Markdown, "external_missing") {
		t.Fatalf("expected aggregated message to hide placeholder reason, got %q", msg.Markdown)
	}
	if !strings.Contains(msg.Markdown, noticeLine("reason", "force_exit")) {
		t.Fatalf("expected aggregated message to keep explicit force_exit reason, got %q", msg.Markdown)
	}
	if strings.Contains(msg.Markdown, "退出类型") {
		t.Fatalf("expected aggregated message to avoid exit type, got %q", msg.Markdown)
	}
}

func TestSendTradeCloseSummaryShowsForceExitReason(t *testing.T) {
	sender := &countSender{}
	mgr := Manager{
		senders: []Sender{sender},
		dedupe:  newDedupeGuard(2 * time.Minute),
	}

	if err := mgr.SendTradeCloseSummary(context.Background(), TradeCloseSummaryNotice{
		TradeID:        7,
		Pair:           "ETH/USDT:USDT",
		IsShort:        true,
		OpenRate:       2305.53,
		CloseRate:      2308.95,
		Amount:         1.619,
		ProfitAbs:      -9.27240156,
		ProfitPct:      -0.0124,
		TradeDurationS: 8606,
		ExitReason:     "force_exit",
		Leverage:       5,
	}); err != nil {
		t.Fatalf("send trade close summary: %v", err)
	}

	msg := sender.lastMessage()
	if !strings.Contains(msg.Markdown, noticeLine("reason", "force_exit")) {
		t.Fatalf("expected close summary to show force_exit reason, got %q", msg.Markdown)
	}
}

func TestPositionCloseSummaryExternalMissingLabelsGrossPnL(t *testing.T) {
	sender := &countSender{}
	mgr := Manager{
		senders: []Sender{sender},
		dedupe:  newDedupeGuard(2 * time.Minute),
	}

	if err := mgr.SendPositionCloseSummary(context.Background(), PositionCloseSummaryNotice{
		Symbol:     "ETHUSDT",
		Direction:  "long",
		Qty:        0.5,
		EntryPrice: 3200,
		ExitPrice:  3310,
		Reason:     "external_missing",
		PnLAmount:  55,
		PnLPct:     0.017,
		PositionID: "pos-1",
	}); err != nil {
		t.Fatalf("send close summary: %v", err)
	}

	msg := sender.lastMessage()
	if !strings.Contains(msg.Markdown, noticeLine("gross_pnl", formatFloat(55))) {
		t.Fatalf("expected gross pnl label, got %q", msg.Markdown)
	}
	if !strings.Contains(msg.Markdown, noticeLine("gross_pnl_pct", formatPercent(0.017))) {
		t.Fatalf("expected gross pnl pct label, got %q", msg.Markdown)
	}
	if strings.Contains(msg.Markdown, noticeLine("reason", "external_missing")) {
		t.Fatalf("expected external_missing reason to stay hidden, got %q", msg.Markdown)
	}
}
