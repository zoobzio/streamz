# Introduction to streamz

## The Problem with Raw Channels

Go channels are powerful, but building robust stream processing systems with raw channels is surprisingly difficult:

```go
// Naive approach - what could go wrong?
func processLogs(logs <-chan LogEntry) <-chan ProcessedLog {
    out := make(chan ProcessedLog)
    
    go func() {
        for log := range logs {
            // What if processing fails?
            // What about backpressure?
            // How do we batch for efficiency?
            // What if we need to add monitoring?
            processed := transform(log)
            out <- processed // This can block forever!
        }
        // Did we remember to close the channel?
    }()
    
    return out
}
```

**Common pitfalls:**
- Goroutine leaks from forgetting to close channels
- Deadlocks from blocking sends
- No standardized error handling
- Difficult to add concerns like monitoring, batching, or rate limiting
- Complex cleanup logic
- Hard to test and reason about

## The streamz Solution

streamz provides **type-safe, composable processors** that handle these concerns correctly:

```go
// With streamz - robust and composable
func processLogs(ctx context.Context, logs <-chan LogEntry) <-chan ProcessedLog {
    transformer := streamz.NewMapper("transform", transformLog)
    return transformer.Process(ctx, logs)
    // ✅ Proper channel lifecycle
    // ✅ Context cancellation
    // ✅ Type safety
    // ✅ Composable with other processors
}
```

## Why streamz?

### 🔒 **Type Safety**
Full compile-time type checking with Go generics. No runtime type assertions or reflection.

```go
// Compile-time error if types don't match
batcher := streamz.NewBatcher[Order](config)    // chan Order -> chan []Order
unbatcher := streamz.NewUnbatcher[Order]()      // chan []Order -> chan Order
filter := streamz.NewFilter("valid", isValid)   // chan Order -> chan Order

// This won't compile - type mismatch
result := unbatcher.Process(ctx, batcher.Process(ctx, orders)) // ✅ Works
wrong := filter.Process(ctx, batcher.Process(ctx, orders))     // ❌ Compile error
```

### 🔧 **Composable**
Build complex pipelines from simple, reusable parts:

```go
// Each processor is independently testable
validator := streamz.NewFilter("valid", validateOrder)
batcher := streamz.NewBatcher[Order](batchConfig)
enricher := streamz.NewAsyncMapper(10, enrichOrder)
monitor := streamz.NewMonitor[Order](time.Second, logThroughput)

// Compose into sophisticated pipeline
pipeline := func(ctx context.Context, orders <-chan Order) <-chan []EnrichedOrder {
    validated := validator.Process(ctx, orders)
    monitored := monitor.Process(ctx, validated)
    enriched := enricher.Process(ctx, monitored)
    return batcher.Process(ctx, enriched)
}
```

### ⚡ **Battle-Tested Patterns**
Built-in solutions for common streaming challenges:

- **Backpressure**: Natural through channel semantics + explicit buffers
- **Error Handling**: Graceful degradation (skip errors) or custom strategies
- **Concurrency**: Order-preserving async processing, fan-in/fan-out
- **Flow Control**: Rate limiting, debouncing, sampling
- **Observability**: Built-in monitoring without performance impact

### 🚀 **Performance**
Optimized for production workloads:

- Minimal allocations in hot paths
- Efficient memory management
- No reflection or runtime type checks
- Configurable buffering strategies

```go
// Benchmark: 1M items processed
BenchmarkStreamz-8    1000000    1205 ns/op    48 B/op    1 allocs/op
BenchmarkRawChannels-8 1000000   1850 ns/op   112 B/op    3 allocs/op
```

### 🛡️ **Production Ready**
Handle real-world streaming challenges:

```go
// Handle burst traffic with dropping buffer
buffer := streamz.NewDroppingBuffer[Event](10000)

// Deduplicate within time window
deduper := streamz.NewDedupe(getID, 5*time.Minute)

// Monitor throughput and latency
monitor := streamz.NewMonitor[Event](time.Second, func(stats StreamStats) {
    metrics.RecordThroughput(stats.Rate)
    metrics.RecordLatency(stats.AvgLatency)
})

// Concurrent processing with order preservation
processor := streamz.NewAsyncMapper(100, processEvent)
```

## When to Use streamz

### ✅ **Perfect For:**
- Real-time data processing pipelines
- API request/response processing with batching
- Log processing and analytics
- IoT telemetry data handling
- Event-driven architectures
- Microservice communication patterns

### ❓ **Consider Alternatives:**
- **Simple transformations**: Raw channels might be simpler
- **Batch processing**: Consider traditional batch frameworks
- **Complex state management**: Consider actor systems or state machines
- **Guaranteed delivery**: Consider message queues with persistence

## Core Philosophy

streamz follows several key principles:

1. **Simplicity**: Each processor has one clear responsibility
2. **Composability**: Processors combine naturally into complex systems
3. **Type Safety**: Catch errors at compile time, not runtime
4. **Performance**: Optimized for high-throughput, low-latency scenarios
5. **Testability**: Every component is independently testable
6. **Production Ready**: Handle edge cases, errors, and resource management correctly

## What's Next?

Ready to build your first pipeline? Check out the [Quick Start Guide](./quick-start.md) to get up and running in 5 minutes.

Want to understand the core concepts? Start with [Processors](./concepts/processors.md) to learn how streamz components work.