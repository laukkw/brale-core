// Package promptreg loads prompt templates from the database prompt_registry,
// falling back to the compiled-in defaults from internal/config/prompts.go.
package promptreg

import (
	"context"
	"fmt"
	"sync"

	"brale-core/internal/store"

	"go.uber.org/zap"
)

// Loader resolves prompt text for a given (role, stage) tuple.
// It queries the DB first, falls back to compiled defaults.
type Loader struct {
	store    store.PromptRegistryStore
	defaults map[string]string // key = "role/stage" e.g. "agent/indicator"
	mu       sync.RWMutex
	cache    map[string]cachedPrompt
	logger   *zap.Logger
}

type cachedPrompt struct {
	text    string
	version string
}

// NewLoader creates a prompt loader backed by the given store.
// If store is nil, only hardcoded defaults are used.
func NewLoader(s store.PromptRegistryStore, defaults map[string]string, logger *zap.Logger) *Loader {
	return &Loader{
		store:    s,
		defaults: defaults,
		cache:    make(map[string]cachedPrompt),
		logger:   logger,
	}
}

// Resolve returns the prompt text and version for a (role, stage) pair.
// If a DB-stored active prompt exists, it takes priority over defaults.
func (l *Loader) Resolve(ctx context.Context, role, stage, locale string) (text string, version string, err error) {
	key := role + "/" + stage

	// Check cache first
	l.mu.RLock()
	if cached, ok := l.cache[key+"@"+locale]; ok {
		l.mu.RUnlock()
		return cached.text, cached.version, nil
	}
	l.mu.RUnlock()

	// Try DB
	if l.store != nil {
		entry, found, dbErr := l.store.FindActivePrompt(ctx, role, stage, locale)
		if dbErr != nil {
			if l.logger != nil {
				l.logger.Warn("prompt registry query failed, using default",
					zap.String("role", role), zap.String("stage", stage), zap.String("locale", locale), zap.Error(dbErr))
			}
		} else if found {
			l.mu.Lock()
			l.cache[key+"@"+locale] = cachedPrompt{text: entry.SystemPrompt, version: entry.Version}
			l.mu.Unlock()
			return entry.SystemPrompt, entry.Version, nil
		}
	}

	// Fall back to compiled default
	def, ok := l.defaults[key]
	if !ok {
		return "", "", fmt.Errorf("no prompt for %s", key)
	}

	l.mu.Lock()
	l.cache[key+"@"+locale] = cachedPrompt{text: def, version: "builtin"}
	l.mu.Unlock()
	return def, "builtin", nil
}

// InvalidateCache clears the cached prompts, forcing re-fetch from DB.
func (l *Loader) InvalidateCache() {
	l.mu.Lock()
	l.cache = make(map[string]cachedPrompt)
	l.mu.Unlock()
}
