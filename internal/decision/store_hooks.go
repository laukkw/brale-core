package decision

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"brale-core/internal/decision/agent"
	"brale-core/internal/decision/decisionfmt"
	"brale-core/internal/decision/fund"
	"brale-core/internal/decision/gate"
	"brale-core/internal/decision/provider"
	"brale-core/internal/llm"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/snapshot"
	"brale-core/internal/store"

	"go.uber.org/zap"
)

type StoreHooks struct {
	Store         store.Store
	SystemHash    string
	StrategyHash  string
	SourceVersion string
	Notifier      Notifier
	TraceDir      string
	TraceLogPath  string
	TraceEnabled  bool
	TraceRedacted bool
}

func (h StoreHooks) SaveAgent(ctx context.Context, snap snapshot.MarketSnapshot, snapID uint, sym string, ind agent.IndicatorSummary, st agent.StructureSummary, mech agent.MechanicsSummary, inputs AgentInputSet, enabled AgentEnabled, prompts AgentPromptSet) error {
	logger := logging.FromContext(ctx).Named("decision").With(zap.String("symbol", sym), zap.Uint("snapshot_id", snapID))
	if h.Store == nil {
		return fmt.Errorf("store is required")
	}
	ts := snap.Timestamp.Unix()
	if ts == 0 {
		ts = time.Now().Unix()
	}
	if enabled.Indicator {
		if err := h.saveAgentStage(ctx, snapID, sym, "indicator", ind, inputs.Indicator.RawJSON, ts, prompts.Indicator); err != nil {
			return err
		}
	}
	if enabled.Structure {
		if err := h.saveAgentStage(ctx, snapID, sym, "structure", st, inputs.Structure.RawJSON, ts, prompts.Structure); err != nil {
			return err
		}
	}
	if enabled.Mechanics {
		if err := h.saveAgentStage(ctx, snapID, sym, "mechanics", mech, inputs.Mechanics.RawJSON, ts, prompts.Mechanics); err != nil {
			return err
		}
	}
	logger.Debug("agent outputs saved")
	return nil
}

func (h StoreHooks) saveAgentStage(ctx context.Context, snapID uint, sym, stage string, payload any, inputRaw []byte, ts int64, prompt LLMStagePrompt) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	rec := &store.AgentEventRecord{
		SnapshotID:         snapID,
		Symbol:             sym,
		Timestamp:          ts,
		Stage:              stage,
		InputJSON:          json.RawMessage(cloneJSONBytes(inputRaw)),
		SystemPrompt:       prompt.System,
		UserPrompt:         prompt.User,
		OutputJSON:         json.RawMessage(raw),
		Fingerprint:        fmt.Sprintf("%x", sum[:]),
		SystemConfigHash:   h.SystemHash,
		StrategyConfigHash: h.StrategyHash,
		SourceVersion:      h.SourceVersion,
		Model:              prompt.Model,
		PromptVersion:      prompt.PromptVersion,
		LatencyMS:          prompt.LatencyMS,
		TokenIn:            prompt.TokenIn,
		TokenOut:           prompt.TokenOut,
		Error:              prompt.Error,
	}
	if roundID, ok := llm.SessionRoundIDFromContext(ctx); ok {
		rec.RoundID = roundID.String()
	}
	return h.Store.SaveAgentEvent(ctx, rec)
}

