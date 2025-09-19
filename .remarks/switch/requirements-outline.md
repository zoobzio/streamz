# Switch Connector Requirements - streamz

## Executive Summary

Design predicate-based routing system for streamz that routes input channel to multiple output channels based on runtime evaluation. Reference pipz patterns with streamz Result[T] integration and channel semantics.

## Core Requirements

### Basic Architecture

**Channel-based routing:**
- Input: `<-chan Result[T]`  
- Outputs: `map[K]<-chan Result[T]` where K is comparable route key
- Predicate: `func(T) K` evaluates successful values only
- Routes defined at construction or runtime modification

**Key differences from pipz:**
- Channel semantics vs single value processing
- Result[T] integration vs direct value handling
- Multiple concurrent outputs vs single processor selection
- Stream splitting vs conditional processing

### Predicate Evaluation Patterns

**Evaluation scope:**
- Success values only: `result.Value()` passed to predicate
- Error Results: Route to special error handling channel or broadcast
- Metadata consideration: Optional predicate access to `result.GetMetadata()`

**Route key types:**
- Any comparable type (string, int, custom enums)
- Type safety through generics: `Switch[T, K comparable]`
- No route found: Configurable behavior (drop, default route, error)

**Example patterns:**
```go
// Business state routing
type PaymentRoute string
const (
    RouteStandard   PaymentRoute = "standard"
    RouteHighValue  PaymentRoute = "high_value" 
    RouteFraud      PaymentRoute = "fraud"
)

// Priority-based routing  
func priorityRouter(order Order) int {
    return order.Priority // 1=high, 2=medium, 3=low
}

// Metadata-aware routing
func regionRouter(result Result[Event]) string {
    if region, found, _ := result.GetStringMetadata("region"); found {
        return region
    }
    return "default"
}
```

### Result[T] Metadata Integration

**Metadata preservation:**
- Route decisions may depend on metadata
- All metadata preserved through routing
- Additional routing metadata added

**Standard metadata keys:**
```go
const (
    MetadataRoute     = "route"      // string - route key taken
    MetadataTimestamp = "timestamp"  // time.Time - routing timestamp  
    MetadataProcessor = "processor"  // string - "switch" 
)
```

**Error routing:**
- StreamError[T] passed to designated error channel
- Error metadata preserved
- Switch errors (predicate panics) create new StreamError[T]

### Channel Management

**Output channel lifecycle:**
- Channels created for each route key at first use
- Channels closed when switch terminates
- Route addition/removal doesn't affect existing channels

**Backpressure handling:**
- Slow consumers block specific routes, not entire switch
- Context cancellation terminates all routing
- Optional buffering per route channel

**Concurrency model:**
- Single goroutine reads input channel
- Route evaluation serialized (no concurrent predicate calls)
- Multiple goroutines write to output channels
- Thread-safe route modification during operation

### Performance Characteristics

**Evaluation efficiency:**
- Predicate called once per successful Result[T]
- Route lookup: O(1) map access
- No reflection or string parsing in hot path
- Struct-based route keys preferred over strings

**Memory patterns:**
- Output channels created lazily
- Route map grows dynamically
- No buffering unless explicitly configured
- Metadata copying minimal overhead

**Benchmarking targets:**
- 100k+ messages/second with 10 routes
- <1ms p99 latency for route evaluation
- <100MB memory for 1M messages with 50 routes

### Error Handling Patterns

**Predicate errors:**
- Panic recovery: Create StreamError[T] with original value
- Return zero value K: Route to default channel or drop
- Non-comparable K: Compile-time prevention

**Route not found:**
- Configurable: Drop, default route, error Result[T]
- Log at DEBUG level, not ERROR (expected condition)
- Metrics: Count of unrouted messages per route key

**Channel errors:**
- Context cancellation: Clean shutdown all routes
- Output channel full: Backpressure to input (blocking)
- Channel close during operation: Panic prevention

### Integration Points

**Result[T] patterns:**
- Error passthrough to error route or broadcast
- Metadata enhancement with routing information
- Window metadata preservation through routing

**Existing connectors:**
- Compose with FanOut for route duplication
- Chain with Filter for pre-routing filtering  
- Integrate with Mapper for data transformation before routing
- Compatible with window processors on output routes

**Monitoring integration:**
- Route hit counts per key
- Predicate evaluation timing
- Unrouted message tracking
- Error rate per route

## Implementation Phases

### Phase 1: Core Implementation
- Basic Switch[T, K] with static routes
- Predicate evaluation for success values
- Error Result[T] passthrough
- Context-aware termination

### Phase 2: Dynamic Routing
- Runtime route addition/removal
- Thread-safe route modification
- Route existence checking
- Default route configuration

### Phase 3: Advanced Features  
- Metadata-aware predicates
- Per-route buffering options
- Route statistics and monitoring
- Performance optimizations

### Phase 4: Integration & Testing
- Comprehensive integration tests
- Performance benchmarks
- Example implementations
- Documentation and patterns

## Key Design Decisions

**Route key type safety:**
Using comparable generics prevents runtime route key errors and enables custom business types.

**Channel splitting vs processor selection:**
Unlike pipz single-processor selection, streamz switch creates multiple output channels for concurrent stream processing.

**Error routing strategy:**
Errors bypass predicate evaluation and route to designated error channel, maintaining stream processing semantics.

**Backpressure model:**
Per-route backpressure prevents slow consumers from blocking unrelated routes while maintaining data integrity.

**Metadata preservation:**
Full metadata preservation with routing enhancement supports complex pipeline compositions and debugging.

## Compatibility Matrix

| Component | Integration | Notes |
|-----------|-------------|-------|
| Result[T] | Native | Core integration |
| FanOut | Compose | Route duplication |
| Filter | Chain | Pre-routing filter |
| Mapper | Chain | Transform before route |
| Batcher | Output | Route-specific batching |
| Window | Output | Windowing per route |
| DLQ | Error | Failed routing handling |

## Success Criteria

- Type-safe route key system
- <1ms p99 routing latency
- 100k+ msg/sec throughput
- Zero data loss under backpressure
- Clean integration with existing streamz patterns
- Comprehensive error handling and recovery