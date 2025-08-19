package streamz

import (
	"context"
)

// Filter selectively passes items through a stream based on a predicate function.
// Only items for which the predicate returns true are emitted to the output channel.
// This is one of the most fundamental stream processing operations.
type Filter[T any] struct {
	predicate func(T) bool
	name      string
}

// NewFilter creates a processor that selectively passes items based on a predicate.
// Only items for which the predicate returns true are forwarded to the output stream.
// This is one of the most fundamental stream operations.
// Use the fluent API to configure optional behavior like custom names.
//
// When to use:
//   - Data validation and quality control
//   - Removing noise or irrelevant data
//   - Implementing business rules and conditions
//   - Subsetting data based on criteria
//   - Security filtering and access control
//
// Example:
//
//	// Simple filter with auto-generated name
//	positive := streamz.NewFilter(func(n int) bool {
//		return n > 0
//	})
//
//	// Filter with custom name for monitoring
//	positive := streamz.NewFilter(func(n int) bool {
//		return n > 0
//	}).WithName("positive-numbers")
//
//	positives := positive.Process(ctx, numbers)
//	for n := range positives {
//		// Only positive numbers pass through
//		fmt.Printf("Positive: %d\n", n)
//	}
//
//	// Filter events by multiple criteria
//	important := streamz.NewFilter(func(e Event) bool {
//		return e.Priority == "HIGH" &&
//		       e.Timestamp.After(cutoffTime) &&
//		       contains(allowedTypes, e.Type)
//	}).WithName("important-events")
//
// Parameters:
//   - predicate: Function that returns true for items to keep
//
// Returns a new Filter processor with fluent configuration.
func NewFilter[T any](predicate func(T) bool) *Filter[T] {
	return &Filter[T]{
		predicate: predicate,
		name:      "filter", // default name
	}
}

// WithName sets a custom name for this processor.
// If not set, defaults to "filter".
func (f *Filter[T]) WithName(name string) *Filter[T] {
	f.name = name
	return f
}

func (f *Filter[T]) Process(ctx context.Context, in <-chan T) <-chan T {
	out := make(chan T)

	go func() {
		defer close(out)

		for item := range in {
			if f.predicate(item) {
				select {
				case out <- item:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out
}

func (f *Filter[T]) Name() string {
	return f.name
}
