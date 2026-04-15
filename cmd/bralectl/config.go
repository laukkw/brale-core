package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"brale-core/internal/config"

	"github.com/spf13/cobra"
)

func configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "配置与运行参数",
	}
	cmd.AddCommand(configShowCmd())
	cmd.AddCommand(configValidateCmd())
	return cmd
}

func configShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "展示当前运行配置摘要",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newRuntimeClient()
			if err != nil {
				return err
			}
			resp, err := client.FetchMonitorStatus(cmd.Context())
			if err != nil {
				return err
			}
			if flagJSON {
				return printJSON(cmd.OutOrStdout(), resp)
			}
			if len(resp.Symbols) == 0 {
				return printBlock(cmd.OutOrStdout(), "No monitored symbols.")
			}
			rows := make([][]string, 0, len(resp.Symbols))
			for _, item := range resp.Symbols {
				rows = append(rows, []string{
					item.Symbol,
					item.KlineInterval,
					fmt.Sprintf("%.4f", item.RiskPct),
					fmt.Sprintf("%.2f", item.MaxLeverage),
					item.NextRun.UTC().Format("2006-01-02 15:04:05"),
				})
			}
			return printTable(cmd.OutOrStdout(), []string{"SYMBOL", "INTERVAL", "RISK_PCT", "MAX_LEVERAGE", "NEXT_RUN"}, rows)
		},
	}
}

func configValidateCmd() *cobra.Command {
	var systemPath string
	var indexPath string
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "离线验证 system/index/symbol/strategy 配置",
		RunE: func(cmd *cobra.Command, args []string) error {
			summary, err := validateConfigTree(systemPath, indexPath)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "validated %d symbol(s)\nsystem=%s\nindex=%s\n", summary.SymbolCount, summary.SystemPath, summary.IndexPath)
			return err
		},
	}
	cmd.Flags().StringVar(&systemPath, "system", "configs/system.toml", "system.toml 路径")
	cmd.Flags().StringVar(&indexPath, "index", "configs/symbols-index.toml", "symbols-index.toml 路径")
	return cmd
}

type configValidationSummary struct {
	SystemPath  string
	IndexPath   string
	SymbolCount int
}

func validateConfigTree(systemPath, indexPath string) (configValidationSummary, error) {
	systemPath = filepath.Clean(systemPath)
	indexPath = filepath.Clean(indexPath)
	sys, err := config.LoadSystemConfig(systemPath)
	if err != nil {
		return configValidationSummary{}, fmt.Errorf("load system config: %w", err)
	}
	indexCfg, err := config.LoadSymbolIndexConfig(indexPath)
	if err != nil {
		return configValidationSummary{}, fmt.Errorf("load symbol index config: %w", err)
	}
	for _, entry := range indexCfg.Symbols {
		symbolPath, err := resolveConfigPath(indexPath, entry.Config)
		if err != nil {
			return configValidationSummary{}, fmt.Errorf("resolve symbol config for %s: %w", entry.Symbol, err)
		}
		symbolCfg, err := config.LoadSymbolConfig(symbolPath)
		if err != nil {
			return configValidationSummary{}, fmt.Errorf("load symbol config %s: %w", entry.Symbol, err)
		}
		if err := config.ValidateSymbolLLMModels(sys, symbolCfg); err != nil {
			return configValidationSummary{}, fmt.Errorf("validate symbol llm models %s: %w", entry.Symbol, err)
		}
		strategyPath, err := resolveConfigPath(indexPath, entry.Strategy)
		if err != nil {
			return configValidationSummary{}, fmt.Errorf("resolve strategy config for %s: %w", entry.Symbol, err)
		}
		if _, err := config.LoadStrategyConfigWithSymbol(strategyPath, entry.Symbol); err != nil {
			return configValidationSummary{}, fmt.Errorf("load strategy config %s: %w", entry.Symbol, err)
		}
	}
	return configValidationSummary{
		SystemPath:  systemPath,
		IndexPath:   indexPath,
		SymbolCount: len(indexCfg.Symbols),
	}, nil
}

func resolveConfigPath(indexPath, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("path is empty")
	}
	if filepath.IsAbs(raw) {
		if _, err := os.Stat(raw); err != nil {
			return "", err
		}
		return raw, nil
	}
	candidates := []string{
		filepath.Clean(raw),
		filepath.Join(filepath.Dir(indexPath), raw),
		filepath.Join(filepath.Dir(filepath.Dir(indexPath)), raw),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return filepath.Join(filepath.Dir(indexPath), raw), nil
}
