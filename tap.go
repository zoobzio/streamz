package streamz

import (
	"context"
	"log"
)

// Tap executes a side effect function for each item while passing items through unchanged.
// It's used for logging, debugging, monitoring, metrics collection, and any other
// observational operations that shouldn't modify the data flow.
//
// Tap is the simplest processor in streamz - it observes without interfering.
// The side effect function is called for every item (both success and error cases)
// but has no effect on what gets passed to the next stage of the pipeline.
//
//nolint:govet // fieldalignment: struct layout optimized for readability
type Tap[T any] struct {
	name string
	fn   func(Result[T])
}

// NewTap creates a processor that executes a side effect function on each Result[T]
// while passing all items through unchanged. The side effect function receives
// the complete Result[T], allowing it to handle both success and error cases.
//
// When to use:
//   - Debug logging and tracing
//   - Metrics collection and monitoring
//   - Audit trails and compliance logging
//   - Performance monitoring and profiling
//   - Testing and verification
//   - Side effect operations that don't modify data
//
// Example:
//
//	// Simple logging
//	logger := streamz.NewTap(func(result Result[Order]) {
//		if result.IsError() {
//			log.Printf("Error processing order: %v", result.Error())
//		} else {
//			log.Printf("Order processed: %+v", result.Value())
//		}
//	})
//
//	// Metrics collection
//	var processedCount, errorCount atomic.Int64
//	metrics := streamz.NewTap(func(result Result[Order]) {
//		if result.IsError() {
//			errorCount.Add(1)
//		} else {
//			processedCount.Add(1)
//		}
//	})
//
//	// Debug at specific pipeline stage
//	debug := streamz.NewTap(func(result Result[Order]) {
//		if result.IsSuccess() {
//			fmt.Printf("DEBUG: Order %s at validation stage\n",
//				result.Value().ID)
//		}
//	}).WithName("validation-debug")
//
//	results := logger.Process(ctx, input)
//	for result := range results {
//		// Side effects executed, items unchanged
//		fmt.Printf("Result: %+v\n", result)
//	}
//
// Parameters:
//   - fn: Side effect function that receives each Result[T]
//
// Returns a new Tap processor.
func NewTap[T any](fn func(Result[T])) *Tap[T] {
	return &Tap[T]{
		name: "tap",
		fn:   fn,
	}
}

// WithName sets a custom name for this processor.
// If not set, defaults to "tap".
// The name is used for debugging, monitoring, and error reporting.
func (t *Tap[T]) WithName(name string) *Tap[T] {
	t.name = name
	return t
}

// Process executes the side effect function on each item while passing all items
// through unchanged. Both successful values and errors are observed and forwarded.
// The side effect function is called with the complete Result[T], allowing it
// to distinguish between success and error cases.
func (t *Tap[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan Result[T] {
	out := make(chan Result[T])

	go func() {
		defer close(out)

		for item := range in {
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Execute side effect function with panic recovery
			// This happens for ALL items - both success and error
			func() {
				defer func() {
					if r := recover(); r != nil {
						// Side effect panicked - log but continue processing
						// We don't want side effect panics to break the pipeline
						log.Printf("Tap[%s]: side effect panicked: %v", t.name, r)
					}
				}()
				t.fn(item)
			}()

			// Pass through unchanged
			select {
			case out <- item:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

// Name returns the processor name for debugging and monitoring.
func (t *Tap[T]) Name() string {
	return t.name
}
