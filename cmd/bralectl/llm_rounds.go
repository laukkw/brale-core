package main

import (
	"context"
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/pgstore"

	"github.com/spf13/cobra"
)

var llmDBDSN string

func llmRoundsCmd() *cobra.Command {
	var symbol string
	var limit int

	cmd := &cobra.Command{
		Use:   "rounds",
		Short: "List LLM round records",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := llmStoreFromFlags()
			if err != nil {
				return err
			}

			rounds, err := s.ListLLMRounds(context.Background(), symbol, limit)
			if err != nil {
				return fmt.Errorf("query llm rounds: %w", err)
			}

			if flagJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(rounds)
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tSymbol\tType\tOutcome\tLatency\tTokens\tCalls\tGate\tStarted")
			for _, r := range rounds {
				tokens := fmt.Sprintf("%d/%d", r.TotalTokenIn, r.TotalTokenOut)
				latency := fmt.Sprintf("%dms", r.TotalLatencyMS)
				started := r.StartedAt.Format(time.DateTime)
				outcome := r.Outcome
				if outcome == "" {
					outcome = "-"
				}
				gate := r.GateAction
				if gate == "" {
					gate = "-"
				}
				idShort := r.ID
				if len(idShort) > 8 {
					idShort = idShort[:8]
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
					idShort, r.Symbol, r.RoundType, outcome, latency, tokens, r.CallCount, gate, started)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&symbol, "symbol", "", "Filter by trading pair")
	cmd.Flags().IntVar(&limit, "limit", 20, "Max results to return")
	cmd.Flags().StringVar(&llmDBDSN, "db", "", "Database DSN")
	return cmd
}

func llmStoreFromFlags() (*pgstore.PGStore, error) {
	dsn := llmDBDSN
	if dsn == "" {
		dsn = "postgres://brale:brale@localhost:5432/brale?sslmode=disable"
	}
	pool, err := pgstore.OpenPool(context.Background(), config.DatabaseConfig{DSN: dsn})
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	return pgstore.New(pool, nil), nil
}
