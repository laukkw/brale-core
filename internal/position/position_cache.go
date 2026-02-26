package position

import (
	"strings"
	"sync"

	"brale-core/internal/execution"
	symbolpkg "brale-core/internal/pkg/symbol"
	"brale-core/internal/store"
)

type PositionSnapshot struct {
	PositionID         string
	Symbol             string
	Qty                float64
	AvgEntry           float64
	InitialStake       float64
	LastPrice          float64
	LastPriceTimestamp int64
	UpdatedAt          int64
}

type PositionCache struct {
	mu               sync.RWMutex
	byID             map[string]PositionSnapshot
	bySymbol         map[string]string
	closeReasonByExt map[string]string
}

func NewPositionCache() *PositionCache {
	return &PositionCache{
		byID:             make(map[string]PositionSnapshot),
		bySymbol:         make(map[string]string),
		closeReasonByExt: make(map[string]string),
	}
}

func (c *PositionCache) Upsert(snapshot PositionSnapshot) {
	if c == nil {
		return
	}
	if strings.TrimSpace(snapshot.PositionID) == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byID[snapshot.PositionID] = snapshot
	if symbol := normalizeSymbol(snapshot.Symbol); symbol != "" {
		c.bySymbol[symbol] = snapshot.PositionID
	}
}

func (c *PositionCache) UpdateFromExternal(ext execution.ExternalPosition) PositionSnapshot {
	snapshot := PositionSnapshot{
		PositionID:   ext.PositionID,
		Symbol:       ext.Symbol,
		Qty:          ext.Quantity,
		AvgEntry:     ext.AvgEntry,
		InitialStake: ext.InitialStake,
		LastPrice:    ext.CurrentPrice,
		UpdatedAt:    ext.UpdatedAt,
	}
	if ext.UpdatedAt > 0 {
		snapshot.LastPriceTimestamp = ext.UpdatedAt
	}
	c.Upsert(snapshot)
	return snapshot
}

func (c *PositionCache) UpdatePrice(positionID, symbol string, price float64, ts int64) {
	if c == nil {
		return
	}
	if strings.TrimSpace(positionID) == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	snap := c.byID[positionID]
	if snap.PositionID == "" {
		snap.PositionID = positionID
		snap.Symbol = symbol
	}
	if price > 0 {
		snap.LastPrice = price
	}
	if ts > 0 {
		snap.LastPriceTimestamp = ts
		snap.UpdatedAt = ts
	}
	c.byID[positionID] = snap
	if sym := normalizeSymbol(snap.Symbol); sym != "" {
		c.bySymbol[sym] = snap.PositionID
	}
}

func (c *PositionCache) GetByID(positionID string) (PositionSnapshot, bool) {
	if c == nil {
		return PositionSnapshot{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	snap, ok := c.byID[positionID]
	return snap, ok
}

func (c *PositionCache) GetBySymbol(symbol string) (PositionSnapshot, bool) {
	if c == nil {
		return PositionSnapshot{}, false
	}
	symbol = normalizeSymbol(symbol)
	if symbol == "" {
		return PositionSnapshot{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	posID, ok := c.bySymbol[symbol]
	if !ok {
		return PositionSnapshot{}, false
	}
	snap, ok := c.byID[posID]
	return snap, ok
}

func (c *PositionCache) FindPositionIDBySymbol(symbol string) (string, bool) {
	if c == nil {
		return "", false
	}
	symbol = normalizeSymbol(symbol)
	if symbol == "" {
		return "", false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	posID, ok := c.bySymbol[symbol]
	return posID, ok
}

func (c *PositionCache) DeleteByID(positionID string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	snap, ok := c.byID[positionID]
	if ok {
		delete(c.bySymbol, normalizeSymbol(snap.Symbol))
	}
	delete(c.byID, positionID)
}

func (c *PositionCache) SetCloseReason(externalID string, reason string) {
	if c == nil {
		return
	}
	externalID = strings.TrimSpace(externalID)
	if externalID == "" {
		return
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return
	}
	c.mu.Lock()
	c.closeReasonByExt[externalID] = reason
	c.mu.Unlock()
}

func (c *PositionCache) GetCloseReason(externalID string) (string, bool) {
	if c == nil {
		return "", false
	}
	externalID = strings.TrimSpace(externalID)
	if externalID == "" {
		return "", false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.closeReasonByExt[externalID]
	return val, ok
}

func (c *PositionCache) HydratePosition(pos store.PositionRecord) store.PositionRecord {
	if c == nil {
		return pos
	}
	if strings.TrimSpace(pos.PositionID) != "" {
		if snap, ok := c.GetByID(pos.PositionID); ok {
			return mergeSnapshot(pos, snap)
		}
	}
	if strings.TrimSpace(pos.Symbol) != "" {
		if snap, ok := c.GetBySymbol(pos.Symbol); ok {
			return mergeSnapshot(pos, snap)
		}
	}
	return pos
}

func mergeSnapshot(pos store.PositionRecord, snap PositionSnapshot) store.PositionRecord {
	if snap.Qty > 0 {
		pos.Qty = snap.Qty
	}
	if snap.AvgEntry > 0 {
		pos.AvgEntry = snap.AvgEntry
	}
	pos.LastPrice = snap.LastPrice
	pos.LastPriceTimestamp = snap.LastPriceTimestamp
	if pos.InitialStake == 0 && snap.InitialStake > 0 {
		pos.InitialStake = snap.InitialStake
	}
	return pos
}

func normalizeSymbol(symbol string) string {
	return symbolpkg.Normalize(symbol)
}
