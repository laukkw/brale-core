// 决策查看模块，提供决策链 API、配置图 API，并内嵌 Roam 风格静态页面。
// Package decisionview 汇总符号决策链与配置，供审计前端使用。
package decisionview

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/decision/decisionfmt"
	"brale-core/internal/decision/newsoverlay"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/store"

	"go.uber.org/zap"
)

const (
	defaultRoundLimit = 30
	maxNewsNodes      = 80
)

//go:embed index.html styles.css main.js favicon-mask.svg vendor/*.js
var content embed.FS

type Server struct {
	Store         store.Store
	BasePath      string
	RoundLimit    int
	SystemConfig  config.SystemConfig
	SymbolConfigs map[string]ConfigBundle
}

func (s Server) Handler() (http.Handler, error) {
	if s.Store == nil {
		return nil, fmt.Errorf("store is required")
	}
	base := normalizeBasePath(s.BasePath)
	roundLimit := s.RoundLimit
	if roundLimit <= 0 {
		roundLimit = defaultRoundLimit
	}
	mux := http.NewServeMux()
	mux.Handle(join(base, "/api/symbols"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		symbols, err := s.Store.ListSymbols(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, mapSymbols(symbols))
	}))
	mux.Handle(join(base, "/api/chains"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		symbol := strings.TrimSpace(r.URL.Query().Get("symbol"))
		limit := roundLimit
		if qs := strings.TrimSpace(r.URL.Query().Get("limit")); qs != "" {
			if v, err := strconv.Atoi(qs); err == nil && v > 0 {
				limit = v
			}
		}
		var targets []string
		if symbol != "" {
			targets = []string{symbol}
		} else {
			syms, err := s.Store.ListSymbols(r.Context())
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			targets = syms
		}
		resp, err := buildResponse(r.Context(), s.Store, targets, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, resp)
	}))
	mux.Handle(join(base, "/api/config-graph"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, s.buildConfigGraph())
	}))
	fs := http.FileServer(http.FS(content))
	mux.Handle(base+"/", http.StripPrefix(base+"/", spaHandler(fs)))
	mux.Handle(base, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, base+"/", http.StatusTemporaryRedirect)
	}))
	return mux, nil
}

type Response struct {
	Symbols []SymbolChain `json:"symbols"`
}

type SymbolMeta struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type SymbolChain struct {
	ID     string  `json:"id"`
	Label  string  `json:"label"`
	Rounds []Round `json:"rounds"`
}

type Round struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	SnapshotID uint   `json:"snapshot_id,omitempty"`
	Timestamp  int64  `json:"timestamp,omitempty"`
	Nodes      []Node `json:"nodes"`
}

type Node struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Summary     string         `json:"summary,omitempty"`
	Stage       string         `json:"stage"`
	Type        string         `json:"type,omitempty"`
	AgentKey    string         `json:"agentKey,omitempty"`
	Refs        []string       `json:"refs,omitempty"`
	Input       any            `json:"input,omitempty"`
	Output      any            `json:"output,omitempty"`
	Meta        map[string]any `json:"meta,omitempty"`
	SymbolID    string         `json:"symbolId,omitempty"`
	SymbolLabel string         `json:"symbolLabel,omitempty"`
	RoundID     string         `json:"roundId,omitempty"`
	RoundLabel  string         `json:"roundLabel,omitempty"`
}

type gateDisplay struct {
	Overall   gateOverall          `json:"overall"`
	Providers []gateProviderStatus `json:"providers,omitempty"`
	Report    string               `json:"report"`
}

type gateOverall struct {
	Tradeable      bool   `json:"tradeable"`
	TradeableText  string `json:"tradeable_text"`
	DecisionAction string `json:"decision_action"`
	DecisionText   string `json:"decision_text"`
	Grade          int    `json:"grade"`
	Reason         string `json:"reason"`
	ReasonCode     string `json:"reason_code"`
	Direction      string `json:"direction"`
	ExpectedSnapID uint   `json:"expected_snapshot_id,omitempty"`
}

