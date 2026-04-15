package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

func llmPromptsCmd() *cobra.Command {
	var role string
	var activeOnly bool

	cmd := &cobra.Command{
		Use:   "prompts",
		Short: "查看 prompt registry",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "列出 prompt registry 条目",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := llmStoreFromFlags()
			if err != nil {
				return err
			}
			items, err := s.ListPromptEntries(context.Background(), role, activeOnly)
			if err != nil {
				return fmt.Errorf("list prompt registry: %w", err)
			}
			if flagJSON {
				return printJSON(cmd.OutOrStdout(), items)
			}
			rows := make([][]string, 0, len(items))
			for _, item := range items {
				rows = append(rows, []string{
					strconv.FormatUint(uint64(item.ID), 10),
					item.Role,
					item.Stage,
					item.Version,
					strconv.FormatBool(item.Active),
					item.CreatedAt.Format("2006-01-02 15:04:05"),
					item.Description,
				})
			}
			return printTable(cmd.OutOrStdout(),
				[]string{"ID", "Role", "Stage", "Version", "Active", "Created", "Description"},
				rows,
			)
		},
	})
	cmd.PersistentFlags().StringVar(&role, "role", "", "Filter by role")
	cmd.PersistentFlags().BoolVar(&activeOnly, "active-only", false, "Only list active prompts")
	cmd.PersistentFlags().StringVar(&llmDBDSN, "db", "", "Database DSN")
	return cmd
}
