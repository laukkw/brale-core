package decisionview

import (
	"sort"

	"brale-core/internal/config"
)

type ConfigBundle struct {
	Symbol   config.SymbolConfig   `json:"symbol"`
	Strategy config.StrategyConfig `json:"strategy"`
}

type ConfigGraphResponse struct {
	Symbols []ConfigSymbolGraph `json:"symbols"`
}

type ConfigSymbolGraph struct {
	ID     string       `json:"id"`
	Label  string       `json:"label"`
	Config ConfigDetail `json:"config"`
}

type ConfigDetail struct {
	Intervals    []string            `json:"intervals"`
	KlineLimit   int                 `json:"kline_limit"`
	AgentEnabled AgentEnabledSummary `json:"agent_enabled"`
	Gate         GateSummary         `json:"gate"`
	LLM          SymbolLLMSummary    `json:"llm"`
	Prompts      SymbolPromptSummary `json:"prompts"`
	Strategy     StrategySummary     `json:"strategy"`
	SystemHash   string              `json:"system_hash"`
	SymbolHash   string              `json:"symbol_hash"`
	StrategyHash string              `json:"strategy_hash"`
	Meta         map[string]any      `json:"meta,omitempty"`
}

type AgentEnabledSummary struct {
	Indicator bool `json:"indicator"`
	Structure bool `json:"structure"`
	Mechanics bool `json:"mechanics"`
}

type GateSummary struct{}

type SymbolLLMSummary struct {
	Agent    LLMModelSet `json:"agent"`
	Provider LLMModelSet `json:"provider"`
}

type LLMModelSet struct {
	Indicator string `json:"indicator"`
	Structure string `json:"structure"`
	Mechanics string `json:"mechanics"`
}

type SymbolPromptSummary struct {
	Agent              PromptSet `json:"agent"`
	Provider           PromptSet `json:"provider"`
	ProviderInPosition PromptSet `json:"provider_in_position"`
}

type PromptSet struct {
	Indicator string `json:"indicator"`
	Structure string `json:"structure"`
	Mechanics string `json:"mechanics"`
}

type StrategySummary struct {
	ID             string                `json:"id"`
	RuleChainPath  string                `json:"rule_chain"`
	RiskManagement RiskManagementSummary `json:"risk_management"`
}

type RiskManagementSummary struct {
	RiskPerTradePct float64            `json:"risk_per_trade_pct"`
	MaxLeverage     float64            `json:"max_leverage"`
	Grade1Factor    float64            `json:"grade_1_factor"`
	Grade2Factor    float64            `json:"grade_2_factor"`
	Grade3Factor    float64            `json:"grade_3_factor"`
	EntryOffsetATR  float64            `json:"entry_offset_atr"`
	BreakevenFeePct float64            `json:"breakeven_fee_pct"`
	InitialExit     InitialExitSummary `json:"initial_exit"`
	TightenATR      TightenATRSummary  `json:"tighten_atr"`
}

type InitialExitSummary struct {
	Policy            string         `json:"policy"`
	StructureInterval string         `json:"structure_interval"`
	Params            map[string]any `json:"params"`
}

type TightenATRSummary struct {
	StructureThreatened  float64 `json:"structure_threatened"`
	TP1ATR               float64 `json:"tp1_atr"`
	TP2ATR               float64 `json:"tp2_atr"`
	MinTPDistancePct     float64 `json:"min_tp_distance_pct"`
	MinTPGapPct          float64 `json:"min_tp_gap_pct"`
	MinUpdateIntervalSec int64   `json:"min_update_interval_sec"`
}

