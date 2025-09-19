# Error Recovery Strategies Investigation for Streamz Processors

## Investigation Date: 2025-09-11

## Executive Summary

Investigated error recovery strategies for streamz processors. Current system uses Result[T] pattern for unified error handling. Each processor has unique semantics requiring different recovery approaches. Found patterns: fail-fast processors (throttle, debounce), accumulating processors (batcher, window), and concurrent processors (async_mapper). 

Critical finding: Recovery strategies must preserve processor semantics. Batcher emitting partial batch on error changes fundamental behavior. Debounce emitting pending on error violates timing contracts.

## Current Error Handling Architecture

### Result[T] Pattern
```go
type Result[T any] struct {
    value T
    err   *StreamError[T]
}
```

**Characteristics:**
- Unified success/error channel
- Preserves item context in errors
- Processors pass errors through unchanged
- No dual-channel complexity

### StreamError[T] Structure
```go
type StreamError[T any] struct {
    Item          T       // Original item that failed
    Err           error   // Underlying error
    ProcessorName string  // Which processor failed
    Timestamp     time.Time
}
```

**Forensic Value:**
- Complete failure context
- Item preservation for retry
- Processing path tracking
- Temporal correlation

## Processor Semantic Analysis

### Time-Based Processors

#### Throttle (Leading Edge)
**Current Behavior:**
- Errors pass through immediately
- Success items throttled
- First item immediate, rest ignored during cooldown

**Recovery Considerations:**
- Cannot retry during cooldown (violates throttle semantics)
- Error bypass is correct behavior
- No partial state to emit

#### Debounce (Trailing Edge)
**Current Behavior:**
- Errors pass through immediately
- Success items debounced
- Only last item emitted after quiet period

**Recovery Considerations:**
- Pending item exists during debounce
- Error could trigger early emission (changes semantics)
- Timer management critical

#### Batcher (Size/Time)
**Current Behavior:**
- Errors pass through immediately
- Success items batched
- Batch emitted on size OR time limit

**Recovery Considerations:**
- Partial batch exists between emissions
- Error could trigger partial batch emission
- Changes fundamental batching contract

### Windowing Processors

#### TumblingWindow
**Current Behavior:**
- Both successes and errors collected in window
- Window emitted at time boundary
- Empty windows not emitted

**Recovery Considerations:**
- Window contains mixed Results
- Already handles errors internally
- No recovery needed - window semantics preserved

#### SlidingWindow
**Current Behavior:**
- Overlapping windows
- Each item in multiple windows
- Mixed Results per window

**Recovery Considerations:**
- Complex state management
- Error affects multiple windows
- Recovery could cause duplicate processing

#### SessionWindow
**Current Behavior:**
- Dynamic windows based on gaps
- Session closes after inactivity
- Mixed Results per session

**Recovery Considerations:**
- Session boundary detection critical
- Error shouldn't close session
- Timeout management complex

### Distribution Processors

#### FanIn
**Current Behavior:**
- Merges multiple channels
- Preserves all Results
- No transformation

**Recovery Considerations:**
- No processing to fail
- Pure routing - no recovery needed
- Error source preserved

#### FanOut
**Current Behavior:**
- Duplicates to multiple channels
- All outputs identical
- No transformation

**Recovery Considerations:**
- No processing to fail
- Distribution guaranteed
- Error propagated to all outputs

### Buffering Processors

#### Buffer
**Current Behavior:**
- Pass-through with buffering
- No transformation
- Preserves order

**Recovery Considerations:**
- No processing to fail
- Buffer overflow is system issue
- Context cancellation handled

### Concurrent Processors

#### AsyncMapper
**Current Behavior:**
- Concurrent transformation
- Ordered/unordered modes
- Error wrapped with context

**Recovery Considerations:**
- Worker pool management
- Order preservation in ordered mode
- Retry could cause reordering
- Circuit breaker integration possible

## Error Recovery Strategy Patterns

### Pattern 1: Retry with Exponential Backoff
```go
type RetryStrategy struct {
    MaxAttempts int
    BaseDelay   time.Duration
    MaxDelay    time.Duration
    WithJitter  bool
}
```

**Applicable To:**
- AsyncMapper (I/O operations)
- External API calls
- Transient failures

**Not Applicable To:**
- Time-sensitive processors (throttle, debounce)
- Windowing (would duplicate items)
- Pure routing (fanin, fanout)

### Pattern 2: Circuit Breaker
```go
type CircuitBreakerStrategy struct {
    FailureThreshold float64
    MinRequests      int64
    RecoveryTimeout  time.Duration
    HalfOpenRequests int64
}
```

**Applicable To:**
- AsyncMapper with external dependencies
- Processors calling services
- Cascading failure prevention

**Not Applicable To:**
- Pure computation processors
- Time-based processors
- Routing processors

### Pattern 3: Dead Letter Queue
```go
type DLQStrategy struct {
    MaxRetries       int
    RetryDelay       time.Duration
    ContinueOnError  bool
    FailedBufferSize int
}
```

**Applicable To:**
- Any processor for permanent failures
- Audit trail requirements
- Manual retry scenarios

**Integration Points:**
- After retry exhaustion
- For non-retryable errors
- Forensic analysis

### Pattern 4: Skip Error
```go
type SkipStrategy struct {
    SkipCount        int
    ErrorClassifier  func(error) bool
}
```

**Applicable To:**
- Non-critical data streams
- High-volume processing
- Acceptable data loss scenarios

**Not Applicable To:**
- Financial transactions
- Critical event processing
- Audit requirements

