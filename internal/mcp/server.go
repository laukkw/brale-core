package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/decision/decisionutil"
	"brale-core/internal/decision/features"
	"brale-core/internal/memory"
	"brale-core/internal/pgstore"
	"brale-core/internal/snapshot"
	"brale-core/internal/store"
	"brale-core/internal/transport/botruntime"
	runtimeapi "brale-core/internal/transport/runtimeapi"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultEpisodicMemoryToolLimit = 10
	maxEpisodicMemoryToolLimit     = 50
)

type Runtime interface {
	FetchObserveReport(context.Context, string) (botruntime.ObserveResponse, error)
	FetchDecisionLatest(context.Context, string) (botruntime.DecisionLatestResponse, error)
	FetchPositionStatus(context.Context) (botruntime.PositionStatusResponse, error)
	FetchDashboardDecisionHistory(context.Context, string, int, uint) (botruntime.DashboardDecisionHistoryResponse, error)
	FetchDashboardAccountSummary(context.Context) (botruntime.DashboardAccountSummaryResponse, error)
	FetchDashboardKline(context.Context, string, string, int) (botruntime.DashboardKlineResponse, error)
	FetchDashboardOverview(context.Context, string) (botruntime.DashboardOverviewResponse, error)
}

type Options struct {
	Name          string
	Version       string
	Runtime       Runtime
	Config        *LocalConfigSource
	Audit         AuditSink
	EpisodicStore store.EpisodicMemoryStore
}

type service struct {
	runtime       Runtime
	config        *LocalConfigSource
	episodicStore store.EpisodicMemoryStore
}

type analyzeMarketInput struct {
	Symbol string `json:"symbol" jsonschema:"交易对，例如 BTCUSDT"`
}

type symbolInput struct {
	Symbol string `json:"symbol" jsonschema:"交易对，例如 BTCUSDT"`
}

type decisionHistoryInput struct {
	Symbol     string `json:"symbol" jsonschema:"交易对，例如 BTCUSDT"`
	Limit      int    `json:"limit,omitempty" jsonschema:"返回条数，默认 20"`
	SnapshotID uint   `json:"snapshot_id,omitempty" jsonschema:"可选 snapshot_id 过滤"`
}

type klineInput struct {
	Symbol   string `json:"symbol" jsonschema:"交易对，例如 BTCUSDT"`
	Interval string `json:"interval" jsonschema:"K 线周期，例如 1h"`
	Limit    int    `json:"limit,omitempty" jsonschema:"返回 K 线条数"`
}

type getConfigInput struct {
	Symbol string `json:"symbol,omitempty" jsonschema:"可选；指定后额外返回 symbol/strategy 配置"`
}

type episodicMemoryInput struct {
	Symbol string `json:"symbol" jsonschema:"交易对，例如 BTCUSDT"`
	Limit  int    `json:"limit,omitempty" jsonschema:"返回条数，默认 10，最大 50"`
}

