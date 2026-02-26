package execution

import (
	"context"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

func newFreqtradeRetryClient(base *http.Client) *retryablehttp.Client {
	client := retryablehttp.NewClient()
	client.RetryMax = 3
	client.RetryWaitMin = 200 * time.Millisecond
	client.RetryWaitMax = 2 * time.Second
	client.Logger = nil
	client.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		if err != nil {
			return true, nil
		}
		if resp == nil {
			return false, nil
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			return true, nil
		}
		return false, nil
	}
	if base != nil {
		client.HTTPClient = base
	}
	return client
}
