# Circuit Breaker Implementation Plan

## Executive Summary

Circuit Breaker has fundamental architectural conflicts with stream processing patterns. While technically implementable, the pattern violates core stream processing principles and creates operational hazards. Not recommended for implementation.

## Architectural Analysis

### Core Pattern Requirements

Circuit Breaker requires three state transitions:

1. **Closed (Normal)**: All requests pass through
2. **Open (Fail Fast)**: All requests immediately rejected 
3. **Half-Open (Testing)**: Limited test requests allowed

### Critical Architectural Conflicts

#### 1. Stream Processing vs Request/Response Impedance Mismatch

**Problem**: Circuit Breaker assumes request/response semantics with controllable failure rates. Stream processing operates on continuous data flows where individual "requests" don't exist.

**Evidence**: 
- Circuit Breaker documentation shows wrapping processors like AsyncMapper that handle individual items
- Stream processing operates on Result[T] flows, not individual request/response pairs
- Failure rate calculation requires bounded request windows, but streams are unbounded

**Impact**: The pattern doesn't map naturally to stream processing concepts.

#### 2. State Persistence Across Stream Sessions

**Problem**: Circuit state must persist across multiple `Process()` calls and stream sessions. This violates streamz architectural patterns.

**Evidence**:
- Existing processors (Throttle, Batcher, etc.) maintain per-Process() session state only
- Circuit Breaker requires global state shared across all stream sessions
- No existing connector maintains cross-session state in streamz

**Mumbai Lesson**: Stateful components that persist across operations create debugging nightmares. Stream processors should be stateless between Process() calls.

#### 3. Backpressure Interference 

**Problem**: Circuit Breaker's "fail fast" behavior conflicts with Go channel backpressure semantics.

**Evidence**:
- When circuit opens, upstream channels continue buffering items
- Immediate rejection creates artificial backpressure that propagates incorrectly
- Stream processors expect natural channel flow control, not artificial rejection

**Impact**: Circuit behavior disrupts natural flow control, causing unpredictable backpressure cascades.

#### 4. Continuous Stream vs Discrete Request Semantics

**Problem**: Circuit Breaker was designed for discrete service calls, not continuous data streams.

**Analysis**:
- Traditional Circuit Breaker: "Service X is failing, stop calling it"
- Stream Circuit Breaker: "Data flow through processor Y is failing, stop processing all data"

This semantic mismatch means Circuit Breaker stops useful work when only some data paths are failing.

## Implementation Challenges

### Challenge 1: Failure Rate Calculation

Circuit Breaker requires tracking success/failure rates over time windows. Stream processing complications:

1. **Window Definition**: What constitutes a measurement window in a continuous stream?
2. **Success Definition**: When is a Result[T] considered successful vs failed?
3. **Rate Calculation**: How to calculate meaningful failure rates across variable data velocities?

### Challenge 2: State Management

Required state persistence:
- Current circuit state (closed/open/half-open)
- Success/failure counters
- Last state change timestamp
- Half-open test request counters

This violates streamz stateless processor architecture.

### Challenge 3: Open Circuit Behavior

When circuit is open, processor must:
1. Immediately reject all incoming items
2. Transform successful items into failures
3. Maintain rejection metadata for debugging

This artificial error injection corrupts the natural error flow patterns in Result[T].

## Alternative Approaches

### 1. Throttling (Existing)

Already implemented in streamz. Provides rate limiting without state persistence issues.

**Advantages**:
- Stateless per-session
- Natural stream semantics
- No artificial error injection
- Works with channel backpressure

### 2. Dead Letter Queue (Existing)

Handle failures through DLQ pattern rather than preventing them.

**Advantages**:
- Isolates failures without stopping processing
- Maintains stream flow semantics
- Debugging-friendly error tracking

### 3. Timeout + Retry (Possible Extension)

Implement timeout behavior in AsyncMapper with retry logic.

**Advantages**:
- Addresses same reliability concerns
- Maintains stream processor semantics
- No cross-session state required

### 4. Health Check Integration

External circuit breaker at the stream ingress level.

**Advantages**:
- Proper separation of concerns
- Circuit logic outside stream processing
- Standard operational patterns

## Recommendation: Do Not Implement

### Technical Reasons

1. **Architectural Mismatch**: Circuit Breaker fundamentally conflicts with stream processing patterns
2. **State Complexity**: Required state persistence violates streamz design principles
3. **Semantic Confusion**: Fails to provide meaningful value in continuous stream context
4. **Debugging Complexity**: Adds stateful behavior that complicates troubleshooting

### Mumbai Lesson Applied

Circuit Breaker represents exactly the kind of "sophisticated" pattern that served my ego rather than the system's needs. It looks professional but creates operational complexity without clear benefits.

The pattern works for request/response services. Forcing it into stream processing creates artificial complexity that will cause 3 AM debugging sessions.

### Alternative Solution

Instead of Circuit Breaker, implement:

1. **Enhanced AsyncMapper**: Add timeout and retry capabilities
2. **Health Check Hooks**: Allow external circuit breakers to control stream ingress
3. **Improved DLQ**: Better failure routing and recovery patterns
4. **Throttling Enhancements**: More sophisticated rate limiting if needed

These approaches solve the same reliability problems without violating stream processing principles.

## Implementation Plan (If Forced)

If organizational requirements force implementation despite architectural issues:

### Phase 1: Minimal Viable Implementation

```go
type CircuitBreaker[T any] struct {
    processor    Processor[T, T]
    state        *int32  // atomic state tracking
    stats        *CircuitStats
    config       CircuitConfig
    lastStateChange time.Time
    mutex        sync.RWMutex
}

type CircuitState int32
const (
    StateClosed CircuitState = iota
    StateOpen
    StateHalfOpen
)
```

### Phase 2: State Transition Logic

```go
func (cb *CircuitBreaker[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan Result[T] {
    out := make(chan Result[T])
    
    go func() {
        defer close(out)
        
        for result := range in {
            if cb.shouldReject() {
                // Convert to failure
                rejected := NewError(result.Value(), ErrCircuitOpen, "circuit-breaker")
                select {
                case out <- rejected:
                case <-ctx.Done():
                    return
                }
                continue
            }
            
            // Process normally and track results
            // [Implementation details]
        }
    }()
    
    return out
}
```

### Phase 3: Monitoring and Testing

- State change callbacks
- Comprehensive unit tests for all state transitions
- Integration tests showing stream behavior
- Performance benchmarks
- Documentation with clear warnings about architectural concerns

### Warning Labels Required

Any implementation must include prominent documentation warnings:

1. "Circuit Breaker violates stream processing best practices"
2. "Consider Throttle or DLQ patterns instead"
3. "Adds stateful complexity that complicates debugging"
4. "Not recommended for production use"

## Complexity Assessment

**Necessary Complexity**: None. The reliability problems Circuit Breaker solves are better addressed through existing streamz patterns.

**Arbitrary Complexity**: All of it. Circuit Breaker forces request/response patterns into stream processing, creating artificial complexity that serves architectural fashion rather than operational needs.

**Mumbai Lesson**: This is exactly the kind of "enterprise pattern" that looks sophisticated but creates debugging nightmares. Stream processing has better reliability patterns that work with the architecture instead of against it.

## Conclusion

Circuit Breaker should not be implemented in streamz. The pattern conflicts with fundamental stream processing principles and creates complexity without corresponding benefits.

Use existing patterns:
- Throttle for rate limiting
- DLQ for failure handling  
- AsyncMapper with timeouts for reliability
- External health checks for service protection

These approaches solve the same problems without violating architectural principles or creating Mumbai-style debugging scenarios.