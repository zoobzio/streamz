# Backpressure Management

Learn how to handle slow consumers, burst traffic, and flow control in streaming systems using streamz's built-in backpressure strategies.

## What is Backpressure?

**Backpressure** occurs when data flows into a system faster than it can be processed. In streaming systems, this happens when:

- Producers generate data faster than consumers can process it
- Downstream systems become temporarily slow or unavailable
- Processing stages have different throughput characteristics
- Burst traffic exceeds normal processing capacity

**Without proper backpressure handling:**
- Memory usage grows unbounded
- Systems become unstable or crash
- Latency increases dramatically
- Data may be lost

## Natural Backpressure in Go Channels

Go channels provide **automatic backpressure** through blocking:

```go
// Producer blocks when consumer is slow
func producer(out chan<- int) {
    for i := 0; i < 1000; i++ {
        out <- i // Blocks if consumer is slow!
    }
    close(out)
}

func consumer(in <-chan int) {
    for n := range in {
        time.Sleep(time.Millisecond) // Slow processing
        fmt.Println(n)
    }
}

// Natural backpressure: producer slows down to match consumer
```

**streamz preserves this natural backpressure** while adding sophisticated strategies for different scenarios.

## Backpressure Strategies

### 1. Natural Backpressure (Default)

**Use when:** Normal processing with predictable load

```go
// Default behavior: producer slows down to match consumer speed
func naturalBackpressurePipeline(ctx context.Context, orders <-chan Order) <-chan ProcessedOrder {
    validator := streamz.NewFilter(isValid).WithName("validate")
    processor := streamz.NewMapper(processOrder).WithName("process")
    
    validated := validator.Process(ctx, orders)
    return processor.Process(ctx, validated) // Natural backpressure
}
```

**Characteristics:**
- ✅ Simple and predictable
- ✅ No data loss
- ✅ Bounded memory usage
- ❌ Can slow down entire pipeline
- ❌ Not suitable for real-time systems

### 2. Buffering Strategy

**Use when:** Smoothing out burst traffic or decoupling component speeds

```go
// Add buffering to handle bursts
func bufferedPipeline(ctx context.Context, orders <-chan Order) <-chan ProcessedOrder {
    // Small buffer for minor variations
    smallBuffer := streamz.NewBuffer[Order](100)
    
    validator := streamz.NewFilter(isValid).WithName("validate")
    
    // Large buffer before expensive processing
    largeBuffer := streamz.NewBuffer[Order](10000)
    
    processor := streamz.NewAsyncMapper(expensiveProcessing).WithWorkers(10)
    
    buffered1 := smallBuffer.Process(ctx, orders)
    validated := validator.Process(ctx, buffered1)
    buffered2 := largeBuffer.Process(ctx, validated)
    return processor.Process(ctx, buffered2)
}
```

**Buffer sizing guidelines:**

```go
// Small buffers (10-100): Smooth minor variations
buffer := streamz.NewBuffer[Event](50)

// Medium buffers (1,000-10,000): Handle burst traffic
buffer := streamz.NewBuffer[Order](5000)

// Large buffers (100,000+): Long-term decoupling (watch memory!)
buffer := streamz.NewBuffer[LogEntry](100000)
```

### 3. Dropping Strategy

**Use when:** Real-time systems where latency matters more than completeness

```go
// Drop items when system is overloaded
func realtimePipeline(ctx context.Context, events <-chan Event) <-chan ProcessedEvent {
    // Drop new items when buffer is full
    dropper := streamz.NewDroppingBuffer[Event](1000)
    
    processor := streamz.NewMapper(processEvent).WithName("realtime-process")
    
    // Monitor drop rate
    monitor := streamz.NewMonitor[Event](time.Second).OnStats(func(stats streamz.StreamStats) {
        if stats.Rate < inputRate*0.9 { // Detecting drops
            metrics.RecordDrops(inputRate - stats.Rate)
        }
    })
    
    dropped := dropper.Process(ctx, events)
    monitored := monitor.Process(ctx, dropped)
    return processor.Process(ctx, monitored)
}
```

**When to use dropping:**
- Live metrics and monitoring
- Real-time analytics where recent data matters most
- Video/audio streaming
- Gaming systems

