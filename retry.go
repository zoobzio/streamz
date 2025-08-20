package streamz

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"
)

// Retry wraps any processor and automatically retries failed operations using
// exponential backoff with jitter. It provides resilient stream processing
// by handling transient failures gracefully while maintaining stream flow.
//
// The processor attempts to process each item multiple times before giving up.
// Failed items are skipped after exhausting retry attempts, allowing the stream
// to continue processing subsequent items.
//
// Key features:
//   - Exponential backoff with configurable base and max delays.
//   - Optional jitter to prevent thundering herd problems.
//   - Custom error classification for retry decisions.
//   - Context-aware delays that respect cancellation.
//   - Comprehensive error reporting via callbacks.
//
// When to use:
//   - Wrapping processors that call external services.
//   - Handling network timeouts and transient failures.
//   - Processing unreliable data sources.
//   - Adding resilience to critical pipeline stages.
//   - Smoothing over temporary resource constraints.
//
// Example:
//
//	// Basic retry with defaults (3 attempts, 100ms base delay).
//	processor := streamz.NewAsyncMapper(func(ctx context.Context, id string) (User, error) {
//		return fetchUserFromAPI(ctx, id) // May fail transiently.
//	}).WithWorkers(5)
//
//	resilient := streamz.NewRetry(processor, clock.Real)
//
//	// Custom retry configuration.
//	resilient := streamz.NewRetry(processor, clock.Real).
//		MaxAttempts(5).
//		BaseDelay(200*time.Millisecond).
//		MaxDelay(10*time.Second).
//		WithJitter(true).
//		WithName("api-retry")
//
//	// Custom error classification.
//	resilient := streamz.NewRetry(processor, clock.Real).
//		OnError(func(err error, attempt int) bool {
//			// Only retry on specific errors.
//			return strings.Contains(err.Error(), "timeout") ||
//			       strings.Contains(err.Error(), "connection refused")
//		})
//
//	results := resilient.Process(ctx, userIDs)
//	for user := range results {
//		// Successfully processed users (failed ones are skipped).
//		fmt.Printf("User: %+v\n", user)
//	}
//
// Performance characteristics:
//   - Minimal overhead for successful operations.
//   - Memory usage scales with retry attempts (temporary buffers).
//   - Latency increases exponentially with retry attempts.
//   - Throughput decreases proportionally to failure rate.
type Retry[T any] struct { //nolint:govet // logical field grouping preferred over memory optimization
	processor   Processor[T, T]
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
	withJitter  bool
	name        string
	onError     func(error, int) bool // Custom retry logic: (error, attempt) -> shouldRetry.
	clock       Clock
}

// NewRetry creates a processor that wraps another processor with retry logic.
// The wrapped processor will be retried on failures using exponential backoff.
//
// Default configuration:
//   - MaxAttempts: 3.
//   - BaseDelay: 100ms.
//   - MaxDelay: 30s.
//   - WithJitter: true.
//   - Name: "retry".
//
// Use the fluent API to customize retry behavior for your specific use case.
//
// Parameters:
//   - processor: The processor to wrap with retry logic.
//   - clock: Clock interface for time operations
//
// Returns a new Retry processor with fluent configuration methods.
func NewRetry[T any](processor Processor[T, T], clock Clock) *Retry[T] {
	return &Retry[T]{
		processor:   processor,
		clock:       clock,
		maxAttempts: 3,
		baseDelay:   100 * time.Millisecond,
		maxDelay:    30 * time.Second,
		withJitter:  true,
		name:        "retry",
	}
}

// MaxAttempts sets the maximum number of retry attempts.
// The processor will try up to this many times before giving up.
// Includes the initial attempt, so MaxAttempts(3) means 1 initial + 2 retries.
func (r *Retry[T]) MaxAttempts(attempts int) *Retry[T] {
	if attempts < 1 {
		attempts = 1
	}
	r.maxAttempts = attempts
	return r
}

// BaseDelay sets the base delay for exponential backoff.
// The actual delay for attempt N is: baseDelay * 2^(N-1).
// For example, with 100ms base: 100ms, 200ms, 400ms, 800ms...
func (r *Retry[T]) BaseDelay(delay time.Duration) *Retry[T] {
	if delay < 0 {
		delay = 0
	}
	r.baseDelay = delay
	return r
}

// MaxDelay sets the maximum delay between retry attempts.
// Exponential backoff will be capped at this value to prevent
// extremely long delays for high retry counts.
func (r *Retry[T]) MaxDelay(delay time.Duration) *Retry[T] {
	if delay < 0 {
		delay = 0
	}
	r.maxDelay = delay
	return r
}

