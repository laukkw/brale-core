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
	addr := flag.String("addr", ":7788", "http listen addr")
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

	fmt.Printf("onboarding listening on http://127.0.0.1%s\n", *addr)
	if err := http.ListenAndServe(*addr, handler); err != nil {
		fmt.Fprintf(os.Stderr, "serve onboarding: %v\n", err)
		os.Exit(1)
	}
}
