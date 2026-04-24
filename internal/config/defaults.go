// 本文件主要内容：提供配置默认值。
package config

import (
	"sort"
	"strings"
)

func DefaultSymbolConfig(sys SystemConfig, symbol string) (SymbolConfig, error) {
	symbol = NormalizeSymbol(symbol)
	indicatorTemp := 0.2
	structureTemp := 0.1
	mechanicsTemp := 0.2
	cfg := SymbolConfig{
		Symbol:     symbol,
		Intervals:  []string{"15m", "1h", "4h"},
		KlineLimit: 300,
		Agent:      AgentConfig{Indicator: boolPtr(true), Structure: boolPtr(true), Mechanics: boolPtr(true)},
		Require: SymbolRequire{
			OI:           true,
			Funding:      true,
			LongShort:    true,
			FearGreed:    true,
			Liquidations: false,
		},
		Features: SymbolFeatures{
			Indicator: IndicatorFeatureConfig{
				EMA:          boolPtr(true),
				RSI:          boolPtr(true),
				ATR:          boolPtr(true),
				OBV:          boolPtr(true),
				STC:          boolPtr(true),
				BB:           boolPtr(true),
				CHOP:         boolPtr(true),
				StochRSI:     boolPtr(true),
				Aroon:        boolPtr(true),
				TDSequential: boolPtr(true),
			},
			Structure: StructureFeatureConfig{
				Supertrend: boolPtr(true),
				EMAContext: boolPtr(true),
				RSIContext: boolPtr(true),
				Patterns:   boolPtr(true),
				SMC:        boolPtr(true),
			},
			Mechanics: MechanicsFeatureConfig{
				OI:               boolPtr(true),
				Funding:          boolPtr(true),
				LongShort:        boolPtr(true),
				FearGreed:        boolPtr(true),
				Liquidations:     boolPtr(false),
				CVD:              boolPtr(true),
				Sentiment:        boolPtr(true),
				FuturesSentiment: boolPtr(true),
			},
		},
		Indicators: IndicatorConfig{
			Engine:         IndicatorEngineTA,
			EMAFast:        21,
			EMAMid:         50,
			EMASlow:        200,
			RSIPeriod:      14,
			ATRPeriod:      14,
			STCFast:        23,
			STCSlow:        50,
			BBPeriod:       20,
			BBMultiplier:   2.0,
			CHOPPeriod:     14,
			StochRSIPeriod: 14,
			AroonPeriod:    25,
			LastN:          5,
		},
		Memory: MemoryConfig{
			Enabled:           false,
			WorkingMemorySize: DefaultWorkingMemorySize,
		},
		Consensus: ConsensusConfig{
			ScoreThreshold:      0.35,
			ConfidenceThreshold: 0.52,
		},
		Cooldown: CooldownConfig{Enabled: false},
		LLM: SymbolLLMConfig{
			Agent: LLMRoleSet{
				Indicator: LLMRoleConfig{Model: "", Temperature: &indicatorTemp},
				Structure: LLMRoleConfig{Model: "", Temperature: &structureTemp},
				Mechanics: LLMRoleConfig{Model: "", Temperature: &mechanicsTemp},
			},
			Provider: LLMRoleSet{
				Indicator: LLMRoleConfig{Model: "", Temperature: &indicatorTemp},
				Structure: LLMRoleConfig{Model: "", Temperature: &structureTemp},
				Mechanics: LLMRoleConfig{Model: "", Temperature: &mechanicsTemp},
			},
		},
	}
	if err := applyDefaultLLMModels(sys, &cfg); err != nil {
		return SymbolConfig{}, err
	}
	h, err := HashSymbolConfig(cfg)
	if err != nil {
		return SymbolConfig{}, err
	}
	cfg.Hash = h
	return cfg, nil
}

func ApplyDecisionDefaults(cfg *SymbolConfig, defaults SymbolConfig) {
	if cfg == nil {
		return
	}
	if cfg.KlineLimit == 0 {
		cfg.KlineLimit = defaults.KlineLimit
	}
	if cfg.Consensus.ScoreThreshold == 0 {
		cfg.Consensus.ScoreThreshold = defaults.Consensus.ScoreThreshold
	}
	if cfg.Consensus.ConfidenceThreshold == 0 {
		cfg.Consensus.ConfidenceThreshold = defaults.Consensus.ConfidenceThreshold
	}
	applyIndicatorDefaults(&cfg.Indicators, defaults.Indicators)
	applyFeatureDefaults(&cfg.Features, defaults.Features, cfg.Require, cfg.Indicators.SkipSTC)
	applyMemoryDefaults(&cfg.Memory, defaults.Memory)
	applyCooldownDefaults(&cfg.Cooldown, defaults.Cooldown)
}

