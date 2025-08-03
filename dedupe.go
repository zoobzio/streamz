package streamz

import (
	"context"
	"sync"
	"time"
)

// Dedupe removes duplicate items from a stream based on a key function.
// It maintains a time-based cache of seen keys with automatic expiration.
type Dedupe[T any, K comparable] struct {
	keyFunc func(T) K
	seen    map[K]time.Time
	name    string
	mu      sync.Mutex
	ttl     time.Duration
}

// NewDedupe creates a processor that filters out duplicate items.
// The keyFunc extracts a comparable key from each item for deduplication.
// The ttl (time-to-live) determines how long to remember seen keys.
//
// When to use:
//   - Remove duplicate events or messages
//   - Implement idempotency in stream processing
//   - Filter repeated sensor readings
//   - Prevent duplicate notifications
//
// Example:
//
//	// Deduplicate events by ID with 5-minute memory
//	dedupe := streamz.NewDedupe(func(e Event) string {
//		return e.ID
//	}, 5*time.Minute)
//
//	// Deduplicate integers by value
//	dedupe := streamz.NewDedupe(func(n int) int {
//		return n
//	}, time.Hour)
//
//	unique := dedupe.Process(ctx, events)
func NewDedupe[T any, K comparable](keyFunc func(T) K, ttl time.Duration) *Dedupe[T, K] {
	return &Dedupe[T, K]{
		keyFunc: keyFunc,
		ttl:     ttl,
		name:    "dedupe",
		seen:    make(map[K]time.Time),
	}
}

func (d *Dedupe[T, K]) Process(ctx context.Context, in <-chan T) <-chan T {
	out := make(chan T)

	go func() {
		defer close(out)

		ticker := time.NewTicker(d.ttl / 2)
		defer ticker.Stop()

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
				now := time.Now()

				if !exists || now.Sub(lastSeen) > d.ttl {
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

			case <-ticker.C:
				d.cleanup()
			}
		}
	}()

	return out
}

func (d *Dedupe[T, K]) cleanup() {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	for key, lastSeen := range d.seen {
		if now.Sub(lastSeen) > d.ttl {
			delete(d.seen, key)
		}
	}
}

func (d *Dedupe[T, K]) Name() string {
	return d.name
}
