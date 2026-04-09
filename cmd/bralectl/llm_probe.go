package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"brale-core/internal/llmctl"

	"github.com/spf13/cobra"
)

func probeLLMCmd() *cobra.Command {
	var repoRoot string
	var stage string

	cmd := &cobra.Command{
		Use:   "probe",
		Short: "探测当前配置的 LLM 是否支持 structured output",
		RunE: func(cmd *cobra.Command, args []string) error {
			targets, err := llmctl.LoadProbeTargets(repoRoot, stage)
			if err != nil {
				return err
			}
			results := llmctl.ProbeTargets(context.Background(), targets)
			failures := 0
			for _, result := range results {
				if result.Supported {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\tOK\tmodel=%s\tendpoint=%s\n",
						result.Target.Stage, result.Target.Model, result.Target.Endpoint)
					continue
				}
				failures++
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\tFAIL\tmodel=%s\tendpoint=%s\terr=%s\n",
					result.Target.Stage, result.Target.Model, result.Target.Endpoint, compactError(result.Error))
			}
			if failures > 0 {
				return fmt.Errorf("%d probe(s) failed", failures)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&repoRoot, "repo", ".", "项目根目录路径")
	cmd.Flags().StringVar(&stage, "stage", "", "只探测单个 stage: indicator|structure|mechanics")
	cmd.SetOut(os.Stdout)
	return cmd
}

func compactError(err error) string {
	if err == nil {
		return ""
	}
	s := strings.TrimSpace(err.Error())
	s = strings.ReplaceAll(s, "\n", " | ")
	return s
}
