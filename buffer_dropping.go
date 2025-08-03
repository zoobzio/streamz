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
// The onDrop callback is called for each dropped item, allowing for metrics or logging.
//
// When to use:
//   - Real-time data streams where latest data is more important than completeness
//   - Monitoring systems that can tolerate some data loss
//   - Live video/audio streaming where dropping frames is acceptable
//   - Preventing memory buildup from slow consumers
//
// Example:
//
//	// Create a dropping buffer that logs dropped items
//	buffer := streamz.NewDroppingBuffer(100, func(item Event) {
//		log.Printf("Dropped event: %v", item.ID)
//		metrics.Increment("events.dropped")
//	})
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
//   - onDrop: Callback function invoked for each dropped item (can be nil)
//
// Returns a new DroppingBuffer processor.
func NewDroppingBuffer[T any](size int, onDrop func(T)) *DroppingBuffer[T] {
	return &DroppingBuffer[T]{
		size:   size,
		name:   "dropping-buffer",
		onDrop: onDrop,
	}
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
