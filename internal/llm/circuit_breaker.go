package llm

import (
	"errors"
	"sync"
	"time"
)

var ErrCircuitOpen = errors.New("llm circuit breaker is open")

type circuitState int

const (
	circuitClosed circuitState = iota
	circuitOpen
	circuitHalfOpen
)

type CircuitBreaker struct {
	mu            sync.Mutex
	state         circuitState
	failCount     int
	failThreshold int
	openDuration  time.Duration
	openedAt      time.Time
	probeInFlight bool
}

func NewCircuitBreaker(failThreshold int, openDuration time.Duration) *CircuitBreaker {
	if failThreshold <= 0 {
		failThreshold = 5
	}
	if openDuration <= 0 {
		openDuration = time.Minute
	}
	return &CircuitBreaker{
		failThreshold: failThreshold,
		openDuration:  openDuration,
	}
}

func (cb *CircuitBreaker) Allow() error {
	if cb == nil {
		return nil
	}
	cb.mu.Lock()
	defer cb.mu.Unlock()
	switch cb.state {
	case circuitOpen:
		if time.Since(cb.openedAt) >= cb.openDuration {
			cb.state = circuitHalfOpen
			cb.probeInFlight = true
			return nil
		}
		return ErrCircuitOpen
	case circuitHalfOpen:
		if cb.probeInFlight {
			return ErrCircuitOpen
		}
		cb.probeInFlight = true
		return nil
	default:
		return nil
	}
}

func (cb *CircuitBreaker) RecordSuccess() {
	if cb == nil {
		return
	}
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failCount = 0
	cb.state = circuitClosed
	cb.openedAt = time.Time{}
	cb.probeInFlight = false
}

func (cb *CircuitBreaker) RecordFailure() {
	if cb == nil {
		return
	}
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.state == circuitHalfOpen {
		cb.state = circuitOpen
		cb.openedAt = time.Now()
		cb.probeInFlight = false
		cb.failCount = cb.failThreshold
		return
	}
	cb.failCount++
	if cb.failCount >= cb.failThreshold {
		cb.state = circuitOpen
		cb.openedAt = time.Now()
		cb.probeInFlight = false
	}
}

type circuitBreakerRegistry struct {
	mu            sync.Mutex
	breakers      map[string]*CircuitBreaker
	failThreshold int
	openDuration  time.Duration
}

func newCircuitBreakerRegistry(failThreshold int, openDuration time.Duration) *circuitBreakerRegistry {
	return &circuitBreakerRegistry{
		breakers:      make(map[string]*CircuitBreaker),
		failThreshold: failThreshold,
		openDuration:  openDuration,
	}
}

func (r *circuitBreakerRegistry) get(model string) *CircuitBreaker {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := normalizeGateKey(model)
	if key == "" {
		key = "default"
	}
	if cb, ok := r.breakers[key]; ok {
		return cb
	}
	cb := NewCircuitBreaker(r.failThreshold, r.openDuration)
	r.breakers[key] = cb
	return cb
}

var defaultCircuitBreakers = newCircuitBreakerRegistry(5, time.Minute)
