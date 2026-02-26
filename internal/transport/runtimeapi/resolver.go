package runtimeapi

import (
	"fmt"
	"sync"

	"brale-core/internal/runtime"

	"golang.org/x/sync/singleflight"
)

type RuntimeSymbolResolver struct {
	mu              sync.RWMutex
	runtimes        map[string]runtime.SymbolRuntime
	baseCount       int
	maxDynamic      int
	builder         func(symbol string) (runtime.SymbolRuntime, error)
	fallbackBuilder func(symbol string) (runtime.SymbolRuntime, error)
	buildGroup      singleflight.Group
}

func NewRuntimeSymbolResolver(
	runtimes map[string]runtime.SymbolRuntime,
	builder func(symbol string) (runtime.SymbolRuntime, error),
	fallbackBuilder func(symbol string) (runtime.SymbolRuntime, error),
	maxDynamicRuntimes int,
) *RuntimeSymbolResolver {
	cloned := make(map[string]runtime.SymbolRuntime, len(runtimes))
	for key, value := range runtimes {
		cloned[key] = value
	}
	return &RuntimeSymbolResolver{
		runtimes:        cloned,
		baseCount:       len(cloned),
		maxDynamic:      maxDynamicRuntimes,
		builder:         builder,
		fallbackBuilder: fallbackBuilder,
	}
}

func (r *RuntimeSymbolResolver) Resolve(symbol string) (ResolvedSymbol, error) {
	normalized := runtime.NormalizeSymbol(symbol)
	if normalized == "" {
		return ResolvedSymbol{}, fmt.Errorf("symbol is required")
	}
	rt, ok := r.getRuntime(normalized)
	if !ok {
		built, err, _ := r.buildGroup.Do(normalized, func() (any, error) {
			if existing, ok := r.getRuntime(normalized); ok {
				return existing, nil
			}
			created, createErr := r.buildRuntime(normalized)
			if createErr != nil {
				return runtime.SymbolRuntime{}, createErr
			}
			if setErr := r.setRuntime(normalized, created); setErr != nil {
				if existing, ok := r.getRuntime(normalized); ok {
					return existing, nil
				}
				return runtime.SymbolRuntime{}, setErr
			}
			return created, nil
		})
		if err != nil {
			return ResolvedSymbol{}, err
		}
		resolved, typeOK := built.(runtime.SymbolRuntime)
		if !typeOK {
			return ResolvedSymbol{}, fmt.Errorf("runtime resolve type mismatch for symbol %s", normalized)
		}
		rt = resolved
	}
	if rt.Pipeline == nil {
		return ResolvedSymbol{}, fmt.Errorf("pipeline missing for symbol %s", normalized)
	}
	return ResolvedSymbol{
		Symbol:      rt.Symbol,
		Intervals:   rt.Intervals,
		KlineLimit:  rt.KlineLimit,
		Pipeline:    rt.Pipeline,
		RiskPercent: rt.RiskPerTradePct,
	}, nil
}

func (r *RuntimeSymbolResolver) buildRuntime(symbol string) (runtime.SymbolRuntime, error) {
	var builderErr error
	if r.builder != nil {
		built, err := r.builder(symbol)
		if err == nil {
			return built, nil
		}
		builderErr = err
	}
	if r.fallbackBuilder != nil {
		fallbackBuilt, fallbackErr := r.fallbackBuilder(symbol)
		if fallbackErr == nil {
			return fallbackBuilt, nil
		}
		if builderErr != nil {
			return runtime.SymbolRuntime{}, fmt.Errorf("build runtime %s failed: builder=%v fallback=%w", symbol, builderErr, fallbackErr)
		}
		return runtime.SymbolRuntime{}, fmt.Errorf("build runtime %s failed: %w", symbol, fallbackErr)
	}
	if builderErr != nil {
		return runtime.SymbolRuntime{}, fmt.Errorf("build runtime %s failed: %w", symbol, builderErr)
	}
	return runtime.SymbolRuntime{}, fmt.Errorf("symbol %s not initialized", symbol)
}

func (r *RuntimeSymbolResolver) getRuntime(symbol string) (runtime.SymbolRuntime, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rt, ok := r.runtimes[symbol]
	return rt, ok
}

func (r *RuntimeSymbolResolver) setRuntime(symbol string, rt runtime.SymbolRuntime) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.runtimes == nil {
		r.runtimes = make(map[string]runtime.SymbolRuntime)
	}
	if _, exists := r.runtimes[symbol]; !exists {
		if r.maxDynamic > 0 && len(r.runtimes) >= r.baseCount+r.maxDynamic {
			return fmt.Errorf("dynamic runtime limit reached (max=%d)", r.maxDynamic)
		}
	}
	r.runtimes[symbol] = rt
	return nil
}