### 4. Sliding Window Strategy

**Use when:** Only recent data matters

```go
// Keep only the latest N items
func slidingWindowPipeline(ctx context.Context, prices <-chan Price) <-chan MovingAverage {
    // Keep only latest 100 prices
    slider := streamz.NewSlidingBuffer[Price](100)
    
    // Calculate moving average on recent prices
    averager := streamz.NewMapper(func(price Price) MovingAverage {
        return calculateMovingAverage(recentPrices, price)
    }).WithName("moving-average")
    
    recent := slider.Process(ctx, prices)
    return averager.Process(ctx, recent)
}
```

**Use cases:**
- Moving averages
- Recent activity monitoring
- Trend analysis
- Real-time dashboards

### 5. Sampling Strategy

**Use when:** Processing all data is unnecessary

```go
// Process only a percentage of items
func samplingPipeline(ctx context.Context, events <-chan Event) <-chan AnalyzedEvent {
    // Sample 10% for detailed analysis
    sampler := streamz.NewSample[Event](0.1)
    
    // Expensive analysis only on sampled data
    analyzer := streamz.NewAsyncMapper(expensiveAnalysis).WithWorkers(5)
    
    sampled := sampler.Process(ctx, events)
    return analyzer.Process(ctx, sampled)
}
```

## Multi-Tier Backpressure

**Combine strategies** for sophisticated flow control:

```go
func multiTierBackpressure(ctx context.Context, input <-chan Event) <-chan ProcessedEvent {
    // Tier 1: Small buffer for normal variations
    tier1 := streamz.NewBuffer[Event](100)
    
    // Tier 2: Fast preprocessing
    filter := streamz.NewFilter(isImportantEvent).WithName("important")
    
    // Tier 3: Medium buffer for burst handling
    tier2 := streamz.NewBuffer[Event](5000)
    
    // Tier 4: Sampling under extreme load
    sampler := streamz.NewSample[Event](0.5) // Sample 50% under load
    
    // Tier 5: Dropping buffer as last resort
    dropper := streamz.NewDroppingBuffer[Event](10000)
    
    // Expensive processing
    processor := streamz.NewAsyncMapper(expensiveProcessing).WithWorkers(20)
    
    // Monitoring at each tier
    monitor := streamz.NewMonitor[Event](time.Second).OnStats(func(stats streamz.StreamStats) {
        metrics.RecordTierThroughput("final", stats.Rate)
    })
    
    // Compose the tiers
    buffered1 := tier1.Process(ctx, input)
    filtered := filter.Process(ctx, buffered1)
    buffered2 := tier2.Process(ctx, filtered)
    sampled := sampler.Process(ctx, buffered2)
    protected := dropper.Process(ctx, sampled)
    monitored := monitor.Process(ctx, protected)
    return processor.Process(ctx, monitored)
}
```

## Dynamic Backpressure

**Adapt strategy based on system load:**

```go
type AdaptiveProcessor[T any] struct {
    name           string
    normalBuffer   *streamz.Buffer[T]
    droppingBuffer *streamz.DroppingBuffer[T]
    loadThreshold  float64
    currentLoad    float64
    mutex          sync.RWMutex
}

func (a *AdaptiveProcessor[T]) Process(ctx context.Context, in <-chan T) <-chan T {
    // Monitor system load
    go a.monitorLoad(ctx)
    
    return a.adaptiveProcess(ctx, in)
}

func (a *AdaptiveProcessor[T]) adaptiveProcess(ctx context.Context, in <-chan T) <-chan T {
    out := make(chan T)
    
    go func() {
        defer close(out)
        
        for {
            select {
            case item, ok := <-in:
                if !ok {
                    return
                }
                
                // Choose strategy based on current load
                a.mutex.RLock()
                useDropping := a.currentLoad > a.loadThreshold
                a.mutex.RUnlock()
                
                if useDropping {
                    // High load: use dropping strategy
                    processed := a.droppingBuffer.Process(ctx, singleItemChan(item))
                    forwardFirst(ctx, processed, out)
                } else {
                    // Normal load: use buffering strategy
                    processed := a.normalBuffer.Process(ctx, singleItemChan(item))
                    forwardFirst(ctx, processed, out)
                }
                
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return out
}
```

