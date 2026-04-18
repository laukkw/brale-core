package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	resourceConfigSystemURI        = "brale://config/system"
	resourceConfigSymbolsURI       = "brale://config/symbols"
	resourceDecisionLatestTemplate = "brale://decision/{symbol}/latest"
	resourceKlineTemplate          = "brale://kline/{symbol}/{interval}"
	promptMarketAnalysis           = "market_analysis"
	promptTradeReview              = "trade_review"
	defaultPromptKlineInterval     = "1h"
	defaultResourceKlineLimit      = 120
	auditResourceConfigSystem      = "resource:config_system"
	auditResourceConfigSymbols     = "resource:config_symbols"
	auditResourceDecisionLatest    = "resource:decision_latest"
	auditResourceKline             = "resource:kline"
	auditPromptMarketAnalysis      = "prompt:market_analysis"
	auditPromptTradeReview         = "prompt:trade_review"
)

func registerResources(server *sdkmcp.Server, audit AuditSink, svc service) {
	server.AddResource(&sdkmcp.Resource{
		Name:        "config_system",
		Title:       "System Config",
		URI:         resourceConfigSystemURI,
		MIMEType:    "application/json",
		Description: "脱敏后的 system.toml 配置。",
	}, auditedResource(auditResourceConfigSystem, audit, svc.handleConfigSystemResource))
	server.AddResource(&sdkmcp.Resource{
		Name:        "config_symbols",
		Title:       "Symbol Index",
		URI:         resourceConfigSymbolsURI,
		MIMEType:    "application/json",
		Description: "symbols-index.toml 中的交易对索引与配置路径。",
	}, auditedResource(auditResourceConfigSymbols, audit, svc.handleConfigSymbolsResource))
	server.AddResourceTemplate(&sdkmcp.ResourceTemplate{
		Name:        "decision_latest",
		Title:       "Latest Decision",
		URITemplate: resourceDecisionLatestTemplate,
		MIMEType:    "application/json",
		Description: "指定交易对的最新决策结果。",
	}, auditedResource(auditResourceDecisionLatest, audit, svc.handleDecisionLatestResource))
	server.AddResourceTemplate(&sdkmcp.ResourceTemplate{
		Name:        "kline",
		Title:       "Kline Snapshot",
		URITemplate: resourceKlineTemplate,
		MIMEType:    "application/json",
		Description: "指定交易对与周期的 K 线数据（默认返回最近 120 根）。",
	}, auditedResource(auditResourceKline, audit, svc.handleKlineResource))
}

func registerPrompts(server *sdkmcp.Server, audit AuditSink, svc service) {
	server.AddPrompt(&sdkmcp.Prompt{
		Name:        promptMarketAnalysis,
		Title:       "Market Analysis",
		Description: "标准化市场分析框架，指导客户端读取资源与只读工具。",
		Arguments: []*sdkmcp.PromptArgument{
			{Name: "symbol", Description: "交易对，例如 BTCUSDT", Required: true},
			{Name: "interval", Description: "观察周期，例如 1h"},
		},
	}, auditedPrompt(auditPromptMarketAnalysis, audit, svc.handleMarketAnalysisPrompt))
	server.AddPrompt(&sdkmcp.Prompt{
		Name:        promptTradeReview,
		Title:       "Trade Review",
		Description: "标准化仓位/决策复盘模板。",
		Arguments: []*sdkmcp.PromptArgument{
			{Name: "symbol", Description: "交易对，例如 BTCUSDT", Required: true},
			{Name: "interval", Description: "观察周期，例如 1h"},
		},
	}, auditedPrompt(auditPromptTradeReview, audit, svc.handleTradeReviewPrompt))
}

func (s service) handleConfigSystemResource(_ context.Context, req *sdkmcp.ReadResourceRequest) (*sdkmcp.ReadResourceResult, error) {
	view, err := s.config.LoadConfigView("")
	if err != nil {
		return nil, err
	}
	return jsonResource(req.Params.URI, view.System)
}

