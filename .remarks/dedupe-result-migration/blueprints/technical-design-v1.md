# Technical Blueprint: Dedupe Implementation with Result[T]

**Date:** 2025-08-29  
**Engineer:** midgel (Chief Engineer)  
**Mission:** Design new dedupe processor using Result[T] and fixing memory leak

---

## Executive Summary

The OLD dedupe implementation has a CRITICAL memory leak bug - it grows unbounded forever when TTL isn't configured. fidgel identified this as priority #2 in the stream-native components. The new implementation will use Result[T], single-goroutine pattern (like debounce), and bounded memory management to prevent production OOM failures.

**Key Architectural Changes:**
- Use Result[T] for unified error handling
- Single-goroutine design for thread safety
- Bounded memory via configurable LRU cache
- Optional TTL expiration for time-based cleanup
- Deterministic testing via Clock interface

---

## Bug Analysis of OLD Implementation

### CRITICAL BUG: Unbounded Memory Growth

**Location:** `/home/zoobzio/code/streamz/OLD/dedupe.go`, line 61
```go
seen:    make(map[K]time.Time),  // GROWS FOREVER WITHOUT TTL
```

**Problem:** When TTL is 0 (default), the `seen` map grows indefinitely. Every unique key is stored forever, causing memory leaks in production.

**Impact Calculation:**
```
unique_keys × sizeof(K + time.Time) = memory_growth
1M unique strings × ~40 bytes = ~40MB
10M unique UUIDs × ~64 bytes = ~640MB  
100M unique IDs × ~64 bytes = ~6.4GB (OOM!)
```

### Additional Issues Found

1. **Race Window in cleanup()**: Lines 139-149
   - Holding mutex for entire cleanup iteration
   - Could block processing during large cleanups
   - Should use batched cleanup with yielding

2. **Inefficient Timer Usage**: Lines 86-93
   - Creates ticker even when TTL=0 (no cleanup needed)
   - Timer frequency is hardcoded to TTL/2
   - Should be configurable or adaptive

3. **No Memory Pressure Handling**:
   - No max size limit option
   - No LRU eviction when memory constrained
   - No metrics on cache hit/miss rates

4. **Key Function Panic Risk**:
   - If keyFunc panics, entire processor dies
   - Should recover and convert to error

---

## New Architecture Design

### Core Design Principles

1. **Bounded Memory**: Never grow unbounded, even without TTL
2. **Single Goroutine**: Eliminate race conditions entirely
3. **Result[T] Native**: Handle errors properly with stream context
4. **Testable Time**: Use Clock interface for deterministic tests
5. **Configurable Limits**: Runtime-tunable memory and time bounds

### Data Structures

```go
type DedupeCache[K comparable] struct {
    // LRU cache for bounded memory
    entries map[K]*cacheEntry
    head    *cacheEntry  // Most recently used
    tail    *cacheEntry  // Least recently used
    maxSize int
    
    // Optional TTL expiration
    ttl time.Duration
    
    // Statistics
    hits    int64
    misses  int64
    evicted int64
}

type cacheEntry struct {
    key       K
    timestamp time.Time
    prev      *cacheEntry
    next      *cacheEntry
}

type Dedupe[T any, K comparable] struct {
    name     string
    keyFunc  func(T) K
    cache    *DedupeCache[K]
    clock    Clock
}
```

### Memory Management Strategy

**Hybrid Approach: LRU + Optional TTL**

1. **Primary Bound**: LRU cache with configurable max size
   - Default: 10,000 entries (reasonable for most use cases)
   - Evicts least recently used when full
   - O(1) access and eviction via doubly-linked list

2. **Secondary Bound**: Optional TTL expiration  
   - Only if configured (TTL > 0)
   - Lazy expiration during access
   - Periodic cleanup timer (configurable frequency)

3. **Memory Pressure Response**:
   ```go
   // Conservative defaults
   DefaultMaxSize = 10_000   // ~400KB for string keys
   DefaultTTL = 0           // No time-based expiration
   DefaultCleanupInterval = 1 * time.Minute
   ```

### Single-Goroutine Architecture

Following the successful debounce pattern:

