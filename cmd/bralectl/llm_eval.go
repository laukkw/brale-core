package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"brale-core/internal/llm/eval"

	"github.com/spf13/cobra"
)

func llmEvalCmd() *cobra.Command {
	var symbol string
	var limit int

	cmd := &cobra.Command{
		Use:   "eval",
		Short: "评估最近的 LLM rounds 元数据质量",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := llmStoreFromFlags()
			if err != nil {
				return err
			}

			report, err := (eval.Harness{Store: s}).Run(context.Background(), eval.Request{
				Symbol: symbol,
				Limit:  limit,
			})
			if err != nil {
				return fmt.Errorf("run llm eval: %w", err)
			}
			if flagJSON {
				return printJSON(cmd.OutOrStdout(), report)
			}

			if err := printBlock(cmd.OutOrStdout(), fmt.Sprintf(
				"Total rounds: %d\nError rounds: %d\nAverage score: %.2f\nAverage latency: %.1fms\nAverage tokens in/out: %.1f / %.1f",
				report.Summary.TotalRounds,
				report.Summary.ErrorRounds,
				report.Summary.AverageScore,
				report.Summary.AverageLatencyMS,
				report.Summary.AverageTokenIn,
				report.Summary.AverageTokenOut,
			)); err != nil {
				return err
			}

			rows := make([][]string, 0, len(report.Results))
			for _, item := range report.Results {
				rows = append(rows, []string{
					shortID(item.RoundID),
					item.Symbol,
					item.RoundType,
					normalizeText(item.PromptVersion, "-"),
					normalizeText(item.Outcome, "-"),
					fmt.Sprintf("%.2f", item.Score),
					strconv.Itoa(item.LatencyMS) + "ms",
					fmt.Sprintf("%d/%d", item.TokenIn, item.TokenOut),
					normalizeText(item.Error, "-"),
				})
			}
			return printTable(cmd.OutOrStdout(),
				[]string{"ID", "Symbol", "Type", "Prompt", "Outcome", "Score", "Latency", "Tokens", "Error"},
				rows,
			)
		},
	}
	cmd.Flags().StringVar(&symbol, "symbol", "", "Filter by trading pair")
	cmd.Flags().IntVar(&limit, "limit", 20, "Max rounds to evaluate")
	cmd.Flags().StringVar(&llmDBDSN, "db", "", "Database DSN")
	return cmd
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func normalizeText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
