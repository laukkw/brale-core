package onboarding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"brale-core/internal/config"
)

func TestPreviewNotificationDefaultsDisabled(t *testing.T) {
	g := NewGenerator(t.TempDir())
	result, err := g.Preview(basePreviewRequest())
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	envContent := generatedFileContent(t, result, ".env")
	if !strings.Contains(envContent, "NOTIFICATION_ENABLED=false") {
		t.Fatalf(".env missing notification false default:\n%s", envContent)
	}
	if !strings.Contains(envContent, "NOTIFICATION_STARTUP_NOTIFY_ENABLED=false") {
		t.Fatalf(".env missing startup notify false default:\n%s", envContent)
	}
	if !strings.Contains(envContent, "NOTIFICATION_TELEGRAM_ENABLED=false") {
		t.Fatalf(".env missing telegram false default:\n%s", envContent)
	}
	if !strings.Contains(envContent, "NOTIFICATION_FEISHU_ENABLED=false") {
		t.Fatalf(".env missing feishu false default:\n%s", envContent)
	}
	if !strings.Contains(envContent, "NOTIFICATION_FEISHU_BOT_ENABLED=false") {
		t.Fatalf(".env missing feishu bot false default:\n%s", envContent)
	}

	systemContent := generatedFileContent(t, result, "configs/system.toml")
	if !strings.Contains(systemContent, "[notification]\nenabled = ${NOTIFICATION_ENABLED}\nstartup_notify_enabled = ${NOTIFICATION_STARTUP_NOTIFY_ENABLED}") {
		t.Fatalf("configs/system.toml missing notification placeholders:\n%s", systemContent)
	}
	if !strings.Contains(systemContent, "[notification.telegram]\nenabled = ${NOTIFICATION_TELEGRAM_ENABLED}") {
		t.Fatalf("configs/system.toml missing telegram placeholder:\n%s", systemContent)
	}
	if !strings.Contains(systemContent, "[notification.feishu]\nenabled = ${NOTIFICATION_FEISHU_ENABLED}") {
		t.Fatalf("configs/system.toml missing feishu placeholder:\n%s", systemContent)
	}
	if !strings.Contains(systemContent, "bot_enabled = ${NOTIFICATION_FEISHU_BOT_ENABLED}") {
		t.Fatalf("configs/system.toml missing feishu bot placeholder:\n%s", systemContent)
	}
}

func TestPreviewNotificationEnabledWhenAnyChannelSelected(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Request)
	}{
		{
			name: "telegram",
			edit: func(req *Request) {
				req.TelegramEnabled = true
			},
		},
		{
			name: "feishu",
			edit: func(req *Request) {
				req.FeishuEnabled = true
			},
		},
		{
			name: "feishu_bot",
			edit: func(req *Request) {
				req.FeishuBotEnabled = true
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGenerator(t.TempDir())
			req := basePreviewRequest()
			tt.edit(&req)

			result, err := g.Preview(req)
			if err != nil {
				t.Fatalf("Preview() error = %v", err)
			}

			envContent := generatedFileContent(t, result, ".env")
			if !strings.Contains(envContent, "NOTIFICATION_ENABLED=true") {
				t.Fatalf(".env missing notification enabled true for %s:\n%s", tt.name, envContent)
			}
		})
	}
}

func TestPreviewWritesSecretsToSeparateFiles(t *testing.T) {
	g := NewGenerator(t.TempDir())
	result, err := g.Preview(basePreviewRequest())
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	envContent := generatedFileContent(t, result, ".env")
	if !strings.Contains(envContent, "EXEC_SECRET=secret") {
		t.Fatalf(".env should contain exec secret:\n%s", envContent)
	}
	if !strings.Contains(envContent, "LLM_INDICATOR_API_KEY=indicator-key") {
		t.Fatalf(".env should contain llm indicator key:\n%s", envContent)
	}
}

func TestPreviewStrategyFilesUseLLMRiskModeByDefault(t *testing.T) {
	g := NewGenerator(t.TempDir())
	result, err := g.Preview(basePreviewRequest())
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	defaultStrategy := generatedFileContent(t, result, "configs/strategies/default.toml")
	if !strings.Contains(defaultStrategy, "[risk_management.risk_strategy]\nmode = \"llm\"") {
		t.Fatalf("default strategy missing llm risk mode:\n%s", defaultStrategy)
	}

	symbolStrategy := generatedFileContent(t, result, "configs/strategies/ETHUSDT.toml")
	if !strings.Contains(symbolStrategy, "[risk_management.risk_strategy]\nmode = \"llm\"") {
		t.Fatalf("symbol strategy missing llm risk mode:\n%s", symbolStrategy)
	}
	if !strings.Contains(symbolStrategy, "[risk_management.initial_exit]\npolicy = \"atr_structure_v1\"") {
		t.Fatalf("symbol strategy missing compatibility initial_exit policy:\n%s", symbolStrategy)
	}
}

