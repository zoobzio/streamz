package streamz

import (
	"context"
	"time"
)

// SlidingWindow groups items into overlapping time-based windows.
// Unlike tumbling windows, sliding windows can overlap, allowing for
// smooth transitions and rolling calculations over time periods.
type SlidingWindow[T any] struct {
	name  string
	size  time.Duration
	slide time.Duration
}

// NewSlidingWindow creates a processor that groups items into overlapping time windows.
// Each window has a fixed duration (size) and windows are created at regular intervals (slide).
// When slide < size, windows overlap; when slide == size, it behaves like a tumbling window.
//
// When to use:
//   - Computing rolling averages or moving statistics
//   - Smooth trend analysis with overlapping data points
//   - Real-time dashboards with continuous updates
//   - Detecting patterns that might span window boundaries
//   - Gradual transitions in time-series analysis
//
// Example:
//
//	// 5-minute windows sliding every minute
//	window := streamz.NewSlidingWindow[Metric](5*time.Minute, time.Minute)
//
//	windows := window.Process(ctx, metrics)
//	for w := range windows {
//		// Each window contains 5 minutes of data
//		// New window every minute (4 minute overlap)
//		avg := calculateAverage(w.Items)
//		fmt.Printf("Rolling avg [%s-%s]: %.2f\n",
//			w.Start.Format("15:04"),
//			w.End.Format("15:04"),
//			avg)
//	}
//
//	// Hourly windows every 15 minutes for trend detection
//	trending := streamz.NewSlidingWindow[Event](time.Hour, 15*time.Minute)
//	trends := trending.Process(ctx, events)
//
// Parameters:
//   - size: Duration of each window (must be > 0)
//   - slide: How often to create new windows (must be > 0)
//
// Returns a new SlidingWindow processor.
func NewSlidingWindow[T any](size, slide time.Duration) *SlidingWindow[T] {
	return &SlidingWindow[T]{
		size:  size,
		slide: slide,
		name:  "sliding-window",
	}
}

func (w *SlidingWindow[T]) Process(ctx context.Context, in <-chan T) <-chan Window[T] {
	out := make(chan Window[T])

	go func() {
		defer close(out)

		ticker := time.NewTicker(w.slide)
		defer ticker.Stop()

		windows := make(map[time.Time]*Window[T])

		for {
			select {
			case <-ctx.Done():
				return

			case item, ok := <-in:
				if !ok {
					for _, window := range windows {
						out <- *window
					}
					return
				}

				now := time.Now()
				windowStart := now.Truncate(w.slide)

				for start := windowStart; start.After(now.Add(-w.size)); start = start.Add(-w.slide) {
					if _, exists := windows[start]; !exists {
						windows[start] = &Window[T]{
							Items: []T{},
							Start: start,
							End:   start.Add(w.size),
						}
					}
					windows[start].Items = append(windows[start].Items, item)
				}

			case <-ticker.C:
				now := time.Now()
				for start, window := range windows {
					if window.End.Before(now) {
						out <- *window
						delete(windows, start)
					}
				}
			}
		}
	}()

	return out
}

func (w *SlidingWindow[T]) Name() string {
	return w.name
}
