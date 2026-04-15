package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func scheduleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "调度控制",
	}
	cmd.AddCommand(scheduleEnableCmd())
	cmd.AddCommand(scheduleDisableCmd())
	cmd.AddCommand(scheduleStatusCmd())
	return cmd
}

func scheduleEnableCmd() *cobra.Command {
	return newScheduleToggleCmd(true)
}

func scheduleDisableCmd() *cobra.Command {
	return newScheduleToggleCmd(false)
}

func newScheduleToggleCmd(enable bool) *cobra.Command {
	var assumeYes bool
	use := "enable"
	short := "启用自动调度"
	prompt := "确认启用自动调度"
	if !enable {
		use = "disable"
		short = "停用自动调度"
		prompt = "确认停用自动调度"
	}
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			ok, err := confirmAction(cmd, prompt, assumeYes)
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
			resp, err := client.PostScheduleToggle(cmd.Context(), enable)
			if err != nil {
				return err
			}
			if flagJSON {
				return printJSON(cmd.OutOrStdout(), resp)
			}
			return printBlock(cmd.OutOrStdout(), resp.Summary)
		},
	}
	cmd.Flags().BoolVar(&assumeYes, "yes", false, "跳过确认提示")
	return cmd
}

func scheduleStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "查看调度状态",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newRuntimeClient()
			if err != nil {
				return err
			}
			resp, err := client.FetchScheduleStatus(cmd.Context())
			if err != nil {
				return err
			}
			if flagJSON {
				return printJSON(cmd.OutOrStdout(), resp)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "mode=%s\nscheduled=%t\n", resp.Mode, resp.LLMScheduled); err != nil {
				return err
			}
			if len(resp.NextRuns) == 0 {
				return printBlock(cmd.OutOrStdout(), resp.Summary)
			}
			rows := make([][]string, 0, len(resp.NextRuns))
			for _, item := range resp.NextRuns {
				rows = append(rows, []string{item.Symbol, item.BarInterval, item.NextExecution})
			}
			return printTable(cmd.OutOrStdout(), []string{"SYMBOL", "BAR_INTERVAL", "NEXT_EXECUTION"}, rows)
		},
	}
}
