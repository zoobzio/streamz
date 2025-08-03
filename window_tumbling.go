package streamz

import (
	"context"
	"time"
)

// TumblingWindow groups items into fixed-size, non-overlapping time windows.
// Each item belongs to exactly one window, and windows are emitted when their
// time period expires, making it ideal for time-based aggregations.
type TumblingWindow[T any] struct {
	name string
	size time.Duration
}

// NewTumblingWindow creates a processor that groups items into fixed-size time windows.
// Unlike sliding windows, tumbling windows don't overlap - each item belongs to exactly
// one window. Windows are emitted when their time period expires.
//
// When to use:
//   - Time-based aggregations (hourly stats, daily summaries)
//   - Periodic batch processing
//   - Rate calculations over fixed intervals
//   - Log rotation and archival
//   - Metrics collection and reporting
//
// Example:
//
//	// Aggregate events into 1-minute windows
//	window := streamz.NewTumblingWindow[Event](time.Minute)
//
//	windows := window.Process(ctx, events)
//	for w := range windows {
//		summary := aggregateEvents(w.Items)
//		log.Printf("Window [%s - %s]: %d events, avg value: %.2f",
//			w.Start.Format("15:04:05"),
//			w.End.Format("15:04:05"),
//			len(w.Items),
//			summary.Average)
//	}
//
//	// Hourly report generation
//	hourly := streamz.NewTumblingWindow[Metric](time.Hour)
//	reports := hourly.Process(ctx, metrics)
//	for window := range reports {
//		generateHourlyReport(window)
//	}
//
// Parameters:
//   - size: Duration of each window (e.g., 1 minute, 1 hour)
//
// Returns a new TumblingWindow processor for time-based grouping.
func NewTumblingWindow[T any](size time.Duration) *TumblingWindow[T] {
	return &TumblingWindow[T]{
		size: size,
		name: "tumbling-window",
	}
}

func (w *TumblingWindow[T]) Process(ctx context.Context, in <-chan T) <-chan Window[T] {
	out := make(chan Window[T])

	go func() {
		defer close(out)

		ticker := time.NewTicker(w.size)
		defer ticker.Stop()

		window := &Window[T]{
			Items: []T{},
			Start: time.Now(),
			End:   time.Now().Add(w.size),
		}

		for {
			select {
			case <-ctx.Done():
				if len(window.Items) > 0 {
					out <- *window
				}
				return

			case item, ok := <-in:
				if !ok {
					if len(window.Items) > 0 {
						out <- *window
					}
					return
				}
				window.Items = append(window.Items, item)

			case <-ticker.C:
				if len(window.Items) > 0 {
					out <- *window
				}
				window = &Window[T]{
					Items: []T{},
					Start: time.Now(),
					End:   time.Now().Add(w.size),
				}
			}
		}
	}()

	return out
}

func (w *TumblingWindow[T]) Name() string {
	return w.name
}
