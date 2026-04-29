package telegrambot

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"brale-core/internal/cardimage"

	"go.uber.org/zap"
)

type botRoundTripFunc func(*http.Request) (*http.Response, error)

func (f botRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type botErrReadCloser struct {
	err error
}

func (r botErrReadCloser) Read(_ []byte) (int, error) {
	return 0, r.err
}

func (r botErrReadCloser) Close() error {
	return nil
}

func newBotTestHTTPClient(fn botRoundTripFunc) *http.Client {
	return &http.Client{Transport: fn}
}

func TestDoTelegramRequestReturnsBodyReadError(t *testing.T) {
	t.Parallel()

	bot := &Bot{
		apiBase: "https://api.telegram.org",
		token:   "token",
		client: newBotTestHTTPClient(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadGateway,
				Status:     "502 Bad Gateway",
				Body:       botErrReadCloser{err: errors.New("body boom")},
			}, nil
		}),
		logger: zap.NewNop(),
	}

	err := bot.doTelegramRequest(context.Background(), http.MethodGet, "getUpdates", nil, nil)
	if err == nil {
		t.Fatal("doTelegramRequest() error = nil, want body read error")
	}
	if !strings.Contains(err.Error(), "read response body") {
		t.Fatalf("error=%q should mention body read context", err.Error())
	}
	if !strings.Contains(err.Error(), "body boom") {
		t.Fatalf("error=%q should mention underlying body read error", err.Error())
	}
}

func TestDoTelegramRequestRedactsTokenFromNetworkError(t *testing.T) {
	t.Parallel()

	bot := &Bot{
		apiBase: "https://api.telegram.org",
		token:   "123:secret-token",
		client: newBotTestHTTPClient(func(r *http.Request) (*http.Response, error) {
			return nil, errors.New(`Get "https://api.telegram.org/bot123:secret-token/getUpdates": unexpected EOF`)
		}),
		logger: zap.NewNop(),
	}

	err := bot.doTelegramRequest(context.Background(), http.MethodGet, "getUpdates", nil, nil)
	if err == nil {
		t.Fatal("doTelegramRequest() error = nil, want network error")
	}
	if strings.Contains(err.Error(), "123:secret-token") {
		t.Fatalf("error leaked token: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "/bot<redacted>") {
		t.Fatalf("error missing redacted bot path: %q", err.Error())
	}
}

func TestSendImageReturnsBodyReadError(t *testing.T) {
	t.Parallel()

	bot := &Bot{
		apiBase: "https://api.telegram.org",
		token:   "token",
		client: newBotTestHTTPClient(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadGateway,
				Status:     "502 Bad Gateway",
				Body:       botErrReadCloser{err: errors.New("body boom")},
			}, nil
		}),
		logger: zap.NewNop(),
	}

	err := bot.sendImage(context.Background(), 42, &cardimage.ImageAsset{Data: []byte("png"), Filename: "observe.png"})
	if err == nil {
		t.Fatal("sendImage() error = nil, want body read error")
	}
	if !strings.Contains(err.Error(), "read response body") {
		t.Fatalf("error=%q should mention body read context", err.Error())
	}
	if !strings.Contains(err.Error(), "body boom") {
		t.Fatalf("error=%q should mention underlying body read error", err.Error())
	}
}