func (s Server) buildConfigGraph() ConfigGraphResponse {
	resp := ConfigGraphResponse{
		Symbols: []ConfigSymbolGraph{},
	}
	if len(s.SymbolConfigs) == 0 {
		return resp
	}
	defaults := config.DefaultPromptDefaults()
	for sym, bundle := range s.SymbolConfigs {
		enabled, err := config.ResolveAgentEnabled(bundle.Symbol.Agent)
		if err != nil {
			enabled = config.AgentEnabled{}
		}
		item := ConfigSymbolGraph{
			ID:    sym,
			Label: sym,
			Config: ConfigDetail{
				Intervals:  append([]string{}, bundle.Symbol.Intervals...),
				KlineLimit: bundle.Symbol.KlineLimit,
				AgentEnabled: AgentEnabledSummary{
					Indicator: enabled.Indicator,
					Structure: enabled.Structure,
					Mechanics: enabled.Mechanics,
				},
				Gate: GateSummary{},
				LLM: SymbolLLMSummary{
					Agent: LLMModelSet{
						Indicator: bundle.Symbol.LLM.Agent.Indicator.Model,
						Structure: bundle.Symbol.LLM.Agent.Structure.Model,
						Mechanics: bundle.Symbol.LLM.Agent.Mechanics.Model,
					},
					Provider: LLMModelSet{
						Indicator: bundle.Symbol.LLM.Provider.Indicator.Model,
						Structure: bundle.Symbol.LLM.Provider.Structure.Model,
						Mechanics: bundle.Symbol.LLM.Provider.Mechanics.Model,
					},
				},
				Prompts: SymbolPromptSummary{
					Agent: PromptSet{
						Indicator: defaults.AgentIndicator,
						Structure: defaults.AgentStructure,
						Mechanics: defaults.AgentMechanics,
					},
					Provider: PromptSet{
						Indicator: defaults.ProviderIndicator,
						Structure: defaults.ProviderStructure,
						Mechanics: defaults.ProviderMechanics,
					},
					ProviderInPosition: PromptSet{
						Indicator: defaults.ProviderInPositionIndicator,
						Structure: defaults.ProviderInPositionStructure,
						Mechanics: defaults.ProviderInPositionMechanics,
					},
				},
				Strategy: StrategySummary{
					ID:            bundle.Strategy.ID,
					RuleChainPath: bundle.Strategy.RuleChainPath,
					RiskManagement: RiskManagementSummary{
						RiskPerTradePct: bundle.Strategy.RiskManagement.RiskPerTradePct,
						MaxLeverage:     bundle.Strategy.RiskManagement.MaxLeverage,
						Grade1Factor:    bundle.Strategy.RiskManagement.Grade1Factor,
						Grade2Factor:    bundle.Strategy.RiskManagement.Grade2Factor,
						Grade3Factor:    bundle.Strategy.RiskManagement.Grade3Factor,
						EntryOffsetATR:  bundle.Strategy.RiskManagement.EntryOffsetATR,
						BreakevenFeePct: bundle.Strategy.RiskManagement.BreakevenFeePct,
						InitialExit: InitialExitSummary{
							Policy:            bundle.Strategy.RiskManagement.InitialExit.Policy,
							StructureInterval: bundle.Strategy.RiskManagement.InitialExit.StructureInterval,
							Params:            bundle.Strategy.RiskManagement.InitialExit.Params,
						},
						TightenATR: TightenATRSummary{
							StructureThreatened:  bundle.Strategy.RiskManagement.TightenATR.StructureThreatened,
							TP1ATR:               bundle.Strategy.RiskManagement.TightenATR.TP1ATR,
							TP2ATR:               bundle.Strategy.RiskManagement.TightenATR.TP2ATR,
							MinTPDistancePct:     bundle.Strategy.RiskManagement.TightenATR.MinTPDistancePct,
							MinTPGapPct:          bundle.Strategy.RiskManagement.TightenATR.MinTPGapPct,
							MinUpdateIntervalSec: bundle.Strategy.RiskManagement.TightenATR.MinUpdateIntervalSec,
						},
					},
				},
				SystemHash:   s.SystemConfig.Hash,
				SymbolHash:   bundle.Symbol.Hash,
				StrategyHash: bundle.Strategy.Hash,
			},
		}
		resp.Symbols = append(resp.Symbols, item)
	}
	sort.Slice(resp.Symbols, func(i, j int) bool { return resp.Symbols[i].ID < resp.Symbols[j].ID })
	return resp
}
