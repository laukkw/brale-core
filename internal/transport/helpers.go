package transport

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

func StartHTTPServer(ctx context.Context, addr string, handler http.Handler, logger *zap.Logger) {
	httpSrv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Warn("http server failed", zap.Error(err))
		}
	}()
	ShutdownServerOnContext(ctx, httpSrv, 5*time.Second)
}

func ShutdownServerOnContext(ctx context.Context, server *http.Server, timeout time.Duration) {
	if server == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func BuildRuntimeBaseURL(addr string) string {
	trimmed := strings.TrimSpace(addr)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return strings.TrimRight(trimmed, "/")
	}
	host, port, err := net.SplitHostPort(trimmed)
	if err != nil {
		return "http://" + strings.TrimRight(trimmed, "/")
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}
