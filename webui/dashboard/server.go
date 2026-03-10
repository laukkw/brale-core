package dashboard

import (
	"embed"
	"fmt"
	"net/http"
	"strings"
)

//go:embed index.html styles.css main.js favicon-mask.svg
var content embed.FS

type Server struct {
	BasePath string
}

func (s Server) Handler() (http.Handler, error) {
	base := normalizeBasePath(s.BasePath)
	if base == "" || base == "/" {
		return nil, fmt.Errorf("base path is invalid")
	}

	mux := http.NewServeMux()
	fs := http.FileServer(http.FS(content))
	mux.Handle(base+"/", http.StripPrefix(base+"/", spaHandler(fs)))
	mux.Handle(base, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, base+"/", http.StatusTemporaryRedirect)
	}))
	return mux, nil
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
	data, err := content.ReadFile("index.html")
	if err != nil {
		http.Error(w, "index not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func normalizeBasePath(base string) string {
	if strings.TrimSpace(base) == "" {
		return "/dashboard"
	}
	out := base
	if !strings.HasPrefix(out, "/") {
		out = "/" + out
	}
	return strings.TrimSuffix(out, "/")
}
