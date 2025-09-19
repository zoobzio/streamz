# Missing Processors Analysis for streamz

## Integration Context

Examined current state. Example pipelines reference processors that don't exist. Production patterns need them.

Current processors:
- `Throttle` - Rate limiting
- `Debounce` - Consolidate rapid events
- `Batcher` - Group items
- `Buffer` - Basic buffering
- `FanIn` - Merge channels
- `FanOut` - Split channels
- `AsyncMapper` - Transform with async operations
- Windows: `TumblingWindow`, `SlidingWindow`, `SessionWindow`

## Critical Missing Processors

### 1. Filter
**Pattern:** `streamz.NewFilter(func(log LogEntry) bool)`
**Use:** Error detection, critical alerts, selective processing
**Evidence:** Lines 173-176, 257-259, 357-359, 495-497 in example
**Why needed:** Can't selectively process without filtering. Every pipeline needs this.

### 2. Mapper (Synchronous)
**Pattern:** `streamz.NewMapper[In, Out](func(In) Out)`
**Use:** Transform types, extract fields, normalize data
**Evidence:** AsyncMapper exists but not sync version
**Why needed:** Not all transformations need async overhead. Simple field extraction common.

### 3. Dedupe
**Pattern:** `streamz.NewDedupe(keyFunc, clock).WithTTL(duration)`
**Use:** Alert deduplication, duplicate event suppression
**Evidence:** Lines 340-342, 477-480 in example
**Why needed:** Alert fatigue real problem. Same error triggers multiple alerts.

### 4. Monitor
**Pattern:** `streamz.NewMonitor[T](interval, clock).OnStats(func(stats))`
**Use:** Observability, adaptive behavior, rate monitoring
**Evidence:** Lines 388-398 in example
**Why needed:** Can't manage backpressure without measuring flow.

### 5. Sample
**Pattern:** `streamz.NewSample[T](rate)`
**Use:** Load shedding, statistical sampling
**Evidence:** Lines 413-414 in example  
**Why needed:** When overwhelmed, controlled degradation better than crash.

### 6. DLQ (Dead Letter Queue)
**Pattern:** Route errors to separate channel
**Use:** Error recovery, failed item reprocessing
**Evidence:** Common pattern when Result[T].IsError()
**Why needed:** Errors need separate handling path. Can't just drop them.

### 7. Switch/Router
**Pattern:** `streamz.NewSwitch[T](func(T) string)` routes to different outputs
**Use:** Content-based routing, service dispatch
**Evidence:** Different log levels need different handling
**Why needed:** One stream → multiple specialized processors.

### 8. Retry
**Pattern:** `streamz.NewRetry[T](maxAttempts, backoff)`
**Use:** Transient failure handling
**Evidence:** Database writes fail transiently
**Why needed:** Network calls fail. Need automatic retry with backoff.

### 9. CircuitBreaker
**Pattern:** Circuit breaker for failing services
**Use:** Prevent cascade failures
**Evidence:** External service dependencies
**Why needed:** When downstream fails, need to stop hammering it.

### 10. Tap
**Pattern:** `streamz.NewTap[T](func(T))` for side effects
**Use:** Logging, metrics, debugging
**Evidence:** MetricsCollector pattern throughout
**Why needed:** Observe without modifying stream.

## Secondary Processors (Less Critical)

### 11. Flatten
**Pattern:** `streamz.NewFlatten[T]()` for []T → T
**Use:** Unbatch, expand arrays
**Evidence:** Opposite of Batcher needed
**Why needed:** Some APIs return arrays. Need to process individually.

### 12. Chunk
**Pattern:** Fixed-size chunks (vs time-based batching)
**Use:** Pagination, fixed-size processing
**Evidence:** Database batch size limits
**Why needed:** Some systems have hard size limits.

### 13. Take
**Pattern:** `streamz.NewTake[T](n)` first N items
**Use:** Testing, sampling, limits
**Evidence:** Development/debugging scenarios
**Why needed:** Process subset for testing.

### 14. Skip
**Pattern:** `streamz.NewSkip[T](n)` skip first N
**Use:** Header skipping, warmup bypass
**Evidence:** File processing patterns
**Why needed:** Initial items often different (headers, warmup).

## Missing Error Handling Patterns

### Result[T] Integration Gaps

1. **Error Router** - Route Result[T].IsError() to error channel
2. **Error Aggregator** - Collect errors over time window
3. **Error Transformer** - Convert StreamError to alerts/metrics

Current Result[T] type good. But no processors that leverage it properly.

## Production Requirements Not Met

### Backpressure Handling
Missing:
- `DroppingBuffer` - Drop oldest when full
- `SlidingBuffer` - Keep most recent N items
- Adaptive sampling based on queue depth

### Monitoring/Observability  
Missing:
- Rate monitoring
- Latency tracking
- Queue depth metrics
- Error rate calculation

### Recovery Patterns
Missing:
- Retry with exponential backoff
- Circuit breaker for cascade prevention
- Dead letter queue for failed items
- Timeout wrapper for slow operations

## Priority Order (Based on Example Usage)

1. **Filter** - Used everywhere, fundamental
2. **Mapper** - Basic transformation
3. **Dedupe** - Alert deduplication critical
4. **Monitor** - Can't manage load without metrics
5. **Sample** - Load shedding essential
6. **DLQ** - Error handling path
7. **Retry** - Transient failure handling
8. **Switch/Router** - Content-based routing
9. **Tap** - Observability
10. **CircuitBreaker** - Prevent cascades

## Integration Test Requirements

Each processor needs:
1. **Boundary tests** - How it handles Result[T] errors
2. **Composition tests** - Works with other processors
3. **Backpressure tests** - Behavior when downstream blocks
4. **Cancellation tests** - Context cancellation handling
5. **Concurrent safety** - Race conditions

## Implementation Notes

### Processors Should Follow Pattern

```go
type ProcessorName[T any] struct {
    // config fields
}

func (p *ProcessorName[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan Result[T] {
    out := make(chan Result[T])
    go func() {
        defer close(out)
        for {
            select {
            case item, ok := <-in:
                if !ok {
                    return
                }
                if item.IsError() {
                    out <- item // Pass through errors
                    continue
                }
                // Process item.Value()
                // Send to out
            case <-ctx.Done():
                return
            }
        }
    }()
    return out
}
```

### Critical Patterns

1. **Always handle Result[T].IsError()** - Pass through or handle explicitly
2. **Respect context cancellation** - Check ctx.Done()
3. **Close output when input closes** - Prevent goroutine leaks
4. **Non-blocking sends** - Use select with ctx.Done()
5. **Preserve StreamError metadata** - Don't lose error context

## Conclusion

Library has foundation but missing critical processors for production use. Filter, Mapper, Dedupe are fundamental. Monitor, Sample, DLQ essential for production. Retry, CircuitBreaker prevent cascading failures.

Focus on processors that solve real problems seen in the example pipeline. Not academic completeness.