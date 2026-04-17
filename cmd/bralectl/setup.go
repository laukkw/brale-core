package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"brale-core/internal/mcp"

	huh "charm.land/huh/v2"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const (
	setupLangZH = "zh"
	setupLangEN = "en"
)

type setupCommandOptions struct {
	repoRoot   string
	lang       string
	installMCP bool
	skipMCP    bool
	mcpTarget  string
	mcpMode    string
	configPath string
	command    string
	name       string
	systemPath string
	indexPath  string
	auditPath  string
}

type setupTexts struct {
	envCreated      string
	envExists       string
	noMCPInstalled  string
	mcpInstalledFmt string
	nextStep        string
	promptInstall   string
	promptTarget    string
	promptEndpoint  string
	promptConfig    string
	yesLabel        string
	noLabel         string
}

var setupMessages = map[string]setupTexts{
	setupLangZH: {
		envCreated:      "[OK] 已从 .env.example 创建 .env",
		envExists:       "[OK] 已存在 .env，保持不覆盖",
		noMCPInstalled:  "[OK] setup 完成：未安装 MCP 客户端配置",
		mcpInstalledFmt: "[OK] setup 完成：已写入 MCP 配置到 %s",
		nextStep:        "[NEXT] 如需填写交易所与 LLM 凭据，请编辑 .env 或运行 make init；完成后执行 make start 启动 Docker 栈。",
		promptInstall:   "是否安装 MCP 客户端配置？",
		promptTarget:    "选择 MCP 客户端目标",
		promptEndpoint:  "输入 MCP endpoint（brale runtime API 地址）",
		promptConfig:    "输入自定义 MCP 配置文件路径",
		yesLabel:        "是",
		noLabel:         "否",
	},
	setupLangEN: {
		envCreated:      "[OK] created .env from .env.example",
		envExists:       "[OK] using existing .env",
		noMCPInstalled:  "[OK] setup finished without installing MCP client config",
		mcpInstalledFmt: "[OK] setup finished: installed MCP client config to %s",
		nextStep:        "[NEXT] Edit .env with your exchange and LLM credentials, or run make init for the interactive CLI wizard. Then run make start to launch the Docker stack.",
		promptInstall:   "Install MCP client config?",
		promptTarget:    "Select MCP client target",
		promptEndpoint:  "Enter MCP endpoint (brale runtime API address)",
		promptConfig:    "Enter a custom MCP config path",
		yesLabel:        "Yes",
		noLabel:         "No",
	},
}

type setupTargetOption struct {
	value   string
	labelZH string
	labelEN string
}

var setupTargets = []setupTargetOption{
	{value: "claude-code", labelZH: "Claude Code", labelEN: "Claude Code"},
	{value: "claude-desktop", labelZH: "Claude Desktop", labelEN: "Claude Desktop"},
	{value: "opencode", labelZH: "OpenCode", labelEN: "OpenCode"},
	{value: "codex", labelZH: "Codex", labelEN: "Codex"},
	{value: "custom", labelZH: "自定义路径", labelEN: "Custom path"},
}

func setupCmd() *cobra.Command {
	opts := setupCommandOptions{}
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "初始化 .env，并可选安装 MCP 客户端配置",
		RunE: func(cmd *cobra.Command, args []string) error {
			err := runSetup(cmd, opts)
			if errors.Is(err, huh.ErrUserAborted) {
				fmt.Fprintln(cmd.ErrOrStderr(), "\n✗ Setup cancelled.")
				return nil
			}
			return err
		},
	}
	cmd.Flags().StringVar(&opts.repoRoot, "repo", ".", "仓库根目录路径")
	cmd.Flags().StringVar(&opts.lang, "lang", "", "向导语言：zh 或 en")
	cmd.Flags().BoolVar(&opts.installMCP, "install-mcp", false, "执行 MCP 客户端配置安装")
	cmd.Flags().BoolVar(&opts.skipMCP, "skip-mcp", false, "跳过 MCP 客户端配置安装")
	cmd.Flags().StringVar(&opts.mcpTarget, "mcp-target", "", "MCP 安装目标：claude-code、claude-desktop、opencode、codex 或 custom")
	cmd.Flags().StringVar(&opts.mcpMode, "mcp-mode", "", "MCP 安装模式：http 或 stdio（默认沿用 install 默认值）")
	cmd.Flags().StringVar(&opts.configPath, "config", "", "显式指定 MCP 配置文件路径")
	cmd.Flags().StringVar(&opts.command, "command", "", "bralectl 可执行文件路径")
	cmd.Flags().StringVar(&opts.name, "name", "brale-core", "MCP server 名称")
	cmd.Flags().StringVar(&opts.systemPath, "system", "", "system.toml 路径（默认使用 <repo>/configs/system.toml）")
	cmd.Flags().StringVar(&opts.indexPath, "index", "", "symbols-index.toml 路径（默认使用 <repo>/configs/symbols-index.toml）")
	cmd.Flags().StringVar(&opts.auditPath, "audit-log", "", "MCP 审计日志文件路径")
	return cmd
}

