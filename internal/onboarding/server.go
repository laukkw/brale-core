package onboarding

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	configonboardingprototype "brale-core/webui/config-onboarding-prototype"
)

type Server struct {
	RepoRoot string
	BasePath string
}

func (s Server) Handler() (http.Handler, error) {
	repoRoot := strings.TrimSpace(s.RepoRoot)
	if repoRoot == "" {
		return nil, fmt.Errorf("repo root is required")
	}
	base := normalizeBasePath(s.BasePath)
	assetPrefix := base
	if assetPrefix == "/" {
		assetPrefix = ""
	}
	gen := NewGenerator(repoRoot)
	runner := &startupRunner{}

	mux := http.NewServeMux()
	mux.Handle(join(base, "/api/status"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := os.Stat(filepath.Join(repoRoot, "data/onboarding/.done"))
		writeJSON(w, map[string]any{"ready": err == nil})
	}))
	mux.Handle(join(base, "/api/preview"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req Request
		if err := decodeJSON(r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		result, err := gen.Preview(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, result)
	}))
	mux.Handle(join(base, "/api/generate"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req Request
		if err := decodeJSON(r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		result, err := gen.Generate(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, result)
	}))
	mux.Handle(join(base, "/api/startup/check"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !isLoopbackRequest(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		writeJSON(w, runStartupCheck(repoRoot))
	}))
	mux.Handle(join(base, "/api/startup/monitor"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !isLoopbackRequest(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		writeJSON(w, runStartupMonitor())
	}))
	mux.Handle(join(base, "/api/startup/service-action"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !isLoopbackRequest(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var req startupServiceActionRequest
		if err := decodeJSON(r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		result := runStartupServiceAction(repoRoot, req)
		if !result.OK {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(result)
			return
		}
		writeJSON(w, result)
	}))
	mux.Handle(join(base, "/api/startup/start-stream"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		streamStartup(repoRoot, runner, w, r)
	}))

	fs := http.FileServer(http.FS(configonboardingprototype.Assets))
	mux.Handle(assetPrefix+"/", http.StripPrefix(assetPrefix+"/", spaHandler(fs)))
	if base != "/" {
		mux.Handle(base, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, base+"/", http.StatusTemporaryRedirect)
		}))
	}
	return mux, nil
}

func decodeJSON(r *http.Request, dst any) error {
	defer func() { _ = r.Body.Close() }()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid json: %w", err)
	}
	return nil
}

func spaHandler(fs http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "" {
			serveIndex(w)
			return
		}
		fs.ServeHTTP(w, r)
	})
}

func serveIndex(w http.ResponseWriter) {
	data, err := configonboardingprototype.Assets.ReadFile("index.html")
	if err != nil {
		http.Error(w, "index not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func writeJSON(w http.ResponseWriter, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func normalizeBasePath(base string) string {
	if strings.TrimSpace(base) == "" {
		return "/"
	}
	out := base
	if !strings.HasPrefix(out, "/") {
		out = "/" + out
	}
	out = strings.TrimSuffix(out, "/")
	if out == "" {
		return "/"
	}
	return out
}

func join(base, p string) string {
	if base == "" || base == "/" {
		return path.Clean(p)
	}
	return path.Clean(base + p)
}
