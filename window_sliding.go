package streamz

import (
	"context"
	"time"
)

// SlidingWindow groups Results into overlapping time-based windows.
// Unlike tumbling windows, sliding windows can overlap, allowing for
// smooth transitions and rolling calculations over time periods.
//
// This version processes Result[T] streams, capturing both successful
// values and errors within overlapping time windows for comprehensive
// monitoring and analysis.
//
// Key characteristics:
//   - Overlapping: Items can belong to multiple windows
//   - Configurable slide: Control overlap with slide interval
//   - Smooth aggregations: Better for trend detection
//
// Performance characteristics:
//   - Window emission latency: At window.End time (size duration after window.Start)
//   - Memory usage: O(active_windows × items_per_window) where active_windows = size/slide
//   - Processing overhead: O(active_windows) map operations per item
//   - Goroutine usage: 1 goroutine per processor instance
//   - Overlap factor: size/slide determines number of concurrent windows
//
//nolint:govet // fieldalignment: struct layout optimized for readability
type SlidingWindow[T any] struct {
	name  string
	clock Clock
	size  time.Duration
	slide time.Duration
}

// NewSlidingWindow creates a processor that groups Results into overlapping time windows.
// Each window has a fixed duration (size) and windows are created at regular intervals (slide).
// Use the fluent API to configure optional behavior like slide interval.
//
// When to use:
//   - Computing rolling averages with error rates over time
//   - Smooth trend analysis with overlapping data points and failure tracking
//   - Real-time dashboards with continuous updates and health monitoring
//   - Detecting patterns that might span window boundaries
//   - Gradual transitions in time-series analysis with error correlation
//
// Example:
//
//	// Tumbling window behavior (no overlap) with Result[T]
//	window := streamz.NewSlidingWindow[Metric](5*time.Minute, streamz.RealClock)
//
//	// Overlapping windows: 5-minute window, new window every minute
//	window := streamz.NewSlidingWindow[Metric](5*time.Minute, streamz.RealClock).
//		WithSlide(time.Minute)
//
//	results := window.Process(ctx, metricResults)
//	for result := range results {
//		// Each result has window metadata attached
//		if meta, err := streamz.GetWindowMetadata(result); err == nil {
//			fmt.Printf("Metric in sliding window [%s-%s]: %v\n",
//				meta.Start.Format("15:04"),
//				meta.End.Format("15:04"),
//				result.Value())
//		}
//	}
//
//	// Collect into windows for analysis when needed
//	collector := streamz.NewWindowCollector[Metric]()
//	collections := collector.Process(ctx, results)
//	for collection := range collections {
//		values := collection.Values()   // Successful metrics only
//		errors := collection.Errors()   // Failed metrics only
//		successRate := float64(collection.SuccessCount()) / float64(collection.Count()) * 100
//
//		if successRate < 90 {
//			alert.Send("Success rate below threshold", collection)
//		}
//	}
//
// Parameters:
//   - size: Duration of each window (must be > 0)
//   - clock: Clock interface for time operations (use RealClock for production)
//
// Returns a new SlidingWindow processor with fluent configuration.
//
// Performance notes:
//   - Memory scales with overlap factor (size/slide)
//   - CPU overhead proportional to number of active windows
//   - Best for smooth aggregations and trend detection
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
//
// Parameters:
//   - slide: Time interval between window starts. If smaller than size, windows overlap.
//
// Returns the SlidingWindow for method chaining.
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

