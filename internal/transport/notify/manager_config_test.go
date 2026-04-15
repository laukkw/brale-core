package notify

import (
	"testing"

	"brale-core/internal/config"
	"brale-core/internal/decision/decisionfmt"
)

func TestFromConfigFeishuMapping(t *testing.T) {
	cfg := config.NotificationConfig{
		Enabled: true,
		Telegram: config.TelegramConfig{
			Enabled: true,
			Token:   "tg-token",
			ChatID:  100,
		},
		Feishu: config.FeishuConfig{
			Enabled:              true,
			BotEnabled:           true,
			AppID:                "cli_test",
			AppSecret:            "secret",
			VerificationToken:    "verify",
			EncryptKey:           "encrypt",
			DefaultReceiveIDType: "chat_id",
			DefaultReceiveID:     "oc_1",
		},
	}

	out := FromConfig(cfg)
	if !out.Feishu.Enabled || out.Feishu.AppID != "cli_test" {
		t.Fatalf("feishu config mapping mismatch: %+v", out.Feishu)
	}
}

func TestNewManager_FeishuOnly(t *testing.T) {
	notifier, err := NewManager(NotificationConfig{
		Enabled: true,
		Feishu: FeishuConfig{
			Enabled:              true,
			AppID:                "cli_test",
			AppSecret:            "secret",
			DefaultReceiveIDType: "chat_id",
			DefaultReceiveID:     "oc_1",
		},
	}, decisionfmt.New())
	if err == nil {
		if notifier == nil {
			t.Fatalf("notifier should not be nil")
		}
		return
	}
	t.Fatalf("unexpected error: %v", err)
}

func TestNewManager_FeishuBotOnly(t *testing.T) {
	notifier, err := NewManager(NotificationConfig{
		Enabled: true,
		Feishu: FeishuConfig{
			BotEnabled: true,
			AppID:      "cli_test",
			AppSecret:  "secret",
			BotMode:    "long_connection",
		},
	}, decisionfmt.New())
	if err == nil {
		t.Fatalf("expected error")
	}
	if notifier != nil {
		t.Fatal("notifier should be nil on invalid config")
	}
}

func TestNewManager_NoChannelConfigured(t *testing.T) {
	_, err := NewManager(NotificationConfig{Enabled: true}, decisionfmt.New())
	if err == nil {
		t.Fatalf("expected error")
	}
	if err.Error() != "notification enabled but no outbound sender configured" {
		t.Fatalf("unexpected error: %v", err)
	}
}
