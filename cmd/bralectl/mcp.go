package main

import (
	"fmt"
	"strings"

	"brale-core/internal/mcp"

	"github.com/spf13/cobra"
)

const defaultMCPHTTPAddr = "127.0.0.1:8765"

func mcpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "MCP server 与安装工具",
	}
	cmd.AddCommand(mcpServeCmd())
	cmd.AddCommand(mcpInstallCmd())
	return cmd
}

func mcpServeCmd() *cobra.Command {
	var mode string
	var addr string
	var systemPath string
	var indexPath string
	var auditPath string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "启动只读 MCP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := normalizeMCPServeMode(mode)
			if err != nil {
				return err
			}
			if strings.TrimSpace(auditPath) == "" {
				defaultPath, err := mcp.DefaultAuditLogPath()
				if err != nil {
					return err
				}
				auditPath = defaultPath
			}
			runtimeClient, err := newRuntimeClient()
			if err != nil {
				return err
			}
			audit, err := mcp.NewFileAuditSink(auditPath)
			if err != nil {
				return err
			}
			defer audit.Close()
			opts := mcp.Options{
				Name:    "brale-core",
				Version: "dev",
				Runtime: runtimeClient,
				Config:  mcp.NewLocalConfigSource(systemPath, indexPath),
				Audit:   audit,
			}
			switch mode {
			case "stdio":
				return mcp.Serve(cmd.Context(), opts)
			case "http":
				return mcp.ServeHTTP(cmd.Context(), opts, addr)
			default:
				return fmt.Errorf("unsupported MCP mode %q", mode)
			}
		},
	}
	cmd.Flags().StringVar(&mode, "mode", "stdio", "MCP 传输模式：stdio 或 http")
	cmd.Flags().StringVar(&addr, "addr", defaultMCPHTTPAddr, "HTTP 模式监听地址")
	cmd.Flags().StringVar(&systemPath, "system", "configs/system.toml", "system.toml 路径")
	cmd.Flags().StringVar(&indexPath, "index", "configs/symbols-index.toml", "symbols-index.toml 路径")
	cmd.Flags().StringVar(&auditPath, "audit-log", "", "MCP audit 日志文件路径")
	return cmd
}

func normalizeMCPServeMode(raw string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		return "stdio", nil
	}
	switch mode {
	case "stdio", "http":
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported MCP mode %q", raw)
	}
}

func mcpInstallCmd() *cobra.Command {
	var (
		target     string
		mode       string
		configPath string
		command    string
		name       string
		systemPath string
		indexPath  string
		auditPath  string
	)
	cmd := &cobra.Command{
		Use:   "install",
		Short: "写入 MCP client 配置",
		RunE: func(cmd *cobra.Command, args []string) error {
			installMode := mode
			if flag := cmd.Flags().Lookup("mode"); flag != nil && !flag.Changed && strings.EqualFold(strings.TrimSpace(target), "codex") {
				installMode = "stdio"
			}
			result, err := mcp.Install(mcp.InstallOptions{
				Name:       name,
				Command:    command,
				ConfigPath: configPath,
				Target:     target,
				Mode:       installMode,
				Endpoint:   flagEndpoint,
				SystemPath: systemPath,
				IndexPath:  indexPath,
				AuditPath:  auditPath,
			})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "installed MCP server %q to %s\n", result.ServerName, result.ConfigPath)
			return err
		},
	}
	cmd.Flags().StringVar(&target, "target", "claude-code", "安装目标：claude-code、claude-desktop、opencode、codex 或 custom")
	cmd.Flags().StringVar(&mode, "mode", "http", "安装模式：http（默认）或 stdio")
	cmd.Flags().StringVar(&configPath, "config", "", "显式指定 MCP 配置文件路径")
	cmd.Flags().StringVar(&command, "command", "", "bralectl 可执行文件路径")
	cmd.Flags().StringVar(&name, "name", "brale-core", "MCP server 名称")
	cmd.Flags().StringVar(&systemPath, "system", "configs/system.toml", "system.toml 路径")
	cmd.Flags().StringVar(&indexPath, "index", "configs/symbols-index.toml", "symbols-index.toml 路径")
	cmd.Flags().StringVar(&auditPath, "audit-log", "", "MCP audit 日志文件路径")
	return cmd
}
