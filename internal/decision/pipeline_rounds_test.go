package decision

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPipelineRoundRecorderTimeout_DefaultWhenUnset(t *testing.T) {
	t.Parallel()

	p := &Pipeline{}
	if got := p.roundRecorderTimeout(); got != defaultRoundRecorderTimeout {
		t.Fatalf("roundRecorderTimeout()=%v want %v", got, defaultRoundRecorderTimeout)
	}
}

func TestPipelineRoundRecorderTimeout_ExplicitZeroUsesDefault(t *testing.T) {
	t.Parallel()

	p := &Pipeline{
		RoundRecorderTimeout:    0,
		RoundRecorderTimeoutSet: true,
	}
	if got := p.roundRecorderTimeout(); got != defaultRoundRecorderTimeout {
		t.Fatalf("roundRecorderTimeout()=%v want %v", got, defaultRoundRecorderTimeout)
	}
}

func TestPipelineRoundRecorderRetries_DefaultWhenUnset(t *testing.T) {
	t.Parallel()

	p := &Pipeline{}
	if got := p.roundRecorderRetries(); got != defaultRoundRecorderRetries {
		t.Fatalf("roundRecorderRetries()=%d want %d", got, defaultRoundRecorderRetries)
	}
}

func TestPipelineRoundRecorderRetries_UsesExplicitZero(t *testing.T) {
	t.Parallel()

	p := &Pipeline{
		RoundRecorderRetries:    0,
		RoundRecorderRetriesSet: true,
	}
	if got := p.roundRecorderRetries(); got != 0 {
		t.Fatalf("roundRecorderRetries()=%d want 0", got)
	}
}

func TestFinishRoundRecorderWith_ZeroRetriesOnlyAttemptsOnce(t *testing.T) {
	t.Parallel()

	attempts := 0
	errBoom := errors.New("boom")
	err := finishRoundRecorderWith(context.Background(), "ok", time.Millisecond, 0, func(context.Context, string) error {
		attempts++
		return errBoom
	})
	if !errors.Is(err, errBoom) {
		t.Fatalf("finishRoundRecorderWith() error=%v want wrapped %v", err, errBoom)
	}
	if attempts != 1 {
		t.Fatalf("attempts=%d want 1", attempts)
	}
}

func TestFinishRoundRecorderWith_ZeroTimeoutUsesDefault(t *testing.T) {
	t.Parallel()

	var remaining time.Duration
	err := finishRoundRecorderWith(context.Background(), "ok", 0, 0, func(ctx context.Context, _ string) error {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("expected deadline when timeout is zero")
		}
		remaining = time.Until(deadline)
		return nil
	})
	if err != nil {
		t.Fatalf("finishRoundRecorderWith() error=%v", err)
	}
	if remaining <= 0 || remaining > defaultRoundRecorderTimeout {
		t.Fatalf("deadline remaining=%v want within default timeout %v", remaining, defaultRoundRecorderTimeout)
	}
}
