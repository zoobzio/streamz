package streamz

import (
	"context"
)

// Mapper transforms items from one type to another using a synchronous function.
// It processes items sequentially without goroutines, making it ideal for fast
// transformations that don't benefit from concurrency overhead.
//
// For CPU-intensive or I/O-bound operations that can benefit from parallelization,
// use AsyncMapper instead.
//
//nolint:govet // fieldalignment: struct layout optimized for readability
type Mapper[In, Out any] struct {
	name string
	fn   func(context.Context, In) (Out, error)
}

// NewMapper creates a processor that transforms items synchronously.
// Unlike AsyncMapper, this processes items one at a time in sequence,
// making it suitable for simple, fast transformations.
//
// When to use:
//   - Simple type conversions and data formatting
//   - Fast computations that don't justify goroutine overhead
//   - Transformations that must maintain strict sequential processing
//   - Operations where concurrency would add complexity without benefit
//   - Memory-sensitive scenarios where goroutine pools are costly
//
// Example:
//
//	// Simple type conversion
//	toString := streamz.NewMapper(func(ctx context.Context, n int) (string, error) {
//		return fmt.Sprintf("%d", n), nil
//	})
//
//	// Data formatting
//	formatUser := streamz.NewMapper(func(ctx context.Context, u User) (string, error) {
//		return fmt.Sprintf("%s <%s>", u.Name, u.Email), nil
//	})
//
//	// Mathematical transformations
//	double := streamz.NewMapper(func(ctx context.Context, n int) (int, error) {
//		return n * 2, nil
//	})
//
//	results := toString.Process(ctx, input)
//	for result := range results {
//		if result.IsError() {
//			log.Printf("Processing error: %v", result.Error())
//		} else {
//			fmt.Printf("Result: %s\n", result.Value())
//		}
//	}
//
// Parameters:
//   - fn: Transformation function that converts In to Out
//
// Returns a new Mapper processor.
func NewMapper[In, Out any](fn func(context.Context, In) (Out, error)) *Mapper[In, Out] {
	return &Mapper[In, Out]{
		name: "mapper",
		fn:   fn,
	}
}

// WithName sets a custom name for this processor.
// If not set, defaults to "mapper".
func (m *Mapper[In, Out]) WithName(name string) *Mapper[In, Out] {
	m.name = name
	return m
}

// Process transforms input items synchronously using the provided function.
// Errors are passed through unchanged. Success values are transformed and
// wrapped in new Result instances.
func (m *Mapper[In, Out]) Process(ctx context.Context, in <-chan Result[In]) <-chan Result[Out] {
	out := make(chan Result[Out])

	go func() {
		defer close(out)

		for item := range in {
			select {
			case <-ctx.Done():
				return
			default:
			}

			if item.IsError() {
				// Pass through errors unchanged with correct type
				select {
				case out <- Result[Out]{err: &StreamError[Out]{
					Item:          *new(Out), // zero value for Out type
					Err:           item.Error(),
					ProcessorName: m.name,
					Timestamp:     item.Error().Timestamp,
				}}:
				case <-ctx.Done():
					return
				}
				continue
			}

			// Transform the success value
			result, err := m.fn(ctx, item.Value())
			if err != nil {
				select {
				case out <- NewError(result, err, m.name):
				case <-ctx.Done():
					return
				}
			} else {
				select {
				case out <- NewSuccess(result):
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out
}

// Name returns the processor name for debugging and monitoring.
func (m *Mapper[In, Out]) Name() string {
	return m.name
}
