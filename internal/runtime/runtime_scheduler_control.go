package runtime

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

type streamTransition struct {
	shouldStart bool
	shouldStop  bool
	streamCtx   context.Context
}

func (s *RuntimeScheduler) SetScheduledDecision(enable bool) {
	s.mu.Lock()
	s.EnableScheduledDecision = enable
	s.ensureSymbolModesLocked()
	for symbol := range s.Symbols {
		mode := s.symbolModes[symbol]
		if mode == SymbolModeOff {
			continue
		}
		if enable {
			s.symbolModes[symbol] = SymbolModeTrade
			continue
		}
		s.symbolModes[symbol] = SymbolModeObserve
	}
	transition := s.planStreamTransitionLocked()
	s.mu.Unlock()
	s.applyStreamTransition(transition)
}

func (s *RuntimeScheduler) SetMonitorSymbols(symbols []string) error {
	if s == nil {
		return fmt.Errorf("scheduler is nil")
	}
	nextMonitorSymbols := make(map[string]struct{}, len(symbols))
	for _, symbol := range symbols {
		if symbol == "" {
			continue
		}
		if _, ok := s.Symbols[symbol]; !ok {
			return fmt.Errorf("symbol %s not found", symbol)
		}
		nextMonitorSymbols[symbol] = struct{}{}
	}

	s.mu.Lock()
	s.monitorSymbols = nextMonitorSymbols
	transition := s.planStreamTransitionLocked()
	s.mu.Unlock()
	s.applyStreamTransition(transition)
	return nil
}

func (s *RuntimeScheduler) ClearMonitorSymbols() {
	s.mu.Lock()
	if s.monitorSymbols == nil {
		s.mu.Unlock()
		return
	}
	for symbol := range s.monitorSymbols {
		delete(s.monitorSymbols, symbol)
	}
	transition := s.planStreamTransitionLocked()
	s.mu.Unlock()
	s.applyStreamTransition(transition)
}

func (s *RuntimeScheduler) GetScheduledDecision() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.EnableScheduledDecision
}

func (s *RuntimeScheduler) SetSymbolMode(symbol string, mode SymbolMode) error {
	if s == nil {
		return fmt.Errorf("scheduler is nil")
	}
	s.mu.Lock()
	if s.symbolModes == nil {
		s.symbolModes = make(map[string]SymbolMode)
	}
	if _, ok := s.Symbols[symbol]; !ok {
		s.mu.Unlock()
		return fmt.Errorf("symbol %s not found", symbol)
	}
	s.symbolModes[symbol] = mode
	transition := s.planStreamTransitionLocked()
	s.mu.Unlock()
	s.applyStreamTransition(transition)
	return nil
}

func (s *RuntimeScheduler) getSymbolMode(symbol string) SymbolMode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.symbolModes == nil {
		return SymbolModeTrade
	}
	mode, ok := s.symbolModes[symbol]
	if !ok {
		return SymbolModeTrade
	}
	return mode
}

func (s *RuntimeScheduler) isSymbolMonitored(symbol string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.monitorSymbols == nil {
		return false
	}
	_, ok := s.monitorSymbols[symbol]
	return ok
}

func (s *RuntimeScheduler) allSymbolsIdleLocked() bool {
	if s.symbolModes == nil {
		return false
	}
	for sym := range s.Symbols {
		if s.symbolModes[sym] == SymbolModeTrade {
			return false
		}
	}
	return true
}

func (s *RuntimeScheduler) ensureSymbolModesLocked() {
	if s.symbolModes == nil {
		s.symbolModes = make(map[string]SymbolMode, len(s.Symbols))
	}
	for symbol := range s.Symbols {
		if _, ok := s.symbolModes[symbol]; !ok {
			s.symbolModes[symbol] = SymbolModeTrade
		}
	}
}

func (s *RuntimeScheduler) shouldPriceStreamBeRunningLocked() bool {
	if len(s.monitorSymbols) > 0 {
		return true
	}
	if !s.EnableScheduledDecision {
		return false
	}
	return !s.allSymbolsIdleLocked()
}

func (s *RuntimeScheduler) planStreamTransitionLocked() streamTransition {
	started, currentStreamCtx := s.lifecycleSnapshot()
	if !started {
		return streamTransition{}
	}
	if s.shouldPriceStreamBeRunningLocked() {
		return streamTransition{shouldStart: true, streamCtx: currentStreamCtx}
	}
	return streamTransition{shouldStop: true}
}

func (s *RuntimeScheduler) applyStreamTransition(transition streamTransition) {
	if transition.shouldStart {
		s.startPriceStream(transition.streamCtx)
		s.startLiquidationStream(transition.streamCtx)
	}
	if transition.shouldStop {
		s.stopPriceStream()
		s.stopLiquidationStream()
	}
}

func (s *RuntimeScheduler) startPriceStream(ctx context.Context) {
	if s.PriceStream == nil || ctx == nil {
		return
	}
	if err := s.PriceStream.Start(ctx); err != nil && s.Logger != nil {
		s.Logger.Warn("price stream start failed", zap.Error(err))
	}
}

func (s *RuntimeScheduler) stopPriceStream() {
	if s.PriceStream == nil {
		return
	}
	s.PriceStream.Close()
}

func (s *RuntimeScheduler) startLiquidationStream(ctx context.Context) {
	if s.LiquidationStream == nil || ctx == nil {
		return
	}
	if err := s.LiquidationStream.Start(ctx); err != nil && s.Logger != nil {
		s.Logger.Warn("liquidation stream start failed", zap.Error(err))
	}
}

func (s *RuntimeScheduler) stopLiquidationStream() {
	if s.LiquidationStream == nil {
		return
	}
	s.LiquidationStream.Close()
}
