# Channel Management

streamz handles Go channels correctly so you don't have to worry about common pitfalls like goroutine leaks, deadlocks, and improper cleanup.

## The Challenge with Raw Channels

Managing channels correctly is surprisingly difficult:

```go
// ❌ Common mistakes with raw channels
func processData(input <-chan Data) <-chan Result {
    output := make(chan Result)
    
    go func() {
        for data := range input {
            result := transform(data)
            output <- result // Can block forever!
        }
        // Forgot to close output channel - goroutine leak!
    }()
    
    return output
}
```

**Problems:**
- Goroutine leaks from unclosed channels
- Deadlocks from blocking sends
- No context cancellation support
- Complex cleanup logic
- Race conditions

## streamz Channel Patterns

### 1. Proper Lifecycle Management

All streamz processors follow the same channel lifecycle pattern:

```go
func (p *Processor[T]) Process(ctx context.Context, in <-chan T) <-chan T {
    out := make(chan T, p.bufferSize) // Optional buffering
    
    go func() {
        defer close(out) // ✅ Always close output
        
        for {
            select {
            case item, ok := <-in:
                if !ok {
                    return // ✅ Exit when input closes
                }
                
                // Process item...
                
                select {
                case out <- result:
                    // Item sent successfully
                case <-ctx.Done():
                    return // ✅ Respect cancellation
                }
                
            case <-ctx.Done():
                return // ✅ Respect cancellation
            }
        }
    }()
    
    return out
}
```

### 2. Context Propagation

Every processor respects context cancellation:

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

// All processors will stop when context is cancelled
filtered := filter.Process(ctx, input)
mapped := mapper.Process(ctx, filtered)
batched := batcher.Process(ctx, mapped)

// Automatically stops after 5 seconds
```

### 3. Backpressure Handling

streamz provides multiple strategies for handling backpressure:

#### Natural Backpressure
```go
// Natural backpressure through channel blocking
// Slow consumers automatically slow down producers
pipeline := func(ctx context.Context, input <-chan Order) <-chan ProcessedOrder {
    validated := validator.Process(ctx, input)
    return processor.Process(ctx, validated)
}
```

#### Explicit Buffering
```go
// Add buffering to handle bursts
buffer := streamz.NewBuffer[Order](10000)
buffered := buffer.Process(ctx, orders)
processed := slowProcessor.Process(ctx, buffered)
```

#### Dropping Strategy
```go
// For real-time systems: drop items when buffer is full
dropper := streamz.NewDroppingBuffer[Event](1000)
realtime := dropper.Process(ctx, highVolumeEvents)
```

#### Sliding Window
```go
// Keep only the latest items
slider := streamz.NewSlidingBuffer[Metric](100)
latest := slider.Process(ctx, metrics)
```

## Channel Types and Transformations

### 1. One-to-One (T → T)
Filters, transforms, rate limiting:

```go
// Filter: chan Order → chan Order
validOrders := filter.Process(ctx, orders)

// Mapper: chan Order → chan EnrichedOrder  
enriched := enricher.Process(ctx, validOrders)

// Throttle: chan Request → chan Request
throttled := throttle.Process(ctx, requests)
```

### 2. One-to-Many (T → []T)
Batching, chunking, windowing:

```go
// Batcher: chan Order → chan []Order
batches := batcher.Process(ctx, orders)

// Window: chan Event → chan Window[Event]
windows := windower.Process(ctx, events)
```

### 3. Many-to-One ([]T → T)
Unbatching, flattening:

```go
// Unbatcher: chan []Order → chan Order
individual := unbatcher.Process(ctx, batches)

// Flatten: chan []Event → chan Event
flattened := flattener.Process(ctx, slices)
```

### 4. Many-to-Many (Multiple Inputs/Outputs)
Fan-in, fan-out:

```go
// FanOut: chan T → multiple chan T
outputs := fanout.Process(ctx, input, 3) // 3 output channels

