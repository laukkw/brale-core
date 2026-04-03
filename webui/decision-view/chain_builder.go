package decisionview

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"brale-core/internal/decision/decisionfmt"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/store"

	"go.uber.org/zap"
)

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

type roundAccumulator struct {
	symbol string
	rounds map[uint]*roundBuf
}

type stageRefs struct {
	providers []string
	agents    []string
}

func buildChain(ctx context.Context, symbol string, events symbolEvents, openPos store.PositionRecord, hasOpenPos bool, limit int) SymbolChain {
	logger := logging.FromContext(ctx).Named("decision-view").With(zap.String("symbol", symbol))
	df := decisionfmt.New()
	acc := newRoundAccumulator(symbol)
	addProviderNodes(acc, df, logger, symbol, events.providers)
	addAgentNodes(acc, df, logger, symbol, events.agents)
	addGateNodes(acc, df, logger, symbol, events.gates)
	addLLMRiskTraceNodes(acc, logger, symbol, events.gates)
	roundList := acc.sorted(limit)
	return buildSymbolChainFromRounds(df, symbol, roundList)
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
	case strings.Contains(s, "llm_risk"):
		return 35
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
	kind := strings.ToLower(t)
	return strings.Contains(kind, "exec") || strings.Contains(kind, "llm_risk")
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

func roundHasGateNode(nodes []nodeWithOrder) bool {
	for _, n := range nodes {
		if isGateNode(n.Node.Type) {
			return true
		}
	}
	return false
}

func hasNodeStage(nodes []nodeWithOrder, stage string) bool {
	want := strings.ToLower(strings.TrimSpace(stage))
	if want == "" {
		return false
	}
	for _, n := range nodes {
		if strings.ToLower(strings.TrimSpace(n.Node.Stage)) == want {
			return true
		}
	}
	return false
}

func roundID(symbol string, snapshotID uint) string {
	if snapshotID == 0 {
		return fmt.Sprintf("%s-unknown", symbol)
	}
	return fmt.Sprintf("%s-%d", symbol, snapshotID)
}
