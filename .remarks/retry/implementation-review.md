# Retry Connector Implementation Review

## Technical Assessment

Reviewed CASE's retry connector implementation plan. Found technically sound with proper integration patterns. Minor gaps identified.

## Verified Components

### 1. Strategy Pattern - CORRECT
Exponential backoff calculation correct:
- Formula: `baseDelay * (multiplier ^ (attempt - 1))`
- Jitter application reasonable
- Max delay capping present

Fixed and linear strategies straightforward.

### 2. Result[T] Integration - CORRECT
- Metadata preservation through `MetadataKeys()` iteration
- `GetIntMetadata()` for retry count retrieval
- `WithMetadata()` for immutable updates
- Thread-safe operations confirmed

### 3. DLQ Integration - PARTIAL
Integration approach works but has inefficiencies:
```go
// Current approach creates goroutine per failure
dlqInput := make(chan Result[T], 1)
dlqInput <- result
close(dlqInput)
_, failures := r.dlq.Process(ctx, dlqInput)
go func() {
    for range failures {
        // Goroutine leak potential
    }
}()
```

Better: Maintain persistent DLQ channels throughout processor lifecycle.

### 4. Clock Integration - CORRECT
Uses `clock.After()` for testable delays. Proper context cancellation handling.

## Identified Issues

### 1. Retry Logic Flaw

**Problem**: Current implementation sends failure back to output channel for retry:
```go
// Line 211-217: Creates retry result and sends to output
retryResult := r.createRetryResult(result, nextAttempt)
select {
case out <- retryResult:
```

This sends error Result[T] downstream. Downstream processors see failures.

**Should be**: Retry internally, only emit successes or permanent failures.

### 2. Error Classification Missing

`ShouldRetry()` checks against `retryableErrs []error` list, but:
- No use of `errors.Is()` or `errors.As()`
- Won't match wrapped errors
- Custom `RetryableError` type defined but not utilized

**Fix**:
```go
func (eb *ExponentialBackoff) ShouldRetry(err error) bool {
    var retryableErr *RetryableError
    return errors.As(err, &retryableErr)
}
```

### 3. Metadata Timestamp Issue

Line 251 uses `r.clock.Now()` but Clock interface doesn't have `Now()`:
```go
retryResult = retryResult.WithMetadata(MetadataTimestamp, r.clock.Now())
```

Clock from clockz only has `After()`, `AfterFunc()`, `Timer()`, `Ticker()`.

**Fix**: Use `time.Now()` or extend Clock interface.

### 4. DLQ Channel Leak

Each failure creates new goroutine (line 267). Never cleaned up. Accumulates over time.

**Fix**: Single persistent DLQ processor per Retry instance.

### 5. Missing Retry Executor

Plan describes retry logic but doesn't show actual retry execution. Needs:
- Function to execute retry attempt
- Success detection to break retry loop
- Error propagation on failure

## Missing Components

### 1. Retry Function Interface

No definition for what gets retried. Need:
```go
type RetryFunc[T any] func(item T) (T, error)
```

Or assume retry happens upstream when error flows back.

### 2. Success After Retry

No path for successful retry. If retry succeeds, how does success reach output?

Current flow: Error → Retry → Error (with updated count) → Output

Should be: Error → Retry → Execute → Success/Error → Output/DLQ

### 3. Metrics Implementation

Metrics struct defined but no actual implementation. Need atomic counters.

## Recommendations

### 1. Fix Retry Execution Model

Two approaches:

**A. External Retry** (Current approach, needs fixing):
- Retry emits error with retry metadata
- Upstream processor detects retry metadata, re-attempts
- Requires coordination protocol

**B. Internal Retry** (Recommended):
- Retry accepts `RetryFunc[T]`
- Executes function on failures
- Only emits final success or permanent failure

### 2. Fix Error Classification

Use `errors.As()` for proper error wrapping support:
```go
func IsRetryable(err error) bool {
    // Check retryable wrapper
    var retryable *RetryableError
    if errors.As(err, &retryable) {
        return true
    }
    
    // Check specific error types
    // Network errors, timeouts, etc.
    return false
}
```

### 3. Fix DLQ Integration

Maintain single DLQ processor:
```go
type Retry[T any] struct {
    // ... other fields
    dlqIn  chan Result[T]
    dlqOut <-chan Result[T]
    dlqErr <-chan Result[T]
}

// Initialize in constructor
r.dlqIn = make(chan Result[T], 1)
r.dlqOut, r.dlqErr = dlq.Process(ctx, r.dlqIn)
```

### 4. Add Execution Context

For internal retry:
```go
type RetryConfig[T any] struct {
    MaxAttempts int
    Strategy    RetryStrategy
    Execute     func(T) (T, error)  // The retry function
    DLQ         *DeadLetterQueue[T]
    Clock       Clock
}
```

## Performance Concerns

1. **Goroutine proliferation**: Each DLQ send spawns goroutine
2. **Channel allocation**: Creates channel per failure
3. **Metadata copying**: Full map copy on each retry (acceptable for correctness)

## Test Coverage Gaps

Missing tests for:
- Wrapped error handling
- Concurrent retry attempts
- DLQ goroutine cleanup
- Clock.Now() usage (will fail compilation)
- Success after retry scenario

## Verdict

**Status**: Requires revision

**Critical Issues**:
1. Retry execution model unclear/broken
2. Clock.Now() doesn't exist
3. DLQ integration causes goroutine leaks
4. Error wrapping not handled

**Strengths**:
1. Strategy pattern well-designed
2. Metadata handling correct
3. Backoff calculations accurate
4. Context cancellation proper

**Recommendation**: Clarify retry execution model first. Internal retry with `RetryFunc[T]` simpler and cleaner than external coordination.