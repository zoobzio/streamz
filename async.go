package streamz

import (
	"context"
	"sync"
)

// AsyncMapper processes items concurrently using multiple worker goroutines while preserving
// the order of items in the output stream. This allows CPU-intensive or I/O-bound operations
// to be parallelized without losing the sequential ordering of the stream.
type AsyncMapper[In, Out any] struct {
	fn      func(context.Context, In) (Out, error)
	name    string
	workers int
}

// NewAsyncMapper creates a processor that executes transformations concurrently.
// Despite parallel execution, output order matches input order exactly, making it
// safe for order-sensitive operations while still gaining concurrency benefits.
// Use the fluent API to configure optional behavior like worker count.
//
// When to use:
//   - CPU-intensive transformations (image processing, encryption)
//   - I/O-bound operations (API calls, database queries)
//   - Parallel enrichment while maintaining sequence
//   - Speeding up independent transformations
//   - Rate-limited API calls with concurrent workers
//
// Example:
//
//	// Parallel API enrichment with default workers (runtime.NumCPU())
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
//	enriched := enricher.Process(ctx, userIDs)
//	for user := range enriched {
//		// Users appear in same order as input IDs
//		fmt.Printf("User: %+v\n", user)
//	}
//
//	// CPU-intensive processing with all cores
//	processor := streamz.NewAsyncMapper(func(ctx context.Context, img Image) (Thumbnail, error) {
//		return generateThumbnail(ctx, img)
//	}).WithWorkers(runtime.NumCPU())
//
// Parameters:
//   - fn: Transformation function that can be safely executed concurrently
//
// Returns a new AsyncMapper processor with fluent configuration.
func NewAsyncMapper[In, Out any](fn func(context.Context, In) (Out, error)) *AsyncMapper[In, Out] {
	return &AsyncMapper[In, Out]{
		workers: 4, // sensible default
		fn:      fn,
		name:    "async-mapper",
	}
}

// WithWorkers sets the number of concurrent workers.
// If not set, defaults to 4.
func (a *AsyncMapper[In, Out]) WithWorkers(workers int) *AsyncMapper[In, Out] {
	a.workers = workers
	return a
}

// WithName sets a custom name for this processor.
// If not set, defaults to "async-mapper".
func (a *AsyncMapper[In, Out]) WithName(name string) *AsyncMapper[In, Out] {
	a.name = name
	return a
}

type sequencedItem[T any] struct {
	item T
	seq  uint64
	skip bool
}

func (a *AsyncMapper[In, Out]) Process(ctx context.Context, in <-chan In) <-chan Out {
	sequenced := make(chan sequencedItem[In])
	results := make(chan sequencedItem[Out], a.workers)
	out := make(chan Out)

	go func() {
		var seq uint64
		for item := range in {
			sequenced <- sequencedItem[In]{seq: seq, item: item}
			seq++
		}
		close(sequenced)
	}()

	var wg sync.WaitGroup
	for i := 0; i < a.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for seqItem := range sequenced {
				result, err := a.fn(ctx, seqItem.item)
				select {
				case results <- sequencedItem[Out]{
					seq:  seqItem.seq,
					item: result,
					skip: err != nil,
				}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	go func() {
		defer close(out)

		pending := make(map[uint64]sequencedItem[Out])
		var nextSeq uint64

		for result := range results {
			pending[result.seq] = result

			for {
				if item, ok := pending[nextSeq]; ok {
					delete(pending, nextSeq)
					nextSeq++

					if !item.skip {
						select {
						case out <- item.item:
						case <-ctx.Done():
							return
						}
					}
				} else {
					break
				}
			}
		}

		for seq := nextSeq; seq < nextSeq+uint64(len(pending)); seq++ {
			if item, ok := pending[seq]; ok {
				if !item.skip {
					select {
					case out <- item.item:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return out
}

func (a *AsyncMapper[In, Out]) Name() string {
	return a.name
}
