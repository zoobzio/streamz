# Retry Connector Implementation Plan

## Overview

The Retry connector will handle transient failures with configurable retry strategies while preserving Result[T] metadata and integrating with the existing DLQ for permanent failures. This implementation follows existing streamz patterns for consistency and maintains simplicity over architectural complexity.

## Struct Definition

```go
// Retry implements configurable retry logic for transient failures.
// Successful results pass through unchanged. Failed results are retried
// according to strategy until max attempts reached, then sent to DLQ.
//
// Key behaviors:
// - Preserves all Result[T] metadata through retry attempts
// - Updates MetadataRetryCount on each attempt
// - Integrates with DLQ for permanent failures
// - Uses strategy pattern for different backoff algorithms
// - Thread-safe for concurrent use
type Retry[T any] struct {
    name        string
    maxAttempts int
    strategy    RetryStrategy
    dlq         *DeadLetterQueue[T]
    clock       Clock
}

// RetryStrategy defines the interface for retry backoff calculations.
type RetryStrategy interface {
    // NextDelay calculates the delay before the next retry attempt.
    // attempt is 1-indexed (first retry is attempt 1)
    NextDelay(attempt int) time.Duration
    
    // ShouldRetry determines if an error should be retried.
    // Some errors (like validation failures) should not be retried.
    ShouldRetry(err error) bool
}
```

## Retry Strategies

### 1. Exponential Backoff Strategy

```go
// ExponentialBackoff implements exponential backoff with optional jitter.
type ExponentialBackoff struct {
    baseDelay   time.Duration // Initial delay (e.g., 100ms)
    maxDelay    time.Duration // Maximum delay cap (e.g., 30s)
    multiplier  float64       // Backoff multiplier (e.g., 2.0)
    jitter      float64       // Jitter percentage [0.0, 1.0]
    retryableErrs []error     // List of retryable error types
}

func (eb *ExponentialBackoff) NextDelay(attempt int) time.Duration {
    // delay = baseDelay * (multiplier ^ (attempt - 1))
    // Apply jitter: delay ± (jitter * delay)
    // Cap at maxDelay
}

func (eb *ExponentialBackoff) ShouldRetry(err error) bool {
    // Check if error type is in retryableErrs list
    // Default: retry all errors except validation types
}
```

### 2. Fixed Delay Strategy

```go
// FixedDelay implements simple fixed delay between retries.
type FixedDelay struct {
    delay         time.Duration
    retryableErrs []error
}

func (fd *FixedDelay) NextDelay(attempt int) time.Duration {
    return fd.delay
}

func (fd *FixedDelay) ShouldRetry(err error) bool {
    // Check retryableErrs list
}
```

### 3. Linear Backoff Strategy

```go
// LinearBackoff implements linear increase in delay.
type LinearBackoff struct {
    baseDelay     time.Duration
    increment     time.Duration 
    maxDelay      time.Duration
    retryableErrs []error
}

func (lb *LinearBackoff) NextDelay(attempt int) time.Duration {
    // delay = baseDelay + (increment * (attempt - 1))
    // Cap at maxDelay
}
```

## Constructor Functions

```go
// NewRetry creates a new Retry processor with the specified strategy.
func NewRetry[T any](maxAttempts int, strategy RetryStrategy, dlq *DeadLetterQueue[T], clock Clock) *Retry[T] {
    return &Retry[T]{
        name:        "retry",
        maxAttempts: maxAttempts,
        strategy:    strategy,
        dlq:         dlq,
        clock:       clock,
    }
}

// NewExponentialBackoff creates exponential backoff strategy.
func NewExponentialBackoff(baseDelay, maxDelay time.Duration, multiplier, jitter float64) *ExponentialBackoff {
    return &ExponentialBackoff{
        baseDelay:  baseDelay,
        maxDelay:   maxDelay,
        multiplier: multiplier,
        jitter:     jitter,
        retryableErrs: []error{
            // Default retryable errors (network, timeout, etc.)
        },
    }
}

// NewFixedDelay creates fixed delay strategy.
func NewFixedDelay(delay time.Duration) *FixedDelay {
    return &FixedDelay{
        delay: delay,
        retryableErrs: []error{
            // Default retryable errors
        },
    }
}
```

## Core Processing Logic