type gateProviderStatus struct {
	Role          string       `json:"role"`
	Tradeable     bool         `json:"tradeable"`
	TradeableText string       `json:"tradeable_text"`
	Factors       []gateFactor `json:"factors,omitempty"`
}

type gateFactor struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Status string `json:"status"`
	Raw    any    `json:"raw,omitempty"`
}

type nodeWithOrder struct {
	Node      Node
	Order     int
	Timestamp int64
}

type roundBuf struct {
	RoundID    string
	SnapshotID uint
	Timestamp  int64
	Nodes      []nodeWithOrder
}

type symbolEvents struct {
	providers []store.ProviderEventRecord
	agents    []store.AgentEventRecord
	gates     []store.GateEventRecord
}

type roundAccumulator struct {
	symbol string
	rounds map[uint]*roundBuf
}

func buildResponse(ctx context.Context, st store.Store, symbols []string, limit int) (Response, error) {
	out := Response{Symbols: make([]SymbolChain, 0, len(symbols))}
	for _, sym := range symbols {
		if strings.TrimSpace(sym) == "" {
			continue
		}
		chain, err := buildSymbolChain(ctx, st, sym, limit)
		if err != nil {
			return Response{}, err
		}
		out.Symbols = append(out.Symbols, chain)
	}
	return out, nil
}

func buildSymbolChain(ctx context.Context, st store.Store, symbol string, limit int) (SymbolChain, error) {
	logger := logging.FromContext(ctx).Named("decision-view").With(zap.String("symbol", symbol))
	df := decisionfmt.New()
	events, err := loadSymbolEvents(ctx, st, symbol, limit)
	if err != nil {
		return SymbolChain{}, err
	}
	acc := newRoundAccumulator(symbol)
	addProviderNodes(acc, df, logger, symbol, events.providers)
	addAgentNodes(acc, df, logger, symbol, events.agents)
	addGateNodes(acc, df, logger, symbol, events.gates)
	roundList := acc.sorted(limit)
	return buildSymbolChainFromRounds(df, symbol, roundList), nil
}

func loadSymbolEvents(ctx context.Context, st store.Store, symbol string, limit int) (symbolEvents, error) {
	providers, err := st.ListProviderEvents(ctx, symbol, limit*4)
	if err != nil {
		return symbolEvents{}, err
	}
	agents, err := st.ListAgentEvents(ctx, symbol, limit*4)
	if err != nil {
		return symbolEvents{}, err
	}
	gates, err := st.ListGateEvents(ctx, symbol, limit*2)
	if err != nil {
		return symbolEvents{}, err
	}
	return symbolEvents{providers: providers, agents: agents, gates: gates}, nil
}

func newRoundAccumulator(symbol string) *roundAccumulator {
	return &roundAccumulator{
		symbol: symbol,
		rounds: map[uint]*roundBuf{},
	}
}

func (r *roundAccumulator) add(snapshotID uint, ts int64, node nodeWithOrder) {
	if r.rounds[snapshotID] == nil {
		r.rounds[snapshotID] = &roundBuf{
			RoundID:    roundID(r.symbol, snapshotID),
			SnapshotID: snapshotID,
			Timestamp:  ts,
			Nodes:      []nodeWithOrder{},
		}
	}
	rb := r.rounds[snapshotID]
	if ts > rb.Timestamp {
		rb.Timestamp = ts
	}
	rb.Nodes = append(rb.Nodes, node)
}

func (r *roundAccumulator) sorted(limit int) []*roundBuf {
	roundList := make([]*roundBuf, 0, len(r.rounds))
	for _, rb := range r.rounds {
		roundList = append(roundList, rb)
	}
	sort.Slice(roundList, func(i, j int) bool {
		return roundList[i].Timestamp > roundList[j].Timestamp
	})
	if limit > 0 && len(roundList) > limit {
		roundList = roundList[:limit]
	}
	sort.Slice(roundList, func(i, j int) bool {
		return roundList[i].Timestamp < roundList[j].Timestamp
	})
	return roundList
}

