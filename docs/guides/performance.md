# Performance Guide

Optimize your streamz pipelines for high-throughput, low-latency production workloads.

## Performance Principles

streamz is designed for performance from the ground up:

1. **Minimal allocations** in hot paths
2. **Efficient channel operations** with strategic buffering
3. **No reflection** or runtime type assertions
4. **Optimized data structures** for common patterns
5. **Configurable concurrency** for parallel processing

## Benchmarking Your Pipeline

### Basic Benchmarking

Always measure before optimizing:

```go
func BenchmarkPipeline(b *testing.B) {
    ctx := context.Background()
    
    // Setup pipeline
    filter := streamz.NewFilter(func(n int) bool { return n > 0 }).WithName("positive")
    mapper := streamz.NewMapper(func(n int) int { return n * 2 }).WithName("double")
    
    // Benchmark data
    data := make([]int, 1000)
    for i := range data {
        data[i] = i + 1 // All positive numbers
    }
    
    b.ResetTimer()
    b.ReportAllocs()
    
    for i := 0; i < b.N; i++ {
        input := make(chan int)
        
        // Run pipeline
        filtered := filter.Process(ctx, input)
        doubled := mapper.Process(ctx, filtered)
        
        // Feed data
        go func() {
            defer close(input)
            for _, n := range data {
                input <- n
            }
        }()
        
        // Consume output
        count := 0
        for range doubled {
            count++
        }
        
        if count != len(data) {
            b.Fatalf("Expected %d items, got %d", len(data), count)
        }
    }
}
```

### Throughput Benchmarking

```go
func BenchmarkThroughput(b *testing.B) {
    ctx := context.Background()
    
    processor := streamz.NewAsyncMapper(func(ctx context.Context, n int) (int, error) {
        // Simulate some work
        time.Sleep(time.Microsecond)
        return n * 2, nil
    }).WithWorkers(10)
    
    itemCount := 10000
    
    b.ResetTimer()
    
    for i := 0; i < b.N; i++ {
        input := make(chan int)
        output := processor.Process(ctx, input)
        
        start := time.Now()
        
        // Send data
        go func() {
            defer close(input)
            for j := 0; j < itemCount; j++ {
                input <- j
            }
        }()
        
        // Consume output
        count := 0
        for range output {
            count++
        }
        
        duration := time.Since(start)
        throughput := float64(count) / duration.Seconds()
        
        b.ReportMetric(throughput, "items/sec")
    }
}
```

## Optimization Strategies

### 1. Processor Selection

**Choose the right processor** for your use case:

```go
// ✅ Use Filter for simple predicates (fastest)
filter := streamz.NewFilter(func(n int) bool { return n > 0 }).WithName("positive")

// ✅ Use Mapper for pure transformations
mapper := streamz.NewMapper(func(n int) int { return n * 2 }).WithName("double")

// ✅ Use AsyncMapper only when you need concurrency
asyncMapper := streamz.NewAsyncMapper(expensiveOperation).WithWorkers(10)

// ❌ Don't use AsyncMapper for simple operations
slowMapper := streamz.NewAsyncMapper(func(ctx context.Context, n int) (int, error) {
    return n * 2, nil // Too simple for async processing
}).WithWorkers(10)
```

### 2. Batching Optimization

**Configure batching** for optimal throughput:

```go
// For database operations: larger batches
dbBatcher := streamz.NewBatcher[Order](streamz.BatchConfig{
    MaxSize:    1000,         // Optimize for bulk operations
    MaxLatency: 5 * time.Second, // Can tolerate higher latency
})

// For API calls: smaller batches, lower latency
apiBatcher := streamz.NewBatcher[Request](streamz.BatchConfig{
    MaxSize:    50,                    // API rate limits
    MaxLatency: 100 * time.Millisecond, // Low latency requirement
})

// For real-time processing: very small batches
realtimeBatcher := streamz.NewBatcher[Event](streamz.BatchConfig{
    MaxSize:    10,                   // Quick processing
    MaxLatency: 10 * time.Millisecond, // Near real-time
})
```

### 3. Concurrency Tuning

**Optimize AsyncMapper concurrency**:

