package streamz

import (
	"context"
)

// Take limits the stream to the first n items.
type Take[T any] struct {
	name  string
	count int
}

// NewTake creates a processor that takes only the first n items from a stream.
// After emitting n items, it closes the output channel and drains remaining input.
//
// When to use:
//   - Limit processing to a sample of data
//   - Implement pagination or batching
//   - Testing with limited data sets
//   - Early termination of infinite streams
//
// Example:
//
//	// Process only the first 100 items
//	take := streamz.NewTake[Event](100)
//	limited := take.Process(ctx, events)
//
//	// Take first 10 results from a search
//	take := streamz.NewTake[SearchResult](10)
//	topResults := take.Process(ctx, results)
func NewTake[T any](count int) *Take[T] {
	return &Take[T]{
		count: count,
		name:  "take",
	}
}

func (t *Take[T]) Process(ctx context.Context, in <-chan T) <-chan T {
	out := make(chan T)

	go func() {
		defer close(out)

		taken := 0
		for item := range in {
			if taken >= t.count {
				break
			}

			select {
			case out <- item:
				taken++
			case <-ctx.Done():
				return
			}
		}

		//nolint:revive // empty-block: necessary to drain remaining items from input channel
		for range in {
		}
	}()

	return out
}

func (t *Take[T]) Name() string {
	return t.name
}
