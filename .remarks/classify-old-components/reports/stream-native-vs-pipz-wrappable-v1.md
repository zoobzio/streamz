# Intelligence Report: OLD/ Component Classification
## Stream-Native vs Pipz-Wrappable Analysis

**Date:** 2025-08-29  
**Agent:** fidgel (Intelligence Officer)  
**Mission:** Classify OLD/ components for migration strategy

---

## Executive Summary

Analysis of 40+ components in OLD/ reveals **15 stream-native components** requiring custom Result[T] migration and **25+ pipz-wrappable components** that should be ignored for now. Critical finding: 3 stream-native components have **severe concurrency bugs** requiring immediate attention.

**Key Finding:** The majority (62%) of OLD/ components are simple channel wrappers around synchronous operations that pipz already handles better. Midgel should focus ONLY on the 15 stream-native components that genuinely require channel-based coordination.

---

## STREAM-NATIVE COMPONENTS (Midgel MUST Migrate)

These components inherently require channels, goroutines, timers, or stateful coordination. They cannot be replaced by pipz and need custom Result[T] migration.

### CRITICAL - Components with Severe Bugs (Fix First!)

#### 1. **debounce.go** ⚠️ SEVERE BUG
- **Bug:** Race condition in timer callback - attempts to unlock already-unlocked mutex
- **Line 81-88:** Unlocks mutex, sends to channel, then tries to lock/unlock again
- **Impact:** Will cause panic under concurrent access
- **Fix Required:** Proper mutex handling in timer callback

#### 2. **dedupe.go** ⚠️ MEMORY LEAK
- **Bug:** Without TTL configuration, grows unbounded forever
- **Impact:** Production OOM after processing enough unique items
- **Fix Required:** Force TTL or implement LRU cache with max size

#### 3. **async.go** ⚠️ MEMORY GROWTH
- **Bug:** Order preservation map grows with (workers × processing_time_variance)
- **Impact:** High memory usage with variable processing times
- **Fix Required:** Bounded queue or streaming order preservation

### Time-Based Components (Require Timers/Clocks)

4. **throttle.go** - Rate limiting with ticker
5. **aggregate.go** - Time-windowed aggregations with state
6. **batcher.go** - Time-based batch triggers
7. **window_sliding.go** - Overlapping time windows
8. **window_tumbling.go** - Non-overlapping time windows  
9. **window_session.go** - Gap-based session windows

### Channel Coordination Components

10. **fanin.go** - Merges multiple channels (inherently channel-based)
11. **fanout.go** - Broadcasts to multiple channels
12. **buffer_dropping.go** - Drops old items when full (needs select with default)
13. **buffer_sliding.go** - Sliding window buffer with eviction

### Stateful Processing Components

14. **monitor.go** - Periodic statistics reporting with atomic counters
15. **sample.go** - Probabilistic sampling (needs state for deterministic sampling)

---

## PIPZ-WRAPPABLE COMPONENTS (Midgel Should IGNORE)

These are just channel wrappers around item-by-item operations. Pipz handles these better with its synchronous approach.

### Direct Pipz Replacements Available

| OLD Component | Pipz Replacement | Notes |
|--------------|------------------|-------|
| **filter.go** | pipz.Filter | Exact match - predicate function |
| **mapper.go** | pipz.Transform | Pure transformation |
| **retry.go** | pipz.Retry | Better implementation with backoff |
| **circuit_breaker.go** | pipz.CircuitBreaker | Production-ready version |
| **tap.go** | pipz.Effect | Side effects without modification |
| **flatten.go** | Custom Transform | Simple slice expansion |
| **chunk.go** | Custom Transform | Array to chunks |
| **skip.go** | Custom Filter | Skip first N items |
| **take.go** | Custom Filter | Take first N items |

### Composite Operations (Combine Pipz Primitives)

- **router.go** → pipz.Switch with routing logic
- **partition.go** → Multiple pipz.Filter in Concurrent
- **split.go** → pipz.Switch with binary decision
- **switch.go** → pipz.Switch (direct equivalent)
- **dlq.go** → pipz.Handle for error routing

### Simple Transformations

- **unbatcher.go** → Just loops through slice (Transform)
- **atomic_time.go** → Utility type, not a processor
- **api.go** → Interface definitions
- **clock.go**, **clock_real.go**, **clock_fake_test.go** → Time abstractions
- **window_types.go** → Type definitions

