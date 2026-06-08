package workflow

import (
	"context"
	"fmt"
	"sync"
)

// Pipeline runs each item through all stages independently. There is
// NO barrier between stages — item A can be in stage 3 while item B
// is still in stage 1. Wall-clock time is the slowest single-item
// chain, not the sum-of-slowest-per-stage.
//
// Each stage callback receives the previous stage's result (or the
// item itself for stage 0), the original item, and the item's index.
// Use originalItem/index in later stages to label work without
// threading context through stage 1's return value.
//
// A stage that returns an error drops that item to nil and skips
// remaining stages for that item. The pipeline itself never fails —
// check individual results for errors.
//
//	results := engine.Pipeline(ctx, items,
//	    func(prev any, item string, idx int) (any, error) { ... },  // stage 0
//	    func(prev any, item string, idx int) (any, error) { ... },  // stage 1
//	)
func (e *Engine) Pipeline(ctx context.Context, items []string, stages ...PipelineStage) []*PipelineResult {
	if len(items) == 0 || len(stages) == 0 {
		return nil
	}

	type indexedResult struct {
		idx int
		res *PipelineResult
	}

	results := make(chan indexedResult, len(items))
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrent(len(items)))

	for i, item := range items {
		wg.Add(1)
		go func(idx int, it string) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					results <- indexedResult{
						idx: idx,
						res: &PipelineResult{
							Item:  it,
							Error: fmt.Errorf("pipeline panic: %v", r),
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
					res: &PipelineResult{Item: it, Error: ctx.Err()},
				}
				return
			}

			var prev any
			for si, stage := range stages {
				out, err := stage(ctx, prev, it, idx, si)
				if err != nil {
					results <- indexedResult{
						idx: idx,
						res: &PipelineResult{
							Item:     it,
							StageIdx: si,
							Output:   fmt.Sprintf("%v", out),
							Error:    err,
						},
					}
					return
				}
				prev = out
			}

			results <- indexedResult{
				idx: idx,
				res: &PipelineResult{
					Item:   it,
					Output: fmt.Sprintf("%v", prev),
				},
			}
		}(i, item)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	out := make([]*PipelineResult, len(items))
	for r := range results {
		out[r.idx] = r.res
	}
	return out
}

// PipelineStage is one step in a pipeline. It receives:
//   - prev: the previous stage's output (nil for stage 0)
//   - item: the original item being processed
//   - idx: the item's index in the input slice
//   - stageIdx: which stage this is (0-based)
type PipelineStage func(ctx context.Context, prev any, item string, idx, stageIdx int) (any, error)

// PipelineResult is the final output for one item through the pipeline.
type PipelineResult struct {
	Item     string // original input item
	Output   string // final stage output (or partial if errored)
	StageIdx int    // which stage produced the error (-1 if all succeeded)
	Error    error  // nil if all stages succeeded
}

// OK returns true when all stages completed without error.
func (r *PipelineResult) OK() bool { return r != nil && r.Error == nil }