func addProviderNodes(acc *roundAccumulator, df decisionfmt.Formatter, logger *zap.Logger, symbol string, providers []store.ProviderEventRecord) {
	for _, rec := range providers {
		displayOut, rawOut, err := df.HumanizeLLMOutput(json.RawMessage(rec.OutputJSON))
		if err != nil {
			logger.Warn("provider output decode failed", zap.Error(err), zap.Uint("provider_id", rec.ID))
		}
		meta := map[string]any{
			"fingerprint": rec.Fingerprint,
			"timestamp":   rec.Timestamp,
			"source":      rec.SourceVersion,
		}
		if rawOut != nil {
			meta["raw_output"] = rawOut
		}
		acc.add(rec.SnapshotID, rec.Timestamp, nodeWithOrder{
			Node: Node{
				ID:       fmt.Sprintf("%s-%d-provider-%d", symbol, rec.SnapshotID, rec.ID),
				Title:    fmt.Sprintf("Provider/%s", rec.Role),
				Stage:    "provider",
				Type:     "provider",
				AgentKey: rec.Role,
				Input: map[string]any{
					"system_prompt": rec.SystemPrompt,
					"user_prompt":   rec.UserPrompt,
				},
				Output: displayOut,
				Meta:   meta,
			},
			Order:     stageOrder("provider"),
			Timestamp: rec.Timestamp,
		})
	}
}

func addAgentNodes(acc *roundAccumulator, df decisionfmt.Formatter, logger *zap.Logger, symbol string, agents []store.AgentEventRecord) {
	for _, rec := range agents {
		displayOut, rawOut, err := df.HumanizeLLMOutput(json.RawMessage(rec.OutputJSON))
		if err != nil {
			logger.Warn("agent output decode failed", zap.Error(err), zap.Uint("agent_id", rec.ID))
		}
		meta := map[string]any{
			"fingerprint": rec.Fingerprint,
			"timestamp":   rec.Timestamp,
			"source":      rec.SourceVersion,
		}
		if rawOut != nil {
			meta["raw_output"] = rawOut
		}
		acc.add(rec.SnapshotID, rec.Timestamp, nodeWithOrder{
			Node: Node{
				ID:       fmt.Sprintf("%s-%d-agent-%s-%d", symbol, rec.SnapshotID, rec.Stage, rec.ID),
				Title:    fmt.Sprintf("Agent/%s", rec.Stage),
				Stage:    rec.Stage,
				Type:     "agent",
				AgentKey: rec.Stage,
				Input: map[string]any{
					"system_prompt": rec.SystemPrompt,
					"user_prompt":   rec.UserPrompt,
				},
				Output: displayOut,
				Meta:   meta,
			},
			Order:     stageOrder(rec.Stage),
			Timestamp: rec.Timestamp,
		})
	}
}

func addGateNodes(acc *roundAccumulator, df decisionfmt.Formatter, logger *zap.Logger, symbol string, gates []store.GateEventRecord) {
	for _, rec := range gates {
		gateEvent := decisionfmt.GateEvent{
			ID:               rec.ID,
			SnapshotID:       rec.SnapshotID,
			GlobalTradeable:  rec.GlobalTradeable,
			DecisionAction:   rec.DecisionAction,
			Grade:            rec.Grade,
			GateReason:       rec.GateReason,
			Direction:        rec.Direction,
			ProviderRefsJSON: json.RawMessage(rec.ProviderRefsJSON),
		}
		rpt, err := df.BuildGateReport(gateEvent)
		if err != nil {
			logger.Warn("gate refs parse failed", zap.Error(err), zap.Uint("gate_id", rec.ID))
			continue
		}
		gateOut := mapGateReportToDisplay(df, rpt)
		acc.add(rec.SnapshotID, rec.Timestamp, nodeWithOrder{
			Node: Node{
				ID:       fmt.Sprintf("%s-%d-gate-%d", symbol, rec.SnapshotID, rec.ID),
				Title:    "Gate",
				Stage:    "gate",
				Type:     "gate",
				AgentKey: "gate",
				Input:    nil,
				Output:   gateOut,
				Meta: map[string]any{
					"fingerprint": rec.Fingerprint,
					"timestamp":   rec.Timestamp,
					"source":      rec.SourceVersion,
				},
				Summary: rec.GateReason,
			},
			Order:     stageOrder("gate"),
			Timestamp: rec.Timestamp,
		})
	}
}