func NewServer(opts Options) (*sdkmcp.Server, error) {
	if opts.Runtime == nil {
		return nil, fmt.Errorf("runtime is required")
	}
	if opts.Config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if opts.Audit == nil {
		return nil, fmt.Errorf("audit sink is required")
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = defaultServerName
	}
	version := strings.TrimSpace(opts.Version)
	if version == "" {
		version = "dev"
	}
	svc := service{
		runtime:       opts.Runtime,
		config:        opts.Config,
		episodicStore: opts.EpisodicStore,
	}
	server := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: name, Version: version},
		&sdkmcp.ServerOptions{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))},
	)
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "analyze_market",
		Description: "返回指定交易对的市场总览、最新决策和最近一次观察结果。",
	}, audited("analyze_market", opts.Audit, svc.handleAnalyzeMarket))
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "get_latest_decision",
		Description: "获取指定交易对最近一次决策结果。",
	}, audited("get_latest_decision", opts.Audit, svc.handleGetLatestDecision))
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "get_positions",
		Description: "获取当前持仓列表。",
	}, audited("get_positions", opts.Audit, svc.handleGetPositions))
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "get_decision_history",
		Description: "获取指定交易对的历史决策记录。",
	}, audited("get_decision_history", opts.Audit, svc.handleGetDecisionHistory))
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "get_account_summary",
		Description: "获取账户余额与总体盈亏摘要。",
	}, audited("get_account_summary", opts.Audit, svc.handleGetAccountSummary))
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "get_kline",
		Description: "获取指定交易对的 K 线数据。",
	}, audited("get_kline", opts.Audit, svc.handleGetKline))
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "compute_indicators",
		Description: "基于本地 symbol 配置与 runtime K 线计算指标快照。",
	}, audited("compute_indicators", opts.Audit, svc.handleComputeIndicators))
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "get_config",
		Description: "读取本地配置并返回脱敏后的 system/index/symbol/strategy 内容。",
	}, audited("get_config", opts.Audit, svc.handleGetConfig))
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "get_episodic_memory",
		Description: "读取指定交易对最近的交易反思记录（只读）。",
	}, audited("get_episodic_memory", opts.Audit, svc.handleGetEpisodicMemory))
	registerResources(server, opts.Audit, svc)
	registerPrompts(server, opts.Audit, svc)
	return server, nil
}

func Serve(ctx context.Context, opts Options) error {
	server, err := NewServer(opts)
	if err != nil {
		return err
	}
	return server.Run(ctx, &sdkmcp.StdioTransport{})
}

func (s service) handleAnalyzeMarket(ctx context.Context, _ *sdkmcp.CallToolRequest, input analyzeMarketInput) (*sdkmcp.CallToolResult, map[string]any, error) {
	symbol, err := normalizeSymbol(input.Symbol)
	if err != nil {
		return nil, nil, err
	}
	overview, err := s.runtime.FetchDashboardOverview(ctx, symbol)
	if err != nil {
		return nil, nil, err
	}
	latest, err := s.runtime.FetchDecisionLatest(ctx, symbol)
	if err != nil {
		return nil, nil, err
	}
	overviewMap, err := asObject(overview)
	if err != nil {
		return nil, nil, err
	}
	latestMap, err := asObject(latest)
	if err != nil {
		return nil, nil, err
	}
	out := map[string]any{
		"symbol":            symbol,
		"overview":          overviewMap,
		"latest_decision":   latestMap,
		"observe_available": false,
	}
	observe, observeErr := s.runtime.FetchObserveReport(ctx, symbol)
	if observeErr != nil {
		out["observe_error"] = fmt.Sprintf("observe report unavailable: %v", observeErr)
		return nil, out, nil
	}
	if observe.Gate == nil {
		observe.Gate = map[string]any{}
	}
	observeMap, err := asObject(observe)
	if err != nil {
		return nil, nil, err
	}
	out["observe_available"] = true
	out["observe"] = observeMap
	return nil, out, nil
}

func (s service) handleGetLatestDecision(ctx context.Context, _ *sdkmcp.CallToolRequest, input symbolInput) (*sdkmcp.CallToolResult, map[string]any, error) {
	symbol, err := normalizeSymbol(input.Symbol)
	if err != nil {
		return nil, nil, err
	}
	out, err := s.runtime.FetchDecisionLatest(ctx, symbol)
	if err != nil {
		return nil, nil, err
	}
	obj, err := asObject(out)
	return nil, obj, err
}

func (s service) handleGetPositions(ctx context.Context, _ *sdkmcp.CallToolRequest, _ struct{}) (*sdkmcp.CallToolResult, map[string]any, error) {
	out, err := s.runtime.FetchPositionStatus(ctx)
	if err != nil {
		return nil, nil, err
	}
	obj, err := asObject(out)
	return nil, obj, err
}

