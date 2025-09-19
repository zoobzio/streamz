package streamz

import (
	"context"
	"time"
)

// Debounce emits items only after a quiet period with no new items.
// It's useful for filtering out rapid successive events.
// Errors are passed through immediately without debouncing.
//
//nolint:govet // fieldalignment: struct layout optimized for readability
type Debounce[T any] struct {
	name     string
	clock    Clock
	duration time.Duration
}

// NewDebounce creates a processor that delays and coalesces rapid events.
// Only the last successful item in a rapid sequence is emitted after the specified
// duration of inactivity. Errors are passed through immediately.
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
//	debounce := streamz.NewDebounce[string](300 * time.Millisecond, streamz.RealClock)
//	debounced := debounce.Process(ctx, searchQueries)
//
//	// Debounce sensor readings
//	debounce := streamz.NewDebounce[SensorData](time.Second, streamz.RealClock)
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

// Process debounces the input stream, emitting only the last item after a period of quiet.
// Errors are passed through immediately without debouncing.
// The last successful item is emitted when the input channel closes.
func (d *Debounce[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan Result[T] {
	out := make(chan Result[T])

	go func() {
		defer close(out)

		var pending Result[T]
		var hasPending bool
		var timer Timer
		var timerC <-chan time.Time

		for {
			// Phase 1: Check timer first with higher priority
			if timerC != nil {
				select {
				case <-timerC:
					// Timer expired, send pending value
					if hasPending {
						select {
						case out <- pending:
							hasPending = false
						case <-ctx.Done():
							return
						}
					}
					// Clear timer references
					timer = nil
					timerC = nil
					continue // Check for more timer events
				default:
					// Timer not ready - proceed to input
				}
			}

			// Phase 2: Process input/context
			select {
			case result, ok := <-in:
				if !ok {
					// Input closed, flush pending if exists
					if timer != nil {
						timer.Stop()
					}
					if hasPending {
						select {
						case out <- pending:
						case <-ctx.Done():
						}
					}
					return
				}

				// Errors pass through immediately without debouncing
				if result.IsError() {
					select {
					case out <- result:
					case <-ctx.Done():
						return
					}
					continue
				}

				// Update pending and create new timer for successful values
				pending = result
				hasPending = true

				// Stop old timer if exists
				if timer != nil {
					timer.Stop()
				}

				// Create new timer (workaround for FakeClock Reset bug)
				timer = d.clock.NewTimer(d.duration)
				timerC = timer.C()

			case <-timerC:
				// Timer fired during input wait
				if hasPending {
					select {
					case out <- pending:
						hasPending = false
					case <-ctx.Done():
						return
					}
				}
				// Clear timer references
				timer = nil
				timerC = nil

			case <-ctx.Done():
				if timer != nil {
					timer.Stop()
				}
				return
			}
		}
	}()

	return out
}

// Name returns the processor name for debugging and monitoring.
func (d *Debounce[T]) Name() string {
	return d.name
}