func buildSymbolChainFromRounds(df decisionfmt.Formatter, symbol string, roundList []*roundBuf) SymbolChain {
	result := SymbolChain{
		ID:     symbol,
		Label:  symbol,
		Rounds: make([]Round, 0, len(roundList)),
	}
	for idxRound, rb := range roundList {
		ensureMissingGate(df, symbol, rb)
		sortRoundNodes(rb.Nodes)
		refMap, gateIDs := buildStageRefs(rb.Nodes)
		label := fmt.Sprintf("Round %d", idxRound+1)
		nodes := attachRoundRefs(rb.Nodes, refMap, gateIDs, symbol, rb.RoundID, label)
		result.Rounds = append(result.Rounds, Round{
			ID:         rb.RoundID,
			Label:      label,
			SnapshotID: rb.SnapshotID,
			Timestamp:  rb.Timestamp,
			Nodes:      nodes,
		})
	}
	return result
}

type stageRefs struct {
	providers []string
	agents    []string
}

func ensureMissingGate(df decisionfmt.Formatter, symbol string, rb *roundBuf) {
	if rb == nil || len(rb.Nodes) == 0 {
		return
	}
	if hasGateNode(rb.Nodes) || !hasProviderNodes(rb.Nodes) {
		return
	}
	rpt := df.BuildMissingGateReport(rb.SnapshotID)
	gateOut := mapGateReportToDisplay(df, rpt)
	rb.Nodes = append(rb.Nodes, nodeWithOrder{
		Node: Node{
			ID:       fmt.Sprintf("%s-%d-gate-missing", symbol, rb.SnapshotID),
			Title:    "Gate (missing)",
			Stage:    "gate",
			Type:     "gate",
			AgentKey: "gate",
			Input:    nil,
			Output:   gateOut,
		},
		Order:     stageOrder("gate"),
		Timestamp: rb.Timestamp,
	})
}

func sortRoundNodes(nodes []nodeWithOrder) {
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Order == nodes[j].Order {
			return nodes[i].Timestamp < nodes[j].Timestamp
		}
		return nodes[i].Order < nodes[j].Order
	})
}

func hasProviderNodes(nodes []nodeWithOrder) bool {
	for _, n := range nodes {
		if isProviderNode(n.Node.Type) {
			return true
		}
	}
	return false
}

func buildStageRefs(nodes []nodeWithOrder) (map[string]*stageRefs, []string) {
	refMap := map[string]*stageRefs{}
	gateIDs := []string{}
	for _, nw := range nodes {
		stageKey := stageKeyForRefs(nw.Node)
		if _, ok := refMap[stageKey]; !ok {
			refMap[stageKey] = &stageRefs{}
		}
		if isProviderNode(nw.Node.Type) {
			refMap[stageKey].providers = append(refMap[stageKey].providers, nw.Node.ID)
		}
		if isAgentNode(nw.Node.Type) {
			refMap[stageKey].agents = append(refMap[stageKey].agents, nw.Node.ID)
		}
		if isGateNode(nw.Node.Type) {
			gateIDs = append(gateIDs, nw.Node.ID)
		}
	}
	return refMap, gateIDs
}