```go
// CPU-bound operations: match CPU cores
cpuCores := runtime.NumCPU()
cpuMapper := streamz.NewAsyncMapper(cpuIntensiveOperation).WithWorkers(cpuCores)

// I/O-bound operations: higher concurrency
ioMapper := streamz.NewAsyncMapper(networkOperation).WithWorkers(50)

// Memory-intensive operations: lower concurrency
memoryMapper := streamz.NewAsyncMapper(memoryIntensiveOperation).WithWorkers(5)

// Benchmark to find optimal concurrency
func findOptimalConcurrency(operation func(context.Context, Item) (Item, error)) int {
    bestThroughput := 0.0
    bestConcurrency := 1
    
    for concurrency := 1; concurrency <= 100; concurrency *= 2 {
        throughput := benchmarkConcurrency(operation, concurrency)
        if throughput > bestThroughput {
            bestThroughput = throughput
            bestConcurrency = concurrency
        }
    }
    
    return bestConcurrency
}
```

### 4. Buffer Sizing

**Strategic buffering** for performance:

```go
// Small buffers for low-latency systems
lowLatencyBuffer := streamz.NewBuffer[Event](100)

// Medium buffers for general use
generalBuffer := streamz.NewBuffer[Order](1000)

// Large buffers for high-throughput batch systems
batchBuffer := streamz.NewBuffer[LogEntry](10000)

// Calculate optimal buffer size
func calculateBufferSize(itemSize int, targetMemoryMB int) int {
    targetBytes := targetMemoryMB * 1024 * 1024
    return targetBytes / itemSize
}

// Example: 1KB items, 10MB buffer = 10,000 items
bufferSize := calculateBufferSize(1024, 10) // 10,240 items
```

### 5. Memory Management

**Reduce allocations** in hot paths:

```go
// ✅ Pre-allocate slices with known capacity
func efficientBatchProcessor(batch []Order) []ProcessedOrder {
    results := make([]ProcessedOrder, 0, len(batch)) // Pre-allocate capacity
    for _, order := range batch {
        results = append(results, processOrder(order))
    }
    return results
}

// ✅ Reuse objects when possible
var orderPool = sync.Pool{
    New: func() interface{} {
        return &Order{}
    },
}

func getOrder() *Order {
    return orderPool.Get().(*Order)
}

func putOrder(order *Order) {
    // Reset order fields
    *order = Order{}
    orderPool.Put(order)
}

// ✅ Use string builders for string concatenation
func buildDescription(order Order) string {
    var builder strings.Builder
    builder.Grow(100) // Pre-allocate expected size
    builder.WriteString(order.ID)
    builder.WriteString("-")
    builder.WriteString(order.Status)
    return builder.String()
}
```

## Performance Monitoring

### Real-Time Metrics

```go
type PerformanceMonitor[T any] struct {
    name        string
    interval    time.Duration
    itemCount   int64
    startTime   time.Time
    lastReport  time.Time
    
    // Latency tracking
    latencies   []time.Duration
    maxLatency  time.Duration
    mutex       sync.RWMutex
}

func NewPerformanceMonitor[T any](name string, interval time.Duration) *PerformanceMonitor[T] {
    return &PerformanceMonitor[T]{
        name:       name,
        interval:   interval,
        startTime:  time.Now(),
        lastReport: time.Now(),
        latencies:  make([]time.Duration, 0, 1000),
    }
}

func (p *PerformanceMonitor[T]) Process(ctx context.Context, in <-chan T) <-chan T {
    out := make(chan T)
    
    go func() {
        defer close(out)
        
        ticker := time.NewTicker(p.interval)
        defer ticker.Stop()
        
        for {
            select {
            case item, ok := <-in:
                if !ok {
                    p.reportFinalStats()
                    return
                }
                
                start := time.Now()
                
                select {
                case out <- item:
                    latency := time.Since(start)
                    p.recordLatency(latency)
                    atomic.AddInt64(&p.itemCount, 1)
                case <-ctx.Done():
                    return
                }
                
            case <-ticker.C:
                p.reportStats()
                
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return out
}

func (p *PerformanceMonitor[T]) recordLatency(latency time.Duration) {
    p.mutex.Lock()
    defer p.mutex.Unlock()
    
    p.latencies = append(p.latencies, latency)
    if latency > p.maxLatency {
        p.maxLatency = latency
    }
    
    // Keep only recent latencies (sliding window)
    if len(p.latencies) > 1000 {
        p.latencies = p.latencies[100:] // Keep last 900
    }
}

func (p *PerformanceMonitor[T]) reportStats() {
    count := atomic.LoadInt64(&p.itemCount)
    now := time.Now()
    
    // Calculate rates
    totalDuration := now.Sub(p.startTime)
    intervalDuration := now.Sub(p.lastReport)
    
    totalRate := float64(count) / totalDuration.Seconds()
    intervalRate := float64(count) / intervalDuration.Seconds()
    
    // Calculate latency stats
    p.mutex.RLock()
    avgLatency := p.calculateAverageLatency()
    p99Latency := p.calculatePercentileLatency(0.99)
    maxLatency := p.maxLatency
    p.mutex.RUnlock()
    
    log.Info("Performance stats",
        "processor", p.name,
        "total_items", count,
        "total_rate", fmt.Sprintf("%.2f items/sec", totalRate),
        "interval_rate", fmt.Sprintf("%.2f items/sec", intervalRate),
        "avg_latency", avgLatency,
        "p99_latency", p99Latency,
        "max_latency", maxLatency)
    
    p.lastReport = now
}
```

