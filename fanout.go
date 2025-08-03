package streamz

import (
	"context"
)

// FanOut distributes items from a single input channel to multiple output channels.
// It implements the fan-out concurrency pattern, duplicating each item to all outputs,
// enabling parallel processing of the same data stream.
type FanOut[T any] struct {
	name  string
	count int
}

// NewFanOut creates a processor that distributes items to multiple output channels.
// This implements the fan-out concurrency pattern, duplicating each input item to
// all output channels, enabling parallel processing of the same data.
//
// When to use:
//   - Parallel processing of the same data
//   - Broadcasting events to multiple consumers
//   - Implementing publish-subscribe patterns
//   - Load distribution for CPU-intensive tasks
//   - Creating processing pipelines with multiple branches
//
// Example:
//
//	// Distribute events to 3 parallel processors
//	fanout := streamz.NewFanOut[Event](3)
//	outputs := fanout.Process(ctx, events)
//
//	// Each output gets a copy of every event
//	go processStream1(outputs[0]) // Real-time alerting
//	go processStream2(outputs[1]) // Analytics
//	go processStream3(outputs[2]) // Archival
//
//	// Fan out for parallel enrichment
//	fanout := streamz.NewFanOut[Record](runtime.NumCPU())
//	branches := fanout.Process(ctx, records)
//
//	// Process each branch with different enrichers
//	enriched := make([]<-chan EnrichedRecord, len(branches))
//	for i, branch := range branches {
//		enriched[i] = enricher[i].Process(ctx, branch)
//	}
//	// Merge results back together
//	merged := fanin.Process(ctx, enriched...)
//
// Parameters:
//   - count: Number of output channels to create
//
// Returns a new FanOut processor that broadcasts to multiple outputs.
func NewFanOut[T any](count int) *FanOut[T] {
	return &FanOut[T]{
		count: count,
		name:  "fanout",
	}
}

func (f *FanOut[T]) Process(ctx context.Context, in <-chan T) []<-chan T {
	outs := make([]<-chan T, f.count)
	channels := make([]chan T, f.count)

	for i := 0; i < f.count; i++ {
		channels[i] = make(chan T)
		outs[i] = channels[i]
	}

	go func() {
		defer func() {
			for _, ch := range channels {
				close(ch)
			}
		}()

		for item := range in {
			for _, ch := range channels {
				select {
				case ch <- item:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return outs
}
