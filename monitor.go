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
type Monitor[T any] struct {
	onStats  func(StreamStats)
	lastTime atomic.Value
	name     string
	interval time.Duration
	count    atomic.Int64
}

// NewMonitor creates a pass-through processor that observes stream performance.
// It periodically reports statistics about throughput and processing rate without
// modifying the stream data, providing essential observability for production systems.
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
//	// Monitor throughput every second
//	monitor := streamz.NewMonitor(time.Second, func(stats streamz.StreamStats) {
//		log.Printf("Processing rate: %.2f items/sec (count: %d)",
//			stats.Rate, stats.Count)
//
//		// Alert on low throughput
//		if stats.Rate < 100 {
//			alertLowThroughput(stats)
//		}
//	})
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
//   - onStats: Callback function invoked with statistics at each interval
//
// Returns a new Monitor processor that observes stream performance.
func NewMonitor[T any](interval time.Duration, onStats func(StreamStats)) *Monitor[T] {
	m := &Monitor[T]{
		name:     "monitor",
		interval: interval,
		onStats:  onStats,
	}
	m.lastTime.Store(time.Now())
	return m
}

func (m *Monitor[T]) Process(ctx context.Context, in <-chan T) <-chan T {
	out := make(chan T)

	go func() {
		defer close(out)

		ticker := time.NewTicker(m.interval)
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

			case <-ticker.C:
				m.reportStats()
			}
		}
	}()

	return out
}

func (m *Monitor[T]) reportStats() {
	count := m.count.Load()
	now := time.Now()
	lastTimeVal := m.lastTime.Load()
	lastTime, ok := lastTimeVal.(time.Time)
	if !ok {
		lastTime = time.Now()
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
