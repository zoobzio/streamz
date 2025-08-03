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
//	// Filter positive numbers
//	positive := streamz.NewFilter("positive", func(n int) bool {
//		return n > 0
//	})
//
//	positives := positive.Process(ctx, numbers)
//	for n := range positives {
//		// Only positive numbers pass through
//		fmt.Printf("Positive: %d\n", n)
//	}
//
//	// Filter events by multiple criteria
//	important := streamz.NewFilter("important-events", func(e Event) bool {
//		return e.Priority == "HIGH" &&
//		       e.Timestamp.After(cutoffTime) &&
//		       contains(allowedTypes, e.Type)
//	})
//
// Parameters:
//   - name: Descriptive name for debugging and monitoring
//   - predicate: Function that returns true for items to keep
//
// Returns a new Filter processor that forwards only matching items.
func NewFilter[T any](name string, predicate func(T) bool) *Filter[T] {
	return &Filter[T]{
		predicate: predicate,
		name:      name,
	}
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
