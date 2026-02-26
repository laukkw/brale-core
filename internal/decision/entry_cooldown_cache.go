package decision

import (
	"strings"
	"sync"
)

type EntryCooldownCache struct {
	mu     sync.RWMutex
	rounds map[string]int
}

func NewEntryCooldownCache() *EntryCooldownCache {
	return &EntryCooldownCache{rounds: make(map[string]int)}
}

func (c *EntryCooldownCache) Arm(symbol string, rounds int) {
	if c == nil {
		return
	}
	key := cleanCooldownSymbol(symbol)
	if key == "" || rounds <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rounds[key] = rounds
}

func (c *EntryCooldownCache) Consume(symbol string) (remaining int, active bool) {
	if c == nil {
		return 0, false
	}
	key := cleanCooldownSymbol(symbol)
	if key == "" {
		return 0, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	left := c.rounds[key]
	if left <= 0 {
		delete(c.rounds, key)
		return 0, false
	}
	left--
	if left <= 0 {
		delete(c.rounds, key)
		return 0, true
	}
	c.rounds[key] = left
	return left, true
}

func (c *EntryCooldownCache) Get(symbol string) int {
	if c == nil {
		return 0
	}
	key := cleanCooldownSymbol(symbol)
	if key == "" {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.rounds[key]
}

func cleanCooldownSymbol(symbol string) string {
	return strings.ToUpper(strings.TrimSpace(symbol))
}
