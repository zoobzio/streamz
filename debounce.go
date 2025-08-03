package streamz

import (
	"context"
	"sync"
	"time"
)

// Debounce emits items only after a quiet period with no new items.
// It's useful for filtering out rapid successive events.
type Debounce[T any] struct {
	name     string
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
//	debounce := streamz.NewDebounce[string](300 * time.Millisecond)
//	debounced := debounce.Process(ctx, searchQueries)
//
//	// Debounce sensor readings
//	debounce := streamz.NewDebounce[SensorData](time.Second)
//	stable := debounce.Process(ctx, readings)
func NewDebounce[T any](duration time.Duration) *Debounce[T] {
	return &Debounce[T]{
		duration: duration,
		name:     "debounce",
	}
}

func (d *Debounce[T]) Process(ctx context.Context, in <-chan T) <-chan T {
	out := make(chan T)

	go func() {
		defer close(out)

		var mu sync.Mutex
		var timer *time.Timer
		var pending T
		var hasPending bool

		for item := range in {
			mu.Lock()
			pending = item
			hasPending = true

			if timer != nil {
				timer.Stop()
			}

			timer = time.AfterFunc(d.duration, func() {
				mu.Lock()
				defer mu.Unlock()

				if hasPending {
					select {
					case out <- pending:
						hasPending = false
					case <-ctx.Done():
					}
				}
			})
			mu.Unlock()
		}

		mu.Lock()
		if timer != nil {
			timer.Stop()
			if hasPending {
				select {
				case out <- pending:
				case <-ctx.Done():
				}
			}
		}
		mu.Unlock()
	}()

	return out
}

func (d *Debounce[T]) Name() string {
	return d.name
}
