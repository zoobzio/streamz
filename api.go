// Package streamz provides type-safe, composable stream processing primitives
// that work with Go channels, enabling real-time data processing through
// batching, windowing, and other streaming operations.
//
// The core abstraction is the Processor interface which transforms input
// channels to output channels. Processors can be composed to build complex
// streaming pipelines while maintaining type safety throughout.
//
// Basic usage:
//
//	ctx := context.Background()
//	source := make(chan int)
//
//	// Create a simple processing pipeline
//	filter := streamz.NewFilter("positive", func(n int) bool { return n > 0 })
//	batcher := streamz.NewBatcher[int](streamz.BatchConfig{
//		MaxSize:    10,
//		MaxLatency: 100 * time.Millisecond,
//	})
//
//	// Chain processors
//	filtered := filter.Process(ctx, source)
//	batched := batcher.Process(ctx, filtered)
//
//	// Process results
//	for batch := range batched {
//		fmt.Printf("Got batch: %v\n", batch)
//	}
//
// The package provides various processors for common streaming patterns:
//   - Batching and unbatching
//   - Windowing (tumbling, sliding, session)
//   - Buffering with different strategies
//   - Filtering and mapping
//   - Fan-in and fan-out
//   - Rate limiting and flow control
//   - Deduplication
//   - Monitoring and observability
package streamz

import (
	"context"
	"time"
)

// Processor is the core interface for stream processing components.
// It transforms an input channel of type In to an output channel of type Out.
// Processors should:
//   - Close the output channel when the input channel is closed
//   - Respect context cancellation
//   - Handle errors gracefully (typically by skipping problematic items)
//   - Be safe for concurrent use
type Processor[In, Out any] interface {
	// Process transforms the input channel to an output channel.
	// It should close the output channel when processing is complete.
	Process(ctx context.Context, in <-chan In) <-chan Out

	// Name returns a descriptive name for the processor, useful for debugging.
	Name() string
}

// BatchConfig configures batching behavior for the Batcher processor.
type BatchConfig struct {
	// MaxLatency is the maximum time to wait before emitting a partial batch.
	// If set, a batch will be emitted after this duration even if it's not full.
	MaxLatency time.Duration

	// MaxSize is the maximum number of items in a batch.
	// A batch is emitted immediately when it reaches this size.
	MaxSize int
}

// WindowConfig configures windowing behavior for window processors.
type WindowConfig struct {
	// Size is the duration of each window.
	Size time.Duration

	// Slide is the slide interval for sliding windows.
	// If 0 or equal to Size, windows don't overlap (tumbling windows).
	// If less than Size, windows overlap (sliding windows).
	Slide time.Duration

	// MaxCount is the maximum number of items per window.
	// If 0, there's no item count limit.
	MaxCount int
}
