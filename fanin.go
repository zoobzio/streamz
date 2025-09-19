package streamz

import (
	"context"
	"sync"
)

// FanIn merges multiple Result[T] input channels into a single output channel.
// It implements the fan-in concurrency pattern, collecting Results from multiple
// sources and combining them into a single stream. This version uses the Result[T]
// pattern for unified error handling instead of dual-channel returns.
type FanIn[T any] struct {
	name string
}

// NewFanIn creates a processor that merges multiple Result[T] channels into one.
// This implements the fan-in concurrency pattern, collecting Results from multiple
// sources and combining them into a single unified stream using the Result[T] pattern.
//
// When to use:
//   - Aggregating data from multiple sources with error handling
//   - Collecting results from parallel workers that may fail
//   - Merging event streams from different services
//   - Consolidating logs or metrics with error propagation
//   - Load balancing consumer implementation
//
// Example:
//
//	// Merge Results from multiple sources
//	fanin := streamz.NewFanIn[Event]()
//
//	// Combine Result streams from different services
//	merged := fanin.Process(ctx,
//		serviceA.EventResults(),
//		serviceB.EventResults(),
//		serviceC.EventResults())
//
//	// Process merged stream with unified error handling
//	for result := range merged {
//		if result.IsError() {
//			log.Printf("FanIn error: %v", result.Error())
//			continue
//		}
//		processEvent(result.Value())
//	}
//
//	// Collect Results from parallel workers
//	fanin := streamz.NewFanIn[ProcessedData]()
//	workers := make([]<-chan Result[ProcessedData], numWorkers)
//	for i := range workers {
//		workers[i] = startWorker(ctx, workQueue)
//	}
//	results := fanin.Process(ctx, workers...)
//
// Returns a new FanIn processor for merging multiple Result streams.
func NewFanIn[T any]() *FanIn[T] {
	return &FanIn[T]{
		name: "fanin",
	}
}

// Process merges multiple Result[T] channels into a single Result[T] channel.
// Both successful values and errors flow through the unified output channel.
// This eliminates the need for dual-channel error handling patterns.
func (*FanIn[T]) Process(ctx context.Context, ins ...<-chan Result[T]) <-chan Result[T] {
	out := make(chan Result[T])
	var wg sync.WaitGroup

	for _, in := range ins {
		wg.Add(1)
		go func(ch <-chan Result[T]) {
			defer wg.Done()
			for result := range ch {
				select {
				case out <- result:
				case <-ctx.Done():
					return
				}
			}
		}(in)
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}
