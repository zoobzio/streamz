# streamz

[![CI Status](https://github.com/zoobzio/streamz/workflows/CI/badge.svg)](https://github.com/zoobzio/streamz/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/zoobzio/streamz/graph/badge.svg?branch=main)](https://codecov.io/gh/zoobzio/streamz)
[![Go Report Card](https://goreportcard.com/badge/github.com/zoobzio/streamz)](https://goreportcard.com/report/github.com/zoobzio/streamz)
[![CodeQL](https://github.com/zoobzio/streamz/workflows/CodeQL/badge.svg)](https://github.com/zoobzio/streamz/security/code-scanning)
[![Go Reference](https://pkg.go.dev/badge/github.com/zoobzio/streamz.svg)](https://pkg.go.dev/github.com/zoobzio/streamz)
[![License](https://img.shields.io/github/license/zoobzio/streamz)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/zoobzio/streamz)](go.mod)
[![Release](https://img.shields.io/github/v/release/zoobzio/streamz)](https://github.com/zoobzio/streamz/releases)

Type-safe, composable stream processing primitives for Go channels, enabling real-time data processing through batching, windowing, and flow control.

Build robust streaming pipelines that are easy to test, reason about, and maintain.

## Why streamz?

- **Type-safe**: Full compile-time type checking with Go generics
- **Composable**: Build complex pipelines from simple, reusable parts
- **Unified error handling**: `Result[T]` pattern eliminates dual-channel complexity
- **Deterministic testing**: Clock abstraction enables reproducible time-based tests
- **Production ready**: Handle edge cases, errors, and resource management correctly
- **Fast**: Minimal allocations, optimized for performance

**Common problems streamz solves:**
- Goroutine leaks from improper channel cleanup
- Deadlocks from blocking channel operations
- Complex backpressure handling
- Flaky tests in time-dependent code
- Type safety in multi-stage processing pipelines

## Quick Start

```go
// Simple pipeline: filter → transform → batch
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
    if batch.IsSuccess() {
        fmt.Printf("Batch: %v\n", batch.Value())
    }
}
```

## Core Concepts

### Result[T] Pattern

streamz uses `Result[T]` to unify success and error handling in a single channel:

```go
// Instead of dual channels:
// values <-chan T, errors <-chan error

// streamz uses:
// results <-chan Result[T]

for result := range output {
    if result.IsSuccess() {
        process(result.Value())
    } else {
        handleError(result.Error())
    }
}
```

### Processors

All processors implement a common pattern:
```go
Process(ctx context.Context, in <-chan Result[T]) <-chan Result[Out]
```

## Processors

### Transformation
- **Filter** - Keep only items matching a predicate
- **Mapper** - Transform items synchronously
- **AsyncMapper** - Concurrent processing with optional order preservation
- **Tap** - Execute side effects without modifying stream
- **Sample** - Random sampling by probability

### Batching & Routing
- **Batcher** - Accumulate items into batches by size or time
- **Partition** - Split stream by hash or round-robin
- **Switch** - Route items to named channels by predicate
- **FanOut** - Broadcast to multiple outputs
- **FanIn** - Merge multiple streams

### Windowing
- **TumblingWindow** - Fixed, non-overlapping time windows
- **SlidingWindow** - Overlapping time windows
- **SessionWindow** - Activity-based windows with gap detection

### Flow Control
- **Throttle** - Rate limiting (leading edge)
- **Debounce** - Emit after quiet period (trailing edge)
- **Buffer** - Decouple producer/consumer speeds
- **DeadLetterQueue** - Separate successes and failures

## Examples

### Batching for Bulk Operations
```go
batcher := NewBatcher[Order](BatchConfig{
    MaxSize:    100,         // Send when reaching 100 items
    MaxLatency: time.Second, // Or after 1 second
})
batches := batcher.Process(ctx, orders)
```

### Concurrent Processing with Order Preservation
```go
enriched := NewAsyncMapper(func(ctx context.Context, order Order) (Order, error) {
    return enrichOrder(order)
}).WithWorkers(10).WithOrdered(true).Process(ctx, orders)
```

### Partitioned Processing
```go
// Hash-partition by customer ID across 4 workers
partition, _ := NewHashPartition[Order, string](4, 100, func(o Order) string {
    return o.CustomerID
})
outputs := partition.Process(ctx, orders)

for i, out := range outputs {
    go processPartition(i, out)
}
```

### Routing by Type
```go
router := NewSwitch[Event](func(e Event) string {
    return e.Type
}).WithRoutes("click", "purchase", "view")

routes := router.Process(ctx, events)
go handleClicks(routes["click"])
go handlePurchases(routes["purchase"])
go handleViews(routes["view"])
```

### Rate Limiting
```go
// Allow one item per 100ms
throttled := NewThrottle[Request](100 * time.Millisecond).Process(ctx, requests)

// Emit only after 500ms of quiet
debounced := NewDebounce[Query](500 * time.Millisecond).Process(ctx, queries)
```

### Error Segregation
```go
dlq := NewDeadLetterQueue[Order]().WithDropWhenFull(true)
success, failure := dlq.Process(ctx, processedOrders)

go saveToDatabase(success)
go logFailures(failure)
```

### Time Windows
```go
// Hourly aggregation
tumbling := NewTumblingWindow[Metric](time.Hour)
hourlyBuckets := tumbling.Process(ctx, metrics)

// 5-minute sliding window, emit every minute
sliding := NewSlidingWindow[Metric](5*time.Minute, time.Minute)
rollingAverages := sliding.Process(ctx, metrics)

// Session window with 30-minute gap
session := NewSessionWindow[Event](30 * time.Minute)
userSessions := session.Process(ctx, events)
```

## Deterministic Testing

streamz uses [clockz](https://github.com/zoobzio/clockz) for time abstraction, enabling deterministic tests:

```go
func TestBatcher(t *testing.T) {
    clock := clockz.NewFake(time.Now())
    batcher := NewBatcher[int](BatchConfig{
        MaxLatency: time.Second,
    }).WithClock(clock)

    // ... setup pipeline ...

    // Advance time deterministically
    clock.Advance(time.Second)

    // Assert batch was emitted
}
```

## Documentation

- **[Complete Documentation](./docs/README.md)**
- **[Testing Guide](./TESTING.md)**

## Installation

```bash
go get github.com/zoobzio/streamz
```

Requires Go 1.23+

## Development

```bash
make test      # Run tests with race detector
make lint      # Run linters
make bench     # Run benchmarks
make coverage  # Generate coverage report
```

## License

MIT
