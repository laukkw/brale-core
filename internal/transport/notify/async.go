package notify

import (
	"context"
	"encoding/json"
	"fmt"

	"brale-core/internal/decision/decisionfmt"
	"brale-core/internal/jobs"
	"brale-core/internal/notifyport"
	braleOtel "brale-core/internal/otel"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

const (
	asyncEventGate                 = "gate"
	asyncEventPositionOpen         = "position_open"
	asyncEventPositionClose        = "position_close"
	asyncEventPositionCloseSummary = "position_close_summary"
	asyncEventRiskUpdate           = "risk_update"
	asyncEventError                = "error"
	asyncEventTradeOpen            = "trade_open"
	asyncEventTradePartialClose    = "trade_partial_close"
	asyncEventTradeCloseSummary    = "trade_close_summary"
)

type gateAsyncPayload struct {
	Input  decisionfmt.DecisionInput  `json:"input"`
	Report decisionfmt.DecisionReport `json:"report"`
}

type AsyncManager struct {
	client *river.Client[pgx.Tx]
	sync   Notifier
	logger *zap.Logger
}

func NewAsyncManager(client *river.Client[pgx.Tx], sync Notifier, logger *zap.Logger) *AsyncManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AsyncManager{client: client, sync: sync, logger: logger}
}

func (m *AsyncManager) SetClient(client *river.Client[pgx.Tx]) {
	if m != nil {
		m.client = client
	}
}

func (m *AsyncManager) enqueue(ctx context.Context, eventType, symbol string, payload any) error {
	ctx, span := braleOtel.Tracer("brale-core/notify").Start(ctx, "brale.notify.enqueue")
	span.SetAttributes(
		attribute.String("notify.event_type", eventType),
		attribute.String("notify.symbol", symbol),
	)
	defer span.End()

	if m.client == nil {
		err := fmt.Errorf("river client is required for async notifications")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("marshal async notify payload: %w", err)
	}
	args := jobs.NotifyRenderArgs{
		EventType: eventType,
		Symbol:    symbol,
		Payload:   json.RawMessage(data),
	}
	if tx := notifyport.TxFromContext(ctx); tx != nil {
		_, err = m.client.InsertTx(ctx, tx, args, nil)
	} else {
		_, err = m.client.Insert(ctx, args, nil)
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("enqueue async notification: %w", err)
	}
	braleOtel.NotifyEnqueueTotal.Add(ctx, 1, otelmetric.WithAttributes(attribute.String("event_type", eventType)))
	return nil
}

func (m *AsyncManager) SendGate(ctx context.Context, input decisionfmt.DecisionInput, report decisionfmt.DecisionReport) error {
	return m.enqueue(ctx, asyncEventGate, input.Symbol, gateAsyncPayload{Input: input, Report: report})
}

func (m *AsyncManager) SendStartup(ctx context.Context, info StartupInfo) error {
	if m.sync == nil {
		return nil
	}
	return m.sync.SendStartup(ctx, info)
}

func (m *AsyncManager) SendShutdown(ctx context.Context, info ShutdownInfo) error {
	if m.sync == nil {
		return nil
	}
	return m.sync.SendShutdown(ctx, info)
}

func (m *AsyncManager) SendError(ctx context.Context, notice ErrorNotice) error {
	return m.enqueue(ctx, asyncEventError, notice.Symbol, notice)
}

func (m *AsyncManager) SendPositionOpen(ctx context.Context, notice PositionOpenNotice) error {
	return m.enqueue(ctx, asyncEventPositionOpen, notice.Symbol, notice)
}

func (m *AsyncManager) SendPositionClose(ctx context.Context, notice PositionCloseNotice) error {
	return m.enqueue(ctx, asyncEventPositionClose, notice.Symbol, notice)
}

func (m *AsyncManager) SendPositionCloseSummary(ctx context.Context, notice PositionCloseSummaryNotice) error {
	return m.enqueue(ctx, asyncEventPositionCloseSummary, notice.Symbol, notice)
}

