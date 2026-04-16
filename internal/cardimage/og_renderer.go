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
	"time"

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
	CardType     string      `json:"card_type,omitempty"`
	Symbol       string      `json:"symbol"`
	CurrentPrice float64     `json:"current_price,omitempty"`
	RawBlocks    ogRawBlocks `json:"raw_blocks"`
	Data         any         `json:"data,omitempty"`
}

type ogRawBlocks struct {
	Gate  ogGate  `json:"gate"`
	Agent ogAgent `json:"agent"`
}

type ogGate struct {
	DecisionAction   string                `json:"decision_action"`
	DecisionText     string                `json:"decision_text"`
	Direction        string                `json:"direction"`
	Grade            int                   `json:"grade"`
	Reason           string                `json:"reason"`
	ReasonCode       string                `json:"reason_code"`
	Tradeable        bool                  `json:"tradeable"`
	StopStep         string                `json:"stop_step,omitempty"`
	RuleName         string                `json:"rule_name,omitempty"`
	ActionBefore     string                `json:"action_before,omitempty"`
	SieveAction      string                `json:"sieve_action,omitempty"`
	SieveReason      string                `json:"sieve_reason,omitempty"`
	Execution        map[string]any        `json:"execution,omitempty"`
	Consensus        *ogDirectionConsensus `json:"direction_consensus,omitempty"`
	Trace            []ogGateTraceStep     `json:"trace,omitempty"`
	SetupQuality     float64               `json:"setup_quality,omitempty"`
	RiskPenalty      float64               `json:"risk_penalty,omitempty"`
	EntryEdge        float64               `json:"entry_edge,omitempty"`
	QualityThreshold float64               `json:"quality_threshold,omitempty"`
	EdgeThreshold    float64               `json:"edge_threshold,omitempty"`
	ScriptName       string                `json:"script_name,omitempty"`
	ScriptBonus      float64               `json:"script_bonus,omitempty"`
	ReasonCategory   string                `json:"reason_category,omitempty"`
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

func (r *OGRenderer) RenderCard(ctx context.Context, cardType string, symbol string, data map[string]any, title string) (*ImageAsset, error) {
	if r == nil {
		return nil, fmt.Errorf("renderer is nil")
	}
	if _, err := os.Stat(r.script); err != nil {
		return nil, fmt.Errorf("og render script unavailable: %w", err)
	}
	translateCardData(data)
	payload := map[string]any{
		"card_type": strings.TrimSpace(cardType),
		"symbol":    strings.TrimSpace(symbol),
		"data":      data,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return r.renderCardPayload(ctx, raw, cardType, symbol, title)
}

func (r *OGRenderer) renderCardPayload(ctx context.Context, raw []byte, cardType string, symbol string, title string) (*ImageAsset, error) {
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
		return nil, fmt.Errorf("render og image (%s) failed: %w (%s)", cardType, err, strings.TrimSpace(string(out)))
	}
	pngBytes, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, err
	}
	ts := fmt.Sprintf("%d", time.Now().Unix())
	filename := fmt.Sprintf("%s-%s-%s.png", strings.ToLower(strings.TrimSpace(symbol)), strings.ToLower(cardType), ts)
	caption := title
	if caption == "" {
		caption = fmt.Sprintf("[%s][%s]", symbol, cardType)
	}
	return &ImageAsset{
		Data:        pngBytes,
		Filename:    filename,
		ContentType: "image/png",
		Caption:     caption,
		AltText:     caption,
	}, nil
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
	payload.CurrentPrice = readDerivedFloat(report.Gate.Derived, "current_price")
	payload.RawBlocks.Gate.StopStep = readDerivedString(report.Gate.Derived, "gate_stop_step")
	payload.RawBlocks.Gate.ActionBefore = readDerivedString(report.Gate.Derived, "gate_action_before_sieve")
	payload.RawBlocks.Gate.SieveAction = readDerivedString(report.Gate.Derived, "sieve_action")
	payload.RawBlocks.Gate.SieveReason = readDerivedString(report.Gate.Derived, "sieve_reason")
	payload.RawBlocks.Gate.Execution = readDerivedMap(report.Gate.Derived, "execution")
	payload.RawBlocks.Gate.Trace = parseGateTrace(report.Gate.Derived)
	payload.RawBlocks.Gate.SetupQuality = readDerivedFloat(report.Gate.Derived, "setup_quality")
	payload.RawBlocks.Gate.RiskPenalty = readDerivedFloat(report.Gate.Derived, "risk_penalty")
	payload.RawBlocks.Gate.EntryEdge = readDerivedFloat(report.Gate.Derived, "entry_edge")
	payload.RawBlocks.Gate.QualityThreshold = readDerivedFloat(report.Gate.Derived, "quality_threshold")
	payload.RawBlocks.Gate.EdgeThreshold = readDerivedFloat(report.Gate.Derived, "edge_threshold")
	payload.RawBlocks.Gate.ScriptName = readDerivedString(report.Gate.Derived, "script_name")
	payload.RawBlocks.Gate.ScriptBonus = readDerivedFloat(report.Gate.Derived, "script_bonus")
	payload.RawBlocks.Gate.ReasonCategory = readDerivedString(report.Gate.Derived, "gate_reason_category")
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
	translatePayload(&payload)
	return payload, nil
}

