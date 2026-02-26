package llmapp

import (
	"sync"
	"time"
)

type LLMRunTracker struct {
	mu           sync.RWMutex
	lastAgent    time.Time
	lastProvider time.Time
}

func NewLLMRunTracker() *LLMRunTracker {
	return &LLMRunTracker{}
}

func (t *LLMRunTracker) MarkAgent() {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.lastAgent = time.Now().UTC()
	t.mu.Unlock()
}

func (t *LLMRunTracker) MarkProvider() {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.lastProvider = time.Now().UTC()
	t.mu.Unlock()
}

func (t *LLMRunTracker) LastAgent() time.Time {
	if t == nil {
		return time.Time{}
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lastAgent
}

func (t *LLMRunTracker) LastProvider() time.Time {
	if t == nil {
		return time.Time{}
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lastProvider
}
