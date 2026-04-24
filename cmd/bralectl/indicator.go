package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"brale-core/internal/config"
	"brale-core/internal/decision/decisionutil"
	"brale-core/internal/decision/features"
	"brale-core/internal/market/binance"
	"brale-core/internal/snapshot"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var newIndicatorDiffFetcher = func() *snapshot.Fetcher {
	return binance.NewSnapshotFetcher(binance.SnapshotOptions{})
}

func indicatorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "indicator",
		Short: "指标引擎工具",
	}
	cmd.AddCommand(indicatorDiffCmd())
	return cmd
}

func indicatorDiffCmd() *cobra.Command {
	var (
		symbol       string
		indexPath    string
		format       string
		baselineName string
		candidate    string
	)
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "对比 talib 与 reference 指标引擎差异",
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := buildIndicatorDiffReport(cmd.Context(), indexPath, symbol, baselineName, candidate)
			if err != nil {
				return err
			}
			return writeIndicatorDiffOutput(cmd.OutOrStdout(), report, format)
		},
	}
	cmd.Flags().StringVar(&symbol, "symbol", "", "交易对，例如 BTCUSDT")
	cmd.Flags().StringVar(&indexPath, "index", "configs/symbols-index.toml", "symbols-index.toml 路径")
	cmd.Flags().StringVar(&format, "format", "text", "输出格式：text|json")
	cmd.Flags().StringVar(&baselineName, "baseline", "talib", "基准引擎：ta|talib|reference")
	cmd.Flags().StringVar(&candidate, "candidate", "ta", "对比引擎：ta|talib|reference")
	_ = cmd.MarkFlagRequired("symbol")
	return cmd
}

func buildIndicatorDiffReport(ctx context.Context, indexPath, symbol, baselineName, candidateName string) (features.EngineDiffReport, error) {
	indexPath = filepath.Clean(indexPath)
	symbol = decisionutil.NormalizeSymbol(symbol)
	if symbol == "" {
		return features.EngineDiffReport{}, fmt.Errorf("symbol is required")
	}
	indexCfg, err := loadIndicatorDiffIndex(indexPath)
	if err != nil {
		return features.EngineDiffReport{}, fmt.Errorf("load symbol index config: %w", err)
	}
	baseline, baselineLabel, err := resolveIndicatorDiffComputer(baselineName)
	if err != nil {
		return features.EngineDiffReport{}, err
	}
	candidate, candidateLabel, err := resolveIndicatorDiffComputer(candidateName)
	if err != nil {
		return features.EngineDiffReport{}, err
	}
	for _, item := range indexCfg.Symbols {
		if decisionutil.NormalizeSymbol(item.Symbol) != symbol {
			continue
		}
		symbolPath := resolveIndicatorDiffPath(filepath.Dir(indexPath), item.Config)
		symbolCfg, err := config.LoadSymbolConfig(symbolPath)
		if err != nil {
			return features.EngineDiffReport{}, fmt.Errorf("load symbol config: %w", err)
		}
		if decisionutil.NormalizeSymbol(symbolCfg.Symbol) != symbol {
			return features.EngineDiffReport{}, fmt.Errorf("symbol config mismatch: %s", symbolCfg.Symbol)
		}
		enabled, err := config.ResolveAgentEnabled(symbolCfg.Agent)
		if err != nil {
			return features.EngineDiffReport{}, fmt.Errorf("resolve agent enabled: %w", err)
		}
		fetcher := newIndicatorDiffFetcher()
		snap, err := fetcher.Fetch(ctx, []string{symbolCfg.Symbol}, symbolCfg.Intervals, symbolCfg.KlineLimit)
		if err != nil {
			return features.EngineDiffReport{}, fmt.Errorf("fetch candles: %w", err)
		}
		trendPresets := config.TrendPresetForIntervals(symbolCfg.Intervals)
		trendOptions := make(map[string]features.TrendCompressOptions, len(trendPresets))
		for interval, preset := range trendPresets {
			trendOptions[interval] = trendOptionsFromPreset(preset)
		}
		return features.RunEngineDiff(features.EngineDiffRequest{
			Symbol:            symbolCfg.Symbol,
			BaselineName:      baselineLabel,
			Baseline:          baseline,
			CandidateName:     candidateLabel,
			Candidate:         candidate,
			IndicatorOptions:  indicatorOptionsFromConfig(symbolCfg.Indicators, enabled.Indicator),
			TrendOptionsByInt: trendOptions,
			DefaultTrendOpts:  trendOptionsFromPreset(config.DefaultTrendPreset()),
			CandlesByInterval: snap.Klines[symbolCfg.Symbol],
		})
	}
	return features.EngineDiffReport{}, fmt.Errorf("symbol %s not found in %s", symbol, indexPath)
}

func resolveIndicatorDiffComputer(name string) (features.IndicatorComputer, string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "ta":
		return features.TAComputer{}, "ta", nil
	case "", "talib":
		return features.TalibComputer{}, "talib", nil
	case "reference":
		return features.ReferenceComputer{}, "reference", nil
	default:
		return nil, "", fmt.Errorf("unsupported engine %q", name)
	}
}