// translatePayload pre-translates all English enum values and free-text fields
// in the OG payload to Chinese. After this, Node render.mjs only does layout.
func translatePayload(p *ogPayload) {
	tr := decisionfmt.TranslateValue
	ts := decisionfmt.TranslateSentence

	// Gate-level fields
	g := &p.RawBlocks.Gate
	g.DecisionAction = tr(g.DecisionAction)
	g.Direction = tr(g.Direction)
	g.Reason = ts(g.Reason)
	g.ReasonCode = tr(g.ReasonCode)
	g.StopStep = tr(g.StopStep)
	g.ActionBefore = tr(g.ActionBefore)
	g.SieveAction = tr(g.SieveAction)
	// Use TranslateValue (not TranslateSentence) so empty sieve_reason stays empty
	// instead of becoming "—" which would render as a spurious "· —" suffix.
	g.SieveReason = tr(g.SieveReason)
	g.ReasonCategory = tr(g.ReasonCategory)
	if g.Execution != nil {
		if reason, ok := g.Execution["blocked_reason"]; ok {
			if rs, ok := reason.(string); ok {
				g.Execution["blocked_reason"] = decisionfmt.TranslateExecutionBlockedReason(rs)
			}
		}
	}

	// Gate trace steps
	for i := range g.Trace {
		g.Trace[i].Step = tr(g.Trace[i].Step)
		g.Trace[i].Reason = tr(g.Trace[i].Reason)
	}

	// Indicator agent
	ind := &p.RawBlocks.Agent.Indicator
	ind.Expansion = tr(ind.Expansion)
	ind.Alignment = tr(ind.Alignment)
	ind.Noise = tr(ind.Noise)
	ind.MomentumDetail = ts(ind.MomentumDetail)
	ind.ConflictDetail = ts(ind.ConflictDetail)

	// Mechanics agent
	mech := &p.RawBlocks.Agent.Mechanics
	mech.LeverageState = tr(mech.LeverageState)
	mech.Crowding = tr(mech.Crowding)
	mech.RiskLevel = tr(mech.RiskLevel)
	mech.OpenInterestCtx = ts(mech.OpenInterestCtx)
	mech.AnomalyDetail = ts(mech.AnomalyDetail)

	// Structure agent
	str := &p.RawBlocks.Agent.Structure
	str.Regime = tr(str.Regime)
	str.LastBreak = tr(str.LastBreak)
	str.Quality = tr(str.Quality)
	str.Pattern = tr(str.Pattern)
	str.VolumeAction = ts(str.VolumeAction)
	str.CandleReaction = ts(str.CandleReaction)
}

// translateCardData translates known English fields in non-decision card data maps
// (position_open, position_close, risk_update, partial_close, etc.)
func translateCardData(data map[string]any) {
	if data == nil {
		return
	}
	tr := decisionfmt.TranslateValue
	// Translate known enum fields
	for _, key := range []string{"direction", "exit_reason", "exit_type", "stop_reason", "tighten_reason", "reason"} {
		if v, ok := data[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				data[key] = tr(s)
			}
		}
	}
	// Translate blocked_by list if present
	if blocked, ok := data["blocked_by"]; ok {
		if list, ok := blocked.([]any); ok {
			for i, item := range list {
				if s, ok := item.(string); ok {
					list[i] = decisionfmt.TranslateExecutionBlockedReason(s)
				}
			}
		}
	}
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

func readDerivedFloat(derived map[string]any, key string) float64 {
	if len(derived) == 0 {
		return 0
	}
	value, ok := derived[key]
	if !ok || value == nil {
		return 0
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" {
		return 0
	}
	out, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0
	}
	return out
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
