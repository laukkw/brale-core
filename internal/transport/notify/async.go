package notify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

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
	tradeLifecycleNotifyDelay      = 5 * time.Second
)

type gateAsyncPayload struct {
	Input  decisionfmt.DecisionInput  `json:"input"`
	Report decisionfmt.DecisionReport `json:"report"`
}

type AsyncManager struct {
	mu     sync.RWMutex
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
		m.mu.Lock()
		defer m.mu.Unlock()
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

	data, err := json.Marshal(payload)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("marshal async notify payload: %w", err)
	}
	client := m.riverClient()
	if client == nil {
		if err := m.deliverSynchronously(ctx, eventType, symbol, json.RawMessage(data)); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return fmt.Errorf("send async notification synchronously before river is ready: %w", err)
		}
		return nil
	}
	args := jobs.NotifyRenderArgs{
		EventType: eventType,
		Symbol:    symbol,
		Payload:   json.RawMessage(data),
	}
	if err := m.enqueueJob(ctx, client, args, notifyRenderInsertOpts(eventType)); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("enqueue async notification: %w", err)
	}
	braleOtel.NotifyEnqueueTotal.Add(ctx, 1, otelmetric.WithAttributes(attribute.String("event_type", eventType)))
	return nil
}

func (m *AsyncManager) riverClient() *river.Client[pgx.Tx] {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.client
}

func (m *AsyncManager) enqueueJob(ctx context.Context, client *river.Client[pgx.Tx], args jobs.NotifyRenderArgs, opts *river.InsertOpts) error {
	if tx := notifyport.TxFromContext(ctx); tx != nil {
		_, err := client.InsertTx(ctx, tx, args, opts)
		return err
	}
	_, err := client.Insert(ctx, args, opts)
	return err
}

func notifyRenderInsertOpts(eventType string) *river.InsertOpts {
	if !shouldDelayTradeLifecycleNotify(eventType) {
		return nil
	}
	scheduledAt := time.Now().Add(tradeLifecycleNotifyDelay)
	opts := (jobs.NotifyRenderArgs{}).InsertOpts()
	opts.ScheduledAt = scheduledAt
	return &opts
}

func shouldDelayTradeLifecycleNotify(eventType string) bool {
	switch eventType {
	case asyncEventPositionOpen,
		asyncEventPositionClose,
		asyncEventPositionCloseSummary,
		asyncEventTradeOpen,
		asyncEventTradePartialClose,
		asyncEventTradeCloseSummary:
		return true
	default:
		return false
	}
}

func (m *AsyncManager) EnqueueRendered(ctx context.Context, eventType, symbol string, rendered json.RawMessage) error {
	client := m.riverClient()
	if client == nil {
		return fmt.Errorf("river client is required for notify delivery enqueue")
	}
	argsList := buildNotifyDeliverArgs(eventType, symbol, rendered, m.deliveryChannels())
	for _, args := range argsList {
		if err := m.enqueueDeliverJob(ctx, client, args); err != nil {
			return fmt.Errorf("enqueue deliver notification channel %q: %w", args.Channel, err)
		}
	}
	return nil
}

func (m *AsyncManager) enqueueDeliverJob(ctx context.Context, client *river.Client[pgx.Tx], args jobs.NotifyDeliverArgs) error {
	if tx := notifyport.TxFromContext(ctx); tx != nil {
		_, err := client.InsertTx(ctx, tx, args, nil)
		return err
	}
	_, err := client.Insert(ctx, args, nil)
	return err
}

type deliveryChannelProvider interface {
	DeliveryChannels() []string
}

type channelScopedNotifier interface {
	NotifierForChannel(channel string) (Notifier, bool)
}

func (m *AsyncManager) deliveryChannels() []string {
	if m == nil || m.sync == nil {
		return nil
	}
	provider, ok := m.sync.(deliveryChannelProvider)
	if !ok {
		return nil
	}
	return provider.DeliveryChannels()
}

func buildNotifyDeliverArgs(eventType, symbol string, rendered json.RawMessage, channels []string) []jobs.NotifyDeliverArgs {
	if len(channels) == 0 {
		return []jobs.NotifyDeliverArgs{{
			EventType: eventType,
			Symbol:    symbol,
			Rendered:  append(json.RawMessage(nil), rendered...),
			DedupeKey: notifyDeliverDedupeKey(eventType, symbol, rendered, ""),
		}}
	}
	out := make([]jobs.NotifyDeliverArgs, 0, len(channels))
	for _, channel := range channels {
		channel = normalizeNotifyChannel(channel)
		if channel == "" {
			continue
		}
		out = append(out, jobs.NotifyDeliverArgs{
			EventType: eventType,
			Symbol:    symbol,
			Rendered:  append(json.RawMessage(nil), rendered...),
			Channel:   channel,
			DedupeKey: notifyDeliverDedupeKey(eventType, symbol, rendered, channel),
		})
	}
	if len(out) == 0 {
		return buildNotifyDeliverArgs(eventType, symbol, rendered, nil)
	}
	return out
}

