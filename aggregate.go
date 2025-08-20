package streamz

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// AggregateFunc combines multiple items into a single aggregated value.
// It receives the current aggregate state and a new item, returning the updated state.
type AggregateFunc[T, A any] func(state A, item T) A

// AggregateWindow represents a window of aggregated data.
type AggregateWindow[A any] struct { //nolint:govet // logical field grouping preferred over memory optimization
	// Result is the aggregated value for this window.
	Result A

	// Start is the window start time.
	Start time.Time

	// End is the window end time.
	End time.Time

	// Count is the number of items in this window.
	Count int
}

// Aggregate performs stateful aggregation over items in a stream.
// It maintains an aggregate state that is updated with each new item
// and emits results based on configured triggers (count, time, or both).
//
// The Aggregate processor is essential for:
//   - Computing running statistics (sum, average, min, max).
//   - Time-based aggregations (hourly totals, daily counts).
//   - Custom aggregations (percentiles, unique counts).
//   - Incremental computation over streams.
//
// Key features:
//   - Flexible aggregation functions.
//   - Multiple trigger types (count, time, or combined).
//   - Windowed or continuous aggregation.
//   - Thread-safe state management.
//   - Proper handling of empty windows.
//
// Example:
//
//	// Sum values in 1-minute windows.
//	summer := streamz.NewAggregate[int, int](
//	    0,                          // Initial state.
//	    func(sum, n int) int {      // Aggregation function.
//	        return sum + n
//	    },
//	    Real,
//	).WithTimeWindow(time.Minute)
//
//	windows := summer.Process(ctx, numbers)
//	for window := range windows {
//	    fmt.Printf("Sum for %s: %d\n", window.Start.Format("15:04"), window.Result)
//	}
//
// Performance characteristics:
//   - O(1) per item processing.
//   - Memory usage proportional to state size.
//   - Efficient incremental computation.
type Aggregate[T, A any] struct { //nolint:govet // logical field grouping preferred over memory optimization
	initial    A                   // Initial state value.
	aggregator AggregateFunc[T, A] // Aggregation function.

	// Trigger configuration.
	maxCount   int           // Emit after N items (0 = disabled).
	maxLatency time.Duration // Emit after duration (0 = disabled).

	// State management.
	mu       sync.RWMutex
	state    A
	count    int
	lastEmit time.Time

	// Metadata.
	name      string
	emitEmpty bool // Whether to emit windows with no data.
	clock     Clock
}

// NewAggregate creates a processor that performs stateful aggregation.
// The initial value provides the starting state, and the aggregator function
// updates the state with each new item.
//
// Default configuration:
//   - No automatic emission (must configure triggers).
//   - Name: "aggregate".
//   - Empty windows not emitted.
//
// Parameters:
//   - initial: The initial aggregate state.
//   - aggregator: Function to update state with new items.
//   - clock: Clock interface for time operations
//
// Returns: A new Aggregate processor with fluent configuration methods.
func NewAggregate[T, A any](initial A, aggregator AggregateFunc[T, A], clock Clock) *Aggregate[T, A] {
	return &Aggregate[T, A]{
		initial:    initial,
		aggregator: aggregator,
		state:      initial,
		name:       "aggregate",
		lastEmit:   clock.Now(),
		clock:      clock,
	}
}

// WithCountWindow configures emission after a specific number of items.
// The aggregate state is reset after each emission.
func (a *Aggregate[T, A]) WithCountWindow(count int) *Aggregate[T, A] {
	if count < 1 {
		count = 1
	}
	a.maxCount = count
	return a
}

// WithTimeWindow configures emission after a specific duration.
// The aggregate state is reset after each emission.
func (a *Aggregate[T, A]) WithTimeWindow(duration time.Duration) *Aggregate[T, A] {
	if duration < 0 {
		duration = 0
	}
	a.maxLatency = duration
	return a
}

// WithEmptyWindows configures whether to emit windows with no data.
// Useful for time-based aggregations where you want regular updates.
func (a *Aggregate[T, A]) WithEmptyWindows(emit bool) *Aggregate[T, A] {
	a.emitEmpty = emit
	return a
}

// WithName sets a custom name for this processor instance.
func (a *Aggregate[T, A]) WithName(name string) *Aggregate[T, A] {
	a.name = name
	return a
}