func (m *AsyncManager) SendRiskPlanUpdate(ctx context.Context, notice RiskPlanUpdateNotice) error {
	return m.enqueue(ctx, asyncEventRiskUpdate, notice.Symbol, notice)
}

func (m *AsyncManager) SendTradeOpen(ctx context.Context, notice TradeOpenNotice) error {
	return m.enqueue(ctx, asyncEventTradeOpen, notice.Pair, notice)
}

func (m *AsyncManager) SendTradePartialClose(ctx context.Context, notice TradePartialCloseNotice) error {
	return m.enqueue(ctx, asyncEventTradePartialClose, notice.Pair, notice)
}

func (m *AsyncManager) SendTradeCloseSummary(ctx context.Context, notice TradeCloseSummaryNotice) error {
	return m.enqueue(ctx, asyncEventTradeCloseSummary, notice.Pair, notice)
}

func (m *AsyncManager) Render(ctx context.Context, eventType, _ string, payload json.RawMessage) (json.RawMessage, error) {
	ctx, span := braleOtel.Tracer("brale-core/notify").Start(ctx, "brale.notify.render")
	span.SetAttributes(attribute.String("notify.event_type", eventType))
	defer span.End()
	return payload, nil
}

func (m *AsyncManager) Deliver(ctx context.Context, eventType, _ string, rendered json.RawMessage) error {
	ctx, span := braleOtel.Tracer("brale-core/notify").Start(ctx, "brale.notify.deliver")
	span.SetAttributes(attribute.String("notify.event_type", eventType))
	defer span.End()

	if m.sync == nil {
		return nil
	}
	var err error
	switch eventType {
	case asyncEventGate:
		var payload gateAsyncPayload
		if err = json.Unmarshal(rendered, &payload); err == nil {
			err = m.sync.SendGate(ctx, payload.Input, payload.Report)
		}
	case asyncEventPositionOpen:
		var notice PositionOpenNotice
		if err = json.Unmarshal(rendered, &notice); err == nil {
			err = m.sync.SendPositionOpen(ctx, notice)
		}
	case asyncEventPositionClose:
		var notice PositionCloseNotice
		if err = json.Unmarshal(rendered, &notice); err == nil {
			err = m.sync.SendPositionClose(ctx, notice)
		}
	case asyncEventPositionCloseSummary:
		var notice PositionCloseSummaryNotice
		if err = json.Unmarshal(rendered, &notice); err == nil {
			err = m.sync.SendPositionCloseSummary(ctx, notice)
		}
	case asyncEventRiskUpdate:
		var notice RiskPlanUpdateNotice
		if err = json.Unmarshal(rendered, &notice); err == nil {
			err = m.sync.SendRiskPlanUpdate(ctx, notice)
		}
	case asyncEventError:
		var notice ErrorNotice
		if err = json.Unmarshal(rendered, &notice); err == nil {
			err = m.sync.SendError(ctx, notice)
		}
	case asyncEventTradeOpen:
		var notice TradeOpenNotice
		if err = json.Unmarshal(rendered, &notice); err == nil {
			err = m.sync.SendTradeOpen(ctx, notice)
		}
	case asyncEventTradePartialClose:
		var notice TradePartialCloseNotice
		if err = json.Unmarshal(rendered, &notice); err == nil {
			err = m.sync.SendTradePartialClose(ctx, notice)
		}
	case asyncEventTradeCloseSummary:
		var notice TradeCloseSummaryNotice
		if err = json.Unmarshal(rendered, &notice); err == nil {
			err = m.sync.SendTradeCloseSummary(ctx, notice)
		}
	default:
		err = fmt.Errorf("unsupported notify event type: %s", eventType)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		braleOtel.NotifyFailTotal.Add(ctx, 1, otelmetric.WithAttributes(attribute.String("event_type", eventType)))
		return err
	}
	braleOtel.NotifyDeliverTotal.Add(ctx, 1, otelmetric.WithAttributes(attribute.String("event_type", eventType)))
	return nil
}

var _ Notifier = (*AsyncManager)(nil)
