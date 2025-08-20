package streamz

import (
	"context"
	"sync/atomic"
	"time"
)

// StreamStats contains statistics about items flowing through a monitored stream.
// It provides insights into processing rate and throughput for observability.
type StreamStats struct {
	// LastUpdate is the timestamp of this statistics snapshot
	LastUpdate time.Time
	// Count is the number of items processed since the last report
	Count int64
	// Rate is the average items per second since the last report
	Rate float64
}

// Monitor observes items passing through a stream and periodically reports statistics.
// It's a pass-through processor that doesn't modify the stream but provides visibility
// into stream performance and throughput.
type Monitor[T any] struct { //nolint:govet // logical field grouping preferred over memory optimization
	onStats  func(StreamStats)
	lastTime AtomicTime
	name     string
	interval time.Duration
	count    atomic.Int64
	clock    Clock
}

// NewMonitor creates a pass-through processor that observes stream performance.
// It periodically reports statistics about throughput and processing rate without
// modifying the stream data, providing essential observability for production systems.
// Use the fluent API to configure optional behavior like statistics callbacks.
//
// When to use:
//   - Production monitoring and alerting
//   - Performance debugging and optimization
//   - Capacity planning and scaling decisions
//   - SLA tracking and reporting
//   - Identifying bottlenecks in pipelines
//
// Example:
//
//	// Simple monitoring (no stats callback)
//	monitor := streamz.NewMonitor[Event](time.Second, clock.Real)
//
//	// With statistics callback
//	monitor := streamz.NewMonitor[Event](time.Second, clock.Real).
//		OnStats(func(stats streamz.StreamStats) {
//			log.Printf("Processing rate: %.2f items/sec (count: %d)",
//				stats.Rate, stats.Count)
//
//			// Alert on low throughput
//			if stats.Rate < 100 {
//				alertLowThroughput(stats)
//			}
//		})
//
//	monitored := monitor.Process(ctx, events)
//	// Events pass through unchanged while being observed
//
//	// Chain monitoring at different pipeline stages
//	input := sourceMonitor.Process(ctx, source)
//	filtered := filter.Process(ctx, input)
//	output := sinkMonitor.Process(ctx, filtered)
//
// Parameters:
//   - interval: How often to report statistics
//   - clock: Clock interface for time operations
//
// Returns a new Monitor processor with fluent configuration.
func NewMonitor[T any](interval time.Duration, clock Clock) *Monitor[T] {
	m := &Monitor[T]{
		name:     "monitor",
		interval: interval,
		clock:    clock,
		// onStats is nil by default (no-op)
	}
	m.lastTime.Store(clock.Now())
	return m
}

// OnStats sets a callback function to receive periodic statistics.
// If not set, statistics are calculated but not reported.
func (m *Monitor[T]) OnStats(fn func(StreamStats)) *Monitor[T] {
	m.onStats = fn
	return m
}

// WithName sets a custom name for this processor.
// If not set, defaults to "monitor".
func (m *Monitor[T]) WithName(name string) *Monitor[T] {
	m.name = name
	return m
}

func (m *Monitor[T]) Process(ctx context.Context, in <-chan T) <-chan T {
	out := make(chan T)

	go func() {
		defer close(out)

		ticker := m.clock.NewTicker(m.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				m.reportStats()
				return

			case item, ok := <-in:
				if !ok {
					m.reportStats()
					return
				}

				m.count.Add(1)

				select {
				case out <- item:
				case <-ctx.Done():
					return
				}

			case <-ticker.C():
				m.reportStats()
			}
		}
	}()

	return out
}

func (m *Monitor[T]) reportStats() {
	count := m.count.Load()
	now := m.clock.Now()
	lastTime := m.lastTime.Load()
	if lastTime.IsZero() {
		lastTime = now
	}
	duration := now.Sub(lastTime).Seconds()

	rate := float64(count) / duration
	if duration == 0 {
		rate = 0
	}

	stats := StreamStats{
		Count:      count,
		Rate:       rate,
		LastUpdate: now,
	}

	if m.onStats != nil {
		m.onStats(stats)
	}

	m.count.Store(0)
	m.lastTime.Store(now)
}

func (m *Monitor[T]) Name() string {
	return m.name
}