func loadIndicatorDiffIndex(path string) (config.SymbolIndexConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return config.SymbolIndexConfig{}, err
	}
	v := viper.NewWithOptions(viper.KeyDelimiter("::"))
	v.SetConfigType("toml")
	if err := v.ReadConfig(bytes.NewReader(data)); err != nil {
		return config.SymbolIndexConfig{}, err
	}
	var cfg config.SymbolIndexConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return config.SymbolIndexConfig{}, err
	}
	config.NormalizeSymbolIndexConfig(&cfg)
	if len(cfg.Symbols) == 0 {
		return config.SymbolIndexConfig{}, fmt.Errorf("symbols is required")
	}
	seen := make(map[string]struct{}, len(cfg.Symbols))
	for _, item := range cfg.Symbols {
		symbol := decisionutil.NormalizeSymbol(item.Symbol)
		if symbol == "" {
			return config.SymbolIndexConfig{}, fmt.Errorf("symbols.symbol is required")
		}
		if _, ok := seen[symbol]; ok {
			return config.SymbolIndexConfig{}, fmt.Errorf("symbols contains duplicate symbol=%s", symbol)
		}
		seen[symbol] = struct{}{}
		if strings.TrimSpace(item.Config) == "" {
			return config.SymbolIndexConfig{}, fmt.Errorf("symbols.%s config path is required", symbol)
		}
	}
	return cfg, nil
}

func writeIndicatorDiffOutput(w io.Writer, report features.EngineDiffReport, format string) error {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "text"
	}
	switch format {
	case "json":
		return printJSON(w, report)
	case "text":
		if err := printBlock(w, fmt.Sprintf("Symbol: %s", report.Symbol)); err != nil {
			return err
		}
		if err := printBlock(w, fmt.Sprintf("Baseline: %s", report.Baseline)); err != nil {
			return err
		}
		if err := printBlock(w, fmt.Sprintf("Candidate: %s", report.Candidate)); err != nil {
			return err
		}
		for _, interval := range report.Intervals {
			if err := printBlock(w, ""); err != nil {
				return err
			}
			if err := printBlock(w, fmt.Sprintf("Interval: %s", interval.Interval)); err != nil {
				return err
			}
			rows := make([][]string, 0, len(interval.Numeric))
			for _, diff := range interval.Numeric {
				latest := "n/a"
				if diff.LatestComparable {
					latest = fmt.Sprintf("%.6f", diff.LatestDiff)
				}
				rows = append(rows, []string{
					diff.Name,
					fmt.Sprintf("%.6f", diff.MaxDiff),
					fmt.Sprintf("%.6f", diff.AvgDiff),
					latest,
					fmt.Sprintf("%d", diff.ComparablePoints),
					fmt.Sprintf("%d/%d", diff.BaselineWarmup, diff.CandidateWarmup),
				})
			}
			if err := printTable(w, []string{"name", "max_diff", "avg_diff", "latest_diff", "points", "warmup"}, rows); err != nil {
				return err
			}
			if err := printBlock(w, "semantic checks:"); err != nil {
				return err
			}
			semanticRows := make([][]string, 0, len(interval.Semantics))
			for _, diff := range interval.Semantics {
				semanticRows = append(semanticRows, []string{diff.Name, diff.Baseline, diff.Candidate, fmt.Sprintf("%t", diff.Match)})
			}
			if err := printTable(w, []string{"name", "baseline", "candidate", "match"}, semanticRows); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}

func indicatorOptionsFromConfig(cfg config.IndicatorConfig, indicatorEnabled bool) features.IndicatorCompressOptions {
	opts := features.IndicatorCompressOptions{
		EMAFast:        cfg.EMAFast,
		EMAMid:         cfg.EMAMid,
		EMASlow:        cfg.EMASlow,
		RSIPeriod:      cfg.RSIPeriod,
		ATRPeriod:      cfg.ATRPeriod,
		STCFast:        cfg.STCFast,
		STCSlow:        cfg.STCSlow,
		BBPeriod:       cfg.BBPeriod,
		BBMultiplier:   cfg.BBMultiplier,
		CHOPPeriod:     cfg.CHOPPeriod,
		StochRSIPeriod: cfg.StochRSIPeriod,
		AroonPeriod:    cfg.AroonPeriod,
		LastN:          cfg.LastN,
		Pretty:         cfg.Pretty,
		SkipSTC:        cfg.SkipSTC,
	}
	if !indicatorEnabled {
		opts.SkipEMA = true
		opts.SkipRSI = true
		opts.SkipSTC = true
	}
	return opts
}

func trendOptionsFromPreset(preset config.TrendPreset) features.TrendCompressOptions {
	return features.TrendCompressOptions{
		FractalSpan:          preset.FractalSpan,
		MaxStructurePoints:   preset.MaxStructurePoints,
		DedupDistanceBars:    preset.DedupDistanceBars,
		DedupATRFactor:       preset.DedupATRFactor,
		SuperTrendPeriod:     preset.SuperTrendPeriod,
		SuperTrendMultiplier: preset.SuperTrendMultiplier,
		SkipSuperTrend:       preset.SkipSuperTrend,
		RSIPeriod:            preset.RSIPeriod,
		ATRPeriod:            preset.ATRPeriod,
		RecentCandles:        preset.RecentCandles,
		VolumeMAPeriod:       preset.VolumeMAPeriod,
		EMA20Period:          preset.EMA20Period,
		EMA50Period:          preset.EMA50Period,
		EMA200Period:         preset.EMA200Period,
		PatternMinScore:      preset.PatternMinScore,
		PatternMaxDetected:   preset.PatternMaxDetected,
		Pretty:               preset.Pretty,
		IncludeCurrentRSI:    preset.IncludeCurrentRSI,
		IncludeStructureRSI:  preset.IncludeStructureRSI,
		EmitEMAContext:       true,
		EmitPatterns:         true,
		EmitSMC:              true,
	}
}

func resolveIndicatorDiffPath(base, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, path)
}
