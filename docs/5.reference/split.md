---
title: Split
description: Separate items by condition into two streams
author: zoobzio
published: 2025-01-09
updated: 2025-01-09
tags:
  - reference
  - processors
  - routing
---

# Split

The Split processor divides a stream into exactly two outputs based on a predicate function. Items for which the predicate returns true go to the True output, while items for which it returns false go to the False output.

## Overview

Split is simpler than Router when you need binary classification and is more explicit about having exactly two outputs. It's ideal for:
- Valid/invalid separation
- Pass/fail classification
- True/false filtering with both outputs needed
- Binary decision trees

## Basic Usage

```go
import (
    "context"
    "github.com/zoobzio/streamz"
)

// Split orders into high/low value
splitter := streamz.NewSplit[Order](func(o Order) bool {
    return o.Total > 1000
})

outputs := splitter.Process(ctx, orders)

// Process high value orders with priority
go processHighValue(outputs.True)

// Process normal orders
go processNormal(outputs.False)
```

## Configuration Options

### WithBufferSize

Sets the buffer size for both output channels:

```go
splitter := streamz.NewSplit[Request](predicate).
    WithBufferSize(100) // Buffer up to 100 items per output
```

### WithName

Sets a custom name for monitoring:

```go
splitter := streamz.NewSplit[Event](predicate).
    WithName("priority-splitter")
```

## Usage Patterns

### Valid/Invalid Separation

```go
// Split valid and invalid records
validator := streamz.NewSplit[Record](func(r Record) bool {
    return r.IsValid()
}).WithName("validator")

outputs := validator.Process(ctx, records)

// Process valid records
go func() {
    for record := range outputs.True {
        database.Insert(record)
    }
}()

// Handle invalid records
go func() {
    for record := range outputs.False {
        errorLog.Record(record)
    }
}()
```

### Priority Classification

```go
// Split high-priority items
prioritizer := streamz.NewSplit[Task](func(t Task) bool {
    return t.Priority >= HighPriority || t.Deadline.Before(time.Now().Add(1*time.Hour))
}).WithBufferSize(50)

outputs := prioritizer.Process(ctx, tasks)

// Handle with different worker pools
urgentWorkerPool.Process(outputs.True)
normalWorkerPool.Process(outputs.False)
```

### A/B Testing

```go
// Split traffic for A/B testing
abSplitter := streamz.NewSplit[Request](func(r Request) bool {
    // Send 20% to variant B
    hash := fnv.New32a()
    hash.Write([]byte(r.UserID))
    return hash.Sum32()%100 < 20
})

outputs := abSplitter.Process(ctx, requests)

// Route to different handlers
variantB.Handle(outputs.True)   // 20% traffic
variantA.Handle(outputs.False)  // 80% traffic
```

## Advanced Examples

### Error Handling Split

```go
// Split successful and failed operations
resultSplitter := streamz.NewSplit[ProcessResult](func(r ProcessResult) bool {
    return r.Error == nil
}).WithName("result-splitter")

outputs := resultSplitter.Process(ctx, results)

// Handle successes
go func() {
    for success := range outputs.True {
        metrics.RecordSuccess(success)
        notifySuccess(success)
    }
}()

// Handle failures
go func() {
    for failure := range outputs.False {
        metrics.RecordError(failure.Error)
        retryQueue.Add(failure)
    }
}()
```

### Threshold-Based Splitting

```go
// Split based on dynamic threshold
thresholdSplitter := streamz.NewSplit[Metric](func(m Metric) bool {
    threshold := getThreshold(m.Type)
    return m.Value > threshold
}).WithBufferSize(100)

outputs := thresholdSplitter.Process(ctx, metrics)

// Alert on anomalies
go alertOnAnomalies(outputs.True)

// Log normal values
go logNormalMetrics(outputs.False)
```

### Chaining with Other Processors

```go
// Filter, then split the remaining items
filter := streamz.NewFilter[Order](func(o Order) bool {
    return o.Status == "pending"
})

splitter := streamz.NewSplit[Order](func(o Order) bool {
    return o.CustomerType == "premium"
})

// Chain processors
filtered := filter.Process(ctx, orders)
outputs := splitter.Process(ctx, filtered)

// Different processing for premium vs regular
premiumProcessor.Process(ctx, outputs.True)
regularProcessor.Process(ctx, outputs.False)
```

## Monitoring and Statistics

### Getting Statistics

```go
stats := splitter.GetStats()
fmt.Printf("Total items: %d\n", stats.TotalItems)
fmt.Printf("True output: %d (%.2f%%)\n", stats.TrueCount, stats.TrueRatio*100)
fmt.Printf("False output: %d (%.2f%%)\n", stats.FalseCount, stats.FalseRatio*100)
```

### Real-time Monitoring

```go
// Monitor split distribution
go func() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        stats := splitter.GetStats()
        log.Printf("Split distribution - True: %.1f%%, False: %.1f%%",
            stats.TrueRatio*100, stats.FalseRatio*100)
            
        // Alert on unexpected distributions
        if stats.TrueRatio > 0.9 || stats.TrueRatio < 0.1 {
            alert("Unusual split distribution detected")
        }
    }
}()
```

## Performance Considerations

1. **Buffer Size**: Set appropriate buffer sizes to handle processing speed differences
2. **Predicate Complexity**: Keep predicate functions simple and fast
3. **Concurrent Consumption**: Always consume both outputs concurrently to avoid blocking
4. **Memory Usage**: Each output channel has its own buffer

## Best Practices

1. **Always Consume Both Outputs**: Failing to consume one output will block the processor
2. **Use Meaningful Names**: Name your splitter to indicate what the split represents
3. **Monitor Distribution**: Track the split ratio to detect unexpected patterns
4. **Handle Context Cancellation**: Ensure consumers respect context cancellation
5. **Consider Buffer Sizes**: Balance between memory usage and blocking prevention

## Common Pitfalls

1. **Single Consumer**: Consuming outputs sequentially instead of concurrently
2. **Unbuffered Channels**: Can cause blocking if consumers are slow
3. **Complex Predicates**: Expensive predicate functions slow down processing
4. **Ignoring One Output**: Not consuming one output causes the processor to block

## Comparison with Router

Use Split when:
- You need exactly two outputs
- The decision is binary (true/false)
- You want explicit True/False semantics
- Simpler API is preferred

Use Router when:
- You need more than two outputs
- Routing logic is complex
- You need named routes
- Items might match multiple routes