package newsoverlay

import (
	"sync"
	"time"
)

type EvidenceItem struct {
	Window string `json:"window"`
	Title  string `json:"title"`
}

type OverlayResult struct {
	UpdatedAt            time.Time                     `json:"updated_at"`
	EntryMultiplierLong  float64                       `json:"entry_multiplier_long"`
	EntryMultiplierShort float64                       `json:"entry_multiplier_short"`
	TightenScoreBySide   map[string]map[string]float64 `json:"tighten_score_by_side"`
	ItemsCount           map[string]int                `json:"items_count"`
	Evidence             []EvidenceItem                `json:"evidence,omitempty"`
}

type NewsItemRaw struct {
	Window       string    `json:"window"`
	Title        string    `json:"title"`
	URL          string    `json:"url"`
	Domain       string    `json:"domain,omitempty"`
	SeenAt       time.Time `json:"seen_at,omitempty"`
	SignalSource string    `json:"signal_source,omitempty"`
}

type Snapshot struct {
	UpdatedAt      time.Time     `json:"updated_at"`
	Overlay        OverlayResult `json:"overlay"`
	NewsItemsRaw   []NewsItemRaw `json:"news_items_raw,omitempty"`
	NewsItemsUsed  []NewsItemRaw `json:"news_items_used,omitempty"`
	LLMDecisionRaw string        `json:"llm_decision_raw"`
}

type Store struct {
	mu       sync.RWMutex
	snapshot Snapshot
	ok       bool
}

func NewStore() *Store {
	return &Store{}
}

func (s *Store) Save(snapshot Snapshot) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot = cloneSnapshot(snapshot)
	s.ok = true
}

func (s *Store) Load() (Snapshot, bool) {
	if s == nil {
		return Snapshot{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.ok {
		return Snapshot{}, false
	}
	return cloneSnapshot(s.snapshot), true
}

func (s *Store) Clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot = Snapshot{}
	s.ok = false
}

func cloneSnapshot(in Snapshot) Snapshot {
	out := in
	out.Overlay = cloneOverlayResult(in.Overlay)
	if len(in.NewsItemsRaw) > 0 {
		out.NewsItemsRaw = cloneNewsItems(in.NewsItemsRaw)
	}
	if len(in.NewsItemsUsed) > 0 {
		out.NewsItemsUsed = cloneNewsItems(in.NewsItemsUsed)
	}
	return out
}

func cloneNewsItems(in []NewsItemRaw) []NewsItemRaw {
	return append([]NewsItemRaw(nil), in...)
}

func cloneOverlayResult(in OverlayResult) OverlayResult {
	out := in
	if len(in.TightenScoreBySide) > 0 {
		out.TightenScoreBySide = make(map[string]map[string]float64, len(in.TightenScoreBySide))
		for side, windowMap := range in.TightenScoreBySide {
			if len(windowMap) == 0 {
				out.TightenScoreBySide[side] = map[string]float64{}
				continue
			}
			copyWindowMap := make(map[string]float64, len(windowMap))
			for window, score := range windowMap {
				copyWindowMap[window] = score
			}
			out.TightenScoreBySide[side] = copyWindowMap
		}
	}
	if len(in.ItemsCount) > 0 {
		out.ItemsCount = make(map[string]int, len(in.ItemsCount))
		for window, count := range in.ItemsCount {
			out.ItemsCount[window] = count
		}
	}
	if len(in.Evidence) > 0 {
		out.Evidence = append([]EvidenceItem(nil), in.Evidence...)
	}
	return out
}

var globalStore = NewStore()

func GlobalStore() *Store {
	return globalStore
}