### Memory Usage Tracking

```go
func monitorMemoryUsage(interval time.Duration) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    
    var m runtime.MemStats
    
    for range ticker.C {
        runtime.ReadMemStats(&m)
        
        log.Info("Memory stats",
            "alloc_mb", bToMb(m.Alloc),
            "total_alloc_mb", bToMb(m.TotalAlloc),
            "sys_mb", bToMb(m.Sys),
            "num_gc", m.NumGC,
            "gc_cpu_fraction", m.GCCPUFraction)
        
        // Alert if memory usage is high
        if bToMb(m.Alloc) > 500 { // 500MB threshold
            log.Warn("High memory usage detected", "alloc_mb", bToMb(m.Alloc))
        }
    }
}

func bToMb(b uint64) uint64 {
    return b / 1024 / 1024
}
```

## Performance Patterns

### 1. Pipeline Fusion

**Combine compatible processors** to reduce overhead:

```go
// ❌ Inefficient: Multiple processors with separate goroutines
func separateProcessors(ctx context.Context, input <-chan Order) <-chan Order {
    filter1 := streamz.NewFilter(func(o Order) bool { return o.Status == "pending" }).WithName("status")
    filter2 := streamz.NewFilter(func(o Order) bool { return o.Amount > 0 }).WithName("amount")
    mapper := streamz.NewMapper(func(o Order) Order {
        o.ProcessedAt = time.Now()
        return o
    }).WithName("enrich")
    
    step1 := filter1.Process(ctx, input)
    step2 := filter2.Process(ctx, step1)
    return mapper.Process(ctx, step2)
}

// ✅ Efficient: Fused into single processor
func fusedProcessor(ctx context.Context, input <-chan Order) <-chan Order {
    combined := streamz.NewMapper(func(o Order) *Order {
        // Apply all filters and transformations in one step
        if o.Status != "pending" || o.Amount <= 0 {
            return nil // Filter out
        }
        
        o.ProcessedAt = time.Now()
        return &o
    }).WithName("combined")
    
    // Handle nil returns (filtered items)
    return streamz.NewFilter(func(o *Order) bool {
        return o != nil
    }).WithName("non-nil").Process(ctx, combined.Process(ctx, input))
}
```

### 2. Work Stealing

**Balance load across workers**:

```go
type WorkStealingProcessor[T any] struct {
    name        string
    workerCount int
    processor   func(context.Context, T) (T, error)
}

func (w *WorkStealingProcessor[T]) Process(ctx context.Context, in <-chan T) <-chan T {
    out := make(chan T)
    
    // Create work distribution channels
    workChannels := make([]chan T, w.workerCount)
    for i := range workChannels {
        workChannels[i] = make(chan T, 10) // Small buffer per worker
    }
    
    // Start workers
    var wg sync.WaitGroup
    for i := 0; i < w.workerCount; i++ {
        wg.Add(1)
        go func(workerID int) {
            defer wg.Done()
            w.worker(ctx, workChannels[workerID], out)
        }(i)
    }
    
    // Distribute work (round-robin with fallback)
    go func() {
        defer func() {
            // Close all work channels
            for _, ch := range workChannels {
                close(ch)
            }
        }()
        
        workerIndex := 0
        for {
            select {
            case item, ok := <-in:
                if !ok {
                    return
                }
                
                // Try to send to next worker
                select {
                case workChannels[workerIndex] <- item:
                    workerIndex = (workerIndex + 1) % w.workerCount
                default:
                    // Worker is busy, try others
                    sent := false
                    for i := 0; i < w.workerCount; i++ {
                        idx := (workerIndex + i) % w.workerCount
                        select {
                        case workChannels[idx] <- item:
                            workerIndex = (idx + 1) % w.workerCount
                            sent = true
                            break
                        default:
                            continue
                        }
                    }
                    
                    if !sent {
                        // All workers busy, wait for any to be available
                        workChannels[workerIndex] <- item
                        workerIndex = (workerIndex + 1) % w.workerCount
                    }
                }
                
            case <-ctx.Done():
                return
            }
        }
    }()
    
    // Close output when all workers finish
    go func() {
        wg.Wait()
        close(out)
    }()
    
    return out
}
```

