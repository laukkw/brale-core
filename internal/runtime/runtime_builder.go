package runtime

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/decision"
	"brale-core/internal/interval"
	"brale-core/internal/llm"
	llmapp "brale-core/internal/llm/app"
	"brale-core/internal/market"
	"brale-core/internal/market/binance"
	symbolpkg "brale-core/internal/pkg/symbol"
	"brale-core/internal/position"
	"brale-core/internal/reconcile"
	"brale-core/internal/snapshot"
	"brale-core/internal/store"
	"brale-core/internal/strategy"
	"brale-core/internal/transport/notify"
)

func NormalizeSymbol(symbol string) string {
	return symbolpkg.Normalize(symbol)
}

type SymbolRuntimeBuildDeps struct {
	Store         store.Store
	StateProvider *reconcile.FSMStateProvider
	Positioner    *position.PositionService
	RiskPlanSvc   *position.RiskPlanService
	PriceSource   market.PriceSource
}

func BuildSymbolRuntime(metricsCtx context.Context, sys config.SystemConfig, indexPath string, index config.SymbolIndexConfig, symbol string, deps SymbolRuntimeBuildDeps) (SymbolRuntime, error) {
	st := deps.Store
	stateProvider := deps.StateProvider
	positioner := deps.Positioner
	riskPlanSvc := deps.RiskPlanSvc
	priceSource := deps.PriceSource
	normalized := NormalizeSymbol(symbol)
	if normalized == "" {
		return SymbolRuntime{}, fmt.Errorf("symbol is required")
	}
	if entry, ok := findSymbolIndexEntry(index, normalized); ok {
		symbolCfg, stratCfg, bind, err := LoadSymbolConfigs(sys, indexPath, entry)
		if err != nil {
			return SymbolRuntime{}, err
		}
		enabledCfg, err := config.ResolveAgentEnabled(symbolCfg.Agent)
		if err != nil {
			return SymbolRuntime{}, err
		}
		barInterval, err := interval.ShortestInterval(symbolCfg.Intervals)
		if err != nil {
			return SymbolRuntime{}, err
		}
		enabledApp := decision.AgentEnabled{Indicator: enabledCfg.Indicator, Structure: enabledCfg.Structure, Mechanics: enabledCfg.Mechanics}
		return buildSymbolRuntimeFromConfig(metricsCtx, sys, symbolCfg, stratCfg, bind, st, stateProvider, positioner, riskPlanSvc, priceSource, barInterval, enabledCfg, enabledApp, enabledCfg.Mechanics)
	}
	base := filepath.Dir(indexPath)
	symbolCfg, stratCfg, bind, enabledCfg, enabledApp, barInterval, err := loadDefaultSymbolConfigs(sys, base, normalized)
	if err != nil {
		return SymbolRuntime{}, err
	}
	return buildSymbolRuntimeFromConfig(metricsCtx, sys, symbolCfg, stratCfg, bind, st, stateProvider, positioner, riskPlanSvc, priceSource, barInterval, enabledCfg, enabledApp, enabledCfg.Mechanics)
}

