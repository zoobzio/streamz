package streamz

import (
	"context"
	"sync"
	"time"
)

// Throttle limits the rate of items passing through the stream using leading edge behavior.
// It emits the first item immediately and then ignores subsequent items for a cooldown period.
// Errors are passed through immediately without throttling.
//
// Concurrent Behavior:
// Multiple goroutines may call Process() on the same Throttle instance.
// The throttling state (lastEmit) is shared across all Process() calls.
//
//nolint:govet // fieldalignment: struct layout optimized for readability
type Throttle[T any] struct {
	name     string
	clock    Clock
	duration time.Duration
	lastEmit time.Time  // Track when we last emitted an item
	mutex    sync.Mutex // Protect lastEmit access
}

// NewThrottle creates a processor that implements leading edge throttling.
// The first item is emitted immediately, then subsequent items are ignored
// until the cooldown period expires. Errors are passed through immediately.
//
// When to use:
//   - Prevent overwhelming downstream services with rapid requests
//   - Implement "first action wins" behavior for rapid user interactions
//   - Rate limiting API calls with immediate first response
//   - Controlling burst traffic patterns
//
// Example:
//
//	// Throttle button clicks - only first click processed per 500ms
//	throttle := streamz.NewThrottle[ClickEvent](500 * time.Millisecond, streamz.RealClock)
//	processed := throttle.Process(ctx, clicks)
//
//	// Throttle API requests - first request immediate, others wait
//	throttle := streamz.NewThrottle[APIRequest](time.Second, streamz.RealClock)
//	limited := throttle.Process(ctx, requests)
//
// Parameters:
//   - duration: The cooldown period during which subsequent items are ignored.
//     If duration is 0, all items pass through without throttling.
//   - clock: Clock interface for time operations
func NewThrottle[T any](duration time.Duration, clock Clock) *Throttle[T] {
	return &Throttle[T]{
		duration: duration,
		name:     "throttle",
		clock:    clock,
		// lastEmit zero value means first item always passes
	}
}

// Process throttles the input stream using leading edge behavior.
// The first item is emitted immediately, then subsequent items are ignored
// until the cooldown period expires. Errors are passed through immediately.
// Uses timestamp comparison instead of timer goroutines for race-free operation.
func (th *Throttle[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan Result[T] {
	out := make(chan Result[T])

	go func() {
		defer close(out)

		for {
			select {
			case result, ok := <-in:
				if !ok {
					return
				}

				// Errors pass through immediately without throttling
				if result.IsError() {
					select {
					case out <- result:
					case <-ctx.Done():
						return
					}
					continue
				}

				// For success values, check if enough time has elapsed
				// Zero duration means no throttling - everything passes
				if th.duration == 0 {
					select {
					case out <- result:
					case <-ctx.Done():
						return
					}
					continue
				}

				// Check elapsed time since last emit
				th.mutex.Lock()
				now := th.clock.Now()
				elapsed := now.Sub(th.lastEmit)

				if elapsed >= th.duration {
					// Cooling period has expired or first emit
					th.lastEmit = now
					th.mutex.Unlock()

					select {
					case out <- result:
					case <-ctx.Done():
						return
					}
				} else {
					// Still cooling - drop the item
					th.mutex.Unlock()
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

// Name returns the processor name for debugging and monitoring.
func (th *Throttle[T]) Name() string {
	return th.name
}
