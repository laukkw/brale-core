package bootstrap

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"brale-core/internal/config"
	"brale-core/internal/execution"
	"brale-core/internal/llm/promptreg"
	"brale-core/internal/memory"
	"brale-core/internal/reconcile"
	"brale-core/internal/runtime"
	"brale-core/internal/store"

	"go.uber.org/zap"
)

type symbolPositionReflector struct {
	bySymbol map[string]*memory.ReconcileReflectorAdapter
}

func (r *symbolPositionReflector) ReflectOnClose(ctx context.Context, pos store.PositionRecord) {
	if r == nil {
		return
	}
	adapter := r.bySymbol[canonicalSymbol(pos.Symbol)]
	if adapter == nil {
		return
	}
	adapter.ReflectOnClose(ctx, pos)
}

func buildPositionReflector(sys config.SystemConfig, symbolIndexPath string, index config.SymbolIndexConfig, st store.Store, tradeFinder execution.TradeFinder) (reconcile.PositionReflector, error) {
	bySymbol := make(map[string]*memory.ReconcileReflectorAdapter)
	locale := config.NormalizePromptLocale(sys.Prompt.Locale)
	loader := promptreg.NewLoader(st, config.PromptRegistryDefaultsForLocale(locale), zap.NewNop())
	systemPrompt, promptVersion, err := loader.Resolve(context.Background(), "reflector", "analysis", locale)
	if err != nil {
		return nil, fmt.Errorf("resolve reflector prompt: %w", err)
	}
	for _, item := range index.Symbols {
		symbolKey := canonicalSymbolFromIndexEntry(item)
		if symbolKey == "" {
			continue
		}
		symbolCfg, err := loadReflectorSymbolConfig(sys, symbolIndexPath, item)
		if err != nil {
			return nil, fmt.Errorf("load symbol reflector config %s: %w", item.Symbol, err)
		}
		if !symbolCfg.Memory.EpisodicEnabled {
			continue
		}
		if tradeFinder == nil {
			return nil, fmt.Errorf("symbol %s has episodic memory enabled but no trade finder configured for reflection", item.Symbol)
		}
		role, ok := pickReflectorRole(symbolCfg)
		if !ok {
			return nil, fmt.Errorf("symbol %s has episodic memory enabled but no LLM role configured for reflection", item.Symbol)
		}
		episodic := buildReflectorEpisodicMemory(symbolCfg, st)
		if episodic == nil {
			continue
		}
		bySymbol[symbolKey] = &memory.ReconcileReflectorAdapter{
			TradeFinder: tradeFinder,
			Reflector: &memory.Reflector{
				LLM:           runtime.NewLLMClient(sys, role),
				Episodic:      episodic,
				Semantic:      buildReflectorSemanticMemory(symbolCfg, st),
				Store:         st,
				SystemPrompt:  systemPrompt,
				PromptVersion: promptVersion,
			},
		}
	}
	if len(bySymbol) == 0 {
		return nil, nil
	}
	return &symbolPositionReflector{bySymbol: bySymbol}, nil
}

func loadReflectorSymbolConfig(sys config.SystemConfig, symbolIndexPath string, item config.SymbolIndexEntry) (config.SymbolConfig, error) {
	base := filepath.Dir(symbolIndexPath)
	symbolPath := strings.TrimSpace(item.Config)
	if symbolPath == "" {
		return config.SymbolConfig{}, fmt.Errorf("symbol config path is empty")
	}
	if !filepath.IsAbs(symbolPath) {
		symbolPath = filepath.Join(base, symbolPath)
	}
	symbolCfg, err := config.LoadSymbolConfig(symbolPath)
	if err != nil {
		return config.SymbolConfig{}, err
	}
	if symbolCfg.Symbol != item.Symbol {
		return config.SymbolConfig{}, fmt.Errorf("symbol config mismatch: %s", symbolCfg.Symbol)
	}
	defaults, err := config.DefaultSymbolConfig(sys, item.Symbol)
	if err != nil {
		return config.SymbolConfig{}, err
	}
	config.ApplyDecisionDefaults(&symbolCfg, defaults)
	if err := config.ValidateSymbolLLMModels(sys, symbolCfg); err != nil {
		return config.SymbolConfig{}, err
	}
	return symbolCfg, nil
}

func pickReflectorRole(cfg config.SymbolConfig) (config.LLMRoleConfig, bool) {
	candidates := []config.LLMRoleConfig{
		cfg.LLM.Provider.Structure,
		cfg.LLM.Provider.Mechanics,
		cfg.LLM.Provider.Indicator,
		cfg.LLM.Agent.Structure,
		cfg.LLM.Agent.Mechanics,
		cfg.LLM.Agent.Indicator,
	}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.Model) != "" {
			return candidate, true
		}
	}
	return config.LLMRoleConfig{}, false
}

func buildReflectorEpisodicMemory(symbolCfg config.SymbolConfig, st store.Store) *memory.EpisodicMemory {
	if !symbolCfg.Memory.EpisodicEnabled {
		return nil
	}
	ttlDays := symbolCfg.Memory.EpisodicTTLDays
	if ttlDays <= 0 {
		ttlDays = config.DefaultEpisodicTTLDays
	}
	maxPerSymbol := symbolCfg.Memory.EpisodicMaxPerSymbol
	if maxPerSymbol <= 0 {
		maxPerSymbol = config.DefaultEpisodicMaxPerSymbol
	}
	return memory.NewEpisodicMemory(st, maxPerSymbol, ttlDays)
}

func buildReflectorSemanticMemory(symbolCfg config.SymbolConfig, st store.Store) *memory.SemanticMemory {
	if !symbolCfg.Memory.SemanticEnabled {
		return nil
	}
	maxRules := symbolCfg.Memory.SemanticMaxRules
	if maxRules <= 0 {
		maxRules = config.DefaultSemanticMaxRules
	}
	return memory.NewSemanticMemory(st, maxRules)
}