```go
func (d *Dedupe[T, K]) Process(ctx context.Context, in <-chan Result[T]) <-chan Result[T] {
    out := make(chan Result[T])
    
    go func() {
        defer close(out)
        
        // Optional cleanup timer
        var cleanupTimer Timer
        var cleanupC <-chan time.Time
        if d.cache.ttl > 0 {
            cleanupTimer = d.clock.NewTimer(d.cleanupInterval)
            cleanupC = cleanupTimer.C()
        }
        
        for {
            select {
            case result, ok := <-in:
                if !ok {
                    // Input closed
                    if cleanupTimer != nil {
                        cleanupTimer.Stop()
                    }
                    return
                }
                
                // Handle result
                processed := d.processResult(result)
                if processed != nil {
                    select {
                    case out <- *processed:
                    case <-ctx.Done():
                        return
                    }
                }
                
            case <-cleanupC:
                d.cache.cleanupExpired(d.clock.Now())
                cleanupTimer.Reset(d.cleanupInterval)
                
            case <-ctx.Done():
                if cleanupTimer != nil {
                    cleanupTimer.Stop()
                }
                return
            }
        }
    }()
    
    return out
}
```

### Error Handling Strategy

**Comprehensive Error Recovery:**

1. **Key Function Panics**:
   ```go
   func (d *Dedupe[T, K]) safeKeyFunc(item T) (key K, err error) {
       defer func() {
           if r := recover(); r != nil {
               err = fmt.Errorf("key function panicked: %v", r)
           }
       }()
       key = d.keyFunc(item)
       return
   }
   ```

2. **Error Pass-Through**:
   ```go
   func (d *Dedupe[T, K]) processResult(result Result[T]) *Result[T] {
       // Errors pass through immediately (like debounce)
       if result.IsError() {
           return &result
       }
       
       // Process successful values
       item := result.Value()
       key, err := d.safeKeyFunc(item)
       if err != nil {
           // Convert panic to error
           return &NewError(item, err, d.name)
       }
       
       // Check for duplicate
       if d.cache.contains(key) {
           return nil // Filtered out
       }
       
       d.cache.add(key, d.clock.Now())
       return &result // Pass through
   }
   ```

---

## Implementation Approach

### Phase 1: Core Cache Implementation

**Files to Create:**
- `dedupe.go` - Main processor
- `dedupe_test.go` - Unit tests
- `testing/integration/dedupe_flow_test.go` - Integration scenarios

**LRU Cache Implementation:**
```go
func (c *DedupeCache[K]) add(key K, timestamp time.Time) {
    if entry, exists := c.entries[key]; exists {
        // Move to head (most recently used)
        c.moveToHead(entry)
        entry.timestamp = timestamp
        return
    }
    
    // Check if at capacity
    if len(c.entries) >= c.maxSize {
        // Evict least recently used
        c.evictTail()
        c.evicted++
    }
    
    // Add new entry at head
    entry := &cacheEntry{
        key:       key,
        timestamp: timestamp,
    }
    c.entries[key] = entry
    c.addToHead(entry)
    c.misses++
}

func (c *DedupeCache[K]) contains(key K) bool {
    entry, exists := c.entries[key]
    if !exists {
        c.misses++
        return false
    }
    
    // Check TTL if configured
    if c.ttl > 0 && time.Since(entry.timestamp) > c.ttl {
        c.remove(entry)
        c.misses++
        return false
    }
    
    // Move to head (LRU update)
    c.moveToHead(entry)
    c.hits++
    return true
}
```

### Phase 2: Configuration API

**Fluent Configuration Pattern:**
```go
// Conservative defaults
dedupe := NewDedupe(keyFunc, RealClock).
    WithMaxSize(50000).                    // 50k entries max
    WithTTL(30 * time.Minute).            // 30min TTL
    WithCleanupInterval(5 * time.Minute). // Cleanup every 5min
    WithName("order-dedupe")

// Memory-optimized for high cardinality
dedupe := NewDedupe(keyFunc, RealClock).
    WithMaxSize(1000).                     // Small cache
    WithTTL(time.Minute).                 // Short TTL
    WithCleanupInterval(10 * time.Second) // Frequent cleanup

// Infinite memory (original behavior)
dedupe := NewDedupe(keyFunc, RealClock).
    WithMaxSize(0).  // 0 = unlimited
    WithTTL(0)       // No TTL
```

