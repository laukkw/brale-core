package onboarding

import (
	"os"
	"path/filepath"
)

// writeAtomic writes content to a file atomically by first writing to a
// temporary file and then renaming. It also applies host ownership when
// running as root inside a container.
func writeAtomic(repoRoot string, path string, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return applyHostOwnership(repoRoot, path)
}

// ensureMap retrieves or creates a nested map[string]any from root at the
// given key. Used by freqtrade config mutation in stack_prepare.go.
func ensureMap(root map[string]any, key string) map[string]any {
	v, ok := root[key]
	if !ok {
		m := map[string]any{}
		root[key] = m
		return m
	}
	m, ok := v.(map[string]any)
	if ok {
		return m
	}
	m = map[string]any{}
	root[key] = m
	return m
}
