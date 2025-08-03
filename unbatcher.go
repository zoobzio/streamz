package streamz

import (
	"context"
)

// Unbatcher flattens batches into individual items.
// It's the inverse operation of Batcher.
type Unbatcher[T any] struct {
	name string
}

// NewUnbatcher creates a processor that converts []T channels to T channels.
// It flattens batches by emitting each item individually, preserving order.
//
// When to use:
//   - After batch processing to continue with individual items
//   - Converting batch results back to streams
//   - Interfacing between batch and stream processors
//
// Example:
//
//	// Process items in batches, then continue individually
//	batcher := streamz.NewBatcher[string](streamz.BatchConfig{MaxSize: 10})
//	batchProcessor := streamz.NewMapper("process-batch", processBatch)
//	unbatcher := streamz.NewUnbatcher[string]()
//
//	batched := batcher.Process(ctx, source)
//	processed := batchProcessor.Process(ctx, batched)
//	items := unbatcher.Process(ctx, processed)
func NewUnbatcher[T any]() *Unbatcher[T] {
	return &Unbatcher[T]{
		name: "unbatcher",
	}
}

func (*Unbatcher[T]) Process(ctx context.Context, in <-chan []T) <-chan T {
	out := make(chan T)

	go func() {
		defer close(out)

		for batch := range in {
			for _, item := range batch {
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

func (u *Unbatcher[T]) Name() string {
	return u.name
}
