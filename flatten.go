package streamz

import (
	"context"
)

// Flatten expands slices into individual items, converting a stream of []T into
// a stream of T. This is the inverse of batching operations, useful for unpacking
// grouped data back into individual elements.
type Flatten[T any] struct {
	name string
}

// NewFlatten creates a processor that flattens slices into individual items.
// Each slice in the input stream is expanded so that each element becomes
// a separate item in the output stream, preserving order.
//
// When to use:
//   - Unpacking results from batch APIs
//   - Processing array fields from JSON/database records
//   - Converting paginated responses to item streams
//   - Expanding grouped data for individual processing
//
// Example:
//
//	// Flatten API responses that return arrays
//	flatten := streamz.NewFlatten[User]()
//
//	// API returns pages of users
//	pages := make(chan []User)
//	users := flatten.Process(ctx, pages)
//
//	for user := range users {
//		// Process individual users from pages
//		processUser(user)
//	}
//
//	// Unpack database query results
//	queryResults := make(chan []Record)
//	individual := flatten.Process(ctx, queryResults)
//
// Returns a new Flatten processor.
func NewFlatten[T any]() *Flatten[T] {
	return &Flatten[T]{
		name: "flatten",
	}
}

func (*Flatten[T]) Process(ctx context.Context, in <-chan []T) <-chan T {
	out := make(chan T)

	go func() {
		defer close(out)

		for slice := range in {
			for _, item := range slice {
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

func (f *Flatten[T]) Name() string {
	return f.name
}
