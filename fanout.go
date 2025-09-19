package streamz

import (
	"context"
)

// FanOut distributes Result[T] items from a single input channel to multiple output channels.
// It implements the fan-out concurrency pattern using the Result[T] pattern for unified
// error handling, duplicating each Result to all outputs, enabling parallel processing
// of both successful values and errors.
type FanOut[T any] struct {
	name  string
	count int
}

// NewFanOut creates a processor that distributes Result[T] items to multiple output channels.
// This implements the fan-out concurrency pattern with Result[T] support, duplicating
// each input Result to all output channels, enabling parallel processing of both
// successful values and errors.
//
// When to use:
//   - Parallel processing of the same data with error handling
//   - Broadcasting events to multiple consumers that need error context
//   - Implementing publish-subscribe patterns with unified error handling
//   - Load distribution for CPU-intensive tasks with failure isolation
//   - Creating processing pipelines with multiple branches and error propagation
//
// Error behavior:
//   - Errors are duplicated to all output channels (each gets an independent copy)
//   - Each output channel receives exactly the same Result sequence
//   - No error transformation occurs - errors flow through unchanged
//   - Backpressure from slow consumers affects all outputs (blocking behavior)
//
// Example:
//
//	// Distribute Result events to 3 parallel processors
//	fanout := streamz.NewFanOut[Event](3)
//	outputs := fanout.Process(ctx, eventResults)
//
//	// Each output gets a copy of every Result (success or error)
//	go processResultStream1(outputs[0]) // Real-time alerting
//	go processResultStream2(outputs[1]) // Analytics
//	go processResultStream3(outputs[2]) // Archival
//
//	// Fan out for parallel enrichment with error handling
//	fanout := streamz.NewFanOut[Record](runtime.NumCPU())
//	branches := fanout.Process(ctx, recordResults)
//
//	// Process each branch with different enrichers
//	enriched := make([]<-chan Result[EnrichedRecord], len(branches))
//	for i, branch := range branches {
//		enriched[i] = enricher[i].Process(ctx, branch)
//	}
//	// Merge results back together with FanIn
//	merged := fanin.Process(ctx, enriched...)
//
// Parameters:
//   - count: Number of output channels to create
//
// Returns a new FanOut processor that broadcasts Result[T] to multiple outputs.
func NewFanOut[T any](count int) *FanOut[T] {
	return &FanOut[T]{
		count: count,
		name:  "fanout",
	}
}

// Process distributes Result[T] items from input to multiple output channels.
// Each Result (success or error) is duplicated to all output channels.
// The processor respects context cancellation and properly closes all output channels.
func (f *FanOut[T]) Process(ctx context.Context, in <-chan Result[T]) []<-chan Result[T] {
	outs := make([]<-chan Result[T], f.count)
	channels := make([]chan Result[T], f.count)

	for i := 0; i < f.count; i++ {
		channels[i] = make(chan Result[T])
		outs[i] = channels[i]
	}

	go func() {
		defer func() {
			for _, ch := range channels {
				close(ch)
			}
		}()

		for result := range in {
			for _, ch := range channels {
				select {
				case ch <- result:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return outs
}
