package main

import (
	"context"
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"brale-core/internal/memory"
	"brale-core/internal/pgstore"

	"github.com/spf13/cobra"
)

func memoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "memory",
		Short: "记忆系统管理",
	}
	cmd.AddCommand(memoryListCmd())
	cmd.AddCommand(memoryAddCmd())
	cmd.AddCommand(memoryEditCmd())
	cmd.AddCommand(memoryDeleteCmd())
	cmd.AddCommand(memoryToggleCmd())
	cmd.AddCommand(memoryEpisodicCmd())
	return cmd
}

var memoryDBDSN string

func memoryStoreFromFlags() (*pgstore.PGStore, error) {
	dsn := memoryDBDSN
	if dsn == "" {
		dsn = "postgres://brale:brale@localhost:5432/brale?sslmode=disable"
	}
	pool, err := pgstore.OpenPool(context.Background(), dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	return pgstore.New(pool, nil), nil
}

func memoryListCmd() *cobra.Command {
	var symbol string
	var source string
	var all bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "列出语义规则",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := memoryStoreFromFlags()
			if err != nil {
				return err
			}
			sem := memory.NewSemanticMemory(s, 100)
			rules, err := sem.ListRules(symbol, !all, 100)
			if err != nil {
				return err
			}
			rules = filterRulesBySource(rules, source)
			if flagJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(rules)
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tSymbol\tSource\tConfidence\tActive\tRule")
			for _, r := range rules {
				sym := r.Symbol
				if sym == "" {
					sym = "(global)"
				}
				fmt.Fprintf(w, "%d\t%s\t%s\t%.1f\t%v\t%s\n", r.ID, sym, r.Source, r.Confidence, r.Active, truncate(r.RuleText, 60))
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&symbol, "symbol", "", "按交易对筛选")
	cmd.Flags().StringVar(&source, "source", "", "按来源筛选 (user/reflector)")
	cmd.Flags().BoolVar(&all, "all", false, "包含已禁用的规则")
	cmd.Flags().StringVar(&memoryDBDSN, "db", "", "数据库 DSN")
	return cmd
}

func memoryAddCmd() *cobra.Command {
	var symbol string
	var rule string
	var confidence float64
	cmd := &cobra.Command{
		Use:   "add",
		Short: "添加语义规则",
		RunE: func(cmd *cobra.Command, args []string) error {
			if rule == "" {
				return fmt.Errorf("--rule is required")
			}
			s, err := memoryStoreFromFlags()
			if err != nil {
				return err
			}
			sem := memory.NewSemanticMemory(s, 100)
			return sem.SaveRule(memory.Rule{
				Symbol:     symbol,
				RuleText:   rule,
				Source:     "user",
				Confidence: confidence,
				Active:     true,
			})
		},
	}
	cmd.Flags().StringVar(&symbol, "symbol", "", "交易对（空=全局）")
	cmd.Flags().StringVar(&rule, "rule", "", "规则文本")
	cmd.Flags().Float64Var(&confidence, "confidence", 0.8, "置信度 0-1")
	cmd.Flags().StringVar(&memoryDBDSN, "db", "", "数据库 DSN")
	return cmd
}

func memoryEditCmd() *cobra.Command {
	var id uint
	var rule string
	var confidence float64
	cmd := &cobra.Command{
		Use:   "edit",
		Short: "编辑语义规则",
		RunE: func(cmd *cobra.Command, args []string) error {
			if id == 0 {
				return fmt.Errorf("--id is required")
			}
			s, err := memoryStoreFromFlags()
			if err != nil {
				return err
			}
			sem := memory.NewSemanticMemory(s, 100)
			updates := map[string]any{}
			if rule != "" {
				updates["rule_text"] = rule
			}
			if cmd.Flags().Changed("confidence") {
				updates["confidence"] = confidence
			}
			if len(updates) == 0 {
				return fmt.Errorf("nothing to update")
			}
			return sem.UpdateRule(id, updates)
		},
	}
	cmd.Flags().UintVar(&id, "id", 0, "规则 ID")
	cmd.Flags().StringVar(&rule, "rule", "", "新的规则文本")
	cmd.Flags().Float64Var(&confidence, "confidence", 0, "新的置信度")
	cmd.Flags().StringVar(&memoryDBDSN, "db", "", "数据库 DSN")
	return cmd
}

func memoryDeleteCmd() *cobra.Command {
	var id uint
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "删除语义规则",
		RunE: func(cmd *cobra.Command, args []string) error {
			if id == 0 {
				return fmt.Errorf("--id is required")
			}
			s, err := memoryStoreFromFlags()
			if err != nil {
				return err
			}
			sem := memory.NewSemanticMemory(s, 100)
			return sem.DeleteRule(id)
		},
	}
	cmd.Flags().UintVar(&id, "id", 0, "规则 ID")
	cmd.Flags().StringVar(&memoryDBDSN, "db", "", "数据库 DSN")
	return cmd
}

func memoryToggleCmd() *cobra.Command {
	var id uint
	cmd := &cobra.Command{
		Use:   "toggle",
		Short: "切换规则启用状态",
		RunE: func(cmd *cobra.Command, args []string) error {
			if id == 0 {
				return fmt.Errorf("--id is required")
			}
			s, err := memoryStoreFromFlags()
			if err != nil {
				return err
			}
			sem := memory.NewSemanticMemory(s, 100)
			return sem.ToggleRule(id)
		},
	}
	cmd.Flags().UintVar(&id, "id", 0, "规则 ID")
	cmd.Flags().StringVar(&memoryDBDSN, "db", "", "数据库 DSN")
	return cmd
}

func memoryEpisodicCmd() *cobra.Command {
	var symbol string
	var limit int
	cmd := &cobra.Command{
		Use:   "episodic",
		Short: "查看交易反思记录",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := memoryStoreFromFlags()
			if err != nil {
				return err
			}
			ep := memory.NewEpisodicMemory(s, limit, 90)
			episodes, err := ep.ListEpisodes(symbol, limit)
			if err != nil {
				return err
			}
			if flagJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(episodes)
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tSymbol\tDirection\tEntry\tExit\tPnL%\tDuration")
			for _, ep := range episodes {
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
					ep.ID, ep.Symbol, ep.Direction, ep.EntryPrice, ep.ExitPrice, ep.PnLPercent, ep.Duration)
			}
			if err := w.Flush(); err != nil {
				return err
			}
			for _, ep := range episodes {
				if ep.Reflection != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "\n--- %s %s (ID: %d) ---\n反思: %s\n", ep.Symbol, ep.Direction, ep.ID, ep.Reflection)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&symbol, "symbol", "", "交易对")
	cmd.Flags().IntVar(&limit, "limit", 10, "返回条数")
	cmd.Flags().StringVar(&memoryDBDSN, "db", "", "数据库 DSN")
	return cmd
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func filterRulesBySource(rules []memory.Rule, source string) []memory.Rule {
	if source == "" {
		return rules
	}
	filtered := make([]memory.Rule, 0, len(rules))
	for _, r := range rules {
		if r.Source == source {
			filtered = append(filtered, r)
		}
	}
	return filtered
}
