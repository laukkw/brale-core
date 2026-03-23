package store

import (
	"context"
	"sort"
	"strings"
)

func (s *GormStore) ListSymbols(ctx context.Context) ([]string, error) {
	merge := func(dst map[string]struct{}, src []string) {
		for _, item := range src {
			trim := strings.TrimSpace(item)
			if trim == "" {
				continue
			}
			dst[trim] = struct{}{}
		}
	}
	set := make(map[string]struct{})
	var tmp []string
	if err := s.db.WithContext(ctx).Model(&PositionRecord{}).Distinct().Pluck("symbol", &tmp).Error; err != nil {
		return nil, err
	}
	merge(set, tmp)
	tmp = tmp[:0]
	if err := s.db.WithContext(ctx).Model(&AgentEventRecord{}).Distinct().Pluck("symbol", &tmp).Error; err != nil {
		return nil, err
	}
	merge(set, tmp)
	tmp = tmp[:0]
	if err := s.db.WithContext(ctx).Model(&ProviderEventRecord{}).Distinct().Pluck("symbol", &tmp).Error; err != nil {
		return nil, err
	}
	merge(set, tmp)
	tmp = tmp[:0]
	if err := s.db.WithContext(ctx).Model(&GateEventRecord{}).Distinct().Pluck("symbol", &tmp).Error; err != nil {
		return nil, err
	}
	merge(set, tmp)
	out := make([]string, 0, len(set))
	for sym := range set {
		out = append(out, sym)
	}
	sort.Strings(out)
	return out, nil
}