func buildSymbolRuntimeFromConfig(metricsCtx context.Context, sys config.SystemConfig, symbolCfg config.SymbolConfig, stratCfg config.StrategyConfig, bind strategy.StrategyBinding, st store.Store, stateProvider *reconcile.FSMStateProvider, positioner *position.PositionService, riskPlanSvc *position.RiskPlanService, priceSource market.PriceSource, barInterval time.Duration, enabledCfg config.AgentEnabled, enabledApp decision.AgentEnabled, requireMechanics bool) (SymbolRuntime, error) {
	sessionManager := llm.NewRoundSessionManager()
	sessionMode, err := config.ResolveSessionMode(sys, symbolCfg)
	if err != nil {
		return SymbolRuntime{}, err
	}
	agentSvc, providerSvc, tracker := buildSymbolAgents(sys, symbolCfg, sessionManager, sessionMode)
	enabledMap := map[string]decision.AgentEnabled{symbolCfg.Symbol: enabledApp}
	fetcher := buildSnapshotFetcher(symbolCfg, requireMechanics)
	compressor := buildCompressor(metricsCtx, symbolCfg, enabledCfg, enabledMap)
	exitConfirmCache := decision.NewExitConfirmCache()
	runner := buildRunner(sys, fetcher, compressor, agentSvc, providerSvc, symbolCfg, bind, enabledMap, sessionManager, sessionMode)
	pipeline, err := buildPipeline(sys, st, stateProvider, positioner, riskPlanSvc, priceSource, barInterval, symbolCfg.Symbol, bind, symbolCfg, stratCfg, &runner, exitConfirmCache, sessionManager, sessionMode)
	if err != nil {
		return SymbolRuntime{}, err
	}
	return SymbolRuntime{
		Symbol:          symbolCfg.Symbol,
		Intervals:       symbolCfg.Intervals,
		KlineLimit:      symbolCfg.KlineLimit,
		BarInterval:     barInterval,
		RiskPerTradePct: stratCfg.RiskManagement.RiskPerTradePct,
		Enabled:         enabledApp,
		LLMTracker:      tracker,
		SessionManager:  sessionManager,
		SessionMode:     sessionMode,
		Pipeline:        pipeline,
	}, nil
}

func findSymbolIndexEntry(index config.SymbolIndexConfig, symbol string) (config.SymbolIndexEntry, bool) {
	for _, entry := range index.Symbols {
		if strings.EqualFold(entry.Symbol, symbol) {
			return entry, true
		}
	}
	return config.SymbolIndexEntry{}, false
}

func loadDefaultSymbolConfigs(sys config.SystemConfig, base, symbol string) (config.SymbolConfig, config.StrategyConfig, strategy.StrategyBinding, config.AgentEnabled, decision.AgentEnabled, time.Duration, error) {
	symbolPath := resolvePath(base, filepath.Join("symbols", "default.toml"))
	strategyPath := resolvePath(base, filepath.Join("strategies", "default.toml"))

	symbolCfg, err := config.LoadSymbolConfig(symbolPath)
	if err != nil {
		return config.SymbolConfig{}, config.StrategyConfig{}, strategy.StrategyBinding{}, config.AgentEnabled{}, decision.AgentEnabled{}, 0, err
	}
	symbolCfg.Symbol = symbol
	defaults, err := config.DefaultSymbolConfig(sys, symbol)
	if err != nil {
		return config.SymbolConfig{}, config.StrategyConfig{}, strategy.StrategyBinding{}, config.AgentEnabled{}, decision.AgentEnabled{}, 0, err
	}
	config.ApplyDecisionDefaults(&symbolCfg, defaults)
	if err := config.ValidateSymbolLLMModels(sys, symbolCfg); err != nil {
		return config.SymbolConfig{}, config.StrategyConfig{}, strategy.StrategyBinding{}, config.AgentEnabled{}, decision.AgentEnabled{}, 0, err
	}
	symbolHash, err := config.HashSymbolConfig(symbolCfg)
	if err != nil {
		return config.SymbolConfig{}, config.StrategyConfig{}, strategy.StrategyBinding{}, config.AgentEnabled{}, decision.AgentEnabled{}, 0, err
	}
	symbolCfg.Hash = symbolHash
	stratCfg, err := config.LoadStrategyConfigWithSymbol(strategyPath, symbol)
	if err != nil {
		return config.SymbolConfig{}, config.StrategyConfig{}, strategy.StrategyBinding{}, config.AgentEnabled{}, decision.AgentEnabled{}, 0, err
	}
	if err := validateInitialExitStructureInterval(symbolCfg, stratCfg); err != nil {
		return config.SymbolConfig{}, config.StrategyConfig{}, strategy.StrategyBinding{}, config.AgentEnabled{}, decision.AgentEnabled{}, 0, err
	}
	stratCfg.Hash = ""
	if updatedHash, err := config.HashStrategyConfig(stratCfg); err == nil {
		stratCfg.Hash = updatedHash
	}
	bind, err := strategy.BuildBinding(sys, stratCfg)
	if err != nil {
		return config.SymbolConfig{}, config.StrategyConfig{}, strategy.StrategyBinding{}, config.AgentEnabled{}, decision.AgentEnabled{}, 0, err
	}
	bind.StrategyHash = config.CombineHashes(symbolCfg.Hash, stratCfg.Hash)
	bind.SystemHash = sys.Hash
	enabledCfg := config.AgentEnabled{Indicator: true, Structure: true, Mechanics: true}
	if resolved, err := config.ResolveAgentEnabled(symbolCfg.Agent); err == nil {
		enabledCfg = resolved
	}
	enabledApp := decision.AgentEnabled{Indicator: enabledCfg.Indicator, Structure: enabledCfg.Structure, Mechanics: enabledCfg.Mechanics}
	barInterval, err := interval.ShortestInterval(symbolCfg.Intervals)
	if err != nil {
		barInterval = time.Minute * 15
	}
	return symbolCfg, stratCfg, bind, enabledCfg, enabledApp, barInterval, nil
}

