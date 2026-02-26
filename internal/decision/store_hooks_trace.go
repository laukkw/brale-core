package decision

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"brale-core/internal/store"
)

func (h StoreHooks) writeRoundTraceMarkdown(ctx context.Context, gateRec *store.GateEventRecord) (string, error) {
	if h.Store == nil {
		return "", fmt.Errorf("store is required")
	}
	if gateRec == nil {
		return "", fmt.Errorf("gate record is required")
	}
	if strings.TrimSpace(gateRec.Symbol) == "" {
		return "", fmt.Errorf("gate record symbol is required")
	}
	if gateRec.SnapshotID == 0 {
		return "", fmt.Errorf("gate record snapshot_id is required")
	}

	agentEvents, err := h.Store.ListAgentEventsBySnapshot(ctx, gateRec.Symbol, gateRec.SnapshotID)
	if err != nil {
		return "", err
	}
	providerEvents, err := h.Store.ListProviderEventsBySnapshot(ctx, gateRec.Symbol, gateRec.SnapshotID)
	if err != nil {
		return "", err
	}

	path := h.roundTraceFilePath(gateRec.Symbol, gateRec.SnapshotID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	body := renderRoundTraceMarkdown(h, gateRec, agentEvents, providerEvents)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func (h StoreHooks) roundTraceFilePath(symbol string, snapshotID uint) string {
	baseDir := strings.TrimSpace(h.TraceDir)
	if baseDir == "" {
		baseDir = filepath.Join("data", "llm-traces")
	}
	safeSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	if safeSymbol == "" {
		safeSymbol = "UNKNOWN"
	}
	return filepath.Join(baseDir, safeSymbol, fmt.Sprintf("%d.md", snapshotID))
}

func renderRoundTraceMarkdown(h StoreHooks, gateRec *store.GateEventRecord, agentEvents []store.AgentEventRecord, providerEvents []store.ProviderEventRecord) string {
	agentEvents = append([]store.AgentEventRecord(nil), agentEvents...)
	providerEvents = append([]store.ProviderEventRecord(nil), providerEvents...)
	sort.SliceStable(agentEvents, func(i, j int) bool {
		if agentEvents[i].Timestamp == agentEvents[j].Timestamp {
			return stageOrder(agentEvents[i].Stage) < stageOrder(agentEvents[j].Stage)
		}
		return agentEvents[i].Timestamp < agentEvents[j].Timestamp
	})
	sort.SliceStable(providerEvents, func(i, j int) bool {
		if providerEvents[i].Timestamp == providerEvents[j].Timestamp {
			return stageOrder(providerEvents[i].Role) < stageOrder(providerEvents[j].Role)
		}
		return providerEvents[i].Timestamp < providerEvents[j].Timestamp
	})

	timestamp := time.Unix(gateRec.Timestamp, 0).UTC().Format(time.RFC3339)
	latencyMS := extractLatencyMS(gateRec.DerivedJSON)
	logPath := strings.TrimSpace(h.TraceLogPath)
	if logPath == "" {
		logPath = "brale-core.log"
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("trace_version: v1\n")
	b.WriteString("snapshot_id: ")
	b.WriteString(strconv.FormatUint(uint64(gateRec.SnapshotID), 10))
	b.WriteString("\n")
	b.WriteString("symbol: ")
	b.WriteString(gateRec.Symbol)
	b.WriteString("\n")
	b.WriteString("timestamp: ")
	b.WriteString(timestamp)
	b.WriteString("\n")
	b.WriteString("pipeline_latency_ms: ")
	b.WriteString(formatFloat(latencyMS))
	b.WriteString("\n\n")
	b.WriteString("global_tradeable: ")
	b.WriteString(strconv.FormatBool(gateRec.GlobalTradeable))
	b.WriteString("\n")
	b.WriteString("gate_reason: ")
	b.WriteString(yamlScalar(gateRec.GateReason))
	b.WriteString("\n")
	b.WriteString("direction: ")
	b.WriteString(yamlScalar(gateRec.Direction))
	b.WriteString("\n")
	b.WriteString("decision_action: ")
	b.WriteString(yamlScalar(gateRec.DecisionAction))
	b.WriteString("\n")
	b.WriteString("grade: ")
	b.WriteString(strconv.Itoa(gateRec.Grade))
	b.WriteString("\n\n")
	b.WriteString("source_version: ")
	b.WriteString(yamlScalar(gateRec.SourceVersion))
	b.WriteString("\n")
	b.WriteString("system_config_hash: ")
	b.WriteString(yamlScalar(gateRec.SystemConfigHash))
	b.WriteString("\n")
	b.WriteString("strategy_config_hash: ")
	b.WriteString(yamlScalar(gateRec.StrategyConfigHash))
	b.WriteString("\n\n")
	b.WriteString("agent_events_count: ")
	b.WriteString(strconv.Itoa(len(agentEvents)))
	b.WriteString("\n")
	b.WriteString("provider_events_count: ")
	b.WriteString(strconv.Itoa(len(providerEvents)))
	b.WriteString("\n")
	b.WriteString("redacted: ")
	b.WriteString(strconv.FormatBool(h.TraceRedacted))
	b.WriteString("\n")
	b.WriteString("---\n\n")

	b.WriteString("# Round Trace - ")
	b.WriteString(gateRec.Symbol)
	b.WriteString(" / ")
	b.WriteString(strconv.FormatUint(uint64(gateRec.SnapshotID), 10))
	b.WriteString("\n\n")

	b.WriteString("## Gate Result\n")
	b.WriteString("- global_tradeable: `")
	b.WriteString(strconv.FormatBool(gateRec.GlobalTradeable))
	b.WriteString("`\n")
	b.WriteString("- gate_reason: `")
	b.WriteString(gateRec.GateReason)
	b.WriteString("`\n")
	b.WriteString("- direction: `")
	b.WriteString(gateRec.Direction)
	b.WriteString("`\n")
	b.WriteString("- decision_action: `")
	b.WriteString(gateRec.DecisionAction)
	b.WriteString("`\n")
	b.WriteString("- grade: `")
	b.WriteString(strconv.Itoa(gateRec.Grade))
	b.WriteString("`\n\n")

	b.WriteString("## Provider Verdicts\n")
	appendProviderVerdict(&b, providerEvents, "indicator")
	appendProviderVerdict(&b, providerEvents, "structure")
	appendProviderVerdict(&b, providerEvents, "mechanics")

	b.WriteString("## Raw LLM I/O\n\n")
	appendAgentSection(&b, agentEvents, "indicator")
	appendAgentSection(&b, agentEvents, "structure")
	appendAgentSection(&b, agentEvents, "mechanics")
	appendProviderSection(&b, providerEvents, "indicator")
	appendProviderSection(&b, providerEvents, "structure")
	appendProviderSection(&b, providerEvents, "mechanics")

	b.WriteString("## Gate Rule Context\n")
	b.WriteString("### Provider Refs JSON\n")
	b.WriteString("```json\n")
	b.WriteString(prettyJSON(gateRec.ProviderRefsJSON))
	b.WriteString("\n```\n\n")
	b.WriteString("### Rule Hit JSON\n")
	b.WriteString("```json\n")
	b.WriteString(prettyJSON(gateRec.RuleHitJSON))
	b.WriteString("\n```\n\n")
	b.WriteString("### Derived JSON\n")
	b.WriteString("```json\n")
	b.WriteString(prettyJSON(gateRec.DerivedJSON))
	b.WriteString("\n```\n\n")

	b.WriteString("## Evidence\n")
	b.WriteString("- gate_event_id: `")
	b.WriteString(strconv.FormatUint(uint64(gateRec.ID), 10))
	b.WriteString("`\n")
	b.WriteString("- provider_event_ids: `")
	b.WriteString(joinProviderIDs(providerEvents))
	b.WriteString("`\n")
	b.WriteString("- agent_event_ids: `")
	b.WriteString(joinAgentIDs(agentEvents))
	b.WriteString("`\n")
	b.WriteString("- log_path: `")
	b.WriteString(logPath)
	b.WriteString("`\n\n")

	b.WriteString("## Notes\n")
	b.WriteString("- generated_at: `")
	b.WriteString(time.Now().UTC().Format(time.RFC3339))
	b.WriteString("`\n")
	b.WriteString("- generator: `brale-core/store-hooks-trace-v1`\n")

	return b.String()
}

func appendProviderVerdict(b *strings.Builder, events []store.ProviderEventRecord, role string) {
	b.WriteString("### ")
	b.WriteString(role)
	b.WriteString("\n")
	rec, ok := findProviderByRole(events, role)
	if !ok {
		b.WriteString("- tradeable: `false`\n")
		b.WriteString("- output: `missing`\n\n")
		return
	}
	b.WriteString("- tradeable: `")
	b.WriteString(strconv.FormatBool(rec.Tradeable))
	b.WriteString("`\n")
	b.WriteString("- output:\n```json\n")
	b.WriteString(prettyJSON(rec.OutputJSON))
	b.WriteString("\n```\n\n")
}

func appendAgentSection(b *strings.Builder, events []store.AgentEventRecord, stage string) {
	rec, ok := findAgentByStage(events, stage)
	b.WriteString("### Agent - ")
	b.WriteString(stage)
	b.WriteString("\n")
	if !ok {
		b.WriteString("- missing: `true`\n\n")
		return
	}
	b.WriteString("- phase: `agent`\n")
	b.WriteString("- stage: `")
	b.WriteString(stage)
	b.WriteString("`\n")
	b.WriteString("- event_id: `")
	b.WriteString(strconv.FormatUint(uint64(rec.ID), 10))
	b.WriteString("`\n")
	b.WriteString("- timestamp: `")
	b.WriteString(time.Unix(rec.Timestamp, 0).UTC().Format(time.RFC3339))
	b.WriteString("`\n")
	b.WriteString("- fingerprint: `")
	b.WriteString(rec.Fingerprint)
	b.WriteString("`\n\n")
	b.WriteString("#### System Prompt\n```text\n")
	b.WriteString(strings.TrimSpace(rec.SystemPrompt))
	b.WriteString("\n```\n\n")
	b.WriteString("#### User Prompt\n```text\n")
	b.WriteString(strings.TrimSpace(rec.UserPrompt))
	b.WriteString("\n```\n\n")
	b.WriteString("#### Output JSON\n```json\n")
	b.WriteString(prettyJSON(rec.OutputJSON))
	b.WriteString("\n```\n\n---\n\n")
}

func appendProviderSection(b *strings.Builder, events []store.ProviderEventRecord, role string) {
	rec, ok := findProviderByRole(events, role)
	b.WriteString("### Provider - ")
	b.WriteString(role)
	b.WriteString("\n")
	if !ok {
		b.WriteString("- missing: `true`\n\n")
		return
	}
	b.WriteString("- phase: `provider`\n")
	b.WriteString("- role: `")
	b.WriteString(role)
	b.WriteString("`\n")
	b.WriteString("- event_id: `")
	b.WriteString(strconv.FormatUint(uint64(rec.ID), 10))
	b.WriteString("`\n")
	b.WriteString("- timestamp: `")
	b.WriteString(time.Unix(rec.Timestamp, 0).UTC().Format(time.RFC3339))
	b.WriteString("`\n")
	b.WriteString("- fingerprint: `")
	b.WriteString(rec.Fingerprint)
	b.WriteString("`\n")
	b.WriteString("- tradeable: `")
	b.WriteString(strconv.FormatBool(rec.Tradeable))
	b.WriteString("`\n\n")
	b.WriteString("#### System Prompt\n```text\n")
	b.WriteString(strings.TrimSpace(rec.SystemPrompt))
	b.WriteString("\n```\n\n")
	b.WriteString("#### User Prompt\n```text\n")
	b.WriteString(strings.TrimSpace(rec.UserPrompt))
	b.WriteString("\n```\n\n")
	b.WriteString("#### Output JSON\n```json\n")
	b.WriteString(prettyJSON(rec.OutputJSON))
	b.WriteString("\n```\n\n---\n\n")
}

func findAgentByStage(events []store.AgentEventRecord, stage string) (store.AgentEventRecord, bool) {
	for _, rec := range events {
		if rec.Stage == stage {
			return rec, true
		}
	}
	return store.AgentEventRecord{}, false
}

func findProviderByRole(events []store.ProviderEventRecord, role string) (store.ProviderEventRecord, bool) {
	for _, rec := range events {
		if rec.Role == role {
			return rec, true
		}
	}
	return store.ProviderEventRecord{}, false
}

func prettyJSON(raw []byte) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "{}"
	}
	var anyVal any
	if err := json.Unmarshal([]byte(trimmed), &anyVal); err != nil {
		return trimmed
	}
	buf, err := json.MarshalIndent(anyVal, "", "  ")
	if err != nil {
		return trimmed
	}
	return string(buf)
}

func stageOrder(stage string) int {
	switch stage {
	case "indicator":
		return 1
	case "structure":
		return 2
	case "mechanics":
		return 3
	default:
		return 99
	}
}

func extractLatencyMS(raw []byte) float64 {
	if len(raw) == 0 {
		return 0
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return 0
	}
	for _, key := range []string{"pipeline_latency_ms", "pipeline_latency", "latency_ms", "latency"} {
		if v, ok := data[key]; ok {
			switch n := v.(type) {
			case float64:
				return n
			case int:
				return float64(n)
			case int64:
				return float64(n)
			}
		}
	}
	return 0
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func yamlScalar(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "\"\""
	}
	if strings.ContainsAny(v, ":#[]{}\"'\n\r\t") {
		return strconv.Quote(v)
	}
	return v
}

func joinProviderIDs(records []store.ProviderEventRecord) string {
	if len(records) == 0 {
		return ""
	}
	parts := make([]string, 0, len(records))
	for _, rec := range records {
		parts = append(parts, strconv.FormatUint(uint64(rec.ID), 10))
	}
	return strings.Join(parts, ",")
}

func joinAgentIDs(records []store.AgentEventRecord) string {
	if len(records) == 0 {
		return ""
	}
	parts := make([]string, 0, len(records))
	for _, rec := range records {
		parts = append(parts, strconv.FormatUint(uint64(rec.ID), 10))
	}
	return strings.Join(parts, ",")
}
