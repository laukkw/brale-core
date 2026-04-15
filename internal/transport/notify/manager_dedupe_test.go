package notify

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"brale-core/internal/cardimage"
	"brale-core/internal/decision/decisionfmt"
)

type countSender struct {
	mu    sync.Mutex
	calls int
}

func (s *countSender) Send(ctx context.Context, msg Message) error {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	return nil
}

func (s *countSender) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
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

func (staticRenderer) RenderCard(ctx context.Context, cardType string, symbol string, data map[string]any, title string) (*cardimage.ImageAsset, error) {
	return &cardimage.ImageAsset{Data: []byte("png"), Filename: "card.png", ContentType: "image/png", Caption: title}, nil
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

type captureRenderer struct {
	mu       sync.Mutex
	calls    int
	cardType string
	title    string
	data     map[string]any
}

func (r *captureRenderer) RenderDecision(ctx context.Context, input decisionfmt.DecisionInput, report decisionfmt.DecisionReport) (*cardimage.ImageAsset, error) {
	return &cardimage.ImageAsset{Data: []byte("png"), Filename: "decision.png", ContentType: "image/png", Caption: report.Symbol}, nil
}

func (r *captureRenderer) RenderCard(ctx context.Context, cardType string, symbol string, data map[string]any, title string) (*cardimage.ImageAsset, error) {
	cloned := make(map[string]any, len(data))
	for k, v := range data {
		cloned[k] = v
	}
	r.mu.Lock()
	r.calls++
	r.cardType = cardType
	r.title = title
	r.data = cloned
	r.mu.Unlock()
	return &cardimage.ImageAsset{Data: []byte("png"), Filename: "card.png", ContentType: "image/png", Caption: title}, nil
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

func TestSendWithKey_PartialFailureDoesNotPoisonDedupe(t *testing.T) {
	okSender := &countSender{}
	failSender := &alwaysFailSender{}
	mgr := Manager{senders: []Sender{okSender, failSender}, dedupe: newDedupeGuard(2 * time.Minute)}

	if err := mgr.sendWithKey(context.Background(), Message{Title: "partial"}, "partial-key"); err == nil {
		t.Fatal("expected first send error")
	}
	if err := mgr.sendWithKey(context.Background(), Message{Title: "partial"}, "partial-key"); err == nil {
		t.Fatal("expected second send error")
	}
	if okSender.callCount() != 2 {
		t.Fatalf("expected successful sender to be retried, got %d calls", okSender.callCount())
	}
}

func TestCloseAggregation_MergesCloseNoticesByExecutorTradeID(t *testing.T) {
	sender := &countSender{}
	renderer := &captureRenderer{}
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
		renderer.mu.Lock()
		defer renderer.mu.Unlock()
		return sender.callCount() == 1 && renderer.calls == 1
	})

	renderer.mu.Lock()
	defer renderer.mu.Unlock()
	if renderer.cardType != "position_close" {
		t.Fatalf("card type = %q want position_close", renderer.cardType)
	}
	if got := renderer.data["position_id"]; got != "pos-1" {
		t.Fatalf("position_id = %v want pos-1", got)
	}
	if got := renderer.data["executor_position_id"]; got != "42" {
		t.Fatalf("executor_position_id = %v want 42", got)
	}
	if got := renderer.data["trade_id"]; got != 42 {
		t.Fatalf("trade_id = %v want 42", got)
	}
	if got := renderer.data["trade_duration_s"]; got != int64(3600) {
		t.Fatalf("trade_duration_s = %v want 3600", got)
	}
}