### Pattern 5: Emit Partial on Error
```go
type EmitPartialStrategy struct {
    OnError func(partial interface{}, err error)
}
```

**Applicable To:**
- Batcher (emit partial batch)
- Aggregators (emit partial aggregation)
- Best-effort processing

**Semantic Changes:**
- Batcher: Changes batch size guarantees
- Debounce: Violates timing contract
- Window: Already handles mixed Results

## Processor-Specific Recovery Recommendations

### Batcher
```go
// Option 1: Emit partial batch on error
func (b *Batcher[T]) WithErrorRecovery(emitPartial bool) *Batcher[T]

// Option 2: Include error in batch result
type BatchResult[T] struct {
    Items  []T
    Errors []StreamError[T]
}
```

**Recommendation:** Option 2 - Preserve batch semantics, include errors in result.

### Debounce
```go
// Option 1: Emit pending on error
func (d *Debounce[T]) WithErrorRecovery(emitOnError bool) *Debounce[T]

// Option 2: Reset timer on error
func (d *Debounce[T]) WithErrorReset(reset bool) *Debounce[T]
```

**Recommendation:** Neither - Errors already pass through. Preserve timing semantics.

### AsyncMapper
```go
// Option 1: Built-in retry
func (a *AsyncMapper[I,O]) WithRetry(strategy RetryStrategy) *AsyncMapper[I,O]

// Option 2: Circuit breaker
func (a *AsyncMapper[I,O]) WithCircuitBreaker(cb CircuitBreakerStrategy) *AsyncMapper[I,O]
```

**Recommendation:** Option 2 - Circuit breaker for external dependencies.

### Window Processors
**Current Behavior Sufficient:**
- Windows already contain mixed Results
- Users can analyze errors per window
- No recovery needed at processor level

## Implementation Approach

### Phase 1: Recovery Primitives
Create standalone recovery processors that wrap existing processors:

```go
// Retry wrapper
retry := NewRetry(processor).
    MaxAttempts(3).
    BaseDelay(100*time.Millisecond)

// Circuit breaker wrapper
cb := NewCircuitBreaker(processor).
    FailureThreshold(0.5).
    RecoveryTimeout(30*time.Second)

// DLQ wrapper
dlq := NewDeadLetterQueue(processor).
    MaxRetries(2).
    OnFailure(handleFailure)
```

### Phase 2: Composition Patterns
```go
// Resilient pipeline
pipeline := NewAsyncMapper(apiCall).
    Wrap(NewRetry().MaxAttempts(3)).
    Wrap(NewCircuitBreaker().FailureThreshold(0.5)).
    Wrap(NewDeadLetterQueue().OnFailure(logFailure))
```

### Phase 3: Processor-Specific Integration
Only where semantics allow:
- AsyncMapper: Built-in circuit breaker for worker pool
- Batcher: BatchResult with error tracking
- No changes to time-based processors

## Testing Requirements

### Integration Tests Needed

1. **Retry with Different Processors**
   - Test retry with AsyncMapper
   - Verify order preservation in ordered mode
   - Test retry exhaustion

2. **Circuit Breaker State Transitions**
   - Closed → Open on threshold
   - Open → Half-Open after timeout
   - Half-Open → Closed on success
   - Half-Open → Open on failure

3. **DLQ with Complex Pipelines**
   - Multi-stage pipeline with DLQ
   - Failed item accessibility
   - Manual retry from DLQ

4. **Error Recovery Composition**
   - Retry + Circuit Breaker
   - Circuit Breaker + DLQ
   - Full resilience stack

## Patterns Found

### Pattern: Semantic Preservation
Time-based processors (throttle, debounce) must preserve timing contracts. Error recovery that changes timing violates processor semantics.

### Pattern: Error Transparency
Window processors already handle mixed Results. No additional recovery needed - error visibility sufficient.

### Pattern: Wrapper Composition
Recovery strategies as wrappers preserve single responsibility. Processors focus on core logic, wrappers handle resilience.

### Pattern: Fail-Fast vs Accumulate
- Fail-fast: Routing processors (fanin, fanout)
- Accumulate: Window processors
- Choice point: Batching processors

## Specific Recommendations

1. **Implement Recovery Wrappers First**
   - Retry, CircuitBreaker, DLQ as standalone
   - Composable with any processor
   - No processor modifications needed

2. **AsyncMapper Gets Circuit Breaker**
   - Natural fit for external dependencies
   - Worker pool management
   - Prevents cascade failures

3. **Preserve Time-Based Semantics**
   - No recovery for throttle/debounce
   - Timing contracts are inviolate
   - Errors already pass through

4. **Window Processors Stay As-Is**
   - Current mixed Result handling sufficient
   - Users can implement window-level recovery
   - No semantic changes needed

5. **Batcher Decision Point**
   - Consider BatchResult[T] with error tracking
   - Preserves size guarantees
   - Enables partial batch analysis

## Next Steps

1. Implement Retry wrapper processor
2. Implement CircuitBreaker wrapper processor
3. Implement DeadLetterQueue wrapper processor
4. Create integration tests for compositions
5. Document recovery patterns in examples/error-recovery/

## Conclusion

Error recovery in streamz requires careful consideration of processor semantics. Wrapper-based approach provides flexibility without violating core contracts. Time-based processors must preserve timing guarantees. Window processors already handle errors appropriately. AsyncMapper is primary candidate for integrated recovery. Composition of recovery wrappers enables complex resilience patterns without processor modifications.