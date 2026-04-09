package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "bralectl",
		Short: "brale-core 配置维护工具",
	}
	root.AddCommand(addSymbolCmd())
	root.AddCommand(llmCmd())
	if err := root.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
