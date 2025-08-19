# streamz

Type-safe, composable stream processing primitives for Go channels, enabling real-time data processing through batching, windowing, and other streaming operations.

Build robust streaming pipelines that are easy to test, reason about, and maintain.

## Why streamz?

- **Type-safe**: Full compile-time type checking with Go generics
- **Composable**: Build complex pipelines from simple, reusable parts
- **Zero dependencies**: Just standard library
- **Battle-tested patterns**: Backpressure, error recovery, flow control built-in
- **Production ready**: Handle edge cases, errors, and resource management correctly
- **Fast**: Minimal allocations, optimized for performance
- **Observable**: Built-in monitoring without performance impact

**Common problems streamz solves:**
- Goroutine leaks from improper channel cleanup
- Deadlocks from blocking channel operations
- Complex backpressure handling
- Adding monitoring, batching, or rate limiting to existing streams
- Type safety in multi-stage processing pipelines

## Quick Start

```go
// Simple pipeline: filter → enrich → batch
filter := streamz.NewFilter(func(n int) bool { return n > 0 }).WithName("positive")
mapper := streamz.NewMapper(func(n int) int { return n * 2 }).WithName("double")
batcher := streamz.NewBatcher[int](streamz.BatchConfig{
    MaxSize:    10,
    MaxLatency: 100 * time.Millisecond,
})

// Compose the pipeline
filtered := filter.Process(ctx, numbers)
doubled := mapper.Process(ctx, filtered)
batched := batcher.Process(ctx, doubled)

// Process results
for batch := range batched {
    fmt.Printf("Batch: %v\n", batch)
}
```

## Core Concepts

streamz provides `Processor[In, Out]` interfaces that transform channels:
- `chan T -> chan T` - filters, maps, rate limiting
- `chan T -> chan []T` - batching
- `chan []T -> chan T` - unbatching  
- `chan T -> chan Window[T]` - windowing

## Integration with pipz

Use `FromChainable` to seamlessly integrate pipz processors into streaming contexts:

```go
// Any pipz T->T processor works in streams
validator := pipz.Transform("validate", validateOrder)
stream := FromChainable(validator).Process(ctx, orderStream)
```

## Batching

Accumulate individual items into batches based on size or time:

```go
batcher := NewBatcher[Order](BatchConfig{
    MaxSize:    100,        // Send when reaching 100 items
    MaxLatency: time.Second, // Or after 1 second
})
batches := batcher.Process(ctx, orders) // chan Order -> chan []Order
```

## Composition

Explicit, type-safe composition maintains compile-time guarantees:

```go
// Start with individual orders
orders := make(chan Order)

// Validate each order using pipz
validated := FromChainable(validator).Process(ctx, orders)

// Batch for efficient processing
batched := batcher.Process(ctx, validated) 

// Process batches (e.g., bulk database insert)
processed := batchProcessor.Process(ctx, batched)

// Return to individual items
unbatched := unbatcher.Process(ctx, processed)

// Final enrichment
final := FromChainable(enricher).Process(ctx, unbatched)
```

## Processors

### Core Processors
- **Batcher** - Accumulate items into batches
- **Unbatcher** - Flatten batches to individual items
- **Filter** - Keep only items matching a predicate
- **Mapper** - Transform items with a function
- **FanOut** - Duplicate stream to multiple outputs
- **FanIn** - Merge multiple streams

### Simple Transformations
- **Take** - Take first N items then close
- **Skip** - Skip first N items then pass through
- **Sample** - Random sampling by percentage
- **Chunk** - Fixed-size groups (simpler than Batcher)
- **Flatten** - Expand slices to individual items

### Windowing
- **TumblingWindow** - Fixed-size time windows
- **SlidingWindow** - Overlapping time windows
- **SessionWindow** - Activity-based windows

### Buffering & Flow Control
- **Buffer** - Add buffering to decouple producer/consumer speeds
- **DroppingBuffer** - Drop items when buffer is full (for real-time systems)
- **SlidingBuffer** - Keep latest N items, dropping oldest when full

### Advanced Processing
- **AsyncMapper** - Concurrent processing with order preservation
- **Dedupe** - Remove duplicate items within a time window
- **Monitor** - Observe stream metrics without changing flow

### Flow Control
- **Tap** - Execute side effects without modifying stream
- **Throttle** - Rate limiting (items per second)
- **Debounce** - Emit only after quiet period

