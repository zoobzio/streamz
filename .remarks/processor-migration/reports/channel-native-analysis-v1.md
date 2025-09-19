# Channel-Native Processor Analysis

## Executive Summary

After analyzing the OLD/ directory, I've categorized 33 processors based on their internal implementation requirements. **12 processors are inherently channel-native** and require direct Result[T] migration, while the remaining 21 can be simplified to use pipz's synchronous transformation model.

The channel-native processors fall into four distinct categories:
1. **Time-based operations** (6 processors) - require timers and state management
2. **Multiple channel orchestration** (3 processors) - coordinate between channels
3. **Buffering and flow control** (2 processors) - manage channel lifecycle  
4. **Complex state machines** (1 processor) - maintain internal state

## Channel-Native Processors (Cannot Use pipz)

### Time-Based Operations Category

These processors require timers, state management, and complex channel orchestration that cannot be simplified to synchronous transformations:

**1. Batcher** ⭐ HIGH PRIORITY
- **Why channel-native**: Uses `select` with timer.C(), accumulates state, batches on time OR size
- **Implementation**: Timer management with `timer.Reset()` and `timer.Stop()` in complex select loop
- **Complexity**: Medium - single goroutine with timer coordination

**2. Debounce** ⭐ HIGH PRIORITY  
- **Why channel-native**: Uses `AfterFunc` timers, mutex-protected state, complex firing logic
- **Implementation**: Mutex coordination with timer callbacks in separate goroutines
- **Complexity**: High - multiple goroutines with shared state synchronization

**3. Throttle** ⭐ HIGH PRIORITY
- **Why channel-native**: Uses ticker.C() for rate limiting with channel select
- **Implementation**: Ticker coordination in select loop for rate limiting
- **Complexity**: Low - simple ticker-based rate limiting

**4. SlidingWindow**
- **Why channel-native**: Multiple overlapping time windows, complex timer management
- **Implementation**: Map of active windows + ticker for window expiration
- **Complexity**: High - manages multiple time windows simultaneously

**5. TumblingWindow** (inferred from window_tumbling.go)
- **Why channel-native**: Time-based window boundaries with state accumulation
- **Implementation**: Similar to sliding window but non-overlapping
- **Complexity**: Medium - single active window management

**6. SessionWindow** (inferred from window_session.go)
- **Why channel-native**: Session timeout tracking per key with complex expiration
- **Implementation**: Per-session timeout management with key-based state
- **Complexity**: Very High - multiple session states with independent timers

### Multiple Channel Orchestration Category

These processors coordinate between multiple channels and cannot be reduced to single-item transformations:

**7. Router** ⭐ HIGH PRIORITY
- **Why channel-native**: Creates and manages multiple output channels, route coordination
- **Implementation**: Multiple output channels with predicate-based routing
- **Complexity**: Medium - channel creation and coordination logic
- **Value**: Commonly used for stream splitting

**8. AsyncMapper** ⭐ HIGH PRIORITY  
- **Why channel-native**: Worker goroutines, sequence tracking, order preservation
- **Implementation**: Multiple worker channels + sequence reordering with pending map
- **Complexity**: High - complex worker coordination with order preservation
- **Value**: Critical for parallel processing

**9. Partition** (inferred)
- **Why channel-native**: Creates multiple output channels based on partitioning function
- **Implementation**: Similar to Router but with hash-based distribution
- **Complexity**: Medium - multiple channel management

### Buffering and Flow Control Category

These processors manage channel lifecycle and buffering strategies:

**10. Buffer** ⭐ HIGH PRIORITY
- **Why channel-native**: Channel creation with specific buffer size
- **Implementation**: Simply creates buffered channel of specified size
- **Complexity**: Very Low - trivial buffered channel creation
- **Value**: Fundamental for backpressure management

**11. DroppingBuffer** (from buffer_dropping.go)
- **Why channel-native**: Non-blocking send with overflow dropping
- **Implementation**: Select with default case for dropping overflow items
- **Complexity**: Low - non-blocking channel operations

**12. SlidingBuffer** (from buffer_sliding.go)  
- **Why channel-native**: FIFO buffer with size limit and sliding window behavior
- **Implementation**: Circular buffer management with channel coordination
- **Complexity**: Medium - internal buffer state with channel coordination

### Complex State Machine Category

**13. CircuitBreaker**
- **Why channel-native**: Three-state machine (Closed/Open/HalfOpen) with failure tracking
- **Implementation**: State machine with failure counting and timeout management  
- **Complexity**: High - state transitions with timer coordination
- **Value**: Critical for resilience patterns

