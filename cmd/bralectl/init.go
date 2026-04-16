package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"brale-core/internal/onboarding"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
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
			return runInit(cmd.OutOrStdout(), cmd.ErrOrStderr(), repoRoot, nonInteractive)
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

	groups := buildEnvGroups(current)

	if nonInteractive {
		fmt.Fprintln(stdout, "[INFO] non-interactive mode: validating existing .env")
	} else {
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "┌──────────────────────────────────────────────┐")
		fmt.Fprintln(stdout, "│       Brale-Core Configuration Wizard        │")
		fmt.Fprintln(stdout, "│  Configure your .env step by step.           │")
		fmt.Fprintln(stdout, "│  Press Enter to accept [default] values.     │")
		fmt.Fprintln(stdout, "└──────────────────────────────────────────────┘")
		fmt.Fprintln(stdout, "")

		for i, group := range groups {
			fmt.Fprintf(stdout, "── %s ──\n", group.Title)
			for j, field := range group.Fields {
				val, promptErr := promptEnvField(field)
				if promptErr != nil {
					return fmt.Errorf("prompt %s: %w", field.Key, promptErr)
				}
				groups[i].Fields[j].Default = val
				current[field.Key] = val
			}
			fmt.Fprintln(stdout, "")
		}
	}

	// Write updated .env preserving structure.
	// Derive notification flags.
	current["NOTIFICATION_ENABLED"] = initNotificationEnabled(current)
	if _, ok := current["NOTIFICATION_STARTUP_NOTIFY_ENABLED"]; !ok {
		current["NOTIFICATION_STARTUP_NOTIFY_ENABLED"] = "false"
	}
	if err := writeUpdatedEnv(envPath, current); err != nil {
		return fmt.Errorf("write .env: %w", err)
	}
	fmt.Fprintln(stdout, "[OK] .env updated")

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
	fmt.Fprintln(stdout, "[NEXT] run 'make start' to launch the trading stack")
	return nil
}

