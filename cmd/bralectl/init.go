package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"brale-core/internal/config"
	"brale-core/internal/onboarding"

	huh "charm.land/huh/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// envField maps a .env key to a human-readable prompt label, a default value,
// and whether the field contains a secret (masked input).
type envField struct {
	Key      string
	Label    string
	Default  string
	Secret   bool
	Optional bool
}

// envGroup collects related fields under a section heading.
type envGroup struct {
	Title  string
	Fields []envField
}

type symbolChoice struct {
	Symbol          string
	ConfigPath      string
	ConfigRelPath   string
	StrategyPath    string
	StrategyRelPath string
	Intervals       []string
	KlineLimit      int
}

type initSummary struct {
	Symbols             []symbolChoice
	LLMEndpoint         string
	ProxyEnabled        bool
	TelegramEnabled     bool
	FeishuEnabled       bool
	MCPEnabled          bool
	DatabaseDescription string
}

type initSymbolConfig struct {
	Symbol     string   `mapstructure:"symbol"`
	Intervals  []string `mapstructure:"intervals"`
	KlineLimit int      `mapstructure:"kline_limit"`
}

func initCmd() *cobra.Command {
	var (
		repoRoot       string
		nonInteractive bool
	)
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Interactive setup wizard: configure .env and generate runtime configs",
		Long: `Walks through all required configuration (exchange, LLM, proxy, notifications)
via an interactive CLI wizard. Reads existing .env values as defaults, writes
the updated .env, then runs prepare-stack to generate runtime config files.

Use --non-interactive to skip prompts and validate the existing .env only.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			err := runInit(cmd.OutOrStdout(), cmd.ErrOrStderr(), repoRoot, nonInteractive)
			if errors.Is(err, huh.ErrUserAborted) {
				fmt.Fprintln(cmd.ErrOrStderr(), "\n✗ Wizard cancelled.")
				return nil
			}
			return err
		},
	}
	cmd.Flags().StringVar(&repoRoot, "repo", ".", "Repository root directory")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Skip prompts; validate and generate from existing .env")
	return cmd
}

func runInit(stdout, stderr io.Writer, repoRoot string, nonInteractive bool) error {
	root, err := filepath.Abs(strings.TrimSpace(repoRoot))
	if err != nil {
		return fmt.Errorf("resolve repo root: %w", err)
	}
	if info, statErr := os.Stat(root); statErr != nil || !info.IsDir() {
		return fmt.Errorf("repo root is not a valid directory: %s", root)
	}

	envPath := filepath.Join(root, ".env")
	examplePath := filepath.Join(root, ".env.example")

	// Ensure .env exists (copy from .env.example if missing).
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		raw, readErr := os.ReadFile(examplePath)
		if readErr != nil {
			return fmt.Errorf(".env.example not found: %w", readErr)
		}
		if writeErr := os.WriteFile(envPath, raw, 0o644); writeErr != nil {
			return fmt.Errorf("write .env: %w", writeErr)
		}
		fmt.Fprintln(stdout, "[OK] created .env from .env.example")
	}

	// Parse current .env as defaults.
	current, err := onboarding.ParseEnvFile(envPath)
	if err != nil {
		return fmt.Errorf("parse .env: %w", err)
	}
	availableSymbols, err := discoverSymbolChoices(root)
	if err != nil {
		return err
	}
	selectedSymbols, err := currentSymbolChoices(root, availableSymbols)
	if err != nil {
		return err
	}

	if nonInteractive {
		fmt.Fprintln(stdout, "[INFO] non-interactive mode: validating existing .env")
	} else {
		if !isTerminalSession() {
			return fmt.Errorf("interactive init requires a TTY; rerun in a terminal or use --non-interactive")
		}
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "┌──────────────────────────────────────────────┐")
		fmt.Fprintln(stdout, "│       Brale-Core Configuration Wizard        │")
		fmt.Fprintln(stdout, "│  Configure your .env step by step.           │")
		fmt.Fprintln(stdout, "│  Press Enter to accept [default] values.     │")
		fmt.Fprintln(stdout, "└──────────────────────────────────────────────┘")
		fmt.Fprintln(stdout, "")
		selectedSymbols, err = promptSymbolSelection(stdout, availableSymbols, selectedSymbols)
		if err != nil {
			return err
		}
		for i := range selectedSymbols {
			updated, promptErr := promptSymbolConfigSelection(selectedSymbols[i])
			if promptErr != nil {
				return promptErr
			}
			selectedSymbols[i] = updated
		}
		for _, group := range buildStaticEnvGroups(current) {
			if err := promptEnvGroup(stdout, group, current); err != nil {
				return err
			}
		}
		if err := promptLLMConfiguration(stdout, current); err != nil {
			return err
		}
		if err := promptProxyConfiguration(stdout, current); err != nil {
			return err
		}
		if err := promptTelegramConfiguration(stdout, current); err != nil {
			return err
		}
		if err := promptFeishuConfiguration(stdout, current); err != nil {
			return err
		}
		if err := promptMCPConfiguration(stdout, current); err != nil {
			return err
		}
	}

	// Write updated .env preserving structure.
	// Derive notification flags.
	current["NOTIFICATION_ENABLED"] = initNotificationEnabled(current)
	normalizeInitNotificationDefaults(current)
	current["ENABLE_MCP"] = formatBoolValue(parseBoolValue(current["ENABLE_MCP"]))
	if err := writeUpdatedEnv(envPath, current); err != nil {
		return fmt.Errorf("write .env: %w", err)
	}
	fmt.Fprintln(stdout, "[OK] .env updated")
	if !nonInteractive {
		if err := persistSymbolSelections(selectedSymbols); err != nil {
			return fmt.Errorf("update symbol configs: %w", err)
		}
		indexPath := filepath.Join(root, "configs", "symbols-index.toml")
		if err := writeSymbolIndex(indexPath, selectedSymbols); err != nil {
			return fmt.Errorf("write symbol index: %w", err)
		}
		fmt.Fprintln(stdout, "[OK] symbols-index.toml updated")
	}

	// Run prepare-stack to generate runtime configs.
	fmt.Fprintln(stdout, "[INFO] generating runtime configs (prepare-stack)...")
	prepareArgs := []string{
		"-env-file", ".env",
		"-config-in", "configs/freqtrade/config.base.json",
		"-config-out", "data/freqtrade/user_data/config.json",
		"-proxy-env-out", "data/freqtrade/proxy.env",
		"-system-in", "configs/system.toml",
	}
	if err := onboarding.RunPrepareStack(prepareArgs, root, stdout); err != nil {
		fmt.Fprintf(stderr, "[WARN] prepare-stack: %v\n", err)
		fmt.Fprintln(stderr, "[WARN] you can run 'make prepare' later to generate runtime configs")
	} else {
		fmt.Fprintln(stdout, "[OK] runtime configs generated")
	}

	fmt.Fprintln(stdout, "")
	fmt.Fprintln(stdout, renderInitSummary(buildInitSummary(current, selectedSymbols)))
	fmt.Fprintln(stdout, "[NEXT] run 'make start' to launch the trading stack")
	return nil
}

func buildStaticEnvGroups(current map[string]string) []envGroup {
	d := func(key, fallback string) string {
		if v, ok := current[key]; ok && v != "" {
			return v
		}
		return fallback
	}
	return []envGroup{
		{
			Title: "Exchange / Freqtrade Credentials",
			Fields: []envField{
				{Key: "EXEC_USERNAME", Label: "Freqtrade API username", Default: d("EXEC_USERNAME", "")},
				{Key: "EXEC_SECRET", Label: "Freqtrade API password", Default: d("EXEC_SECRET", ""), Secret: true},
			},
		},
		{
			Title: "Database",
			Fields: []envField{
				{Key: "POSTGRES_USER", Label: "PostgreSQL user", Default: d("POSTGRES_USER", "brale"), Optional: true},
				{Key: "POSTGRES_PASSWORD", Label: "PostgreSQL password", Default: d("POSTGRES_PASSWORD", "brale"), Secret: true, Optional: true},
				{Key: "POSTGRES_DB", Label: "PostgreSQL database", Default: d("POSTGRES_DB", "brale"), Optional: true},
			},
		},
		{
			Title: "Go Proxy (for Docker build)",
			Fields: []envField{
				{Key: "GOPROXY", Label: "GOPROXY", Default: d("GOPROXY", "https://goproxy.cn,direct"), Optional: true},
				{Key: "GOSUMDB", Label: "GOSUMDB", Default: d("GOSUMDB", "sum.golang.google.cn"), Optional: true},
			},
		},
	}
}

func promptEnvGroup(_ io.Writer, group envGroup, current map[string]string) error {
	if len(group.Fields) == 0 {
		return nil
	}
	fields := make([]huh.Field, 0, len(group.Fields))
	values := make(map[string]*string)
	for i := range group.Fields {
		f := &group.Fields[i]
		val := f.Default
		values[f.Key] = &val
		input := huh.NewInput().
			Title(f.Label).
			Value(&val)
		if f.Secret {
			input = input.EchoMode(huh.EchoModePassword)
		}
		if !f.Optional {
			input = input.Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("required")
				}
				return nil
			})
		}
		fields = append(fields, input)
	}
	if err := huh.NewForm(
		huh.NewGroup(fields...).Title(group.Title),
	).Run(); err != nil {
		return fmt.Errorf("prompt %s: %w", group.Title, err)
	}
	for key, val := range values {
		current[key] = *val
	}
	return nil
}

func discoverSymbolChoices(repoRoot string) ([]symbolChoice, error) {
	pattern := filepath.Join(repoRoot, "configs", "symbols", "*.toml")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("discover symbol configs: %w", err)
	}
	slices.Sort(paths)
	choices := make([]symbolChoice, 0, len(paths))
	for _, path := range paths {
		base := filepath.Base(path)
		if base == "default.toml" {
			continue
		}
		symbolCfg, err := loadInitSymbolConfig(path)
		if err != nil {
			return nil, fmt.Errorf("load symbol config %s: %w", path, err)
		}
		strategyPath := filepath.Join(repoRoot, "configs", "strategies", base)
		if _, err := os.Stat(strategyPath); err != nil {
			return nil, fmt.Errorf("stat strategy config %s: %w", strategyPath, err)
		}
		choices = append(choices, symbolChoice{
			Symbol:          strings.ToUpper(strings.TrimSpace(symbolCfg.Symbol)),
			ConfigPath:      path,
			ConfigRelPath:   filepath.ToSlash(filepath.Join("symbols", base)),
			StrategyPath:    strategyPath,
			StrategyRelPath: filepath.ToSlash(filepath.Join("strategies", base)),
			Intervals:       append([]string(nil), symbolCfg.Intervals...),
			KlineLimit:      symbolCfg.KlineLimit,
		})
	}
	if len(choices) == 0 {
		return nil, fmt.Errorf("no symbol configs found under %s", pattern)
	}
	return choices, nil
}

func loadInitSymbolConfig(path string) (initSymbolConfig, error) {
	var cfg initSymbolConfig
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}
	v := viper.NewWithOptions(viper.KeyDelimiter("::"))
	v.SetConfigType("toml")
	if err := v.ReadConfig(bytes.NewReader(raw)); err != nil {
		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := v.Unmarshal(&cfg); err != nil {
		return cfg, fmt.Errorf("unmarshal config %s: %w", path, err)
	}
	cfg.Symbol = strings.ToUpper(strings.TrimSpace(cfg.Symbol))
	if cfg.Symbol == "" {
		return cfg, fmt.Errorf("symbol is required")
	}
	if len(cfg.Intervals) == 0 {
		return cfg, fmt.Errorf("intervals is required")
	}
	if cfg.KlineLimit <= 0 {
		return cfg, fmt.Errorf("kline_limit must be > 0")
	}
	return cfg, nil
}

func currentSymbolChoices(repoRoot string, available []symbolChoice) ([]symbolChoice, error) {
	lookup := map[string]symbolChoice{}
	for _, choice := range available {
		lookup[choice.Symbol] = cloneSymbolChoice(choice)
	}
	indexPath := filepath.Join(repoRoot, "configs", "symbols-index.toml")
	if _, err := os.Stat(indexPath); err != nil {
		if os.IsNotExist(err) {
			if len(available) > 0 {
				return []symbolChoice{cloneSymbolChoice(available[0])}, nil
			}
			return nil, fmt.Errorf("load symbol index %s: %w", indexPath, err)
		}
		return nil, fmt.Errorf("stat symbol index %s: %w", indexPath, err)
	}
	indexCfg, err := config.LoadSymbolIndexConfig(indexPath)
	if err != nil {
		return nil, fmt.Errorf("load symbol index %s: %w", indexPath, err)
	}
	selected := make([]symbolChoice, 0, len(indexCfg.Symbols))
	for _, entry := range indexCfg.Symbols {
		if choice, ok := lookup[strings.ToUpper(strings.TrimSpace(entry.Symbol))]; ok {
			selected = append(selected, cloneSymbolChoice(choice))
		}
	}
	if len(selected) == 0 && len(available) > 0 {
		selected = append(selected, cloneSymbolChoice(available[0]))
	}
	return selected, nil
}

func promptSymbolSelection(_ io.Writer, available []symbolChoice, current []symbolChoice) ([]symbolChoice, error) {
	currentSet := map[string]struct{}{}
	for _, c := range current {
		currentSet[c.Symbol] = struct{}{}
	}
	options := make([]huh.Option[string], 0, len(available))
	for _, choice := range available {
		label := fmt.Sprintf("%s  [%s]  kline_limit=%d", choice.Symbol, strings.Join(choice.Intervals, ", "), choice.KlineLimit)
		opt := huh.NewOption(label, choice.Symbol)
		if _, ok := currentSet[choice.Symbol]; ok {
			opt = opt.Selected(true)
		}
		options = append(options, opt)
	}
	var selected []string
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Trading Symbols").
				Description("Select which symbols to activate (space to toggle, enter to confirm)").
				Options(options...).
				Value(&selected),
		),
	).Run()
	if err != nil {
		return nil, fmt.Errorf("prompt symbol selection: %w", err)
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("at least one active symbol is required")
	}
	return parseSymbolSelection(strings.Join(selected, ","), available)
}

func parseSymbolSelection(raw string, available []symbolChoice) ([]symbolChoice, error) {
	lookup := map[string]symbolChoice{}
	for _, choice := range available {
		lookup[choice.Symbol] = cloneSymbolChoice(choice)
	}
	parts := splitCSV(raw)
	if len(parts) == 0 {
		return nil, fmt.Errorf("at least one active symbol is required")
	}
	selected := make([]symbolChoice, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		symbol := strings.ToUpper(strings.TrimSpace(part))
		choice, ok := lookup[symbol]
		if !ok {
			return nil, fmt.Errorf("unknown symbol %q", symbol)
		}
		if _, ok := seen[symbol]; ok {
			continue
		}
		seen[symbol] = struct{}{}
		selected = append(selected, cloneSymbolChoice(choice))
	}
	return selected, nil
}

// commonIntervals lists typical Binance kline intervals for multi-select.
var commonIntervals = []string{"1m", "3m", "5m", "15m", "30m", "1h", "2h", "4h", "6h", "8h", "12h", "1d", "3d", "1w"}

func promptSymbolConfigSelection(choice symbolChoice) (symbolChoice, error) {
	currentSet := map[string]struct{}{}
	for _, iv := range choice.Intervals {
		currentSet[strings.TrimSpace(iv)] = struct{}{}
	}
	options := make([]huh.Option[string], 0, len(commonIntervals))
	for _, iv := range commonIntervals {
		opt := huh.NewOption(iv, iv)
		if _, ok := currentSet[iv]; ok {
			opt = opt.Selected(true)
		}
		options = append(options, opt)
	}
	var intervals []string
	klineStr := strconv.Itoa(choice.KlineLimit)
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title(fmt.Sprintf("Intervals for %s", choice.Symbol)).
				Description("Select kline intervals (space to toggle, enter to confirm)").
				Options(options...).
				Value(&intervals),
			huh.NewInput().
				Title(fmt.Sprintf("Kline limit for %s", choice.Symbol)).
				Value(&klineStr).
				Validate(func(s string) error {
					v, err := strconv.Atoi(strings.TrimSpace(s))
					if err != nil || v <= 0 {
						return fmt.Errorf("must be a positive integer")
					}
					return nil
				}),
		),
	).Run()
	if err != nil {
		return choice, fmt.Errorf("prompt symbol config: %w", err)
	}
	if len(intervals) == 0 {
		return choice, fmt.Errorf("symbol %s requires at least one interval", choice.Symbol)
	}
	klineLimit := parseIntValue(klineStr, choice.KlineLimit)
	if klineLimit <= 0 {
		return choice, fmt.Errorf("symbol %s requires a positive kline limit", choice.Symbol)
	}
	choice.Intervals = intervals
	choice.KlineLimit = klineLimit
	return choice, nil
}

func promptLLMConfiguration(_ io.Writer, current map[string]string) error {
	sharedDefault := current["LLM_INDICATOR_ENDPOINT"] != "" &&
		current["LLM_INDICATOR_ENDPOINT"] == current["LLM_STRUCTURE_ENDPOINT"] &&
		current["LLM_INDICATOR_ENDPOINT"] == current["LLM_MECHANICS_ENDPOINT"] &&
		current["LLM_INDICATOR_API_KEY"] == current["LLM_STRUCTURE_API_KEY"] &&
		current["LLM_INDICATOR_API_KEY"] == current["LLM_MECHANICS_API_KEY"]
	var shared bool
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Use the same endpoint and API key for all LLM agents?").
				Value(&shared).
				Affirmative("Yes").
				Negative("No"),
		).Title("LLM Configuration"),
	).Run(); err != nil {
		return fmt.Errorf("prompt LLM shared: %w", err)
	}
	if !sharedDefault && !shared {
		// User chose not to share, keep default
	} else if sharedDefault && !shared {
		// User had shared but now wants separate
	}
	if shared {
		endpoint := current["LLM_INDICATOR_ENDPOINT"]
		apiKey := current["LLM_INDICATOR_API_KEY"]
		if err := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Shared LLM API endpoint").
					Value(&endpoint),
				huh.NewInput().
					Title("Shared LLM API key").
					Value(&apiKey).
					EchoMode(huh.EchoModePassword),
			).Title("Shared LLM Credentials"),
		).Run(); err != nil {
			return fmt.Errorf("prompt shared LLM: %w", err)
		}
		for _, key := range []string{"LLM_INDICATOR_ENDPOINT", "LLM_STRUCTURE_ENDPOINT", "LLM_MECHANICS_ENDPOINT"} {
			current[key] = endpoint
		}
		for _, key := range []string{"LLM_INDICATOR_API_KEY", "LLM_STRUCTURE_API_KEY", "LLM_MECHANICS_API_KEY"} {
			current[key] = apiKey
		}
	}
	for _, item := range []struct {
		Title       string
		ModelKey    string
		EndpointKey string
		APIKey      string
	}{
		{Title: "Indicator Agent", ModelKey: "LLM_MODEL_INDICATOR", EndpointKey: "LLM_INDICATOR_ENDPOINT", APIKey: "LLM_INDICATOR_API_KEY"},
		{Title: "Structure Agent", ModelKey: "LLM_MODEL_STRUCTURE", EndpointKey: "LLM_STRUCTURE_ENDPOINT", APIKey: "LLM_STRUCTURE_API_KEY"},
		{Title: "Mechanics Agent", ModelKey: "LLM_MODEL_MECHANICS", EndpointKey: "LLM_MECHANICS_ENDPOINT", APIKey: "LLM_MECHANICS_API_KEY"},
	} {
		model := current[item.ModelKey]
		fields := []huh.Field{
			huh.NewInput().
				Title(fmt.Sprintf("Model name (%s)", strings.ToLower(item.Title))).
				Value(&model),
		}
		endpoint := current[item.EndpointKey]
		apiKey := current[item.APIKey]
		if !shared {
			fields = append(fields,
				huh.NewInput().
					Title(fmt.Sprintf("API endpoint (%s)", strings.ToLower(item.Title))).
					Value(&endpoint),
				huh.NewInput().
					Title(fmt.Sprintf("API key (%s)", strings.ToLower(item.Title))).
					Value(&apiKey).
					EchoMode(huh.EchoModePassword),
			)
		}
		if err := huh.NewForm(
			huh.NewGroup(fields...).Title(item.Title),
		).Run(); err != nil {
			return fmt.Errorf("prompt LLM %s: %w", item.Title, err)
		}
		current[item.ModelKey] = model
		if !shared {
			current[item.EndpointKey] = endpoint
			current[item.APIKey] = apiKey
		}
	}
	return nil
}

func promptProxyConfiguration(_ io.Writer, current map[string]string) error {
	enabled := parseBoolValue(current["PROXY_ENABLED"])
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Enable network proxy?").
				Value(&enabled).
				Affirmative("Yes").
				Negative("No"),
		).Title("Proxy"),
	).Run(); err != nil {
		return fmt.Errorf("prompt proxy: %w", err)
	}
	current["PROXY_ENABLED"] = formatBoolValue(enabled)
	if enabled {
		proxyHost := defaultValue(current, "PROXY_HOST", "host.docker.internal")
		proxyPort := defaultValue(current, "PROXY_PORT", "7890")
		proxyScheme := normalizeInitProxyScheme(current["PROXY_SCHEME"])
		proxyNoProxy := defaultValue(current, "PROXY_NO_PROXY", "localhost,127.0.0.1,brale,freqtrade")
		if err := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Proxy host").
					Value(&proxyHost),
				huh.NewInput().
					Title("Proxy port").
					Value(&proxyPort),
				huh.NewInput().
					Title("No proxy hosts (comma-separated)").
					Value(&proxyNoProxy),
			).Title("Proxy Settings"),
		).Run(); err != nil {
			return fmt.Errorf("prompt proxy settings: %w", err)
		}
		current["PROXY_HOST"] = proxyHost
		current["PROXY_PORT"] = proxyPort
		current["PROXY_SCHEME"] = proxyScheme
		current["PROXY_NO_PROXY"] = proxyNoProxy
	}
	return nil
}

func normalizeInitProxyScheme(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "https":
		return "https"
	case "socks5":
		return "socks5"
	default:
		return "http"
	}
}

func promptTelegramConfiguration(_ io.Writer, current map[string]string) error {
	enabled := parseBoolValue(current["NOTIFICATION_TELEGRAM_ENABLED"])
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Enable Telegram notifications?").
				Value(&enabled).
				Affirmative("Yes").
				Negative("No"),
		).Title("Notifications — Telegram"),
	).Run(); err != nil {
		return fmt.Errorf("prompt telegram: %w", err)
	}
	current["NOTIFICATION_TELEGRAM_ENABLED"] = formatBoolValue(enabled)
	if enabled {
		token := current["NOTIFICATION_TELEGRAM_TOKEN"]
		chatID := current["NOTIFICATION_TELEGRAM_CHAT_ID"]
		if err := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Telegram bot token").
					Value(&token).
					EchoMode(huh.EchoModePassword),
				huh.NewInput().
					Title("Telegram chat ID").
					Value(&chatID),
			).Title("Telegram Settings"),
		).Run(); err != nil {
			return fmt.Errorf("prompt telegram settings: %w", err)
		}
		current["NOTIFICATION_TELEGRAM_TOKEN"] = token
		current["NOTIFICATION_TELEGRAM_CHAT_ID"] = chatID
	}
	return nil
}

func promptFeishuConfiguration(_ io.Writer, current map[string]string) error {
	enabled := parseBoolValue(current["NOTIFICATION_FEISHU_ENABLED"])
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Enable Feishu notifications?").
				Value(&enabled).
				Affirmative("Yes").
				Negative("No"),
		).Title("Notifications — Feishu"),
	).Run(); err != nil {
		return fmt.Errorf("prompt feishu: %w", err)
	}
	current["NOTIFICATION_FEISHU_ENABLED"] = formatBoolValue(enabled)
	if enabled {
		appID := current["NOTIFICATION_FEISHU_APP_ID"]
		appSecret := current["NOTIFICATION_FEISHU_APP_SECRET"]
		if err := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Feishu app ID").
					Value(&appID),
				huh.NewInput().
					Title("Feishu app secret").
					Value(&appSecret).
					EchoMode(huh.EchoModePassword),
			).Title("Feishu Credentials"),
		).Run(); err != nil {
			return fmt.Errorf("prompt feishu credentials: %w", err)
		}
		current["NOTIFICATION_FEISHU_APP_ID"] = appID
		current["NOTIFICATION_FEISHU_APP_SECRET"] = appSecret

		botEnabled := parseBoolValue(current["NOTIFICATION_FEISHU_BOT_ENABLED"])
		if err := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("Enable Feishu bot delivery?").
					Value(&botEnabled).
					Affirmative("Yes").
					Negative("No"),
			),
		).Run(); err != nil {
			return fmt.Errorf("prompt feishu bot: %w", err)
		}
		current["NOTIFICATION_FEISHU_BOT_ENABLED"] = formatBoolValue(botEnabled)
		if botEnabled {
			botMode := defaultValue(current, "NOTIFICATION_FEISHU_BOT_MODE", "long_connection")
			receiveIDType := defaultValue(current, "NOTIFICATION_FEISHU_DEFAULT_RECEIVE_ID_TYPE", "chat_id")
			receiveID := current["NOTIFICATION_FEISHU_DEFAULT_RECEIVE_ID"]
			if err := huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("Feishu bot mode").
						Options(
							huh.NewOption("Long Connection", "long_connection"),
							huh.NewOption("Webhook", "webhook"),
						).
						Value(&botMode),
					huh.NewSelect[string]().
						Title("Feishu receive ID type").
						Options(
							huh.NewOption("Chat ID", "chat_id"),
							huh.NewOption("Open ID", "open_id"),
							huh.NewOption("User ID", "user_id"),
							huh.NewOption("Union ID", "union_id"),
							huh.NewOption("Email", "email"),
						).
						Value(&receiveIDType),
					huh.NewInput().
						Title("Feishu receive ID").
						Value(&receiveID),
				).Title("Feishu Bot Settings"),
			).Run(); err != nil {
				return fmt.Errorf("prompt feishu bot settings: %w", err)
			}
			current["NOTIFICATION_FEISHU_BOT_MODE"] = botMode
			current["NOTIFICATION_FEISHU_DEFAULT_RECEIVE_ID_TYPE"] = receiveIDType
			current["NOTIFICATION_FEISHU_DEFAULT_RECEIVE_ID"] = receiveID
		}
	}
	return nil
}

func promptMCPConfiguration(stdout io.Writer, current map[string]string) error {
	enabled := parseBoolValue(current["ENABLE_MCP"])
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Enable Docker-backed MCP service for editor integration?").
				Description("Exposes read-only MCP tools at http://127.0.0.1:8765/mcp after startup").
				Value(&enabled).
				Affirmative("Yes").
				Negative("No"),
		).Title("MCP Service"),
	).Run(); err != nil {
		return fmt.Errorf("prompt MCP: %w", err)
	}
	current["ENABLE_MCP"] = formatBoolValue(enabled)
	if enabled {
		fmt.Fprintln(stdout, "  ✓ MCP will be exposed at http://127.0.0.1:8765/mcp after startup.")
	}
	return nil
}

func promptPlainText(label, initialValue string, secret bool) (string, error) {
	value := initialValue
	input := huh.NewInput().
		Title(label).
		Value(&value)
	if secret {
		input = input.EchoMode(huh.EchoModePassword)
	}
	if err := huh.NewForm(huh.NewGroup(input)).Run(); err != nil {
		return "", fmt.Errorf("prompt %s: %w", label, err)
	}
	return strings.TrimSpace(value), nil
}

func persistSymbolSelections(choices []symbolChoice) error {
	for _, choice := range choices {
		if err := updateSymbolConfigSelection(choice.ConfigPath, choice.Intervals, choice.KlineLimit); err != nil {
			return err
		}
	}
	return nil
}

func updateSymbolConfigSelection(path string, intervals []string, klineLimit int) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(raw), "\n")
	foundIntervals := false
	foundKline := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "intervals"):
			lines[i] = preserveIndent(line) + "intervals = " + renderTOMLStringArray(intervals)
			foundIntervals = true
		case strings.HasPrefix(trimmed, "kline_limit"):
			lines[i] = preserveIndent(line) + "kline_limit = " + strconv.Itoa(klineLimit)
			foundKline = true
		}
	}
	if !foundIntervals || !foundKline {
		return fmt.Errorf("symbol config %s is missing intervals or kline_limit", path)
	}
	output := strings.Join(lines, "\n")
	if !strings.HasSuffix(output, "\n") {
		output += "\n"
	}
	return os.WriteFile(path, []byte(output), 0o644)
}

func writeSymbolIndex(indexPath string, choices []symbolChoice) error {
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		return err
	}
	var builder strings.Builder
	builder.WriteString("# brale-core 币种清单（由 bralectl init / bralectl add-symbol 维护）\n")
	builder.WriteString("# - symbol: 币种名（必须大写）\n")
	builder.WriteString("# - config: 币种配置文件路径\n")
	builder.WriteString("# - strategy: 币种策略文件路径\n\n")
	for _, choice := range choices {
		builder.WriteString("[[symbols]]\n")
		builder.WriteString(fmt.Sprintf("symbol = %q\n", choice.Symbol))
		builder.WriteString(fmt.Sprintf("config = %q\n", choice.ConfigRelPath))
		builder.WriteString(fmt.Sprintf("strategy = %q\n\n", choice.StrategyRelPath))
	}
	return os.WriteFile(indexPath, []byte(builder.String()), 0o644)
}

func buildInitSummary(current map[string]string, symbols []symbolChoice) initSummary {
	return initSummary{
		Symbols:             cloneSymbolChoices(symbols),
		LLMEndpoint:         firstNonEmpty(current["LLM_INDICATOR_ENDPOINT"], current["LLM_STRUCTURE_ENDPOINT"], current["LLM_MECHANICS_ENDPOINT"]),
		ProxyEnabled:        parseBoolValue(current["PROXY_ENABLED"]),
		TelegramEnabled:     parseBoolValue(current["NOTIFICATION_TELEGRAM_ENABLED"]),
		FeishuEnabled:       parseBoolValue(current["NOTIFICATION_FEISHU_ENABLED"]) || parseBoolValue(current["NOTIFICATION_FEISHU_BOT_ENABLED"]),
		MCPEnabled:          parseBoolValue(current["ENABLE_MCP"]),
		DatabaseDescription: fmt.Sprintf("%s / %s", defaultValue(current, "POSTGRES_USER", "brale"), defaultValue(current, "POSTGRES_DB", "brale")),
	}
}

func renderInitSummary(summary initSummary) string {
	return strings.Join([]string{
		"[SUMMARY]",
		fmt.Sprintf("- Symbols: %s", formatSymbolSummary(summary.Symbols)),
		fmt.Sprintf("- LLM endpoint: %s", defaultIfBlank(summary.LLMEndpoint, "not set")),
		fmt.Sprintf("- Proxy: %s", onOff(summary.ProxyEnabled)),
		fmt.Sprintf("- Notifications: Telegram %s / Feishu %s", onOff(summary.TelegramEnabled), onOff(summary.FeishuEnabled)),
		fmt.Sprintf("- MCP: %s", mcpSummary(summary.MCPEnabled)),
		fmt.Sprintf("- Database: %s", summary.DatabaseDescription),
	}, "\n")
}

func formatSymbolSummary(symbols []symbolChoice) string {
	parts := make([]string, 0, len(symbols))
	for _, choice := range symbols {
		parts = append(parts, fmt.Sprintf("%s [%s]", choice.Symbol, strings.Join(choice.Intervals, ",")))
	}
	return strings.Join(parts, "; ")
}

func mcpSummary(enabled bool) string {
	if enabled {
		return "enabled (127.0.0.1:8765/mcp)"
	}
	return "disabled"
}

func onOff(enabled bool) string {
	if enabled {
		return "on"
	}
	return "off"
}

func joinSymbolNames(symbols []symbolChoice) string {
	names := make([]string, 0, len(symbols))
	for _, choice := range symbols {
		names = append(names, choice.Symbol)
	}
	return strings.Join(names, ",")
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func renderTOMLStringArray(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, fmt.Sprintf("%q", strings.TrimSpace(value)))
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func preserveIndent(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	return line[:len(line)-len(trimmed)]
}

func cloneSymbolChoices(values []symbolChoice) []symbolChoice {
	cloned := make([]symbolChoice, 0, len(values))
	for _, value := range values {
		cloned = append(cloned, cloneSymbolChoice(value))
	}
	return cloned
}

func cloneSymbolChoice(value symbolChoice) symbolChoice {
	value.Intervals = append([]string(nil), value.Intervals...)
	return value
}

func defaultValue(values map[string]string, key, fallback string) string {
	if v := strings.TrimSpace(values[key]); v != "" {
		return v
	}
	return fallback
}

func defaultIfBlank(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func formatBoolValue(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

// writeUpdatedEnv reads the existing .env, updates values from the map,
// and writes back preserving comments and structure.
func writeUpdatedEnv(envPath string, values map[string]string) error {
	raw, err := os.ReadFile(envPath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(raw), "\n")
	seen := map[string]struct{}{}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		work := trimmed
		prefix := ""
		if strings.HasPrefix(work, "export ") {
			prefix = "export "
			work = strings.TrimSpace(strings.TrimPrefix(work, "export "))
		}

		eq := strings.Index(work, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(work[:eq])
		if key == "" {
			continue
		}
		seen[key] = struct{}{}

		if newVal, ok := values[key]; ok {
			// Preserve leading whitespace from original line.
			indent := ""
			if idx := strings.Index(line, trimmed); idx > 0 {
				indent = line[:idx]
			}
			lines[i] = indent + prefix + key + " = " + newVal
		}
	}

	// Append any keys that weren't in the original file.
	var missing []string
	for key, val := range values {
		if _, ok := seen[key]; ok {
			continue
		}
		if val == "" {
			continue
		}
		missing = append(missing, key+" = "+val)
	}
	if len(missing) > 0 {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, missing...)
	}

	out := strings.Join(lines, "\n")
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return os.WriteFile(envPath, []byte(out), 0o644)
}

// notificationEnabled derives the NOTIFICATION_ENABLED flag.
func initNotificationEnabled(values map[string]string) string {
	for _, key := range []string{
		"NOTIFICATION_TELEGRAM_ENABLED",
		"NOTIFICATION_FEISHU_ENABLED",
		"NOTIFICATION_FEISHU_BOT_ENABLED",
	} {
		if parseBoolValue(values[key]) {
			return "true"
		}
	}
	return "false"
}

func normalizeInitNotificationDefaults(values map[string]string) {
	for _, key := range []string{
		"NOTIFICATION_ENABLED",
		"NOTIFICATION_STARTUP_NOTIFY_ENABLED",
		"NOTIFICATION_TELEGRAM_ENABLED",
		"NOTIFICATION_FEISHU_ENABLED",
		"NOTIFICATION_FEISHU_BOT_ENABLED",
	} {
		values[key] = formatBoolValue(parseBoolValue(values[key]))
	}
	if strings.TrimSpace(values["NOTIFICATION_TELEGRAM_CHAT_ID"]) == "" {
		values["NOTIFICATION_TELEGRAM_CHAT_ID"] = "0"
	}
	if strings.TrimSpace(values["NOTIFICATION_FEISHU_BOT_MODE"]) == "" {
		values["NOTIFICATION_FEISHU_BOT_MODE"] = "long_connection"
	}
	if strings.TrimSpace(values["NOTIFICATION_FEISHU_DEFAULT_RECEIVE_ID_TYPE"]) == "" {
		values["NOTIFICATION_FEISHU_DEFAULT_RECEIVE_ID_TYPE"] = "chat_id"
	}
}

func parseBoolValue(raw string) bool {
	v := strings.ToLower(strings.TrimSpace(raw))
	return v == "true" || v == "1" || v == "yes" || v == "on"
}

func parseIntValue(raw string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return v
}

// Unused but kept for potential future use in interactive mode.
var _ = parseIntValue
