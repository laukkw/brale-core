package runtime

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/decision"
	"brale-core/internal/interval"
	"brale-core/internal/strategy"
)

type symbolRuntimeConfig struct {
	Symbol           config.SymbolConfig
	Strategy         config.StrategyConfig
	Binding          strategy.StrategyBinding
	EnabledConfig    config.AgentEnabled
	EnabledApp       decision.AgentEnabled
	EnabledMap       map[string]decision.AgentEnabled
	BarInterval      time.Duration
	RequireMechanics bool
}

func buildRuntimeConfig(symbolCfg config.SymbolConfig, stratCfg config.StrategyConfig, bind strategy.StrategyBinding) (symbolRuntimeConfig, error) {
	enabledCfg, err := config.ResolveAgentEnabled(symbolCfg.Agent)
	if err != nil {
		return symbolRuntimeConfig{}, err
	}
	barInterval, err := interval.ShortestInterval(symbolCfg.Intervals)
	if err != nil {
		return symbolRuntimeConfig{}, err
	}
	enabledApp := decision.AgentEnabled{Indicator: enabledCfg.Indicator, Structure: enabledCfg.Structure, Mechanics: enabledCfg.Mechanics}
	enabledMap := map[string]decision.AgentEnabled{symbolCfg.Symbol: enabledApp}
	return symbolRuntimeConfig{
		Symbol:           symbolCfg,
		Strategy:         stratCfg,
		Binding:          bind,
		EnabledConfig:    enabledCfg,
		EnabledApp:       enabledApp,
		EnabledMap:       enabledMap,
		BarInterval:      barInterval,
		RequireMechanics: enabledCfg.Mechanics,
	}, nil
}

func loadDefaultRuntimeConfig(sys config.SystemConfig, base, symbol string) (symbolRuntimeConfig, error) {
	symbolPath := resolvePath(base, filepath.Join("symbols", "default.toml"))
	strategyPath := resolvePath(base, filepath.Join("strategies", "default.toml"))

	symbolCfg, err := config.LoadSymbolConfig(symbolPath)
	if err != nil {
		return symbolRuntimeConfig{}, err
	}
	symbolCfg.Symbol = symbol
	defaults, err := config.DefaultSymbolConfig(sys, symbol)
	if err != nil {
		return symbolRuntimeConfig{}, err
	}
	config.ApplyDecisionDefaults(&symbolCfg, defaults)
	if err := config.ValidateSymbolLLMModels(sys, symbolCfg); err != nil {
		return symbolRuntimeConfig{}, err
	}
	symbolHash, err := config.HashSymbolConfig(symbolCfg)
	if err != nil {
		return symbolRuntimeConfig{}, err
	}
	symbolCfg.Hash = symbolHash
	stratCfg, err := config.LoadStrategyConfigWithSymbol(strategyPath, symbol)
	if err != nil {
		return symbolRuntimeConfig{}, err
	}
	if err := validateInitialExitStructureInterval(symbolCfg, stratCfg); err != nil {
		return symbolRuntimeConfig{}, err
	}
	stratCfg.Hash = ""
	if updatedHash, err := config.HashStrategyConfig(stratCfg); err == nil {
		stratCfg.Hash = updatedHash
	}
	bind, err := strategy.BuildBinding(sys, stratCfg)
	if err != nil {
		return symbolRuntimeConfig{}, err
	}
	bind.StrategyHash = config.CombineHashes(symbolCfg.Hash, stratCfg.Hash)
	bind.SystemHash = sys.Hash
	runtimeCfg, err := buildRuntimeConfig(symbolCfg, stratCfg, bind)
	if err != nil {
		return symbolRuntimeConfig{}, err
	}
	if runtimeCfg.BarInterval <= 0 {
		runtimeCfg.BarInterval = 15 * time.Minute
	}
	return runtimeCfg, nil
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
