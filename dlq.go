package streamz

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// DLQItem represents an item that failed processing and was sent to the DLQ.
type DLQItem[T any] struct { //nolint:govet // logical field grouping preferred over memory optimization
	Item      T                      // The original item that failed.
	Error     error                  // The error that caused the failure.
	Timestamp time.Time              // When the failure occurred.
	Attempts  int                    // Number of processing attempts.
	Metadata  map[string]interface{} // Additional context.
}

// DLQHandler processes items that have been sent to the dead letter queue.
type DLQHandler[T any] func(ctx context.Context, item DLQItem[T])

// DeadLetterQueue captures failed items for later analysis or retry.
// It wraps a processor and catches items that fail processing, sending
// them to a dead letter queue for inspection, logging, or retry.
//
// The DLQ can be configured to:
//   - Retry failed items with configurable attempts.
//   - Store failed items for later processing.
//   - Alert on failures.
//   - Provide statistics on failure rates.
//
// Key features:
//   - Configurable retry attempts before sending to DLQ.
//   - Custom error classification.
//   - Failure callbacks for monitoring.
//   - Statistics tracking.
//   - Option to continue or halt on failures.
//
// When to use:
//   - When you need to handle processing failures gracefully.
//   - For auditing failed items.
//   - To implement retry logic with eventual give-up.
//   - For monitoring and alerting on failures.
//   - To prevent losing items due to transient failures.
//
// Example:
//
//	// Basic DLQ with retry.
//	dlq := streamz.NewDeadLetterQueue(processor).
//		MaxRetries(3).
//		OnFailure(func(item DLQItem[Order]) {
//			log.Error("Order failed", "id", item.Item.ID, "error", item.Error)
//			storeInDatabase(item) // Save for manual review.
//		})
//
//	// Process with DLQ protection.
//	protected := dlq.Process(ctx, orders)
//
//	// Access failed items.
//	for failed := range dlq.FailedItems() {
//		handleFailedOrder(failed)
//	}
//
// Performance characteristics:
//   - Minimal overhead for successful items.
//   - Additional goroutine for managing failed items.
//   - Memory usage proportional to failed item count.
type DeadLetterQueue[T any] struct { //nolint:govet // logical field grouping preferred over memory optimization
	processor       Processor[T, T]
	name            string
	maxRetries      int
	retryDelay      time.Duration
	continueOnError bool
	clock           Clock

	// Error classification.
	shouldRetry func(error) bool

	// Failed items channel.
	failedItems      chan DLQItem[T]
	failedBufferSize int

	// Callbacks.
	onFailure DLQHandler[T]
	onRetry   func(item T, attempt int, err error)

	// Statistics.
	processed atomic.Int64
	succeeded atomic.Int64
	failed    atomic.Int64
	retried   atomic.Int64

	// State management.
	mutex  sync.RWMutex
	closed bool
}

// NewDeadLetterQueue creates a DLQ wrapper around a processor.
// Failed items are captured and can be accessed via the FailedItems channel.
//
// Default configuration:
//   - MaxRetries: 0 (no retries).
//   - ContinueOnError: true.
//   - FailedBufferSize: 100.
//   - Name: "dlq".
//
// Parameters:
//   - processor: The processor to wrap with DLQ functionality.
//   - clock: Clock interface for time operations
//
// Returns a new DeadLetterQueue with fluent configuration methods.
func NewDeadLetterQueue[T any](processor Processor[T, T], clock Clock) *DeadLetterQueue[T] {
	dlq := &DeadLetterQueue[T]{
		processor:        processor,
		name:             "dlq",
		maxRetries:       0,
		retryDelay:       time.Second,
		continueOnError:  true,
		failedBufferSize: 100,
		clock:            clock,
		shouldRetry: func(_ error) bool {
			return true // Default: retry all errors.
		},
	}

	// Initialize failed items channel.
	dlq.failedItems = make(chan DLQItem[T], dlq.failedBufferSize)

	return dlq
}

// MaxRetries sets the maximum number of retry attempts before sending to DLQ.
func (dlq *DeadLetterQueue[T]) MaxRetries(attempts int) *DeadLetterQueue[T] {
	if attempts < 0 {
		attempts = 0
	}
	dlq.maxRetries = attempts
	return dlq
}

// RetryDelay sets the delay between retry attempts.
func (dlq *DeadLetterQueue[T]) RetryDelay(delay time.Duration) *DeadLetterQueue[T] {
	if delay < 0 {
		delay = 0
	}
	dlq.retryDelay = delay
	return dlq
}

// ContinueOnError determines whether processing continues after failures.
// If false, the pipeline stops on first failure.
func (dlq *DeadLetterQueue[T]) ContinueOnError(cont bool) *DeadLetterQueue[T] {
	dlq.continueOnError = cont
	return dlq
}

// WithFailedBufferSize sets the buffer size for the failed items channel.
func (dlq *DeadLetterQueue[T]) WithFailedBufferSize(size int) *DeadLetterQueue[T] {
	if size < 1 {
		size = 1
	}
	dlq.failedBufferSize = size
	// Recreate channel with new size.
	dlq.mutex.Lock()
	close(dlq.failedItems)
	dlq.failedItems = make(chan DLQItem[T], size)
	dlq.mutex.Unlock()
	return dlq
}

// ShouldRetry sets a function to determine if an error should be retried.
func (dlq *DeadLetterQueue[T]) ShouldRetry(fn func(error) bool) *DeadLetterQueue[T] {
	if fn != nil {
		dlq.shouldRetry = fn
	}
	return dlq
}

