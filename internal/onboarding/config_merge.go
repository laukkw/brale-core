package onboarding

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
)

func (g *Generator) buildSymbolsIndexContent(symbols []string) (string, error) {
	existing, _ := g.readRepoFile("configs/symbols-index.toml")
	return renderSymbolsIndex(existing, symbols), nil
}

func (g *Generator) buildSymbolConfigContent(symbol string, detail SymbolDetail) (string, error) {
	rel := fmt.Sprintf("configs/symbols/%s.toml", symbol)
	base, targetExists := g.readRepoFile(rel)
	if !targetExists {
		base, _ = g.readRepoFile("configs/symbols/default.toml")
	}
	if strings.TrimSpace(base) == "" {
		tpl := "symbol-default.toml.tmpl"
		if symbol == "ETHUSDT" {
			tpl = "symbol-eth.toml.tmpl"
		}
		text, err := executeTemplate(mustTemplate(tpl, onboardingTemplateFuncs()), symbolTemplateContext{
			Symbol:     symbol,
			Intervals:  detail.Intervals,
			EMAFast:    detail.EMAFast,
			EMAMid:     detail.EMAMid,
			EMASlow:    detail.EMASlow,
			RSIPeriod:  detail.RSIPeriod,
			LastN:      detail.LastN,
			MACDFast:   detail.MACDFast,
			MACDSlow:   detail.MACDSlow,
			MACDSignal: detail.MACDSignal,
		})
		if err != nil {
			return "", err
		}
		base = text
	}
	out, err := applyTomlUpdates(base, symbolConfigRegistry(symbol, detail))
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out, nil
}

func (g *Generator) buildDefaultSymbolConfigContent() (string, error) {
	detail := defaultDetailForSymbol("DEFAULT")
	rel := "configs/symbols/default.toml"
	base, exists := g.readRepoFile(rel)
	if !exists {
		text, err := executeTemplate(mustTemplate("symbol-default-observe.toml.tmpl", onboardingTemplateFuncs()), symbolTemplateContext{
			Symbol:     "DEFAULT",
			Intervals:  detail.Intervals,
			EMAFast:    detail.EMAFast,
			EMAMid:     detail.EMAMid,
			EMASlow:    detail.EMASlow,
			RSIPeriod:  detail.RSIPeriod,
			LastN:      detail.LastN,
			MACDFast:   detail.MACDFast,
			MACDSlow:   detail.MACDSlow,
			MACDSignal: detail.MACDSignal,
		})
		if err != nil {
			return "", err
		}
		base = text
	}
	out, err := applyTomlUpdates(base, defaultSymbolConfigRegistry(detail))
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out, nil
}

func (g *Generator) buildStrategyConfigContent(symbol string, detail SymbolDetail) (string, error) {
	rel := fmt.Sprintf("configs/strategies/%s.toml", symbol)
	base, targetExists := g.readRepoFile(rel)
	if !targetExists {
		base, _ = g.readRepoFile("configs/strategies/default.toml")
	}
	if strings.TrimSpace(base) == "" {
		tpl := "strategy-generic.toml.tmpl"
		if symbol == "ETHUSDT" {
			tpl = "strategy-eth.toml.tmpl"
		}
		text, err := executeTemplate(mustTemplate(tpl, onboardingTemplateFuncs()), strategyTemplateContext{
			Symbol:                      symbol,
			SymbolLower:                 strings.ToLower(symbol),
			RiskPerTradePct:             detail.RiskPerTradePct,
			MaxInvestPct:                detail.MaxInvestPct,
			MaxLeverage:                 detail.MaxLeverage,
			EntryMode:                   detail.EntryMode,
			ExitPolicy:                  detail.ExitPolicy,
			TightenMinUpdateIntervalSec: detail.TightenMinUpdateIntervalSec,
		})
		if err != nil {
			return "", err
		}
		base = text
	}
	out, err := applyTomlUpdates(base, strategyConfigRegistry(symbol, detail, !targetExists && symbol != "ETHUSDT"))
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out, nil
}

func (g *Generator) buildDefaultStrategyObserveContent() (string, error) {
	base, exists := g.readRepoFile("configs/strategies/default.toml")
	if !exists {
		text, err := executeTemplate(mustTemplate("strategy-default.toml.tmpl", onboardingTemplateFuncs()), strategyTemplateContext{})
		if err != nil {
			return "", err
		}
		base = text
	}
	out, err := applyTomlUpdates(base, defaultStrategyObserveRegistry())
	if err != nil {
		return "", err
	}
	out, err = applyObserveModeOnSieveRows(out)
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out, nil
}

func (g *Generator) readRepoFile(rel string) (string, bool) {
	abs := filepath.Join(g.repoRoot, rel)
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", false
	}
	return string(data), true
}

