package mcp

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	httpProbeTimeout     = 2 * time.Second
	httpReadinessWindow  = 30 * time.Second
	httpProbeInterval    = 1 * time.Second
	dockerCommandTimeout = 15 * time.Second
)

var (
	httpClientFactory = func(timeout time.Duration) *http.Client {
		return &http.Client{Timeout: timeout}
	}
	lookPathFunc                 = exec.LookPath
	probeHTTPFunc                = probeHTTP
	probeMCPTransportFunc        = probeMCPTransport
	waitForHTTPFunc              = waitForHTTP
	waitForMCPTransportFunc      = waitForMCPTransport
	checkDockerPrerequisitesFunc = checkDockerPrerequisites
	startHTTPViaDockerFunc       = startHTTPViaDocker
	runCommandFunc               = func(ctx context.Context, dir string, name string, args ...string) error {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		if err != nil {
			if len(output) == 0 {
				return fmt.Errorf("%s %v: %w", name, args, err)
			}
			return fmt.Errorf("%s %v: %w: %s", name, args, err, string(output))
		}
		return nil
	}
)

func ensureHTTPAvailable(prepared preparedInstall) error {
	if prepared.mode != "http" {
		return nil
	}
	healthURL, err := httpHealthURL(prepared.httpURL)
	if err != nil {
		return err
	}
	if err := probeHTTPFunc(healthURL, httpProbeTimeout); err == nil {
		if err := probeMCPTransportFunc(prepared.httpURL, httpProbeTimeout); err == nil {
			return nil
		}
	}
	if err := checkDockerPrerequisitesFunc(prepared.repoRoot); err != nil {
		return err
	}
	if err := startHTTPViaDockerFunc(prepared.repoRoot); err != nil {
		return err
	}
	if err := waitForHTTPFunc(healthURL, httpReadinessWindow); err != nil {
		return err
	}
	if err := waitForMCPTransportFunc(prepared.httpURL, httpReadinessWindow); err != nil {
		return err
	}
	return nil
}

func httpHealthURL(endpoint string) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse MCP HTTP endpoint %q: %w", endpoint, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid MCP HTTP endpoint %q", endpoint)
	}
	parsed.Path = "/healthz"
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func probeHTTP(endpoint string, timeout time.Duration) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build HTTP probe request: %w", err)
	}
	resp, err := httpClientFactory(timeout).Do(req)
	if err != nil {
		return fmt.Errorf("probe HTTP endpoint %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("probe HTTP endpoint %s: unexpected status %d", endpoint, resp.StatusCode)
	}
	return nil
}

func probeMCPTransport(endpoint string, timeout time.Duration) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"brale-mcp-probe","version":"1.0"}}}`))
	if err != nil {
		return fmt.Errorf("build MCP transport probe request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-06-18")
	resp, err := httpClientFactory(timeout).Do(req)
	if err != nil {
		return fmt.Errorf("probe MCP transport endpoint %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("probe MCP transport endpoint %s: unexpected status %d", endpoint, resp.StatusCode)
	}
	return nil
}

func waitForHTTP(endpoint string, maxWait time.Duration) error {
	deadline := time.Now().Add(maxWait)
	var lastErr error
	for {
		if err := probeHTTP(endpoint, httpProbeTimeout); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(httpProbeInterval)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("timed out waiting for HTTP endpoint")
	}
	return fmt.Errorf("HTTP readiness timeout for %s: %w", endpoint, lastErr)
}

func waitForMCPTransport(endpoint string, maxWait time.Duration) error {
	deadline := time.Now().Add(maxWait)
	var lastErr error
	for {
		if err := probeMCPTransport(endpoint, httpProbeTimeout); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(httpProbeInterval)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("timed out waiting for MCP transport endpoint")
	}
	return fmt.Errorf("MCP transport readiness timeout for %s: %w", endpoint, lastErr)
}

func checkDockerPrerequisites(repoRoot string) error {
	if _, err := lookPathFunc("docker"); err != nil {
		return fmt.Errorf("docker executable not found: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), dockerCommandTimeout)
	defer cancel()
	if err := runCommand(ctx, repoRoot, "docker", "info"); err != nil {
		return fmt.Errorf("docker daemon is not reachable (attempted: docker info): %w", err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), dockerCommandTimeout)
	defer cancel()
	if err := runCommand(ctx, repoRoot, "docker", "compose", "version"); err != nil {
		return fmt.Errorf("docker compose is unavailable (attempted: docker compose version): %w", err)
	}
	composePath := filepath.Join(repoRoot, "docker-compose.yml")
	info, err := os.Stat(composePath)
	if err != nil {
		return fmt.Errorf("compose file not found at %s: %w", composePath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("compose path is a directory, expected file: %s", composePath)
	}
	return nil
}

func startHTTPViaDocker(repoRoot string) error {
	ctx, cancel := context.WithTimeout(context.Background(), dockerCommandTimeout)
	defer cancel()
	if err := runCommand(ctx, repoRoot, "docker", "compose", "--profile", "mcp", "up", "-d", "--build", "mcp"); err != nil {
		return fmt.Errorf("start docker-backed MCP HTTP service (attempted: docker compose --profile mcp up -d --build mcp): %w", err)
	}
	return nil
}
