package workflow

import (
	"context"
	"fmt"
	"sync"
)

// Parallel runs n agent thunks concurrently and returns their results.
// A thunk that panics or its agent call errors resolves to nil in the
// result slice — the call itself never panics, so the orchestrator
// can filter and continue.
//
// This is a BARRIER: all thunks complete before Parallel returns.
// Use it only when downstream work genuinely needs all results
// together (dedup, merge, early-exit on zero findings). For
// independent item processing where each item flows through
// multiple stages, use Pipeline instead.
//
// Concurrency is capped by the engine's max concurrent agents (16
// or fewer on limited CPUs). Excess thunks queue and run as slots
// free up.
func (e *Engine) Parallel(ctx context.Context, thunks []func() (string, error)) []*ParallelResult {
	if len(thunks) == 0 {
		return nil
	}

	type indexedResult struct {
		idx int
		res *ParallelResult
	}

	sem := make(chan struct{}, maxConcurrent(len(thunks)))
	results := make(chan indexedResult, len(thunks))
	var wg sync.WaitGroup

	for i, thunk := range thunks {
		wg.Add(1)
		go func(idx int, fn func() (string, error)) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					results <- indexedResult{
						idx: idx,
						res: &ParallelResult{
							Output: "",
							Error:  fmt.Errorf("agent panic: %v", r),
						},
					}
				}
			}()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results <- indexedResult{
					idx: idx,
					res: &ParallelResult{
						Error: ctx.Err(),
					},
				}
				return
			}

			output, err := fn()
			results <- indexedResult{
				idx: idx,
				res: &ParallelResult{
					Output: output,
					Error:  err,
				},
			}
		}(i, thunk)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect in order
	out := make([]*ParallelResult, len(thunks))
	for r := range results {
		out[r.idx] = r.res
	}
	return out
}

// ParallelResult is the outcome of one agent in a parallel batch.
type ParallelResult struct {
	Output string
	Error  error
}

// OK returns true when the agent completed without error.
func (r *ParallelResult) OK() bool { return r != nil && r.Error == nil }

// maxConcurrent returns the concurrency cap, bounded by len and
// the engine's maximum (16 by default, fewer on constrained CPUs).
func maxConcurrent(total int) int {
	cap := 16
	if total < cap {
		cap = total
	}
	if cap < 1 {
		cap = 1
	}
	return cap
}
