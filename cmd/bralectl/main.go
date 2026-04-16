package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	if err := runCLI(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var (
	flagEndpoint string
	flagJSON     bool
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "bralectl",
		Short:        "brale-core 管理与运维工具",
		SilenceUsage: true,
	}
	root.PersistentFlags().StringVar(&flagEndpoint, "endpoint", "http://127.0.0.1:9991", "brale-core runtime API 地址")
	root.PersistentFlags().BoolVar(&flagJSON, "json", false, "以 JSON 格式输出")
	root.AddCommand(addSymbolCmd())
	root.AddCommand(llmCmd())
	root.AddCommand(observeCmd())
	root.AddCommand(positionCmd())
	root.AddCommand(decisionCmd())
	root.AddCommand(configCmd())
	root.AddCommand(scheduleCmd())
	root.AddCommand(backtestCmd())
	root.AddCommand(indicatorCmd())
	root.AddCommand(mcpCmd())
	root.AddCommand(memoryCmd())
	root.AddCommand(setupCmd())
	root.AddCommand(initCmd())
	root.AddCommand(prepareStackCmd())
	return root
}

func runCLI(args []string, stdout, stderr io.Writer) error {
	root := newRootCmd()
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(args)
	return root.Execute()
}