```go
// Process implements the retry logic with DLQ integration.
func (r *Retry[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan Result[T] {
    out := make(chan Result[T])
    
    go func() {
        defer close(out)
        
        for {
            select {
            case <-ctx.Done():
                return
            case result, ok := <-in:
                if !ok {
                    return
                }
                
                // Success results pass through unchanged
                if result.IsSuccess() {
                    select {
                    case out <- result:
                    case <-ctx.Done():
                        return
                    }
                    continue
                }
                
                // Process failure with retry logic
                r.processFailure(ctx, result, out)
            }
        }
    }()
    
    return out
}

// processFailure handles retry logic for failed results.
func (r *Retry[T]) processFailure(ctx context.Context, result Result[T], out chan<- Result[T]) {
    streamErr := result.Error()
    
    // Check if error should be retried
    if !r.strategy.ShouldRetry(streamErr.Err) {
        // Send to DLQ immediately for non-retryable errors
        r.sendToDLQ(ctx, result)
        return
    }
    
    // Get current retry count from metadata
    retryCount := r.getCurrentRetryCount(result)
    
    // Check if we've exceeded max attempts
    if retryCount >= r.maxAttempts {
        // Send to DLQ after exhausting retries
        r.sendToDLQ(ctx, result)
        return
    }
    
    // Calculate delay for next attempt
    nextAttempt := retryCount + 1
    delay := r.strategy.NextDelay(nextAttempt)
    
    // Wait for backoff delay (respecting context cancellation)
    timer := r.clock.After(delay)
    select {
    case <-ctx.Done():
        return
    case <-timer:
        // Proceed with retry
    }
    
    // Create retry result with updated metadata
    retryResult := r.createRetryResult(result, nextAttempt)
    
    select {
    case out <- retryResult:
    case <-ctx.Done():
        return
    }
}
```

## Metadata Management

```go
// getCurrentRetryCount extracts retry count from Result metadata.
func (r *Retry[T]) getCurrentRetryCount(result Result[T]) int {
    if retryCount, found, err := result.GetIntMetadata(MetadataRetryCount); found && err == nil {
        return retryCount
    }
    return 0 // First attempt
}

// createRetryResult creates a new Result with updated retry metadata.
func (r *Retry[T]) createRetryResult(original Result[T], attemptNumber int) Result[T] {
    // Preserve original error and value
    streamErr := original.Error()
    retryResult := NewError(streamErr.Item, streamErr.Err, r.name)
    
    // Preserve all existing metadata
    for _, key := range original.MetadataKeys() {
        if value, exists := original.GetMetadata(key); exists {
            retryResult = retryResult.WithMetadata(key, value)
        }
    }
    
    // Update retry count
    retryResult = retryResult.WithMetadata(MetadataRetryCount, attemptNumber)
    
    // Add processor metadata
    retryResult = retryResult.WithMetadata(MetadataProcessor, r.name)
    retryResult = retryResult.WithMetadata(MetadataTimestamp, r.clock.Now())
    
    return retryResult
}

// sendToDLQ routes permanently failed items to the dead letter queue.
func (r *Retry[T]) sendToDLQ(ctx context.Context, result Result[T]) {
    // Create single-item channel for DLQ
    dlqInput := make(chan Result[T], 1)
    dlqInput <- result
    close(dlqInput)
    
    // Process through DLQ (only need failures channel)
    _, failures := r.dlq.Process(ctx, dlqInput)
    
    // Consume DLQ output to prevent goroutine leak
    go func() {
        for range failures {
            // DLQ handles logging and metrics
        }
    }()
}
```

## Error Types and Classification

```go
// RetryableError wraps errors that should be retried.
type RetryableError struct {
    Underlying error
    Reason     string
}

func (re *RetryableError) Error() string {
    return fmt.Sprintf("retryable error (%s): %v", re.Reason, re.Underlying)
}

func (re *RetryableError) Unwrap() error {
    return re.Underlying
}

// Common error classification
var (
    // Network-related errors (retryable)
    ErrNetworkTimeout = &RetryableError{Underlying: errors.New("network timeout"), Reason: "network"}
    ErrConnectionReset = &RetryableError{Underlying: errors.New("connection reset"), Reason: "network"}
    
    // Service errors (retryable)
    ErrServiceUnavailable = &RetryableError{Underlying: errors.New("service unavailable"), Reason: "service"}
    ErrRateLimited = &RetryableError{Underlying: errors.New("rate limited"), Reason: "rate_limit"}
    
    // Validation errors (non-retryable)
    ErrInvalidInput = errors.New("invalid input")
    ErrValidationFailed = errors.New("validation failed")
)

// IsRetryableError checks if an error should be retried.
func IsRetryableError(err error) bool {
    var retryableErr *RetryableError
    return errors.As(err, &retryableErr)
}
```

## Testing Approach

### Unit Test Structure

```go
// retry_test.go focuses on isolated retry logic testing

func TestRetry_SuccessPassesThrough(t *testing.T) {
    // Test: Success results pass through without delay
}

func TestRetry_ExponentialBackoff(t *testing.T) {
    // Test: Verify exponential backoff timing with fake clock
}

func TestRetry_MaxAttemptsExceeded(t *testing.T) {
    // Test: Items sent to DLQ after max attempts
}

func TestRetry_NonRetryableErrors(t *testing.T) {
    // Test: Validation errors go directly to DLQ
}

func TestRetry_MetadataPreservation(t *testing.T) {
    // Test: All metadata preserved through retry attempts
    // Test: Retry count metadata updated correctly
}

func TestRetry_ContextCancellation(t *testing.T) {
    // Test: Graceful shutdown during backoff delays
}

func TestRetry_ConcurrentProcessing(t *testing.T) {
    // Test: Thread safety with multiple goroutines
}
```

