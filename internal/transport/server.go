package transport

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

var okBody = []byte(`{"status":"ok"}`)

type Server struct {
	Addr string
}

func (s *Server) Start(ctx context.Context) error {
	if err := s.validate(); err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(okBody)
	})
	srv := &http.Server{
		Addr:    s.Addr,
		Handler: mux,
	}
	ShutdownServerOnContext(ctx, srv, 5*time.Second)
	return srv.ListenAndServe()
}

func (s *Server) validate() error {
	if strings.TrimSpace(s.Addr) == "" {
		return fmt.Errorf("addr is required")
	}
	return nil
}