func attachRoundRefs(nodes []nodeWithOrder, refMap map[string]*stageRefs, gateIDs []string, symbol string, roundID string, label string) []Node {
	out := make([]Node, 0, len(nodes))
	for idx, nw := range nodes {
		n := nw.Node
		if len(n.Refs) == 0 {
			stageKey := stageKeyForRefs(n)
			switch {
			case isAgentNode(n.Type):
				if refs := refMap[stageKey].providers; len(refs) > 0 {
					n.Refs = append(n.Refs, refs...)
				}
			case isGateNode(n.Type):
				for _, stages := range refMap {
					n.Refs = append(n.Refs, stages.providers...)
				}
			case isExecutionNode(n.Type):
				if len(gateIDs) > 0 {
					n.Refs = append(n.Refs, gateIDs...)
				} else {
					for _, stages := range refMap {
						n.Refs = append(n.Refs, stages.providers...)
					}
				}
			}
			if len(n.Refs) == 0 && idx > 0 && !isProviderNode(n.Type) {
				n.Refs = []string{nodes[idx-1].Node.ID}
			}
		}
		n.SymbolID = symbol
		n.SymbolLabel = symbol
		n.RoundID = roundID
		n.RoundLabel = label
		out = append(out, n)
	}
	return out
}

func mapSymbols(symbols []string) []SymbolMeta {
	out := make([]SymbolMeta, 0, len(symbols))
	for _, s := range symbols {
		if strings.TrimSpace(s) == "" {
			continue
		}
		out = append(out, SymbolMeta{ID: s, Label: s})
	}
	return out
}

func stageOrder(stage string) int {
	s := strings.ToLower(stage)
	switch {
	case strings.Contains(s, "provider"):
		return 10
	case strings.Contains(s, "indicator"):
		return 20
	case strings.Contains(s, "structure"):
		return 22
	case strings.Contains(s, "mechanics"):
		return 26
	case strings.Contains(s, "gate"):
		return 30
	case strings.Contains(s, "exec"):
		return 40
	default:
		return 50
	}
}

func isProviderNode(t string) bool {
	return strings.Contains(strings.ToLower(t), "provider")
}

func isAgentNode(t string) bool {
	return strings.Contains(strings.ToLower(t), "agent")
}

func isGateNode(t string) bool {
	return strings.Contains(strings.ToLower(t), "gate")
}

func isExecutionNode(t string) bool {
	return strings.Contains(strings.ToLower(t), "exec")
}

func normalizeStageKey(stage string) string {
	s := strings.ToLower(stage)
	switch {
	case strings.Contains(s, "provider"):
		return "provider"
	case strings.Contains(s, "indicator"):
		return "indicator"
	case strings.Contains(s, "structure"):
		return "structure"
	case strings.Contains(s, "mechanics"):
		return "mechanics"
	case strings.Contains(s, "gate"):
		return "gate"
	case strings.Contains(s, "exec"):
		return "execution"
	default:
		return s
	}
}

func stageKeyForRefs(n Node) string {
	if strings.TrimSpace(n.AgentKey) != "" {
		return normalizeStageKey(n.AgentKey)
	}
	return normalizeStageKey(n.Stage)
}

func hasGateNode(nodes []nodeWithOrder) bool {
	for _, n := range nodes {
		if isGateNode(n.Node.Type) {
			return true
		}
	}
	return false
}

func mapGateReportToDisplay(df decisionfmt.Formatter, rpt decisionfmt.GateReport) gateDisplay {
	provs := make([]gateProviderStatus, len(rpt.Providers))
	for i, p := range rpt.Providers {
		factors := make([]gateFactor, len(p.Factors))
		for j, f := range p.Factors {
			factors[j] = gateFactor{
				Key:    f.Key,
				Label:  f.Label,
				Status: f.Status,
				Raw:    f.Raw,
			}
		}
		provs[i] = gateProviderStatus{
			Role:          p.Role,
			Tradeable:     p.Tradeable,
			TradeableText: p.TradeableText,
			Factors:       factors,
		}
	}
	return gateDisplay{
		Overall: gateOverall{
			Tradeable:      rpt.Overall.Tradeable,
			TradeableText:  rpt.Overall.TradeableText,
			DecisionAction: rpt.Overall.DecisionAction,
			DecisionText:   rpt.Overall.DecisionText,
			Grade:          rpt.Overall.Grade,
			Reason:         rpt.Overall.Reason,
			ReasonCode:     rpt.Overall.ReasonCode,
			Direction:      rpt.Overall.Direction,
			ExpectedSnapID: rpt.Overall.ExpectedSnapID,
		},
		Providers: provs,
		Report:    df.RenderGateText(rpt),
	}
}

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

