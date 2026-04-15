package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func positionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "position",
		Short: "持仓查询",
	}
	cmd.AddCommand(positionListCmd())
	cmd.AddCommand(positionHistoryCmd())
	return cmd
}

func positionListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "查看当前持仓",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newRuntimeClient()
			if err != nil {
				return err
			}
			resp, err := client.FetchPositionStatus(cmd.Context())
			if err != nil {
				return err
			}
			if flagJSON {
				return printJSON(cmd.OutOrStdout(), resp)
			}
			if len(resp.Positions) == 0 {
				return printBlock(cmd.OutOrStdout(), "No open positions.")
			}
			rows := make([][]string, 0, len(resp.Positions))
			for _, pos := range resp.Positions {
				rows = append(rows, []string{
					pos.Symbol,
					pos.Side,
					fmt.Sprintf("%.6f", pos.EntryPrice),
					fmt.Sprintf("%.6f", pos.CurrentPrice),
					fmt.Sprintf("%.4f", pos.ProfitTotal),
				})
			}
			return printTable(cmd.OutOrStdout(), []string{"SYMBOL", "SIDE", "ENTRY", "CURRENT", "PNL"}, rows)
		},
	}
}

func positionHistoryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "history",
		Short: "查看持仓历史",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newRuntimeClient()
			if err != nil {
				return err
			}
			resp, err := client.FetchTradeHistory(cmd.Context())
			if err != nil {
				return err
			}
			if flagJSON {
				return printJSON(cmd.OutOrStdout(), resp)
			}
			if len(resp.Trades) == 0 {
				return printBlock(cmd.OutOrStdout(), "No trade history.")
			}
			rows := make([][]string, 0, len(resp.Trades))
			for _, trade := range resp.Trades {
				rows = append(rows, []string{
					trade.Symbol,
					trade.Side,
					trade.OpenedAt.UTC().Format("2006-01-02 15:04:05"),
					fmt.Sprintf("%d", trade.DurationSec),
					fmt.Sprintf("%.4f", trade.Profit),
				})
			}
			return printTable(cmd.OutOrStdout(), []string{"SYMBOL", "SIDE", "OPENED", "DURATION_S", "PROFIT"}, rows)
		},
	}
}
