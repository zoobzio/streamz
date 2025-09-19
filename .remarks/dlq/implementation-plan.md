# DeadLetterQueue Implementation Plan

## Architecture Decision: Dual-Channel Output Pattern

The DeadLetterQueue processor breaks the standard single-channel Result[T] pattern to provide separate success and failure streams. This architectural deviation is justified for error handling scenarios where success and failure processing require different downstream handling.

### Return Signature
```go
func (dlq *DeadLetterQueue[T]) Process(ctx context.Context, in <-chan Result[T]) (successes, failures <-chan Result[T])
```

This dual-channel approach enables:
- Independent backpressure control for success and failure streams
- Different processing strategies for successful vs failed items
- Selective consumption - can ignore failures or successes as needed
- Clear separation of concerns in error handling pipelines

## Core Implementation Strategy

### 1. Dual Channel Creation and Management
```go
type DeadLetterQueue[T any] struct {
    name string
    // No wrapped processor - DLQ operates on incoming Result[T] stream
}

func (dlq *DeadLetterQueue[T]) Process(ctx context.Context, in <-chan Result[T]) (<-chan Result[T], <-chan Result[T]) {
    successCh := make(chan Result[T])
    failureCh := make(chan Result[T])
    
    // Single goroutine manages distribution logic
    go dlq.distribute(ctx, in, successCh, failureCh)
    
    return successCh, failureCh
}
```

### 2. Distribution Logic
The core distribution logic runs in a single goroutine to avoid race conditions:
```go
func (dlq *DeadLetterQueue[T]) distribute(ctx context.Context, in <-chan Result[T], successCh, failureCh chan Result[T]) {
    defer close(successCh)
    defer close(failureCh)
    
    for {
        select {
        case <-ctx.Done():
            return
        case result, ok := <-in:
            if !ok {
                return
            }
            
            if result.IsError() {
                dlq.sendToFailures(ctx, result, failureCh)
            } else {
                dlq.sendToSuccesses(ctx, result, successCh)
            }
        }
    }
}
```

### 3. Context Cancellation Strategy
Both channels respect context cancellation:
- Distribution goroutine exits on context cancellation
- Both output channels are closed when distribution exits
- No orphaned goroutines or channel leaks

### 4. Buffering Strategy Analysis

**Unbuffered Channels (Recommended Initial Approach):**
- Provides natural backpressure
- Forces explicit handling of both streams
- Prevents memory accumulation
- Simpler to reason about

```go
successCh := make(chan Result[T])
failureCh := make(chan Result[T])
```

**Buffered Channels (Future Option):**
- Could add WithBuffer(size) configuration method
- Different buffer sizes for success vs failure streams
- Risk: accumulated failures could consume excessive memory

### 5. Non-Consumed Channel Handling

Critical consideration: What happens if one channel isn't consumed?

**Problem Scenario:**
```go
dlq := NewDeadLetterQueue[Order](RealClock)
successes, failures := dlq.Process(ctx, input)
// Only consume successes, ignore failures
for result := range successes {
    // Process success
}
// failures channel never consumed - blocks distribution
```

**Solution: Non-blocking Send with Context**
```go
func (dlq *DeadLetterQueue[T]) sendToFailures(ctx context.Context, result Result[T], failureCh chan Result[T]) {
    select {
    case failureCh <- result:
        // Sent successfully
    case <-ctx.Done():
        // Context cancelled, exit gracefully
        return
    default:
        // Channel blocked - could log warning or apply policy
        dlq.handleBlockedFailure(result)
    }
}
```

**Policy Options for Blocked Channels:**
1. **Drop and log** - Prevents deadlock, loses data
2. **Context-based timeout** - Attempt send with timeout
3. **Buffer to disk** - Expensive but preserves data
4. **Panic** - Fail fast on programming errors

Initial implementation: Drop and log (safest for stability)

## Interface Compatibility

### Processor Interface Deviation
Standard processors implement:
```go
Process(context.Context, <-chan Result[T]) <-chan Result[T]
```

