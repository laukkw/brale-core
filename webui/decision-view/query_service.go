package decisionview

import (
	"context"
	"strings"

	"brale-core/internal/position"
	"brale-core/internal/store"
)

type symbolEvents struct {
	providers []store.ProviderEventRecord
	agents    []store.AgentEventRecord
	gates     []store.GateEventRecord
}

func buildResponse(ctx context.Context, st decisionViewStore, symbols []string, limit int) (Response, error) {
	out := Response{Symbols: make([]SymbolChain, 0, len(symbols))}
	for _, sym := range symbols {
		if strings.TrimSpace(sym) == "" {
			continue
		}
		chain, err := buildSymbolChain(ctx, st, sym, limit)
		if err != nil {
			return Response{}, err
		}
		out.Symbols = append(out.Symbols, chain)
	}
	return out, nil
}

func buildSymbolChain(ctx context.Context, st decisionViewStore, symbol string, limit int) (SymbolChain, error) {
	events, openPos, hasOpenPos, err := loadSymbolChainData(ctx, st, symbol, limit)
	if err != nil {
		return SymbolChain{}, err
	}
	return buildChain(ctx, symbol, events, openPos, hasOpenPos, limit), nil
}

func loadSymbolChainData(ctx context.Context, st decisionViewStore, symbol string, limit int) (symbolEvents, store.PositionRecord, bool, error) {
	events, err := loadSymbolEvents(ctx, st, symbol, limit)
	if err != nil {
		return symbolEvents{}, store.PositionRecord{}, false, err
	}
	openPos, hasOpenPos, err := st.FindPositionBySymbol(ctx, symbol, position.OpenPositionStatuses)
	if err != nil {
		return symbolEvents{}, store.PositionRecord{}, false, err
	}
	return events, openPos, hasOpenPos, nil
}

func loadSymbolEvents(ctx context.Context, st store.TimelineQueryStore, symbol string, limit int) (symbolEvents, error) {
	providers, err := st.ListProviderEvents(ctx, symbol, limit*4)
	if err != nil {
		return symbolEvents{}, err
	}
	agents, err := st.ListAgentEvents(ctx, symbol, limit*4)
	if err != nil {
		return symbolEvents{}, err
	}
	gates, err := st.ListGateEvents(ctx, symbol, limit*2)
	if err != nil {
		return symbolEvents{}, err
	}
	return symbolEvents{providers: providers, agents: agents, gates: gates}, nil
}
