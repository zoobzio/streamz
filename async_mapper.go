package streamz

import (
	"context"
	"runtime"
	"sync"
)

// AsyncMapper processes items concurrently using multiple worker goroutines.
// It supports both ordered processing (preserving input sequence) and unordered
// processing (emitting results as they complete). This enables parallelization
// of CPU-intensive or I/O-bound operations while maintaining flexibility in
// ordering requirements.
//
//nolint:govet // fieldalignment: struct layout optimized for readability
type AsyncMapper[In, Out any] struct {
	name       string
	fn         func(context.Context, In) (Out, error)
	workers    int
	ordered    bool
	bufferSize int
}

// NewAsyncMapper creates a processor that executes transformations concurrently.
// By default, it preserves input order and uses runtime.NumCPU() workers.
// Use the fluent API to configure behavior like worker count and ordering.
//
// When to use:
//   - CPU-intensive transformations (image processing, encryption)
//   - I/O-bound operations (API calls, database queries)
//   - Parallel enrichment while optionally maintaining sequence
//   - Speeding up independent transformations
//   - Rate-limited API calls with concurrent workers
//
// Example:
//
//	// Parallel API enrichment with preserved order
//	enricher := streamz.NewAsyncMapper(func(ctx context.Context, id string) (User, error) {
//		// Each API call happens in parallel
//		return fetchUserFromAPI(ctx, id)
//	})
//
//	// Custom worker count for rate-limited APIs
//	enricher := streamz.NewAsyncMapper(func(ctx context.Context, id string) (User, error) {
//		return fetchUserFromAPI(ctx, id)
//	}).WithWorkers(10)
//
//	// Unordered processing for maximum throughput
//	processor := streamz.NewAsyncMapper(func(ctx context.Context, img Image) (Thumbnail, error) {
//		return generateThumbnail(ctx, img)
//	}).WithOrdered(false).WithWorkers(runtime.NumCPU())
//
//	results := processor.Process(ctx, input)
//	for result := range results {
//		if result.IsError() {
//			log.Printf("Processing error: %v", result.Error())
//		} else {
//			fmt.Printf("Result: %+v\n", result.Value())
//		}
//	}
//
// Parameters:
//   - fn: Transformation function that can be safely executed concurrently
//
// Returns a new AsyncMapper processor with fluent configuration.
func NewAsyncMapper[In, Out any](fn func(context.Context, In) (Out, error)) *AsyncMapper[In, Out] {
	return &AsyncMapper[In, Out]{
		name:       "async-mapper",
		fn:         fn,
		workers:    runtime.NumCPU(),
		ordered:    true,
		bufferSize: 100, // reasonable default for reorder buffer
	}
}

// WithWorkers sets the number of concurrent workers.
// If not set, defaults to runtime.NumCPU().
func (a *AsyncMapper[In, Out]) WithWorkers(workers int) *AsyncMapper[In, Out] {
	if workers > 0 {
		a.workers = workers
	}
	return a
}

// WithOrdered controls whether output preserves input order.
// If ordered=true (default), output items maintain their input sequence despite
// variable processing times. If ordered=false, results are emitted as they complete.
func (a *AsyncMapper[In, Out]) WithOrdered(ordered bool) *AsyncMapper[In, Out] {
	a.ordered = ordered
	return a
}

// WithBufferSize sets the reorder buffer size for ordered processing.
// This controls memory usage when processing times vary significantly.
// Only affects ordered mode. Defaults to 100.
func (a *AsyncMapper[In, Out]) WithBufferSize(size int) *AsyncMapper[In, Out] {
	if size > 0 {
		a.bufferSize = size
	}
	return a
}

// WithName sets a custom name for this processor.
// If not set, defaults to "async-mapper".
func (a *AsyncMapper[In, Out]) WithName(name string) *AsyncMapper[In, Out] {
	a.name = name
	return a
}

// sequencedItem tracks items with their original sequence number for ordered processing.
type sequencedItem[T any] struct {
	item T
	seq  uint64
}