func TestPreviewGeneratedStrategyFilesRemainLoadable(t *testing.T) {
	g := NewGenerator(t.TempDir())
	result, err := g.Preview(basePreviewRequest())
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	for _, rel := range []string{"configs/strategies/default.toml", "configs/strategies/ETHUSDT.toml"} {
		t.Run(rel, func(t *testing.T) {
			content := generatedFileContent(t, result, rel)
			path := filepath.Join(t.TempDir(), filepath.Base(rel))
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatalf("write %s: %v", rel, err)
			}
			cfg, err := config.LoadStrategyConfig(path)
			if err != nil {
				t.Fatalf("LoadStrategyConfig(%s) error = %v\n%s", rel, err, content)
			}
			if err := config.ValidateStrategyConfig(cfg); err != nil {
				t.Fatalf("ValidateStrategyConfig(%s) error = %v\n%s", rel, err, content)
			}
		})
	}
}

func TestPreviewGeneratedSymbolFilesRemainLoadable(t *testing.T) {
	g := NewGenerator(t.TempDir())
	req := basePreviewRequest()
	setPreviewEnv(t, req)
	result, err := g.Preview(req)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	for _, rel := range []string{"configs/symbols/default.toml", "configs/symbols/ETHUSDT.toml"} {
		t.Run(rel, func(t *testing.T) {
			content := generatedFileContent(t, result, rel)
			path := filepath.Join(t.TempDir(), filepath.Base(rel))
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatalf("write %s: %v", rel, err)
			}
			cfg, err := config.LoadSymbolConfig(path)
			if err != nil {
				t.Fatalf("LoadSymbolConfig(%s) error = %v\n%s", rel, err, content)
			}
			if err := config.ValidateSymbolConfig(cfg); err != nil {
				t.Fatalf("ValidateSymbolConfig(%s) error = %v\n%s", rel, err, content)
			}
		})
	}
}

func TestPreviewSymbolDefaultsIncludeShadowEngineAndMemory(t *testing.T) {
	g := NewGenerator(t.TempDir())
	result, err := g.Preview(basePreviewRequest())
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	symbolContent := generatedFileContent(t, result, "configs/symbols/ETHUSDT.toml")
	for _, want := range []string{
		"engine = \"talib\"",
		"shadow_engine = \"reference\"",
		"stc_fast = 23",
		"stc_slow = 50",
		"bb_period = 20",
		"bb_multiplier = 2.0",
		"chop_period = 14",
		"stoch_rsi_period = 14",
		"aroon_period = 25",
		"[memory]",
		"enabled = true",
		"working_memory_size = 5",
		"episodic_enabled = true",
		"episodic_ttl_days = 90",
		"episodic_max_per_symbol = 3",
		"semantic_enabled = true",
		"semantic_max_rules = 10",
	} {
		if !strings.Contains(symbolContent, want) {
			t.Fatalf("symbol config missing %q:\n%s", want, symbolContent)
		}
	}

	defaultContent := generatedFileContent(t, result, "configs/symbols/default.toml")
	for _, want := range []string{
		"engine = \"talib\"",
		"shadow_engine = \"\"",
		"[memory]",
		"enabled = true",
		"semantic_enabled = true",
	} {
		if !strings.Contains(defaultContent, want) {
			t.Fatalf("default symbol config missing %q:\n%s", want, defaultContent)
		}
	}
}

func TestPreviewSystemTemplateIncludesStructuredOutput(t *testing.T) {
	g := NewGenerator(t.TempDir())
	req := basePreviewRequest()
	setPreviewEnv(t, req)
	result, err := g.Preview(req)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	systemContent := generatedFileContent(t, result, "configs/system.toml")
	if strings.Count(systemContent, "structured_output = true") != 3 {
		t.Fatalf("configs/system.toml missing structured_output flags:\n%s", systemContent)
	}
	if !strings.Contains(systemContent, "llm_min_interval = \"20s\"") {
		t.Fatalf("configs/system.toml missing llm_min_interval sync:\n%s", systemContent)
	}

	path := filepath.Join(t.TempDir(), "system.toml")
	if err := os.WriteFile(path, []byte(systemContent), 0o600); err != nil {
		t.Fatalf("write system config: %v", err)
	}
	cfg, err := config.LoadSystemConfig(path)
	if err != nil {
		t.Fatalf("LoadSystemConfig() error = %v\n%s", err, systemContent)
	}
	if err := config.ValidateSystemConfig(cfg); err != nil {
		t.Fatalf("ValidateSystemConfig() error = %v\n%s", err, systemContent)
	}
}