func runSetup(cmd *cobra.Command, opts setupCommandOptions) error {
	if opts.installMCP && opts.skipMCP {
		return fmt.Errorf("--install-mcp and --skip-mcp cannot be used together")
	}
	if opts.skipMCP && (strings.TrimSpace(opts.mcpTarget) != "" || strings.TrimSpace(opts.mcpMode) != "" || strings.TrimSpace(opts.configPath) != "") {
		return fmt.Errorf("--skip-mcp cannot be combined with MCP install flags")
	}

	repoRoot, err := resolveSetupRepoRoot(opts.repoRoot)
	if err != nil {
		return err
	}
	interactive := isTerminalSession()
	lang, err := resolveSetupLanguage(opts.lang, interactive)
	if err != nil {
		return err
	}
	texts := setupMessages[lang]

	installMCP := shouldInstallMCP(opts)
	if !installMCP && !opts.skipMCP && interactive {
		installMCP, err = promptSetupConfirm(texts)
		if err != nil {
			return err
		}
	}
	if !installMCP || opts.skipMCP {
		created, err := ensureEnvFile(repoRoot)
		if err != nil {
			return err
		}
		if created {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), texts.envCreated); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), texts.envExists); err != nil {
				return err
			}
		}
		return printSetupSummary(cmd.OutOrStdout(), texts.noMCPInstalled, texts.nextStep)
	}

	target, err := resolveSetupTarget(opts.mcpTarget, lang, interactive)
	if err != nil {
		return err
	}
	configPath, err := resolveSetupConfigPath(target, opts.configPath, texts, interactive)
	if err != nil {
		return err
	}
	endpoint, err := resolveSetupEndpoint(cmd, texts, interactive)
	if err != nil {
		return err
	}
	systemPath := defaultSetupPath(repoRoot, opts.systemPath, "configs/system.toml")
	indexPath := defaultSetupPath(repoRoot, opts.indexPath, "configs/symbols-index.toml")
	installMode := strings.TrimSpace(opts.mcpMode)
	if installMode == "" && target == "codex" {
		installMode = "stdio"
	}
	installOpts := mcp.InstallOptions{
		Name:       opts.name,
		Command:    opts.command,
		ConfigPath: configPath,
		Target:     target,
		Mode:       installMode,
		Endpoint:   endpoint,
		SystemPath: systemPath,
		IndexPath:  indexPath,
		AuditPath:  opts.auditPath,
	}
	if err := mcp.ValidateInstallOptions(installOpts); err != nil {
		return err
	}

	created, err := ensureEnvFile(repoRoot)
	if err != nil {
		return err
	}
	if created {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), texts.envCreated); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), texts.envExists); err != nil {
			return err
		}
	}

	result, err := mcp.Install(installOpts)
	if err != nil {
		return err
	}
	return printSetupSummary(cmd.OutOrStdout(), fmt.Sprintf(texts.mcpInstalledFmt, result.ConfigPath), texts.nextStep)
}

func resolveSetupRepoRoot(path string) (string, error) {
	root := strings.TrimSpace(path)
	if root == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve repo root: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat repo root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("repo root must be a directory")
	}
	return abs, nil
}

func ensureEnvFile(repoRoot string) (bool, error) {
	examplePath := filepath.Join(repoRoot, ".env.example")
	envPath := filepath.Join(repoRoot, ".env")
	if _, err := os.Stat(envPath); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("stat .env: %w", err)
	}
	raw, err := os.ReadFile(examplePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, fmt.Errorf(".env.example not found under %s", repoRoot)
		}
		return false, fmt.Errorf("read .env.example: %w", err)
	}
	if err := os.WriteFile(envPath, raw, 0o644); err != nil {
		return false, fmt.Errorf("write .env: %w", err)
	}
	return true, nil
}

