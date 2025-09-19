package streamz

import (
	"context"
	"time"
)

// TumblingWindow groups items into fixed-size, non-overlapping time windows.
// Each item gets window metadata attached and is emitted when its window expires,
// making it ideal for time-based aggregations with Result[T] metadata flow.
//
// This version processes Result[T] streams, attaching window metadata to each
// individual Result for comprehensive monitoring and downstream processing.
//
// Key characteristics:
//   - Non-overlapping: Each item belongs to exactly one window
//   - Fixed duration: All windows have the same size
//   - Metadata-driven: Results carry window context via metadata
//   - Predictable emission: Results emit at exact window boundaries
//
// Performance characteristics:
//   - Result emission latency: Exactly at window boundary (size duration)
//   - Memory usage: O(items_per_window) - bounded by window size
//   - Processing overhead: Metadata attachment per item
//   - Goroutine usage: 1 goroutine per processor instance
//   - No unbounded memory growth - results are emitted and cleared
//
//nolint:govet // fieldalignment: struct layout optimized for readability
type TumblingWindow[T any] struct {
	name  string
	clock Clock
	size  time.Duration
}

// NewTumblingWindow creates a processor that groups Results into fixed-size time windows.
// Unlike sliding windows, tumbling windows don't overlap - each Result belongs to exactly
// one window. Windows are emitted when their time period expires.
//
// When to use:
//   - Time-based aggregations with error tracking (hourly stats, daily summaries)
//   - Periodic batch processing with failure monitoring
//   - Rate calculations over fixed intervals
//   - Log analysis and error reporting over time periods
//   - Metrics collection with success/failure rates
//
// Example:
//
//	// Process events with 1-minute window metadata
//	window := streamz.NewTumblingWindow[Event](time.Minute, streamz.RealClock)
//
//	results := window.Process(ctx, eventResults)
//	for result := range results {
//		// Each result now has window metadata attached
//		if meta, err := streamz.GetWindowMetadata(result); err == nil {
//			fmt.Printf("Event in window [%s - %s]: %v\n",
//				meta.Start.Format("15:04:05"),
//				meta.End.Format("15:04:05"),
//				result.Value())
//		}
//	}
//
//	// Collect into traditional windows when needed
//	collector := streamz.NewWindowCollector[Event]()
//	collections := collector.Process(ctx, results)
//	for collection := range collections {
//		values := collection.Values()   // Only successful events
//		errors := collection.Errors()   // Only errors
//		generateReport(values, errors)
//	}
//
// Parameters:
//   - size: Duration of each window (must be > 0)
//   - clock: Clock interface for time operations (use RealClock for production)
//
// Returns a new TumblingWindow processor for time-based grouping with Result[T] support.
//
// Performance notes:
//   - Optimal for non-overlapping aggregations
//   - Minimal memory overhead (single active window)
//   - Predictable latency: exactly window size duration
func NewTumblingWindow[T any](size time.Duration, clock Clock) *TumblingWindow[T] {
	return &TumblingWindow[T]{
		size:  size,
		name:  "tumbling-window",
		clock: clock,
	}
}

// WithName sets a custom name for this processor.
func (w *TumblingWindow[T]) WithName(name string) *TumblingWindow[T] {
	w.name = name
	return w
}

// Process groups Results into fixed-size time windows, emitting individual Results with window metadata.
// Both successful values and errors are captured with their window context, enabling comprehensive
// error tracking and success rate monitoring over time periods.
//
// Window behavior:
//   - Each Result gets window metadata attached (start, end, type, size)
//   - Results are emitted exactly at their window boundary expiration
//   - Empty windows produce no output
//   - On context cancellation or input close, partial windows emit their Results if non-empty
//
// Performance and resource usage:
//   - Zero allocation for window tracking (single active window)
//   - Predictable memory: size = items_per_window × average_item_size
//   - Latency: Items buffered for up to window size duration
//   - Thread-safe: Single goroutine architecture prevents races
func (w *TumblingWindow[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan Result[T] {
	out := make(chan Result[T])

	go func() {
		defer close(out)

		ticker := w.clock.NewTicker(w.size)
		defer ticker.Stop()

		now := w.clock.Now()
		currentWindow := WindowMetadata{
			Start: now,
			End:   now.Add(w.size),
			Type:  "tumbling",
			Size:  w.size,
		}

		var windowResults []Result[T]

		for {
			select {
			case <-ctx.Done():
				// Emit remaining results with window metadata
				w.emitWindowResults(ctx, out, windowResults, currentWindow)
				return

			case result, ok := <-in:
				if !ok {
					// Input closed, emit remaining results
					w.emitWindowResults(ctx, out, windowResults, currentWindow)
					return
				}
				windowResults = append(windowResults, result)

			case <-ticker.C():
				// Window expired, emit all results with window metadata
				w.emitWindowResults(ctx, out, windowResults, currentWindow)

				// Create new window
				windowResults = nil
				now := w.clock.Now()
				currentWindow = WindowMetadata{
					Start: now,
					End:   now.Add(w.size),
					Type:  "tumbling",
					Size:  w.size,
				}
			}
		}
	}()

	return out
}

// emitWindowResults emits all results in the window with window metadata attached.
func (*TumblingWindow[T]) emitWindowResults(ctx context.Context, out chan<- Result[T], results []Result[T], meta WindowMetadata) {
	for _, result := range results {
		enhanced := AddWindowMetadata(result, meta)
		select {
		case out <- enhanced:
		case <-ctx.Done():
			return
		}
	}
}

// Name returns the processor name for debugging and monitoring.
func (w *TumblingWindow[T]) Name() string {
	return w.name
}
