package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

type FreqtradeStatusFetcher struct {
	Endpoint  string
	Client    *http.Client
	Retry     *retryablehttp.Client
	APIKey    string
	APISecret string
	AuthType  string
}

var ErrOrderNotFound = errors.New("order not found")

func (f *FreqtradeStatusFetcher) Fetch(ctx context.Context, externalOrderID string) (OrderStatus, error) {
	if f == nil {
		return OrderStatus{}, fmt.Errorf("fetcher is nil")
	}
	endpoint := strings.TrimSpace(f.Endpoint)
	if endpoint == "" || strings.TrimSpace(externalOrderID) == "" {
		return OrderStatus{}, fmt.Errorf("endpoint and order id are required")
	}
	client := f.client()
	if ctx == nil {
		ctx = context.Background()
	}
	base := normalizeFreqtradeEndpoint(endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/orders/"+externalOrderID, nil)
	if err != nil {
		return OrderStatus{}, err
	}
	rreq, err := retryablehttp.FromRequest(req)
	if err != nil {
		return OrderStatus{}, err
	}
	rreq.Header.Set("Accept", "application/json")
	applyExecAuth(rreq.Request, f.AuthType, f.APIKey, f.APISecret)
	resp, err := f.retryClient(client).Do(rreq)
	if err != nil {
		return OrderStatus{}, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode == http.StatusNotFound {
		return OrderStatus{}, ErrOrderNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return OrderStatus{}, fmt.Errorf("freqtrade status %s", resp.Status)
	}
	var payload freqtradeOrderStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return OrderStatus{}, err
	}
	return OrderStatus{
		Status:       payload.Status,
		Filled:       payload.Filled,
		Price:        payload.Price,
		Fee:          payload.Fee,
		Timestamp:    payload.Timestamp,
		Reason:       payload.Reason,
		RawStatus:    payload.Status,
		CancelReason: payload.Reason,
	}, nil
}

func normalizeFreqtradeEndpoint(endpoint string) string {
	base := normalizeFreqtradeEndpointBase(endpoint)
	if base == "" {
		return ""
	}
	if strings.HasSuffix(strings.ToLower(base), "/api/v1") {
		return base
	}
	return base + "/api/v1"
}

func normalizeFreqtradeEndpointBase(endpoint string) string {
	return strings.TrimRight(strings.TrimSpace(endpoint), "/")
}

type freqtradeOrderStatusResponse struct {
	Status    string  `json:"status"`
	Filled    float64 `json:"filled"`
	Price     float64 `json:"price"`
	Fee       float64 `json:"fee"`
	Timestamp int64   `json:"timestamp"`
	Reason    string  `json:"reason"`
}

func (f *FreqtradeStatusFetcher) client() *http.Client {
	if f != nil && f.Client != nil {
		return f.Client
	}
	return &http.Client{Timeout: 5 * time.Second}
}

func (f *FreqtradeStatusFetcher) retryClient(base *http.Client) *retryablehttp.Client {
	if f != nil && f.Retry != nil {
		return f.Retry
	}
	return newFreqtradeRetryClient(base)
}
