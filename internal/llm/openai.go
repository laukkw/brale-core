// 本文件主要内容：封装 OpenAI Chat Completions 调用。

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	braleOtel "brale-core/internal/otel"
	"brale-core/internal/pkg/httpclient"
	"brale-core/internal/pkg/logging"

	"github.com/hashicorp/go-retryablehttp"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

type OpenAIClient struct {
	Endpoint         string
	Model            string
	APIKey           string
	HTTPClient       *http.Client
	Timeout          time.Duration
	Temperature      float64
	StructuredOutput bool
	Breaker          *CircuitBreaker
}

type chatRequest struct {
	Model          string        `json:"model"`
	Messages       []ChatMessage `json:"messages"`
	MaxTokens      int           `json:"max_tokens"`
	ResponseFormat any           `json:"response_format,omitempty"`
	Temperature    float64       `json:"temperature"`
}

// ChatMessage represents a single message in an LLM conversation.
// Exported so that external packages (reflection, MCP interaction) can
// build multi-turn message histories.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

type callResult struct {
	Content  string
	Model    string
	TokenIn  int
	TokenOut int
}

func (c *OpenAIClient) Call(ctx context.Context, system, user string) (string, error) {
	messages := []ChatMessage{{Role: "system", Content: system}, {Role: "user", Content: user}}
	return c.doCall(ctx, messages, jsonObjectFormat())
}

func (c *OpenAIClient) CallStructured(ctx context.Context, system, user string, schema *JSONSchema) (string, error) {
	if !c.StructuredOutput {
		return c.Call(ctx, system, user)
	}
	messages := []ChatMessage{{Role: "system", Content: system}, {Role: "user", Content: user}}
	return c.doCall(ctx, messages, jsonSchemaFormat(schema))
}

// CallMultiTurn accepts a pre-built message history for multi-turn
// conversations. Intended for reflection, interactive CLI, and MCP
// workflows — not for the main trading decision pipeline.
func (c *OpenAIClient) CallMultiTurn(ctx context.Context, messages []ChatMessage) (string, error) {
	return c.doCall(ctx, messages, jsonObjectFormat())
}

func (c *OpenAIClient) doCall(ctx context.Context, messages []ChatMessage, responseFormat any) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	logger := logging.FromContext(ctx).Named("llm").With(zap.String("model", c.Model))
	start := time.Now()
	endpoint := strings.TrimRight(c.Endpoint, "/")
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1"
	}
	meta, _ := CallMetadataFromContext(ctx)
	logger = logger.With(zap.String("endpoint", endpoint))
	ctx, span := braleOtel.Tracer("brale-core/llm").Start(ctx, "llm.call")
	span.SetAttributes(
		attribute.String("llm.model", c.Model),
		attribute.String("llm.endpoint", endpoint),
		attribute.String("llm.role", meta.Role),
		attribute.String("llm.stage", meta.Stage),
		attribute.String("llm.symbol", meta.Symbol),
		attribute.String("llm.prompt_version", meta.PromptVersion),
	)
	defer span.End()

	breaker := c.Breaker
	if breaker == nil {
		breaker = defaultCircuitBreakers.get(endpoint + "|" + c.Model)
	}
	if err := breaker.Allow(); err != nil {
		emitCallStats(ctx, endpoint, meta, CallStats{Model: c.Model, Endpoint: endpoint, Role: meta.Role, Stage: meta.Stage, Symbol: meta.Symbol, PromptVersion: meta.PromptVersion, Err: err})
		logger.Error("llm circuit breaker open", zap.Error(err))
		return "", err
	}
	release, err := defaultModelGates.Acquire(ctx, c.Model)
	if err != nil {
		emitCallStats(ctx, endpoint, meta, CallStats{Model: c.Model, Endpoint: endpoint, Role: meta.Role, Stage: meta.Stage, Symbol: meta.Symbol, PromptVersion: meta.PromptVersion, Err: err})
		logger.Error("llm acquire gate failed", zap.Error(err))
		return "", err
	}
	defer release()

	logger.Info("llm request started", zap.Int("messages", len(messages)))
	url := endpoint + "/chat/completions"
	payload := chatRequest{
		Model:          c.Model,
		Messages:       messages,
		MaxTokens:      4096,
		Temperature:    c.Temperature,
		ResponseFormat: responseFormat,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 300 * time.Second
	}

	var lastErr error
	var result callResult
	for attempt := 0; attempt < 3; attempt++ {
		output, retryAfter, err := c.callOnce(ctx, url, raw, timeout)
		if err == nil {
			result = output
			latencyMs := time.Since(start).Milliseconds()
			emitCallStats(ctx, endpoint, meta, CallStats{
				Model:         chooseModel(result.Model, c.Model),
				Endpoint:      endpoint,
				Role:          meta.Role,
				Stage:         meta.Stage,
				Symbol:        meta.Symbol,
				PromptVersion: meta.PromptVersion,
				LatencyMs:     latencyMs,
				TokenIn:       result.TokenIn,
				TokenOut:      result.TokenOut,
			})
			breaker.RecordSuccess()
			logger.Info("llm request completed", zap.Duration("latency", time.Since(start)), zap.Int("token_in", result.TokenIn), zap.Int("token_out", result.TokenOut))
			return result.Content, nil
		}
		lastErr = err
		if retryAfter > 0 && attempt < 2 {
			until := time.Now().Add(retryAfter)
			defaultModelGates.SetCooldown(c.Model, until)
			logger.Warn("llm rate limited, retrying",
				zap.Int("attempt", attempt+1),
				zap.Duration("retry_after", retryAfter),
				zap.Error(err),
			)
			if err := sleepWithContext(ctx, retryAfter); err != nil {
				logger.Error("llm retry wait cancelled", zap.Int("attempt", attempt+1), zap.Error(err))
				return "", err
			}
			continue
		}
		break
	}

	emitCallStats(ctx, endpoint, meta, CallStats{
		Model:         c.Model,
		Endpoint:      endpoint,
		Role:          meta.Role,
		Stage:         meta.Stage,
		Symbol:        meta.Symbol,
		PromptVersion: meta.PromptVersion,
		LatencyMs:     time.Since(start).Milliseconds(),
		Err:           lastErr,
	})
	breaker.RecordFailure()
	logger.Error("llm request failed", zap.Error(lastErr), zap.Duration("latency", time.Since(start)))
	return "", lastErr
}

