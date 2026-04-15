package otel

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// Meters for brale-core core metrics.
var (
	meter = otel.Meter("brale-core")

	// Pipeline metrics
	PipelineRoundsTotal   metric.Int64Counter
	PipelineLatencyMs     metric.Int64Histogram
	PipelineTokensTotal   metric.Int64Counter
	PipelineErrorsTotal   metric.Int64Counter
	PipelineGateDecisions metric.Int64Counter

	// LLM call metrics
	LLMCallLatencyMs metric.Int64Histogram
	LLMCallTokenIn   metric.Int64Counter
	LLMCallTokenOut  metric.Int64Counter
	LLMCallErrors    metric.Int64Counter

	// Notification metrics
	NotifyEnqueueTotal metric.Int64Counter
	NotifyDeliverTotal metric.Int64Counter
	NotifyFailTotal    metric.Int64Counter

	// Position metrics
	PositionOpenTotal  metric.Int64Counter
	PositionCloseTotal metric.Int64Counter
)

func init() {
	var err error

	PipelineRoundsTotal, err = meter.Int64Counter("brale.pipeline.rounds.total",
		metric.WithDescription("Total decision pipeline rounds"))
	must(err)

	PipelineLatencyMs, err = meter.Int64Histogram("brale.pipeline.latency_ms",
		metric.WithDescription("Pipeline round latency in ms"))
	must(err)

	PipelineTokensTotal, err = meter.Int64Counter("brale.pipeline.tokens.total",
		metric.WithDescription("Total tokens consumed by pipeline rounds"))
	must(err)

	PipelineErrorsTotal, err = meter.Int64Counter("brale.pipeline.errors.total",
		metric.WithDescription("Total pipeline round errors"))
	must(err)

	PipelineGateDecisions, err = meter.Int64Counter("brale.pipeline.gate.decisions",
		metric.WithDescription("Gate decisions by action"))
	must(err)

	LLMCallLatencyMs, err = meter.Int64Histogram("brale.llm.call.latency_ms",
		metric.WithDescription("Individual LLM call latency"))
	must(err)

	LLMCallTokenIn, err = meter.Int64Counter("brale.llm.call.token_in",
		metric.WithDescription("LLM input tokens"))
	must(err)

	LLMCallTokenOut, err = meter.Int64Counter("brale.llm.call.token_out",
		metric.WithDescription("LLM output tokens"))
	must(err)

	LLMCallErrors, err = meter.Int64Counter("brale.llm.call.errors",
		metric.WithDescription("LLM call errors"))
	must(err)

	NotifyEnqueueTotal, err = meter.Int64Counter("brale.notify.enqueue.total",
		metric.WithDescription("Notifications enqueued"))
	must(err)

	NotifyDeliverTotal, err = meter.Int64Counter("brale.notify.deliver.total",
		metric.WithDescription("Notifications delivered"))
	must(err)

	NotifyFailTotal, err = meter.Int64Counter("brale.notify.fail.total",
		metric.WithDescription("Notification delivery failures"))
	must(err)

	PositionOpenTotal, err = meter.Int64Counter("brale.position.open.total",
		metric.WithDescription("Positions opened"))
	must(err)

	PositionCloseTotal, err = meter.Int64Counter("brale.position.close.total",
		metric.WithDescription("Positions closed"))
	must(err)
}

func must(err error) {
	if err != nil {
		panic("otel metric init: " + err.Error())
	}
}
