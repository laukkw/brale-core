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

	"brale-core/internal/pkg/httpclient"
	"brale-core/internal/pkg/logging"

	"go.uber.org/zap"
)

type OpenAIClient struct {
	Endpoint    string
	Model       string
	APIKey      string
	Timeout     time.Duration
	Temperature float64
}

type chatRequest struct {
	Model          string            `json:"model"`
	Messages       []chatMessage     `json:"messages"`
	MaxTokens      int               `json:"max_tokens"`
	ResponseFormat map[string]string `json:"response_format,omitempty"`
	Temperature    float64           `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (c *OpenAIClient) Call(ctx context.Context, system, user string) (string, error) {
	return c.callWithMessages(ctx, []chatMessage{{Role: "system", Content: system}, {Role: "user", Content: user}})
}

func (c *OpenAIClient) callWithMessages(ctx context.Context, messages []chatMessage) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	logger := logging.FromContext(ctx).Named("llm").With(zap.String("model", c.Model))
	start := time.Now()
	endpoint := strings.TrimRight(c.Endpoint, "/")
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1"
	}
	logger = logger.With(zap.String("endpoint", endpoint))

	release, err := defaultModelGates.Acquire(ctx, c.Model)
	if err != nil {
		logger.Error("llm acquire gate failed", zap.Error(err))
		return "", err
	}
	defer release()

	logger.Debug("llm request", zap.Int("messages", len(messages)))
	url := endpoint + "/chat/completions"
	payload := chatRequest{
		Model:          c.Model,
		Messages:       messages,
		MaxTokens:      4096,
		Temperature:    c.Temperature,
		ResponseFormat: map[string]string{"type": "json_object"},
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
	for attempt := 0; attempt < 3; attempt++ {
		output, retryAfter, err := c.callOnce(ctx, url, raw, timeout)
		if err == nil {
			logger.Debug("llm response", zap.String("output", output), zap.Duration("latency", time.Since(start)))
			return output, nil
		}
		lastErr = err
		if retryAfter > 0 && attempt < 2 {
			until := time.Now().Add(retryAfter)
			defaultModelGates.SetCooldown(c.Model, until)
			if err := sleepWithContext(ctx, retryAfter); err != nil {
				logger.Error("llm retry wait cancelled", zap.Error(err))
				return "", err
			}
			continue
		}
		break
	}

	logger.Error("llm request failed", zap.Error(lastErr), zap.Duration("latency", time.Since(start)))
	return "", lastErr
}

func (c *OpenAIClient) callOnce(ctx context.Context, url string, raw []byte, timeout time.Duration) (string, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(c.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.APIKey))
	}
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return "", 0, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	bodyBytes, _ := httpclient.ReadLimitedBody(resp.Body, 4*1024*1024)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyText := strings.TrimSpace(string(bodyBytes))
		if resp.StatusCode == http.StatusTooManyRequests {
			return "", parseRetryWait(resp.Header), fmt.Errorf("status %s: %s", resp.Status, bodyText)
		}
		if bodyText == "" {
			return "", 0, fmt.Errorf("status %s", resp.Status)
		}
		return "", 0, fmt.Errorf("status %s: %s", resp.Status, bodyText)
	}

	var parsed chatResponse
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return "", 0, err
	}
	if len(parsed.Choices) == 0 {
		return "", 0, fmt.Errorf("empty choices")
	}
	return parsed.Choices[0].Message.Content, 0, nil
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
