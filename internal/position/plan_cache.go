package position

import (
	"strings"
	"sync"
	"time"

	"brale-core/internal/execution"
)

type PlanCache struct {
	mu       sync.Mutex
	bySymbol map[string]*PlanEntry
}

type PlanEntry struct {
	Plan          execution.ExecutionPlan
	ExternalID    string
	ClientOrderID string
	SubmittedAt   int64
}

type PlanUpsertResult struct {
	Replaced      bool
	Reason        string
	Previous      *execution.ExecutionPlan
	PreviousEntry *PlanEntry
}

const (
	PlanReplaceAllow = "ALLOW_REPLACE"
	PlanReplaceWait  = "NO_REPLACE_WAIT"
	PlanReplaceVeto  = "NO_REPLACE_VETO"
)

func NewPlanCache() *PlanCache {
	return &PlanCache{bySymbol: make(map[string]*PlanEntry)}
}

func (c *PlanCache) Get(symbol string) (*execution.ExecutionPlan, bool) {
	entry, ok := c.GetEntry(symbol)
	if !ok || entry == nil {
		return nil, false
	}
	copy := entry.Plan
	return &copy, true
}

func (c *PlanCache) GetEntry(symbol string) (*PlanEntry, bool) {
	if c == nil {
		return nil, false
	}
	symbol = normalizePlanSymbol(symbol)
	if symbol == "" {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.bySymbol[symbol]
	if !ok || entry == nil {
		return nil, false
	}
	copy := *entry
	return &copy, true
}

func normalizePlanSymbol(symbol string) string {
	return strings.ToUpper(strings.TrimSpace(symbol))
}

func (c *PlanCache) updateOrderUnsafe(symbol string, externalID string, clientOrderID string, submittedAt int64) {
	entry, ok := c.bySymbol[symbol]
	if !ok || entry == nil {
		return
	}
	if strings.TrimSpace(externalID) != "" {
		entry.ExternalID = strings.TrimSpace(externalID)
	}
	if strings.TrimSpace(clientOrderID) != "" {
		entry.ClientOrderID = strings.TrimSpace(clientOrderID)
	}
	if submittedAt > 0 {
		entry.SubmittedAt = submittedAt
	}
}

func (c *PlanCache) Remove(symbol string) {
	if c == nil {
		return
	}
	symbol = normalizePlanSymbol(symbol)
	if symbol == "" {
		return
	}
	c.mu.Lock()
	delete(c.bySymbol, symbol)
	c.mu.Unlock()
}

func (c *PlanCache) ListEntries() []PlanEntry {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]PlanEntry, 0, len(c.bySymbol))
	for _, entry := range c.bySymbol {
		if entry == nil {
			continue
		}
		out = append(out, *entry)
	}
	return out
}

func (c *PlanCache) UpdateOrder(symbol string, externalID string, clientOrderID string, submittedAt int64) (*PlanEntry, bool) {
	if c == nil {
		return nil, false
	}
	symbol = normalizePlanSymbol(symbol)
	if symbol == "" {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.bySymbol[symbol]
	if !ok || entry == nil {
		return nil, false
	}
	c.updateOrderUnsafe(symbol, externalID, clientOrderID, submittedAt)
	copy := *entry
	return &copy, true
}

func (c *PlanCache) ResetOrder(symbol string) (*PlanEntry, bool) {
	if c == nil {
		return nil, false
	}
	symbol = normalizePlanSymbol(symbol)
	if symbol == "" {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.bySymbol[symbol]
	if !ok || entry == nil {
		return nil, false
	}
	entry.ExternalID = ""
	entry.ClientOrderID = ""
	entry.SubmittedAt = 0
	copy := *entry
	return &copy, true
}

func (c *PlanCache) UpdatePlan(symbol string, plan execution.ExecutionPlan) (*PlanEntry, bool) {
	if c == nil {
		return nil, false
	}
	symbol = normalizePlanSymbol(symbol)
	if symbol == "" {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.bySymbol[symbol]
	if !ok || entry == nil {
		return nil, false
	}
	entry.Plan = plan
	copy := *entry
	return &copy, true
}

func (c *PlanCache) ForceUpsert(symbol string, plan execution.ExecutionPlan) (*PlanEntry, *PlanEntry) {
	if c == nil {
		return nil, nil
	}
	symbol = normalizePlanSymbol(symbol)
	if symbol == "" {
		return nil, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var prevCopy *PlanEntry
	if prev, ok := c.bySymbol[symbol]; ok && prev != nil {
		copy := *prev
		prevCopy = &copy
	}
	entry := &PlanEntry{Plan: plan}
	c.bySymbol[symbol] = entry
	copy := *entry
	return &copy, prevCopy
}

func (c *PlanCache) ClearIfMatch(symbol string, externalID string, clientOrderID string) bool {
	if c == nil {
		return false
	}
	symbol = normalizePlanSymbol(symbol)
	if symbol == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.bySymbol[symbol]
	if !ok || entry == nil {
		return false
	}
	if externalID != "" && !strings.EqualFold(strings.TrimSpace(externalID), strings.TrimSpace(entry.ExternalID)) {
		return false
	}
	if clientOrderID != "" && !strings.EqualFold(strings.TrimSpace(clientOrderID), strings.TrimSpace(entry.ClientOrderID)) {
		return false
	}
	delete(c.bySymbol, symbol)
	return true
}

func (c *PlanCache) UpsertIfAllow(symbol string, plan execution.ExecutionPlan, gateAction string, valid bool) PlanUpsertResult {
	if c == nil {
		return PlanUpsertResult{Replaced: false, Reason: PlanReplaceWait}
	}
	symbol = normalizePlanSymbol(symbol)
	if symbol == "" {
		return PlanUpsertResult{Replaced: false, Reason: PlanReplaceWait}
	}
	if !valid {
		return PlanUpsertResult{Replaced: false, Reason: PlanReplaceWait}
	}
	action := strings.ToUpper(strings.TrimSpace(gateAction))
	if action != "ALLOW" {
		reason := PlanReplaceWait
		if action == "VETO" {
			reason = PlanReplaceVeto
		}
		return PlanUpsertResult{Replaced: false, Reason: reason}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var prevCopy *execution.ExecutionPlan
	var prevEntryCopy *PlanEntry
	if prev, ok := c.bySymbol[symbol]; ok && prev != nil {
		copy := prev.Plan
		prevCopy = &copy
		entryCopy := *prev
		prevEntryCopy = &entryCopy
	}
	entry := &PlanEntry{Plan: plan}
	c.bySymbol[symbol] = entry
	return PlanUpsertResult{Replaced: true, Reason: PlanReplaceAllow, Previous: prevCopy, PreviousEntry: prevEntryCopy}
}

func (c *PlanCache) ExpireIfNeeded(symbol string, now time.Time) (bool, *PlanEntry) {
	if c == nil {
		return false, nil
	}
	symbol = normalizePlanSymbol(symbol)
	if symbol == "" {
		return false, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.bySymbol[symbol]
	if !ok || entry == nil {
		return false, nil
	}
	plan := entry.Plan
	if plan.ExpiresAt.IsZero() || now.Before(plan.ExpiresAt) {
		entryCopy := *entry
		return false, &entryCopy
	}
	entryCopy := *entry
	delete(c.bySymbol, symbol)
	return true, &entryCopy
}
