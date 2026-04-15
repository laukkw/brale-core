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
	riskMonitorExec func(ctx context.Context, symbol string) error,
	renderFn func(ctx context.Context, eventType, symbol string, payload json.RawMessage) (json.RawMessage, error),
	deliverFn func(ctx context.Context, eventType, symbol string, rendered json.RawMessage) error,
) *river.Workers {
	workers := river.NewWorkers()
	river.AddWorker(workers, &ObserveWorker{Execute: observeExec})
	river.AddWorker(workers, &DecideWorker{Execute: decideExec})
	river.AddWorker(workers, &ReconcileWorker{Execute: reconcileExec})
	river.AddWorker(workers, &RiskMonitorWorker{Execute: riskMonitorExec})
	river.AddWorker(workers, &NotifyRenderWorker{Render: renderFn})
	river.AddWorker(workers, &NotifyDeliverWorker{Deliver: deliverFn})
	return workers
}

type PeriodicSchedule struct {
	Symbol              string
	ObserveInterval     time.Duration
	DecideInterval      time.Duration
	ReconcileInterval   time.Duration
	RiskMonitorInterval time.Duration
}

// BuildPeriodicJobs creates periodic job schedules for each symbol using its own intervals.
func BuildPeriodicJobs(schedules []PeriodicSchedule) []*river.PeriodicJob {
	var jobs []*river.PeriodicJob

	for _, schedule := range schedules {
		sym := schedule.Symbol
		if sym == "" {
			continue
		}

		if schedule.ObserveInterval > 0 {
			jobs = append(jobs, river.NewPeriodicJob(
				river.PeriodicInterval(schedule.ObserveInterval),
				func() (river.JobArgs, *river.InsertOpts) {
					return ObserveArgs{Symbol: sym}, nil
				},
				&river.PeriodicJobOpts{RunOnStart: true},
			))
		}

		if schedule.DecideInterval > 0 {
			jobs = append(jobs, river.NewPeriodicJob(
				river.PeriodicInterval(schedule.DecideInterval),
				func() (river.JobArgs, *river.InsertOpts) {
					return DecideArgs{Symbol: sym}, nil
				},
				&river.PeriodicJobOpts{RunOnStart: false},
			))
		}

		if schedule.ReconcileInterval > 0 {
			jobs = append(jobs, river.NewPeriodicJob(
				river.PeriodicInterval(schedule.ReconcileInterval),
				func() (river.JobArgs, *river.InsertOpts) {
					return ReconcileArgs{Symbol: sym}, nil
				},
				&river.PeriodicJobOpts{RunOnStart: false},
			))
		}
		if schedule.RiskMonitorInterval > 0 {
			jobs = append(jobs, river.NewPeriodicJob(
				river.PeriodicInterval(schedule.RiskMonitorInterval),
				func() (river.JobArgs, *river.InsertOpts) {
					return RiskMonitorArgs{Symbol: sym}, nil
				},
				&river.PeriodicJobOpts{RunOnStart: false},
			))
		}
	}

	return jobs
}
