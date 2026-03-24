package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"brale-core/internal/onboarding"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "prepare-stack":
			cwd, err := os.Getwd()
			if err != nil {
				fmt.Fprintf(os.Stderr, "resolve working directory: %v\n", err)
				os.Exit(1)
			}
			if err := onboarding.RunPrepareStack(os.Args[2:], cwd, os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "prepare stack: %v\n", err)
				os.Exit(1)
			}
			return
		case "serve":
			runServe(os.Args[2:], os.Stderr)
			return
		case "help", "-h", "--help":
			printHelp(os.Stdout)
			return
		}
	}

	runServe(os.Args[1:], os.Stderr)
}

func runServe(args []string, stderr io.Writer) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	fs.SetOutput(stderr)

	addr := fs.String("addr", "127.0.0.1:9992", "http listen addr")
	repo := fs.String("repo", ".", "repository root")
	basePath := fs.String("base", "/", "http base path")
	allowNonLoopback := fs.Bool("allow-non-loopback", false, "allow non-loopback requests for trusted containerized deployments")
	fs.Parse(args)

	repoRoot, err := filepath.Abs(*repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve repo root: %v\n", err)
		os.Exit(1)
	}

	handler, err := onboarding.Server{RepoRoot: repoRoot, BasePath: *basePath, AllowNonLoopback: *allowNonLoopback}.Handler()
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

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "onboarding command")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  onboarding serve [flags]")
	fmt.Fprintln(w, "  onboarding prepare-stack [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "When called without a subcommand, onboarding runs in serve mode for compatibility.")
	fmt.Fprintln(w, "Run `onboarding serve --help` or `onboarding prepare-stack --help` for details.")
}
