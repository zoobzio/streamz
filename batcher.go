package streamz

import (
	"context"
	"time"
)

// Batcher collects items from a stream and groups them into batches based on size or time constraints.
// It emits a batch when either the maximum size is reached or the maximum latency expires,
// whichever comes first. This is useful for optimizing downstream operations that work more
// efficiently with groups of items rather than individual items.
//
// Batcher handles errors by separating them from successful batches. Errors are passed through
// immediately without being included in batches. This ensures error visibility while maintaining
// batch integrity.
//
//nolint:govet // fieldalignment: struct layout optimized for readability
type Batcher[T any] struct {
	config BatchConfig
	name   string
	clock  Clock
}

// NewBatcher creates a processor that intelligently groups items into batches.
// Batches are emitted when either the size limit is reached OR the time limit expires,
// whichever comes first. This dual-trigger approach balances throughput with latency.
//
// Key behaviors:
//   - Errors are passed through immediately without affecting batches
//   - Successful items are batched according to MaxSize and MaxLatency constraints
//   - Timeouts are treated as normal batch emissions (not errors)
//   - Memory usage is bounded by MaxSize configuration
//   - Single-goroutine pattern prevents race conditions
//
// When to use:
//   - Optimizing database writes with bulk operations
//   - Reducing API calls by batching requests
//   - Implementing micro-batching for stream processing
//   - Buffering events for periodic processing
//   - Cost optimization through batch operations
//
// Example:
//
//	// Batch up to 1000 items or 5 seconds, whichever comes first
//	batcher := streamz.NewBatcher[Event](streamz.BatchConfig{
//		MaxSize:    1000,
//		MaxLatency: 5 * time.Second,
//	}, streamz.RealClock)
//
//	batches := batcher.Process(ctx, events)
//	for result := range batches {
//		if result.IsError() {
//			log.Printf("Individual item error: %v", result.Error())
//			continue
//		}
//		batch := result.Value()
//		// Process batch of up to 1000 items
//		// Never waits more than 5 seconds
//		bulkInsert(batch)
//	}
//
//	// Optimize API calls with smart batching
//	apiBatcher := streamz.NewBatcher[Request](streamz.BatchConfig{
//		MaxSize:    100,  // API limit
//		MaxLatency: 100 * time.Millisecond, // Max acceptable delay
//	}, streamz.RealClock)
//
// Parameters:
//   - config: Batch configuration with size and latency constraints
//   - clock: Clock interface for time operations (use RealClock in production)
//
// Returns a new Batcher processor that groups items efficiently.
func NewBatcher[T any](config BatchConfig, clock Clock) *Batcher[T] {
	return &Batcher[T]{
		config: config,
		name:   "batcher",
		clock:  clock,
	}
}

// Process groups input items into batches according to the configured constraints.
// It returns a channel of Result[[]T] where successful results contain batches and
// error results contain individual item processing errors.
//
// Batching behavior:
//   - Errors pass through immediately without being batched
//   - Successful items are collected into batches
//   - Batches are emitted when MaxSize is reached OR MaxLatency expires
//   - Final partial batch is emitted when input channel closes
//   - Context cancellation stops processing immediately
//
// Memory safety:
//   - Bounded memory usage limited by MaxSize
//   - Single timer instance - no timer leaks
//   - Proper cleanup on context cancellation
func (b *Batcher[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan Result[[]T] {
	out := make(chan Result[[]T])

	go func() {
		defer close(out)

		batch := make([]T, 0, b.config.MaxSize)
		var timer Timer
		var timerC <-chan time.Time

		for {
			// Phase 1: Check timer first with higher priority
			if timerC != nil {
				select {
				case <-timerC:
					// Timer expired, emit current batch
					if len(batch) > 0 {
						select {
						case out <- NewSuccess(batch):
							// Create new batch with pre-allocated capacity
							batch = make([]T, 0, b.config.MaxSize)
						case <-ctx.Done():
							return
						}
					}
					// Clear timer references
					timer = nil
					timerC = nil
					continue // Check for more timer events before processing input
				default:
					// Timer not ready - proceed to input
				}
			}

			// Phase 2: Process input/context
			select {
			case result, ok := <-in:
				if !ok {
					// Input closed, flush pending batch if exists
					if timer != nil {
						timer.Stop()
					}
					if len(batch) > 0 {
						select {
						case out <- NewSuccess(batch):
						case <-ctx.Done():
						}
					}
					return
				}

				// Errors pass through immediately without affecting batches
				if result.IsError() {
					// Convert error from Result[T] to Result[[]T]
					errorResult := NewError(make([]T, 0), result.Error().Err, result.Error().ProcessorName)
					select {
					case out <- errorResult:
					case <-ctx.Done():
						return
					}
					continue
				}

				// Add successful item to batch
				batch = append(batch, result.Value())

				// Start timer for first item if MaxLatency is configured
				if len(batch) == 1 && b.config.MaxLatency > 0 {
					// Stop old timer if exists
					if timer != nil {
						timer.Stop()
					}
					// Create new timer (following debounce pattern for FakeClock compatibility)
					timer = b.clock.NewTimer(b.config.MaxLatency)
					timerC = timer.C()
				}

				// Emit batch if size limit reached
				if len(batch) >= b.config.MaxSize {
					// Stop timer since we're emitting now
					if timer != nil {
						timer.Stop()
						timer = nil
						timerC = nil
					}

					select {
					case out <- NewSuccess(batch):
						// Create new batch with pre-allocated capacity
						batch = make([]T, 0, b.config.MaxSize)
					case <-ctx.Done():
						return
					}
				}

			case <-timerC:
				// Timer fired during input wait - duplicate Phase 1 logic
				if len(batch) > 0 {
					select {
					case out <- NewSuccess(batch):
						// Create new batch
						batch = make([]T, 0, b.config.MaxSize)
					case <-ctx.Done():
						return
					}
				}
				// Clear timer references
				timer = nil
				timerC = nil

			case <-ctx.Done():
				if timer != nil {
					timer.Stop()
				}
				return
			}
		}
	}()

	return out
}

// Name returns the processor name for debugging and monitoring.
func (b *Batcher[T]) Name() string {
	return b.name
}
