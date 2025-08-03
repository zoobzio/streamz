package streamz

import (
	"context"
	"time"
)

// Throttle limits the rate of items passing through the stream.
type Throttle[T any] struct {
	name string
	rps  float64
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
//	throttle := streamz.NewThrottle[APIRequest](10.0)
//	throttled := throttle.Process(ctx, requests)
//
//	// Process at most 100 items per second
//	throttle := streamz.NewThrottle[Event](100.0)
//	controlled := throttle.Process(ctx, events)
func NewThrottle[T any](rps float64) *Throttle[T] {
	return &Throttle[T]{
		rps:  rps,
		name: "throttle",
	}
}

func (t *Throttle[T]) Process(ctx context.Context, in <-chan T) <-chan T {
	out := make(chan T)

	go func() {
		defer close(out)

		interval := time.Second / time.Duration(t.rps)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for item := range in {
			select {
			case <-ticker.C:
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
