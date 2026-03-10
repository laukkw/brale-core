package dashboard

import "net/http"

func Start() http.Handler {
	h, err := (Server{BasePath: "/dashboard"}).Handler()
	if err != nil {
		return nil
	}
	return h
}
