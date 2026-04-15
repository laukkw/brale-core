package promptreg

import (
	"context"
	"testing"
)

func TestLoaderFallsBackToDefaults(t *testing.T) {
	defaults := map[string]string{
		"agent/indicator": "default indicator prompt",
	}
	loader := NewLoader(nil, defaults, nil)

	text, version, err := loader.Resolve(context.Background(), "agent", "indicator")
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
	_, _, _ = loader.Resolve(context.Background(), "agent", "structure")

	// Second call should come from cache
	text, version, err := loader.Resolve(context.Background(), "agent", "structure")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "structure prompt" || version != "builtin" {
		t.Fatalf("cache miss: %s / %s", text, version)
	}
}

func TestLoaderMissingPromptReturnsError(t *testing.T) {
	loader := NewLoader(nil, map[string]string{}, nil)
	_, _, err := loader.Resolve(context.Background(), "agent", "missing")
	if err == nil {
		t.Fatal("expected error for missing prompt")
	}
}

func TestLoaderInvalidateCache(t *testing.T) {
	defaults := map[string]string{
		"agent/indicator": "prompt v1",
	}
	loader := NewLoader(nil, defaults, nil)

	_, _, _ = loader.Resolve(context.Background(), "agent", "indicator")
	loader.InvalidateCache()

	// Should re-resolve from defaults
	text, _, err := loader.Resolve(context.Background(), "agent", "indicator")
	if err != nil {
		t.Fatalf("unexpected error after cache invalidation: %v", err)
	}
	if text != "prompt v1" {
		t.Fatalf("expected prompt v1, got: %s", text)
	}
}
