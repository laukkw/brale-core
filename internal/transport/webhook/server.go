// 本文件主要内容：提供 freqtrade webhook 接收服务。

package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"brale-core/internal/pkg/logging"
	"brale-core/internal/pkg/parseutil"
	"brale-core/internal/transport"

	"go.uber.org/zap"
)

type Handler interface {
	HandleWebhook(ctx context.Context, payload FreqtradeWebhookPayload) error
}

type Server struct {
	Addr        string
	Handler     Handler
	Secret      string
	IPAllowlist []string
}

type errorResponse struct {
	Code    string `json:"code"`
	Msg     string `json:"msg"`
	Details any    `json:"details,omitempty"`
}

type FreqtradeWebhookPayload struct {
	Type        string          `json:"type"`
	TradeID     Int64OrString   `json:"trade_id"`
	Pair        string          `json:"pair"`
	EnterTag    string          `json:"enter_tag,omitempty"`
	Direction   string          `json:"direction"`
	Amount      Float64OrString `json:"amount,omitempty"`
	StakeAmount Float64OrString `json:"stake_amount,omitempty"`
	OrderRate   Float64OrString `json:"order_rate,omitempty"`
	OpenRate    Float64OrString `json:"open_rate,omitempty"`
	CloseRate   Float64OrString `json:"close_rate,omitempty"`
	Leverage    Float64OrString `json:"leverage,omitempty"`
	OpenDate    string          `json:"open_date,omitempty"`
	CloseDate   string          `json:"close_date,omitempty"`
	ExitReason  string          `json:"exit_reason,omitempty"`
	Reason      string          `json:"reason,omitempty"`
}

type Int64OrString int64

func (v *Int64OrString) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	if s == "" || s == "null" {
		*v = 0
		return nil
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = strings.Trim(s, "\"")
	}
	if s == "" {
		*v = 0
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	*v = Int64OrString(n)
	return nil
}

type Float64OrString float64

func (v *Float64OrString) UnmarshalJSON(data []byte) error {
	n, err := parseutil.ParseNullableFloatJSON(data)
	if err != nil {
		return err
	}
	*v = Float64OrString(n)
	return nil
}

func (s *Server) Start(ctx context.Context) error {
	mux, err := s.Mux()
	if err != nil {
		return err
	}
	server := &http.Server{Addr: s.Addr, Handler: mux}
	transport.ShutdownServerOnContext(ctx, server, 5*time.Second)
	return server.ListenAndServe()
}

// Mux 构造可复用的 http.Handler，便于外部组合更多路由。
func (s *Server) Mux() (*http.ServeMux, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/api/live/freqtrade/webhook", s.handleFreqtradeWebhook)
	return mux, nil
}

func (s *Server) validate() error {
	if s.Handler == nil {
		return fmt.Errorf("handler is required")
	}
	if strings.TrimSpace(s.Addr) == "" {
		return fmt.Errorf("addr is required")
	}
	return nil
}

func (s *Server) handleFreqtradeWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "仅支持 POST", nil)
		return
	}
	logger := logging.FromContext(r.Context()).Named("webhook")
	if err := s.validateSecret(r); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_webhook_secret", err.Error(), nil)
		return
	}
	if err := s.validateIP(r); err != nil {
		writeError(w, http.StatusForbidden, "client_ip_not_allowed", err.Error(), nil)
		return
	}
	var payload FreqtradeWebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		logger.Error("webhook decode failed", zap.Error(err))
		writeError(w, http.StatusBadRequest, "invalid_body", "请求体解析失败", err.Error())
		return
	}
	logger.Info("webhook received",
		zap.String("type", strings.TrimSpace(payload.Type)),
		zap.String("pair", strings.TrimSpace(payload.Pair)),
		zap.Int64("trade_id", int64(payload.TradeID)),
	)
	if err := s.Handler.HandleWebhook(r.Context(), payload); err != nil {
		writeError(w, http.StatusInternalServerError, "webhook_failed", "webhook 处理失败", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func writeError(w http.ResponseWriter, status int, code, msg string, details any) {
	resp := errorResponse{Code: code, Msg: msg, Details: details}
	transport.WriteJSON(w, status, resp)
}

func (s *Server) validateSecret(r *http.Request) error {
	secret := strings.TrimSpace(s.Secret)
	if secret == "" {
		return nil
	}
	got := strings.TrimSpace(r.Header.Get("X-Webhook-Secret"))
	if got != secret {
		return fmt.Errorf("invalid webhook secret")
	}
	return nil
}

func (s *Server) validateIP(r *http.Request) error {
	if len(s.IPAllowlist) == 0 {
		return nil
	}
	ip := clientIP(r)
	if ip == nil {
		return fmt.Errorf("invalid client ip")
	}
	for _, entry := range s.IPAllowlist {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			_, cidr, err := net.ParseCIDR(entry)
			if err != nil {
				continue
			}
			if cidr.Contains(ip) {
				return nil
			}
			continue
		}
		if ip.Equal(net.ParseIP(entry)) {
			return nil
		}
	}
	return fmt.Errorf("client ip not allowed")
}

func clientIP(r *http.Request) net.IP {
	if r == nil {
		return nil
	}
	if fwd := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); fwd != "" {
		return net.ParseIP(fwd)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return net.ParseIP(host)
	}
	return net.ParseIP(r.RemoteAddr)
}

func ParseWebhookTimestamp(payload FreqtradeWebhookPayload, now func() int64) int64 {
	if ts := parseTimestamp(payload.CloseDate); ts > 0 {
		return ts
	}
	if ts := parseTimestamp(payload.OpenDate); ts > 0 {
		return ts
	}
	if now != nil {
		return now()
	}
	return time.Now().UnixMilli()
}

func parseTimestamp(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t.UnixMilli()
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UnixMilli()
	}
	return 0
}
