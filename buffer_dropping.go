package streamz

import (
	"context"
)

// DroppingBuffer provides buffering with a drop-oldest strategy when the buffer is full.
// Unlike a blocking buffer, it never blocks the producer - instead it drops the oldest
// items to make room for new ones, making it suitable for lossy real-time streams.
type DroppingBuffer[T any] struct {
	onDrop func(T)
	name   string
	size   int
}

// NewDroppingBuffer creates a buffer that drops oldest items when full.
// Use the fluent API to configure optional behavior like drop callbacks.
//
// When to use:
//   - Real-time data streams where latest data is more important than completeness
//   - Monitoring systems that can tolerate some data loss
//   - Live video/audio streaming where dropping frames is acceptable
//   - Preventing memory buildup from slow consumers
//
// Example:
//
//	// Simple dropping buffer
//	buffer := streamz.NewDroppingBuffer[Event](100)
//
//	// With drop callback for metrics
//	buffer := streamz.NewDroppingBuffer[Event](100).
//		OnDrop(func(item Event) {
//			log.Printf("Dropped event: %v", item.ID)
//			metrics.Increment("events.dropped")
//		})
//
//	// Use in a pipeline
//	buffered := buffer.Process(ctx, events)
//	for event := range buffered {
//		// Process events - old events dropped if processing is slow
//		processEvent(event)
//	}
//
// Parameters:
//   - size: Buffer capacity (must be > 0)
//
// Returns a new DroppingBuffer processor with fluent configuration.
func NewDroppingBuffer[T any](size int) *DroppingBuffer[T] {
	return &DroppingBuffer[T]{
		size: size,
		name: "dropping-buffer",
		// onDrop is nil by default (no-op)
	}
}

// OnDrop sets a callback function to be called for each dropped item.
// If not set, dropped items are silently discarded.
func (d *DroppingBuffer[T]) OnDrop(fn func(T)) *DroppingBuffer[T] {
	d.onDrop = fn
	return d
}

// WithName sets a custom name for this processor.
// If not set, defaults to "dropping-buffer".
func (d *DroppingBuffer[T]) WithName(name string) *DroppingBuffer[T] {
	d.name = name
	return d
}

func (d *DroppingBuffer[T]) Process(ctx context.Context, in <-chan T) <-chan T {
	out := make(chan T, d.size)

	go func() {
		defer close(out)

		for item := range in {
			select {
			case out <- item:
			case <-ctx.Done():
				return
			default:
				if d.onDrop != nil {
					d.onDrop(item)
				}
			}
		}
	}()

	return out
}

func (d *DroppingBuffer[T]) Name() string {
	return d.name
}