### Phase 3: Observability

**Statistics and Metrics:**
```go
type DedupeStats struct {
    CacheSize    int     `json:"cache_size"`
    MaxSize      int     `json:"max_size"`
    Hits         int64   `json:"hits"`
    Misses       int64   `json:"misses"`
    Evicted      int64   `json:"evicted"`
    HitRate      float64 `json:"hit_rate"`
    MemoryUsage  int64   `json:"estimated_memory_bytes"`
}

func (d *Dedupe[T, K]) Stats() DedupeStats {
    return d.cache.stats()
}
```

---

## Test Strategy

### Unit Test Coverage

**Core Functionality Tests:**
1. **Basic deduplication** - Same key filtered out
2. **Different keys pass through** - Unique items preserved
3. **Error pass-through** - Errors not deduplicated
4. **Key function panics** - Converted to errors
5. **Context cancellation** - Proper cleanup

**Memory Management Tests:**
1. **LRU eviction** - Old entries removed when full
2. **TTL expiration** - Old entries expire correctly
3. **Memory bounds** - Never exceed max size
4. **Cleanup timer** - Periodic TTL cleanup works

**Configuration Tests:**
1. **Fluent API** - All configuration options work
2. **Default values** - Sensible defaults applied
3. **Edge cases** - Zero/negative values handled

### Integration Test Scenarios

**Located in:** `testing/integration/dedupe_flow_test.go`

1. **High Cardinality Stress Test:**
   ```go
   func TestDedupeFlow_HighCardinality(t *testing.T) {
       // Test 1M unique keys with 10k cache
       // Verify memory stays bounded
       // Measure hit/miss rates
   }
   ```

2. **Mixed Success/Error Stream:**
   ```go
   func TestDedupeFlow_MixedResults(t *testing.T) {
       // Stream with successes and errors
       // Verify errors pass through immediately
       // Verify successes are deduplicated
   }
   ```

3. **Time-Based Expiration:**
   ```go
   func TestDedupeFlow_TTLExpiration(t *testing.T) {
       // Use FakeClock to advance time
       // Verify entries expire correctly
       // Test cleanup timer behavior
   }
   ```

### Performance Benchmarks

**Located in:** `testing/benchmarks/dedupe_bench_test.go`

1. **Throughput Benchmark:**
   ```go
   func BenchmarkDedupe_Throughput(b *testing.B) {
       // Measure items/second processing
       // Compare with OLD implementation
   }
   ```

2. **Memory Usage Benchmark:**
   ```go
   func BenchmarkDedupe_MemoryGrowth(b *testing.B) {
       // Verify bounded growth
       // Measure peak memory usage
   }
   ```

3. **Cache Performance:**
   ```go
   func BenchmarkDedupe_CacheOperations(b *testing.B) {
       // LRU add/contains/evict performance
       // Hit rate with different patterns
   }
   ```

---

## Configuration Defaults and Tuning

### Production-Ready Defaults

```go
const (
    DefaultMaxSize         = 10_000              // ~400KB memory
    DefaultTTL            = 0                    // No TTL (LRU only)
    DefaultCleanupInterval = 1 * time.Minute    // Cleanup frequency
    DefaultName           = "dedupe"
)
```

### Tuning Guidelines

**High Throughput, Low Cardinality:**
```go
// Example: Processing user events (limited user base)
dedupe := NewDedupe(getUserID, RealClock).
    WithMaxSize(1000).     // Small cache sufficient
    WithTTL(time.Hour)     // Long TTL for session-based dedup
```

**High Throughput, High Cardinality:**
```go
// Example: Processing unique device IDs
dedupe := NewDedupe(getDeviceID, RealClock).
    WithMaxSize(100000).        // Large cache
    WithTTL(15 * time.Minute).  // Short TTL to bound memory
    WithCleanupInterval(time.Minute) // Frequent cleanup
```

**Memory-Constrained Environment:**
```go
// Example: Edge computing with limited RAM
dedupe := NewDedupe(getRequestID, RealClock).
    WithMaxSize(1000).           // Very small cache
    WithTTL(30 * time.Second).   // Very short TTL
    WithCleanupInterval(10 * time.Second)
```

---

## Risk Analysis and Mitigation