func applyIndicatorDefaults(cfg *IndicatorConfig, defaults IndicatorConfig) {
	if strings.TrimSpace(cfg.Engine) == "" {
		cfg.Engine = defaults.Engine
	}
	if cfg.EMAFast == 0 {
		cfg.EMAFast = defaults.EMAFast
	}
	if cfg.EMAMid == 0 {
		cfg.EMAMid = defaults.EMAMid
	}
	if cfg.EMASlow == 0 {
		cfg.EMASlow = defaults.EMASlow
	}
	if cfg.RSIPeriod == 0 {
		cfg.RSIPeriod = defaults.RSIPeriod
	}
	if cfg.ATRPeriod == 0 {
		cfg.ATRPeriod = defaults.ATRPeriod
	}
	if cfg.STCFast == 0 {
		cfg.STCFast = defaults.STCFast
	}
	if cfg.STCSlow == 0 {
		cfg.STCSlow = defaults.STCSlow
	}
	if cfg.BBPeriod == 0 {
		cfg.BBPeriod = defaults.BBPeriod
	}
	if cfg.BBMultiplier == 0 {
		cfg.BBMultiplier = defaults.BBMultiplier
	}
	if cfg.CHOPPeriod == 0 {
		cfg.CHOPPeriod = defaults.CHOPPeriod
	}
	if cfg.StochRSIPeriod == 0 {
		cfg.StochRSIPeriod = defaults.StochRSIPeriod
	}
	if cfg.AroonPeriod == 0 {
		cfg.AroonPeriod = defaults.AroonPeriod
	}
	if cfg.LastN == 0 {
		cfg.LastN = defaults.LastN
	}
}

func applySystemDefaults(cfg *SystemConfig) {
	if cfg == nil {
		return
	}
	cfg.Prompt.Locale = NormalizePromptLocale(cfg.Prompt.Locale)
	if cfg.Database.MaxOpenConns == 0 {
		cfg.Database.MaxOpenConns = 20
	}
	if cfg.Database.MaxIdleConns == 0 {
		cfg.Database.MaxIdleConns = 5
	}
	if cfg.Telemetry.ServiceName == "" {
		cfg.Telemetry.ServiceName = "brale-core"
	}
	if cfg.Telemetry.ExporterType == "" {
		cfg.Telemetry.ExporterType = "otlp"
	}
	if cfg.Scheduler.Backend == "" {
		cfg.Scheduler.Backend = "river"
	}
}

func applyMemoryDefaults(cfg *MemoryConfig, defaults MemoryConfig) {
	if cfg == nil {
		return
	}
	if cfg.WorkingMemorySize == 0 {
		cfg.WorkingMemorySize = defaults.WorkingMemorySize
	}
}

func applyFeatureDefaults(cfg *SymbolFeatures, defaults SymbolFeatures, require SymbolRequire, skipSTC bool) {
	if cfg == nil {
		return
	}
	applyIndicatorFeatureDefaults(&cfg.Indicator, defaults.Indicator, skipSTC)
	applyStructureFeatureDefaults(&cfg.Structure, defaults.Structure)
	applyMechanicsFeatureDefaults(&cfg.Mechanics, defaults.Mechanics, require)
}

func applyIndicatorFeatureDefaults(cfg *IndicatorFeatureConfig, defaults IndicatorFeatureConfig, skipSTC bool) {
	if cfg == nil {
		return
	}
	if cfg.EMA == nil {
		cfg.EMA = cloneBoolPtr(defaults.EMA)
	}
	if cfg.RSI == nil {
		cfg.RSI = cloneBoolPtr(defaults.RSI)
	}
	if cfg.ATR == nil {
		cfg.ATR = cloneBoolPtr(defaults.ATR)
	}
	if cfg.OBV == nil {
		cfg.OBV = cloneBoolPtr(defaults.OBV)
	}
	if cfg.STC == nil {
		cfg.STC = boolPtr(!skipSTC)
	}
	if cfg.BB == nil {
		cfg.BB = cloneBoolPtr(defaults.BB)
	}
	if cfg.CHOP == nil {
		cfg.CHOP = cloneBoolPtr(defaults.CHOP)
	}
	if cfg.StochRSI == nil {
		cfg.StochRSI = cloneBoolPtr(defaults.StochRSI)
	}
	if cfg.Aroon == nil {
		cfg.Aroon = cloneBoolPtr(defaults.Aroon)
	}
	if cfg.TDSequential == nil {
		cfg.TDSequential = cloneBoolPtr(defaults.TDSequential)
	}
}