### Strategy Testing

```go
// retry_strategy_test.go tests individual strategies

func TestExponentialBackoff_DelayCalculation(t *testing.T) {
    // Test: Verify exponential delay formula
    // Test: Jitter application
    // Test: Max delay capping
}

func TestExponentialBackoff_JitterDistribution(t *testing.T) {
    // Test: Jitter produces values in expected range
}

func TestFixedDelay_ConsistentTiming(t *testing.T) {
    // Test: Fixed delay returns constant duration
}

func TestLinearBackoff_LinearIncrease(t *testing.T) {
    // Test: Verify linear delay progression
}
```

### Integration Testing

```go
// retry_integration_test.go tests with real components

func TestRetry_WithRealDLQ(t *testing.T) {
    // Test: Integration with actual DLQ processor
}

func TestRetry_InPipeline(t *testing.T) {
    // Test: Retry in multi-processor pipeline
}
```

## Usage Examples

### Basic Usage

```go
// Create DLQ for permanent failures
dlq := streamz.NewDeadLetterQueue[Order](streamz.RealClock)

// Create exponential backoff strategy
strategy := streamz.NewExponentialBackoff(
    100*time.Millisecond, // base delay
    30*time.Second,       // max delay
    2.0,                  // multiplier
    0.1,                  // 10% jitter
)

// Create retry processor
retry := streamz.NewRetry(3, strategy, dlq, streamz.RealClock)

// Process orders with retry logic
retried := retry.Process(ctx, orders)
successes, permanentFailures := dlq.Process(ctx, retried)

// Handle results
go func() {
    for success := range successes {
        processOrder(success.Value())
    }
}()

go func() {
    for failure := range permanentFailures {
        logPermanentFailure(failure.Error())
    }
}()
```

### Custom Strategy

```go
// Custom strategy for database operations
type DatabaseRetryStrategy struct {
    maxDelay time.Duration
}

func (d *DatabaseRetryStrategy) NextDelay(attempt int) time.Duration {
    // Fibonacci-based backoff for database retries
    return fibonacciDelay(attempt, d.maxDelay)
}

func (d *DatabaseRetryStrategy) ShouldRetry(err error) bool {
    // Only retry connection and timeout errors
    return isDatabaseConnectionError(err) || isDatabaseTimeoutError(err)
}
```

## Configuration and Monitoring

### Retry Metrics

```go
// RetryMetrics captures retry behavior for monitoring
type RetryMetrics struct {
    TotalAttempts   uint64
    SuccessfulRetries uint64
    FailedRetries   uint64
    DLQSent         uint64
    AverageDelay    time.Duration
}

// GetMetrics returns current retry statistics
func (r *Retry[T]) GetMetrics() RetryMetrics {
    // Implementation with atomic counters
}
```

### Configuration Validation

```go
// ValidateConfig ensures retry configuration is sensible
func ValidateRetryConfig(maxAttempts int, strategy RetryStrategy) error {
    if maxAttempts < 1 {
        return errors.New("maxAttempts must be >= 1")
    }
    if maxAttempts > 10 {
        return errors.New("maxAttempts > 10 may cause excessive delays")
    }
    // Additional validation for strategy parameters
}
```

## Implementation Notes

### Complexity Justification

**Necessary Complexity:**
- Strategy pattern: Required for different backoff algorithms in production
- Metadata preservation: Essential for debugging retry chains
- DLQ integration: Prevents infinite retry loops
- Context cancellation: Required for graceful shutdown
- Thread safety: Concurrent access is expected in stream processing

**Rejected Complexity:**
- Circuit breaker integration: Separate concern, different failure mode
- Retry batching: Complicates error handling without clear benefit
- Custom schedulers: Time.After() sufficient for retry delays
- Pluggable DLQ: Single DLQ implementation meets all requirements

### Error Context Preservation

The implementation preserves complete error context through the retry chain:
1. Original item that caused failure
2. Root cause error with full stack
3. Processing timestamp for each attempt
4. Retry count for monitoring
5. All upstream metadata (windows, batching, etc.)

This enables debugging retry loops and understanding failure patterns without losing critical context.

### Performance Considerations

- Uses channel operations instead of timers for backoff (eliminates goroutine per retry)
- Preserves metadata through copy rather than mutation (thread-safe)
- Integrates with existing DLQ rather than reimplementing failure handling
- Minimal allocations during retry processing

## Files to Implement

1. `retry.go` - Main Retry processor and strategies
2. `retry_test.go` - Comprehensive unit tests
3. Update `api.go` - Add RetryConfig if needed
4. Update `result.go` - Add MetadataRetryCount constant (already exists)

## Deliverables

1. Complete Retry processor implementation
2. Three retry strategies (exponential, fixed, linear)
3. Full unit test suite with >80% coverage
4. Integration with existing DLQ
5. Godoc documentation for all public APIs
6. Usage examples in godoc

The implementation follows the principle of necessary complexity - each component serves a documented requirement without adding architectural overhead for its own sake.