package main

import (
	"strings"

	"brale-core/internal/transport/botruntime"

	"github.com/spf13/cobra"
)

func observeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "observe",
		Short: "观察市场数据",
	}
	cmd.AddCommand(observeReportCmd())
	cmd.AddCommand(observeRunCmd())
	return cmd
}

func observeReportCmd() *cobra.Command {
	var symbol string
	cmd := &cobra.Command{
		Use:   "report",
		Short: "查看最近一次观察报告",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newRuntimeClient()
			if err != nil {
				return err
			}
			resp, err := client.FetchObserveReport(cmd.Context(), symbol)
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

func observeRunCmd() *cobra.Command {
	var symbol string
	var assumeYes bool
	cmd := &cobra.Command{
		Use:   "run",
		Short: "手动触发一次观察",
		RunE: func(cmd *cobra.Command, args []string) error {
			ok, err := confirmAction(cmd, "确认手动触发 observe", assumeYes)
			if err != nil {
				return err
			}
			if !ok {
				return printBlock(cmd.OutOrStdout(), "Canceled.")
			}
			client, err := newRuntimeClient()
			if err != nil {
				return err
			}
			resp, err := client.RunObserve(cmd.Context(), botruntime.ObserveRunRequest{Symbol: symbol})
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
	cmd.Flags().BoolVar(&assumeYes, "yes", false, "跳过确认提示")
	_ = cmd.MarkFlagRequired("symbol")
	return cmd
}