// OnFailure sets a callback for items sent to the DLQ.
func (dlq *DeadLetterQueue[T]) OnFailure(handler DLQHandler[T]) *DeadLetterQueue[T] {
	dlq.onFailure = handler
	return dlq
}

// OnRetry sets a callback invoked before each retry attempt.
func (dlq *DeadLetterQueue[T]) OnRetry(fn func(item T, attempt int, err error)) *DeadLetterQueue[T] {
	dlq.onRetry = fn
	return dlq
}

// WithName sets a custom name for this processor.
func (dlq *DeadLetterQueue[T]) WithName(name string) *DeadLetterQueue[T] {
	dlq.name = name
	return dlq
}

// Process implements the Processor interface with DLQ functionality.
func (dlq *DeadLetterQueue[T]) Process(ctx context.Context, in <-chan T) <-chan T {
	out := make(chan T)

	go func() {
		defer close(out)
		defer dlq.closeFailedItems()

		// Process item.s with retry logic.
		for {
			select {
			case item, ok := <-in:
				if !ok {
					return
				}

				dlq.processed.Add(1)

				// Try processing with retries.
				success, err := dlq.processWithRetry(ctx, item, out)

				if success {
					dlq.succeeded.Add(1)
				} else {
					dlq.failed.Add(1)

					// Send to DLQ.
					dlqItem := DLQItem[T]{
						Item:      item,
						Error:     err,
						Timestamp: dlq.clock.Now(),
						Attempts:  dlq.maxRetries + 1,
						Metadata:  make(map[string]interface{}),
					}

					// Call failure handler.
					if dlq.onFailure != nil {
						dlq.onFailure(ctx, dlqItem)
					}

					// Send to failed items channel (check if not closed).
					dlq.mutex.RLock()
					if !dlq.closed {
						dlq.mutex.RUnlock()
						select {
						case dlq.failedItems <- dlqItem:
						case <-ctx.Done():
							return
						}
					} else {
						dlq.mutex.RUnlock()
					}

					// Stop processing if configured.
					if !dlq.continueOnError {
						return
					}
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

// processWithRetry attempts to process an item with retries.
func (dlq *DeadLetterQueue[T]) processWithRetry(ctx context.Context, item T, out chan<- T) (bool, error) {
	var lastErr error

	for attempt := 0; attempt <= dlq.maxRetries; attempt++ {
		// Create single-item channel for processor.
		input := make(chan T, 1)
		input <- item
		close(input)

		// Process item.
		output := dlq.processor.Process(ctx, input)

		// Check result.
		select {
		case result, ok := <-output:
			if ok {
				// Success - send to output.
				select {
				case out <- result:
					return true, nil
				case <-ctx.Done():
					return false, ctx.Err()
				}
			}
			// Channel closed without result - treat as error.
			lastErr = fmt.Errorf("processor failed to produce output")

		case <-dlq.clock.After(30 * time.Second): // Timeout.
			lastErr = fmt.Errorf("processor timeout")

		case <-ctx.Done():
			return false, ctx.Err()
		}

		// Check if we should retry.
		if attempt < dlq.maxRetries && dlq.shouldRetry(lastErr) {
			dlq.retried.Add(1)

			// Call retry callback.
			if dlq.onRetry != nil {
				dlq.onRetry(item, attempt+1, lastErr)
			}

			// Wait before retry.
			if dlq.retryDelay > 0 {
				select {
				case <-dlq.clock.After(dlq.retryDelay):
				case <-ctx.Done():
					return false, ctx.Err()
				}
			}
		} else {
			// No more retries or error not retryable.
			break
		}
	}

	return false, lastErr
}

// FailedItems returns a channel of items that failed processing.
// This channel is closed when the processor completes.
func (dlq *DeadLetterQueue[T]) FailedItems() <-chan DLQItem[T] {
	return dlq.failedItems
}

// closeFailedItems closes the failed items channel.
func (dlq *DeadLetterQueue[T]) closeFailedItems() {
	dlq.mutex.Lock()
	defer dlq.mutex.Unlock()

	if !dlq.closed {
		close(dlq.failedItems)
		dlq.closed = true
	}
}

// GetStats returns current DLQ statistics.
func (dlq *DeadLetterQueue[T]) GetStats() DLQStats {
	return DLQStats{
		Processed: dlq.processed.Load(),
		Succeeded: dlq.succeeded.Load(),
		Failed:    dlq.failed.Load(),
		Retried:   dlq.retried.Load(),
	}
}

// Name returns the processor name for debugging and monitoring.
func (dlq *DeadLetterQueue[T]) Name() string {
	return dlq.name
}

// DLQStats contains statistics about DLQ operations.
type DLQStats struct { //nolint:govet // logical field grouping preferred over memory optimization
	Processed int64 // Total items processed.
	Succeeded int64 // Items successfully processed.
	Failed    int64 // Items sent to DLQ.
	Retried   int64 // Total retry attempts.
}

// SuccessRate returns the percentage of successful items.
func (s DLQStats) SuccessRate() float64 {
	if s.Processed == 0 {
		return 0
	}
	return float64(s.Succeeded) / float64(s.Processed) * 100
}

// FailureRate returns the percentage of failed items.
func (s DLQStats) FailureRate() float64 {
	if s.Processed == 0 {
		return 0
	}
	return float64(s.Failed) / float64(s.Processed) * 100
}
