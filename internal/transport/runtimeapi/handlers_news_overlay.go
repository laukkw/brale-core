package runtimeapi

import (
	"net/http"
	"strings"
	"time"

	"brale-core/internal/decision/newsoverlay"
)

func (s *Server) handleNewsOverlayLatest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !ensureMethod(ctx, w, r, http.MethodGet) {
		return
	}
	snapshot, ok := newsoverlay.GlobalStore().Load()
	if !ok {
		writeJSON(w, NewsOverlayLatestResponse{
			Status:    "ok",
			Summary:   "暂无舆情信息",
			RequestID: requestIDFromContext(ctx),
		})
		return
	}
	staleAfter := s.NewsOverlayStaleAfter
	if staleAfter <= 0 {
		staleAfter = 4 * time.Hour
	}
	stale := snapshot.UpdatedAt.IsZero() || time.Since(snapshot.UpdatedAt) > staleAfter
	llmDecisionRaw := strings.TrimSpace(snapshot.LLMDecisionRaw)
	if llmDecisionRaw == "" {
		llmDecisionRaw = "{}"
	}
	summary := "已返回最新舆情信息"
	if stale {
		summary = "舆情信息已过期"
	}
	updatedAt := ""
	if !snapshot.UpdatedAt.IsZero() {
		updatedAt = snapshot.UpdatedAt.UTC().Format(time.RFC3339)
	}
	writeJSON(w, NewsOverlayLatestResponse{
		Status:         "ok",
		UpdatedAt:      updatedAt,
		LLMDecisionRaw: llmDecisionRaw,
		Stale:          stale,
		StaleAfter:     staleAfter.String(),
		Summary:        summary,
		RequestID:      requestIDFromContext(ctx),
	})
}
