# Understanding Processors

The **Processor interface** is the foundation of streamz. Understanding how processors work is key to building robust streaming systems.

## The Processor Interface

Every streamz component implements this simple interface:

```go
type Processor[In, Out any] interface {
    // Process transforms the input channel to an output channel
    Process(ctx context.Context, in <-chan In) <-chan Out
    
    // Name returns a descriptive name for debugging
    Name() string
}
```

## How Processors Work

### 1. Channel Transformation

Processors transform one channel into another:

```go
// Filter: chan T -> chan T
filter := streamz.NewFilter(func(n int) bool { return n > 0 }).WithName("positive")
filtered := filter.Process(ctx, numbers)

// Batcher: chan T -> chan []T  
batcher := streamz.NewBatcher[int](config)
batched := batcher.Process(ctx, numbers)

// Mapper: chan T -> chan U
mapper := streamz.NewMapper(func(n int) string {
    return fmt.Sprintf("num-%d", n)
}).WithName("string")
strings := mapper.Process(ctx, numbers)
```

### 2. Lifecycle Management

All processors follow the same lifecycle:

```go
func (p *SomeProcessor[T]) Process(ctx context.Context, in <-chan T) <-chan T {
    out := make(chan T)
    
    go func() {
        defer close(out) // ✅ Always close output channel
        
        for {
            select {
            case item, ok := <-in:
                if !ok {
                    return // ✅ Exit when input closes
                }
                // Process item...
                select {
                case out <- result:
                case <-ctx.Done():
                    return // ✅ Respect context cancellation
                }
                
            case <-ctx.Done():
                return // ✅ Respect context cancellation
            }
        }
    }()
    
    return out
}
```

### 3. Error Handling Philosophy

streamz processors follow a "graceful degradation" approach:

- **Skip problematic items** rather than failing the entire stream
- **Log errors** for debugging but don't break the pipeline
- **Continue processing** remaining items

```go
// AsyncMapper handles errors by skipping items
asyncMapper := streamz.NewAsyncMapper(func(ctx context.Context, item Item) (Item, error) {
    result, err := expensiveOperation(item)
    if err != nil {
        // Error is logged internally, item is skipped
        return item, err
    }
    return result, nil
}).WithWorkers(10)
```

## Processor Categories

### Transform Processors
Change data from one form to another:

```go
// Mapper: Transform each item
mapper := streamz.NewMapper(func(s string) string {
    return strings.ToUpper(s)
}).WithName("uppercase")

// AsyncMapper: Concurrent transformation with order preservation
asyncMapper := streamz.NewAsyncMapper(expensiveTransform).WithWorkers(10)
```

### Filter Processors
Selectively pass items through:

```go
// Filter: Keep items matching predicate
filter := streamz.NewFilter(func(n int) bool { return n > 0 }).WithName("positive")

// Sample: Keep random percentage of items
sampler := streamz.NewSample[Event](0.1) // Keep 10%

// Take: Keep only first N items
limiter := streamz.NewTake[Event](1000)
```

### Aggregation Processors
Combine multiple items:

```go
// Batcher: Group items by size/time
batcher := streamz.NewBatcher[Event](streamz.BatchConfig{
    MaxSize:    100,
    MaxLatency: time.Second,
})

// Chunk: Fixed-size groups
chunker := streamz.NewChunk[Event](25)
```

### Flow Control Processors
Manage stream flow and timing:

```go
// Buffer: Add buffering to handle bursts
buffer := streamz.NewBuffer[Event](10000)

// Throttle: Rate limiting
throttle := streamz.NewThrottle[Request](100) // 100 requests/sec

// Debounce: Wait for quiet period
debouncer := streamz.NewDebounce[Query](500 * time.Millisecond)
```

### Observability Processors
Monitor without changing data:

