package main

import (
	"fmt"
	"os"

	"brale-core/internal/onboarding"

	"github.com/spf13/cobra"
)

func prepareStackCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prepare-stack",
		Short: "Generate runtime configs from .env and base config templates",
		Long: `Reads .env and base config files, then generates freqtrade runtime config,
proxy env file, and optionally a system config output. This is the same logic
used by 'make prepare' and 'bralectl init'.`,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("resolve working directory: %w", err)
			}
			return onboarding.RunPrepareStack(args, cwd, cmd.OutOrStdout())
		},
	}
	return cmd
}