func renderSymbolsIndex(existing string, symbols []string) string {
	header := strings.TrimSpace(symbolsIndexHeader(existing))
	if header == "" {
		header = "# brale-core 币种清单（显式列出要跑的 symbol 与配置文件路径）\n# - symbol: 币种名（必须大写）\n# - config: 币种配置文件路径\n# - strategy: 策略配置文件路径"
	}
	parts := []string{header}
	for i, symbol := range symbols {
		parts = append(parts,
			"[[symbols]]",
			fmt.Sprintf("symbol = %s # 币种名，例：ETHUSDT", tomlQuoted(symbol)),
			fmt.Sprintf("config = %s # 币种配置路径，例：symbols/ETHUSDT.toml", tomlQuoted("symbols/"+symbol+".toml")),
			fmt.Sprintf("strategy = %s # 策略配置路径，例：strategies/ETHUSDT.toml", tomlQuoted("strategies/"+symbol+".toml")),
		)
		if i < len(symbols)-1 {
			parts = append(parts, "#", "#", "#", "#")
		}
	}
	return strings.Join(parts, "\n") + "\n"
}

func symbolsIndexHeader(content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	cut := len(lines)
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "[[symbols]]") {
			cut = i
			break
		}
	}
	return strings.Join(lines[:cut], "\n")
}

func tomlQuoted(v string) string {
	return strconv.Quote(v)
}

func tomlStringArray(values []string) string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, strconv.Quote(v))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func tomlFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func onboardingTemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"joinQuoted": func(in []string) string {
			quoted := make([]string, 0, len(in))
			for _, v := range in {
				quoted = append(quoted, strconv.Quote(v))
			}
			return strings.Join(quoted, ", ")
		},
	}
}

func symbolConfigRegistry(symbol string, detail SymbolDetail) []tomlUpdate {
	return []tomlUpdate{
		{Path: []string{"symbol"}, Value: tomlQuoted(symbol)},
		{Path: []string{"intervals"}, Value: tomlStringArray(detail.Intervals)},
		{Path: []string{"indicators", "ema_fast"}, Value: strconv.Itoa(detail.EMAFast)},
		{Path: []string{"indicators", "ema_mid"}, Value: strconv.Itoa(detail.EMAMid)},
		{Path: []string{"indicators", "ema_slow"}, Value: strconv.Itoa(detail.EMASlow)},
		{Path: []string{"indicators", "rsi_period"}, Value: strconv.Itoa(detail.RSIPeriod)},
		{Path: []string{"indicators", "macd_fast"}, Value: strconv.Itoa(detail.MACDFast)},
		{Path: []string{"indicators", "macd_slow"}, Value: strconv.Itoa(detail.MACDSlow)},
		{Path: []string{"indicators", "macd_signal"}, Value: strconv.Itoa(detail.MACDSignal)},
		{Path: []string{"indicators", "last_n"}, Value: strconv.Itoa(detail.LastN)},
	}
}

func defaultSymbolConfigRegistry(detail SymbolDetail) []tomlUpdate {
	return []tomlUpdate{
		{Path: []string{"symbol"}, Value: tomlQuoted("DEFAULT")},
		{Path: []string{"intervals"}, Value: tomlStringArray(detail.Intervals)},
	}
}

func strategyConfigRegistry(symbol string, detail SymbolDetail, setID bool) []tomlUpdate {
	updates := []tomlUpdate{
		{Path: []string{"symbol"}, Value: tomlQuoted(symbol)},
		{Path: []string{"risk_management", "risk_per_trade_pct"}, Value: tomlFloat(detail.RiskPerTradePct)},
		{Path: []string{"risk_management", "max_invest_pct"}, Value: tomlFloat(detail.MaxInvestPct)},
		{Path: []string{"risk_management", "max_leverage"}, Value: strconv.Itoa(detail.MaxLeverage)},
		{Path: []string{"risk_management", "entry_mode"}, Value: tomlQuoted(detail.EntryMode)},
		{Path: []string{"risk_management", "initial_exit", "policy"}, Value: tomlQuoted(detail.ExitPolicy)},
		{Path: []string{"risk_management", "tighten_atr", "min_update_interval_sec"}, Value: strconv.Itoa(detail.TightenMinUpdateIntervalSec)},
	}
	if setID {
		updates = append([]tomlUpdate{{Path: []string{"id"}, Value: tomlQuoted("default-" + strings.ToLower(symbol))}}, updates...)
	}
	return updates
}

func defaultStrategyObserveRegistry() []tomlUpdate {
	return []tomlUpdate{
		{Path: []string{"symbol"}, Value: tomlQuoted("DEFAULT")},
		{Path: []string{"id"}, Value: tomlQuoted("default-observe")},
		{Path: []string{"risk_management", "risk_per_trade_pct"}, Value: "0.0"},
		{Path: []string{"risk_management", "max_invest_pct"}, Value: "0.0"},
		{Path: []string{"risk_management", "max_leverage"}, Value: "1.0"},
		{Path: []string{"risk_management", "grade_3_factor"}, Value: "0.0"},
		{Path: []string{"risk_management", "grade_2_factor"}, Value: "0.0"},
		{Path: []string{"risk_management", "grade_1_factor"}, Value: "0.0"},
		{Path: []string{"risk_management", "sieve", "min_size_factor"}, Value: "0.0"},
		{Path: []string{"risk_management", "sieve", "default_gate_action"}, Value: tomlQuoted("WAIT")},
		{Path: []string{"risk_management", "sieve", "default_size_factor"}, Value: "0.0"},
	}
}