// WithJitter enables or disables jitter in retry delays.
// Jitter adds randomness to delays to prevent thundering herd
// problems when many instances retry simultaneously.
// When enabled, delays are randomized between 50% and 100% of calculated delay.
func (r *Retry[T]) WithJitter(enabled bool) *Retry[T] {
	r.withJitter = enabled
	return r
}

// WithName sets a custom name for this processor.
// Useful for monitoring and debugging complex pipelines.
func (r *Retry[T]) WithName(name string) *Retry[T] {
	r.name = name
	return r
}

// OnError sets a custom error classification function.
// The function receives the error and attempt number, and should return
// true if the operation should be retried, false otherwise.
// If not set, all errors are considered retryable.
//
// Example:
//
//	retry.OnError(func(err error, attempt int) bool {
//		// Don't retry after 2 attempts for auth errors.
//		if strings.Contains(err.Error(), "unauthorized") && attempt >= 2 {
//			return false
//		}
//		// Always retry timeout errors.
//		return strings.Contains(err.Error(), "timeout")
//	})
func (r *Retry[T]) OnError(fn func(error, int) bool) *Retry[T] {
	r.onError = fn
	return r
}

// Process wraps the underlying processor with retry logic.
// Items that fail after all retry attempts are skipped to maintain stream flow.
func (r *Retry[T]) Process(ctx context.Context, in <-chan T) <-chan T {
	out := make(chan T)

	go func() {
		defer close(out)

		// Create a single-item processor for retry logic.
		for {
			select {
			case item, ok := <-in:
				if !ok {
					return
				}

				// Attempt to process the item with retries.
				if result, success := r.processWithRetry(ctx, item); success {
					select {
					case out <- result:
					case <-ctx.Done():
						return
					}
				}
				// If all retries failed, item is skipped.

			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

// processWithRetry handles the retry logic for a single item.
// Returns the result and whether processing was successful.
func (r *Retry[T]) processWithRetry(ctx context.Context, item T) (T, bool) {
	var lastErr error

	for attempt := 1; attempt <= r.maxAttempts; attempt++ {
		// Add delay before retry attempts (not on first attempt).
		if attempt > 1 {
			delay := r.calculateDelay(attempt - 1)
			select {
			case <-r.clock.After(delay):
			case <-ctx.Done():
				var zero T
				return zero, false
			}
		}

		// Process item using underlying processor.
		input := make(chan T, 1)
		input <- item
		close(input)

		output := r.processor.Process(ctx, input)

		// Check if we get a result.
		select {
		case result, ok := <-output:
			if ok {
				// Success! Return the result.
				return result, true
			}
			// Channel closed without result indicates failure.
			lastErr = fmt.Errorf("processor closed without result")

		case <-r.clock.After(100 * time.Millisecond): // Timeout waiting for result.
			lastErr = fmt.Errorf("processor timeout")

		case <-ctx.Done():
			// Context canceled, stop retrying.
			var zero T
			return zero, false
		}

		// Check if we should retry this error.
		if r.onError != nil && !r.onError(lastErr, attempt) {
			break
		}

		// Otherwise continue retrying.
	}

	// All retries exhausted.
	var zero T
	return zero, false
}

// calculateDelay computes the delay for a given retry attempt using exponential backoff.
func (r *Retry[T]) calculateDelay(attempt int) time.Duration {
	// Exponential backoff: baseDelay * 2^attempt.
	delay := float64(r.baseDelay) * math.Pow(2, float64(attempt))

	// Cap at maximum delay.
	if time.Duration(delay) > r.maxDelay {
		delay = float64(r.maxDelay)
	}

	// Add jitter if enabled (50% to 100% of calculated delay).
	if r.withJitter {
		// Generate cryptographically secure random jitter between 0.5 and 1.0.
		n, err := crand.Int(crand.Reader, big.NewInt(500))
		if err != nil {
			// Fallback to fixed value on error.
			n = big.NewInt(250)
		}
		jitter := 0.5 + float64(n.Int64())/1000.0 // 0.5 to 1.0.
		delay *= jitter
	}

	return time.Duration(delay)
}

// Name returns the processor name for debugging and monitoring.
func (r *Retry[T]) Name() string {
	return r.name
}

// isRetryableError determines if an error should trigger a retry.
// This is a basic implementation that considers most errors retryable.
// In practice, you'd customize this via OnError() for specific error types.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	// Common retryable error patterns.
	retryablePatterns := []string{
		"timeout",
		"connection refused",
		"connection reset",
		"network",
		"temporary",
		"unavailable",
		"rate limit",
		"too many requests",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	// Non-retryable error patterns.
	nonRetryablePatterns := []string{
		"authentication failed",
		"unauthorized",
		"forbidden",
		"not found",
		"invalid",
		"malformed",
	}

	for _, pattern := range nonRetryablePatterns {
		if strings.Contains(errStr, pattern) {
			return false
		}
	}

	// Default: most errors are retryable.
	return true
}
