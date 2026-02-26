package parallel

import (
	"context"
	"fmt"
	"sync"
)

// RunBestEffort runs all funcs and collects per-task errors.
// It does not cancel siblings on error; caller context governs cancellation.
func RunBestEffort(ctx context.Context, fns ...func(context.Context) error) []error {
	if len(fns) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	errs := make([]error, len(fns))
	var wg sync.WaitGroup
	wg.Add(len(fns))
	for i, fn := range fns {
		i, fn := i, fn
		if fn == nil {
			errs[i] = fmt.Errorf("nil function at index %d", i)
			wg.Done()
			continue
		}
		go func() {
			defer wg.Done()
			errs[i] = fn(ctx)
		}()
	}

	wg.Wait()
	return errs
}