func (s service) handleGetDecisionHistory(ctx context.Context, _ *sdkmcp.CallToolRequest, input decisionHistoryInput) (*sdkmcp.CallToolResult, map[string]any, error) {
	symbol, err := normalizeSymbol(input.Symbol)
	if err != nil {
		return nil, nil, err
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}
	out, err := s.runtime.FetchDashboardDecisionHistory(ctx, symbol, limit, input.SnapshotID)
	if err != nil {
		return nil, nil, err
	}
	obj, err := asObject(out)
	return nil, obj, err
}

func (s service) handleGetAccountSummary(ctx context.Context, _ *sdkmcp.CallToolRequest, _ struct{}) (*sdkmcp.CallToolResult, map[string]any, error) {
	out, err := s.runtime.FetchDashboardAccountSummary(ctx)
	if err != nil {
		return nil, nil, err
	}
	obj, err := asObject(out)
	return nil, obj, err
}

func (s service) handleGetKline(ctx context.Context, _ *sdkmcp.CallToolRequest, input klineInput) (*sdkmcp.CallToolResult, map[string]any, error) {
	symbol, err := normalizeSymbol(input.Symbol)
	if err != nil {
		return nil, nil, err
	}
	interval := strings.TrimSpace(input.Interval)
	if interval == "" {
		return nil, nil, fmt.Errorf("interval is required")
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 120
	}
	out, err := s.runtime.FetchDashboardKline(ctx, symbol, interval, limit)
	if err != nil {
		return nil, nil, err
	}
	obj, err := asObject(out)
	return nil, obj, err
}

func (s service) handleComputeIndicators(ctx context.Context, _ *sdkmcp.CallToolRequest, input klineInput) (*sdkmcp.CallToolResult, map[string]any, error) {
	symbol, err := normalizeSymbol(input.Symbol)
	if err != nil {
		return nil, nil, err
	}
	interval := strings.TrimSpace(input.Interval)
	if interval == "" {
		return nil, nil, fmt.Errorf("interval is required")
	}
	spec, err := s.config.LoadIndicatorSpec(symbol)
	if err != nil {
		return nil, nil, err
	}
	computer, err := features.IndicatorComputerForEngine(spec.Engine)
	if err != nil {
		return nil, nil, err
	}
	limit := input.Limit
	if limit <= 0 {
		limit = spec.KlineLimit
	}
	required := indicatorRequiredLimit(spec.Options)
	if limit < required {
		return nil, nil, fmt.Errorf("limit %d is below required bars %d", limit, required)
	}
	resp, err := s.runtime.FetchDashboardKline(ctx, symbol, interval, limit)
	if err != nil {
		return nil, nil, err
	}
	candles := toSnapshotCandles(resp.Candles)
	out, err := features.BuildIndicatorCompressedInputWithComputer(symbol, interval, candles, spec.Options, computer)
	if err != nil {
		return nil, nil, err
	}
	obj, err := asObject(out)
	return nil, obj, err
}

func (s service) handleGetConfig(_ context.Context, _ *sdkmcp.CallToolRequest, input getConfigInput) (*sdkmcp.CallToolResult, map[string]any, error) {
	out, err := s.config.LoadConfigView(input.Symbol)
	if err != nil {
		return nil, nil, err
	}
	return nil, buildConfigOutput(out), nil
}

