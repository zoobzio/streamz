package streamz

import (
	"context"
)

// Chunk groups items into fixed-size arrays without time constraints.
// Unlike Batcher which considers both size and time, Chunk only emits
// when exactly 'size' items have been collected, making it predictable
// for fixed-size batch operations.
type Chunk[T any] struct {
	name string
	size int
}

// NewChunk creates a processor that groups items into fixed-size chunks.
// The last chunk may be smaller if the stream ends before filling completely.
//
// When to use:
//   - Processing data in fixed-size batches
//   - Implementing pagination or fixed-size packets
//   - Database bulk inserts with specific batch sizes
//   - Matrix operations requiring fixed dimensions
//
// Example:
//
//	// Process items in groups of 100
//	chunker := streamz.NewChunk[Record](100)
//
//	chunks := chunker.Process(ctx, records)
//	for chunk := range chunks {
//		// Each chunk has exactly 100 records (except possibly the last)
//		bulkInsert(chunk)
//	}
//
//	// Fixed-size matrix operations
//	matrixChunker := streamz.NewChunk[float64](16) // 4x4 matrices
//	matrices := matrixChunker.Process(ctx, values)
//	for matrix := range matrices {
//		processMatrix(matrix) // Always 16 elements
//	}
//
// Parameters:
//   - size: Number of items per chunk (must be > 0)
//
// Returns a new Chunk processor.
func NewChunk[T any](size int) *Chunk[T] {
	return &Chunk[T]{
		size: size,
		name: "chunk",
	}
}

func (c *Chunk[T]) Process(ctx context.Context, in <-chan T) <-chan []T {
	out := make(chan []T)

	go func() {
		defer close(out)

		chunk := make([]T, 0, c.size)

		for item := range in {
			chunk = append(chunk, item)

			if len(chunk) >= c.size {
				select {
				case out <- chunk:
					chunk = make([]T, 0, c.size)
				case <-ctx.Done():
					return
				}
			}
		}

		if len(chunk) > 0 {
			select {
			case out <- chunk:
			case <-ctx.Done():
			}
		}
	}()

	return out
}

func (c *Chunk[T]) Name() string {
	return c.name
}
