package onboarding

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type startupRunner struct {
	mu      sync.Mutex
	running bool
}

func (r *startupRunner) begin() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return false
	}
	r.running = true
	return true
}

func (r *startupRunner) end() {
	r.mu.Lock()
	r.running = false
	r.mu.Unlock()
}

type startupCheckResult struct {
	DockerInstalled  bool   `json:"docker_installed"`
	DockerVersion    string `json:"docker_version"`
	ComposeInstalled bool   `json:"compose_installed"`
	ComposeVersion   string `json:"compose_version"`
	ConfigOK         bool   `json:"config_ok"`
	ConfigDetail     string `json:"config_detail"`
	Ready            bool   `json:"ready"`
	BraleRunning     bool   `json:"brale_running"`
	BraleURL         string `json:"brale_url"`
	FreqtradeRunning bool   `json:"freqtrade_running"`
	FreqtradeURL     string `json:"freqtrade_url"`
}

type startupMonitorResult struct {
	BraleRunning     bool   `json:"brale_running"`
	BraleURL         string `json:"brale_url"`
	FreqtradeRunning bool   `json:"freqtrade_running"`
	FreqtradeURL     string `json:"freqtrade_url"`
}

type startupServiceActionRequest struct {
	Service string `json:"service"`
	Action  string `json:"action"`
}

type startupServiceActionResult struct {
	OK      bool                 `json:"ok"`
	Service string               `json:"service"`
	Action  string               `json:"action"`
	Output  string               `json:"output"`
	Error   string               `json:"error,omitempty"`
	Monitor startupMonitorResult `json:"monitor"`
}

const (
	braleDashboardAddr = "127.0.0.1:9991"
	braleDashboardURL  = "http://127.0.0.1:9991/dashboard/"
	freqtradeAddr      = "127.0.0.1:8080"
	freqtradeURL       = "http://127.0.0.1:8080"
)

var startupHTTPClient = &http.Client{
	Timeout: 700 * time.Millisecond,
	Transport: &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout: 700 * time.Millisecond,
		}).DialContext,
	},
}

func runStartupCheck(repoRoot string) startupCheckResult {
	out := startupCheckResult{}
	if _, err := exec.LookPath("docker"); err == nil {
		out.DockerInstalled = true
		if v, err := runCommandWithTimeout(repoRoot, 5*time.Second, "docker", "--version"); err == nil {
			out.DockerVersion = firstLine(v)
		}
		if v, err := runCommandWithTimeout(repoRoot, 5*time.Second, "docker", "compose", "version"); err == nil {
			out.ComposeInstalled = true
			out.ComposeVersion = firstLine(v)
		}
	}

	if v, err := runCommandWithTimeout(repoRoot, 60*time.Second, "make", "check"); err == nil {
		out.ConfigOK = true
		out.ConfigDetail = strings.TrimSpace(v)
	} else {
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			trimmed = strings.TrimSpace(err.Error())
		}
		out.ConfigDetail = trimmed
	}
	out.Ready = out.DockerInstalled && out.ComposeInstalled && out.ConfigOK
	monitor := runStartupMonitor()
	out.BraleRunning = monitor.BraleRunning
	out.BraleURL = monitor.BraleURL
	out.FreqtradeRunning = monitor.FreqtradeRunning
	out.FreqtradeURL = monitor.FreqtradeURL
	return out
}

func runStartupMonitor() startupMonitorResult {
	return startupMonitorResult{
		BraleRunning:     probeReachable([]string{"http://127.0.0.1:9991/healthz", "http://brale:9991/healthz"}),
		BraleURL:         braleDashboardURL,
		FreqtradeRunning: probeReachable([]string{"http://127.0.0.1:8080/api/v1/ping", "http://freqtrade:8080/api/v1/ping"}),
		FreqtradeURL:     freqtradeURL,
	}
}