DLQ returns two channels, breaking this interface. This is intentional - DLQ is a specialized processor for error handling workflows where dual streams are valuable.

### Integration with Standard Processors
```go
// Common usage pattern
dlq := NewDeadLetterQueue[Order](RealClock)
successes, failures := dlq.Process(ctx, input)

// Continue with standard processors on success stream
mapper := NewMapper[Order, ProcessedOrder](processOrder)
processed := mapper.Process(ctx, successes)

// Handle failures separately
go func() {
    for failure := range failures {
        log.Error("Processing failed", "error", failure.Error())
        metrics.FailureCount.Inc()
    }
}()
```

## Error Handling Within DLQ

DLQ itself should not fail. Any internal errors are logged but don't interrupt processing:

```go
func (dlq *DeadLetterQueue[T]) handleBlockedFailure(result Result[T]) {
    // Log but don't fail
    log.Warn("Failure channel blocked, dropping item",
        "processor", dlq.name,
        "error", result.Error())
    
    // Could emit metrics
    metrics.DLQDropped.Inc()
}
```

## Memory Safety Analysis

### Goroutine Lifecycle
- Single distribution goroutine per DLQ instance
- Goroutine exits when input channel closes or context cancelled
- No goroutine leaks

### Channel Lifecycle  
- Both output channels closed when distribution exits
- Consumers can detect completion via channel closure
- No dangling channels

### Memory Accumulation Prevention
- No internal buffering of Results
- Immediate forwarding to appropriate channel
- Blocked sends handled with drop policy (initial implementation)

## Testing Strategy

### Unit Tests Required
1. **Success Distribution**: Verify success items route to success channel
2. **Failure Distribution**: Verify error items route to failure channel
3. **Context Cancellation**: Both channels close on context cancel
4. **Input Channel Closure**: Both channels close when input closes
5. **Non-Consumed Channel**: Verify drop policy works without deadlock
6. **Race Conditions**: Concurrent access with `-race` flag

### Race Detection Scenarios
- Context cancellation during channel sends
- Input channel closure during distribution
- Multiple goroutines consuming both channels
- Context timeout during blocked sends

## API Design

### Constructor
```go
func NewDeadLetterQueue[T any](clock Clock) *DeadLetterQueue[T] {
    return &DeadLetterQueue[T]{
        name:  "dlq",
        clock: clock,
    }
}
```

### Fluent Configuration
```go
func (dlq *DeadLetterQueue[T]) WithName(name string) *DeadLetterQueue[T] {
    dlq.name = name
    return dlq
}
```

### Core Method
```go
func (dlq *DeadLetterQueue[T]) Process(ctx context.Context, in <-chan Result[T]) (<-chan Result[T], <-chan Result[T]) {
    // Implementation as outlined above
}
```

### Naming Method (Standard Interface)
```go
func (dlq *DeadLetterQueue[T]) Name() string {
    return dlq.name
}
```

## Implementation Priority

1. **Core distribution logic** - Single goroutine managing channel routing
2. **Context cancellation** - Proper cleanup and exit handling  
3. **Basic unit tests** - Success/failure routing verification
4. **Non-consumed channel handling** - Drop policy implementation
5. **Race condition testing** - Concurrent access validation
6. **Documentation** - Clear usage examples and patterns

## Maintenance Considerations

### Complexity Assessment
- **Visible Complexity**: Dual-channel return signature is explicit
- **Hidden Complexity**: Context handling and channel lifecycle
- **Testing Burden**: Higher due to concurrent channel operations

### Future Enhancement Points
- Configurable buffering strategies
- Pluggable blocked channel policies  
- Metrics and observability hooks
- Alternative distribution strategies (round-robin, weighted)

### Potential Pitfalls
- Deadlocks from non-consumed channels
- Race conditions in concurrent access
- Context cancellation edge cases
- Memory accumulation if policies change

This implementation provides a foundation for error-aware stream processing while maintaining clear architectural boundaries and testability.