package onboarding

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"
	"time"
)

type Generator struct {
	repoRoot string
}

type GeneratedFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type GenerateResult struct {
	Files []GeneratedFile `json:"files"`
}

type envContext struct {
	ExecUsername               string
	ExecSecret                 string
	ProxyEnabled               bool
	ProxyHost                  string
	ProxyPort                  int
	ProxyScheme                string
	ProxyNoProxy               string
	LLMModelIndicator          string
	LLMIndicatorEndpoint       string
	LLMIndicatorAPIKey         string
	LLMModelStructure          string
	LLMStructureEndpoint       string
	LLMStructureAPIKey         string
	LLMModelMechanics          string
	LLMMechanicsEndpoint       string
	LLMMechanicsAPIKey         string
	TelegramEnabled            bool
	TelegramToken              string
	TelegramChatID             string
	FeishuEnabled              bool
	FeishuAppID                string
	FeishuAppSecret            string
	FeishuBotEnabled           bool
	FeishuBotMode              string
	FeishuVerificationToken    string
	FeishuEncryptKey           string
	FeishuDefaultReceiveIDType string
	FeishuDefaultReceiveID     string
}

type systemContext struct {
	ExecEndpoint string
}

type symbolTemplateContext struct {
	Symbol     string
	Intervals  []string
	EMAFast    int
	EMAMid     int
	EMASlow    int
	RSIPeriod  int
	LastN      int
	MACDFast   int
	MACDSlow   int
	MACDSignal int
}

type strategyTemplateContext struct {
	Symbol                      string
	SymbolLower                 string
	RiskPerTradePct             float64
	MaxInvestPct                float64
	MaxLeverage                 int
	EntryMode                   string
	ExitPolicy                  string
	TightenMinUpdateIntervalSec int
}

func NewGenerator(repoRoot string) *Generator {
	return &Generator{repoRoot: strings.TrimSpace(repoRoot)}
}

func (g *Generator) Preview(req Request) (GenerateResult, error) {
	normalized, err := normalizeRequest(req)
	if err != nil {
		return GenerateResult{}, err
	}
	return g.buildFiles(normalized)
}

func (g *Generator) Generate(req Request) (GenerateResult, error) {
	result, err := g.Preview(req)
	if err != nil {
		return GenerateResult{}, err
	}
	for _, f := range result.Files {
		if err := writeAtomic(filepath.Join(g.repoRoot, f.Path), f.Content); err != nil {
			return GenerateResult{}, err
		}
	}
	done := fmt.Sprintf("generated_at=%s\n", time.Now().UTC().Format(time.RFC3339))
	if err := writeAtomic(filepath.Join(g.repoRoot, "data/onboarding/.done"), done); err != nil {
		return GenerateResult{}, err
	}
	return result, nil
}