func runStartupServiceAction(repoRoot string, req startupServiceActionRequest) startupServiceActionResult {
	service := strings.ToLower(strings.TrimSpace(req.Service))
	action := strings.ToLower(strings.TrimSpace(req.Action))
	out := startupServiceActionResult{
		Service: service,
		Action:  action,
	}

	var args []string
	timeout := 180 * time.Second
	switch service {
	case "brale":
		switch action {
		case "start":
			args = []string{"start-brale"}
		case "stop":
			args = []string{"stop-brale"}
		case "pull-rebuild":
			args = []string{"onboarding-refresh-brale"}
			timeout = 10 * time.Minute
		default:
			out.Error = "unsupported action"
			out.Monitor = runStartupMonitor()
			return out
		}
	case "freqtrade":
		switch action {
		case "start":
			args = []string{"start-freqtrade", "wait-freqtrade"}
		case "stop":
			args = []string{"stop-freqtrade"}
		default:
			out.Error = "unsupported action"
			out.Monitor = runStartupMonitor()
			return out
		}
	case "stack":
		switch action {
		case "make-start":
			args = []string{"apply-config"}
			timeout = 10 * time.Minute
		case "apply-config":
			args = []string{"apply-config"}
			timeout = 10 * time.Minute
		default:
			out.Error = "unsupported action"
			out.Monitor = runStartupMonitor()
			return out
		}
	default:
		out.Error = "unsupported service"
		out.Monitor = runStartupMonitor()
		return out
	}

	cmdArgs := append([]string{"make"}, args...)
	text, err := runCommandWithTimeout(repoRoot, timeout, cmdArgs[0], cmdArgs[1:]...)
	out.Output = truncateOutput(strings.TrimSpace(text), 12000)
	out.Monitor = runStartupMonitor()
	if err != nil {
		out.Error = err.Error()
		return out
	}
	out.OK = true
	return out
}

func truncateOutput(in string, max int) string {
	if max <= 0 || len(in) <= max {
		return in
	}
	if max <= 3 {
		return in[:max]
	}
	return in[:max-3] + "..."
}

func isTCPReachable(addr string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func probeReachable(targets []string) bool {
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		if looksLikeURL(target) {
			if isHTTPReachable(target) {
				return true
			}
			continue
		}
		if isTCPReachable(target, 700*time.Millisecond) {
			return true
		}
	}
	return false
}

func looksLikeURL(target string) bool {
	u, err := url.Parse(target)
	if err != nil {
		return false
	}
	return u.Scheme != "" && u.Host != ""
}

func isHTTPReachable(target string) bool {
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return false
	}
	resp, err := startupHTTPClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}

func runCommandWithTimeout(repoRoot string, timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = repoRoot
	data, err := cmd.CombinedOutput()
	return string(data), err
}

func firstLine(in string) string {
	line := strings.TrimSpace(in)
	if line == "" {
		return ""
	}
	if idx := strings.IndexByte(line, '\n'); idx >= 0 {
		return strings.TrimSpace(line[:idx])
	}
	return line
}

func streamStartup(repoRoot string, runner *startupRunner, allowNonLoopback bool, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !requestAllowed(r, allowNonLoopback) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	if !runner.begin() {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		_ = writeSSE(w, "done", map[string]any{"ok": false, "error": "已有启动任务在运行，请稍后重试"})
		flusher.Flush()
		return
	}
	defer runner.end()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	cmdCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, "make", "apply-config")
	cmd.Dir = repoRoot

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = writeSSE(w, "done", map[string]any{"ok": false, "error": err.Error()})
		flusher.Flush()
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = writeSSE(w, "done", map[string]any{"ok": false, "error": err.Error()})
		flusher.Flush()
		return
	}

	if err := cmd.Start(); err != nil {
		_ = writeSSE(w, "done", map[string]any{"ok": false, "error": err.Error()})
		flusher.Flush()
		return
	}

	_ = writeSSE(w, "status", map[string]any{"status": "running", "message": "已启动 make apply-config"})
	flusher.Flush()

	lines := make(chan string, 256)
	var wg sync.WaitGroup
	scan := func(prefix string, src io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(src)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)
		for scanner.Scan() {
			text := strings.TrimSpace(scanner.Text())
			if text == "" {
				continue
			}
			if prefix != "" {
				text = prefix + text
			}
			lines <- text
		}
	}

	wg.Add(2)
	go scan("", stdout)
	go scan("[stderr] ", stderr)

	go func() {
		wg.Wait()
		close(lines)
	}()

	progress := newPullProgressTracker()
	clientClosed := false
	heartbeat := time.NewTicker(10 * time.Second)
	defer heartbeat.Stop()

	for lines != nil {
		select {
		case line, ok := <-lines:
			if !ok {
				lines = nil
				continue
			}
			if !clientClosed {
				if err := writeSSE(w, "log", map[string]any{"line": line}); err != nil {
					clientClosed = true
				}
			}
			if p, ok := progress.Update(line); ok && !clientClosed {
				if err := writeSSE(w, "progress", p); err != nil {
					clientClosed = true
				}
			}
			if !clientClosed {
				flusher.Flush()
			}
		case <-heartbeat.C:
			if !clientClosed {
				if err := writeSSEComment(w, "ping"); err != nil {
					clientClosed = true
				} else {
					flusher.Flush()
				}
			}
		}
	}

	err = cmd.Wait()
	if err != nil {
		if !clientClosed {
			_ = writeSSE(w, "done", map[string]any{"ok": false, "error": err.Error()})
			flusher.Flush()
		}
		return
	}
	if !clientClosed {
		_ = writeSSE(w, "done", map[string]any{"ok": true})
		flusher.Flush()
	}
}