func (h StoreHooks) SaveProvider(ctx context.Context, snap snapshot.MarketSnapshot, snapID uint, sym string, providers fund.ProviderBundle, dataCtx ProviderDataContext, prompts ProviderPromptSet) error {
	logger := logging.FromContext(ctx).Named("decision").With(zap.String("symbol", sym), zap.Uint("snapshot_id", snapID))
	if h.Store == nil {
		return fmt.Errorf("store is required")
	}
	ts := snap.Timestamp.Unix()
	if ts == 0 {
		ts = time.Now().Unix()
	}
	indicatorTradeable := gate.IndicatorTradeable(gate.IndicatorAtomic{
		MomentumExpansion: providers.Indicator.MomentumExpansion,
		Alignment:         providers.Indicator.Alignment,
		MeanRevNoise:      providers.Indicator.MeanRevNoise,
	})
	structureTradeable := gate.StructureTradeable(gate.StructureAtomic{
		ClearStructure: providers.Structure.ClearStructure,
		Integrity:      providers.Structure.Integrity,
	})
	mechanicsTradeable := gate.MechanicsTradeable(gate.MechanicsAtomic{
		LiquidationStress: providers.Mechanics.LiquidationStress.Value,
		SignalTag:         providers.Mechanics.SignalTag,
	})
	if providers.Enabled.Indicator {
		if err := h.saveProviderStage(ctx, snapID, sym, "indicator", providers.Indicator, dataCtx.IndicatorCrossTF, indicatorTradeable, ts, prompts.Indicator); err != nil {
			return err
		}
	}
	if providers.Enabled.Structure {
		if err := h.saveProviderStage(ctx, snapID, sym, "structure", providers.Structure, dataCtx.StructureAnchorCtx, structureTradeable, ts, prompts.Structure); err != nil {
			return err
		}
	}
	if providers.Enabled.Mechanics {
		if err := h.saveProviderStage(ctx, snapID, sym, "mechanics", providers.Mechanics, dataCtx.MechanicsCtx, mechanicsTradeable, ts, prompts.Mechanics); err != nil {
			return err
		}
	}
	logger.Debug("provider outputs saved",
		zap.Bool("indicator_tradeable", providers.Enabled.Indicator && indicatorTradeable),
		zap.Bool("structure_tradeable", providers.Enabled.Structure && structureTradeable),
		zap.Bool("mechanics_tradeable", providers.Enabled.Mechanics && mechanicsTradeable),
	)
	return nil
}

func (h StoreHooks) saveProviderStage(ctx context.Context, snapID uint, sym, role string, payload any, dataCtx any, tradeable bool, ts int64, prompt LLMStagePrompt) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	dataCtxRaw, err := marshalContextJSON(dataCtx)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	rec := &store.ProviderEventRecord{
		SnapshotID:         snapID,
		Symbol:             sym,
		Timestamp:          ts,
		Role:               role,
		DataContextJSON:    json.RawMessage(dataCtxRaw),
		SystemPrompt:       prompt.System,
		UserPrompt:         prompt.User,
		OutputJSON:         json.RawMessage(raw),
		Tradeable:          tradeable,
		Fingerprint:        fmt.Sprintf("%x", sum[:]),
		SystemConfigHash:   h.SystemHash,
		StrategyConfigHash: h.StrategyHash,
		SourceVersion:      h.SourceVersion,
		Model:              prompt.Model,
		PromptVersion:      prompt.PromptVersion,
		LatencyMS:          prompt.LatencyMS,
		TokenIn:            prompt.TokenIn,
		TokenOut:           prompt.TokenOut,
		Error:              prompt.Error,
	}
	if roundID, ok := llm.SessionRoundIDFromContext(ctx); ok {
		rec.RoundID = roundID.String()
	}
	return h.Store.SaveProviderEvent(ctx, rec)
}

