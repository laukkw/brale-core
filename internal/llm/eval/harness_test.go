package eval

import (
	"context"
	"testing"

	"brale-core/internal/store"
)

type fakeRoundStore struct {
	rounds []store.LLMRoundRecord
}

func (f fakeRoundStore) SaveLLMRound(context.Context, *store.LLMRoundRecord) error {
	return nil
}

func (f fakeRoundStore) FindLLMRound(context.Context, string) (store.LLMRoundRecord, bool, error) {
	return store.LLMRoundRecord{}, false, nil
}

func (f fakeRoundStore) ListLLMRounds(_ context.Context, symbol string, limit int) ([]store.LLMRoundRecord, error) {
	if limit > 0 && limit < len(f.rounds) {
		return f.rounds[:limit], nil
	}
	return f.rounds, nil
}

func TestHarnessRunBuildsSummary(t *testing.T) {
	h := Harness{
		Store: fakeRoundStore{
			rounds: []store.LLMRoundRecord{
				{
					ID:             "r1",
					Symbol:         "BTCUSDT",
					RoundType:      "decide",
					PromptVersion:  "builtin",
					Outcome:        "ok",
					TotalLatencyMS: 1200,
					TotalTokenIn:   100,
					TotalTokenOut:  40,
					CallCount:      3,
				},
				{
					ID:             "r2",
					Symbol:         "ETHUSDT",
					RoundType:      "observe",
					PromptVersion:  "v2",
					Outcome:        "runner_error",
					Error:          "timeout",
					TotalLatencyMS: 500,
					TotalTokenIn:   10,
					TotalTokenOut:  0,
					CallCount:      1,
				},
			},
		},
	}

	report, err := h.Run(context.Background(), Request{Limit: 10})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if report.Summary.TotalRounds != 2 {
		t.Fatalf("TotalRounds=%d want 2", report.Summary.TotalRounds)
	}
	if report.Summary.ErrorRounds != 1 {
		t.Fatalf("ErrorRounds=%d want 1", report.Summary.ErrorRounds)
	}
	if len(report.Results) != 2 {
		t.Fatalf("len(Results)=%d want 2", len(report.Results))
	}
	if report.Results[0].RoundID != "r1" {
		t.Fatalf("first round id=%q want r1", report.Results[0].RoundID)
	}
	if report.Results[1].Score != 0 {
		t.Fatalf("error round score=%v want 0", report.Results[1].Score)
	}
	if got := report.Summary.PromptBreakdown[0].Name; got != "builtin" {
		t.Fatalf("top prompt bucket=%q want builtin", got)
	}
}