func applyStructureFeatureDefaults(cfg *StructureFeatureConfig, defaults StructureFeatureConfig) {
	if cfg == nil {
		return
	}
	if cfg.Supertrend == nil {
		cfg.Supertrend = cloneBoolPtr(defaults.Supertrend)
	}
	if cfg.EMAContext == nil {
		cfg.EMAContext = cloneBoolPtr(defaults.EMAContext)
	}
	if cfg.RSIContext == nil {
		cfg.RSIContext = cloneBoolPtr(defaults.RSIContext)
	}
	if cfg.Patterns == nil {
		cfg.Patterns = cloneBoolPtr(defaults.Patterns)
	}
	if cfg.SMC == nil {
		cfg.SMC = cloneBoolPtr(defaults.SMC)
	}
}

func applyMechanicsFeatureDefaults(cfg *MechanicsFeatureConfig, defaults MechanicsFeatureConfig, require SymbolRequire) {
	if cfg == nil {
		return
	}
	if cfg.OI == nil {
		cfg.OI = boolPtr(require.OI)
	}
	if cfg.Funding == nil {
		cfg.Funding = boolPtr(require.Funding)
	}
	if cfg.LongShort == nil {
		cfg.LongShort = boolPtr(require.LongShort)
	}
	if cfg.FearGreed == nil {
		cfg.FearGreed = boolPtr(require.FearGreed)
	}
	if cfg.Liquidations == nil {
		cfg.Liquidations = boolPtr(require.Liquidations)
	}
	if cfg.CVD == nil {
		cfg.CVD = cloneBoolPtr(defaults.CVD)
	}
	if cfg.Sentiment == nil {
		cfg.Sentiment = cloneBoolPtr(defaults.Sentiment)
	}
	if cfg.FuturesSentiment == nil {
		cfg.FuturesSentiment = cloneBoolPtr(defaults.FuturesSentiment)
	}
}

func ApplyStrategyDefaults(cfg *StrategyConfig, defaults StrategyConfig) {
	if cfg == nil {
		return
	}
	if strings.TrimSpace(cfg.RiskManagement.RiskStrategy.Mode) == "" {
		cfg.RiskManagement.RiskStrategy.Mode = defaults.RiskManagement.RiskStrategy.Mode
	}
	if strings.TrimSpace(cfg.RiskManagement.EntryMode) == "" {
		cfg.RiskManagement.EntryMode = defaults.RiskManagement.EntryMode
	}
	if cfg.RiskManagement.OrderbookDepth == 0 {
		cfg.RiskManagement.OrderbookDepth = defaults.RiskManagement.OrderbookDepth
	}
	if strings.TrimSpace(cfg.RiskManagement.InitialExit.Policy) == "" {
		cfg.RiskManagement.InitialExit.Policy = defaults.RiskManagement.InitialExit.Policy
	}
	if strings.TrimSpace(cfg.RiskManagement.InitialExit.StructureInterval) == "" {
		cfg.RiskManagement.InitialExit.StructureInterval = defaults.RiskManagement.InitialExit.StructureInterval
	}
	if len(cfg.RiskManagement.InitialExit.Params) == 0 {
		cfg.RiskManagement.InitialExit.Params = cloneMapAny(defaults.RiskManagement.InitialExit.Params)
	}
	if cfg.RiskManagement.Gate.QualityThreshold == 0 {
		cfg.RiskManagement.Gate.QualityThreshold = defaults.RiskManagement.Gate.QualityThreshold
	}
	if cfg.RiskManagement.Gate.EdgeThreshold == 0 {
		cfg.RiskManagement.Gate.EdgeThreshold = defaults.RiskManagement.Gate.EdgeThreshold
	}
	applyGateHardStopDefaults(&cfg.RiskManagement.Gate.HardStop, defaults.RiskManagement.Gate.HardStop)
	applyHardGuardToggleDefaults(&cfg.RiskManagement.HardGuard, defaults.RiskManagement.HardGuard)
}