// Process aggregates items from the input stream and emits windowed results.
func (a *Aggregate[T, A]) Process(ctx context.Context, in <-chan T) <-chan AggregateWindow[A] {
	out := make(chan AggregateWindow[A])

	// Reset state for new processing
	a.mu.Lock()
	a.state = a.initial
	a.count = 0
	a.lastEmit = a.clock.Now()
	a.mu.Unlock()

	go func() {
		defer close(out)

		// Set up time-based emission if configured
		var ticker Ticker
		var tickerChan <-chan time.Time

		if a.maxLatency > 0 {
			ticker = a.clock.NewTicker(a.maxLatency)
			tickerChan = ticker.C()
			defer ticker.Stop()
		}

		for {
			select {
			case item, ok := <-in:
				if !ok {
					// Emit final window if there's data
					a.mu.Lock()
					if a.count > 0 {
						window := AggregateWindow[A]{
							Result: a.state,
							Start:  a.lastEmit,
							End:    a.clock.Now(),
							Count:  a.count,
						}
						a.mu.Unlock()

						select {
						case out <- window:
						case <-ctx.Done():
						}
					} else {
						a.mu.Unlock()
					}
					return
				}

				// Update aggregate state
				a.mu.Lock()
				a.state = a.aggregator(a.state, item)
				a.count++
				currentCount := a.count
				a.mu.Unlock()

				// Check count-based trigger
				if a.maxCount > 0 && currentCount >= a.maxCount {
					a.emit(ctx, out)
				}

			case <-tickerChan:
				// Time-based trigger
				a.mu.RLock()
				hasData := a.count > 0
				a.mu.RUnlock()

				if hasData || a.emitEmpty {
					a.emit(ctx, out)
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

// emit sends the current aggregate window and resets state.
func (a *Aggregate[T, A]) emit(ctx context.Context, out chan<- AggregateWindow[A]) {
	a.mu.Lock()
	window := AggregateWindow[A]{
		Result: a.state,
		Start:  a.lastEmit,
		End:    a.clock.Now(),
		Count:  a.count,
	}

	// Reset state
	a.state = a.initial
	a.count = 0
	a.lastEmit = window.End
	a.mu.Unlock()

	select {
	case out <- window:
	case <-ctx.Done():
	}
}

// GetCurrentState returns the current aggregate state (for monitoring).
// This is a snapshot and may change immediately after returning.
func (a *Aggregate[T, A]) GetCurrentState() (state A, count int) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.state, a.count
}

// Name returns the processor name.
func (a *Aggregate[T, A]) Name() string {
	return a.name
}

// Common aggregation functions

// Sum returns an aggregator that sums numeric values.
func Sum[T ~int | ~int32 | ~int64 | ~float32 | ~float64]() AggregateFunc[T, T] {
	return func(sum, item T) T {
		return sum + item
	}
}

// Count returns an aggregator that counts items.
func Count[T any]() AggregateFunc[T, int] {
	return func(count int, _ T) int {
		return count + 1
	}
}

// Average maintains a running average.
type Average struct { //nolint:govet // logical field grouping preferred over memory optimization
	Sum   float64
	Count int
}

// Avg returns an aggregator that computes the average of numeric values.
func Avg[T ~int | ~int32 | ~int64 | ~float32 | ~float64]() AggregateFunc[T, Average] {
	return func(avg Average, item T) Average {
		avg.Sum += float64(item)
		avg.Count++
		return avg
	}
}

// Value returns the computed average.
func (a Average) Value() float64 {
	if a.Count == 0 {
		return 0
	}
	return a.Sum / float64(a.Count)
}

// MinMax tracks minimum and maximum values.
type MinMax[T comparable] struct { //nolint:govet // logical field grouping preferred over memory optimization
	Min   T
	Max   T
	Count int
}

// MinMaxAgg returns an aggregator that tracks min and max values.
func MinMaxAgg[T ~int | ~int32 | ~int64 | ~float32 | ~float64]() AggregateFunc[T, MinMax[T]] {
	return func(mm MinMax[T], item T) MinMax[T] {
		if mm.Count == 0 {
			return MinMax[T]{Min: item, Max: item, Count: 1}
		}
		if item < mm.Min {
			mm.Min = item
		}
		if item > mm.Max {
			mm.Max = item
		}
		mm.Count++
		return mm
	}
}

// String returns a string representation of the min/max values.
func (mm MinMax[T]) String() string {
	return fmt.Sprintf("Min: %v, Max: %v, Count: %d", mm.Min, mm.Max, mm.Count)
}
