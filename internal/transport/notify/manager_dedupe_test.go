package notify

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"brale-core/internal/cardimage"
	"brale-core/internal/decision/decisionfmt"
)

type countSender struct {
	mu      sync.Mutex
	calls   int
	lastMsg Message
}

func (s *countSender) Send(ctx context.Context, msg Message) error {
	s.mu.Lock()
	s.calls++
	s.lastMsg = msg
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
		ExitType:       "force_exit",
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
		ExitType:       "external",
		Leverage:       2,
	}); err != nil {
		t.Fatalf("send trade close summary: %v", err)
	}

	waitForCondition(t, 300*time.Millisecond, func() bool {
		return sender.callCount() == 1
	})

	msg := sender.lastMessage()
	if !strings.Contains(msg.Markdown, noticeLine("exit", formatFloat(3312))) {
		t.Fatalf("expected trade close exit price, got %q", msg.Markdown)
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
	if !strings.Contains(msg.Markdown, noticeLine("stop", formatFloat(3100))) {
		t.Fatalf("expected close summary stop metadata, got %q", msg.Markdown)
	}
}
