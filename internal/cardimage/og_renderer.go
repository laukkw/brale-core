package cardimage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"brale-core/internal/decision/decisionfmt"
)

type ImageAsset struct {
	Data        []byte
	Filename    string
	ContentType string
	Caption     string
	AltText     string
}

type OGRenderer struct {
	nodeBin   string
	scriptDir string
	script    string
}

type ogPayload struct {
	Symbol    string      `json:"symbol"`
	RawBlocks ogRawBlocks `json:"raw_blocks"`
}

type ogRawBlocks struct {
	Gate  ogGate  `json:"gate"`
	Agent ogAgent `json:"agent"`
}

type ogGate struct {
	DecisionAction string                `json:"decision_action"`
	DecisionText   string                `json:"decision_text"`
	Direction      string                `json:"direction"`
	Grade          int                   `json:"grade"`
	Reason         string                `json:"reason"`
	ReasonCode     string                `json:"reason_code"`
	Tradeable      bool                  `json:"tradeable"`
	StopStep       string                `json:"stop_step,omitempty"`
	RuleName       string                `json:"rule_name,omitempty"`
	ActionBefore   string                `json:"action_before,omitempty"`
	SieveAction    string                `json:"sieve_action,omitempty"`
	SieveReason    string                `json:"sieve_reason,omitempty"`
	Execution      map[string]any        `json:"execution,omitempty"`
	Consensus      *ogDirectionConsensus `json:"direction_consensus,omitempty"`
	Trace          []ogGateTraceStep     `json:"trace,omitempty"`
}

type ogGateTraceStep struct {
	Step   string `json:"step"`
	OK     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
}

type ogDirectionConsensus struct {
	Score               *float64 `json:"score,omitempty"`
	Confidence          *float64 `json:"confidence,omitempty"`
	ScoreThreshold      *float64 `json:"score_threshold,omitempty"`
	ConfidenceThreshold *float64 `json:"confidence_threshold,omitempty"`
	ScorePassed         *bool    `json:"score_passed,omitempty"`
	ConfidencePassed    *bool    `json:"confidence_passed,omitempty"`
	Passed              *bool    `json:"passed,omitempty"`
}

type ogAgent struct {
	Indicator ogIndicator `json:"indicator"`
	Mechanics ogMechanics `json:"mechanics"`
	Structure ogStructure `json:"structure"`
}

type ogIndicator struct {
	Expansion          string  `json:"expansion"`
	Alignment          string  `json:"alignment"`
	Noise              string  `json:"noise"`
	MomentumDetail     string  `json:"momentum_detail"`
	ConflictDetail     string  `json:"conflict_detail"`
	MovementScore      float64 `json:"movement_score"`
	MovementConfidence float64 `json:"movement_confidence"`
}

type ogMechanics struct {
	LeverageState      string  `json:"leverage_state"`
	Crowding           string  `json:"crowding"`
	RiskLevel          string  `json:"risk_level"`
	OpenInterestCtx    string  `json:"open_interest_context"`
	AnomalyDetail      string  `json:"anomaly_detail"`
	MovementScore      float64 `json:"movement_score"`
	MovementConfidence float64 `json:"movement_confidence"`
}

type ogStructure struct {
	Regime             string  `json:"regime"`
	LastBreak          string  `json:"last_break"`
	Quality            string  `json:"quality"`
	Pattern            string  `json:"pattern"`
	VolumeAction       string  `json:"volume_action"`
	CandleReaction     string  `json:"candle_reaction"`
	MovementScore      float64 `json:"movement_score"`
	MovementConfidence float64 `json:"movement_confidence"`
}

func NewOGRenderer() *OGRenderer {
	script := defaultScriptPath()
	scriptDir := filepath.Dir(script)
	nodeBin := strings.TrimSpace(os.Getenv("BRALE_NOTIFY_NODE_BIN"))
	if nodeBin == "" {
		nodeBin = "node"
	}
	return &OGRenderer{nodeBin: nodeBin, scriptDir: scriptDir, script: script}
}