func (h StoreHooks) SaveProviderInPosition(ctx context.Context, snap snapshot.MarketSnapshot, snapID uint, sym string, ind provider.InPositionIndicatorOut, st provider.InPositionStructureOut, mech provider.InPositionMechanicsOut, prompts ProviderPromptSet, enabled AgentEnabled) error {
	logger := logging.FromContext(ctx).Named("decision").With(zap.String("symbol", sym), zap.Uint("snapshot_id", snapID))
	if h.Store == nil {
		return fmt.Errorf("store is required")
	}
	ts := snap.Timestamp.Unix()
	if ts == 0 {
		ts = time.Now().Unix()
	}
	if enabled.Indicator {
		if err := h.saveProviderStage(ctx, snapID, sym, "indicator_in_position", ind, nil, false, ts, prompts.Indicator); err != nil {
			return err
		}
	}
	if enabled.Structure {
		if err := h.saveProviderStage(ctx, snapID, sym, "structure_in_position", st, nil, false, ts, prompts.Structure); err != nil {
			return err
		}
	}
	if enabled.Mechanics {
		if err := h.saveProviderStage(ctx, snapID, sym, "mechanics_in_position", mech, nil, false, ts, prompts.Mechanics); err != nil {
			return err
		}
	}
	logger.Debug("provider in position outputs saved")
	return nil
}

func (h StoreHooks) SaveGate(ctx context.Context, snap snapshot.MarketSnapshot, snapID uint, sym string, gate fund.GateDecision, providers fund.ProviderBundle) error {
	logger := logging.FromContext(ctx).Named("decision").With(zap.String("symbol", sym), zap.Uint("snapshot_id", snapID))
	if h.Store == nil {
		return fmt.Errorf("store is required")
	}
	ts := snap.Timestamp.Unix()
	if ts == 0 {
		ts = time.Now().Unix()
	}
	refJSON, _ := buildGateProviderRefs(gate, providers)
	ruleJSON := []byte("{}")
	if gate.RuleHit != nil {
		if raw, err := json.Marshal(gate.RuleHit); err == nil {
			ruleJSON = raw
		}
	}
	derivedJSON := []byte("{}")
	if len(gate.Derived) > 0 {
		if raw, err := json.Marshal(gate.Derived); err == nil {
			derivedJSON = raw
		}
	}
	rec := &store.GateEventRecord{
		SnapshotID:         snapID,
		Symbol:             sym,
		Timestamp:          ts,
		GlobalTradeable:    gate.GlobalTradeable,
		DecisionAction:     gate.DecisionAction,
		Grade:              gate.Grade,
		GateReason:         gate.GateReason,
		Direction:          gate.Direction,
		ProviderRefsJSON:   json.RawMessage(refJSON),
		RuleHitJSON:        json.RawMessage(ruleJSON),
		DerivedJSON:        json.RawMessage(derivedJSON),
		Fingerprint:        fmt.Sprintf("%x", sha256.Sum256(refJSON)),
		SystemConfigHash:   h.SystemHash,
		StrategyConfigHash: h.StrategyHash,
		SourceVersion:      h.SourceVersion,
	}
	if roundID, ok := llm.SessionRoundIDFromContext(ctx); ok {
		rec.RoundID = roundID.String()
	}

	if err := h.Store.SaveGateEvent(ctx, rec); err != nil {
		logger.Error("save gate event failed", zap.Error(err))
		return err
	}
	logger.Info("gate decision saved",
		zap.Bool("global_tradeable", gate.GlobalTradeable),
		zap.String("gate_reason", gate.GateReason),
		zap.String("direction", gate.Direction),
	)
	if h.TraceEnabled {
		path, err := h.writeRoundTraceMarkdown(ctx, rec)
		if err != nil {
			logger.Error("write round trace markdown failed", zap.Error(err))
		} else {
			logger.Info("round trace markdown saved", zap.String("path", path))
		}
	}
	if err := h.notifyGate(ctx, rec); err != nil {
		logger.Error("gate notify failed", zap.Error(err))
		if h.Notifier != nil {
			if notifyErr := h.Notifier.SendError(ctx, ErrorNotice{Component: "decision", Message: err.Error()}); notifyErr != nil {
				logger.Error("notify error failed", zap.Error(notifyErr))
			}
		}
		return err
	}
	logger.Info("gate notify sent",
		zap.String("gate_reason", gate.GateReason),
		zap.String("direction", gate.Direction),
	)
	return nil
}

