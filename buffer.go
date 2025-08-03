package streamz

import (
	"context"
)

// Buffer adds buffering capacity to a stream by creating an output channel with a buffer.
// This helps decouple producers and consumers, allowing producers to continue sending
// items even when consumers are temporarily slower.
type Buffer[T any] struct {
	name string
	size int
}

// NewBuffer creates a processor with a simple buffered output channel.
// This provides basic decoupling between producers and consumers, allowing
// the producer to continue sending items even when the consumer is temporarily slower.
//
// When to use:
//   - Smoothing out temporary processing speed mismatches
//   - Decoupling producer and consumer goroutines
//   - Handling brief bursts of high throughput
//   - Providing breathing room for downstream processors
//   - Simple async boundaries in pipelines
//
// Example:
//
//	// Add a buffer of 1000 items
//	buffer := streamz.NewBuffer[Message](1000)
//
//	// Producer can send up to 1000 items without blocking
//	buffered := buffer.Process(ctx, messages)
//	for msg := range buffered {
//		// Slower processing won't block producer
//		expensiveOperation(msg)
//	}
//
//	// Chain with other processors for burst handling
//	buffer := streamz.NewBuffer[Event](5000)
//	throttle := streamz.NewThrottle[Event](100) // 100/sec
//
//	buffered := buffer.Process(ctx, events)
//	limited := throttle.Process(ctx, buffered)
//
// Parameters:
//   - size: Buffer capacity (0 for unbuffered, >0 for buffered channel)
//
// Returns a new Buffer processor with the specified capacity.
func NewBuffer[T any](size int) *Buffer[T] {
	return &Buffer[T]{
		size: size,
		name: "buffer",
	}
}

func (b *Buffer[T]) Process(ctx context.Context, in <-chan T) <-chan T {
	out := make(chan T, b.size)

	go func() {
		defer close(out)

		for item := range in {
			select {
			case out <- item:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

func (b *Buffer[T]) Name() string {
	return b.name
}
