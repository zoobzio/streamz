package streamz

import (
	"context"
	"time"

	"streamz/clock"
)

// Throttle limits the rate of items passing through the stream.
//
//nolint:govet // fieldalignment: struct layout optimized for readability
type Throttle[T any] struct {
	name  string
	clock clock.Clock
	rps   float64
}

// NewThrottle creates a processor that rate-limits items.
// The rps parameter specifies the maximum requests (items) per second.
//
// When to use:
//   - Prevent overwhelming downstream services
//   - Comply with API rate limits
//   - Control resource consumption
//   - Smooth out traffic spikes
//
// Example:
//
//	// Limit API calls to 10 per second
//	throttle := streamz.NewThrottle[APIRequest](10.0, clock.Real)
//	throttled := throttle.Process(ctx, requests)
//
//	// Process at most 100 items per second
//	throttle := streamz.NewThrottle[Event](100.0, clock.Real)
//	controlled := throttle.Process(ctx, events)
//
// Parameters:
//   - rps: Maximum requests (items) per second
//   - clock: Clock interface for time operations
func NewThrottle[T any](rps float64, clock clock.Clock) *Throttle[T] {
	return &Throttle[T]{
		rps:   rps,
		name:  "throttle",
		clock: clock,
	}
}

func (t *Throttle[T]) Process(ctx context.Context, in <-chan T) <-chan T {
	out := make(chan T)

	go func() {
		defer close(out)

		interval := time.Second / time.Duration(t.rps)
		ticker := t.clock.NewTicker(interval)
		defer ticker.Stop()

		for item := range in {
			select {
			case <-ticker.C():
				select {
				case out <- item:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

func (t *Throttle[T]) Name() string {
	return t.name
}