## Monitoring Backpressure

### Key Metrics to Track

```go
func monitoredPipeline(ctx context.Context, input <-chan Order) <-chan ProcessedOrder {
    // Input rate monitoring
    inputMonitor := streamz.NewMonitor[Order](time.Second).OnStats(func(stats streamz.StreamStats) {
        metrics.InputRate.Set(stats.Rate)
        
        // Detect input spikes
        if stats.Rate > normalRate*2 {
            alerting.SendAlert("High input rate", stats.Rate)
        }
    })
    
    buffer := streamz.NewBuffer[Order](10000)
    
    // Processing rate monitoring
    processor := streamz.NewAsyncMapper(processOrder).WithWorkers(10)
    
    outputMonitor := streamz.NewMonitor[ProcessedOrder](time.Second, func(stats streamz.StreamStats) {
        metrics.OutputRate.Set(stats.Rate)
        metrics.ProcessingLatency.Observe(stats.AvgLatency.Seconds())
        
        // Detect backpressure by comparing input/output rates
        inputRate := metrics.InputRate.Get()
        outputRate := stats.Rate
        
        if inputRate > outputRate*1.2 { // 20% difference indicates backpressure
            metrics.BackpressureDetected.Inc()
            log.Warn("Backpressure detected", "input_rate", inputRate, "output_rate", outputRate)
        }
    })
    
    inputMonitored := inputMonitor.Process(ctx, input)
    buffered := buffer.Process(ctx, inputMonitored)
    processed := processor.Process(ctx, buffered)
    return outputMonitor.Process(ctx, processed)
}
```

### Buffer Health Monitoring

```go
// Custom buffer with health metrics
type MonitoredBuffer[T any] struct {
    *streamz.Buffer[T]
    capacity    int
    currentSize int64
    mutex       sync.RWMutex
}

func (m *MonitoredBuffer[T]) Process(ctx context.Context, in <-chan T) <-chan T {
    // Report buffer utilization
    go func() {
        ticker := time.NewTicker(time.Second)
        defer ticker.Stop()
        
        for {
            select {
            case <-ticker.C:
                m.mutex.RLock()
                utilization := float64(m.currentSize) / float64(m.capacity)
                m.mutex.RUnlock()
                
                metrics.BufferUtilization.Set(utilization)
                
                if utilization > 0.8 {
                    log.Warn("Buffer nearly full", "utilization", utilization)
                }
                
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return m.Buffer.Process(ctx, in)
}
```

## Circuit Breaker Pattern

**Fail fast** when downstream systems are overloaded:

```go
type CircuitBreaker[T any] struct {
    name         string
    maxFailures  int
    resetTimeout time.Duration
    failures     int64
    lastFailure  time.Time
    state        int32 // 0=closed, 1=open, 2=half-open
    mutex        sync.RWMutex
}

const (
    CircuitClosed = iota
    CircuitOpen
    CircuitHalfOpen
)

func (cb *CircuitBreaker[T]) Process(ctx context.Context, in <-chan T) <-chan T {
    out := make(chan T)
    
    go func() {
        defer close(out)
        
        for {
            select {
            case item, ok := <-in:
                if !ok {
                    return
                }
                
                if cb.shouldReject() {
                    // Circuit is open, reject item
                    metrics.CircuitBreakerRejects.Inc()
                    continue
                }
                
                // Try to process
                if cb.processItem(ctx, item, out) {
                    cb.recordSuccess()
                } else {
                    cb.recordFailure()
                }
                
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return out
}

func (cb *CircuitBreaker[T]) shouldReject() bool {
    cb.mutex.RLock()
    defer cb.mutex.RUnlock()
    
    state := atomic.LoadInt32(&cb.state)
    
    switch state {
    case CircuitClosed:
        return false
    case CircuitOpen:
        if time.Since(cb.lastFailure) > cb.resetTimeout {
            atomic.StoreInt32(&cb.state, CircuitHalfOpen)
            return false
        }
        return true
    case CircuitHalfOpen:
        return false
    default:
        return false
    }
}
```