func writeSSE(w io.Writer, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return err
	}
	return nil
}

func writeSSEComment(w io.Writer, message string) error {
	_, err := fmt.Fprintf(w, ": %s\n\n", message)
	return err
}

func requestAllowed(r *http.Request, allowNonLoopback bool) bool {
	if allowNonLoopback {
		return true
	}
	return isLoopbackRequest(r)
}

func isLoopbackRequest(r *http.Request) bool {
	host := strings.TrimSpace(r.RemoteAddr)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	ip := net.ParseIP(host)
	if ip != nil {
		return ip.IsLoopback()
	}
	return strings.EqualFold(host, "localhost")
}

type pullProgressTracker struct {
	lastBytes float64
	lastAt    time.Time
}

var (
	reRatio                = regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]+)?)\s*([kmgt]?i?b)\s*/\s*([0-9]+(?:\.[0-9]+)?)\s*([kmgt]?i?b)`)
	reSpeed                = regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]+)?)\s*([kmgt]?i?b/s)`)
	sizeToBytesMultipliers = map[string]float64{
		"kb":  1000,
		"mb":  1000 * 1000,
		"gb":  1000 * 1000 * 1000,
		"tb":  1000 * 1000 * 1000 * 1000,
		"kib": 1024,
		"mib": 1024 * 1024,
		"gib": 1024 * 1024 * 1024,
		"tib": 1024 * 1024 * 1024 * 1024,
	}
)

func newPullProgressTracker() *pullProgressTracker {
	return &pullProgressTracker{}
}

func (p *pullProgressTracker) Update(line string) (map[string]any, bool) {
	lowerLine := strings.ToLower(line)
	if !strings.Contains(lowerLine, "pull") && !strings.Contains(lowerLine, "download") {
		return nil, false
	}
	m := reRatio.FindStringSubmatch(line)
	if len(m) != 5 {
		return nil, false
	}
	cur := parseSizeToBytes(m[1], m[2])
	total := parseSizeToBytes(m[3], m[4])
	if total <= 0 {
		return nil, false
	}
	percent := math.Min(100, (cur/total)*100)
	now := time.Now()
	speedBps := 0.0

	if sm := reSpeed.FindStringSubmatch(line); len(sm) == 3 {
		speedBps = parseSizeToBytes(sm[1], strings.TrimSuffix(strings.ToLower(sm[2]), "/s"))
	} else if !p.lastAt.IsZero() && cur >= p.lastBytes {
		delta := cur - p.lastBytes
		sec := now.Sub(p.lastAt).Seconds()
		if sec > 0 {
			speedBps = delta / sec
		}
	}
	p.lastBytes = cur
	p.lastAt = now

	return map[string]any{
		"percent":   percent,
		"current":   humanBytes(cur),
		"total":     humanBytes(total),
		"speed":     humanRate(speedBps),
		"raw_line":  line,
		"phase":     "pull",
		"speed_bps": speedBps,
	}, true
}

func parseSizeToBytes(rawNum string, rawUnit string) float64 {
	n, err := strconv.ParseFloat(strings.TrimSpace(rawNum), 64)
	if err != nil {
		return 0
	}
	unit := strings.ToLower(strings.TrimSpace(rawUnit))
	if unit == "b" {
		return n
	}
	m, ok := sizeToBytesMultipliers[unit]
	if !ok {
		return 0
	}
	return n * m
}

func humanBytes(v float64) string {
	if v <= 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	idx := 0
	for v >= 1000 && idx < len(units)-1 {
		v /= 1000
		idx++
	}
	return fmt.Sprintf("%.2f %s", v, units[idx])
}

func humanRate(v float64) string {
	if v <= 0 {
		return "-"
	}
	return humanBytes(v) + "/s"
}