### HIGH RISK: Key Function Performance

**Risk:** Complex key functions could become bottleneck
**Mitigation:** 
- Document performance requirements
- Provide benchmarking utilities
- Add panic recovery

### MEDIUM RISK: Hash Collisions

**Risk:** Different items with same key cause incorrect deduplication
**Mitigation:**
- Document key function requirements (must be unique)
- Consider adding collision detection in debug mode
- Provide guidance for composite keys

### MEDIUM RISK: Memory Estimation Accuracy

**Risk:** Memory usage estimation might be inaccurate for different key types
**Mitigation:**
- Use conservative estimates
- Provide tuning guides for different key types
- Add memory monitoring utilities

### LOW RISK: Clock Skew in Distributed Systems

**Risk:** TTL behavior inconsistent across nodes with different clocks
**Mitigation:**
- Document clock requirements
- Use monotonic time for TTL calculations
- Provide guidance for distributed deployments

---

## Integration with Existing Ecosystem

### Compatibility with New Result[T] Pattern

The dedupe processor follows the established Result[T] pattern:
- Input: `<-chan Result[T]`  
- Output: `<-chan Result[T]`
- Errors pass through immediately
- Success values are processed

### Usage Patterns

**Simple Deduplication:**
```go
// Deduplicate orders by ID
dedupe := streamz.NewDedupe(
    func(order Order) string { return order.ID },
    streamz.RealClock,
)

uniqueOrders := dedupe.Process(ctx, orderStream)
```

**Composite Key Deduplication:**
```go
// Deduplicate by user+action combination
dedupe := streamz.NewDedupe(
    func(event UserEvent) string { 
        return fmt.Sprintf("%s:%s", event.UserID, event.Action) 
    },
    streamz.RealClock,
).WithTTL(time.Hour)

uniqueEvents := dedupe.Process(ctx, eventStream)
```

**Pipeline Integration:**
```go
// Complete processing pipeline
pipeline := streamz.Compose(
    validator,                                    // Validate inputs
    mapper,                                       // Transform
    dedupe.WithMaxSize(10000).WithTTL(time.Hour), // Deduplicate
    batcher,                                      // Batch for efficiency
)

processed := pipeline.Process(ctx, rawEvents)
```

---

## Success Metrics

### Technical Metrics

1. **Memory Safety**: No unbounded growth under any configuration
2. **Performance**: Comparable or better than OLD implementation
3. **Reliability**: Zero race conditions, proper error handling
4. **Testability**: 100% deterministic tests with FakeClock

### Operational Metrics

1. **Hit Rate**: Cache effectiveness measurement
2. **Memory Usage**: Actual vs estimated memory consumption  
3. **Eviction Rate**: LRU eviction frequency
4. **Error Rate**: Key function panic frequency

### Adoption Metrics

1. **API Simplicity**: Easy migration from OLD implementation
2. **Configuration Flexibility**: Covers common use cases
3. **Documentation Quality**: Clear tuning guidance

---

## Implementation Timeline

**Phase 1 (Days 1-2):** Core implementation
- LRU cache with bounded memory
- Basic dedupe processor with Result[T]
- Unit tests for core functionality

**Phase 2 (Day 3):** Advanced features  
- TTL expiration with cleanup timer
- Fluent configuration API
- Comprehensive error handling

**Phase 3 (Day 4):** Testing and validation
- Integration tests with complex scenarios
- Performance benchmarks
- Memory usage validation

**Phase 4 (Day 5):** Documentation and polish
- Usage examples and tuning guides
- Performance comparison with OLD
- Migration documentation

---

## Conclusion

The new dedupe implementation addresses the CRITICAL memory leak in the OLD version while providing a more robust, configurable, and testable solution. By using bounded LRU cache with optional TTL, we eliminate the risk of production OOM failures while maintaining the flexibility needed for different use cases.

The single-goroutine design eliminates race conditions, and the Result[T] integration provides proper error handling. The implementation follows established patterns from the successful debounce migration, ensuring consistency across the streamz ecosystem.

**Bottom Line:** This is production-ready architecture that fixes the memory leak bug while improving testability, configurability, and operational visibility. It's the kind of solution that prevents 3 AM production outages.

---

*Architecture designed by midgel - tested in the fires of production debugging.*