## Best Practices

### 1. **Choose the Right Strategy**

```go
// ✅ Real-time systems: Use dropping
realtime := streamz.NewDroppingBuffer[Event](1000)

// ✅ Batch processing: Use buffering
batch := streamz.NewBuffer[Order](10000)

// ✅ Analytics: Use sampling
analytics := streamz.NewSample[Metric](0.1)

// ✅ Recent data only: Use sliding
recent := streamz.NewSlidingBuffer[Price](100)
```

### 2. **Size Buffers Appropriately**

```go
// Consider memory usage: sizeof(T) * bufferSize
type LargeStruct struct {
    Data [1024]byte // 1KB per item
}

// 10,000 items = ~10MB memory
buffer := streamz.NewBuffer[LargeStruct](10000)

// For large items, use smaller buffers
buffer := streamz.NewBuffer[LargeStruct](1000) // ~1MB memory
```

### 3. **Monitor and Alert**

```go
// Set up alerting for backpressure indicators
monitor := streamz.NewMonitor[Order](time.Second, func(stats streamz.StreamStats) {
    if stats.Rate < expectedMinRate {
        alerting.SendAlert("Pipeline slow", map[string]interface{}{
            "rate":           stats.Rate,
            "expected_rate":  expectedMinRate,
            "avg_latency":    stats.AvgLatency,
        })
    }
})
```

### 4. **Test Under Load**

```go
func TestBackpressureHandling(t *testing.T) {
    ctx := context.Background()
    
    // Create high-volume input
    input := make(chan Order)
    go func() {
        defer close(input)
        for i := 0; i < 100000; i++ {
            input <- Order{ID: fmt.Sprintf("order-%d", i)}
        }
    }()
    
    // Slow processor to create backpressure
    slowProcessor := streamz.NewMapper("slow", func(ctx context.Context, order Order) Order {
        time.Sleep(time.Millisecond) // Simulate slow processing
        return order
    })
    
    buffer := streamz.NewBuffer[Order](1000)
    
    buffered := buffer.Process(ctx, input)
    processed := slowProcessor.Process(ctx, buffered)
    
    // Verify system handles backpressure gracefully
    count := 0
    for range processed {
        count++
    }
    
    assert.True(t, count > 0, "Should process some items despite backpressure")
}
```

### 5. **Plan for Recovery**

```go
func resilientPipeline(ctx context.Context, input <-chan Order) <-chan ProcessedOrder {
    // Multiple strategies for different failure modes
    
    // Network issues: Retry with backoff
    retryProcessor := &RetryProcessor[Order]{
        maxRetries: 3,
        baseDelay:  time.Millisecond * 100,
        processor:  networkCall,
    }
    
    // Overload protection: Circuit breaker
    circuitBreaker := &CircuitBreaker[Order]{
        maxFailures:  10,
        resetTimeout: time.Minute,
    }
    
    // Memory protection: Dropping buffer
    dropper := streamz.NewDroppingBuffer[Order](50000)
    
    protected := dropper.Process(ctx, input)
    circuitProtected := circuitBreaker.Process(ctx, protected)
    return retryProcessor.Process(ctx, circuitProtected)
}
```

## Common Anti-Patterns

### ❌ **Unbounded Buffering**
```go
// DON'T: This can consume all available memory
hugeBuffer := streamz.NewBuffer[Event](math.MaxInt) // Dangerous!
```

### ❌ **Ignoring Backpressure Signals**
```go
// DON'T: Using non-blocking sends without handling full channels
select {
case out <- item:
    // Sent successfully
default:
    // Channel full, but ignoring it!
}
```

### ❌ **No Monitoring**
```go
// DON'T: Running without monitoring
pipeline := processor.Process(ctx, input) // No visibility into performance
```

## What's Next?

- **[Error Handling](./error-handling.md)**: Advanced error strategies in streaming
- **[Performance Guide](../guides/performance.md)**: Optimize backpressure handling
- **[Testing Guide](../guides/testing.md)**: Test backpressure scenarios
- **[API Reference: Buffer](../api/buffer.md)**: Complete buffer documentation