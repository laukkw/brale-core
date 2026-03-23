package runtime

import (
	"sort"
	"time"
)

type ScheduleStatus struct {
	IsScheduled bool            `json:"is_scheduled"`
	NextRuns    []SymbolNextRun `json:"next_runs"`
	Details     string          `json:"details"`
}

type SymbolNextRun struct {
	Symbol        string `json:"symbol"`
	NextExecution string `json:"next_execution"`
	Waiting       string `json:"waiting"`
	BarInterval   string `json:"bar_interval"`
	LastBarTime   int64  `json:"last_bar_time"`
	Details       string `json:"details"`
	Mode          string `json:"mode"`
}

func (s *RuntimeScheduler) GetScheduleStatus() ScheduleStatus {
	s.mu.RLock()
	scheduled := s.EnableScheduledDecision
	s.mu.RUnlock()
	policy := s.policy()
	now := time.Now()
	keys := make([]string, 0, len(s.Symbols))
	for sym := range s.Symbols {
		keys = append(keys, sym)
	}
	sort.Strings(keys)
	results := make([]SymbolNextRun, 0, len(keys))
	for _, sym := range keys {
		rt := s.Symbols[sym]
		mode := s.getSymbolMode(sym)
		interval := rt.BarInterval
		if interval <= 0 {
			continue
		}
		monitored := s.isSymbolMonitored(sym)
		nextExecution, waiting, details := policy.DescribeSymbolStatus(scheduled, mode, monitored, now, interval)
		results = append(results, SymbolNextRun{
			Symbol:        sym,
			NextExecution: nextExecution,
			Waiting:       waiting,
			BarInterval:   interval.String(),
			LastBarTime:   0,
			Details:       details,
			Mode:          string(mode),
		})
	}
	monitoredCount := 0
	for _, sym := range keys {
		if s.isSymbolMonitored(sym) {
			monitoredCount++
		}
	}
	summary := policy.Summary(scheduled, len(results), monitoredCount)
	return ScheduleStatus{IsScheduled: scheduled, NextRuns: results, Details: summary}
}
