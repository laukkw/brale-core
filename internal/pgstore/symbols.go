package pgstore

import (
	"context"
	"sort"
	"strings"
)

// ─── SymbolCatalogQueryStore ─────────────────────────────────────

func (s *PGStore) ListSymbols(ctx context.Context) ([]string, error) {
	set := make(map[string]struct{})
	tables := []string{"positions", "agent_events", "provider_events", "gate_events"}
	for _, tbl := range tables {
		rows, err := s.query(ctx, "SELECT DISTINCT symbol FROM "+tbl)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var sym string
			if err := rows.Scan(&sym); err != nil {
				rows.Close()
				return nil, err
			}
			sym = strings.TrimSpace(sym)
			if sym != "" {
				set[sym] = struct{}{}
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	out := make([]string, 0, len(set))
	for sym := range set {
		out = append(out, sym)
	}
	sort.Strings(out)
	return out, nil
}
