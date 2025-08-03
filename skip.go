package streamz

import (
	"context"
)

// Skip discards the first n items from a stream.
type Skip[T any] struct {
	name  string
	count int
}

// NewSkip creates a processor that skips the first n items.
// After skipping n items, all subsequent items are passed through.
//
// When to use:
//   - Skip headers or metadata at the start of a stream
//   - Ignore warm-up data from sensors
//   - Implement offset-based pagination
//   - Skip known invalid initial data
//
// Example:
//
//	// Skip the first 10 warm-up readings
//	skip := streamz.NewSkip[SensorData](10)
//	stable := skip.Process(ctx, readings)
//
//	// Skip header rows in a data stream
//	skip := streamz.NewSkip[Row](1)
//	data := skip.Process(ctx, rows)
func NewSkip[T any](count int) *Skip[T] {
	return &Skip[T]{
		count: count,
		name:  "skip",
	}
}

func (s *Skip[T]) Process(ctx context.Context, in <-chan T) <-chan T {
	out := make(chan T)

	go func() {
		defer close(out)

		skipped := 0
		for item := range in {
			if skipped < s.count {
				skipped++
				continue
			}

			select {
			case out <- item:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

func (s *Skip[T]) Name() string {
	return s.name
}
