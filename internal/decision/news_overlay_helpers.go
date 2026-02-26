package decision

import (
	"strings"
	"time"

	"brale-core/internal/decision/newsoverlay"
)

func (r *Runner) currentNewsOverlayPayload() map[string]any {
	if r == nil || !r.NewsOverlayEnabled {
		return nil
	}
	snapshot, ok := newsoverlay.GlobalStore().Load()
	if !ok {
		return nil
	}
	if isSnapshotStale(snapshot.UpdatedAt, r.NewsOverlayStaleAfter) {
		return nil
	}
	return map[string]any{
		"updated_at":            snapshot.Overlay.UpdatedAt,
		"entry_multiplier_long": snapshot.Overlay.EntryMultiplierLong,
		"entry_multiplier_short": snapshot.Overlay.EntryMultiplierShort,
		"tighten_score_by_side": snapshot.Overlay.TightenScoreBySide,
		"items_count":           snapshot.Overlay.ItemsCount,
		"evidence":              snapshot.Overlay.Evidence,
	}
}

func (p *Pipeline) currentNewsOverlayPayload() map[string]any {
	if p == nil {
		return nil
	}
	if p.Runner != nil {
		return p.Runner.currentNewsOverlayPayload()
	}
	if !p.NewsOverlayEnabled {
		return nil
	}
	snapshot, ok := newsoverlay.GlobalStore().Load()
	if !ok {
		return nil
	}
	if isSnapshotStale(snapshot.UpdatedAt, p.NewsOverlayStaleAfter) {
		return nil
	}
	return map[string]any{
		"updated_at":            snapshot.Overlay.UpdatedAt,
		"entry_multiplier_long": snapshot.Overlay.EntryMultiplierLong,
		"entry_multiplier_short": snapshot.Overlay.EntryMultiplierShort,
		"tighten_score_by_side": snapshot.Overlay.TightenScoreBySide,
		"items_count":           snapshot.Overlay.ItemsCount,
		"evidence":              snapshot.Overlay.Evidence,
	}
}

func normalizeSide(raw string) string {
	side := strings.ToLower(strings.TrimSpace(raw))
	switch side {
	case "long", "short":
		return side
	default:
		return ""
	}
}

func resolveNewsWindow(barInterval time.Duration) string {
	if barInterval <= time.Hour {
		return "1h"
	}
	return "4h"
}

func isSnapshotStale(updatedAt time.Time, staleAfter time.Duration) bool {
	if updatedAt.IsZero() {
		return true
	}
	if staleAfter <= 0 {
		staleAfter = 4 * time.Hour
	}
	return time.Since(updatedAt) > staleAfter
}

