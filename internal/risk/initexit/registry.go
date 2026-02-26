package initexit

import (
	"fmt"
	"strings"
	"sync"
)

var (
	registryMu sync.RWMutex
	registry   = map[string]Policy{}
)

func Register(policy Policy) {
	if policy == nil {
		panic("initexit: nil policy")
	}
	name := normalizePolicyName(policy.Name())
	if name == "" {
		panic("initexit: empty policy name")
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registry[name]; exists {
		panic("initexit: duplicate policy: " + name)
	}
	registry[name] = policy
}

func Get(name string) (Policy, bool) {
	key := normalizePolicyName(name)
	if key == "" {
		return nil, false
	}
	registryMu.RLock()
	defer registryMu.RUnlock()
	p, ok := registry[key]
	return p, ok
}

func MustGet(name string) (Policy, error) {
	p, ok := Get(name)
	if ok {
		return p, nil
	}
	return nil, fmt.Errorf("initial exit policy not found: %s", strings.TrimSpace(name))
}

func normalizePolicyName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
