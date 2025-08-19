# Batcher

Accumulates items from a stream and groups them into batches based on size or time constraints.

## Overview

The Batcher processor collects individual items and emits them as slices when either a size threshold is reached or a time limit expires, whichever comes first. This dual-trigger approach balances throughput with latency.

**Type Signature:** `chan T → chan []T`

## When to Use

- **Database operations**: Bulk inserts/updates for efficiency
- **API calls**: Reduce request count by batching
- **Micro-batching**: Stream processing with small batches
- **Cost optimization**: Batch operations to reduce per-operation costs
- **Network efficiency**: Reduce network overhead

## Constructor

```go
func NewBatcher[T any](config BatchConfig) *Batcher[T]
```

### BatchConfig

```go
type BatchConfig struct {
    // MaxSize is the maximum number of items in a batch.
    // A batch is emitted immediately when it reaches this size.
    MaxSize int
    
    // MaxLatency is the maximum time to wait before emitting a partial batch.
    // If set, a batch will be emitted after this duration even if it's not full.
    MaxLatency time.Duration
}
```

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `MaxSize` | `int` | Yes | Maximum items per batch (triggers immediate emission) |
| `MaxLatency` | `time.Duration` | No | Maximum wait time for partial batches |

## Examples

### Basic Batching

```go
// Batch up to 100 items, no time limit
batcher := streamz.NewBatcher[Order](streamz.BatchConfig{
    MaxSize: 100,
})

batches := batcher.Process(ctx, orders)
for batch := range batches {
    // Process batches of up to 100 orders
    fmt.Printf("Processing batch of %d orders\n", len(batch))
    bulkInsertOrders(batch)
}
```

### Time-Based Batching

```go
// Emit batch every 5 seconds, regardless of size
batcher := streamz.NewBatcher[LogEntry](streamz.BatchConfig{
    MaxSize:    1000,         // Max size (may not be reached)
    MaxLatency: 5 * time.Second, // Emit every 5 seconds
})

batches := batcher.Process(ctx, logs)
for batch := range batches {
    // Batches emitted every 5 seconds
    saveLogs(batch)
}
```

### API Request Batching

```go
// Optimize API calls with smart batching
batcher := streamz.NewBatcher[APIRequest](streamz.BatchConfig{
    MaxSize:    10,                    // API accepts max 10 items
    MaxLatency: 100 * time.Millisecond, // Low latency for real-time feel
})

batches := batcher.Process(ctx, requests)
for batch := range batches {
    // Efficient API calls with good latency
    response := apiClient.BatchProcess(batch)
    handleResponse(response)
}
```

### Database Bulk Operations

```go
// Efficient database operations
batcher := streamz.NewBatcher[User](streamz.BatchConfig{
    MaxSize:    500,         // Optimal batch size for database
    MaxLatency: 10 * time.Second, // Don't wait too long
})

batches := batcher.Process(ctx, users)
for batch := range batches {
    // Bulk insert for efficiency
    if err := db.BulkInsert(batch); err != nil {
        log.Error("Bulk insert failed", "error", err, "count", len(batch))
    }
}
```

## Pipeline Integration

### With Validation

```go
// Validate before batching for efficiency
validator := streamz.NewFilter("valid", isValidOrder)
batcher := streamz.NewBatcher[Order](streamz.BatchConfig{
    MaxSize:    100,
    MaxLatency: time.Second,
})

validated := validator.Process(ctx, orders)
batches := batcher.Process(ctx, validated)
```

### With Unbatching

```go
// Batch → Process → Unbatch pattern
batcher := streamz.NewBatcher[Order](config)
batchProcessor := streamz.NewMapper("process-batch", processBatch)
unbatcher := streamz.NewUnbatcher[ProcessedOrder]()

batched := batcher.Process(ctx, orders)
processed := batchProcessor.Process(ctx, batched)
individual := unbatcher.Process(ctx, processed)
```

### With Monitoring