func applyCooldownDefaults(cfg *CooldownConfig, defaults CooldownConfig) {
	if cfg.EntryCooldownSec == 0 {
		cfg.EntryCooldownSec = defaults.EntryCooldownSec
	}
}

func DefaultStrategyConfig(symbol string) StrategyConfig {
	symbol = NormalizeSymbol(symbol)
	cfg := StrategyConfig{
		Symbol:        symbol,
		ID:            "default-" + symbol,
		RuleChainPath: "configs/rules/default.json",
		RiskManagement: RiskManagementConfig{
			RiskPerTradePct:   0.01,
			MaxInvestPct:      1.0,
			MaxLeverage:       3.0,
			Grade1Factor:      0.3,
			Grade2Factor:      0.6,
			Grade3Factor:      1.0,
			EntryOffsetATR:    0.1,
			EntryMode:         "orderbook",
			OrderbookDepth:    5,
			BreakevenFeePct:   0.0,
			SlippageBufferPct: 0.0005,
			RiskStrategy: RiskStrategyConfig{
				Mode: "llm",
			},
			InitialExit: InitialExitConfig{
				Policy:            "atr_structure_v1",
				StructureInterval: "auto",
				Params: map[string]any{
					"stop_atr_multiplier":   2.0,
					"stop_min_distance_pct": 0.005,
					"take_profit_rr":        []float64{1.5, 3.0},
				},
			},
			TightenATR: TightenATRConfig{
				StructureThreatened:  0.5,
				MinUpdateIntervalSec: 300,
			},
			Gate: GateConfig{
				QualityThreshold: 0.35,
				EdgeThreshold:    0.10,
				HardStop: GateHardStopConfig{
					StructureInvalidation: boolPtr(true),
					LiquidationCascade:    boolPtr(true),
				},
			},
			HardGuard: HardGuardToggleConfig{
				Enabled:        boolPtr(true),
				StopLoss:       boolPtr(true),
				RSIExtreme:     boolPtr(true),
				CircuitBreaker: boolPtr(true),
			},
			Sieve: RiskManagementSieveConfig{
				MinSizeFactor:     0.1,
				DefaultGateAction: "ALLOW",
				DefaultSizeFactor: 1.0,
			},
		},
	}
	h, err := HashStrategyConfig(cfg)
	if err != nil {
		h = CombineHashes(symbol, "default_strategy")
	}
	cfg.Hash = h
	return cfg
}

func applyDefaultLLMModels(sys SystemConfig, cfg *SymbolConfig) error {
	if len(sys.LLMModels) == 0 {
		return nil
	}
	keys := make([]string, 0, len(sys.LLMModels))
	for key := range sys.LLMModels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pick := func(idx int) string {
		return keys[idx%len(keys)]
	}
	cfg.LLM.Agent.Indicator.Model = pick(0)
	cfg.LLM.Agent.Structure.Model = pick(1)
	cfg.LLM.Agent.Mechanics.Model = pick(2)
	cfg.LLM.Provider.Indicator.Model = pick(0)
	cfg.LLM.Provider.Structure.Model = pick(1)
	cfg.LLM.Provider.Mechanics.Model = pick(2)
	return nil
}

func boolPtr(v bool) *bool {
	return &v
}

func cloneBoolPtr(v *bool) *bool {
	if v == nil {
		return nil
	}
	cloned := *v
	return &cloned
}

func applyGateHardStopDefaults(cfg *GateHardStopConfig, defaults GateHardStopConfig) {
	if cfg == nil {
		return
	}
	if cfg.StructureInvalidation == nil {
		cfg.StructureInvalidation = cloneBoolPtr(defaults.StructureInvalidation)
	}
	if cfg.LiquidationCascade == nil {
		cfg.LiquidationCascade = cloneBoolPtr(defaults.LiquidationCascade)
	}
}

func applyHardGuardToggleDefaults(cfg *HardGuardToggleConfig, defaults HardGuardToggleConfig) {
	if cfg == nil {
		return
	}
	if cfg.Enabled == nil {
		cfg.Enabled = cloneBoolPtr(defaults.Enabled)
	}
	if cfg.StopLoss == nil {
		cfg.StopLoss = cloneBoolPtr(defaults.StopLoss)
	}
	if cfg.RSIExtreme == nil {
		cfg.RSIExtreme = cloneBoolPtr(defaults.RSIExtreme)
	}
	if cfg.CircuitBreaker == nil {
		cfg.CircuitBreaker = cloneBoolPtr(defaults.CircuitBreaker)
	}
}

func cloneMapAny(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}
