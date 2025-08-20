package streamz

import (
	"context"
	"sync"
	"time"
)

// Debounce emits items only after a quiet period with no new items.
// It's useful for filtering out rapid successive events.
//
//nolint:govet // fieldalignment: struct layout optimized for readability
type Debounce[T any] struct {
	name     string
	clock    Clock
	duration time.Duration
}

// NewDebounce creates a processor that delays and coalesces rapid events.
// Only the last item in a rapid sequence is emitted after the specified duration of inactivity.
//
// When to use:
//   - User input handling (e.g., search-as-you-type)
//   - Sensor readings that fluctuate rapidly
//   - File system change notifications
//   - Preventing excessive API calls from UI events
//
// Example:
//
//	// Debounce search queries - only search after 300ms of no typing
//	debounce := streamz.NewDebounce[string](300 * time.Millisecond, Real)
//	debounced := debounce.Process(ctx, searchQueries)
//
//	// Debounce sensor readings
//	debounce := streamz.NewDebounce[SensorData](time.Second, Real)
//	stable := debounce.Process(ctx, readings)
//
// Parameters:
//   - duration: The quiet period before emitting an item
//   - clock: Clock interface for time operations
func NewDebounce[T any](duration time.Duration, clock Clock) *Debounce[T] {
	return &Debounce[T]{
		duration: duration,
		name:     "debounce",
		clock:    clock,
	}
}

func (d *Debounce[T]) Process(ctx context.Context, in <-chan T) <-chan T {
	out := make(chan T)

	go func() {
		defer close(out)

		var mu sync.Mutex
		var timer Timer
		var pending T
		var hasPending bool
		var closed bool

		for item := range in {
			mu.Lock()
			pending = item
			hasPending = true

			if timer != nil {
				timer.Stop()
			}

			timer = d.clock.AfterFunc(d.duration, func() {
				mu.Lock()
				defer mu.Unlock()

				if closed {
					return // Don't send if we're already closed
				}

				if hasPending {
					itemToSend := pending
					hasPending = false
					mu.Unlock()

					select {
					case out <- itemToSend:
					case <-ctx.Done():
					}
					mu.Lock() // Re-acquire for defer
				}
			})
			mu.Unlock()
		}

		// Channel closed, flush any pending item
		mu.Lock()
		closed = true
		if timer != nil {
			timer.Stop()
			if hasPending {
				finalItem := pending
				mu.Unlock()
				select {
				case out <- finalItem:
				case <-ctx.Done():
				}
			} else {
				mu.Unlock()
			}
		} else {
			mu.Unlock()
		}
	}()

	return out
}

func (d *Debounce[T]) Name() string {
	return d.name
}