```go
// Monitor batching effectiveness
batcher := streamz.NewBatcher[Event](config)
monitor := streamz.NewMonitor[[]Event](time.Minute, func(stats streamz.StreamStats) {
    log.Info("Batch stats", "rate", stats.Rate, "avg_latency", stats.AvgLatency)
})

batched := batcher.Process(ctx, events)
monitored := monitor.Process(ctx, batched)
```

## Performance Characteristics

### Memory Usage
- **Memory per batch**: `sizeof(T) * MaxSize`
- **Total memory**: Approximately `sizeof(T) * MaxSize * 2` (one building, one ready)
- **Cleanup**: Automatic when context is cancelled

### Latency
- **Best case**: Immediate when batch is full
- **Worst case**: `MaxLatency` duration
- **Typical**: Depends on input rate vs batch size

### Throughput
- **Optimal batch size**: Balance between latency and efficiency
- **Larger batches**: Better throughput, higher latency
- **Smaller batches**: Lower latency, reduced throughput

## Configuration Guidelines

### Choosing MaxSize

```go
// For database operations (optimize for bulk operations)
MaxSize: 1000

// For API calls (respect API limits)  
MaxSize: 100

// For real-time processing (small batches for low latency)
MaxSize: 10

// For analytics (larger batches for efficiency)
MaxSize: 10000
```

### Choosing MaxLatency

```go
// Real-time systems (low latency priority)
MaxLatency: 100 * time.Millisecond

// Interactive systems (balance latency/efficiency)
MaxLatency: time.Second

// Batch processing (efficiency priority)
MaxLatency: 30 * time.Second

// No time limit (only size-based)
MaxLatency: 0 // Or omit field
```

## Error Handling

The Batcher processor handles errors gracefully:

- **Context cancellation**: Stops immediately, may emit partial batch
- **Channel closure**: Emits final partial batch if any items pending
- **Memory pressure**: No built-in protection (monitor memory usage)

```go
// Handle context cancellation gracefully
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

batches := batcher.Process(ctx, orders)
for batch := range batches {
    // Process batches until context timeout
    processBatch(batch)
}
// Final partial batch is emitted automatically
```

## Common Patterns

### Conditional Batching

```go
// Different batch sizes based on priority
highPriorityBatcher := streamz.NewBatcher[Order](streamz.BatchConfig{
    MaxSize:    10,                   // Smaller batches
    MaxLatency: 100 * time.Millisecond, // Faster processing
})

normalBatcher := streamz.NewBatcher[Order](streamz.BatchConfig{
    MaxSize:    100,        // Larger batches  
    MaxLatency: 5 * time.Second, // Can wait longer
})
```

### Multi-Level Batching

```go
// First level: Small batches for responsiveness
level1 := streamz.NewBatcher[Event](streamz.BatchConfig{
    MaxSize:    10,
    MaxLatency: 100 * time.Millisecond,
})

// Second level: Larger batches for efficiency
level2 := streamz.NewBatcher[[]Event](streamz.BatchConfig{
    MaxSize:    10, // 10 batches = up to 100 events
    MaxLatency: time.Second,
})

batched1 := level1.Process(ctx, events)
batched2 := level2.Process(ctx, batched1)
```

## Best Practices

1. **Choose appropriate batch sizes** based on downstream system capabilities
2. **Set reasonable latency limits** to prevent indefinite delays
3. **Monitor batch effectiveness** - track batch sizes and processing times
4. **Handle partial batches** properly when context is cancelled
5. **Consider memory usage** for large batch sizes with high-memory items

## Related Processors

- **[Unbatcher](./unbatcher.md)**: Reverse operation (slice → individual items)
- **[Chunk](./chunk.md)**: Fixed-size grouping without time constraints
- **[Buffer](./buffer.md)**: Add buffering without grouping
- **[AsyncMapper](./async-mapper.md)**: Process batches concurrently

## See Also

- **[Concepts: Composition](../concepts/composition.md)**: Learn about pipeline composition
- **[Guides: Patterns](../guides/patterns.md)**: Real-world batching patterns
- **[Guides: Performance](../guides/performance.md)**: Optimize batch processing