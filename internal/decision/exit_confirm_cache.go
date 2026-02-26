package decision

import (
	"fmt"
	"strings"
	"sync"

	"brale-core/internal/decision/fsm"
)

type ExitConfirmCache struct {
	mu     sync.Mutex
	counts map[string]int
}

func NewExitConfirmCache() *ExitConfirmCache {
	return &ExitConfirmCache{counts: make(map[string]int)}
}

func (c *ExitConfirmCache) Get(symbol string, state fsm.PositionState, positionID string) int {
	if c == nil {
		return 0
	}
	key := exitConfirmCacheKey(symbol, state, positionID)
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counts[key]
}

func (c *ExitConfirmCache) Set(symbol string, state fsm.PositionState, positionID string, count int) {
	if c == nil {
		return
	}
	key := exitConfirmCacheKey(symbol, state, positionID)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts[key] = count
}

func exitConfirmCacheKey(symbol string, state fsm.PositionState, positionID string) string {
	cleanSymbol := strings.TrimSpace(symbol)
	cleanPositionID := strings.TrimSpace(positionID)
	return fmt.Sprintf("%s|%s|%s", cleanSymbol, state, cleanPositionID)
}
