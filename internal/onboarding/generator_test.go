package onboarding

import (
	"strings"
	"testing"
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