func (s service) handleConfigSymbolsResource(_ context.Context, req *sdkmcp.ReadResourceRequest) (*sdkmcp.ReadResourceResult, error) {
	view, err := s.config.LoadConfigView("")
	if err != nil {
		return nil, err
	}
	return jsonResource(req.Params.URI, map[string]any{
		"system_path": view.SystemPath,
		"index_path":  view.IndexPath,
		"index":       buildIndexEntries(view.Index),
	})
}

func (s service) handleDecisionLatestResource(ctx context.Context, req *sdkmcp.ReadResourceRequest) (*sdkmcp.ReadResourceResult, error) {
	symbol, err := parseDecisionLatestResourceURI(req.Params.URI)
	if err != nil {
		return nil, err
	}
	out, err := s.runtime.FetchDecisionLatest(ctx, symbol)
	if err != nil {
		return nil, err
	}
	return jsonResource(req.Params.URI, out)
}

func (s service) handleKlineResource(ctx context.Context, req *sdkmcp.ReadResourceRequest) (*sdkmcp.ReadResourceResult, error) {
	symbol, interval, err := parseKlineResourceURI(req.Params.URI)
	if err != nil {
		return nil, err
	}
	out, err := s.runtime.FetchDashboardKline(ctx, symbol, interval, defaultResourceKlineLimit)
	if err != nil {
		return nil, err
	}
	return jsonResource(req.Params.URI, out)
}

func (s service) handleMarketAnalysisPrompt(_ context.Context, req *sdkmcp.GetPromptRequest) (*sdkmcp.GetPromptResult, error) {
	symbol, interval, err := promptSymbolInterval(req)
	if err != nil {
		return nil, err
	}
	text := fmt.Sprintf(
		"请基于 brale-core 当前可读数据，对 %s 做一份结构化市场分析。\n"+
			"优先读取以下资源：\n"+
			"- brale://decision/%s/latest\n"+
			"- brale://kline/%s/%s\n"+
			"必要时调用以下只读工具补充上下文：analyze_market、get_kline、get_decision_history、get_config。\n"+
			"输出建议结构：\n"+
			"1. 当前市场状态\n"+
			"2. 最近决策与其核心理由\n"+
			"3. 关键风险与失效条件\n"+
			"4. 接下来最值得验证的信号",
		symbol, symbol, symbol, interval,
	)
	return promptTextResult("标准化市场分析框架。", text), nil
}

func (s service) handleTradeReviewPrompt(_ context.Context, req *sdkmcp.GetPromptRequest) (*sdkmcp.GetPromptResult, error) {
	symbol, interval, err := promptSymbolInterval(req)
	if err != nil {
		return nil, err
	}
	text := fmt.Sprintf(
		"请复盘 %s 最近的决策与市场演变，并总结哪些判断应保留、哪些判断应修正。\n"+
			"优先读取以下资源：\n"+
			"- brale://decision/%s/latest\n"+
			"- brale://kline/%s/%s\n"+
			"必要时调用以下只读工具补充：get_decision_history、get_episodic_memory、analyze_market、get_config。\n"+
			"输出建议结构：\n"+
			"1. 最近决策是否与当前市场结构匹配\n"+
			"2. 若判断失真，更接近指标/结构/机制哪一层失真\n"+
			"3. 应保留的规则与证据\n"+
			"4. 下一轮继续观察的关键信号",
		symbol, symbol, symbol, interval,
	)
	return promptTextResult("标准化交易/决策复盘模板。", text), nil
}

func promptSymbolInterval(req *sdkmcp.GetPromptRequest) (string, string, error) {
	args := map[string]string{}
	if req != nil && req.Params.Arguments != nil {
		args = req.Params.Arguments
	}
	symbol, err := normalizeSymbol(args["symbol"])
	if err != nil {
		return "", "", err
	}
	interval := strings.TrimSpace(args["interval"])
	if interval == "" {
		interval = defaultPromptKlineInterval
	}
	return symbol, interval, nil
}

