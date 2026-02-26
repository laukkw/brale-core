package gdelt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"brale-core/internal/pkg/httpclient"
)

const defaultDocBaseURL = "https://api.gdeltproject.org/api/v2/doc/doc"

const (
	defaultClientTimeout         = 60 * time.Second
	defaultTLSHandshakeTimeout   = 40 * time.Second
	defaultResponseHeaderTimeout = 60 * time.Second
	defaultIdleConnTimeout       = 360 * time.Second
	defaultMaxIdleConnsPerHost   = 20
)

var defaultHTTPClient = &http.Client{
	Timeout: defaultClientTimeout,
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   defaultMaxIdleConnsPerHost,
		IdleConnTimeout:       defaultIdleConnTimeout,
		TLSHandshakeTimeout:   defaultTLSHandshakeTimeout,
		ResponseHeaderTimeout: defaultResponseHeaderTimeout,
		ExpectContinueTimeout: 1 * time.Second,
	},
}

type Client struct {
	BaseURL      string
	HTTPClient   *http.Client
	MaxBodyBytes int64

	ThrottleInterval time.Duration

	mu          sync.Mutex
	nextAllowed time.Time
}

type RateLimitError struct {
	StatusCode int
	Body       string
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("gdelt rate limited: %d", e.StatusCode)
}

type Article struct {
	Title  string
	URL    string
	Domain string
	SeenAt time.Time
}

type responsePayload struct {
	Articles []struct {
		Title    string `json:"title"`
		URL      string `json:"url"`
		Domain   string `json:"domain"`
		SeenDate string `json:"seendate"`
		Source   string `json:"sourcecountry"`
	} `json:"articles"`
}

func (c *Client) FetchArticles(ctx context.Context, query string, timespan string, maxRecords int) ([]Article, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query is required")
	}
	if strings.TrimSpace(timespan) == "" {
		return nil, fmt.Errorf("timespan is required")
	}
	if maxRecords <= 0 {
		return nil, fmt.Errorf("maxrecords must be > 0")
	}
	candidates := expandTimespanCandidates(timespan)
	var errs []string
	for _, candidate := range candidates {
		articles, err := c.fetchArticlesOnce(ctx, query, candidate, maxRecords)
		if err == nil {
			return articles, nil
		}
		var rle *RateLimitError
		if errors.As(err, &rle) {
			return nil, err
		}
		errs = append(errs, fmt.Sprintf("timespan=%s: %v", candidate, err))
	}
	if len(errs) == 0 {
		return nil, fmt.Errorf("gdelt fetch failed")
	}
	if len(errs) == 1 {
		return nil, fmt.Errorf("gdelt fetch failed: %s", errs[0])
	}
	return nil, fmt.Errorf("gdelt fetch failed: %s; %s", errs[0], errs[1])
}

func (c *Client) fetchArticlesOnce(ctx context.Context, query string, timespan string, maxRecords int) ([]Article, error) {
	if err := c.throttle(ctx); err != nil {
		return nil, err
	}

	baseURL := strings.TrimSpace(c.BaseURL)
	if baseURL == "" {
		baseURL = defaultDocBaseURL
	}
	params := url.Values{}
	params.Set("query", query)
	params.Set("timespan", timespan)
	params.Set("mode", "artlist")
	params.Set("maxrecords", strconv.Itoa(maxRecords))
	params.Set("format", "json")
	params.Set("sort", "date")
	endpoint := baseURL + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "brale-core/news-overlay")
	client := c.HTTPClient
	if client == nil {
		client = defaultHTTPClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := httpclient.ReadLimitedBody(resp.Body, 64*1024)
		body := strings.TrimSpace(string(bodyBytes))

		if resp.StatusCode == 429 {
			return nil, &RateLimitError{StatusCode: resp.StatusCode, Body: body}
		}

		if body == "" {
			return nil, fmt.Errorf("gdelt status %s", resp.Status)
		}
		return nil, fmt.Errorf("gdelt status %s: %s", resp.Status, body)
	}
	limit := c.MaxBodyBytes
	if limit <= 0 {
		limit = 4 * 1024 * 1024
	}
	bodyBytes, err := httpclient.ReadLimitedBody(resp.Body, limit)
	if err != nil {
		return nil, err
	}
	var payload responsePayload
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		body := abbreviateBody(bodyBytes, 240)
		return nil, fmt.Errorf("invalid json response: %w; body=%q", err, body)
	}
	out := make([]Article, 0, len(payload.Articles))
	for _, item := range payload.Articles {
		title := strings.TrimSpace(item.Title)
		link := strings.TrimSpace(item.URL)
		if title == "" || link == "" {
			continue
		}
		out = append(out, Article{
			Title:  title,
			URL:    link,
			Domain: strings.TrimSpace(item.Domain),
			SeenAt: parseSeenDate(item.SeenDate),
		})
	}
	return out, nil
}

func (c *Client) throttle(ctx context.Context) error {
	c.mu.Lock()
	now := time.Now()
	if now.After(c.nextAllowed) {
		c.nextAllowed = now
	}

	wait := c.nextAllowed.Sub(now)

	interval := c.ThrottleInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	c.nextAllowed = c.nextAllowed.Add(interval)
	c.mu.Unlock()

	if wait > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
	return nil
}

func expandTimespanCandidates(raw string) []string {
	base := strings.ToLower(strings.TrimSpace(raw))
	seen := map[string]struct{}{}
	out := make([]string, 0, 6)
	appendUnique := func(value string) {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	appendUnique(base)
	if strings.HasSuffix(base, "h") && len(base) > 1 {
		numText := strings.TrimSuffix(base, "h")
		if num, err := strconv.Atoi(numText); err == nil && num > 0 {
			if num == 1 {
				appendUnique("1hour")
			} else {
				appendUnique(fmt.Sprintf("%dhours", num))
			}
			appendUnique(fmt.Sprintf("%dmin", num*60))
			appendUnique(fmt.Sprintf("%dminutes", num*60))
		}
	}
	if strings.HasSuffix(base, "d") && len(base) > 1 {
		numText := strings.TrimSuffix(base, "d")
		if num, err := strconv.Atoi(numText); err == nil && num > 0 {
			if num == 1 {
				appendUnique("1day")
			} else {
				appendUnique(fmt.Sprintf("%ddays", num))
			}
			appendUnique(fmt.Sprintf("%dh", num*24))
		}
	}
	return out
}

func abbreviateBody(raw []byte, maxLen int) string {
	text := strings.TrimSpace(string(raw))
	if maxLen <= 0 || len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "...(truncated)"
}

func parseSeenDate(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	layouts := []string{
		"20060102T150405Z",
		time.RFC3339,
	}
	for _, layout := range layouts {
		if tm, err := time.Parse(layout, raw); err == nil {
			return tm.UTC()
		}
	}
	return time.Time{}
}