func LoadSymbolConfigs(sys config.SystemConfig, indexPath string, item config.SymbolIndexEntry) (config.SymbolConfig, config.StrategyConfig, strategy.StrategyBinding, error) {
	base := filepath.Dir(indexPath)
	symbolPath := resolvePath(base, item.Config)
	strategyPath := resolvePath(base, item.Strategy)
	symbolCfg, err := config.LoadSymbolConfig(symbolPath)
	if err != nil {
		return config.SymbolConfig{}, config.StrategyConfig{}, strategy.StrategyBinding{}, err
	}
	if symbolCfg.Symbol != item.Symbol {
		return config.SymbolConfig{}, config.StrategyConfig{}, strategy.StrategyBinding{}, fmt.Errorf("symbol config mismatch: %s", symbolCfg.Symbol)
	}
	defaults, err := config.DefaultSymbolConfig(sys, item.Symbol)
	if err != nil {
		return config.SymbolConfig{}, config.StrategyConfig{}, strategy.StrategyBinding{}, err
	}
	config.ApplyDecisionDefaults(&symbolCfg, defaults)
	if err := config.ValidateSymbolLLMModels(sys, symbolCfg); err != nil {
		return config.SymbolConfig{}, config.StrategyConfig{}, strategy.StrategyBinding{}, err
	}
	stratCfg, err := config.LoadStrategyConfig(strategyPath)
	if err != nil {
		return config.SymbolConfig{}, config.StrategyConfig{}, strategy.StrategyBinding{}, err
	}
	if stratCfg.Symbol != item.Symbol {
		return config.SymbolConfig{}, config.StrategyConfig{}, strategy.StrategyBinding{}, fmt.Errorf("strategy config mismatch: %s", stratCfg.Symbol)
	}
	if err := validateInitialExitStructureInterval(symbolCfg, stratCfg); err != nil {
		return config.SymbolConfig{}, config.StrategyConfig{}, strategy.StrategyBinding{}, err
	}
	bind, err := strategy.BuildBinding(sys, stratCfg)
	if err != nil {
		return config.SymbolConfig{}, config.StrategyConfig{}, strategy.StrategyBinding{}, err
	}
	bind.StrategyHash = config.CombineHashes(symbolCfg.Hash, stratCfg.Hash)
	bind.SystemHash = sys.Hash
	return symbolCfg, stratCfg, bind, nil
}

func resolvePath(base, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, path)
}

func validateInitialExitStructureInterval(symbolCfg config.SymbolConfig, stratCfg config.StrategyConfig) error {
	iv := strings.ToLower(strings.TrimSpace(stratCfg.RiskManagement.InitialExit.StructureInterval))
	if iv == "" || iv == "auto" {
		return nil
	}
	for _, candidate := range symbolCfg.Intervals {
		if strings.EqualFold(strings.TrimSpace(candidate), iv) {
			return nil
		}
	}
	return fmt.Errorf("risk_management.initial_exit.structure_interval=%q not found in symbol intervals %v", iv, symbolCfg.Intervals)
}

