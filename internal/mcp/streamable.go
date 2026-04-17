package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewStreamableHandler returns an http.Handler that serves MCP over the
// Streamable HTTP transport (MCP spec 2025-03-26+).
func NewStreamableHandler(opts Options) (http.Handler, error) {
	server, err := NewServer(opts)
	if err != nil {
		return nil, err
	}
	mcpHandler := sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server {
		return server
	}, &sdkmcp.StreamableHTTPOptions{
		Logger: slog.Default(),
	})
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	return mux, nil
}

// ServeHTTP starts an HTTP server that serves MCP over the Streamable HTTP
// transport. The server shuts down gracefully when ctx is cancelled.
func ServeHTTP(ctx context.Context, opts Options, addr string) error {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return fmt.Errorf("http addr is required")
	}
	handler, err := NewStreamableHandler(opts)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