// FanIn: multiple chan T → chan T
merged := fanin.Process(ctx, input1, input2, input3)
```

## Buffer Strategies

### When to Use Each Strategy

#### Regular Buffer
- **Use for**: Smoothing out burst traffic
- **Behavior**: Blocks when full
- **Good for**: Most general-purpose scenarios

```go
buffer := streamz.NewBuffer[Event](10000)
smoothed := buffer.Process(ctx, burstyEvents)
```

#### Dropping Buffer  
- **Use for**: Real-time systems where latency matters more than completeness
- **Behavior**: Drops new items when full
- **Good for**: Live metrics, real-time monitoring

```go
dropper := streamz.NewDroppingBuffer[Metric](1000)
realtime := dropper.Process(ctx, highFrequencyMetrics)
```

#### Sliding Buffer
- **Use for**: When you only care about recent items
- **Behavior**: Drops oldest items when full
- **Good for**: Moving averages, recent history

```go
slider := streamz.NewSlidingBuffer[Price](100)
recentPrices := slider.Process(ctx, allPrices)
```

## Channel Sizing Guidelines

### Default Sizing
```go
// Most processors use unbuffered channels by default
// This provides natural backpressure
out := make(chan T) // Unbuffered
```

### When to Add Buffering
```go
// 1. Producer faster than consumer
buffer := streamz.NewBuffer[Event](1000)

// 2. Burst traffic handling  
buffer := streamz.NewBuffer[Request](10000)

// 3. Decoupling components
buffer := streamz.NewBuffer[Order](100)
```

### Buffer Size Rules of Thumb
- **Small buffers (10-100)**: Smooth out small variations
- **Medium buffers (1000-10000)**: Handle burst traffic
- **Large buffers (100000+)**: Long-term decoupling (be careful of memory usage)

## Error Scenarios and Recovery

### Slow Consumer
```go
// Problem: Slow database writes blocking the pipeline
slowDB := slowDatabaseWriter.Process(ctx, orders)

// Solution: Add buffering
buffer := streamz.NewBuffer[Order](10000)
buffered := buffer.Process(ctx, orders)
smoothed := slowDatabaseWriter.Process(ctx, buffered)
```

### Fast Producer
```go
// Problem: High-frequency events overwhelming consumers
events := highFrequencyProducer()

// Solution: Sampling or dropping
sampler := streamz.NewSample[Event](0.1) // Keep 10%
manageable := sampler.Process(ctx, events)
```

### Context Cancellation
```go
// All processors handle cancellation gracefully
ctx, cancel := context.WithCancel(context.Background())

pipeline := func() {
    filtered := filter.Process(ctx, input)
    processed := processor.Process(ctx, filtered)
    
    for result := range processed {
        // Process results...
    }
}

// Cancel anytime - all goroutines will stop cleanly
cancel()
```

## Best Practices

### 1. Use Context Everywhere
```go
// ✅ Always pass context
filtered := filter.Process(ctx, input)

// ❌ Don't ignore context
filtered := filter.Process(context.Background(), input) // Poor practice
```

### 2. Size Buffers Appropriately
```go
// ✅ Consider your workload
buffer := streamz.NewBuffer[Event](estimatedBurstSize)

// ❌ Don't use arbitrary large buffers
buffer := streamz.NewBuffer[Event](1000000) // Likely too large
```

### 3. Monitor Channel Health
```go
// Add monitoring to detect backpressure
monitor := streamz.NewMonitor[Order](time.Second).OnStats(func(stats streamz.StreamStats) {
    if stats.Rate < expectedRate {
        log.Warn("Pipeline slowing down", "rate", stats.Rate)
    }
})
```

### 4. Test Channel Closure
```go
func TestChannelClosure(t *testing.T) {
    ctx := context.Background()
    processor := streamz.NewFilter(func(n int) bool { return true }).WithName("test")
    
    in := make(chan int)
    out := processor.Process(ctx, in)
    
    // Close input
    close(in)
    
    // Verify output closes
    _, ok := <-out
    assert.False(t, ok, "Output channel should be closed")
}
```

## What's Next?

- **[Composition](./composition.md)**: Learn advanced composition patterns
- **[Error Handling](./error-handling.md)**: Error strategies in streaming
- **[Performance Guide](../guides/performance.md)**: Optimize channel performance