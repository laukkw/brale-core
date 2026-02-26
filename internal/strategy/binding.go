// 本文件主要内容：根据系统与单币种策略配置构建策略绑定。

package strategy

import (
	"fmt"
	"strings"

	"brale-core/internal/config"
)

type StrategyBinding struct {
	Symbol          string
	RuleChainPath   string
	RiskManagement  config.RiskManagementConfig
	StrategyID      string
	StrategyHash    string
	SystemHash      string
}

func BuildBinding(sys config.SystemConfig, strat config.StrategyConfig) (StrategyBinding, error) {
	if err := ValidateBinding(sys, strat); err != nil {
		return StrategyBinding{}, err
	}
	return StrategyBinding{
		Symbol:          strat.Symbol,
		RuleChainPath:   strat.RuleChainPath,
		RiskManagement:  strat.RiskManagement,
		StrategyID:      strat.ID,
		StrategyHash:    strat.Hash,
		SystemHash:      sys.Hash,
	}, nil
}

func ValidateBinding(_ config.SystemConfig, strat config.StrategyConfig) error {
	sym := strings.TrimSpace(strat.Symbol)
	if sym == "" {
		return fmt.Errorf("symbol is required")
	}
	return nil
}
