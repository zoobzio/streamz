// Package streamz provides type-safe, composable stream processing primitives
// that work with Go channels, enabling real-time data processing through
// batching, windowing, and other streaming operations.
//
// The core abstraction uses the Result[T] pattern which unifies success and
// error cases into a single channel. This eliminates dual-channel complexity
// while providing explicit error handling and better monitoring capabilities.
//
// Basic usage:
//
//	ctx := context.Background()
//	source := make(chan Result[int])
//
//	// Create a simple processing pipeline
//	fanin := streamz.NewFanIn[int]()
//
//	// Process returns a single Result[T] channel
//	results := fanin.Process(ctx, source)
//
//	// Handle both success and error cases from single channel
//	for result := range results {
//		if result.IsError() {
//			log.Printf("Processing error: %v", result.Error())
//		} else {
//			fmt.Printf("Got item: %v\n", result.Value())
//		}
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
	"time"
)

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