---

## Bug Analysis for Stream-Native Components

### Race Conditions Found

1. **debounce.go:81-88**
   ```go
   mu.Unlock()
   select {
   case out <- itemToSend:  // UNLOCKED HERE!
   case <-ctx.Done():
   }
   mu.Lock() // Re-acquire for defer - BUT WE'RE IN CALLBACK!
   ```

2. **monitor.go** - Potential race
   - Line 141-144: Reading lastTime while another goroutine might update
   - Needs atomic operations or better synchronization

3. **async.go** - Order preservation issue
   - Pending map grows unbounded with slow workers
   - No backpressure mechanism

### Resource Leaks Found

1. **dedupe.go** - Unbounded memory growth
   - Map never cleaned without TTL
   - Should enforce max size or TTL

2. **aggregate.go** - Timer lifecycle
   - Creates ticker even when not needed (line 164)
   - Should lazy-create on demand

3. **batcher.go** - Timer management
   - Stops/starts timer frequently (line 72, 96, 100)
   - Could reuse timer instance

---

## Migration Priority Recommendations

### Phase 1: Fix Critical Bugs (IMMEDIATE)
1. Fix debounce.go race condition
2. Add memory bounds to dedupe.go
3. Fix async.go memory growth

### Phase 2: Migrate Core Stream Components
1. **batcher.go** - Most commonly used
2. **throttle.go** - Critical for rate limiting
3. **fanin.go/fanout.go** - Channel coordination
4. **buffer_*.go** - Buffering strategies

### Phase 3: Migrate Window Operations
1. **window_sliding.go**
2. **window_tumbling.go**
3. **window_session.go**
4. **aggregate.go**

### Phase 4: Monitoring/Observability
1. **monitor.go**
2. **sample.go**

### IGNORE (Will wrap pipz later)
- All components in "Pipz-Wrappable" section
- These will eventually just be thin channel wrappers around pipz operations

---

## Integration Opportunities

Once migrated to Result[T], these patterns emerge:

1. **Composed Rate Control**
   ```go
   throttled := NewThrottle(100) // 100 items/sec
   batched := NewBatcher(1000, 5*time.Second)
   pipeline := Compose(throttled, batched)
   ```

2. **Windowed Aggregation Pipeline**
   ```go
   window := NewSlidingWindow(5*time.Minute)
   aggregate := NewAggregate(Sum())
   pipeline := Compose(window, aggregate)
   ```

3. **Resilient Stream Processing**
   ```go
   // Wrap pipz processor in streaming context
   resilient := WrapPipz(
       pipz.CircuitBreaker("api",
           pipz.Retry("retry", processor, 3),
       ),
   )
   ```

---

## Appendix: Fascinating Patterns Discovered

### Pattern 1: The Hidden State Machine
CircuitBreaker implements a proper 3-state machine (Closed/Open/HalfOpen) with atomic transitions. This pattern could be extracted into a generic state machine for other components.

### Pattern 2: Clock Abstraction Excellence  
Every time-based component accepts a Clock interface, enabling deterministic testing with FakeClock. This is a masterclass in testable time-based code.

### Pattern 3: The Order Preservation Complexity
AsyncMapper's order preservation using sequence numbers and a pending map is clever but has unbounded growth issues. A circular buffer approach would be more memory-efficient.

### Pattern 4: Graceful Shutdown Patterns
Most components properly handle context cancellation and drain remaining items. However, the pattern is inconsistently applied - could be standardized.

---

## Final Recommendation for Midgel

**FOCUS ONLY ON THESE 15 STREAM-NATIVE COMPONENTS:**

1. debounce (FIX BUG FIRST)
2. dedupe (ADD MEMORY BOUNDS)  
3. async (FIX MEMORY GROWTH)
4. throttle
5. aggregate
6. batcher
7. window_sliding
8. window_tumbling
9. window_session
10. fanin
11. fanout
12. buffer_dropping
13. buffer_sliding
14. monitor
15. sample

**IGNORE EVERYTHING ELSE** - They're just channel wrappers that pipz already does better.

---

*Intelligence gathered through static analysis and pattern recognition. No dynamic testing performed.*