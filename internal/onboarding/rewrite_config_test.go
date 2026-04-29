package onboarding

import (
	"strings"
	"testing"
)

func TestRewriteSymbolConfigReplacesOnlySymbolField(t *testing.T) {
	base := strings.Join([]string{
		`# symbol comment`,
		`symbol = "ETHUSDT"`,
		`intervals = ["15m", "1h"]`,
		``,
		`[agent]`,
		`indicator = true`,
	}, "\n")

	got, err := RewriteSymbolConfig(base, "XAGUSDT")
	if err != nil {
		t.Fatalf("RewriteSymbolConfig() error = %v", err)
	}
	if !strings.Contains(got, `symbol = "XAGUSDT"`) {
		t.Fatalf("rewritten symbol config missing target symbol:\n%s", got)
	}
	if strings.Contains(got, `symbol = "ETHUSDT"`) {
		t.Fatalf("rewritten symbol config still contains template symbol:\n%s", got)
	}
	if !strings.Contains(got, `intervals = ["15m", "1h"]`) {
		t.Fatalf("rewritten symbol config should preserve unrelated fields:\n%s", got)
	}
	if !strings.Contains(got, `# symbol comment`) {
		t.Fatalf("rewritten symbol config should preserve comments:\n%s", got)
	}
}

func TestRewriteSymbolConfigRewritesHeaderCommentAndSymbol(t *testing.T) {
	base := strings.Join([]string{
		`# SOLUSDT 币种配置 / SOLUSDT Symbol Configuration`,
		`# 每个活跃交易品种需要独立的配置文件。`,
		`symbol = "SOLUSDT"`,
		`intervals = ["30m", "1h"]`,
	}, "\n")

	got, err := RewriteSymbolConfig(base, "XAGUSDT")
	if err != nil {
		t.Fatalf("RewriteSymbolConfig() error = %v", err)
	}
	if !strings.Contains(got, `# XAGUSDT 币种配置 / XAGUSDT Symbol Configuration`) {
		t.Fatalf("rewritten symbol config missing target header:\n%s", got)
	}
	if strings.Contains(got, `# SOLUSDT 币种配置 / SOLUSDT Symbol Configuration`) {
		t.Fatalf("rewritten symbol config still contains template header:\n%s", got)
	}
	if !strings.Contains(got, `symbol = "XAGUSDT"`) {
		t.Fatalf("rewritten symbol config missing target symbol:\n%s", got)
	}
	if !strings.Contains(got, `# 每个活跃交易品种需要独立的配置文件。`) {
		t.Fatalf("rewritten symbol config should preserve unrelated comments:\n%s", got)
	}
}

func TestRewriteSymbolConfigPreservesNonHeaderComment(t *testing.T) {
	base := strings.Join([]string{
		`# 自定义模板说明`,
		`symbol = "SOLUSDT"`,
	}, "\n")

	got, err := RewriteSymbolConfig(base, "XAGUSDT")
	if err != nil {
		t.Fatalf("RewriteSymbolConfig() error = %v", err)
	}
	if !strings.Contains(got, `# 自定义模板说明`) {
		t.Fatalf("rewritten symbol config should preserve custom header:\n%s", got)
	}
}

func TestRewriteStrategyConfigRewritesSymbolAndDefaultID(t *testing.T) {
	base := strings.Join([]string{
		`symbol = "ETHUSDT"`,
		`id = "eth-breakout-1"`,
		`rule_chain = "configs/rules/default.json"`,
		``,
		`[risk_management]`,
		`entry_mode = "orderbook"`,
	}, "\n")

	got, err := RewriteStrategyConfig(base, "XAGUSDT")
	if err != nil {
		t.Fatalf("RewriteStrategyConfig() error = %v", err)
	}
	if !strings.Contains(got, `symbol = "XAGUSDT"`) {
		t.Fatalf("rewritten strategy config missing target symbol:\n%s", got)
	}
	if !strings.Contains(got, `id = "default-xagusdt"`) {
		t.Fatalf("rewritten strategy config missing default target id:\n%s", got)
	}
	if strings.Contains(got, `id = "eth-breakout-1"`) {
		t.Fatalf("rewritten strategy config should replace template id:\n%s", got)
	}
	if !strings.Contains(got, `entry_mode = "orderbook"`) {
		t.Fatalf("rewritten strategy config should preserve unrelated fields:\n%s", got)
	}
}
