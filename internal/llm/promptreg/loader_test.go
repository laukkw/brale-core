package promptreg

import (
	"context"
	"errors"
	"testing"

	"brale-core/internal/store"
)

type errPromptStore struct{}

func (errPromptStore) SavePromptEntry(context.Context, *store.PromptRegistryEntry) error {
	return nil
}

func (errPromptStore) FindActivePrompt(context.Context, string, string, string) (store.PromptRegistryEntry, bool, error) {
	return store.PromptRegistryEntry{}, false, errors.New("boom")
}

func (errPromptStore) ListPromptEntries(context.Context, string, bool) ([]store.PromptRegistryEntry, error) {
	return nil, nil
}

func TestLoaderFallsBackToDefaults(t *testing.T) {
	defaults := map[string]string{
		"agent/indicator": "default indicator prompt",
	}
	loader := NewLoader(nil, defaults, nil)

	text, version, err := loader.Resolve(context.Background(), "agent", "indicator", "zh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "default indicator prompt" {
		t.Fatalf("expected default prompt, got: %s", text)
	}
	if version != "builtin" {
		t.Fatalf("expected builtin version, got: %s", version)
	}
}

func TestLoaderCachesResult(t *testing.T) {
	defaults := map[string]string{
		"agent/structure": "structure prompt",
	}
	loader := NewLoader(nil, defaults, nil)

	// First call
	_, _, _ = loader.Resolve(context.Background(), "agent", "structure", "zh")

	// Second call should come from cache
	text, version, err := loader.Resolve(context.Background(), "agent", "structure", "zh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "structure prompt" || version != "builtin" {
		t.Fatalf("cache miss: %s / %s", text, version)
	}
}

func TestLoaderMissingPromptReturnsError(t *testing.T) {
	loader := NewLoader(nil, map[string]string{}, nil)
	_, _, err := loader.Resolve(context.Background(), "agent", "missing", "zh")
	if err == nil {
		t.Fatal("expected error for missing prompt")
	}
}

func TestLoaderInvalidateCache(t *testing.T) {
	defaults := map[string]string{
		"agent/indicator": "prompt v1",
	}
	loader := NewLoader(nil, defaults, nil)

	_, _, _ = loader.Resolve(context.Background(), "agent", "indicator", "zh")
	loader.InvalidateCache()

	// Should re-resolve from defaults
	text, _, err := loader.Resolve(context.Background(), "agent", "indicator", "zh")
	if err != nil {
		t.Fatalf("unexpected error after cache invalidation: %v", err)
	}
	if text != "prompt v1" {
		t.Fatalf("expected prompt v1, got: %s", text)
	}
}

func TestLoaderResolveWithNilLoggerAndStoreErrorFallsBack(t *testing.T) {
	loader := NewLoader(errPromptStore{}, map[string]string{
		"agent/indicator": "fallback prompt",
	}, nil)

	text, version, err := loader.Resolve(context.Background(), "agent", "indicator", "zh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "fallback prompt" {
		t.Fatalf("text=%q want fallback", text)
	}
	if version != "builtin" {
		t.Fatalf("version=%q want builtin", version)
	}
}
