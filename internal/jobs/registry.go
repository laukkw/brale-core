package jobs

import (
	"context"
	"encoding/json"
	"time"

	"github.com/riverqueue/river"
)

// RegisterWorkers creates a river.Workers with all brale-core job workers registered.
func RegisterWorkers(
	observeExec func(ctx context.Context, symbol string) error,
	decideExec func(ctx context.Context, symbol string) error,
	reconcileExec func(ctx context.Context, symbol string) error,
	renderFn func(ctx context.Context, eventType, symbol string, payload json.RawMessage) (json.RawMessage, error),
	deliverFn func(ctx context.Context, eventType, symbol string, rendered json.RawMessage) error,
) *river.Workers {
	workers := river.NewWorkers()
	river.AddWorker(workers, &ObserveWorker{Execute: observeExec})
	river.AddWorker(workers, &DecideWorker{Execute: decideExec})
	river.AddWorker(workers, &ReconcileWorker{Execute: reconcileExec})
	river.AddWorker(workers, &NotifyRenderWorker{Render: renderFn})
	river.AddWorker(workers, &NotifyDeliverWorker{Deliver: deliverFn})
	return workers
}

// BuildPeriodicJobs creates periodic job schedules for each symbol.
// observeInterval and decideInterval control how often the observe/decide tasks run.
func BuildPeriodicJobs(symbols []string, observeInterval, decideInterval, reconcileInterval time.Duration) []*river.PeriodicJob {
	var jobs []*river.PeriodicJob

	for _, sym := range symbols {
		sym := sym

		jobs = append(jobs, river.NewPeriodicJob(
			river.PeriodicInterval(observeInterval),
			func() (river.JobArgs, *river.InsertOpts) {
				return ObserveArgs{Symbol: sym}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		))

		jobs = append(jobs, river.NewPeriodicJob(
			river.PeriodicInterval(decideInterval),
			func() (river.JobArgs, *river.InsertOpts) {
				return DecideArgs{Symbol: sym}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: false},
		))

		if reconcileInterval > 0 {
			jobs = append(jobs, river.NewPeriodicJob(
				river.PeriodicInterval(reconcileInterval),
				func() (river.JobArgs, *river.InsertOpts) {
					return ReconcileArgs{Symbol: sym}, nil
				},
				&river.PeriodicJobOpts{RunOnStart: false},
			))
		}
	}

	return jobs
}
