package streamz

import (
	"context"
)

// Tap allows observing items without modifying the stream.
type Tap[T any] struct {
	fn   func(T)
	name string
}

// NewTap creates a processor that executes a side effect for each item.
// Items pass through unchanged, making it useful for debugging and monitoring.
//
// When to use:
//   - Debugging and logging
//   - Collecting metrics or statistics
//   - Triggering side effects (notifications, cache updates)
//   - Monitoring data flow without interference
//
// Example:
//
//	// Log all items passing through
//	tap := streamz.NewTap(func(item Event) {
//		log.Printf("Processing event: %+v", item)
//	})
//	logged := tap.Process(ctx, events)
//
//	// Update metrics
//	tap := streamz.NewTap(func(req Request) {
//		metrics.IncrementCounter("requests_processed")
//	})
//	monitored := tap.Process(ctx, requests)
func NewTap[T any](fn func(T)) *Tap[T] {
	return &Tap[T]{
		fn:   fn,
		name: "tap",
	}
}

func (t *Tap[T]) Process(ctx context.Context, in <-chan T) <-chan T {
	out := make(chan T)

	go func() {
		defer close(out)

		for item := range in {
			t.fn(item)

			select {
			case out <- item:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

func (t *Tap[T]) Name() string {
	return t.name
}
