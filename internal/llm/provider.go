package llm

import "context"

type Provider interface {
	Call(ctx context.Context, system, user string) (string, error)
}