func buildGateProviderRefs(gate fund.GateDecision, providers fund.ProviderBundle) ([]byte, error) {
	if strings.EqualFold(strings.TrimSpace(fmt.Sprint(gate.Derived["gate_trace_mode"])), "monitor") {
		return json.Marshal(map[string]any{})
	}
	ref := struct {
		Indicator provider.IndicatorProviderOut `json:"indicator"`
		Structure provider.StructureProviderOut `json:"structure"`
		Mechanics provider.MechanicsProviderOut `json:"mechanics"`
	}{
		Indicator: providers.Indicator,
		Structure: providers.Structure,
		Mechanics: providers.Mechanics,
	}
	return json.Marshal(ref)
}

func cloneJSONBytes(raw []byte) []byte {
	if len(raw) == 0 {
		return nil
	}
	return slices.Clone(raw)
}

func marshalContextJSON(data any) ([]byte, error) {
	if data == nil {
		return nil, nil
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	if string(raw) == "null" || string(raw) == "{}" {
		return nil, nil
	}
	return raw, nil
}

func (h StoreHooks) notifyGate(ctx context.Context, rec *store.GateEventRecord) error {
	if h.Notifier == nil {
		return nil
	}
	if h.Store == nil {
		return fmt.Errorf("store is required")
	}
	if rec == nil {
		return fmt.Errorf("gate record is required")
	}
	providers, err := h.Store.ListProviderEventsBySnapshot(ctx, rec.Symbol, rec.SnapshotID)
	if err != nil {
		return err
	}
	agents, err := h.Store.ListAgentEventsBySnapshot(ctx, rec.Symbol, rec.SnapshotID)
	if err != nil {
		return err
	}
	formatter := decisionfmt.New()
	input := decisionfmt.DecisionInput{
		Symbol:     rec.Symbol,
		SnapshotID: rec.SnapshotID,
		Gate:       toDecisionGateEvent(*rec),
		Providers:  toDecisionProviderEvents(providers),
		Agents:     toDecisionAgentEvents(agents),
	}
	report, err := formatter.BuildDecisionReport(input)
	if err != nil {
		return err
	}
	return h.Notifier.SendGate(ctx, input, report)
}

func toDecisionGateEvent(rec store.GateEventRecord) decisionfmt.GateEvent {
	return decisionfmt.GateEvent{
		ID:               rec.ID,
		SnapshotID:       rec.SnapshotID,
		GlobalTradeable:  rec.GlobalTradeable,
		DecisionAction:   rec.DecisionAction,
		Grade:            rec.Grade,
		GateReason:       rec.GateReason,
		Direction:        rec.Direction,
		ProviderRefsJSON: json.RawMessage(rec.ProviderRefsJSON),
		RuleHitJSON:      json.RawMessage(rec.RuleHitJSON),
		DerivedJSON:      json.RawMessage(rec.DerivedJSON),
	}
}

func toDecisionProviderEvents(records []store.ProviderEventRecord) []decisionfmt.ProviderEvent {
	if len(records) == 0 {
		return nil
	}
	out := make([]decisionfmt.ProviderEvent, 0, len(records))
	for _, rec := range records {
		out = append(out, decisionfmt.ProviderEvent{
			SnapshotID: rec.SnapshotID,
			OutputJSON: json.RawMessage(rec.OutputJSON),
			Role:       rec.Role,
		})
	}
	return out
}

func toDecisionAgentEvents(records []store.AgentEventRecord) []decisionfmt.AgentEvent {
	if len(records) == 0 {
		return nil
	}
	out := make([]decisionfmt.AgentEvent, 0, len(records))
	for _, rec := range records {
		out = append(out, decisionfmt.AgentEvent{
			SnapshotID: rec.SnapshotID,
			OutputJSON: json.RawMessage(rec.OutputJSON),
			Stage:      rec.Stage,
		})
	}
	return out
}
