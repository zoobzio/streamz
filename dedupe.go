package streamz

import (
	"context"
	"sync"
	"time"

	"streamz/clock"
)

// Dedupe removes duplicate items from a stream based on a key function.
// It maintains a time-based cache of seen keys with automatic expiration.
//
//nolint:govet // fieldalignment: struct layout optimized for readability
type Dedupe[T any, K comparable] struct {
	name    string
	clock   clock.Clock
	keyFunc func(T) K
	seen    map[K]time.Time
	mu      sync.Mutex
	ttl     time.Duration
}

// NewDedupe creates a processor that filters out duplicate items.
// The keyFunc extracts a comparable key from each item for deduplication.
// Use the fluent API to configure optional behavior like TTL.
//
// When to use:
//   - Remove duplicate events or messages
//   - Implement idempotency in stream processing
//   - Filter repeated sensor readings
//   - Prevent duplicate notifications
//
// Example:
//
//	// Deduplicate events by ID (infinite memory by default)
//	dedupe := streamz.NewDedupe(func(e Event) string {
//		return e.ID
//	}, clock.Real)
//
//	// With time-based expiration
//	dedupe := streamz.NewDedupe(func(e Event) string {
//		return e.ID
//	}, clock.Real).WithTTL(5*time.Minute)
//
//	// Deduplicate integers by value
//	dedupe := streamz.NewDedupe(func(n int) int {
//		return n
//	}, clock.Real).WithTTL(time.Hour)
//
//	unique := dedupe.Process(ctx, events)
//
// Parameters:
//   - keyFunc: Function to extract comparable key from items
//   - clock: Clock interface for time operations
//
// Returns a new Dedupe processor with fluent configuration.
func NewDedupe[T any, K comparable](keyFunc func(T) K, clock clock.Clock) *Dedupe[T, K] {
	return &Dedupe[T, K]{
		keyFunc: keyFunc,
		ttl:     0, // 0 means infinite (no expiration)
		name:    "dedupe",
		seen:    make(map[K]time.Time),
		clock:   clock,
	}
}

// WithTTL sets the time-to-live for remembered keys.
// If not set or set to 0, keys are remembered forever.
func (d *Dedupe[T, K]) WithTTL(ttl time.Duration) *Dedupe[T, K] {
	d.ttl = ttl
	return d
}

// WithName sets a custom name for this processor.
// If not set, defaults to "dedupe".
func (d *Dedupe[T, K]) WithName(name string) *Dedupe[T, K] {
	d.name = name
	return d
}

func (d *Dedupe[T, K]) Process(ctx context.Context, in <-chan T) <-chan T {
	out := make(chan T)

	go func() {
		defer close(out)

		// Only create ticker if TTL is set
		var ticker clock.Ticker
		var tickerChan <-chan time.Time
		if d.ttl > 0 {
			ticker = d.clock.NewTicker(d.ttl / 2)
			defer ticker.Stop()
			tickerChan = ticker.C()
		}

		for {
			select {
			case <-ctx.Done():
				return

			case item, ok := <-in:
				if !ok {
					return
				}

				key := d.keyFunc(item)

				d.mu.Lock()
				lastSeen, exists := d.seen[key]
				now := d.clock.Now()

				// Check if item should pass through
				shouldPass := !exists
				if exists && d.ttl > 0 {
					shouldPass = now.Sub(lastSeen) > d.ttl
				}

				if shouldPass {
					d.seen[key] = now
					d.mu.Unlock()

					select {
					case out <- item:
					case <-ctx.Done():
						return
					}
				} else {
					d.mu.Unlock()
				}

			case <-tickerChan:
				d.cleanup()
			}
		}
	}()

	return out
}

func (d *Dedupe[T, K]) cleanup() {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := d.clock.Now()
	for key, lastSeen := range d.seen {
		if now.Sub(lastSeen) > d.ttl {
			delete(d.seen, key)
		}
	}
}

func (d *Dedupe[T, K]) Name() string {
	return d.name
}
