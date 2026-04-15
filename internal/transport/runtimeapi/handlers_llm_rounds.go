package runtimeapi

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) handleLLMRounds(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !ensureMethod(ctx, w, r, http.MethodGet) {
		return
	}

	symbol := strings.TrimSpace(r.URL.Query().Get("symbol"))
	limitStr := strings.TrimSpace(r.URL.Query().Get("limit"))
	limit := 50
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	if s.Store == nil {
		writeError(ctx, w, http.StatusInternalServerError, "store_unavailable", "store not configured", nil)
		return
	}

	rounds, err := s.Store.ListLLMRounds(ctx, symbol, limit)
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, "query_error", err.Error(), nil)
		return
	}

	type roundItem struct {
		ID             string `json:"id"`
		SnapshotID     uint   `json:"snapshot_id,omitempty"`
		Symbol         string `json:"symbol"`
		RoundType      string `json:"round_type"`
		StartedAt      string `json:"started_at"`
		FinishedAt     string `json:"finished_at,omitempty"`
		TotalLatencyMS int    `json:"total_latency_ms"`
		TotalTokenIn   int    `json:"total_token_in"`
		TotalTokenOut  int    `json:"total_token_out"`
		CallCount      int    `json:"call_count"`
		Outcome        string `json:"outcome,omitempty"`
		PromptVersion  string `json:"prompt_version,omitempty"`
		Error          string `json:"error,omitempty"`
		AgentCount     int    `json:"agent_count"`
		ProviderCount  int    `json:"provider_count"`
		GateAction     string `json:"gate_action,omitempty"`
	}

	items := make([]roundItem, 0, len(rounds))
	for _, r := range rounds {
		item := roundItem{
			ID:             r.ID,
			SnapshotID:     r.SnapshotID,
			Symbol:         r.Symbol,
			RoundType:      r.RoundType,
			StartedAt:      r.StartedAt.Format("2006-01-02T15:04:05Z"),
			TotalLatencyMS: r.TotalLatencyMS,
			TotalTokenIn:   r.TotalTokenIn,
			TotalTokenOut:  r.TotalTokenOut,
			CallCount:      r.CallCount,
			Outcome:        r.Outcome,
			PromptVersion:  r.PromptVersion,
			Error:          r.Error,
			AgentCount:     r.AgentCount,
			ProviderCount:  r.ProviderCount,
			GateAction:     r.GateAction,
		}
		if !r.FinishedAt.IsZero() {
			item.FinishedAt = r.FinishedAt.Format("2006-01-02T15:04:05Z")
		}
		items = append(items, item)
	}

	writeJSON(w, map[string]any{
		"status":     "ok",
		"rounds":     items,
		"count":      len(items),
		"request_id": requestIDFromContext(ctx),
	})
}
