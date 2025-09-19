# Window Processor Usage Pattern Analysis

## Executive Summary

Window processors return `<-chan Window[T]` making them terminal nodes. Breaks composition. Real-world usage shows windows feed downstream processors for aggregation, alerting, persistence. Design mismatch.

## Current Design

### Window Processor Signatures
```go
func (w *TumblingWindow[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan Window[T]
func (w *SlidingWindow[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan Window[T]  
func (w *SessionWindow[T]) Process(ctx context.Context, in <-chan Result[T]) <-chan Window[T]
```

Return `Window[T]` directly. Not `Result[Window[T]]`. Incompatible with all other processors.

## Real-World Usage Patterns Found

### 1. Window → Aggregation → Alert

**Evidence: examples/log-processing/pipeline.go:238-254**
```go
// Window logs for rate-based detection
windower := streamz.NewTumblingWindow[LogEntry](AlertWindow, streamz.RealClock)
windows := windower.Process(ctx, streams[1])

// Aggregate windows to detect spikes
aggregator := NewWindowAggregator(AlertWindow, ErrorRateThreshold, Alerting)
alerts := aggregator.Process(ctx, windows)

for alert := range alerts {
    if err := Alerting.SendAlert(ctx, alert); err == nil {
        MetricsCollector.IncrementAlerts(1)
    }
}
```

Pattern: Window → Custom Aggregator → Alert Service

### 2. Window → Threshold Check → Action

**Evidence: examples/log-processing/processors.go:92-159**
```go
func (wa *WindowAggregator) Process(ctx context.Context, in <-chan streamz.Window[LogEntry]) <-chan Alert {
    // Count errors by service
    errorCounts := make(map[string]int)
    
    for _, log := range window.Items {
        if log.Level == LogLevelError || log.Level == LogLevelCritical {
            errorCounts[log.Service]++
        }
    }
    
    // Generate alerts for services exceeding threshold
    for service, count := range errorCounts {
        if count >= wa.alertThreshold {
            // Create and send alert
        }
    }
}
```

Pattern: Window → Count/Sum → Threshold → Alert

### 3. Window → Metrics Calculation

**Evidence: examples/log-processing/processors.go:108-114**
```go
// Count errors and successes
successCount := w.SuccessCount()
errorCount := w.ErrorCount()
values := w.Values() // Only successful events
errors := w.Errors() // Only errors

log.Printf("Window [%s - %s]: %d events, %d errors, success rate: %.2f%%",
    w.Start.Format("15:04:05"),
    w.End.Format("15:04:05"),
    successCount, errorCount,
    float64(successCount)/float64(w.Count())*100)
```

Pattern: Window → Calculate Rates/Percentages → Log/Store

## Usage Patterns NOT Found

### Terminal Usage
No evidence of windows as final output. Always processed further:
- Never sent directly to log files
- Never returned to API responses raw
- Never used as final result

### Direct Storage
Windows not persisted as-is. Always transformed:
- Aggregated to metrics first
- Converted to alerts/events
- Summarized before storage

## Downstream Processing Requirements

### What Users Do With Windows

1. **Aggregation**
   - Sum/count values in window
   - Calculate averages/rates
   - Find min/max/percentiles
   - Group by attributes

2. **Alerting**
   - Check thresholds
   - Detect anomalies
   - Generate notifications
   - Escalate based on severity

3. **Further Routing**
   - Send metrics to monitoring
   - Store summaries to database
   - Forward to analytics pipelines
   - Distribute to multiple consumers

4. **Transformation**
   - Convert to different formats
   - Enrich with metadata
   - Combine with other streams
   - Filter based on content

## Composability Failures

### Cannot Use Standard Processors

```go
windows := tumbling.Process(ctx, events) // <-chan Window[T]

// ALL FAIL - Type mismatch
mapped := mapper.Process(ctx, windows)     // Expects Result[T]
filtered := filter.Process(ctx, windows)   // Expects Result[T]
throttled := throttle.Process(ctx, windows) // Expects Result[T]
fanned := fanout.Process(ctx, windows)     // Expects Result[T]
```

### Required Workarounds

Users must wrap manually:
```go
windows := tumbling.Process(ctx, events)

// Manual wrapper required
wrapped := make(chan Result[Window[Event]])
go func() {
    defer close(wrapped)
    for w := range windows {
        wrapped <- NewSuccess(w)
    }
}()

// Now can compose
mapped := mapper.Process(ctx, wrapped)
```

## Pattern Recognition

### Windows Are Intermediate, Not Terminal

Evidence shows windows always feed downstream:
1. Aggregation processors
2. Alert generators
3. Metric calculators
4. Storage transformers

Never terminal output.

### Custom Processors Required

Users create custom processors to handle `Window[T]`:
- `WindowAggregator` - processes windows
- `MetricsCollector` - extracts metrics
- Alert generators - threshold checking

Would be unnecessary if windows were `Result[Window[T]]`.

### Composition Desired

Multiple downstream paths common:
```go
windows → aggregator → alerts
       → metrics → storage
       → filter → special handling
```

Current design forces custom fan-out.

## Impact Analysis

### Current Pain Points

1. **Type Incompatibility**
   - Cannot chain with existing processors
   - Forces custom wrapper code
   - Breaks pipeline consistency

2. **Code Duplication**
   - Every user reimplements window handling
   - Custom aggregators everywhere
   - No reusable patterns

3. **Error Handling**
   - Window errors not captured
   - No way to signal window processing failure
   - Error context lost

### With Result[Window[T]] Pattern

1. **Full Composability**
   ```go
   windows := tumbling.Process(ctx, events) // <-chan Result[Window[T]]
   
   // Direct composition
   aggregated := mapper.Process(ctx, windows)
   alerts := filter.Process(ctx, aggregated)
   stored := tap.Process(ctx, alerts)
   ```

2. **Error Propagation**
   ```go
   for result := range windows {
       if result.IsError() {
           // Handle window processing error
       } else {
           window := result.Value()
           // Process window
       }
   }
   ```

3. **Standard Patterns**
   - Use existing processors
   - Leverage error handling
   - Maintain pipeline consistency

## Recommendation

### Change Window Processors to Return Result[Window[T]]

**Benefits:**
- Full composability with all processors
- Consistent error handling
- Eliminates custom wrappers
- Enables standard patterns

**Implementation:**
- Wrap Window[T] in Result
- Maintain window semantics
- Enable error propagation

### Migration Path

1. **Option A: Breaking Change**
   - Change signature directly
   - Update all usage
   - Clean, consistent

2. **Option B: Dual API**
   - Keep `Process()` → `Window[T]`
   - Add `ProcessResult()` → `Result[Window[T]]`
   - Gradual migration

## Conclusion

Windows are not terminal. Always processed further. Current design forces workarounds.

Users want:
- Aggregate window contents
- Generate alerts from windows
- Route windows to multiple processors
- Transform windows to metrics

All require composition. Window[T] prevents it. Result[Window[T]] enables it.

Terminal node assumption wrong. Windows are intermediate aggregations. Need composability.