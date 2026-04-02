package decisionview

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
)

const (
	defaultRoundLimit = 30
)

func (s Server) Handler() (http.Handler, error) {
	if s.Store == nil {
		return nil, fmt.Errorf("store is required")
	}
	base := normalizeBasePath(s.BasePath)
	roundLimit := s.RoundLimit
	if roundLimit <= 0 {
		roundLimit = defaultRoundLimit
	}
	mux := http.NewServeMux()
	mux.Handle(join(base, "/api/symbols"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		symbols, err := s.Store.ListSymbols(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, mapSymbols(symbols))
	}))
	mux.Handle(join(base, "/api/chains"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp, err := s.queryChains(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, resp)
	}))
	mux.Handle(join(base, "/api/config-graph"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, s.buildConfigGraph())
	}))
	fs := http.FileServer(http.FS(content))
	mux.Handle(base+"/", http.StripPrefix(base+"/", spaHandler(fs)))
	mux.Handle(base, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, base+"/", http.StatusTemporaryRedirect)
	}))
	return mux, nil
}

func (s Server) queryChains(r *http.Request) (Response, error) {
	symbol := strings.TrimSpace(r.URL.Query().Get("symbol"))
	limit := s.RoundLimit
	if limit <= 0 {
		limit = defaultRoundLimit
	}
	if qs := strings.TrimSpace(r.URL.Query().Get("limit")); qs != "" {
		if v, err := strconv.Atoi(qs); err == nil && v > 0 {
			limit = v
		}
	}
	targets, err := s.resolveTargets(r, symbol)
	if err != nil {
		return Response{}, err
	}
	return buildResponse(r.Context(), s.Store, targets, limit)
}

func (s Server) resolveTargets(r *http.Request, symbol string) ([]string, error) {
	if symbol != "" {
		return []string{symbol}, nil
	}
	return s.Store.ListSymbols(r.Context())
}

func writeJSON(w http.ResponseWriter, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func normalizeBasePath(base string) string {
	if strings.TrimSpace(base) == "" {
		return "/decision-view"
	}
	out := base
	if !strings.HasPrefix(out, "/") {
		out = "/" + out
	}
	out = strings.TrimSuffix(out, "/")
	return out
}

func join(base, p string) string {
	if base == "" || base == "/" {
		return path.Clean(p)
	}
	return path.Clean(base + p)
}
