package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func decisionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "decision",
		Short: "决策查询",
	}
	cmd.AddCommand(decisionLatestCmd())
	cmd.AddCommand(decisionHistoryCmd())
	return cmd
}

func decisionLatestCmd() *cobra.Command {
	var symbol string
	cmd := &cobra.Command{
		Use:   "latest",
		Short: "查看最新决策",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newRuntimeClient()
			if err != nil {
				return err
			}
			resp, err := client.FetchDecisionLatest(cmd.Context(), symbol)
			if err != nil {
				return err
			}
			if flagJSON {
				return printJSON(cmd.OutOrStdout(), resp)
			}
			body := strings.TrimSpace(resp.ReportMarkdown)
			if body == "" {
				body = strings.TrimSpace(resp.Report)
			}
			if body == "" {
				body = strings.TrimSpace(resp.Summary)
			}
			return printBlock(cmd.OutOrStdout(), body)
		},
	}
	cmd.Flags().StringVar(&symbol, "symbol", "", "交易对，例如 BTCUSDT")
	_ = cmd.MarkFlagRequired("symbol")
	return cmd
}

func decisionHistoryCmd() *cobra.Command {
	var symbol string
	var limit int
	var snapshotID uint
	cmd := &cobra.Command{
		Use:   "history",
		Short: "查看历史决策",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newRuntimeClient()
			if err != nil {
				return err
			}
			resp, err := client.FetchDashboardDecisionHistory(cmd.Context(), symbol, limit, snapshotID)
			if err != nil {
				return err
			}
			if flagJSON {
				return printJSON(cmd.OutOrStdout(), resp)
			}
			if len(resp.Items) == 0 {
				if strings.TrimSpace(resp.Message) != "" {
					return printBlock(cmd.OutOrStdout(), resp.Message)
				}
				return printBlock(cmd.OutOrStdout(), "No decision history.")
			}
			rows := make([][]string, 0, len(resp.Items))
			for _, item := range resp.Items {
				rows = append(rows, []string{
					fmt.Sprintf("%d", item.SnapshotID),
					item.Action,
					item.At,
					item.Reason,
				})
			}
			if err := printTable(cmd.OutOrStdout(), []string{"SNAPSHOT", "ACTION", "AT", "REASON"}, rows); err != nil {
				return err
			}
			if resp.Detail != nil && strings.TrimSpace(resp.Detail.ReportMarkdown) != "" {
				_, _ = fmt.Fprintln(cmd.OutOrStdout())
				return printBlock(cmd.OutOrStdout(), strings.TrimSpace(resp.Detail.ReportMarkdown))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&symbol, "symbol", "", "交易对，例如 BTCUSDT")
	cmd.Flags().IntVar(&limit, "limit", 20, "返回条数")
	cmd.Flags().UintVar(&snapshotID, "snapshot-id", 0, "指定快照 ID 时返回该轮详情")
	_ = cmd.MarkFlagRequired("symbol")
	return cmd
}
