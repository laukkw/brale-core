package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
)

func NewJSONRequest(ctx context.Context, method, url string, payload any) (*http.Request, error) {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewBuffer(raw)
	} else {
		body = bytes.NewBuffer(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func ReadLimitedBody(body io.Reader, limit int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(body, limit))
}
