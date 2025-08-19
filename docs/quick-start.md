# Quick Start Guide

Build your first streaming pipeline in 5 minutes!

## Your First Pipeline

Let's build a real-time log processing system that filters, enriches, and batches log entries for efficient storage.

```go
package main

import (
    "context"
    "fmt"
    "strings"
    "time"
    
    "github.com/zoobzio/streamz"
)

// Define your data types
type LogEntry struct {
    Level     string
    Message   string
    Timestamp time.Time
    Source    string
}

type EnrichedLog struct {
    LogEntry
    ProcessedAt time.Time
    WordCount   int
}

func main() {
    ctx := context.Background()
    
    // Create input stream
    logs := make(chan LogEntry)
    
    // Step 1: Filter out debug logs
    errorFilter := streamz.NewFilter(func(log LogEntry) bool {
        return log.Level == "ERROR" || log.Level == "WARN"
    }).WithName("errors-only")
    
    // Step 2: Enrich logs with metadata
    enricher := streamz.NewMapper(func(log LogEntry) EnrichedLog {
        return EnrichedLog{
            LogEntry:    log,
            ProcessedAt: time.Now(),
            WordCount:   len(strings.Fields(log.Message)),
        }
    }).WithName("enrich")
    
    // Step 3: Batch for efficient storage
    batcher := streamz.NewBatcher[EnrichedLog](streamz.BatchConfig{
        MaxSize:    100,               // Batch up to 100 logs
        MaxLatency: 5 * time.Second,   // Or every 5 seconds
    })
    
    // Step 4: Compose the pipeline
    filtered := errorFilter.Process(ctx, logs)
    enriched := enricher.Process(ctx, filtered)
    batched := batcher.Process(ctx, enriched)
    
    // Send sample data
    go func() {
        defer close(logs)
        
        for i := 0; i < 50; i++ {
            logs <- LogEntry{
                Level:     []string{"DEBUG", "INFO", "WARN", "ERROR"}[i%4],
                Message:   fmt.Sprintf("Log message %d with some content", i),
                Timestamp: time.Now(),
                Source:    "app-server",
            }
            time.Sleep(10 * time.Millisecond)
        }
    }()
    
    // Process batches
    for batch := range batched {
        fmt.Printf("📦 Received batch of %d logs\n", len(batch))
        for _, log := range batch {
            fmt.Printf("  %s: %s (words: %d)\n", 
                log.Level, log.Message, log.WordCount)
        }
        fmt.Println()
        
        // Here you would typically save the batch to database
        // saveBatchToDatabase(batch)
    }
    
    fmt.Println("✅ Pipeline completed!")
}
```

## What Just Happened?

1. **Type Safety**: The pipeline is fully type-checked. `chan LogEntry` → `chan LogEntry` → `chan EnrichedLog` → `chan []EnrichedLog`

2. **Automatic Cleanup**: All channels are properly closed when the input closes

3. **Efficient Batching**: Logs are batched either when 100 accumulate OR 5 seconds pass

4. **Composable**: Each step is independent and reusable

## Run It

```bash
go run main.go
```

You'll see output like:
```
📦 Received batch of 25 logs
  WARN: Log message 2 with some content (words: 6)
  ERROR: Log message 3 with some content (words: 6)
  WARN: Log message 6 with some content (words: 6)
  ...
```

## Add Monitoring

Let's add monitoring to see throughput:

```go
// Add this after the enricher
monitor := streamz.NewMonitor[EnrichedLog](time.Second).OnStats(func(stats streamz.StreamStats) {
    fmt.Printf("📊 Processing %.1f logs/sec\n", stats.Rate)
})

// Update the pipeline
filtered := errorFilter.Process(ctx, logs)
enriched := enricher.Process(ctx, filtered)
monitored := monitor.Process(ctx, enriched)  // Add monitoring
batched := batcher.Process(ctx, monitored)
```

## Add Concurrent Processing

For CPU-intensive enrichment, add concurrency while preserving order:

```go
// Replace the simple mapper with async processing
asyncEnricher := streamz.NewAsyncMapper(func(ctx context.Context, log LogEntry) (EnrichedLog, error) {
    // Simulate expensive processing
    time.Sleep(time.Millisecond)
    
    return EnrichedLog{
        LogEntry:    log,
        ProcessedAt: time.Now(),
        WordCount:   len(strings.Fields(log.Message)),
    }, nil
}).WithWorkers(10)

// Use it in the pipeline
enriched := asyncEnricher.Process(ctx, filtered)
```

## Real-World Patterns

### Backpressure Handling

```go
// Add buffer to handle bursts
buffer := streamz.NewBuffer[LogEntry](10000)
buffered := buffer.Process(ctx, logs)
filtered := errorFilter.Process(ctx, buffered)
```

### Error Recovery

```go
// Skip malformed logs instead of failing
validator := streamz.NewFilter(func(log LogEntry) bool {
    return log.Message != "" && log.Timestamp != (time.Time{})
}).WithName("valid")
```

### Sampling for Monitoring

```go
// Sample 1% of logs for detailed analysis
sampler := streamz.NewSample[LogEntry](0.01)
sample := sampler.Process(ctx, logs)
// Send sample to monitoring system
```

## Key Takeaways

1. **Start Simple**: Basic filters and transformations
2. **Compose Gradually**: Add processors as needed
3. **Type Safety**: Compiler catches pipeline mismatches
4. **Monitor Everything**: Add monitoring without changing logic
5. **Handle Backpressure**: Use buffers for burst traffic

## What's Next?

- **[Core Concepts](./concepts/processors.md)**: Understand how processors work
- **[Composition Patterns](./concepts/composition.md)**: Advanced pipeline patterns
- **[Performance Guide](./guides/performance.md)**: Optimize for production workloads
- **[Common Patterns](./guides/patterns.md)**: Real-world streaming patterns

Ready to build production systems? Check out the [Best Practices Guide](./guides/best-practices.md)!