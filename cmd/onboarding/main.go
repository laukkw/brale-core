package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"brale-core/internal/onboarding"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:9992", "http listen addr")
	repo := flag.String("repo", ".", "repository root")
	basePath := flag.String("base", "/", "http base path")
	flag.Parse()

	repoRoot, err := filepath.Abs(*repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve repo root: %v\n", err)
		os.Exit(1)
	}

	handler, err := onboarding.Server{RepoRoot: repoRoot, BasePath: *basePath}.Handler()
	if err != nil {
		fmt.Fprintf(os.Stderr, "init onboarding server: %v\n", err)
		os.Exit(1)
	}

	listenURL := "http://" + *addr
	if len(*addr) > 0 && (*addr)[0] == ':' {
		listenURL = "http://127.0.0.1" + *addr
	}
	fmt.Printf("onboarding listening on %s\n", listenURL)
	if err := http.ListenAndServe(*addr, handler); err != nil {
		fmt.Fprintf(os.Stderr, "serve onboarding: %v\n", err)
		os.Exit(1)
	}
}
