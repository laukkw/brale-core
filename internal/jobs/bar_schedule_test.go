package jobs

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

func TestAlignedBarCloseScheduleNext(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("UTC+8", 8*60*60)
	tests := []struct {
		name     string
		now      time.Time
		interval time.Duration
		want     time.Time
	}{
		{
			name:     "30m from off-cycle time",
			now:      time.Date(2026, 4, 16, 21, 22, 45, 0, loc),
			interval: 30 * time.Minute,
			want:     time.Date(2026, 4, 16, 21, 30, 10, 0, loc),
		},
		{
			name:     "30m from exact bar close advances to next bar",
			now:      time.Date(2026, 4, 16, 21, 30, 10, 0, loc),
			interval: 30 * time.Minute,
			want:     time.Date(2026, 4, 16, 22, 0, 10, 0, loc),
		},
		{
			name:     "1h from off-cycle time",
			now:      time.Date(2026, 4, 16, 21, 22, 45, 0, loc),
			interval: time.Hour,
			want:     time.Date(2026, 4, 16, 22, 0, 10, 0, loc),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := AlignedBarCloseSchedule(tc.interval).Next(tc.now)
			if !got.Equal(tc.want) {
				t.Fatalf("Next(%v) = %v, want %v", tc.now, got, tc.want)
			}
		})
	}
}

func TestBuildPeriodicJobsUsesIndependentAlignedSchedules(t *testing.T) {
	t.Parallel()

	schedules := []PeriodicSchedule{
		{
			Symbol:          "BTCUSDT",
			ObserveInterval: 30 * time.Minute,
			DecideInterval:  30 * time.Minute,
		},
		{
			Symbol:          "ETHUSDT",
			ObserveInterval: time.Hour,
			DecideInterval:  time.Hour,
		},
	}

	jobs := BuildPeriodicJobs(schedules)
	if len(jobs) != 4 {
		t.Fatalf("BuildPeriodicJobs() created %d jobs, want 4", len(jobs))
	}

	loc := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2026, 4, 16, 21, 22, 45, 0, loc)

	got30m := AlignedBarCloseSchedule(30 * time.Minute).Next(now)
	want30m := time.Date(2026, 4, 16, 21, 30, 10, 0, loc)
	if !got30m.Equal(want30m) {
		t.Fatalf("30m schedule = %v, want %v", got30m, want30m)
	}

	got1h := AlignedBarCloseSchedule(time.Hour).Next(now)
	want1h := time.Date(2026, 4, 16, 22, 0, 10, 0, loc)
	if !got1h.Equal(want1h) {
		t.Fatalf("1h schedule = %v, want %v", got1h, want1h)
	}
}

func TestNotifyDeliverArgsInsertOptsDisablesAutomaticRetry(t *testing.T) {
	t.Parallel()

	opts := (NotifyDeliverArgs{}).InsertOpts()
	if opts.MaxAttempts != 1 {
		t.Fatalf("MaxAttempts=%d want 1", opts.MaxAttempts)
	}
	if !opts.UniqueOpts.ByArgs {
		t.Fatalf("UniqueOpts.ByArgs=false want true")
	}
	if opts.UniqueOpts.ByPeriod != 2*time.Minute {
		t.Fatalf("UniqueOpts.ByPeriod=%v want 2m", opts.UniqueOpts.ByPeriod)
	}
}

func TestNotifyRenderArgsInsertOptsDedupesShortWindow(t *testing.T) {
	t.Parallel()

	opts := (NotifyRenderArgs{}).InsertOpts()
	if !opts.UniqueOpts.ByArgs {
		t.Fatalf("UniqueOpts.ByArgs=false want true")
	}
	if opts.UniqueOpts.ByPeriod != 2*time.Minute {
		t.Fatalf("UniqueOpts.ByPeriod=%v want 2m", opts.UniqueOpts.ByPeriod)
	}
}

func TestNotifyDeliverWorkerSkipsRetryAttempts(t *testing.T) {
	t.Parallel()

	calls := 0
	worker := &NotifyDeliverWorker{
		Deliver: func(context.Context, string, string, json.RawMessage) error {
			calls++
			return nil
		},
	}

	err := worker.Work(context.Background(), &river.Job[NotifyDeliverArgs]{
		JobRow: &rivertype.JobRow{Attempt: 2},
		Args: NotifyDeliverArgs{
			EventType: "gate",
			Symbol:    "ETHUSDT",
			Rendered:  json.RawMessage(`{"ok":true}`),
		},
	})
	if err != nil {
		t.Fatalf("Work() error=%v", err)
	}
	if calls != 0 {
		t.Fatalf("Deliver calls=%d want 0", calls)
	}
}
