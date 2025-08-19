package streamz

import (
	"context"
)

// Mapper transforms each item in a stream from one type to another using a mapping function.
// This is a fundamental operation for data transformation in streaming pipelines,
// allowing type-safe conversions and data enrichment.
type Mapper[In, Out any] struct {
	fn   func(In) Out
	name string
}

// NewMapper creates a processor that transforms items from one type to another.
// This is the fundamental transformation operation, allowing type-safe conversions
// and data enrichment throughout the stream pipeline.
// Use the fluent API to configure optional behavior like custom names.
//
// When to use:
//   - Type conversions between data representations
//   - Data enrichment and augmentation
//   - Extracting fields or computing derived values
//   - Normalizing data formats
//   - Implementing business logic transformations
//
// Example:
//
//	// Simple mapper with auto-generated name
//	upper := streamz.NewMapper(strings.ToUpper)
//
//	// Mapper with custom name for monitoring
//	upper := streamz.NewMapper(strings.ToUpper).WithName("uppercase")
//
//	uppercased := upper.Process(ctx, strings)
//	for s := range uppercased {
//		fmt.Println(s) // All uppercase
//	}
//
//	// Extract and transform nested data
//	usernames := streamz.NewMapper(func(u User) string {
//		return fmt.Sprintf("%s <%s>", u.Name, u.Email)
//	}).WithName("extract-username")
//
//	// Type conversion with computation
//	totals := streamz.NewMapper(func(order Order) OrderSummary {
//		return OrderSummary{
//			OrderID:   order.ID,
//			Total:     calculateTotal(order.Items),
//			ItemCount: len(order.Items),
//			Status:    order.Status,
//		}
//	}).WithName("calculate-total")
//
// Parameters:
//   - fn: Pure transformation function from input to output type
//
// Returns a new Mapper processor with fluent configuration.
func NewMapper[In, Out any](fn func(In) Out) *Mapper[In, Out] {
	return &Mapper[In, Out]{
		fn:   fn,
		name: "mapper", // default name
	}
}

// WithName sets a custom name for this processor.
// If not set, defaults to "mapper".
func (m *Mapper[In, Out]) WithName(name string) *Mapper[In, Out] {
	m.name = name
	return m
}

func (m *Mapper[In, Out]) Process(ctx context.Context, in <-chan In) <-chan Out {
	out := make(chan Out)

	go func() {
		defer close(out)

		for item := range in {
			select {
			case out <- m.fn(item):
			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

func (m *Mapper[In, Out]) Name() string {
	return m.name
}