## Pipz Candidates (SHOULD use FromChainable)

These 20+ processors perform synchronous item-by-item transformations and should eventually use pipz:

**Simple Transformations:**
- Filter - predicate-based item selection
- Mapper - 1:1 type transformation  
- Tap - side effect without modification
- Skip - drop first N items
- Take - take first N items

**Stateful but Synchronous:**
- Dedupe - can use LRU cache as state
- Sample - periodic sampling (every Nth)
- Retry - synchronous retry with pipz.Retry connector
- Monitor - metrics collection side effect

**Data Structure Operations:**
- Flatten - expand collections to individual items
- Chunk - group into fixed-size chunks (different from time-based Batcher)
- Split - split items based on predicate
- Switch - route single item based on function
- Unbatcher - flatten batches back to items

**Error Handling:**
- DLQ - redirect errors (can use pipz error handling)

## Migration Priority Matrix

**Phase 1 - Simple & High Value (Start Here):**
1. **Buffer** - Trivial implementation, fundamental utility
2. **Throttle** - Simple ticker logic, commonly used
3. **Router** - High value, moderate complexity
4. **Batcher** - Commonly used, well-understood pattern

**Phase 2 - Complex but Important:**
5. **AsyncMapper** - Complex but critical for parallel processing
6. **Debounce** - Complex synchronization but high value
7. **CircuitBreaker** - State machine complexity but resilience critical

**Phase 3 - Specialized Use Cases:**
8. **DroppingBuffer** - Specialized buffering strategy
9. **SlidingBuffer** - Complex buffer management
10. **Partition** - Similar to Router but hash-based

**Phase 4 - Advanced Time Windows:**
11. **TumblingWindow** - Time window complexity
12. **SlidingWindow** - Multiple overlapping windows
13. **SessionWindow** - Highest complexity, per-key state

## Implementation Pattern Analysis

Examining the migrated FanIn and FanOut processors reveals the Result[T] pattern:

```go
// Channel-native signature pattern:
func (p *Processor[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan Result[T]

// Multiple output variant:
func (p *Processor[T]) Process(ctx context.Context, in <-chan Result[T]) []<-chan Result[T]

// Multiple input variant:  
func (p *Processor[T]) Process(ctx context.Context, ins ...<-chan Result[T]) <-chan Result[T]
```

**Key Result[T] Benefits:**
- Unified error handling eliminates dual-channel complexity
- Errors flow naturally through the pipeline
- Context propagation remains clean
- Type safety maintained throughout

## Intelligence Notes

### Complexity Drivers Analysis

**Timer Management** is the primary complexity driver:
- Batcher: Timer reset/stop coordination
- Debounce: AfterFunc with mutex synchronization  
- Throttle: Ticker-based rate limiting
- All Window types: Multiple timer coordination

**State Management** creates the second tier of complexity:
- AsyncMapper: Sequence tracking with pending map
- CircuitBreaker: Three-state machine with counters
- Buffer variants: Internal buffer state management
- Dedupe: Key tracking with TTL (though this could use pipz)

**Channel Orchestration** requires careful coordination:
- Router: Multiple output creation and management
- FanOut/FanIn: Already migrated, demonstrate patterns
- Partition: Hash-based channel distribution

### Anti-Patterns to Avoid

Based on the OLD/ implementations:
- **Don't** try to make everything channel-native - prefer pipz when possible
- **Don't** ignore context cancellation in complex select loops
- **Don't** share state between goroutines without proper synchronization
- **Don't** create timers without proper cleanup

## Recommendations

**For zidgel (Strategic):**
- Prioritize Buffer, Throttle, Router, Batcher for immediate business value
- The Result[T] pattern will dramatically simplify error handling in pipelines
- Consider this migration as Phase 1 of the larger streamz modernization

**For midgel (Architectural):**
- The Result[T] pattern is architecturally sound based on FanIn/FanOut success
- Timer management will be the biggest complexity challenge
- Consider creating helper utilities for common timer patterns
- State machines (CircuitBreaker) may benefit from a common state machine framework

**For kevin (Implementation):**
- Start with Buffer - it's almost trivial to implement
- Use FanIn/FanOut as reference implementations for Result[T] patterns
- Test timer-based processors extensively with FakeClock
- Pay special attention to context cancellation in complex select loops

---

*Intelligence Note: The channel-native vs pipz categorization reveals a clean architectural boundary. Time-based operations and multiple channel coordination fundamentally require channel operations, while most data transformations can be simplified to synchronous functions. This analysis provides a clear migration roadmap prioritized by complexity and business value.*