// Process transforms input items concurrently across multiple workers.
// In ordered mode, output maintains input sequence despite variable processing times.
// In unordered mode, results are emitted as they complete for maximum throughput.
// Errors are wrapped in StreamError with original item context.
func (a *AsyncMapper[In, Out]) Process(ctx context.Context, in <-chan Result[In]) <-chan Result[Out] {
	if a.ordered {
		return a.processOrdered(ctx, in)
	}
	return a.processUnordered(ctx, in)
}

// processUnordered handles concurrent processing without order preservation.
// Results are emitted as they complete for maximum throughput.
func (a *AsyncMapper[In, Out]) processUnordered(ctx context.Context, in <-chan Result[In]) <-chan Result[Out] {
	out := make(chan Result[Out])

	go func() {
		defer close(out)

		// Create work channel for distributing to workers
		work := make(chan Result[In], a.workers)

		// Start workers
		var wg sync.WaitGroup
		for i := 0; i < a.workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for item := range work {
					if item.IsError() {
						// Pass through errors unchanged
						select {
						case out <- Result[Out]{err: &StreamError[Out]{
							Item:          *new(Out), // zero value
							Err:           item.Error(),
							ProcessorName: a.name,
							Timestamp:     item.Error().Timestamp,
						}}:
						case <-ctx.Done():
							return
						}
						continue
					}

					// Process the item
					result, err := a.fn(ctx, item.Value())
					if err != nil {
						select {
						case out <- NewError(result, err, a.name):
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
		}

		// Feed work to workers
		go func() {
			defer close(work)
			for item := range in {
				select {
				case work <- item:
				case <-ctx.Done():
					return
				}
			}
		}()

		// Wait for all workers to complete
		wg.Wait()
	}()

	return out
}

// processOrdered handles concurrent processing with order preservation.
// Uses sequence numbers and a reorder buffer to maintain input order.
func (a *AsyncMapper[In, Out]) processOrdered(ctx context.Context, in <-chan Result[In]) <-chan Result[Out] {
	sequenced := make(chan sequencedItem[Result[In]], a.workers)
	results := make(chan sequencedItem[Result[Out]], a.workers)
	out := make(chan Result[Out])

	// Sequence incoming items
	go func() {
		defer close(sequenced)
		var seq uint64
		for item := range in {
			select {
			case sequenced <- sequencedItem[Result[In]]{item: item, seq: seq}:
				seq++
			case <-ctx.Done():
				return
			}
		}
	}()

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < a.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for seqItem := range sequenced {
				var result Result[Out]

				if seqItem.item.IsError() {
					// Pass through errors with original sequence
					result = Result[Out]{err: &StreamError[Out]{
						Item:          *new(Out), // zero value
						Err:           seqItem.item.Error(),
						ProcessorName: a.name,
						Timestamp:     seqItem.item.Error().Timestamp,
					}}
				} else {
					// Process the item
					processed, err := a.fn(ctx, seqItem.item.Value())
					if err != nil {
						result = NewError(processed, err, a.name)
					} else {
						result = NewSuccess(processed)
					}
				}

				select {
				case results <- sequencedItem[Result[Out]]{item: result, seq: seqItem.seq}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// Close results when workers finish
	go func() {
		wg.Wait()
		close(results)
	}()

	// Reorder results and emit in sequence
	go func() {
		defer close(out)

		pending := make(map[uint64]sequencedItem[Result[Out]])
		var nextSeq uint64

		for result := range results {
			pending[result.seq] = result

			// Emit any sequential results starting from nextSeq
			for {
				if item, ok := pending[nextSeq]; ok {
					delete(pending, nextSeq)
					nextSeq++

					select {
					case out <- item.item:
					case <-ctx.Done():
						return
					}
				} else {
					break
				}
			}
		}

		// Emit any remaining results (shouldn't happen in normal operation)
		seq := nextSeq
		for len(pending) > 0 {
			if item, ok := pending[seq]; ok {
				delete(pending, seq)
				select {
				case out <- item.item:
				case <-ctx.Done():
					return
				}
			}
			seq++
		}
	}()

	return out
}

// Name returns the processor name for debugging and monitoring.
func (a *AsyncMapper[In, Out]) Name() string {
	return a.name
}
