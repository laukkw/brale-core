package main

import "github.com/spf13/cobra"

func llmCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "llm",
		Short: "LLM 诊断工具",
	}
	cmd.AddCommand(probeLLMCmd())
	cmd.AddCommand(llmRoundsCmd())
	return cmd
}