func (g *Generator) buildFiles(req Request) (GenerateResult, error) {
	files := make([]GeneratedFile, 0, 14+len(req.Symbols)*2)

	envText, err := g.buildEnvContent(req)
	if err != nil {
		return GenerateResult{}, err
	}
	files = append(files, GeneratedFile{Path: ".env", Content: envText})

	configSystem, err := executeTemplate(mustTemplate("system.toml.tmpl", nil), systemContext{ExecEndpoint: "http://127.0.0.1:8080/api/v1"})
	if err != nil {
		return GenerateResult{}, err
	}
	files = append(files, GeneratedFile{Path: "configs/system.toml", Content: configSystem})

	runtimeSystem, err := executeTemplate(mustTemplate("system.toml.tmpl", nil), systemContext{ExecEndpoint: "http://freqtrade:8080/api/v1"})
	if err != nil {
		return GenerateResult{}, err
	}
	files = append(files, GeneratedFile{Path: "data/brale/system.toml", Content: runtimeSystem})

	indexText, err := g.buildSymbolsIndexContent(req.Symbols)
	if err != nil {
		return GenerateResult{}, err
	}
	files = append(files, GeneratedFile{Path: "configs/symbols-index.toml", Content: indexText})

	rulesDefaultBytes, err := templatesFS.ReadFile("templates/rules-default.json")
	if err != nil {
		return GenerateResult{}, err
	}
	files = append(files, GeneratedFile{Path: "configs/rules/default.json", Content: string(rulesDefaultBytes)})

	for _, symbol := range req.Symbols {
		detail := mergeSymbolDetail(symbol, req.SymbolDetail[symbol])

		symbolText, symbolErr := g.buildSymbolConfigContent(symbol, detail)
		if symbolErr != nil {
			return GenerateResult{}, symbolErr
		}
		files = append(files, GeneratedFile{Path: fmt.Sprintf("configs/symbols/%s.toml", symbol), Content: symbolText})

		strategyText, strategyErr := g.buildStrategyConfigContent(symbol, detail)
		if strategyErr != nil {
			return GenerateResult{}, strategyErr
		}
		files = append(files, GeneratedFile{Path: fmt.Sprintf("configs/strategies/%s.toml", symbol), Content: strategyText})
	}

	defaultSymbolText, err := g.buildDefaultSymbolConfigContent()
	if err != nil {
		return GenerateResult{}, err
	}
	files = append(files, GeneratedFile{Path: "configs/symbols/default.toml", Content: defaultSymbolText})

	defaultStrategyText, err := g.buildDefaultStrategyObserveContent()
	if err != nil {
		return GenerateResult{}, err
	}
	files = append(files, GeneratedFile{Path: "configs/strategies/default.toml", Content: defaultStrategyText})

	freqtradeConfig, err := renderFreqtradeConfig(req)
	if err != nil {
		return GenerateResult{}, err
	}
	files = append(files, GeneratedFile{Path: "data/freqtrade/user_data/config.json", Content: freqtradeConfig})

	files = append(files, GeneratedFile{Path: "data/freqtrade/proxy.env", Content: renderProxyEnv(req)})

	freqtradeBaseBytes, err := templatesFS.ReadFile("templates/freqtrade-config.base.json")
	if err != nil {
		return GenerateResult{}, err
	}
	files = append(files, GeneratedFile{Path: "configs/freqtrade/config.base.json", Content: string(freqtradeBaseBytes) + "\n"})

	strategyBytes, err := templatesFS.ReadFile("templates/brale_shared_strategy.py")
	if err != nil {
		return GenerateResult{}, err
	}
	files = append(files, GeneratedFile{Path: "configs/freqtrade/brale_shared_strategy.py", Content: string(strategyBytes)})
	files = append(files, GeneratedFile{Path: "data/freqtrade/user_data/strategies/BraleSharedStrategy.py", Content: string(strategyBytes)})

	return GenerateResult{Files: files}, nil
}

func renderFreqtradeConfig(req Request) (string, error) {
	raw, err := templatesFS.ReadFile("templates/freqtrade-config.base.json")
	if err != nil {
		return "", err
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", err
	}

	apiServer := ensureMap(cfg, "api_server")
	apiServer["username"] = req.ExecUsername
	apiServer["password"] = req.ExecSecret

	webhook := ensureMap(cfg, "webhook")
	webhook["url"] = "http://brale:9991/api/live/freqtrade/webhook"

	exchange := ensureMap(cfg, "exchange")
	ccxtCfg := ensureMap(exchange, "ccxt_config")
	ccxtAsync := ensureMap(exchange, "ccxt_async_config")
	if req.ProxyEnabled {
		proxyURL := fmt.Sprintf("%s://%s:%d", req.ProxyScheme, req.ProxyHost, req.ProxyPort)
		ccxtCfg["proxies"] = map[string]any{"http": proxyURL, "https": proxyURL}
		ccxtAsync["aiohttp_proxy"] = proxyURL
	} else {
		delete(ccxtCfg, "proxies")
		delete(ccxtAsync, "aiohttp_proxy")
	}

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out) + "\n", nil
}

