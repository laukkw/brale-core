package llm

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

type SessionFactory func() (string, error)

type SessionCleanup func(sessionID string) error

type RoundSessionManager struct {
	mu    sync.Mutex
	lanes map[RoundLaneKey]*laneSessionState
}

type laneSessionState struct {
	mode      SessionMode
	sessionID string
	ready     bool
	creating  bool
	wait      chan struct{}
}

func NewRoundSessionManager() *RoundSessionManager {
	return &RoundSessionManager{lanes: map[RoundLaneKey]*laneSessionState{}}
}

func (m *RoundSessionManager) AcquireOrCreate(key RoundLaneKey, factory SessionFactory) (string, bool, error) {
	if m == nil {
		return "", false, nil
	}
	if factory == nil {
		return "", false, fmt.Errorf("session factory is required")
	}

	for {
		m.mu.Lock()
		state := m.getOrCreateLocked(key)
		if state.mode == SessionModeStateless {
			m.mu.Unlock()
			return "", false, nil
		}
		if state.ready {
			sessionID := state.sessionID
			m.mu.Unlock()
			return sessionID, true, nil
		}
		if state.creating {
			wait := state.wait
			m.mu.Unlock()
			<-wait
			continue
		}

		state.creating = true
		state.wait = make(chan struct{})
		m.mu.Unlock()

		sessionID, err := factory()

		m.mu.Lock()
		if state.mode == SessionModeStateless {
			state.creating = false
			m.closeWaitLocked(state)
			m.mu.Unlock()
			return "", false, nil
		}
		if err != nil {
			state.creating = false
			m.closeWaitLocked(state)
			m.mu.Unlock()
			return "", false, err
		}
		state.sessionID = sessionID
		state.ready = true
		state.creating = false
		m.closeWaitLocked(state)
		m.mu.Unlock()

		return sessionID, false, nil
	}
}

func (m *RoundSessionManager) MarkFallback(key RoundLaneKey) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.getOrCreateLocked(key)
	state.mode = SessionModeStateless
	state.sessionID = ""
	state.ready = false
	if state.creating {
		state.creating = false
		m.closeWaitLocked(state)
	}
}

func (m *RoundSessionManager) Mode(key RoundLaneKey) SessionMode {
	if m == nil {
		return SessionModeSession
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if state, ok := m.lanes[key]; ok {
		return state.mode
	}
	return SessionModeSession
}

func (m *RoundSessionManager) CleanupRound(roundID RoundID, cleanup SessionCleanup) error {
	if m == nil {
		return nil
	}
	lanes := m.takeLanesLocked(func(key RoundLaneKey) bool {
		parsedRoundID, _, ok := splitRoundLaneKey(key)
		return ok && parsedRoundID == roundID.String()
	})
	return cleanupLaneSessions(lanes, cleanup)
}

func (m *RoundSessionManager) CleanupSymbolRound(roundID RoundID, symbol string, cleanup SessionCleanup) error {
	if m == nil {
		return nil
	}
	trimmedSymbol := strings.TrimSpace(symbol)
	lanes := m.takeLanesLocked(func(key RoundLaneKey) bool {
		parsedRoundID, parsedSymbol, ok := splitRoundLaneKey(key)
		if !ok {
			return false
		}
		return parsedRoundID == roundID.String() && parsedSymbol == trimmedSymbol
	})
	return cleanupLaneSessions(lanes, cleanup)
}

func (m *RoundSessionManager) getOrCreateLocked(key RoundLaneKey) *laneSessionState {
	if state, ok := m.lanes[key]; ok {
		return state
	}
	state := &laneSessionState{mode: SessionModeSession}
	m.lanes[key] = state
	return state
}

func (m *RoundSessionManager) closeWaitLocked(state *laneSessionState) {
	if state.wait == nil {
		return
	}
	close(state.wait)
	state.wait = nil
}

func (m *RoundSessionManager) takeLanesLocked(match func(key RoundLaneKey) bool) []*laneSessionState {
	m.mu.Lock()
	defer m.mu.Unlock()

	lanes := make([]*laneSessionState, 0)
	for key, state := range m.lanes {
		if !match(key) {
			continue
		}
		delete(m.lanes, key)
		if state.creating {
			state.creating = false
			m.closeWaitLocked(state)
		}
		lanes = append(lanes, state)
	}
	return lanes
}

func cleanupLaneSessions(lanes []*laneSessionState, cleanup SessionCleanup) error {
	if cleanup == nil {
		return nil
	}

	errs := make([]error, 0)
	for _, lane := range lanes {
		if lane == nil || !lane.ready || strings.TrimSpace(lane.sessionID) == "" {
			continue
		}
		if err := safeCleanup(cleanup, lane.sessionID); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func safeCleanup(cleanup SessionCleanup, sessionID string) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("session cleanup panic: %v", recovered)
		}
	}()
	return cleanup(sessionID)
}

func splitRoundLaneKey(key RoundLaneKey) (string, string, bool) {
	parts := strings.Split(key.String(), roundLaneKeyDelimiter)
	if len(parts) != 4 {
		return "", "", false
	}
	return parts[0], parts[1], true
}
