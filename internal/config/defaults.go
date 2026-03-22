// 本文件主要内容：提供配置默认值。
package config

import (
	"sort"
	"strings"

	"brale-core/internal/llm"
)

func DefaultSymbolConfig(sys SystemConfig, symbol string) (SymbolConfig, error) {
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
		Indicators: IndicatorConfig{
			EMAFast:    21,
			EMAMid:     50,
			EMASlow:    200,
			RSIPeriod:  14,
			ATRPeriod:  14,
			MACDFast:   12,
			MACDSlow:   26,
			MACDSignal: 9,
			LastN:      5,
		},
		Consensus: ConsensusConfig{
			ScoreThreshold:      0.35,
			ConfidenceThreshold: 0.52,
		},
		Cooldown: CooldownConfig{Enabled: false},
		LLM: SymbolLLMConfig{
			SessionMode: defaultSessionModeString(),
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
	cfg.Hash = CombineHashes(symbol, "default")
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
	if strings.TrimSpace(cfg.LLM.SessionMode) == "" {
		cfg.LLM.SessionMode = defaults.LLM.SessionMode
	}
	applyCooldownDefaults(&cfg.Cooldown, defaults.Cooldown)
}

func applySystemDefaults(cfg *SystemConfig) {
	if cfg == nil {
		return
	}
	if strings.TrimSpace(cfg.LLM.SessionMode) == "" {
		cfg.LLM.SessionMode = defaultSessionModeString()
	}
}

func ResolveSessionMode(sys SystemConfig, symbolCfg SymbolConfig) (llm.SessionMode, error) {
	raw := strings.TrimSpace(symbolCfg.LLM.SessionMode)
	if raw == "" {
		raw = strings.TrimSpace(sys.LLM.SessionMode)
	}
	if raw == "" {
		raw = defaultSessionModeString()
	}
	return llm.NewSessionMode(raw)
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
}

func applyCooldownDefaults(cfg *CooldownConfig, defaults CooldownConfig) {
	if cfg.EntryCooldownSec == 0 {
		cfg.EntryCooldownSec = defaults.EntryCooldownSec
	}
}

func DefaultStrategyConfig(symbol string) StrategyConfig {
	return StrategyConfig{
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
			Sieve: RiskManagementSieveConfig{
				MinSizeFactor:     0.1,
				DefaultGateAction: "ALLOW",
				DefaultSizeFactor: 1.0,
			},
		},
		Hash: CombineHashes(symbol, "default_strategy"),
	}
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

func defaultSessionModeString() string {
	return llm.SessionModeSession.String()
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