func renderProxyEnv(req Request) string {
	lines := []string{"# generated by onboarding"}
	if req.ProxyEnabled {
		proxyURL := fmt.Sprintf("%s://%s:%d", req.ProxyScheme, req.ProxyHost, req.ProxyPort)
		lines = append(lines,
			"HTTP_PROXY="+proxyURL,
			"HTTPS_PROXY="+proxyURL,
			"http_proxy="+proxyURL,
			"https_proxy="+proxyURL,
		)
	}
	lines = append(lines,
		"NO_PROXY="+req.ProxyNoProxy,
		"no_proxy="+req.ProxyNoProxy,
	)
	return strings.Join(lines, "\n") + "\n"
}

func ensureMap(root map[string]any, key string) map[string]any {
	v, ok := root[key]
	if !ok {
		m := map[string]any{}
		root[key] = m
		return m
	}
	m, ok := v.(map[string]any)
	if ok {
		return m
	}
	m = map[string]any{}
	root[key] = m
	return m
}

func executeTemplate(t *template.Template, data any) (string, error) {
	buf := bytes.NewBuffer(nil)
	if err := t.Execute(buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func normalizeRequest(req Request) (Request, error) {
	out := req
	out.Symbols = uniqueUpperSymbols(req.Symbols)
	if len(out.Symbols) == 0 {
		return Request{}, fmt.Errorf("at least one symbol is required")
	}
	out.SymbolDetail = cloneSymbolMap(req.SymbolDetail)
	out.ExecUsername = strings.TrimSpace(out.ExecUsername)
	out.ExecSecret = strings.TrimSpace(out.ExecSecret)
	out.ProxyHost = strings.TrimSpace(out.ProxyHost)
	out.ProxyScheme = strings.ToLower(strings.TrimSpace(out.ProxyScheme))
	out.ProxyNoProxy = strings.TrimSpace(out.ProxyNoProxy)
	out.LLMModelIndicator = strings.TrimSpace(out.LLMModelIndicator)
	out.LLMIndicatorEndpoint = strings.TrimSpace(out.LLMIndicatorEndpoint)
	out.LLMIndicatorKey = strings.TrimSpace(out.LLMIndicatorKey)
	out.LLMModelStructure = strings.TrimSpace(out.LLMModelStructure)
	out.LLMStructureEndpoint = strings.TrimSpace(out.LLMStructureEndpoint)
	out.LLMStructureKey = strings.TrimSpace(out.LLMStructureKey)
	out.LLMModelMechanics = strings.TrimSpace(out.LLMModelMechanics)
	out.LLMMechanicsEndpoint = strings.TrimSpace(out.LLMMechanicsEndpoint)
	out.LLMMechanicsKey = strings.TrimSpace(out.LLMMechanicsKey)
	out.TelegramToken = strings.TrimSpace(out.TelegramToken)
	out.TelegramChatID = strings.TrimSpace(out.TelegramChatID)
	out.FeishuAppID = strings.TrimSpace(out.FeishuAppID)
	out.FeishuAppSecret = strings.TrimSpace(out.FeishuAppSecret)
	out.FeishuBotMode = strings.TrimSpace(out.FeishuBotMode)
	out.FeishuVerificationToken = strings.TrimSpace(out.FeishuVerificationToken)
	out.FeishuEncryptKey = strings.TrimSpace(out.FeishuEncryptKey)
	out.FeishuDefaultReceiveIDType = strings.TrimSpace(out.FeishuDefaultReceiveIDType)
	out.FeishuDefaultReceiveID = strings.TrimSpace(out.FeishuDefaultReceiveID)

	if out.ExecUsername == "" || out.ExecSecret == "" {
		return Request{}, fmt.Errorf("exec_username and exec_secret are required")
	}
	if out.ProxyHost == "" {
		out.ProxyHost = "host.docker.internal"
	}
	if out.ProxyPort == 0 {
		out.ProxyPort = 7890
	}
	if out.ProxyPort < 1 || out.ProxyPort > 65535 {
		return Request{}, fmt.Errorf("proxy_port must be in [1,65535]")
	}
	if out.ProxyScheme == "" {
		out.ProxyScheme = "http"
	}
	if !slices.Contains([]string{"http", "https", "socks5"}, out.ProxyScheme) {
		return Request{}, fmt.Errorf("proxy_scheme must be one of http/https/socks5")
	}
	if out.ProxyNoProxy == "" {
		out.ProxyNoProxy = "localhost,127.0.0.1,brale,freqtrade"
	}

	if out.LLMModelIndicator == "" || out.LLMIndicatorEndpoint == "" || out.LLMIndicatorKey == "" {
		return Request{}, fmt.Errorf("indicator model config is required")
	}
	if out.LLMModelStructure == "" || out.LLMStructureEndpoint == "" || out.LLMStructureKey == "" {
		return Request{}, fmt.Errorf("structure model config is required")
	}
	if out.LLMModelMechanics == "" || out.LLMMechanicsEndpoint == "" || out.LLMMechanicsKey == "" {
		return Request{}, fmt.Errorf("mechanics model config is required")
	}

	if out.FeishuBotMode == "" {
		out.FeishuBotMode = "long_connection"
	}
	if out.FeishuDefaultReceiveIDType == "" {
		out.FeishuDefaultReceiveIDType = "chat_id"
	}

	for _, symbol := range out.Symbols {
		base := defaultDetailForSymbol(symbol)
		merged := mergeSymbolDetail(symbol, out.SymbolDetail[symbol])
		if len(merged.Intervals) == 0 {
			merged.Intervals = base.Intervals
		}
		if merged.EntryMode == "" {
			merged.EntryMode = base.EntryMode
		}
		if merged.ExitPolicy == "" {
			merged.ExitPolicy = base.ExitPolicy
		}
		if merged.MaxLeverage <= 0 {
			return Request{}, fmt.Errorf("%s max_leverage must be > 0", symbol)
		}
		if merged.EMAFast <= 0 || merged.EMAMid <= 0 || merged.EMASlow <= 0 || merged.RSIPeriod <= 0 || merged.LastN <= 0 {
			return Request{}, fmt.Errorf("%s indicator periods must be > 0", symbol)
		}
		if merged.MACDFast <= 0 || merged.MACDSlow <= 0 || merged.MACDSignal <= 0 {
			return Request{}, fmt.Errorf("%s macd periods must be > 0", symbol)
		}
		out.SymbolDetail[symbol] = merged
	}

	return out, nil
}

func envContextFromRequest(req Request) envContext {
	return envContext{
		ExecUsername:               req.ExecUsername,
		ExecSecret:                 req.ExecSecret,
		ProxyEnabled:               req.ProxyEnabled,
		ProxyHost:                  req.ProxyHost,
		ProxyPort:                  req.ProxyPort,
		ProxyScheme:                req.ProxyScheme,
		ProxyNoProxy:               req.ProxyNoProxy,
		LLMModelIndicator:          req.LLMModelIndicator,
		LLMIndicatorEndpoint:       req.LLMIndicatorEndpoint,
		LLMIndicatorAPIKey:         req.LLMIndicatorKey,
		LLMModelStructure:          req.LLMModelStructure,
		LLMStructureEndpoint:       req.LLMStructureEndpoint,
		LLMStructureAPIKey:         req.LLMStructureKey,
		LLMModelMechanics:          req.LLMModelMechanics,
		LLMMechanicsEndpoint:       req.LLMMechanicsEndpoint,
		LLMMechanicsAPIKey:         req.LLMMechanicsKey,
		TelegramEnabled:            req.TelegramEnabled,
		TelegramToken:              req.TelegramToken,
		TelegramChatID:             req.TelegramChatID,
		FeishuEnabled:              req.FeishuEnabled,
		FeishuAppID:                req.FeishuAppID,
		FeishuAppSecret:            req.FeishuAppSecret,
		FeishuBotEnabled:           req.FeishuBotEnabled,
		FeishuBotMode:              req.FeishuBotMode,
		FeishuVerificationToken:    req.FeishuVerificationToken,
		FeishuEncryptKey:           req.FeishuEncryptKey,
		FeishuDefaultReceiveIDType: req.FeishuDefaultReceiveIDType,
		FeishuDefaultReceiveID:     req.FeishuDefaultReceiveID,
	}
}

func mergeSymbolDetail(symbol string, override SymbolDetail) SymbolDetail {
	base := defaultDetailForSymbol(symbol)
	if override.RiskPerTradePct > 0 {
		base.RiskPerTradePct = override.RiskPerTradePct
	}
	if override.MaxInvestPct > 0 {
		base.MaxInvestPct = override.MaxInvestPct
	}
	if override.MaxLeverage > 0 {
		base.MaxLeverage = override.MaxLeverage
	}
	if len(override.Intervals) > 0 {
		base.Intervals = sanitizeIntervals(override.Intervals)
	}
	if strings.TrimSpace(override.EntryMode) != "" {
		base.EntryMode = strings.TrimSpace(override.EntryMode)
	}
	if strings.TrimSpace(override.ExitPolicy) != "" {
		base.ExitPolicy = strings.TrimSpace(override.ExitPolicy)
	}
	if override.TightenMinUpdateIntervalSec > 0 {
		base.TightenMinUpdateIntervalSec = override.TightenMinUpdateIntervalSec
	}
	if override.EMAFast > 0 {
		base.EMAFast = override.EMAFast
	}
	if override.EMAMid > 0 {
		base.EMAMid = override.EMAMid
	}
	if override.EMASlow > 0 {
		base.EMASlow = override.EMASlow
	}
	if override.RSIPeriod > 0 {
		base.RSIPeriod = override.RSIPeriod
	}
	if override.LastN > 0 {
		base.LastN = override.LastN
	}
	if override.MACDFast > 0 {
		base.MACDFast = override.MACDFast
	}
	if override.MACDSlow > 0 {
		base.MACDSlow = override.MACDSlow
	}
	if override.MACDSignal > 0 {
		base.MACDSignal = override.MACDSignal
	}
	return base
}

func defaultDetailForSymbol(symbol string) SymbolDetail {
	if symbol == "ETHUSDT" {
		return SymbolDetail{
			RiskPerTradePct:             0.5,
			MaxInvestPct:                1.0,
			MaxLeverage:                 10,
			Intervals:                   []string{"1h", "4h", "1d"},
			EntryMode:                   "orderbook",
			ExitPolicy:                  "atr_structure_v1",
			TightenMinUpdateIntervalSec: 600,
			EMAFast:                     21,
			EMAMid:                      50,
			EMASlow:                     200,
			RSIPeriod:                   14,
			LastN:                       5,
			MACDFast:                    12,
			MACDSlow:                    26,
			MACDSignal:                  9,
		}
	}
	return SymbolDetail{
		RiskPerTradePct:             0.5,
		MaxInvestPct:                1.0,
		MaxLeverage:                 3,
		Intervals:                   []string{"1h", "4h", "1d"},
		EntryMode:                   "orderbook",
		ExitPolicy:                  "atr_structure_v1",
		TightenMinUpdateIntervalSec: 300,
		EMAFast:                     21,
		EMAMid:                      50,
		EMASlow:                     200,
		RSIPeriod:                   14,
		LastN:                       5,
		MACDFast:                    12,
		MACDSlow:                    26,
		MACDSignal:                  9,
	}
}

func sanitizeIntervals(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, item := range in {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return []string{"1h", "4h", "1d"}
	}
	return out
}

func uniqueUpperSymbols(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, item := range in {
		s := strings.ToUpper(strings.TrimSpace(item))
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	slices.Sort(out)
	return out
}

func cloneSymbolMap(in map[string]SymbolDetail) map[string]SymbolDetail {
	if in == nil {
		return map[string]SymbolDetail{}
	}
	out := make(map[string]SymbolDetail, len(in))
	for k, v := range in {
		copied := v
		copied.Intervals = append([]string(nil), v.Intervals...)
		out[strings.ToUpper(strings.TrimSpace(k))] = copied
	}
	return out
}

func writeAtomic(path string, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
