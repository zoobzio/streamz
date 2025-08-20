package streamz

import (
	"context"
	"time"
)

// SlidingWindow groups items into overlapping time-based windows.
// Unlike tumbling windows, sliding windows can overlap, allowing for
// smooth transitions and rolling calculations over time periods.
//
//nolint:govet // fieldalignment: struct layout optimized for readability
type SlidingWindow[T any] struct {
	name  string
	clock Clock
	size  time.Duration
	slide time.Duration
}

// NewSlidingWindow creates a processor that groups items into overlapping time windows.
// Each window has a fixed duration (size) and windows are created at regular intervals.
// Use the fluent API to configure optional behavior like slide interval.
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
//	// Tumbling window (no overlap) by default
//	window := streamz.NewSlidingWindow[Metric](5*time.Minute, Real)
//
//	// With overlapping slide interval
//	window := streamz.NewSlidingWindow[Metric](5*time.Minute, Real).
//		WithSlide(time.Minute)
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
//	trending := streamz.NewSlidingWindow[Event](time.Hour, Real).
//		WithSlide(15*time.Minute)
//	trends := trending.Process(ctx, events)
//
// Parameters:
//   - size: Duration of each window (must be > 0)
//   - clock: Clock interface for time operations
//
// Returns a new SlidingWindow processor with fluent configuration.
func NewSlidingWindow[T any](size time.Duration, clock Clock) *SlidingWindow[T] {
	return &SlidingWindow[T]{
		size:  size,
		slide: size, // default to tumbling window behavior
		name:  "sliding-window",
		clock: clock,
	}
}

// WithSlide sets the slide interval for creating new windows.
// If not set, defaults to the window size (tumbling window behavior).
func (w *SlidingWindow[T]) WithSlide(slide time.Duration) *SlidingWindow[T] {
	w.slide = slide
	return w
}

// WithName sets a custom name for this processor.
// If not set, defaults to "sliding-window".
func (w *SlidingWindow[T]) WithName(name string) *SlidingWindow[T] {
	w.name = name
	return w
}

func (w *SlidingWindow[T]) Process(ctx context.Context, in <-chan T) <-chan Window[T] {
	out := make(chan Window[T])

	go func() {
		defer close(out)

		ticker := w.clock.NewTicker(w.slide)
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

				now := w.clock.Now()
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

			case <-ticker.C():
				now := w.clock.Now()
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