func writeJSON(w http.ResponseWriter, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func normalizeBasePath(base string) string {
	if strings.TrimSpace(base) == "" {
		return "/decision-view"
	}
	out := base
	if !strings.HasPrefix(out, "/") {
		out = "/" + out
	}
	out = strings.TrimSuffix(out, "/")
	return out
}

func join(base, p string) string {
	if base == "" || base == "/" {
		return path.Clean(p)
	}
	return path.Clean(base + p)
}

func roundID(symbol string, snapshotID uint) string {
	if snapshotID == 0 {
		return fmt.Sprintf("%s-unknown", symbol)
	}
	return fmt.Sprintf("%s-%d", symbol, snapshotID)
}

type ConfigBundle struct {
	Symbol   config.SymbolConfig   `json:"symbol"`
	Strategy config.StrategyConfig `json:"strategy"`
}

type ConfigGraphResponse struct {
	Symbols     []ConfigSymbolGraph `json:"symbols"`
	NewsOverlay *NewsOverlayGraph   `json:"news_overlay,omitempty"`
}

type ConfigSymbolGraph struct {
	ID     string       `json:"id"`
	Label  string       `json:"label"`
	Config ConfigDetail `json:"config"`
}

type NewsOverlayGraph struct {
	UpdatedAt      string                 `json:"updated_at,omitempty"`
	Stale          bool                   `json:"stale"`
	StaleAfter     string                 `json:"stale_after,omitempty"`
	ItemsUsedCount int                    `json:"items_used_count"`
	LLMDecisionRaw string                 `json:"llm_decision_raw,omitempty"`
	Items          []NewsOverlayItemGraph `json:"items,omitempty"`
}

type NewsOverlayItemGraph struct {
	Window       string `json:"window"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	Domain       string `json:"domain,omitempty"`
	SeenAt       string `json:"seen_at,omitempty"`
	SignalSource string `json:"signal_source,omitempty"`
}

type ConfigDetail struct {
	Intervals    []string            `json:"intervals"`
	KlineLimit   int                 `json:"kline_limit"`
	AgentEnabled AgentEnabledSummary `json:"agent_enabled"`
	Gate         GateSummary         `json:"gate"`
	LLM          SymbolLLMSummary    `json:"llm"`
	Prompts      SymbolPromptSummary `json:"prompts"`
	Strategy     StrategySummary     `json:"strategy"`
	SystemHash   string              `json:"system_hash"`
	SymbolHash   string              `json:"symbol_hash"`
	StrategyHash string              `json:"strategy_hash"`
	Meta         map[string]any      `json:"meta,omitempty"`
}

type AgentEnabledSummary struct {
	Indicator bool `json:"indicator"`
	Structure bool `json:"structure"`
	Mechanics bool `json:"mechanics"`
}

type GateSummary struct{}

type SymbolLLMSummary struct {
	Agent    LLMModelSet `json:"agent"`
	Provider LLMModelSet `json:"provider"`
}

type LLMModelSet struct {
	Indicator string `json:"indicator"`
	Structure string `json:"structure"`
	Mechanics string `json:"mechanics"`
}

type SymbolPromptSummary struct {
	Agent              PromptSet `json:"agent"`
	Provider           PromptSet `json:"provider"`
	ProviderInPosition PromptSet `json:"provider_in_position"`
}

type PromptSet struct {
	Indicator string `json:"indicator"`
	Structure string `json:"structure"`
	Mechanics string `json:"mechanics"`
}

type StrategySummary struct {
	ID             string                `json:"id"`
	RuleChainPath  string                `json:"rule_chain"`
	RiskManagement RiskManagementSummary `json:"risk_management"`
}

type RiskManagementSummary struct {
	RiskPerTradePct float64            `json:"risk_per_trade_pct"`
	MaxLeverage     float64            `json:"max_leverage"`
	Grade1Factor    float64            `json:"grade_1_factor"`
	Grade2Factor    float64            `json:"grade_2_factor"`
	Grade3Factor    float64            `json:"grade_3_factor"`
	EntryOffsetATR  float64            `json:"entry_offset_atr"`
	BreakevenFeePct float64            `json:"breakeven_fee_pct"`
	InitialExit     InitialExitSummary `json:"initial_exit"`
	TightenATR      TightenATRSummary  `json:"tighten_atr"`
}

type InitialExitSummary struct {
	Policy            string         `json:"policy"`
	StructureInterval string         `json:"structure_interval"`
	Params            map[string]any `json:"params"`
}

type TightenATRSummary struct {
	StructureThreatened  float64 `json:"structure_threatened"`
	MinUpdateIntervalSec int64   `json:"min_update_interval_sec"`
}

func (s Server) buildConfigGraph() ConfigGraphResponse {
	resp := ConfigGraphResponse{
		Symbols:     []ConfigSymbolGraph{},
		NewsOverlay: s.buildNewsOverlayGraph(),
	}
	if len(s.SymbolConfigs) == 0 {
		return resp
	}
	defaults := config.DefaultPromptDefaults()
	for sym, bundle := range s.SymbolConfigs {
		enabled, err := config.ResolveAgentEnabled(bundle.Symbol.Agent)
		if err != nil {
			enabled = config.AgentEnabled{}
		}
		item := ConfigSymbolGraph{
			ID:    sym,
			Label: sym,
			Config: ConfigDetail{
				Intervals:  append([]string{}, bundle.Symbol.Intervals...),
				KlineLimit: bundle.Symbol.KlineLimit,
				AgentEnabled: AgentEnabledSummary{
					Indicator: enabled.Indicator,
					Structure: enabled.Structure,
					Mechanics: enabled.Mechanics,
				},
				Gate: GateSummary{},
				LLM: SymbolLLMSummary{
					Agent: LLMModelSet{
						Indicator: bundle.Symbol.LLM.Agent.Indicator.Model,
						Structure: bundle.Symbol.LLM.Agent.Structure.Model,
						Mechanics: bundle.Symbol.LLM.Agent.Mechanics.Model,
					},
					Provider: LLMModelSet{
						Indicator: bundle.Symbol.LLM.Provider.Indicator.Model,
						Structure: bundle.Symbol.LLM.Provider.Structure.Model,
						Mechanics: bundle.Symbol.LLM.Provider.Mechanics.Model,
					},
				},
				Prompts: SymbolPromptSummary{
					Agent: PromptSet{
						Indicator: defaults.AgentIndicator,
						Structure: defaults.AgentStructure,
						Mechanics: defaults.AgentMechanics,
					},
					Provider: PromptSet{
						Indicator: defaults.ProviderIndicator,
						Structure: defaults.ProviderStructure,
						Mechanics: defaults.ProviderMechanics,
					},
					ProviderInPosition: PromptSet{
						Indicator: defaults.ProviderInPositionIndicator,
						Structure: defaults.ProviderInPositionStructure,
						Mechanics: defaults.ProviderInPositionMechanics,
					},
				},
				Strategy: StrategySummary{
					ID:            bundle.Strategy.ID,
					RuleChainPath: bundle.Strategy.RuleChainPath,
					RiskManagement: RiskManagementSummary{
						RiskPerTradePct: bundle.Strategy.RiskManagement.RiskPerTradePct,
						MaxLeverage:     bundle.Strategy.RiskManagement.MaxLeverage,
						Grade1Factor:    bundle.Strategy.RiskManagement.Grade1Factor,
						Grade2Factor:    bundle.Strategy.RiskManagement.Grade2Factor,
						Grade3Factor:    bundle.Strategy.RiskManagement.Grade3Factor,
						EntryOffsetATR:  bundle.Strategy.RiskManagement.EntryOffsetATR,
						BreakevenFeePct: bundle.Strategy.RiskManagement.BreakevenFeePct,
						InitialExit: InitialExitSummary{
							Policy:            bundle.Strategy.RiskManagement.InitialExit.Policy,
							StructureInterval: bundle.Strategy.RiskManagement.InitialExit.StructureInterval,
							Params:            bundle.Strategy.RiskManagement.InitialExit.Params,
						},
						TightenATR: TightenATRSummary{
							StructureThreatened:  bundle.Strategy.RiskManagement.TightenATR.StructureThreatened,
							MinUpdateIntervalSec: bundle.Strategy.RiskManagement.TightenATR.MinUpdateIntervalSec,
						},
					},
				},
				SystemHash:   s.SystemConfig.Hash,
				SymbolHash:   bundle.Symbol.Hash,
				StrategyHash: bundle.Strategy.Hash,
			},
		}
		resp.Symbols = append(resp.Symbols, item)
	}
	sort.Slice(resp.Symbols, func(i, j int) bool { return resp.Symbols[i].ID < resp.Symbols[j].ID })
	return resp
}

func (s Server) buildNewsOverlayGraph() *NewsOverlayGraph {
	snapshot, ok := newsoverlay.GlobalStore().Load()
	if !ok {
		return nil
	}
	// 仅展示当前参与 News Overlay 决策（used）的消息，
	// 保持与“每条正在使用的消息一个圆圈”的可视化语义一致。
	items := snapshot.NewsItemsUsed
	sort.SliceStable(items, func(i, j int) bool {
		ti := items[i].SeenAt
		tj := items[j].SeenAt
		switch {
		case !ti.IsZero() && !tj.IsZero() && !ti.Equal(tj):
			return ti.After(tj)
		case !ti.IsZero() && tj.IsZero():
			return true
		case ti.IsZero() && !tj.IsZero():
			return false
		}
		return strings.ToLower(items[i].Title) < strings.ToLower(items[j].Title)
	})
	if len(items) > maxNewsNodes {
		items = items[:maxNewsNodes]
	}
	graphItems := make([]NewsOverlayItemGraph, 0, len(items))
	for _, item := range items {
		entry := NewsOverlayItemGraph{
			Window:       item.Window,
			Title:        item.Title,
			URL:          item.URL,
			Domain:       item.Domain,
			SignalSource: item.SignalSource,
		}
		if !item.SeenAt.IsZero() {
			entry.SeenAt = item.SeenAt.UTC().Format(time.RFC3339)
		}
		graphItems = append(graphItems, entry)
	}

	staleAfter := parseNewsOverlayStaleAfter(s.SystemConfig.NewsOverlay.SnapshotStaleAfter)
	updatedAt := ""
	if !snapshot.UpdatedAt.IsZero() {
		updatedAt = snapshot.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return &NewsOverlayGraph{
		UpdatedAt:      updatedAt,
		Stale:          snapshot.UpdatedAt.IsZero() || time.Since(snapshot.UpdatedAt) > staleAfter,
		StaleAfter:     staleAfter.String(),
		ItemsUsedCount: len(snapshot.NewsItemsUsed),
		LLMDecisionRaw: strings.TrimSpace(snapshot.LLMDecisionRaw),
		Items:          graphItems,
	}
}

func parseNewsOverlayStaleAfter(raw string) time.Duration {
	text := strings.TrimSpace(raw)
	if text == "" {
		return 4 * time.Hour
	}
	val, err := time.ParseDuration(text)
	if err != nil || val <= 0 {
		return 4 * time.Hour
	}
	return val
}
