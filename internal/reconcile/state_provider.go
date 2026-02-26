package reconcile

import (
	"context"
	"fmt"

	"brale-core/internal/decision/fsm"
	"brale-core/internal/position"
	"brale-core/internal/store"
)

type FSMStateProvider struct {
	Store store.Store
}

func NewFSMStateProvider(s store.Store) *FSMStateProvider {
	return &FSMStateProvider{Store: s}
}

func (p *FSMStateProvider) Load(ctx context.Context, symbol string) (fsm.PositionState, string, error) {
	if p == nil || p.Store == nil {
		return "", "", fmt.Errorf("state provider store is nil")
	}
	pos, found, err := p.Store.FindPositionBySymbol(ctx, symbol, position.OpenPositionStatuses)
	if err != nil {
		return fsm.StateFlat, "", err
	}
	if found {
		return fsm.StateInPosition, pos.PositionID, nil
	}

	return fsm.StateFlat, "", nil
}