func defaultScriptPath() string {
	if custom := strings.TrimSpace(os.Getenv("BRALE_NOTIFY_OG_SCRIPT")); custom != "" {
		return custom
	}
	if cwd, err := os.Getwd(); err == nil {
		candidate := filepath.Join(cwd, "webui", "og-card-demo", "render.mjs")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if exe, err := os.Executable(); err == nil {
		base := filepath.Dir(exe)
		candidates := []string{
			filepath.Join(base, "..", "..", "app", "webui", "og-card-demo", "render.mjs"),
			filepath.Join(base, "..", "webui", "og-card-demo", "render.mjs"),
		}
		for _, candidate := range candidates {
			resolved := filepath.Clean(candidate)
			if _, err := os.Stat(resolved); err == nil {
				return resolved
			}
		}
	}
	root := repoRoot()
	return filepath.Join(root, "webui", "og-card-demo", "render.mjs")
}

func (r *OGRenderer) RenderDecision(ctx context.Context, input decisionfmt.DecisionInput, report decisionfmt.DecisionReport) (*ImageAsset, error) {
	if r == nil {
		return nil, fmt.Errorf("renderer is nil")
	}
	payload, err := buildPayload(input, report)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(r.script); err != nil {
		return nil, fmt.Errorf("og render script unavailable: %w", err)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return r.renderPayload(ctx, raw, report.Symbol, report.SnapshotID, decisionfmt.ResolveExecutionTitle(report))
}

func (r *OGRenderer) RenderRuntimePayload(ctx context.Context, symbol string, snapshotID uint, gate map[string]any, agent map[string]any, title string) (*ImageAsset, error) {
	if r == nil {
		return nil, fmt.Errorf("renderer is nil")
	}
	if _, err := os.Stat(r.script); err != nil {
		return nil, fmt.Errorf("og render script unavailable: %w", err)
	}
	if len(gate) == 0 {
		return nil, fmt.Errorf("runtime gate payload is empty")
	}
	if len(agent) == 0 {
		return nil, fmt.Errorf("runtime agent payload is empty")
	}
	payload := map[string]any{
		"symbol": symbol,
		"raw_blocks": map[string]any{
			"gate":  gate,
			"agent": agent,
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return r.renderPayload(ctx, raw, symbol, snapshotID, title)
}

func (r *OGRenderer) renderPayload(ctx context.Context, raw []byte, symbol string, snapshotID uint, title string) (*ImageAsset, error) {
	tmpDir, err := os.MkdirTemp("", "brale-og-render-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)
	inputPath := filepath.Join(tmpDir, "input.json")
	outputPath := filepath.Join(tmpDir, "output.png")
	if err := os.WriteFile(inputPath, raw, 0o644); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, r.nodeBin, r.script, inputPath, outputPath)
	cmd.Dir = r.scriptDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("render og image failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	pngBytes, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, err
	}
	filename := fmt.Sprintf("%s-snapshot-%d.png", strings.ToLower(strings.TrimSpace(symbol)), snapshotID)
	caption := fmt.Sprintf("[%s][snapshot:%d] %s", symbol, snapshotID, title)
	return &ImageAsset{
		Data:        pngBytes,
		Filename:    filename,
		ContentType: "image/png",
		Caption:     caption,
		AltText:     caption,
	}, nil
}

func buildPayload(input decisionfmt.DecisionInput, report decisionfmt.DecisionReport) (ogPayload, error) {
	payload := ogPayload{
		Symbol: input.Symbol,
		RawBlocks: ogRawBlocks{
			Gate: ogGate{
				DecisionAction: report.Gate.Overall.DecisionAction,
				DecisionText:   report.Gate.Overall.DecisionText,
				Direction:      report.Gate.Overall.Direction,
				Grade:          report.Gate.Overall.Grade,
				Reason:         report.Gate.Overall.Reason,
				ReasonCode:     report.Gate.Overall.ReasonCode,
				Tradeable:      report.Gate.Overall.Tradeable,
			},
		},
	}
	payload.RawBlocks.Gate.Consensus = parseDirectionConsensus(report.Gate.Derived)
	payload.RawBlocks.Gate.StopStep = readDerivedString(report.Gate.Derived, "gate_stop_step")
	payload.RawBlocks.Gate.ActionBefore = readDerivedString(report.Gate.Derived, "gate_action_before_sieve")
	payload.RawBlocks.Gate.SieveAction = readDerivedString(report.Gate.Derived, "sieve_action")
	payload.RawBlocks.Gate.SieveReason = readDerivedString(report.Gate.Derived, "sieve_reason")
	payload.RawBlocks.Gate.Execution = readDerivedMap(report.Gate.Derived, "execution")
	payload.RawBlocks.Gate.Trace = parseGateTrace(report.Gate.Derived)
	if report.Gate.RuleHit != nil {
		payload.RawBlocks.Gate.RuleName = strings.TrimSpace(report.Gate.RuleHit.Name)
	}
	for _, agent := range input.Agents {
		switch normalizeStage(agent.Stage) {
		case "indicator":
			if len(agent.OutputJSON) == 0 {
				continue
			}
			if err := json.Unmarshal(agent.OutputJSON, &payload.RawBlocks.Agent.Indicator); err != nil {
				return ogPayload{}, fmt.Errorf("decode indicator agent output: %w", err)
			}
		case "mechanics":
			if len(agent.OutputJSON) == 0 {
				continue
			}
			if err := json.Unmarshal(agent.OutputJSON, &payload.RawBlocks.Agent.Mechanics); err != nil {
				return ogPayload{}, fmt.Errorf("decode mechanics agent output: %w", err)
			}
		case "structure":
			if len(agent.OutputJSON) == 0 {
				continue
			}
			if err := json.Unmarshal(agent.OutputJSON, &payload.RawBlocks.Agent.Structure); err != nil {
				return ogPayload{}, fmt.Errorf("decode structure agent output: %w", err)
			}
		}
	}
	return payload, nil
}

func readDerivedMap(derived map[string]any, key string) map[string]any {
	if len(derived) == 0 {
		return nil
	}
	value, ok := derived[key]
	if !ok || value == nil {
		return nil
	}
	raw, ok := value.(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make(map[string]any, len(raw))
	for k, v := range raw {
		out[k] = v
	}
	return out
}

func readDerivedString(derived map[string]any, key string) string {
	if len(derived) == 0 {
		return ""
	}
	value, ok := derived[key]
	if !ok || value == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "<nil>" {
		return ""
	}
	return text
}

func parseGateTrace(derived map[string]any) []ogGateTraceStep {
	if len(derived) == 0 {
		return nil
	}
	rawTrace, ok := derived["gate_trace"].([]any)
	if !ok || len(rawTrace) == 0 {
		return nil
	}
	steps := make([]ogGateTraceStep, 0, len(rawTrace))
	for _, raw := range rawTrace {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		step := strings.TrimSpace(fmt.Sprint(entry["step"]))
		if step == "" || step == "<nil>" {
			continue
		}
		reason := strings.TrimSpace(fmt.Sprint(entry["reason"]))
		if reason == "<nil>" {
			reason = ""
		}
		steps = append(steps, ogGateTraceStep{
			Step:   step,
			OK:     parseTraceOK(entry["ok"]),
			Reason: reason,
		})
	}
	if len(steps) == 0 {
		return nil
	}
	return steps
}

func parseTraceOK(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case float64:
		return v != 0
	case float32:
		return v != 0
	case int:
		return v != 0
	case int64:
		return v != 0
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return false
	}
}

func parseDirectionConsensus(derived map[string]any) *ogDirectionConsensus {
	if len(derived) == 0 {
		return nil
	}
	raw, ok := derived["direction_consensus"]
	if !ok {
		return nil
	}
	consensus, ok := raw.(map[string]any)
	if !ok || len(consensus) == 0 {
		return nil
	}
	out := &ogDirectionConsensus{}
	if v, ok := toFloatPtr(consensus["score"]); ok {
		out.Score = v
	}
	if v, ok := toFloatPtr(consensus["confidence"]); ok {
		out.Confidence = v
	}
	if v, ok := toFloatPtr(consensus["score_threshold"]); ok {
		out.ScoreThreshold = v
	}
	if v, ok := toFloatPtr(consensus["confidence_threshold"]); ok {
		out.ConfidenceThreshold = v
	}
	if v, ok := toBoolPtr(consensus["score_passed"]); ok {
		out.ScorePassed = v
	}
	if v, ok := toBoolPtr(consensus["confidence_passed"]); ok {
		out.ConfidencePassed = v
	}
	if v, ok := toBoolPtr(consensus["passed"]); ok {
		out.Passed = v
	}
	if out.Score == nil && out.Confidence == nil && out.ScoreThreshold == nil && out.ConfidenceThreshold == nil && out.ScorePassed == nil && out.ConfidencePassed == nil && out.Passed == nil {
		return nil
	}
	return out
}

func toFloatPtr(v any) (*float64, bool) {
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "" {
		return nil, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, false
	}
	return &f, true
}

func toBoolPtr(v any) (*bool, bool) {
	b, ok := v.(bool)
	if ok {
		return &b, true
	}
	s := strings.TrimSpace(strings.ToLower(fmt.Sprint(v)))
	if s == "" {
		return nil, false
	}
	if s == "true" {
		value := true
		return &value, true
	}
	if s == "false" {
		value := false
		return &value, true
	}
	return nil, false
}

func normalizeStage(stage string) string {
	s := strings.ToLower(strings.TrimSpace(stage))
	switch {
	case strings.Contains(s, "indicator"):
		return "indicator"
	case strings.Contains(s, "mechanics"):
		return "mechanics"
	case strings.Contains(s, "structure"):
		return "structure"
	default:
		return s
	}
}

func repoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if ok {
		return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	}
	if cwd, err := os.Getwd(); err == nil {
		return filepath.Clean(cwd)
	}
	return "."
}