func TestPreviewTemplateBackedStrategiesUseConservativeSieveFallback(t *testing.T) {
	g := NewGenerator(t.TempDir())
	result, err := g.Preview(basePreviewRequest())
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	for _, rel := range []string{"configs/strategies/default.toml", "configs/strategies/ETHUSDT.toml"} {
		t.Run(rel, func(t *testing.T) {
			content := generatedFileContent(t, result, rel)
			if !strings.Contains(content, "default_gate_action = \"ALLOW\"") {
				t.Fatalf("%s missing default_gate_action:\n%s", rel, content)
			}
			if !strings.Contains(content, "default_size_factor = 1.0") {
				t.Fatalf("%s missing default_size_factor:\n%s", rel, content)
			}
		})
	}
}

func basePreviewRequest() Request {
	return Request{
		Symbols:              []string{"ETHUSDT"},
		ExecUsername:         "user",
		ExecSecret:           "secret",
		LLMModelIndicator:    "indicator-model",
		LLMIndicatorEndpoint: "https://indicator.example.com",
		LLMIndicatorKey:      "indicator-key",
		LLMModelStructure:    "structure-model",
		LLMStructureEndpoint: "https://structure.example.com",
		LLMStructureKey:      "structure-key",
		LLMModelMechanics:    "mechanics-model",
		LLMMechanicsEndpoint: "https://mechanics.example.com",
		LLMMechanicsKey:      "mechanics-key",
	}
}

func generatedFileContent(t *testing.T, result GenerateResult, path string) string {
	t.Helper()
	for _, file := range result.Files {
		if file.Path == path {
			return file.Content
		}
	}
	t.Fatalf("generated file %q not found", path)
	return ""
}

func setPreviewEnv(t *testing.T, req Request) {
	t.Helper()
	t.Setenv("EXEC_USERNAME", req.ExecUsername)
	t.Setenv("EXEC_SECRET", req.ExecSecret)
	t.Setenv("LLM_MODEL_INDICATOR", req.LLMModelIndicator)
	t.Setenv("LLM_INDICATOR_ENDPOINT", req.LLMIndicatorEndpoint)
	t.Setenv("LLM_INDICATOR_API_KEY", req.LLMIndicatorKey)
	t.Setenv("LLM_MODEL_STRUCTURE", req.LLMModelStructure)
	t.Setenv("LLM_STRUCTURE_ENDPOINT", req.LLMStructureEndpoint)
	t.Setenv("LLM_STRUCTURE_API_KEY", req.LLMStructureKey)
	t.Setenv("LLM_MODEL_MECHANICS", req.LLMModelMechanics)
	t.Setenv("LLM_MECHANICS_ENDPOINT", req.LLMMechanicsEndpoint)
	t.Setenv("LLM_MECHANICS_API_KEY", req.LLMMechanicsKey)
	t.Setenv("NOTIFICATION_ENABLED", "false")
	t.Setenv("NOTIFICATION_STARTUP_NOTIFY_ENABLED", "false")
	t.Setenv("NOTIFICATION_TELEGRAM_ENABLED", "false")
	t.Setenv("NOTIFICATION_TELEGRAM_TOKEN", "")
	t.Setenv("NOTIFICATION_TELEGRAM_CHAT_ID", "0")
	t.Setenv("NOTIFICATION_FEISHU_ENABLED", "false")
	t.Setenv("NOTIFICATION_FEISHU_APP_ID", "")
	t.Setenv("NOTIFICATION_FEISHU_APP_SECRET", "")
	t.Setenv("NOTIFICATION_FEISHU_BOT_ENABLED", "false")
	t.Setenv("NOTIFICATION_FEISHU_BOT_MODE", "long_connection")
	t.Setenv("NOTIFICATION_FEISHU_VERIFICATION_TOKEN", "")
	t.Setenv("NOTIFICATION_FEISHU_ENCRYPT_KEY", "")
	t.Setenv("NOTIFICATION_FEISHU_DEFAULT_RECEIVE_ID_TYPE", "chat_id")
	t.Setenv("NOTIFICATION_FEISHU_DEFAULT_RECEIVE_ID", "")
	t.Setenv("DATABASE_DSN", "postgres://brale:brale@localhost:5432/brale?sslmode=disable")
}