func buildSymbolAgents(sys config.SystemConfig, symbolCfg config.SymbolConfig, sessionManager *llm.RoundSessionManager, sessionMode llm.SessionMode) (decision.AgentService, decision.ProviderService, *llmapp.LLMRunTracker) {
	cache := llmapp.NewLLMStageCache()
	tracker := llmapp.NewLLMRunTracker()
	defaults := config.DefaultPromptDefaults()
	builder := llmapp.LLMPromptBuilder{
		AgentIndicatorSystem:      defaults.AgentIndicator,
		AgentStructureSystem:      defaults.AgentStructure,
		AgentMechanicsSystem:      defaults.AgentMechanics,
		ProviderIndicatorSystem:   defaults.ProviderIndicator,
		ProviderStructureSystem:   defaults.ProviderStructure,
		ProviderMechanicsSystem:   defaults.ProviderMechanics,
		ProviderInPosIndicatorSys: defaults.ProviderInPositionIndicator,
		ProviderInPosStructureSys: defaults.ProviderInPositionStructure,
		ProviderInPosMechanicsSys: defaults.ProviderInPositionMechanics,
		RiskFlatInitSystem:        defaults.RiskFlatInit,
		UserFormat:                llmapp.UserPromptFormatBullet,
	}
	agentRunner := &decision.AgentRunner{
		Indicator: newLLMClient(sys, symbolCfg.LLM.Agent.Indicator),
		Structure: newLLMClient(sys, symbolCfg.LLM.Agent.Structure),
		Mechanics: newLLMClient(sys, symbolCfg.LLM.Agent.Mechanics),
	}
	providerRunner := &decision.ProviderRunner{
		Indicator: newLLMClient(sys, symbolCfg.LLM.Provider.Indicator),
		Structure: newLLMClient(sys, symbolCfg.LLM.Provider.Structure),
		Mechanics: newLLMClient(sys, symbolCfg.LLM.Provider.Mechanics),
	}
	return llmapp.LLMAgentService{Runner: agentRunner, Prompts: builder, Cache: cache, Tracker: tracker, SessionManager: sessionManager, SessionMode: sessionMode}, llmapp.LLMProviderService{Runner: providerRunner, Prompts: builder, Cache: cache, Tracker: tracker, SessionManager: sessionManager, SessionMode: sessionMode}, tracker
}

func newLLMClient(sys config.SystemConfig, role config.LLMRoleConfig) *llm.OpenAIClient {
	temp := 0.0
	if role.Temperature != nil {
		temp = *role.Temperature
	}
	modelCfg := sys.LLMModels[role.Model]
	timeoutSec := 30
	if modelCfg.TimeoutSec != nil {
		timeoutSec = *modelCfg.TimeoutSec
	}
	return &llm.OpenAIClient{
		Endpoint:    modelCfg.Endpoint,
		Model:       role.Model,
		APIKey:      modelCfg.APIKey,
		Timeout:     time.Duration(timeoutSec) * time.Second,
		Temperature: temp,
	}
}
func buildSnapshotFetcher(symbolCfg config.SymbolConfig, requireMechanics bool) *snapshot.Fetcher {
	fetcher := binance.NewSnapshotFetcher(binance.SnapshotOptions{
		RequireOI:           requireMechanics && symbolCfg.Require.OI,
		RequireFunding:      requireMechanics && symbolCfg.Require.Funding,
		RequireLongShort:    requireMechanics && symbolCfg.Require.LongShort,
		RequireFearGreed:    requireMechanics && symbolCfg.Require.FearGreed,
		RequireLiquidations: requireMechanics && symbolCfg.Require.Liquidations,
	})
	if requireMechanics {
		return fetcher
	}
	fetcher.OI = nil
	fetcher.Funding = nil
	fetcher.LongShort = nil
	fetcher.FearGreed = nil
	fetcher.Liquidations = nil
	fetcher.RequireOI = false
	fetcher.RequireFunding = false
	fetcher.RequireLongShort = false
	fetcher.RequireFearGreed = false
	fetcher.RequireLiquidations = false
	return fetcher
}