## Examples

### Backpressure Handling
```go
// Fast producer, slow consumer - use buffer
fast := producer.Process(ctx, source)
buffered := NewBuffer[Order](1000).Process(ctx, fast)
slow := consumer.Process(ctx, buffered)
```

### Concurrent Processing with Order
```go
// Process 10 items concurrently, maintain order
enriched := NewAsyncMapper(func(ctx context.Context, order Order) (Order, error) {
    // Expensive enrichment operation
    return enrichOrder(order)
}).WithWorkers(10).Process(ctx, orders)
```

### Real-time Deduplication
```go
// Remove duplicate events within 5 minute window
deduped := NewDedupe(func(e Event) string { 
    return e.ID 
}).WithTTL(5*time.Minute).Process(ctx, events)
```

### Stream Monitoring
```go
// Monitor throughput without affecting flow
monitored := NewMonitor[Order](time.Second).OnStats(func(stats StreamStats) {
    log.Printf("Processing %f orders/sec", stats.Rate)
}).Process(ctx, orders)
```

### Simple Transformations
```go
// Process only first 1000 items
limited := NewTake[Event](1000).Process(ctx, events)

// Skip CSV header
data := NewSkip[Row](1).Process(ctx, csvRows)

// Sample 1% for monitoring
sampled := NewSample[Metric](0.01).Process(ctx, metrics)

// Fixed-size chunks for bulk operations
chunks := NewChunk[Email](25).Process(ctx, emails)

// Flatten batch results
results := batchAPI.Process(ctx, batches) // Returns chan []Result
individual := NewFlatten[Result]().Process(ctx, results)
```

### Flow Control
```go
// Debug logging without modifying stream
logged := NewTap[Order](func(o Order) {
    log.Printf("Processing order %s", o.ID)
}).Process(ctx, orders)

// Rate limit API calls to 100/second
throttled := NewThrottle[Request](100).Process(ctx, requests)

// Debounce search queries (wait 500ms after typing stops)
searches := NewDebounce[Query](500*time.Millisecond).Process(ctx, queries)
```

## Documentation

📚 **[Complete Documentation](./docs/README.md)**

- **[Introduction](./docs/introduction.md)** - Why streamz and core philosophy
- **[Quick Start Guide](./docs/quick-start.md)** - Build your first pipeline in 5 minutes
- **[Installation](./docs/installation.md)** - Get up and running
- **[Concepts](./docs/concepts/processors.md)** - Deep dive into processors and composition
- **[Guides](./docs/guides/patterns.md)** - Production patterns and best practices
- **[API Reference](./docs/api/)** - Complete processor documentation

## Performance

streamz is designed for production workloads:

- Minimal allocations in hot paths
- Efficient error propagation  
- No reflection or runtime type assertions
- Optimized for high-throughput, low-latency scenarios

```bash
# Run benchmarks
make bench
```

Typical performance (processing 1M items):
```
BenchmarkFilter-8     1000000    1205 ns/op    48 B/op    1 allocs/op
BenchmarkMapper-8     1000000    1180 ns/op    64 B/op    1 allocs/op
BenchmarkBatcher-8    1000000    2340 ns/op   156 B/op    2 allocs/op
```

## Development

### Prerequisites
- Go 1.21 or higher
- golangci-lint (install with `make install-tools`)

### Quick Start
```bash
# Install development tools
make install-tools

# Run tests
make test

# Run linters
make lint

# Run both tests and linters
make check
```

### Available Commands
```bash
make help           # Show all available commands
make test           # Run tests with race detector
make test-examples  # Run example tests
make bench          # Run benchmarks
make lint           # Run linters
make lint-fix       # Run linters with auto-fix
make coverage       # Generate coverage report
make clean          # Clean generated files
```

### Code Quality
This project uses comprehensive linting with golangci-lint, including:
- Security analysis (gosec)
- Error handling checks (errcheck, errorlint)
- Code quality checks (govet, staticcheck)
- Style consistency (gofmt, goimports)
- Performance suggestions (prealloc, copyloopvar)

Configuration is in `.golangci.yml`. CI runs these checks automatically on all PRs.

### Contributing
1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Run tests and linters (`make check`)
4. Commit your changes
5. Push to the branch
6. Open a Pull Request

All PRs must pass CI checks including tests, linting, and coverage requirements.