package llm

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	defaultMinInterval          = 5 * time.Second
	defaultSameModelConcurrency = 1
)

var defaultModelGates = NewModelGateRegistry(defaultSameModelConcurrency)

func init() {
	defaultModelGates.SetMinInterval(defaultMinInterval)
}

func AcquireModel(ctx context.Context, model string) (func(), error) {
	return defaultModelGates.Acquire(ctx, model)
}

func SetModelCooldown(model string, until time.Time) {
	defaultModelGates.SetCooldown(model, until)
}

func ConfigureMinInterval(min time.Duration) {
	defaultModelGates.SetMinInterval(min)
}

func ConfigureModelConcurrency(defaultLimit int, modelLimits map[string]int) {
	defaultModelGates.SetDefaultLimit(defaultLimit)
	defaultModelGates.SetModelLimits(modelLimits)
}

type ModelGateRegistry struct {
	mu           sync.Mutex
	defaultLimit int
	minInterval  time.Duration
	modelLimits  map[string]int
	gates        map[string]*modelGate
}

type modelGate struct {
	sem chan struct{}

	mu            sync.Mutex
	cooldownUntil time.Time
	lastAcquire   time.Time
	minInterval   time.Duration
}

func NewModelGateRegistry(defaultLimit int) *ModelGateRegistry {
	defaultLimit = max(1, defaultLimit)
	return &ModelGateRegistry{defaultLimit: defaultLimit, minInterval: 0, modelLimits: make(map[string]int), gates: make(map[string]*modelGate)}
}

func (r *ModelGateRegistry) SetDefaultLimit(limit int) {
	if limit <= 0 {
		limit = 1
	}
	r.mu.Lock()
	r.defaultLimit = limit
	r.mu.Unlock()
}

func (r *ModelGateRegistry) SetModelLimits(limits map[string]int) {
	normalized := make(map[string]int)
	for key, limit := range limits {
		gateKey := normalizeGateKey(key)
		if gateKey == "" {
			continue
		}
		normalized[gateKey] = max(1, limit)
	}
	r.mu.Lock()
	r.modelLimits = normalized
	r.mu.Unlock()
}

func (r *ModelGateRegistry) Acquire(ctx context.Context, key string) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	key = normalizeGateKey(key)
	if key == "" {
		return nil, fmt.Errorf("gate key is required")
	}
	g := r.getOrCreate(key)
	for {
		if err := g.waitCooldown(ctx); err != nil {
			return nil, err
		}
		select {
		case g.sem <- struct{}{}:
			var once sync.Once
			release := func() {
				once.Do(func() {
					select {
					case <-g.sem:
					default:
					}
				})
			}
			if g.isCoolingDown() {
				release()
				continue
			}
			if err := g.waitMinInterval(ctx); err != nil {
				release()
				return nil, err
			}
			g.markAcquire()
			return release, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (r *ModelGateRegistry) SetCooldown(key string, until time.Time) {
	key = normalizeGateKey(key)
	if key == "" {
		return
	}
	g := r.getOrCreate(key)
	g.setCooldown(until)
}

func (r *ModelGateRegistry) SetMinInterval(min time.Duration) {
	if min < 0 {
		min = 0
	}
	r.mu.Lock()
	r.minInterval = min
	for _, g := range r.gates {
		g.setMinInterval(min)
	}
	r.mu.Unlock()
}

func (r *ModelGateRegistry) getOrCreate(key string) *modelGate {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.gates == nil {
		r.gates = make(map[string]*modelGate)
	}
	if g, ok := r.gates[key]; ok {
		return g
	}
	limit := r.defaultLimit
	if configuredLimit, ok := r.modelLimits[key]; ok {
		limit = configuredLimit
	}
	g := &modelGate{sem: make(chan struct{}, limit), minInterval: r.minInterval}
	r.gates[key] = g
	return g
}

func (g *modelGate) setCooldown(until time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if until.After(g.cooldownUntil) {
		g.cooldownUntil = until
	}
}

func (g *modelGate) setMinInterval(min time.Duration) {
	g.mu.Lock()
	g.minInterval = min
	g.mu.Unlock()
}

func (g *modelGate) isCoolingDown() bool {
	g.mu.Lock()
	until := g.cooldownUntil
	g.mu.Unlock()
	if until.IsZero() {
		return false
	}
	return time.Now().Before(until)
}

func (g *modelGate) waitCooldown(ctx context.Context) error {
	g.mu.Lock()
	until := g.cooldownUntil
	g.mu.Unlock()
	if until.IsZero() {
		return nil
	}
	wait := time.Until(until)
	if wait <= 0 {
		return nil
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g *modelGate) waitMinInterval(ctx context.Context) error {
	g.mu.Lock()
	min := g.minInterval
	last := g.lastAcquire
	g.mu.Unlock()
	if min <= 0 || last.IsZero() {
		return nil
	}
	wait := time.Until(last.Add(min))
	if wait <= 0 {
		return nil
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g *modelGate) markAcquire() {
	g.mu.Lock()
	g.lastAcquire = time.Now()
	g.mu.Unlock()
}

func normalizeGateKey(key string) string {
	return strings.TrimSpace(key)
}