func (s service) handleGetEpisodicMemory(ctx context.Context, _ *sdkmcp.CallToolRequest, input episodicMemoryInput) (*sdkmcp.CallToolResult, map[string]any, error) {
	symbol, err := normalizeSymbol(input.Symbol)
	if err != nil {
		return nil, nil, err
	}
	limit := input.Limit
	if limit <= 0 {
		limit = defaultEpisodicMemoryToolLimit
	}
	if limit > maxEpisodicMemoryToolLimit {
		return nil, nil, fmt.Errorf("limit must be in [1,%d]", maxEpisodicMemoryToolLimit)
	}

	store, closeStore, err := s.openEpisodicStore(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer closeStore()

	mem := memory.NewEpisodicMemory(store, limit, config.DefaultEpisodicTTLDays)
	episodes, err := mem.ListEpisodes(symbol, limit)
	if err != nil {
		return nil, nil, err
	}
	return nil, map[string]any{
		"symbol": symbol,
		"limit":  limit,
		"count":  len(episodes),
		"items":  buildEpisodicMemoryItems(episodes),
	}, nil
}

func (s service) openEpisodicStore(ctx context.Context) (store.EpisodicMemoryStore, func(), error) {
	if s.episodicStore != nil {
		return s.episodicStore, func() {}, nil
	}
	if s.config == nil {
		return nil, nil, fmt.Errorf("config is required")
	}
	sys, err := config.LoadSystemConfig(s.config.SystemPath)
	if err != nil {
		return nil, nil, fmt.Errorf("load system config: %w", err)
	}
	pool, err := pgstore.OpenPool(ctx, sys.Database)
	if err != nil {
		return nil, nil, fmt.Errorf("open memory store: %w", err)
	}
	st := pgstore.New(pool, nil)
	return st, st.Close, nil
}

func audited[In, Out any](tool string, audit AuditSink, next sdkmcp.ToolHandlerFor[In, Out]) sdkmcp.ToolHandlerFor[In, Out] {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest, input In) (*sdkmcp.CallToolResult, Out, error) {
		var zero Out
		start := time.Now()
		res, out, err := next(ctx, req, input)
		event := AuditEvent{
			At:         time.Now().UTC(),
			Tool:       tool,
			Arguments:  input,
			DurationMS: time.Since(start).Milliseconds(),
			Success:    err == nil && (res == nil || !res.IsError),
		}
		if err != nil {
			event.Error = err.Error()
		}
		if auditErr := audit.Record(ctx, event); auditErr != nil {
			if err != nil {
				return nil, zero, fmt.Errorf("%v (audit: %w)", err, auditErr)
			}
			return nil, zero, fmt.Errorf("record audit event: %w", auditErr)
		}
		return res, out, err
	}
}

func normalizeSymbol(raw string) (string, error) {
	symbol := decisionutil.NormalizeSymbol(raw)
	if symbol == "" {
		return "", fmt.Errorf("symbol is required")
	}
	return symbol, nil
}

func indicatorRequiredLimit(opts features.IndicatorCompressOptions) int {
	required := 0
	candidates := []int{
		config.EMARequiredBars(opts.EMAFast),
		config.EMARequiredBars(opts.EMAMid),
		config.EMARequiredBars(opts.EMASlow),
		config.RSIRequiredBars(opts.RSIPeriod),
		config.ATRRequiredBars(opts.ATRPeriod),
		config.BBRequiredBars(opts.BBPeriod),
		config.CHOPRequiredBars(opts.CHOPPeriod),
		config.StochRSIRequiredBars(opts.RSIPeriod, opts.StochRSIPeriod),
		config.AroonRequiredBars(opts.AroonPeriod),
	}
	if !opts.SkipSTC {
		candidates = append(candidates, config.STCRequiredBars(opts.STCFast, opts.STCSlow))
	}
	for _, candidate := range candidates {
		if candidate > required {
			required = candidate
		}
	}
	return required
}

func toSnapshotCandles(candles []runtimeapi.DashboardCandle) []snapshot.Candle {
	out := make([]snapshot.Candle, 0, len(candles))
	for _, candle := range candles {
		out = append(out, snapshot.Candle{
			OpenTime: candle.OpenTime,
			Open:     candle.Open,
			High:     candle.High,
			Low:      candle.Low,
			Close:    candle.Close,
			Volume:   candle.Volume,
		})
	}
	return out
}

