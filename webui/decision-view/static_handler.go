package decisionview

import "net/http"

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