func jsonObjectFormat() any {
	return map[string]string{"type": "json_object"}
}

func jsonSchemaFormat(schema *JSONSchema) any {
	if schema == nil {
		return jsonObjectFormat()
	}
	return map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   schema.Name,
			"strict": true,
			"schema": schema.Schema,
		},
	}
}

func (c *OpenAIClient) callOnce(ctx context.Context, url string, raw []byte, timeout time.Duration) (callResult, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return callResult{}, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(c.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.APIKey))
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	} else if client.Timeout <= 0 {
		cloned := *client
		cloned.Timeout = timeout
		client = &cloned
	}
	retryClient := newLLMRetryClient(client)
	retryReq, err := retryablehttp.FromRequest(req)
	if err != nil {
		return callResult{}, 0, err
	}
	resp, err := retryClient.Do(retryReq)
	if err != nil {
		return callResult{}, 0, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	bodyBytes, err := httpclient.ReadLimitedBody(resp.Body, 4*1024*1024)
	if err != nil {
		return callResult{}, 0, fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyText := strings.TrimSpace(string(bodyBytes))
		if resp.StatusCode == http.StatusTooManyRequests {
			return callResult{}, parseRetryWait(resp.Header), fmt.Errorf("status %s: %s", resp.Status, bodyText)
		}
		if bodyText == "" {
			return callResult{}, 0, fmt.Errorf("status %s", resp.Status)
		}
		return callResult{}, 0, fmt.Errorf("status %s: %s", resp.Status, bodyText)
	}

	var parsed chatResponse
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return callResult{}, 0, err
	}
	if len(parsed.Choices) == 0 {
		return callResult{}, 0, fmt.Errorf("empty choices")
	}
	return callResult{
		Content:  parsed.Choices[0].Message.Content,
		Model:    parsed.Model,
		TokenIn:  parsed.Usage.PromptTokens,
		TokenOut: parsed.Usage.CompletionTokens,
	}, 0, nil
}

func chooseModel(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}

func emitCallStats(ctx context.Context, endpoint string, meta CallMetadata, stats CallStats) {
	stats.Endpoint = endpoint
	ObserveCall(ctx, stats)
	attrs := []attribute.KeyValue{
		attribute.String("model", chooseModel(stats.Model, "")),
		attribute.String("role", meta.Role),
		attribute.String("stage", meta.Stage),
		attribute.String("symbol", meta.Symbol),
		attribute.String("prompt_version", meta.PromptVersion),
	}
	options := []otelmetric.RecordOption{otelmetric.WithAttributes(attrs...)}
	braleOtel.LLMCallLatencyMs.Record(ctx, stats.LatencyMs, options...)
	if stats.TokenIn > 0 {
		braleOtel.LLMCallTokenIn.Add(ctx, int64(stats.TokenIn), otelmetric.WithAttributes(attrs...))
	}
	if stats.TokenOut > 0 {
		braleOtel.LLMCallTokenOut.Add(ctx, int64(stats.TokenOut), otelmetric.WithAttributes(attrs...))
	}
	if stats.Err != nil {
		braleOtel.LLMCallErrors.Add(ctx, 1, otelmetric.WithAttributes(attrs...))
	}
}

func newLLMRetryClient(base *http.Client) *retryablehttp.Client {
	client := retryablehttp.NewClient()
	client.RetryMax = 2
	client.RetryWaitMin = 200 * time.Millisecond
	client.RetryWaitMax = 2 * time.Second
	client.Logger = logging.RetryableHTTPLogger{Logger: logging.L().Named("llm-retry")}
	client.ErrorHandler = retryablehttp.PassthroughErrorHandler
	client.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		if err != nil {
			return true, nil
		}
		if resp == nil {
			return false, nil
		}
		switch resp.StatusCode {
		case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true, nil
		case http.StatusTooManyRequests:
			return false, nil
		default:
			return false, nil
		}
	}
	if base != nil {
		client.HTTPClient = base
	}
	return client
}

func parseRetryWait(h http.Header) time.Duration {
	if h == nil {
		return 2 * time.Second
	}
	if d := parseWaitHeader(h.Get("x-ratelimit-reset-requests")); d > 0 {
		return d
	}
	if d := parseWaitHeader(h.Get("Retry-After")); d > 0 {
		return d
	}
	return 2 * time.Second
}

func parseWaitHeader(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if d, err := time.ParseDuration(raw); err == nil {
		return d
	}
	if sec, err := strconv.Atoi(raw); err == nil {
		return time.Duration(sec) * time.Second
	}
	return 0
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
