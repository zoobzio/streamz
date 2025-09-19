package streamz

import (
	"context"
)

// Filter selectively passes items through a stream based on a predicate function.
// Only items for which the predicate returns true are emitted to the output channel.
// Items that don't match the predicate are discarded.
//
// Filter is one of the most fundamental stream processing operations, commonly used for:
//   - Data validation and quality control
//   - Business rule application
//   - Security filtering and content moderation
//   - Performance optimization by reducing downstream load
//   - A/B testing and conditional data routing
//
//nolint:govet // fieldalignment: struct layout optimized for readability
type Filter[T any] struct {
	name      string
	predicate func(T) bool
}

// NewFilter creates a processor that selectively passes items based on a predicate.
// Items for which the predicate returns true are forwarded unchanged.
// Items for which the predicate returns false are discarded.
//
// The predicate function should be pure (no side effects) and deterministic
// for consistent and predictable filtering behavior.
//
// When to use:
//   - Remove invalid or unwanted data from streams
//   - Apply business rules and validation logic
//   - Filter based on data quality requirements
//   - Implement conditional processing logic
//   - Reduce processing load by filtering upstream
//
// Example:
//
//	// Filter positive numbers
//	positive := streamz.NewFilter(func(n int) bool {
//		return n > 0
//	})
//
//	// Filter non-empty strings
//	nonEmpty := streamz.NewFilter(func(s string) bool {
//		return strings.TrimSpace(s) != ""
//	})
//
//	// Filter valid orders
//	validOrders := streamz.NewFilter(func(order Order) bool {
//		return order.ID != "" && order.Amount > 0 && order.Status == "pending"
//	})
//
//	results := positive.Process(ctx, input)
//	for result := range results {
//		if result.IsError() {
//			log.Printf("Processing error: %v", result.Error())
//		} else {
//			fmt.Printf("Filtered result: %v\n", result.Value())
//		}
//	}
//
// Parameters:
//   - predicate: Function that returns true for items to keep, false to discard
//
// Returns a new Filter processor.
func NewFilter[T any](predicate func(T) bool) *Filter[T] {
	return &Filter[T]{
		name:      "filter",
		predicate: predicate,
	}
}

// WithName sets a custom name for this processor.
// If not set, defaults to "filter".
// The name is used for debugging, monitoring, and error reporting.
func (f *Filter[T]) WithName(name string) *Filter[T] {
	f.name = name
	return f
}

// Process filters input items based on the predicate function.
// Items that match the predicate (return true) are forwarded unchanged.
// Items that don't match the predicate are discarded.
// Errors are passed through unchanged without applying the predicate.
func (f *Filter[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan Result[T] {
	out := make(chan Result[T])

	go func() {
		defer close(out)

		for item := range in {
			select {
			case <-ctx.Done():
				return
			default:
			}

			if item.IsError() {
				// Pass through errors unchanged
				select {
				case out <- Result[T]{err: &StreamError[T]{
					Item:          item.Error().Item,
					Err:           item.Error().Err,
					ProcessorName: f.name,
					Timestamp:     item.Error().Timestamp,
				}}:
				case <-ctx.Done():
					return
				}
				continue
			}

			// Apply predicate to success values
			if f.predicate(item.Value()) {
				// Keep the item - forward unchanged
				select {
				case out <- item:
				case <-ctx.Done():
					return
				}
			}
			// Items that don't match predicate are silently discarded
		}
	}()

	return out
}

// Name returns the processor name for debugging and monitoring.
func (f *Filter[T]) Name() string {
	return f.name
}