func notifyDeliverDedupeKey(eventType, symbol string, rendered json.RawMessage, channel string) string {
	sum := sha256.Sum256(rendered)
	parts := []string{
		"notify",
		normalizeNotifyChannel(eventType),
		strings.ToUpper(strings.TrimSpace(symbol)),
		hex.EncodeToString(sum[:8]),
	}
	if channel != "" {
		parts = append(parts, normalizeNotifyChannel(channel))
	}
	return strings.Join(parts, ":")
}

func (m *AsyncManager) deliverSynchronously(ctx context.Context, eventType, symbol string, rendered json.RawMessage) error {
	channels := m.deliveryChannels()
	if len(channels) == 0 {
		return m.Deliver(ctx, eventType, symbol, "", rendered)
	}
	errDetails := make([]string, 0)
	for _, channel := range channels {
		if err := m.Deliver(ctx, eventType, symbol, channel, rendered); err != nil {
			errDetails = append(errDetails, fmt.Sprintf("%s: %v", channel, err))
		}
	}
	if len(errDetails) > 0 {
		return fmt.Errorf("sync notify delivery failed: %s", strings.Join(errDetails, "; "))
	}
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

func (m *AsyncManager) Deliver(ctx context.Context, eventType, _ string, channel string, rendered json.RawMessage) error {
	ctx, span := braleOtel.Tracer("brale-core/notify").Start(ctx, "brale.notify.deliver")
	span.SetAttributes(
		attribute.String("notify.event_type", eventType),
		attribute.String("notify.channel", strings.TrimSpace(channel)),
	)
	defer span.End()

	notifier, err := m.notifierForChannel(channel)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		braleOtel.NotifyFailTotal.Add(ctx, 1, otelmetric.WithAttributes(attribute.String("event_type", eventType)))
		return err
	}
	if notifier == nil {
		return nil
	}
	switch eventType {
	case asyncEventGate:
		var payload gateAsyncPayload
		if err = json.Unmarshal(rendered, &payload); err == nil {
			err = notifier.SendGate(ctx, payload.Input, payload.Report)
		}
	case asyncEventPositionOpen:
		var notice PositionOpenNotice
		if err = json.Unmarshal(rendered, &notice); err == nil {
			err = notifier.SendPositionOpen(ctx, notice)
		}
	case asyncEventPositionClose:
		var notice PositionCloseNotice
		if err = json.Unmarshal(rendered, &notice); err == nil {
			err = notifier.SendPositionClose(ctx, notice)
		}
	case asyncEventPositionCloseSummary:
		var notice PositionCloseSummaryNotice
		if err = json.Unmarshal(rendered, &notice); err == nil {
			err = notifier.SendPositionCloseSummary(ctx, notice)
		}
	case asyncEventRiskUpdate:
		var notice RiskPlanUpdateNotice
		if err = json.Unmarshal(rendered, &notice); err == nil {
			err = notifier.SendRiskPlanUpdate(ctx, notice)
		}
	case asyncEventError:
		var notice ErrorNotice
		if err = json.Unmarshal(rendered, &notice); err == nil {
			err = notifier.SendError(ctx, notice)
		}
	case asyncEventTradeOpen:
		var notice TradeOpenNotice
		if err = json.Unmarshal(rendered, &notice); err == nil {
			err = notifier.SendTradeOpen(ctx, notice)
		}
	case asyncEventTradePartialClose:
		var notice TradePartialCloseNotice
		if err = json.Unmarshal(rendered, &notice); err == nil {
			err = notifier.SendTradePartialClose(ctx, notice)
		}
	case asyncEventTradeCloseSummary:
		var notice TradeCloseSummaryNotice
		if err = json.Unmarshal(rendered, &notice); err == nil {
			err = notifier.SendTradeCloseSummary(ctx, notice)
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

func (m *AsyncManager) notifierForChannel(channel string) (Notifier, error) {
	if m == nil || m.sync == nil {
		return nil, nil
	}
	channel = normalizeNotifyChannel(channel)
	if channel == "" {
		return m.sync, nil
	}
	scoped, ok := m.sync.(channelScopedNotifier)
	if !ok {
		return nil, fmt.Errorf("notify channel %q requested but sync notifier does not support channel routing", channel)
	}
	notifier, ok := scoped.NotifierForChannel(channel)
	if !ok {
		return nil, fmt.Errorf("notify channel %q is not configured", channel)
	}
	return notifier, nil
}

var _ Notifier = (*AsyncManager)(nil)
