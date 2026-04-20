package pgstore

import (
	"context"
	"testing"
	"time"

	"brale-core/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type captureRow struct {
	scanFn func(dest ...any) error
}

func (r captureRow) Scan(dest ...any) error {
	if r.scanFn != nil {
		return r.scanFn(dest...)
	}
	return nil
}

type captureQueryer struct {
	sql  string
	args []any
}

func (q *captureQueryer) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (q *captureQueryer) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	q.sql = sql
	q.args = append([]any(nil), args...)
	return captureRow{
		scanFn: func(dest ...any) error {
			if len(dest) == 1 {
				if createdAt, ok := dest[0].(*time.Time); ok {
					*createdAt = time.Unix(123, 0).UTC()
				}
			}
			return nil
		},
	}
}

func (q *captureQueryer) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func TestSaveLLMRoundKeepsEmptyPromptVersionNonNull(t *testing.T) {
	queryer := &captureQueryer{}
	s := &PGStore{dbOverride: queryer}
	rec := &store.LLMRoundRecord{
		ID:            "round-early-fail",
		SnapshotID:    1,
		Symbol:        "ETHUSDT",
		RoundType:     "decide",
		StartedAt:     time.Unix(10, 0).UTC(),
		FinishedAt:    time.Unix(20, 0).UTC(),
		CallCount:     0,
		Outcome:       "runner_error",
		PromptVersion: "",
		Error:         "klines EOF",
	}

	if err := s.SaveLLMRound(context.Background(), rec); err != nil {
		t.Fatalf("SaveLLMRound() error = %v", err)
	}

	if len(queryer.args) != 16 {
		t.Fatalf("arg count = %d want 16", len(queryer.args))
	}
	if queryer.args[11] == nil {
		t.Fatal("prompt_version arg = nil want empty string")
	}
	got, ok := queryer.args[11].(string)
	if !ok {
		t.Fatalf("prompt_version arg type = %T want string", queryer.args[11])
	}
	if got != "" {
		t.Fatalf("prompt_version arg = %q want empty string", got)
	}
	if rec.CreatedAt.IsZero() {
		t.Fatal("created_at should be set by scan")
	}
}