func asObject(v any) (map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal output: %w", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("unmarshal output: %w", err)
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func buildConfigOutput(view ConfigView) map[string]any {
	system, _ := asObject(view.System)
	out := map[string]any{
		"system_path": view.SystemPath,
		"index_path":  view.IndexPath,
		"system":      system,
		"index":       buildIndexEntries(view.Index),
	}
	if view.Symbol != nil {
		out["symbol"] = buildSymbolOutput(*view.Symbol)
	}
	if view.Strategy != nil {
		out["strategy"] = buildStrategyOutput(*view.Strategy)
	}
	return out
}

func buildIndexEntries(entries []config.SymbolIndexEntry) []map[string]any {
	out := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		out = append(out, map[string]any{
			"symbol":   entry.Symbol,
			"config":   entry.Config,
			"strategy": entry.Strategy,
		})
	}
	return out
}

func buildEpisodicMemoryItems(episodes []memory.Episode) []map[string]any {
	out := make([]map[string]any, 0, len(episodes))
	for _, episode := range episodes {
		out = append(out, map[string]any{
			"id":             episode.ID,
			"symbol":         episode.Symbol,
			"position_id":    episode.PositionID,
			"direction":      episode.Direction,
			"entry_price":    episode.EntryPrice,
			"exit_price":     episode.ExitPrice,
			"pnl_percent":    episode.PnLPercent,
			"duration":       episode.Duration,
			"reflection":     episode.Reflection,
			"key_lessons":    episode.KeyLessons,
			"market_context": episode.MarketContext,
			"created_at":     episode.CreatedAt,
		})
	}
	return out
}

func buildSymbolOutput(cfg config.SymbolConfig) map[string]any {
	return map[string]any{
		"symbol":      cfg.Symbol,
		"intervals":   cfg.Intervals,
		"kline_limit": cfg.KlineLimit,
		"agent": map[string]any{
			"indicator": boolValue(cfg.Agent.Indicator),
			"structure": boolValue(cfg.Agent.Structure),
			"mechanics": boolValue(cfg.Agent.Mechanics),
		},
		"require": map[string]any{
			"oi":           cfg.Require.OI,
			"funding":      cfg.Require.Funding,
			"long_short":   cfg.Require.LongShort,
			"fear_greed":   cfg.Require.FearGreed,
			"liquidations": cfg.Require.Liquidations,
		},
		"indicators": map[string]any{
			"engine":           cfg.Indicators.Engine,
			"shadow_engine":    cfg.Indicators.ShadowEngine,
			"ema_fast":         cfg.Indicators.EMAFast,
			"ema_mid":          cfg.Indicators.EMAMid,
			"ema_slow":         cfg.Indicators.EMASlow,
			"rsi_period":       cfg.Indicators.RSIPeriod,
			"atr_period":       cfg.Indicators.ATRPeriod,
			"stc_fast":         cfg.Indicators.STCFast,
			"stc_slow":         cfg.Indicators.STCSlow,
			"bb_period":        cfg.Indicators.BBPeriod,
			"bb_multiplier":    cfg.Indicators.BBMultiplier,
			"chop_period":      cfg.Indicators.CHOPPeriod,
			"stoch_rsi_period": cfg.Indicators.StochRSIPeriod,
			"aroon_period":     cfg.Indicators.AroonPeriod,
			"skip_stc":         cfg.Indicators.SkipSTC,
			"last_n":           cfg.Indicators.LastN,
			"pretty":           cfg.Indicators.Pretty,
		},
		"consensus": map[string]any{
			"score_threshold":      cfg.Consensus.ScoreThreshold,
			"confidence_threshold": cfg.Consensus.ConfidenceThreshold,
		},
		"cooldown": map[string]any{
			"enabled":            cfg.Cooldown.Enabled,
			"entry_cooldown_sec": cfg.Cooldown.EntryCooldownSec,
		},
		"memory": map[string]any{
			"enabled":                 cfg.Memory.Enabled,
			"working_memory_size":     cfg.Memory.WorkingMemorySize,
			"episodic_enabled":        cfg.Memory.EpisodicEnabled,
			"episodic_ttl_days":       cfg.Memory.EpisodicTTLDays,
			"episodic_max_per_symbol": cfg.Memory.EpisodicMaxPerSymbol,
			"semantic_enabled":        cfg.Memory.SemanticEnabled,
			"semantic_max_rules":      cfg.Memory.SemanticMaxRules,
		},
		"llm": map[string]any{
			"agent":    buildRoleSetOutput(cfg.LLM.Agent),
			"provider": buildRoleSetOutput(cfg.LLM.Provider),
		},
	}
}