// Process groups Results into overlapping time windows, emitting individual Results with window metadata.
// Results can belong to multiple windows if they overlap. Both successful values
// and errors are captured with their window context, enabling comprehensive
// analysis of patterns across overlapping time periods.
//
// Window behavior:
// - Each Result gets window metadata attached (start, end, type, size, slide)
// - Results are assigned to all overlapping windows they belong to
// - Results are emitted when their windows expire (current time > window end)
// - All Results (success and errors) within the window timeframe are included
//
// Performance and resource usage:
//   - Memory usage scales with overlap: up to size/slide concurrent windows
//   - Each item is duplicated in all overlapping windows (by reference)
//   - Emission checked at slide intervals via ticker
//   - Special optimization: When slide == size, uses efficient tumbling mode
//   - Thread-safe: Single goroutine architecture prevents races
//
// Trade-offs:
//   - Higher memory usage than tumbling windows due to overlap
//   - More CPU per item (checking multiple windows)
//   - Smoother aggregations and trend detection
//   - Better for detecting patterns spanning window boundaries
func (w *SlidingWindow[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan Result[T] {
	out := make(chan Result[T])

	go func() {
		defer close(out)

		// Special case: if slide equals size, behave like tumbling window
		if w.slide == w.size {
			w.processTumblingMode(ctx, in, out)
			return
		}

		ticker := w.clock.NewTicker(w.slide)
		defer ticker.Stop()

		// Track active windows and their results
		windows := make(map[time.Time]*windowState[T])
		var firstItemTime time.Time
		var firstItemReceived bool

		for {
			select {
			case <-ctx.Done():
				// Emit all remaining windows
				w.emitAllWindows(ctx, out, windows)
				return

			case result, ok := <-in:
				if !ok {
					// Input closed, emit all windows
					w.emitAllWindows(ctx, out, windows)
					return
				}

				now := w.clock.Now()
				if !firstItemReceived {
					firstItemTime = now
					firstItemReceived = true
				}

				// Add to existing windows that should contain this item
				for start, window := range windows {
					if !start.After(now) && now.Before(window.meta.End) {
						window.results = append(window.results, result)
					}
				}

				// Create new window at current slide boundary if needed
				currentWindowStart := now.Truncate(w.slide)
				if _, exists := windows[currentWindowStart]; !exists && !currentWindowStart.Before(firstItemTime) {
					slidePtr := &w.slide
					windows[currentWindowStart] = &windowState[T]{
						meta: WindowMetadata{
							Start: currentWindowStart,
							End:   currentWindowStart.Add(w.size),
							Type:  "sliding",
							Size:  w.size,
							Slide: slidePtr,
						},
						results: []Result[T]{result},
					}
				}

			case <-ticker.C():
				now := w.clock.Now()
				// Emit expired windows
				expiredStarts := make([]time.Time, 0)

				for start, window := range windows {
					if !window.meta.End.After(now) {
						w.emitWindowResults(ctx, out, window.results, window.meta)
						expiredStarts = append(expiredStarts, start)
					}
				}

				// Clean up expired windows
				for _, start := range expiredStarts {
					delete(windows, start)
				}
			}
		}
	}()

	return out
}

// processTumblingMode handles the special case when slide == size (tumbling window behavior).
func (w *SlidingWindow[T]) processTumblingMode(ctx context.Context, in <-chan Result[T], out chan<- Result[T]) {
	ticker := w.clock.NewTicker(w.size)
	defer ticker.Stop()

	now := w.clock.Now()
	currentWindow := WindowMetadata{
		Start: now,
		End:   now.Add(w.size),
		Type:  "sliding",
		Size:  w.size,
		Slide: &w.slide,
	}

	var windowResults []Result[T]

	for {
		select {
		case <-ctx.Done():
			if len(windowResults) > 0 {
				w.emitWindowResults(ctx, out, windowResults, currentWindow)
			}
			return

		case result, ok := <-in:
			if !ok {
				if len(windowResults) > 0 {
					w.emitWindowResults(ctx, out, windowResults, currentWindow)
				}
				return
			}
			windowResults = append(windowResults, result)

		case <-ticker.C():
			if len(windowResults) > 0 {
				w.emitWindowResults(ctx, out, windowResults, currentWindow)
			}

			windowResults = nil
			now := w.clock.Now()
			currentWindow = WindowMetadata{
				Start: now,
				End:   now.Add(w.size),
				Type:  "sliding",
				Size:  w.size,
				Slide: &w.slide,
			}
		}
	}
}

// emitWindowResults emits all results in the window with window metadata attached.
func (*SlidingWindow[T]) emitWindowResults(ctx context.Context, out chan<- Result[T], results []Result[T], meta WindowMetadata) {
	for _, result := range results {
		enhanced := AddWindowMetadata(result, meta)
		select {
		case out <- enhanced:
		case <-ctx.Done():
			return
		}
	}
}

// windowState tracks state for sliding windows.
type windowState[T any] struct {
	meta    WindowMetadata
	results []Result[T]
}

// emitAllWindows emits all windows when processing ends.
func (w *SlidingWindow[T]) emitAllWindows(ctx context.Context, out chan<- Result[T], windows map[time.Time]*windowState[T]) {
	for _, window := range windows {
		if len(window.results) > 0 {
			w.emitWindowResults(ctx, out, window.results, window.meta)
		}
	}
}

// Name returns the processor name for debugging and monitoring.
func (w *SlidingWindow[T]) Name() string {
	return w.name
}