func buildMetricsService(metricsCtx context.Context, symbolCfg config.SymbolConfig, enabled config.AgentEnabled) *market.MetricsService {
	if !enabled.Mechanics {
		return nil
	}
	if len(symbolCfg.Intervals) == 0 {
		return nil
	}
	svc, err := market.NewMetricsService(binance.NewFuturesMarket(), []string{symbolCfg.Symbol}, symbolCfg.Intervals)
	if err != nil || svc == nil {
		return nil
	}
	if metricsCtx == nil {
		metricsCtx = context.Background()
	}
	go svc.Start(metricsCtx)
	svc.RefreshSymbol(metricsCtx, symbolCfg.Symbol)
	return svc
}

func buildCompressor(metricsCtx context.Context, symbolCfg config.SymbolConfig, enabled config.AgentEnabled, enabledMap map[string]decision.AgentEnabled) *decision.FeatureCompressor {
	metricsSvc := buildMetricsService(metricsCtx, symbolCfg, enabled)
	trendPresets := config.TrendPresetForIntervals(symbolCfg.Intervals)
	trendOptions := make(map[string]decision.TrendCompressOptions, len(trendPresets))
	for iv, preset := range trendPresets {
		trendOptions[iv] = toTrendOptionsFromPreset(preset)
	}
	defaultPreset := config.DefaultTrendPreset()
	return &decision.FeatureCompressor{
		Indicators: decision.DefaultIndicatorBuilder{Options: toIndicatorOptions(symbolCfg.Indicators, enabled.Indicator)},
		Trends: decision.IntervalTrendBuilder{
			OptionsByInterval: trendOptions,
			DefaultOptions:    toTrendOptionsFromPreset(defaultPreset),
		},
		Mechanics: decision.ConditionalMechanicsBuilder{
			Enabled: enabledMap,
			EnabledBuilder: decision.DefaultMechanicsBuilder{Options: decision.MechanicsCompressOptions{
				Metrics: metricsSvc,
			}},
		},
	}
}

func buildRunner(sys config.SystemConfig, fetcher *snapshot.Fetcher, compressor *decision.FeatureCompressor, agentSvc decision.AgentService, providerSvc decision.ProviderService, symbolCfg config.SymbolConfig, bind strategy.StrategyBinding, enabledMap map[string]decision.AgentEnabled, sessionManager *llm.RoundSessionManager, sessionMode llm.SessionMode) decision.Runner {
	defaults := config.DefaultPromptDefaults()
	riskPrompts := llmapp.LLMPromptBuilder{
		RiskFlatInitSystem: defaults.RiskFlatInit,
		UserFormat:         llmapp.UserPromptFormatBullet,
	}
	riskSvc := llmapp.LLMRiskService{
		Provider:       newLLMClient(sys, symbolCfg.LLM.Provider.Structure),
		Prompts:        riskPrompts,
		SessionManager: sessionManager,
		SessionMode:    sessionMode,
	}
	return decision.Runner{
		Snapshotter:     fetcher,
		Compressor:      compressor,
		Agent:           agentSvc,
		Provider:        providerSvc,
		FlatRiskInitLLM: riskSvc.FlatRiskInitLLM(),
		TightenRiskLLM:  riskSvc.TightenRiskLLM(),
		Bindings:        map[string]strategy.StrategyBinding{symbolCfg.Symbol: bind},
		Configs:         map[string]config.SymbolConfig{symbolCfg.Symbol: symbolCfg},
		Enabled:         enabledMap,
	}
}

