package decisionview

import (
	"net/http"

	"brale-core/internal/config"
	"brale-core/internal/runtime"
	"brale-core/internal/store"

	"go.uber.org/zap"
)

// BuildViewerConfigs loads symbol configs for decision viewer.
func BuildViewerConfigs(logger *zap.Logger, sys config.SystemConfig, indexPath string, index config.SymbolIndexConfig) map[string]ConfigBundle {
	cfgs := make(map[string]ConfigBundle, len(index.Symbols))
	for _, item := range index.Symbols {
		symCfg, stratCfg, _, err := runtime.LoadSymbolConfigs(sys, indexPath, item)
		if err != nil {
			logger.Warn("load symbol config for viewer failed", zap.Error(err), zap.String("symbol", item.Symbol))
			continue
		}
		cfgs[item.Symbol] = ConfigBundle{
			Symbol:   symCfg,
			Strategy: stratCfg,
		}
	}
	return cfgs
}

// StartDecisionViewer builds decision viewer handler with defaults.
func StartDecisionViewer(logger *zap.Logger, sys config.SystemConfig, indexPath string, index config.SymbolIndexConfig, st store.Store) http.Handler {
	viewConfigs := BuildViewerConfigs(logger, sys, indexPath, index)
	viewer := Server{
		Store:         st,
		BasePath:      "/decision-view",
		RoundLimit:    50,
		SystemConfig:  sys,
		SymbolConfigs: viewConfigs,
	}
	h, err := viewer.Handler()
	if err != nil {
		logger.Warn("decision viewer init failed", zap.Error(err))
		return nil
	}
	return h
}