func shouldInstallMCP(opts setupCommandOptions) bool {
	return opts.installMCP ||
		strings.TrimSpace(opts.mcpTarget) != "" ||
		strings.TrimSpace(opts.mcpMode) != "" ||
		strings.TrimSpace(opts.configPath) != ""
}

func resolveSetupLanguage(raw string, interactive bool) (string, error) {
	lang := strings.ToLower(strings.TrimSpace(raw))
	if lang == "" && interactive {
		return promptSetupLanguage()
	}
	if lang == "" {
		return setupLangEN, nil
	}
	if lang != setupLangZH && lang != setupLangEN {
		return "", fmt.Errorf("unsupported language %q", raw)
	}
	return lang, nil
}

func resolveSetupTarget(raw string, lang string, interactive bool) (string, error) {
	target := strings.ToLower(strings.TrimSpace(raw))
	if target == "" && interactive {
		return promptSetupTarget(lang)
	}
	if target == "" {
		return "claude-code", nil
	}
	for _, item := range mcp.SupportedInstallTargets() {
		if target == item {
			return target, nil
		}
	}
	return "", fmt.Errorf("unsupported MCP install target %q", raw)
}

func resolveSetupConfigPath(target string, raw string, texts setupTexts, interactive bool) (string, error) {
	configPath := strings.TrimSpace(raw)
	if configPath != "" {
		return configPath, nil
	}
	if target != "custom" {
		return "", nil
	}
	if !interactive {
		return "", fmt.Errorf("install target %q requires --config", target)
	}
	value, err := promptSetupInput(texts.promptConfig, "")
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("install target %q requires --config", target)
	}
	return value, nil
}

func resolveSetupEndpoint(cmd *cobra.Command, texts setupTexts, interactive bool) (string, error) {
	endpoint := strings.TrimSpace(flagEndpoint)
	if endpoint == "" {
		endpoint = "http://127.0.0.1:9991"
	}
	flag := cmd.Flag("endpoint")
	if !interactive || (flag != nil && flag.Changed) {
		return endpoint, nil
	}
	return promptSetupInput(texts.promptEndpoint, endpoint)
}

func defaultSetupPath(repoRoot string, current string, fallback string) string {
	path := strings.TrimSpace(current)
	if path == "" {
		path = filepath.Join(repoRoot, filepath.FromSlash(fallback))
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(repoRoot, path)
}

func printSetupSummary(out io.Writer, lines ...string) error {
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	return nil
}

func promptSetupLanguage() (string, error) {
	var lang string
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select language / 选择语言").
				Options(
					huh.NewOption("中文", setupLangZH),
					huh.NewOption("English", setupLangEN),
				).
				Value(&lang),
		),
	).Run(); err != nil {
		return "", fmt.Errorf("prompt language: %w", err)
	}
	return lang, nil
}

func promptSetupConfirm(texts setupTexts) (bool, error) {
	var install bool
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(texts.promptInstall).
				Value(&install).
				Affirmative(texts.yesLabel).
				Negative(texts.noLabel),
		),
	).Run(); err != nil {
		return false, fmt.Errorf("prompt install MCP: %w", err)
	}
	return install, nil
}

func promptSetupTarget(lang string) (string, error) {
	label := setupMessages[lang].promptTarget
	opts := make([]huh.Option[string], 0, len(setupTargets))
	for _, t := range setupTargets {
		display := t.labelEN
		if lang == setupLangZH {
			display = t.labelZH
		}
		opts = append(opts, huh.NewOption(display, t.value))
	}
	var target string
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(label).
				Options(opts...).
				Value(&target),
		),
	).Run(); err != nil {
		return "", fmt.Errorf("prompt MCP target: %w", err)
	}
	return target, nil
}

func promptSetupInput(label string, initial string) (string, error) {
	value := initial
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(label).
				Value(&value).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("value cannot be empty")
					}
					return nil
				}),
		),
	).Run(); err != nil {
		return "", fmt.Errorf("prompt input: %w", err)
	}
	return strings.TrimSpace(value), nil
}

func isTerminalSession() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}