func promptTextResult(description, text string) *sdkmcp.GetPromptResult {
	return &sdkmcp.GetPromptResult{
		Description: description,
		Messages: []*sdkmcp.PromptMessage{
			{
				Role:    "user",
				Content: &sdkmcp.TextContent{Text: text},
			},
		},
	}
}

func parseDecisionLatestResourceURI(raw string) (string, error) {
	uri, parts, err := parseBraleURI(raw)
	if err != nil {
		return "", err
	}
	if uri.Host != "decision" || len(parts) != 2 || !strings.EqualFold(parts[1], "latest") {
		return "", sdkmcp.ResourceNotFoundError(raw)
	}
	return normalizeSymbol(parts[0])
}

func parseKlineResourceURI(raw string) (string, string, error) {
	uri, parts, err := parseBraleURI(raw)
	if err != nil {
		return "", "", err
	}
	if uri.Host != "kline" || len(parts) != 2 {
		return "", "", sdkmcp.ResourceNotFoundError(raw)
	}
	symbol, err := normalizeSymbol(parts[0])
	if err != nil {
		return "", "", err
	}
	interval := strings.TrimSpace(parts[1])
	if interval == "" {
		return "", "", sdkmcp.ResourceNotFoundError(raw)
	}
	return symbol, interval, nil
}

func parseBraleURI(raw string) (*url.URL, []string, error) {
	uri, err := url.Parse(raw)
	if err != nil {
		return nil, nil, sdkmcp.ResourceNotFoundError(raw)
	}
	if uri.Scheme != "brale" {
		return nil, nil, sdkmcp.ResourceNotFoundError(raw)
	}
	parts := strings.Split(strings.Trim(strings.TrimSpace(uri.Path), "/"), "/")
	if len(parts) == 1 && parts[0] == "" {
		parts = nil
	}
	return uri, parts, nil
}

func jsonResource(uri string, payload any) (*sdkmcp.ReadResourceResult, error) {
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal resource output: %w", err)
	}
	return &sdkmcp.ReadResourceResult{
		Contents: []*sdkmcp.ResourceContents{
			{
				URI:      uri,
				MIMEType: "application/json",
				Text:     string(raw),
			},
		},
	}, nil
}

func auditedResource(name string, audit AuditSink, next sdkmcp.ResourceHandler) sdkmcp.ResourceHandler {
	return func(ctx context.Context, req *sdkmcp.ReadResourceRequest) (*sdkmcp.ReadResourceResult, error) {
		start := time.Now()
		res, err := next(ctx, req)
		if auditErr := recordAudit(ctx, audit, AuditEvent{
			At:         time.Now().UTC(),
			Tool:       name,
			Arguments:  req.Params.URI,
			DurationMS: time.Since(start).Milliseconds(),
			Success:    err == nil,
			Error:      errorText(err),
		}); auditErr != nil {
			if err != nil {
				return nil, fmt.Errorf("%v (audit: %w)", err, auditErr)
			}
			return nil, fmt.Errorf("record audit event: %w", auditErr)
		}
		return res, err
	}
}

func auditedPrompt(name string, audit AuditSink, next sdkmcp.PromptHandler) sdkmcp.PromptHandler {
	return func(ctx context.Context, req *sdkmcp.GetPromptRequest) (*sdkmcp.GetPromptResult, error) {
		start := time.Now()
		res, err := next(ctx, req)
		args := map[string]string(nil)
		if req != nil && req.Params.Arguments != nil {
			args = req.Params.Arguments
		}
		if auditErr := recordAudit(ctx, audit, AuditEvent{
			At:         time.Now().UTC(),
			Tool:       name,
			Arguments:  args,
			DurationMS: time.Since(start).Milliseconds(),
			Success:    err == nil,
			Error:      errorText(err),
		}); auditErr != nil {
			if err != nil {
				return nil, fmt.Errorf("%v (audit: %w)", err, auditErr)
			}
			return nil, fmt.Errorf("record audit event: %w", auditErr)
		}
		return res, err
	}
}

func recordAudit(ctx context.Context, audit AuditSink, event AuditEvent) error {
	if audit == nil {
		return nil
	}
	return audit.Record(ctx, event)
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
