package llm

import "context"

type callMetadataContextKey struct{}
type callObserverContextKey struct{}

type CallMetadata struct {
	Role          string
	Stage         string
	Symbol        string
	PromptVersion string
}

type CallStats struct {
	Model         string
	Endpoint      string
	Role          string
	Stage         string
	Symbol        string
	PromptVersion string
	LatencyMs     int64
	TokenIn       int
	TokenOut      int
	Err           error
}

type CallObserver interface {
	ObserveCall(ctx context.Context, stats CallStats)
}

type CallObserverFunc func(ctx context.Context, stats CallStats)

func (f CallObserverFunc) ObserveCall(ctx context.Context, stats CallStats) {
	f(ctx, stats)
}

func WithCallMetadata(ctx context.Context, meta CallMetadata) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, callMetadataContextKey{}, meta)
}

func CallMetadataFromContext(ctx context.Context) (CallMetadata, bool) {
	if ctx == nil {
		return CallMetadata{}, false
	}
	meta, ok := ctx.Value(callMetadataContextKey{}).(CallMetadata)
	return meta, ok
}

func WithCallObserver(ctx context.Context, observer CallObserver) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, callObserverContextKey{}, observer)
}

func CallObserverFromContext(ctx context.Context) (CallObserver, bool) {
	if ctx == nil {
		return nil, false
	}
	observer, ok := ctx.Value(callObserverContextKey{}).(CallObserver)
	return observer, ok
}

func ObserveCall(ctx context.Context, stats CallStats) {
	observer, ok := CallObserverFromContext(ctx)
	if !ok || observer == nil {
		return
	}
	observer.ObserveCall(ctx, stats)
}

func ChainCallObservers(observers ...CallObserver) CallObserver {
	filtered := make([]CallObserver, 0, len(observers))
	for _, observer := range observers {
		if observer != nil {
			filtered = append(filtered, observer)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return CallObserverFunc(func(ctx context.Context, stats CallStats) {
		for _, observer := range filtered {
			observer.ObserveCall(ctx, stats)
		}
	})
}