```go
// Monitor: Track throughput and latency
monitor := streamz.NewMonitor[Event](time.Second).OnStats(func(stats streamz.StreamStats) {
    log.Printf("Rate: %.1f/sec, Avg Latency: %v", stats.Rate, stats.AvgLatency)
})

// Tap: Execute side effects
tap := streamz.NewTap[Order](func(order Order) {
    log.Printf("Processing order %s", order.ID)
})
```

## Custom Processors

You can implement your own processors for domain-specific needs:

```go
// Custom rate limiter with token bucket
type RateLimiter[T any] struct {
    name    string
    limiter *rate.Limiter
}

func (r *RateLimiter[T]) Name() string {
    return r.name
}

func (r *RateLimiter[T]) Process(ctx context.Context, in <-chan T) <-chan T {
    out := make(chan T)
    
    go func() {
        defer close(out)
        
        for {
            select {
            case item, ok := <-in:
                if !ok {
                    return
                }
                
                // Wait for rate limiter
                if err := r.limiter.Wait(ctx); err != nil {
                    return // Context cancelled
                }
                
                select {
                case out <- item:
                case <-ctx.Done():
                    return
                }
                
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return out
}

// Use like any built-in processor
limiter := &RateLimiter[Order]{
    name:    "order-rate-limit",
    limiter: rate.NewLimiter(rate.Every(time.Second/100), 10),
}
limited := limiter.Process(ctx, orders)
```

## Processor Composition

Processors compose naturally because they all implement the same interface:

```go
// Each step transforms the channel type appropriately
orders := make(chan Order)                          // chan Order

validated := validator.Process(ctx, orders)         // chan Order
enriched := enricher.Process(ctx, validated)       // chan EnrichedOrder  
batched := batcher.Process(ctx, enriched)          // chan []EnrichedOrder
processed := batchProcessor.Process(ctx, batched)  // chan []ProcessedOrder
final := unbatcher.Process(ctx, processed)         // chan ProcessedOrder
```

## Best Practices

### 1. Name Your Processors
Use descriptive names for debugging:

```go
validator := streamz.NewFilter(isValidOrder).WithName("order-validator")
enricher := streamz.NewMapper(enrichWithCustomerData).WithName("customer-enricher")
```

### 2. Keep Processors Simple
Each processor should have one clear responsibility:

```go
// ✅ Good: Single responsibility
emailValidator := streamz.NewFilter(isValidEmail).WithName("email-validator")
phoneValidator := streamz.NewFilter(isValidPhone).WithName("phone-validator")

// ❌ Avoid: Multiple responsibilities in one processor
complexValidator := streamz.NewFilter(func(user User) bool {
    return isValidEmail(user.Email) && 
           isValidPhone(user.Phone) && 
           isValidAddress(user.Address) // Too much
}).WithName("complex-validator")
```

### 3. Handle Context Properly
Always respect context cancellation:

```go
// ✅ Good: Check context in all operations
select {
case out <- result:
case <-ctx.Done():
    return
}

// ❌ Avoid: Blocking operations without context
out <- result // Can block forever if output is full
```

### 4. Test Processors Independently
Each processor should be unit testable:

```go
func TestEmailValidator(t *testing.T) {
    ctx := context.Background()
    validator := streamz.NewFilter(isValidEmail).WithName("email-validator")
    
    in := make(chan User)
    out := validator.Process(ctx, in)
    
    // Send test data
    go func() {
        in <- User{Email: "valid@example.com"}
        in <- User{Email: "invalid"}
        close(in)
    }()
    
    // Verify results
    users := collectAll(out)
    assert.Len(t, users, 1)
    assert.Equal(t, "valid@example.com", users[0].Email)
}
```

## What's Next?

- **[Channel Management](./channels.md)**: Learn how streamz handles channels
- **[Composition](./composition.md)**: Advanced composition patterns
- **[Error Handling](./error-handling.md)**: Error strategies in streaming
- **[Performance Guide](../guides/performance.md)**: Optimize your processors