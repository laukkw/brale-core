package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"brale-core/internal/decision/features"
	"brale-core/internal/transport/botruntime"
	runtimeapi "brale-core/internal/transport/runtimeapi"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerListsAndCallsReadOnlyTools(t *testing.T) {
	systemPath, indexPath := writeMCPConfigTree(t)
	audit := &memoryAuditSink{}
	server, err := NewServer(Options{
		Name:    "brale-core",
		Version: "test",
		Runtime: stubRuntime{
			latestDecision: botruntime.DecisionLatestResponse{
				Status:    "ok",
				Symbol:    "BTCUSDT",
				Summary:   "latest decision",
				RequestID: "req-latest",
			},
			positions: botruntime.PositionStatusResponse{
				Status:  "ok",
				Summary: "positions",
				Positions: []botruntime.PositionStatusItem{
					{Symbol: "BTCUSDT", Side: "long", Amount: 0.1, CurrentPrice: 101.2},
				},
				RequestID: "req-positions",
			},
			decisionHistory: botruntime.DashboardDecisionHistoryResponse{
				Status:    "ok",
				Symbol:    "BTCUSDT",
				Limit:     2,
				Summary:   "history",
				RequestID: "req-history",
				Items: []runtimeapi.DashboardDecisionHistoryItem{
					{SnapshotID: 10, Action: "ALLOW", Reason: "PASS"},
				},
			},
			accountSummary: botruntime.DashboardAccountSummaryResponse{
				Status:    "ok",
				Summary:   "account",
				RequestID: "req-account",
				Balance:   runtimeapi.DashboardBalanceCard{Currency: "USDT", Total: 1000},
			},
			kline: botruntime.DashboardKlineResponse{
				Status:    "ok",
				Symbol:    "BTCUSDT",
				Interval:  "1h",
				Limit:     3,
				Summary:   "kline",
				RequestID: "req-kline",
				Candles:   indicatorTestCandles(3),
			},
		},
		Config: NewLocalConfigSource(systemPath, indexPath),
		Audit:  audit,
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	session := connectTestSession(t, server)
	defer session.Close()

	list, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	gotTools := make(map[string]struct{}, len(list.Tools))
	for _, tool := range list.Tools {
		gotTools[tool.Name] = struct{}{}
	}
	for _, want := range []string{
		"analyze_market",
		"get_latest_decision",
		"get_positions",
		"get_decision_history",
		"get_account_summary",
		"get_kline",
		"compute_indicators",
		"get_config",
	} {
		if _, ok := gotTools[want]; !ok {
			t.Fatalf("tool %q not registered", want)
		}
	}

	latest := callToolMap(t, session, "get_latest_decision", map[string]any{"symbol": "BTCUSDT"})
	if latest["symbol"] != "BTCUSDT" || latest["summary"] != "latest decision" {
		t.Fatalf("latest=%v", latest)
	}

	positions := callToolMap(t, session, "get_positions", map[string]any{})
	if positions["summary"] != "positions" {
		t.Fatalf("positions=%v", positions)
	}

	account := callToolMap(t, session, "get_account_summary", map[string]any{})
	if account["summary"] != "account" {
		t.Fatalf("account=%v", account)
	}

	history := callToolMap(t, session, "get_decision_history", map[string]any{"symbol": "BTCUSDT", "limit": 2})
	if history["summary"] != "history" {
		t.Fatalf("history=%v", history)
	}

	kline := callToolMap(t, session, "get_kline", map[string]any{"symbol": "BTCUSDT", "interval": "1h", "limit": 3})
	if got := int(kline["limit"].(float64)); got != 3 {
		t.Fatalf("kline limit=%d want 3", got)
	}

	if len(audit.events) != 5 {
		t.Fatalf("audit events=%d want 5", len(audit.events))
	}
}

func TestAnalyzeMarketToolCombinesRuntimeViews(t *testing.T) {
	systemPath, indexPath := writeMCPConfigTree(t)
	server, err := NewServer(Options{
		Name:    "brale-core",
		Version: "test",
		Runtime: stubRuntime{
			overview: botruntime.DashboardOverviewResponse{
				Status:    "ok",
				Symbol:    "BTCUSDT",
				Summary:   "overview",
				RequestID: "req-overview",
				Symbols: []runtimeapi.DashboardOverviewSymbol{
					{Symbol: "BTCUSDT", Position: runtimeapi.DashboardPositionCard{Side: "long", CurrentPrice: 101.2}},
				},
			},
			latestDecision: botruntime.DecisionLatestResponse{
				Status:    "ok",
				Symbol:    "BTCUSDT",
				Summary:   "latest",
				RequestID: "req-latest",
			},
			observe: botruntime.ObserveResponse{
				Symbol:    "BTCUSDT",
				Status:    "ok",
				Summary:   "observe",
				RequestID: "req-observe",
			},
		},
		Config: NewLocalConfigSource(systemPath, indexPath),
		Audit:  &memoryAuditSink{},
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	session := connectTestSession(t, server)
	defer session.Close()

	got := callToolMap(t, session, "analyze_market", map[string]any{"symbol": "BTCUSDT"})
	if got["symbol"] != "BTCUSDT" {
		t.Fatalf("symbol=%v want BTCUSDT", got["symbol"])
	}
	if got["observe"].(map[string]any)["summary"] != "observe" {
		t.Fatalf("observe=%v", got["observe"])
	}
	if got["latest_decision"].(map[string]any)["summary"] != "latest" {
		t.Fatalf("latest_decision=%v", got["latest_decision"])
	}
	if got["overview"].(map[string]any)["summary"] != "overview" {
		t.Fatalf("overview=%v", got["overview"])
	}
	if got["observe_available"] != true {
		t.Fatalf("observe_available=%v want true", got["observe_available"])
	}
}

func TestAnalyzeMarketToolDegradesWhenObserveReportMissing(t *testing.T) {
	systemPath, indexPath := writeMCPConfigTree(t)
	server, err := NewServer(Options{
		Name:    "brale-core",
		Version: "test",
		Runtime: stubRuntime{
			overview: botruntime.DashboardOverviewResponse{
				Status:    "ok",
				Symbol:    "BTCUSDT",
				Summary:   "overview",
				RequestID: "req-overview",
			},
			latestDecision: botruntime.DecisionLatestResponse{
				Status:    "ok",
				Symbol:    "BTCUSDT",
				Summary:   "latest",
				RequestID: "req-latest",
			},
			observeErr: errors.New("暂无该符号观察结果"),
		},
		Config: NewLocalConfigSource(systemPath, indexPath),
		Audit:  &memoryAuditSink{},
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	session := connectTestSession(t, server)
	defer session.Close()

	got := callToolMap(t, session, "analyze_market", map[string]any{"symbol": "BTCUSDT"})
	if got["symbol"] != "BTCUSDT" {
		t.Fatalf("symbol=%v want BTCUSDT", got["symbol"])
	}
	if got["latest_decision"].(map[string]any)["summary"] != "latest" {
		t.Fatalf("latest_decision=%v", got["latest_decision"])
	}
	if got["overview"].(map[string]any)["summary"] != "overview" {
		t.Fatalf("overview=%v", got["overview"])
	}
	if got["observe_available"] != false {
		t.Fatalf("observe_available=%v want false", got["observe_available"])
	}
	errMsg, _ := got["observe_error"].(string)
	if !strings.Contains(errMsg, "暂无该符号观察结果") {
		t.Fatalf("observe_error=%v", got["observe_error"])
	}
	if _, ok := got["observe"]; ok {
		t.Fatalf("observe should be omitted on fallback: %v", got["observe"])
	}
}

func TestGetConfigToolRedactsSecrets(t *testing.T) {
	systemPath, indexPath := writeMCPConfigTree(t)
	server, err := NewServer(Options{
		Name:    "brale-core",
		Version: "test",
		Runtime: stubRuntime{},
		Config:  NewLocalConfigSource(systemPath, indexPath),
		Audit:   &memoryAuditSink{},
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	session := connectTestSession(t, server)
	defer session.Close()

	got := callToolMap(t, session, "get_config", map[string]any{"symbol": "BTCUSDT"})
	system := got["system"].(map[string]any)
	if _, ok := system["exec_api_key"]; ok {
		t.Fatalf("system leaked exec_api_key: %v", system)
	}
	if system["has_exec_api_key"] != true {
		t.Fatalf("system=%v should expose has_exec_api_key=true", system)
	}
	models := system["llm_models"].(map[string]any)
	mock := models["mock"].(map[string]any)
	if _, ok := mock["api_key"]; ok {
		t.Fatalf("llm model leaked api_key: %v", mock)
	}
	if mock["has_api_key"] != true {
		t.Fatalf("mock=%v should expose has_api_key=true", mock)
	}
	symbolCfg := got["symbol"].(map[string]any)
	if symbolCfg["symbol"] != "BTCUSDT" {
		t.Fatalf("symbol config=%v", symbolCfg)
	}
	indicators := symbolCfg["indicators"].(map[string]any)
	if indicators["engine"] != "ta" {
		t.Fatalf("indicators=%v", indicators)
	}
	if indicators["shadow_engine"] != "reference" {
		t.Fatalf("indicators=%v", indicators)
	}
	memoryCfg := symbolCfg["memory"].(map[string]any)
	if memoryCfg["enabled"] != true || memoryCfg["episodic_enabled"] != true || memoryCfg["semantic_enabled"] != true {
		t.Fatalf("memory=%v", memoryCfg)
	}
	if got := int(memoryCfg["working_memory_size"].(float64)); got != 5 {
		t.Fatalf("memory=%v", memoryCfg)
	}
	strategyCfg := got["strategy"].(map[string]any)
	if strategyCfg["id"] != "default-BTCUSDT" {
		t.Fatalf("strategy=%v", strategyCfg)
	}
}

func TestComputeIndicatorsToolUsesLocalIndicatorConfig(t *testing.T) {
	systemPath, indexPath := writeMCPConfigTree(t)
	server, err := NewServer(Options{
		Name:    "brale-core",
		Version: "test",
		Runtime: stubRuntime{
			kline: botruntime.DashboardKlineResponse{
				Status:    "ok",
				Symbol:    "BTCUSDT",
				Interval:  "1h",
				Limit:     260,
				Summary:   "kline",
				RequestID: "req-kline",
				Candles:   indicatorTestCandles(260),
			},
		},
		Config: NewLocalConfigSource(systemPath, indexPath),
		Audit:  &memoryAuditSink{},
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	session := connectTestSession(t, server)
	defer session.Close()

	got := callToolMap(t, session, "compute_indicators", map[string]any{"symbol": "BTCUSDT", "interval": "1h", "limit": 260})
	market := got["market"].(map[string]any)
	if market["symbol"] != "BTCUSDT" || market["interval"] != "1h" {
		t.Fatalf("market=%v", market)
	}
	data := got["data"].(map[string]any)
	for _, key := range []string{"ema_fast", "rsi", "atr", "bb", "chop", "aroon"} {
		if data[key] == nil {
			t.Fatalf("indicator %q missing in data=%v", key, data)
		}
	}
}

func TestComputeIndicatorsToolUsesConfiguredReferenceEngine(t *testing.T) {
	systemPath, indexPath := writeMCPConfigTree(t)
	symbolPath := filepath.Join(filepath.Dir(indexPath), "symbols", "BTCUSDT.toml")
	raw, err := os.ReadFile(symbolPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	raw = bytes.Replace(raw, []byte("[indicators]\n"), []byte("[indicators]\nengine = \"reference\"\n"), 1)
	raw = bytes.Replace(raw, []byte("shadow_engine = \"reference\"\n"), []byte("shadow_engine = \"talib\"\n"), 1)
	if err := os.WriteFile(symbolPath, raw, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	candles := indicatorTestCandles(260)
	server, err := NewServer(Options{
		Name:    "brale-core",
		Version: "test",
		Runtime: stubRuntime{
			kline: botruntime.DashboardKlineResponse{
				Status:    "ok",
				Symbol:    "BTCUSDT",
				Interval:  "1h",
				Limit:     260,
				Summary:   "kline",
				RequestID: "req-kline",
				Candles:   candles,
			},
		},
		Config: NewLocalConfigSource(systemPath, indexPath),
		Audit:  &memoryAuditSink{},
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	session := connectTestSession(t, server)
	defer session.Close()

	got := callToolMap(t, session, "compute_indicators", map[string]any{"symbol": "BTCUSDT", "interval": "1h", "limit": 260})
	want, err := features.BuildIndicatorCompressedInputWithComputer("BTCUSDT", "1h", toSnapshotCandles(candles), features.IndicatorCompressOptions{
		EMAFast:        21,
		EMAMid:         50,
		EMASlow:        200,
		RSIPeriod:      14,
		ATRPeriod:      14,
		STCFast:        23,
		STCSlow:        50,
		BBPeriod:       20,
		BBMultiplier:   2.0,
		CHOPPeriod:     14,
		StochRSIPeriod: 14,
		AroonPeriod:    25,
		LastN:          5,
	}, features.ReferenceComputer{})
	if err != nil {
		t.Fatalf("BuildIndicatorCompressedInputWithComputer() error = %v", err)
	}
	data := got["data"].(map[string]any)
	emaFast := data["ema_fast"].(map[string]any)
	if emaFast["latest"] != want.Data.EMAFast.Latest {
		t.Fatalf("ema_fast latest=%v want %v", emaFast["latest"], want.Data.EMAFast.Latest)
	}
}

func TestServerListsReadsResourcesAndPrompts(t *testing.T) {
	systemPath, indexPath := writeMCPConfigTree(t)
	audit := &memoryAuditSink{}
	server, err := NewServer(Options{
		Name:    "brale-core",
		Version: "test",
		Runtime: stubRuntime{
			latestDecision: botruntime.DecisionLatestResponse{
				Status:    "ok",
				Symbol:    "BTCUSDT",
				Summary:   "latest decision",
				RequestID: "req-latest",
			},
			kline: botruntime.DashboardKlineResponse{
				Status:    "ok",
				Symbol:    "BTCUSDT",
				Interval:  "1h",
				Limit:     3,
				Summary:   "kline",
				RequestID: "req-kline",
				Candles:   indicatorTestCandles(3),
			},
		},
		Config: NewLocalConfigSource(systemPath, indexPath),
		Audit:  audit,
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	session := connectTestSession(t, server)
	defer session.Close()

	resourceURIs := listResourceURIs(t, session)
	for _, want := range []string{"brale://config/system", "brale://config/symbols"} {
		if !containsTestString(resourceURIs, want) {
			t.Fatalf("resource %q not registered: %v", want, resourceURIs)
		}
	}
	resourceTemplates := listResourceTemplates(t, session)
	for _, want := range []string{"brale://decision/{symbol}/latest", "brale://kline/{symbol}/{interval}"} {
		if !containsTestString(resourceTemplates, want) {
			t.Fatalf("resource template %q not registered: %v", want, resourceTemplates)
		}
	}
	promptNames := listPromptNames(t, session)
	for _, want := range []string{"market_analysis", "trade_review"} {
		if !containsTestString(promptNames, want) {
			t.Fatalf("prompt %q not registered: %v", want, promptNames)
		}
	}

	system := readResourceMap(t, session, "brale://config/system")
	if _, ok := system["exec_api_key"]; ok {
		t.Fatalf("system resource leaked exec_api_key: %v", system)
	}
	if system["has_exec_api_key"] != true {
		t.Fatalf("system resource=%v should expose has_exec_api_key=true", system)
	}
	latest := readResourceMap(t, session, "brale://decision/BTCUSDT/latest")
	if latest["summary"] != "latest decision" {
		t.Fatalf("latest resource=%v", latest)
	}

	marketPrompt, err := session.GetPrompt(t.Context(), &sdkmcp.GetPromptParams{
		Name:      "market_analysis",
		Arguments: map[string]string{"symbol": "BTCUSDT", "interval": "1h"},
	})
	if err != nil {
		t.Fatalf("GetPrompt(market_analysis) error = %v", err)
	}
	if len(marketPrompt.Messages) == 0 {
		t.Fatalf("market_analysis returned no messages")
	}
	marketText, ok := marketPrompt.Messages[0].Content.(*sdkmcp.TextContent)
	if !ok {
		t.Fatalf("market_analysis content type = %T", marketPrompt.Messages[0].Content)
	}
	if !strings.Contains(marketText.Text, "brale://decision/BTCUSDT/latest") {
		t.Fatalf("market_analysis text=%q missing latest decision resource", marketText.Text)
	}

	reviewPrompt, err := session.GetPrompt(t.Context(), &sdkmcp.GetPromptParams{
		Name:      "trade_review",
		Arguments: map[string]string{"symbol": "BTCUSDT", "interval": "1h"},
	})
	if err != nil {
		t.Fatalf("GetPrompt(trade_review) error = %v", err)
	}
	if len(reviewPrompt.Messages) == 0 {
		t.Fatalf("trade_review returned no messages")
	}
	reviewText, ok := reviewPrompt.Messages[0].Content.(*sdkmcp.TextContent)
	if !ok {
		t.Fatalf("trade_review content type = %T", reviewPrompt.Messages[0].Content)
	}
	if !strings.Contains(reviewText.Text, "get_decision_history") {
		t.Fatalf("trade_review text=%q missing get_decision_history guidance", reviewText.Text)
	}

	if len(audit.events) != 4 {
		t.Fatalf("audit events=%d want 4", len(audit.events))
	}
}

func TestNewSSEHandlerServesToolsResourcesAndPrompts(t *testing.T) {
	systemPath, indexPath := writeMCPConfigTree(t)
	handler, err := NewSSEHandler(Options{
		Name:    "brale-core",
		Version: "test",
		Runtime: stubRuntime{
			latestDecision: botruntime.DecisionLatestResponse{
				Status:    "ok",
				Symbol:    "BTCUSDT",
				Summary:   "latest decision",
				RequestID: "req-latest",
			},
			kline: botruntime.DashboardKlineResponse{
				Status:    "ok",
				Symbol:    "BTCUSDT",
				Interval:  "1h",
				Limit:     3,
				Summary:   "kline",
				RequestID: "req-kline",
				Candles:   indicatorTestCandles(3),
			},
		},
		Config: NewLocalConfigSource(systemPath, indexPath),
		Audit:  &memoryAuditSink{},
	})
	if err != nil {
		t.Fatalf("NewSSEHandler() error = %v", err)
	}
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	session, err := client.Connect(t.Context(), &sdkmcp.SSEClientTransport{Endpoint: httpServer.URL}, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	defer session.Close()

	latest := callToolMap(t, session, "get_latest_decision", map[string]any{"symbol": "BTCUSDT"})
	if latest["summary"] != "latest decision" {
		t.Fatalf("latest tool=%v", latest)
	}
	resource := readResourceMap(t, session, "brale://decision/BTCUSDT/latest")
	if resource["summary"] != "latest decision" {
		t.Fatalf("decision resource=%v", resource)
	}
	prompt, err := session.GetPrompt(t.Context(), &sdkmcp.GetPromptParams{
		Name:      "market_analysis",
		Arguments: map[string]string{"symbol": "BTCUSDT", "interval": "1h"},
	})
	if err != nil {
		t.Fatalf("GetPrompt(market_analysis) error = %v", err)
	}
	if len(prompt.Messages) == 0 {
		t.Fatalf("market_analysis returned no messages")
	}
}

func TestFileAuditSinkWritesJSONLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	sink, err := NewFileAuditSink(path)
	if err != nil {
		t.Fatalf("NewFileAuditSink() error = %v", err)
	}
	event := AuditEvent{
		Tool:       "get_positions",
		Arguments:  map[string]any{"symbol": "BTCUSDT"},
		DurationMS: 12,
		Success:    true,
	}
	if err := sink.Record(context.Background(), event); err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"tool":"get_positions"`) || !strings.Contains(text, `"duration_ms":12`) {
		t.Fatalf("audit log=%s", text)
	}
}

type stubRuntime struct {
	observe         botruntime.ObserveResponse
	observeErr      error
	latestDecision  botruntime.DecisionLatestResponse
	positions       botruntime.PositionStatusResponse
	decisionHistory botruntime.DashboardDecisionHistoryResponse
	accountSummary  botruntime.DashboardAccountSummaryResponse
	kline           botruntime.DashboardKlineResponse
	overview        botruntime.DashboardOverviewResponse
}

func (s stubRuntime) FetchObserveReport(context.Context, string) (botruntime.ObserveResponse, error) {
	return s.observe, s.observeErr
}

func (s stubRuntime) FetchDecisionLatest(context.Context, string) (botruntime.DecisionLatestResponse, error) {
	return s.latestDecision, nil
}

func (s stubRuntime) FetchPositionStatus(context.Context) (botruntime.PositionStatusResponse, error) {
	return s.positions, nil
}

func (s stubRuntime) FetchDashboardDecisionHistory(context.Context, string, int, uint) (botruntime.DashboardDecisionHistoryResponse, error) {
	return s.decisionHistory, nil
}

func (s stubRuntime) FetchDashboardAccountSummary(context.Context) (botruntime.DashboardAccountSummaryResponse, error) {
	return s.accountSummary, nil
}

func (s stubRuntime) FetchDashboardKline(context.Context, string, string, int) (botruntime.DashboardKlineResponse, error) {
	return s.kline, nil
}

func (s stubRuntime) FetchDashboardOverview(context.Context, string) (botruntime.DashboardOverviewResponse, error) {
	return s.overview, nil
}

type memoryAuditSink struct {
	events []AuditEvent
}

func (s *memoryAuditSink) Record(_ context.Context, event AuditEvent) error {
	s.events = append(s.events, event)
	return nil
}

func connectTestSession(t *testing.T, server *sdkmcp.Server) *sdkmcp.ClientSession {
	t.Helper()
	ctx := t.Context()
	clientTransport, serverTransport := sdkmcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	return session
}

func callToolMap(t *testing.T, session *sdkmcp.ClientSession, name string, args map[string]any) map[string]any {
	t.Helper()
	res, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%s) error = %v", name, err)
	}
	if res.IsError {
		var text string
		for _, item := range res.Content {
			if content, ok := item.(*sdkmcp.TextContent); ok {
				text = content.Text
				break
			}
		}
		t.Fatalf("CallTool(%s) returned tool error: %s", name, text)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("Marshal structured content: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("Unmarshal structured content: %v", err)
	}
	return out
}

func listResourceURIs(t *testing.T, session *sdkmcp.ClientSession) []string {
	t.Helper()
	var out []string
	for resource, err := range session.Resources(t.Context(), nil) {
		if err != nil {
			t.Fatalf("Resources() error = %v", err)
		}
		out = append(out, resource.URI)
	}
	return out
}

func listResourceTemplates(t *testing.T, session *sdkmcp.ClientSession) []string {
	t.Helper()
	var out []string
	for resource, err := range session.ResourceTemplates(t.Context(), nil) {
		if err != nil {
			t.Fatalf("ResourceTemplates() error = %v", err)
		}
		out = append(out, resource.URITemplate)
	}
	return out
}

func listPromptNames(t *testing.T, session *sdkmcp.ClientSession) []string {
	t.Helper()
	var out []string
	for prompt, err := range session.Prompts(t.Context(), nil) {
		if err != nil {
			t.Fatalf("Prompts() error = %v", err)
		}
		out = append(out, prompt.Name)
	}
	return out
}

func readResourceMap(t *testing.T, session *sdkmcp.ClientSession, uri string) map[string]any {
	t.Helper()
	res, err := session.ReadResource(t.Context(), &sdkmcp.ReadResourceParams{URI: uri})
	if err != nil {
		t.Fatalf("ReadResource(%s) error = %v", uri, err)
	}
	if len(res.Contents) != 1 {
		t.Fatalf("ReadResource(%s) contents=%d want 1", uri, len(res.Contents))
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.Contents[0].Text), &out); err != nil {
		t.Fatalf("Unmarshal resource %s: %v", uri, err)
	}
	return out
}

func containsTestString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func writeMCPConfigTree(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	systemPath := filepath.Join(dir, "system.toml")
	indexPath := filepath.Join(dir, "symbols-index.toml")
	symbolDir := filepath.Join(dir, "symbols")
	strategyDir := filepath.Join(dir, "strategies")
	if err := os.MkdirAll(symbolDir, 0o755); err != nil {
		t.Fatalf("mkdir symbols: %v", err)
	}
	if err := os.MkdirAll(strategyDir, 0o755); err != nil {
		t.Fatalf("mkdir strategies: %v", err)
	}
	writeTestFile(t, systemPath, `
db_path = "`+filepath.Join(dir, "brale.db")+`"
execution_system = "freqtrade"
exec_endpoint = "http://localhost:8080"
exec_api_key = "top-secret-key"
exec_api_secret = "top-secret-secret"
exec_auth = "token"
log_level = "info"

[llm_models.mock]
endpoint = "http://localhost:11434/v1"
api_key = "dummy-secret"

[notification.telegram]
enabled = true
token = "telegram-secret"
chat_id = 42
`)
	writeTestFile(t, indexPath, `
[[symbols]]
symbol = "BTCUSDT"
config = "symbols/BTCUSDT.toml"
strategy = "strategies/BTCUSDT.toml"
`)
	writeTestFile(t, filepath.Join(symbolDir, "BTCUSDT.toml"), `
symbol = "BTCUSDT"
intervals = ["15m", "1h", "4h"]
kline_limit = 260

[agent]
indicator = true
structure = true
mechanics = true

[indicators]
shadow_engine = "reference"
ema_fast = 21
ema_mid = 50
ema_slow = 200
rsi_period = 14
atr_period = 14
stc_fast = 23
stc_slow = 50
bb_period = 20
bb_multiplier = 2.0
chop_period = 14
stoch_rsi_period = 14
aroon_period = 25
last_n = 5

[memory]
enabled = true
working_memory_size = 5
episodic_enabled = true
episodic_ttl_days = 30
episodic_max_per_symbol = 4
semantic_enabled = true
semantic_max_rules = 8

[consensus]
score_threshold = 0.35
confidence_threshold = 0.52

[cooldown]
enabled = false

[llm.agent.indicator]
model = "mock"
temperature = 0.2
[llm.agent.structure]
model = "mock"
temperature = 0.1
[llm.agent.mechanics]
model = "mock"
temperature = 0.2
[llm.provider.indicator]
model = "mock"
temperature = 0.2
[llm.provider.structure]
model = "mock"
temperature = 0.1
[llm.provider.mechanics]
model = "mock"
temperature = 0.2
`)
	writeTestFile(t, filepath.Join(strategyDir, "BTCUSDT.toml"), `
symbol = "BTCUSDT"
id = "default-BTCUSDT"
rule_chain = "configs/rules/default.json"

[risk_management]
risk_per_trade_pct = 0.01
max_invest_pct = 1.0
max_leverage = 3.0
grade_1_factor = 0.3
grade_2_factor = 0.6
grade_3_factor = 1.0
entry_offset_atr = 0.1
entry_mode = "atr_offset"
orderbook_depth = 5
breakeven_fee_pct = 0.0
slippage_buffer_pct = 0.0005

[risk_management.risk_strategy]
mode = "native"

[risk_management.initial_exit]
policy = "atr_structure_v1"
structure_interval = "auto"

[risk_management.initial_exit.params]
stop_atr_multiplier = 2.0
stop_min_distance_pct = 0.005
take_profit_rr = [1.5, 3.0]

[risk_management.tighten_atr]
structure_threatened = 0.5
tp1_atr = 0.0
tp2_atr = 0.0
min_tp_distance_pct = 0.0
min_tp_gap_pct = 0.0
min_update_interval_sec = 300

[risk_management.gate]
quality_threshold = 0.35
edge_threshold = 0.1

[risk_management.sieve]
min_size_factor = 0.1
default_gate_action = "ALLOW"
default_size_factor = 1.0
`)
	return systemPath, indexPath
}

func indicatorTestCandles(n int) []runtimeapi.DashboardCandle {
	out := make([]runtimeapi.DashboardCandle, 0, n)
	base := time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC)
	price := 100.0
	for i := 0; i < n; i++ {
		openTime := base.Add(time.Duration(i) * time.Hour)
		out = append(out, runtimeapi.DashboardCandle{
			OpenTime:  openTime.UnixMilli(),
			CloseTime: openTime.Add(time.Hour).UnixMilli(),
			Open:      price,
			High:      price + 1.2,
			Low:       price - 0.8,
			Close:     price + 0.5,
			Volume:    1000 + float64(i),
		})
		price += 0.4
	}
	return out
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