func buildPipeline(sys config.SystemConfig, st store.Store, stateProvider *reconcile.FSMStateProvider, positioner *position.PositionService, riskPlanSvc *position.RiskPlanService, priceSource market.PriceSource, barInterval time.Duration, symbol string, bind strategy.StrategyBinding, symbolCfg config.SymbolConfig, stratCfg config.StrategyConfig, runner *decision.Runner, exitConfirmCache *decision.ExitConfirmCache, sessionManager *llm.RoundSessionManager, sessionMode llm.SessionMode) (*decision.Pipeline, error) {
	formatter := decision.NewFormatter()
	notifier, err := notify.NewManager(notify.FromConfig(sys.Notification), formatter)
	if err != nil {
		return nil, err
	}
	hooks := decision.StoreHooks{
		Store:         st,
		SystemHash:    sys.Hash,
		StrategyHash:  config.CombineHashes(symbolCfg.Hash, stratCfg.Hash),
		SourceVersion: "v0",
		Notifier:      notifier,
		TraceDir:      filepath.Join("data", "llm-traces"),
		TraceLogPath:  config.ResolveLogPath(sys),
		TraceEnabled:  true,
		TraceRedacted: false,
	}
	return &decision.Pipeline{
		Runner:                  runner,
		Store:                   st,
		Positioner:              positioner,
		RiskPlans:               riskPlanSvc,
		PriceSource:             priceSource,
		BarInterval:             barInterval,
		ExecutionSystem:         sys.ExecutionSystem,
		States:                  stateProvider,
		Bindings:                map[string]strategy.StrategyBinding{symbol: bind},
		PlanCache:               positioner.PlanCache,
		ExitConfirmCache:        exitConfirmCache,
		EntryCooldownCache:      decision.NewEntryCooldownCache(),
		EntryCooldownRounds:     2,
		AgentStore:              hooks.SaveAgent,
		ProviderStore:           hooks.SaveProvider,
		ProviderInPositionStore: hooks.SaveProviderInPosition,
		GateStore:               hooks.SaveGate,
		Notifier:                notifier,
		SessionManager:          sessionManager,
		SessionCleanup:          llm.CleanupOpenAISession,
		SessionMode:             sessionMode,
		TightenRiskLLM:          runner.TightenRiskLLM,
	}, nil
}
func toIndicatorOptions(cfg config.IndicatorConfig, indicatorEnabled bool) decision.IndicatorCompressOptions {
	opts := decision.IndicatorCompressOptions{
		EMAFast:    cfg.EMAFast,
		EMAMid:     cfg.EMAMid,
		EMASlow:    cfg.EMASlow,
		RSIPeriod:  cfg.RSIPeriod,
		ATRPeriod:  cfg.ATRPeriod,
		MACDFast:   cfg.MACDFast,
		MACDSlow:   cfg.MACDSlow,
		MACDSignal: cfg.MACDSignal,
		LastN:      cfg.LastN,
		Pretty:     cfg.Pretty,
	}
	if !indicatorEnabled {
		opts.SkipEMA = true
		opts.SkipRSI = true
		opts.SkipMACD = true
	}
	return opts
}

func toTrendOptionsFromPreset(preset config.TrendPreset) decision.TrendCompressOptions {
	return decision.TrendCompressOptions{
		FractalSpan:         preset.FractalSpan,
		MaxStructurePoints:  preset.MaxStructurePoints,
		DedupDistanceBars:   preset.DedupDistanceBars,
		DedupATRFactor:      preset.DedupATRFactor,
		RSIPeriod:           preset.RSIPeriod,
		ATRPeriod:           preset.ATRPeriod,
		RecentCandles:       preset.RecentCandles,
		VolumeMAPeriod:      preset.VolumeMAPeriod,
		EMA20Period:         preset.EMA20Period,
		EMA50Period:         preset.EMA50Period,
		EMA200Period:        preset.EMA200Period,
		PatternMinScore:     preset.PatternMinScore,
		PatternMaxDetected:  preset.PatternMaxDetected,
		Pretty:              preset.Pretty,
		IncludeCurrentRSI:   preset.IncludeCurrentRSI,
		IncludeStructureRSI: preset.IncludeStructureRSI,
	}
}