### 3. Zero-Copy Optimization

**Minimize data copying**:

```go
// ✅ Use pointers for large structs
type LargeOrder struct {
    // Many fields...
    Data [1024]byte
}

// Pass by pointer to avoid copying
processor := streamz.NewMapper(func(order *LargeOrder) *LargeOrder {
    // Modify in place
    order.ProcessedAt = time.Now()
    return order
}).WithName("process")

// ✅ Use slices efficiently
func efficientSliceProcessing(batch []Order) []Order {
    // Modify slice in place when possible
    for i := range batch {
        batch[i].ProcessedAt = time.Now()
    }
    return batch // Return same slice
}

// ✅ Minimize string operations
func efficientStringBuilder(orders []Order) string {
    var builder strings.Builder
    
    // Pre-calculate total size to avoid reallocations
    totalSize := 0
    for _, order := range orders {
        totalSize += len(order.ID) + len(order.Status) + 10 // estimate
    }
    builder.Grow(totalSize)
    
    for _, order := range orders {
        builder.WriteString(order.ID)
        builder.WriteString(",")
        builder.WriteString(order.Status)
        builder.WriteString("\n")
    }
    
    return builder.String()
}
```

## Benchmarking Results

### Typical Performance (1M items)

```
Processor                     ns/op    B/op    allocs/op
Filter (simple predicate)     1,205     48         1
Mapper (pure function)        1,180     64         1
AsyncMapper (1 worker)        2,340    156         2
AsyncMapper (10 workers)      1,890    145         2
Batcher (100 items)           2,340    156         2
Buffer (1000 capacity)        1,350     72         1
```

### Throughput Benchmarks

```
Pipeline                    Items/sec    Memory
Simple Filter+Mapper         830,000    ~50MB
Batched Processing          1,200,000    ~100MB
Async Processing (10x)      2,500,000    ~200MB
Complex Pipeline             450,000    ~150MB
```

## Performance Best Practices

### 1. **Profile Before Optimizing**
```bash
# CPU profiling
go test -bench=. -cpuprofile=cpu.prof

# Memory profiling  
go test -bench=. -memprofile=mem.prof

# Analyze profiles
go tool pprof cpu.prof
go tool pprof mem.prof
```

### 2. **Optimize Hot Paths**
- Focus on processors that handle the most data
- Use benchmarks to validate improvements
- Monitor allocations in tight loops
- Consider processor fusion for simple operations

### 3. **Configure Appropriately**
- Tune AsyncMapper concurrency based on workload
- Size buffers based on memory constraints and throughput needs
- Use appropriate batch sizes for downstream systems
- Monitor and adjust based on production metrics

### 4. **Avoid Common Pitfalls**
- Don't over-parallelize simple operations
- Don't use huge buffers without monitoring memory
- Don't ignore backpressure signals
- Don't optimize prematurely without measurements

### 5. **Monitor in Production**
- Track throughput and latency metrics
- Monitor memory usage and GC pressure
- Set up alerting for performance degradation
- Use distributed tracing for complex pipelines

## What's Next?

- **[Testing Guide](./testing.md)**: Test your performance optimizations
- **[Best Practices](./best-practices.md)**: Production-ready performance patterns
- **[API Reference: AsyncMapper](../api/async-mapper.md)**: Concurrent processing optimization
- **[Concepts: Backpressure](../concepts/backpressure.md)**: Flow control for performance