// buildEnvGroups returns the prompt groups with defaults from the current .env.
func buildEnvGroups(current map[string]string) []envGroup {
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
			Title: "LLM Indicator Agent",
			Fields: []envField{
				{Key: "LLM_MODEL_INDICATOR", Label: "Model name (indicator)", Default: d("LLM_MODEL_INDICATOR", "")},
				{Key: "LLM_INDICATOR_ENDPOINT", Label: "API endpoint (indicator)", Default: d("LLM_INDICATOR_ENDPOINT", "")},
				{Key: "LLM_INDICATOR_API_KEY", Label: "API key (indicator)", Default: d("LLM_INDICATOR_API_KEY", ""), Secret: true},
			},
		},
		{
			Title: "LLM Structure Agent",
			Fields: []envField{
				{Key: "LLM_MODEL_STRUCTURE", Label: "Model name (structure)", Default: d("LLM_MODEL_STRUCTURE", "")},
				{Key: "LLM_STRUCTURE_ENDPOINT", Label: "API endpoint (structure)", Default: d("LLM_STRUCTURE_ENDPOINT", "")},
				{Key: "LLM_STRUCTURE_API_KEY", Label: "API key (structure)", Default: d("LLM_STRUCTURE_API_KEY", ""), Secret: true},
			},
		},
		{
			Title: "LLM Mechanics Agent",
			Fields: []envField{
				{Key: "LLM_MODEL_MECHANICS", Label: "Model name (mechanics)", Default: d("LLM_MODEL_MECHANICS", "")},
				{Key: "LLM_MECHANICS_ENDPOINT", Label: "API endpoint (mechanics)", Default: d("LLM_MECHANICS_ENDPOINT", "")},
				{Key: "LLM_MECHANICS_API_KEY", Label: "API key (mechanics)", Default: d("LLM_MECHANICS_API_KEY", ""), Secret: true},
			},
		},
		{
			Title: "Proxy (optional, press Enter to skip)",
			Fields: []envField{
				{Key: "PROXY_ENABLED", Label: "Enable proxy? (true/false)", Default: d("PROXY_ENABLED", "false"), Optional: true},
				{Key: "PROXY_HOST", Label: "Proxy host", Default: d("PROXY_HOST", "host.docker.internal"), Optional: true},
				{Key: "PROXY_PORT", Label: "Proxy port", Default: d("PROXY_PORT", "7890"), Optional: true},
				{Key: "PROXY_SCHEME", Label: "Proxy scheme (http/https/socks5)", Default: d("PROXY_SCHEME", "http"), Optional: true},
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
			Title: "Notifications — Telegram (optional)",
			Fields: []envField{
				{Key: "NOTIFICATION_TELEGRAM_ENABLED", Label: "Enable Telegram? (true/false)", Default: d("NOTIFICATION_TELEGRAM_ENABLED", "false"), Optional: true},
				{Key: "NOTIFICATION_TELEGRAM_TOKEN", Label: "Telegram bot token", Default: d("NOTIFICATION_TELEGRAM_TOKEN", ""), Optional: true, Secret: true},
				{Key: "NOTIFICATION_TELEGRAM_CHAT_ID", Label: "Telegram chat ID", Default: d("NOTIFICATION_TELEGRAM_CHAT_ID", ""), Optional: true},
			},
		},
		{
			Title: "Notifications — Feishu (optional)",
			Fields: []envField{
				{Key: "NOTIFICATION_FEISHU_ENABLED", Label: "Enable Feishu? (true/false)", Default: d("NOTIFICATION_FEISHU_ENABLED", "false"), Optional: true},
				{Key: "NOTIFICATION_FEISHU_APP_ID", Label: "Feishu app ID", Default: d("NOTIFICATION_FEISHU_APP_ID", ""), Optional: true},
				{Key: "NOTIFICATION_FEISHU_APP_SECRET", Label: "Feishu app secret", Default: d("NOTIFICATION_FEISHU_APP_SECRET", ""), Optional: true, Secret: true},
				{Key: "NOTIFICATION_FEISHU_BOT_ENABLED", Label: "Enable Feishu bot? (true/false)", Default: d("NOTIFICATION_FEISHU_BOT_ENABLED", "false"), Optional: true},
				{Key: "NOTIFICATION_FEISHU_BOT_MODE", Label: "Feishu bot mode", Default: d("NOTIFICATION_FEISHU_BOT_MODE", "long_connection"), Optional: true},
				{Key: "NOTIFICATION_FEISHU_DEFAULT_RECEIVE_ID_TYPE", Label: "Feishu receive ID type", Default: d("NOTIFICATION_FEISHU_DEFAULT_RECEIVE_ID_TYPE", "chat_id"), Optional: true},
				{Key: "NOTIFICATION_FEISHU_DEFAULT_RECEIVE_ID", Label: "Feishu receive ID", Default: d("NOTIFICATION_FEISHU_DEFAULT_RECEIVE_ID", ""), Optional: true},
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

func promptEnvField(field envField) (string, error) {
	label := field.Label
	if field.Default != "" {
		label = fmt.Sprintf("%s [%s]", field.Label, maskDefault(field.Default, field.Secret))
	}

	if field.Secret {
		prompt := promptui.Prompt{
			Label: label,
			Mask:  '*',
			Validate: func(input string) error {
				if !field.Optional && input == "" && field.Default == "" {
					return fmt.Errorf("required")
				}
				return nil
			},
			AllowEdit: false,
		}
		result, err := prompt.Run()
		if err != nil {
			return "", err
		}
		if result == "" {
			return field.Default, nil
		}
		return strings.TrimSpace(result), nil
	}

	prompt := promptui.Prompt{
		Label:   label,
		Default: "",
		Validate: func(input string) error {
			if !field.Optional && input == "" && field.Default == "" {
				return fmt.Errorf("required")
			}
			return nil
		},
	}
	result, err := prompt.Run()
	if err != nil {
		return "", err
	}
	if result == "" {
		return field.Default, nil
	}
	return strings.TrimSpace(result), nil
}

// maskDefault shows the first 4 and last 2 chars of a secret, replacing the
// middle with asterisks. Short values are fully masked.
func maskDefault(val string, secret bool) string {
	if !secret || val == "" {
		return val
	}
	if len(val) <= 8 {
		return strings.Repeat("*", len(val))
	}
	return val[:4] + strings.Repeat("*", len(val)-6) + val[len(val)-2:]
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
