package streamz

import (
	"context"
)

// SlidingBuffer maintains a fixed-size window of the most recent items.
// When the buffer is full, it evicts the oldest item to make room for new ones,
// implementing a FIFO eviction policy with guaranteed delivery of the latest N items.
type SlidingBuffer[T any] struct {
	name string
	size int
}

// NewSlidingBuffer creates a buffer that maintains a sliding window of recent items.
// Unlike DroppingBuffer, this ensures all items pass through - it just limits how many
// can be buffered at once, providing backpressure when the consumer is slower.
//
// When to use:
//   - Maintaining a cache of recent items
//   - Smoothing out bursts while preserving all data
//   - Implementing moving averages or recent history
//   - Rate adaptation between fast producers and slow consumers
//
// Example:
//
//	// Keep last 1000 items buffered
//	buffer := streamz.NewSlidingBuffer[Metric](1000)
//
//	// Use for burst absorption
//	buffered := buffer.Process(ctx, metrics)
//	for metric := range buffered {
//		// Process metrics - buffer smooths out bursts
//		storeMetric(metric)
//	}
//
//	// Combine with other processors
//	recent := streamz.NewSlidingBuffer[Event](100)
//	monitor := streamz.NewMonitor[Event](time.Second, logStats)
//
//	buffered := recent.Process(ctx, events)
//	monitored := monitor.Process(ctx, buffered)
//
// Parameters:
//   - size: Maximum buffer size (must be > 0)
//
// Returns a new SlidingBuffer processor.
func NewSlidingBuffer[T any](size int) *SlidingBuffer[T] {
	return &SlidingBuffer[T]{
		size: size,
		name: "sliding-buffer",
	}
}

func (s *SlidingBuffer[T]) Process(ctx context.Context, in <-chan T) <-chan T {
	out := make(chan T, s.size)

	go func() {
		defer close(out)

		for item := range in {
			select {
			case out <- item:
			case <-ctx.Done():
				return
			default:
				select {
				case <-out:
					out <- item
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out
}

func (s *SlidingBuffer[T]) Name() string {
	return s.name
}
