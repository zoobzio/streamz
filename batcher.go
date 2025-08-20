package streamz

import (
	"context"
)

// Batcher collects items from a stream and groups them into batches based on size or time constraints.
// It emits a batch when either the maximum size is reached or the maximum latency expires,
// whichever comes first. This is useful for optimizing downstream operations that work more
// efficiently with groups of items rather than individual items.
//
//nolint:govet // fieldalignment: struct layout optimized for readability
type Batcher[T any] struct {
	config BatchConfig
	name   string
	clock  Clock
}

// NewBatcher creates a processor that intelligently groups items into batches.
// Batches are emitted when either the size limit is reached OR the time limit expires,
// whichever comes first. This dual-trigger approach balances throughput with latency.
//
// When to use:
//   - Optimizing database writes with bulk operations
//   - Reducing API calls by batching requests
//   - Implementing micro-batching for stream processing
//   - Buffering events for periodic processing
//   - Cost optimization through batch operations
//
// Example:
//
//	// Batch up to 1000 items or 5 seconds, whichever comes first
//	batcher := streamz.NewBatcher[Event](streamz.BatchConfig{
//		MaxSize:    1000,
//		MaxLatency: 5 * time.Second,
//	}, Real)
//
//	batches := batcher.Process(ctx, events)
//	for batch := range batches {
//		// Process batch of up to 1000 items
//		// Never waits more than 5 seconds
//		bulkInsert(batch)
//	}
//
//	// Optimize API calls with smart batching
//	apiBatcher := streamz.NewBatcher[Request](streamz.BatchConfig{
//		MaxSize:    100,  // API limit
//		MaxLatency: 100 * time.Millisecond, // Max acceptable delay
//	}, Real)
//
// Parameters:
//   - config: Batch configuration with size and latency constraints
//   - clock: Clock interface for time operations
//
// Returns a new Batcher processor that groups items efficiently.
func NewBatcher[T any](config BatchConfig, clock Clock) *Batcher[T] {
	return &Batcher[T]{
		config: config,
		name:   "batcher",
		clock:  clock,
	}
}

func (b *Batcher[T]) Process(ctx context.Context, in <-chan T) <-chan []T {
	out := make(chan []T)

	go func() {
		defer close(out)

		batch := make([]T, 0, b.config.MaxSize)
		timer := b.clock.NewTimer(b.config.MaxLatency)
		timer.Stop()

		for {
			select {
			case <-ctx.Done():
				if len(batch) > 0 {
					select {
					case out <- batch:
					case <-ctx.Done():
					}
				}
				return

			case item, ok := <-in:
				if !ok {
					if len(batch) > 0 {
						out <- batch
					}
					return
				}

				batch = append(batch, item)

				if len(batch) == 1 {
					timer.Reset(b.config.MaxLatency)
				}

				if len(batch) >= b.config.MaxSize {
					timer.Stop()
					out <- batch
					batch = make([]T, 0, b.config.MaxSize)
				}

			case <-timer.C():
				if len(batch) > 0 {
					out <- batch
					batch = make([]T, 0, b.config.MaxSize)
				}
			}
		}
	}()

	return out
}

func (b *Batcher[T]) Name() string {
	return b.name
}
