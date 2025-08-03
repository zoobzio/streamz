package streamz

import (
	"context"
	"sync"
)

// FanIn merges multiple input channels into a single output channel.
// It implements the fan-in concurrency pattern, collecting items from multiple
// sources and combining them into a single stream.
type FanIn[T any] struct {
	name string
}

// NewFanIn creates a processor that merges multiple input channels into one.
// This implements the fan-in concurrency pattern, collecting items from multiple
// sources and combining them into a single unified stream.
//
// When to use:
//   - Aggregating data from multiple sources
//   - Collecting results from parallel workers
//   - Merging event streams from different services
//   - Consolidating logs or metrics
//   - Load balancing consumer implementation
//
// Example:
//
//	// Merge events from multiple sources
//	fanin := streamz.NewFanIn[Event]()
//
//	// Combine streams from different services
//	merged := fanin.Process(ctx,
//		serviceA.Events(),
//		serviceB.Events(),
//		serviceC.Events())
//
//	for event := range merged {
//		// Process events from all sources
//		processEvent(event)
//	}
//
//	// Collect results from parallel workers
//	fanin := streamz.NewFanIn[Result]()
//	workers := make([]<-chan Result, numWorkers)
//	for i := range workers {
//		workers[i] = startWorker(ctx, workQueue)
//	}
//	results := fanin.Process(ctx, workers...)
//
// Returns a new FanIn processor for merging multiple streams.
func NewFanIn[T any]() *FanIn[T] {
	return &FanIn[T]{
		name: "fanin",
	}
}

func (*FanIn[T]) Process(ctx context.Context, ins ...<-chan T) <-chan T {
	out := make(chan T)
	var wg sync.WaitGroup

	for _, in := range ins {
		wg.Add(1)
		go func(ch <-chan T) {
			defer wg.Done()
			for item := range ch {
				select {
				case out <- item:
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
