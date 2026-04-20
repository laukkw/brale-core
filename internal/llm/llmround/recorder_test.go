package llmround

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"brale-core/internal/llm"
	"brale-core/internal/store"
)

type mockRoundStore struct {
	saved *store.LLMRoundRecord
}

func (m *mockRoundStore) SaveLLMRound(_ context.Context, rec *store.LLMRoundRecord) error {
	m.saved = rec
	return nil
}

func (m *mockRoundStore) FindLLMRound(_ context.Context, _ string) (store.LLMRoundRecord, bool, error) {
	return store.LLMRoundRecord{}, false, nil
}

func (m *mockRoundStore) ListLLMRounds(_ context.Context, _ string, _ int) ([]store.LLMRoundRecord, error) {
	return nil, nil
}

func TestRecorderAccumulatesCallsAndPersists(t *testing.T) {
	ms := &mockRoundStore{}
	rec := NewRecorder(ms, "round-1", "BTCUSDT", "decide")

	rec.RecordCall(100, 50, "v1.0")
	rec.RecordCall(200, 80, "v1.0")
	rec.RecordCall(150, 60, "v1.1")

	if err := rec.Finish(context.Background(), "open_long"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ms.saved == nil {
		t.Fatal("expected saved round record")
	}
	if ms.saved.ID != "round-1" {
		t.Fatalf("expected round-1, got %s", ms.saved.ID)
	}
	if ms.saved.TotalTokenIn != 450 {
		t.Fatalf("expected 450 token in, got %d", ms.saved.TotalTokenIn)
	}
	if ms.saved.TotalTokenOut != 190 {
		t.Fatalf("expected 190 token out, got %d", ms.saved.TotalTokenOut)
	}
	if ms.saved.CallCount != 3 {
		t.Fatalf("expected 3 calls, got %d", ms.saved.CallCount)
	}
	if ms.saved.PromptVersion != "v1.0,v1.1" {
		t.Fatalf("expected version set, got %s", ms.saved.PromptVersion)
	}
	if ms.saved.TotalLatencyMS < 0 {
		t.Fatal("latency should be non-negative")
	}
}

func TestRecorderNilStoreIsNoop(t *testing.T) {
	rec := NewRecorder(nil, "round-2", "ETHUSDT", "observe")
	rec.RecordCall(100, 50, "v1")
	if err := rec.Finish(context.Background(), "wait"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecorderFieldValues(t *testing.T) {
	ms := &mockRoundStore{}
	rec := NewRecorder(ms, "round-3", "BTCUSDT", "risk")
	rec.RecordCall(10, 20, "")

	_ = rec.Finish(context.Background(), "tighten")

	if ms.saved.Symbol != "BTCUSDT" || ms.saved.RoundType != "risk" || ms.saved.Outcome != "tighten" {
		t.Fatalf("unexpected field values: %+v", ms.saved)
	}
	if ms.saved.PromptVersion != "" {
		t.Fatalf("prompt_version=%q want empty string", ms.saved.PromptVersion)
	}
	if ms.saved.CreatedAt.Before(time.Now().Add(-time.Minute)) {
		t.Fatal("created_at should be recent")
	}
}

func TestRecorderConcurrentObserveCall(t *testing.T) {
	ms := &mockRoundStore{}
	rec := NewRecorder(ms, "round-concurrent", "BTCUSDT", "decide")
	const callN = 64

	var wg sync.WaitGroup
	for i := range callN {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rec.ObserveCall(context.Background(), llm.CallStats{
				TokenIn:       10,
				TokenOut:      5,
				LatencyMs:     20,
				PromptVersion: "v" + strconv.Itoa(i%4),
			})
		}(i)
	}
	wg.Wait()

	if err := rec.Finish(context.Background(), "allow"); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if ms.saved == nil {
		t.Fatal("expected saved round record")
	}
	if ms.saved.CallCount != callN {
		t.Fatalf("call_count=%d want %d", ms.saved.CallCount, callN)
	}
	if ms.saved.TotalTokenIn != callN*10 {
		t.Fatalf("token_in=%d want %d", ms.saved.TotalTokenIn, callN*10)
	}
	if ms.saved.TotalTokenOut != callN*5 {
		t.Fatalf("token_out=%d want %d", ms.saved.TotalTokenOut, callN*5)
	}
	if ms.saved.TotalLatencyMS != callN*20 {
		t.Fatalf("latency=%d want %d", ms.saved.TotalLatencyMS, callN*20)
	}
	if ms.saved.PromptVersion != "v0,v1,v2,v3" {
		t.Fatalf("prompt_version=%q", ms.saved.PromptVersion)
	}
}

func TestRecorderBudgetWarnThresholdTriggersOnce(t *testing.T) {
	rec := NewRecorder(nil, "round-warn", "BTCUSDT", "decide")
	warnCalls := 0
	exceedCalls := 0
	rec.SetTokenBudget(1000, 80,
		func(roundID string, totalTokens, warnThreshold, budget int) {
			warnCalls++
			if roundID != "round-warn" {
				t.Fatalf("roundID=%q want round-warn", roundID)
			}
			if totalTokens != 850 {
				t.Fatalf("totalTokens=%d want 850", totalTokens)
			}
			if warnThreshold != 800 {
				t.Fatalf("warnThreshold=%d want 800", warnThreshold)
			}
			if budget != 1000 {
				t.Fatalf("budget=%d want 1000", budget)
			}
		},
		func(string, int, int) {
			exceedCalls++
		},
	)

	rec.RecordCall(300, 200, "")
	rec.RecordCall(150, 200, "")
	rec.RecordCall(5, 5, "")

	if warnCalls != 1 {
		t.Fatalf("warnCalls=%d want 1", warnCalls)
	}
	if exceedCalls != 0 {
		t.Fatalf("exceedCalls=%d want 0", exceedCalls)
	}
	if rec.BudgetExceeded() {
		t.Fatal("BudgetExceeded()=true want false")
	}
}

func TestRecorderBudgetExceededWarnsOnceWithoutEarlyWarn(t *testing.T) {
	rec := NewRecorder(nil, "round-exceed", "BTCUSDT", "decide")
	warnCalls := 0
	exceedCalls := 0
	rec.SetTokenBudget(1000, 80,
		func(string, int, int, int) {
			warnCalls++
		},
		func(roundID string, totalTokens, budget int) {
			exceedCalls++
			if roundID != "round-exceed" {
				t.Fatalf("roundID=%q want round-exceed", roundID)
			}
			if totalTokens != 1100 {
				t.Fatalf("totalTokens=%d want 1100", totalTokens)
			}
			if budget != 1000 {
				t.Fatalf("budget=%d want 1000", budget)
			}
		},
	)

	rec.RecordCall(600, 500, "")
	rec.RecordCall(10, 10, "")

	if warnCalls != 0 {
		t.Fatalf("warnCalls=%d want 0", warnCalls)
	}
	if exceedCalls != 1 {
		t.Fatalf("exceedCalls=%d want 1", exceedCalls)
	}
	if !rec.BudgetExceeded() {
		t.Fatal("BudgetExceeded()=false want true")
	}
}