func buildStrategyOutput(cfg config.StrategyConfig) map[string]any {
	return map[string]any{
		"symbol":          cfg.Symbol,
		"id":              cfg.ID,
		"rule_chain":      cfg.RuleChainPath,
		"risk_management": buildRiskManagementOutput(cfg.RiskManagement),
	}
}

func buildRiskManagementOutput(cfg config.RiskManagementConfig) map[string]any {
	return map[string]any{
		"risk_per_trade_pct":  cfg.RiskPerTradePct,
		"max_invest_pct":      cfg.MaxInvestPct,
		"max_leverage":        cfg.MaxLeverage,
		"grade_1_factor":      cfg.Grade1Factor,
		"grade_2_factor":      cfg.Grade2Factor,
		"grade_3_factor":      cfg.Grade3Factor,
		"entry_offset_atr":    cfg.EntryOffsetATR,
		"entry_mode":          cfg.EntryMode,
		"orderbook_depth":     cfg.OrderbookDepth,
		"breakeven_fee_pct":   cfg.BreakevenFeePct,
		"slippage_buffer_pct": cfg.SlippageBufferPct,
		"risk_strategy":       map[string]any{"mode": cfg.RiskStrategy.Mode},
		"initial_exit":        map[string]any{"policy": cfg.InitialExit.Policy, "structure_interval": cfg.InitialExit.StructureInterval, "params": cfg.InitialExit.Params},
		"tighten_atr":         map[string]any{"structure_threatened": cfg.TightenATR.StructureThreatened, "tp1_atr": cfg.TightenATR.TP1ATR, "tp2_atr": cfg.TightenATR.TP2ATR, "min_tp_distance_pct": cfg.TightenATR.MinTPDistancePct, "min_tp_gap_pct": cfg.TightenATR.MinTPGapPct, "min_update_interval_sec": cfg.TightenATR.MinUpdateIntervalSec},
		"gate":                map[string]any{"quality_threshold": cfg.Gate.QualityThreshold, "edge_threshold": cfg.Gate.EdgeThreshold},
		"sieve":               buildSieveOutput(cfg.Sieve),
	}
}

func buildSieveOutput(cfg config.RiskManagementSieveConfig) map[string]any {
	rows := make([]map[string]any, 0, len(cfg.Rows))
	for _, row := range cfg.Rows {
		rows = append(rows, map[string]any{
			"mechanics_tag":  row.MechanicsTag,
			"liq_confidence": row.LiqConfidence,
			"crowding_align": row.CrowdingAlign,
			"gate_action":    row.GateAction,
			"size_factor":    row.SizeFactor,
			"reason_code":    row.ReasonCode,
		})
	}
	return map[string]any{
		"min_size_factor":     cfg.MinSizeFactor,
		"default_gate_action": cfg.DefaultGateAction,
		"default_size_factor": cfg.DefaultSizeFactor,
		"rows":                rows,
	}
}

func buildRoleSetOutput(cfg config.LLMRoleSet) map[string]any {
	return map[string]any{
		"indicator": buildRoleOutput(cfg.Indicator),
		"structure": buildRoleOutput(cfg.Structure),
		"mechanics": buildRoleOutput(cfg.Mechanics),
	}
}

func buildRoleOutput(cfg config.LLMRoleConfig) map[string]any {
	out := map[string]any{
		"model": cfg.Model,
	}
	if cfg.Temperature != nil {
		out["temperature"] = *cfg.Temperature
	}
	return out
}

func boolValue(v *bool) bool {
	return v != nil && *v
}
