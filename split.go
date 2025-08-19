package streamz

import (
	"context"
	"sync/atomic"
)

// SplitOutput provides access to the two output channels from a split operation.
// The True channel receives items for which the predicate returns true,
// while the False channel receives items for which the predicate returns false.
type SplitOutput[T any] struct { //nolint:govet // logical field grouping preferred over memory optimization
	// True receives items where predicate returns true
	True <-chan T

	// False receives items where predicate returns false
	False <-chan T
}

// Split divides a stream into exactly two outputs based on a predicate function.
// Items for which the predicate returns true go to the True output, while
// items for which it returns false go to the False output.
//
// Split is simpler than Router when you need binary classification and is
// more explicit about having exactly two outputs. It's ideal for:
//   - Valid/invalid separation
//   - Pass/fail classification
//   - True/false filtering with both outputs needed
//   - Binary decision trees
//
// Key features:
//   - Exactly two outputs (no more, no less)
//   - Non-blocking sends to both outputs
//   - Concurrent processing of both streams
//   - Statistics tracking for both outputs
//   - Graceful shutdown handling
//
// Example:
//
//	// Split orders into high/low value
//	splitter := streamz.NewSplit[Order](func(o Order) bool {
//	    return o.Total > 1000
//	})
//
//	outputs := splitter.Process(ctx, orders)
//
//	// Process high value orders with priority
//	go processHighValue(outputs.True)
//
//	// Process normal orders
//	go processNormal(outputs.False)
//
// Performance characteristics:
//   - O(1) per item processing
//   - Non-blocking sends (when using buffered channels)
//   - Minimal overhead for classification
type Split[T any] struct { //nolint:govet // logical field grouping preferred over memory optimization
	predicate  func(T) bool
	bufferSize int
	name       string

	// Statistics
	trueCount  atomic.Int64
	falseCount atomic.Int64
	totalCount atomic.Int64
}

// NewSplit creates a processor that splits items into two streams based on a predicate.
// Items are sent to the appropriate output channel based on whether the predicate
// returns true or false.
//
// The predicate function is called once for each item to determine routing.
// Both output channels should be consumed concurrently to avoid blocking.
//
// Default configuration:
//   - Buffer size: 0 (unbuffered channels)
//   - Name: "split"
//
// Parameters:
//   - predicate: Function that returns true/false for each item
//
// Returns: A new Split processor with fluent configuration methods.
func NewSplit[T any](predicate func(T) bool) *Split[T] {
	return &Split[T]{
		predicate:  predicate,
		bufferSize: 0,
		name:       "split",
	}
}

// WithBufferSize sets the buffer size for both output channels.
// A larger buffer can help prevent blocking when consumers process at different speeds.
func (s *Split[T]) WithBufferSize(size int) *Split[T] {
	if size < 0 {
		size = 0
	}
	s.bufferSize = size
	return s
}

// WithName sets a custom name for this processor instance.
func (s *Split[T]) WithName(name string) *Split[T] {
	s.name = name
	return s
}

// Process splits the input stream into two outputs based on the predicate.
// Both output channels must be consumed to avoid blocking.
func (s *Split[T]) Process(ctx context.Context, in <-chan T) SplitOutput[T] {
	trueOut := make(chan T, s.bufferSize)
	falseOut := make(chan T, s.bufferSize)

	go func() {
		defer close(trueOut)
		defer close(falseOut)

		for {
			select {
			case item, ok := <-in:
				if !ok {
					return
				}

				s.totalCount.Add(1)

				// Evaluate predicate
				if s.predicate(item) {
					s.trueCount.Add(1)
					select {
					case trueOut <- item:
					case <-ctx.Done():
						return
					}
				} else {
					s.falseCount.Add(1)
					select {
					case falseOut <- item:
					case <-ctx.Done():
						return
					}
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	return SplitOutput[T]{
		True:  trueOut,
		False: falseOut,
	}
}

// GetStats returns statistics about the split distribution.
func (s *Split[T]) GetStats() SplitStats {
	total := s.totalCount.Load()
	trueCount := s.trueCount.Load()
	falseCount := s.falseCount.Load()

	var trueRatio, falseRatio float64
	if total > 0 {
		trueRatio = float64(trueCount) / float64(total)
		falseRatio = float64(falseCount) / float64(total)
	}

	return SplitStats{
		TotalItems: total,
		TrueCount:  trueCount,
		FalseCount: falseCount,
		TrueRatio:  trueRatio,
		FalseRatio: falseRatio,
	}
}

// Name returns the processor name.
func (s *Split[T]) Name() string {
	return s.name
}

// SplitStats contains statistics about split distribution.
type SplitStats struct { //nolint:govet // logical field grouping preferred over memory optimization
	TotalItems int64   // Total items processed
	TrueCount  int64   // Items sent to True output
	FalseCount int64   // Items sent to False output
	TrueRatio  float64 // Percentage sent to True (0.0-1.0)
	FalseRatio float64 // Percentage sent to False (0.0-1.0)
}
