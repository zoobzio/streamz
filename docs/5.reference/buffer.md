---
title: Buffer
description: Decouple producer and consumer speeds with buffering
author: zoobzio
published: 2025-01-09
updated: 2025-01-09
tags:
  - reference
  - processors
  - flow-control
---

# Buffer

Adds buffering between producers and consumers to handle burst traffic and decouple processing speeds. Essential for managing backpressure in streaming systems.

## Overview

The Buffer processor provides a configurable buffer between stream processing stages. It helps smooth out variations in processing rates, handle burst traffic, and prevent fast producers from overwhelming slow consumers.

**Type Signature:** `chan T → chan T`  
**Buffering:** Configurable capacity  
**Backpressure:** Blocks when full (natural backpressure)

## When to Use

- **Rate mismatch** - Producer faster than consumer
- **Burst traffic handling** - Smooth out traffic spikes
- **Decoupling components** - Allow independent scaling of pipeline stages
- **Network jitter** - Handle variable network delays
- **Batch processing** - Accumulate items before expensive operations
- **Resource utilization** - Improve CPU/memory efficiency

## Constructor

```go
func NewBuffer[T any](capacity int) *Buffer[T]
```

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `capacity` | `int` | Yes | Buffer size (number of items, must be > 0) |

## Examples

### Basic Buffering

```go
// Add buffering between fast producer and slow consumer
buffer := streamz.NewBuffer[int](1000)

// Fast producer
input := make(chan int)
go func() {
    defer close(input)
    for i := 0; i < 10000; i++ {
        input <- i // Sends quickly
    }
}()

// Add buffer
buffered := buffer.Process(ctx, input)

// Slow consumer
go func() {
    for item := range buffered {
        time.Sleep(time.Millisecond) // Slow processing
        processItem(item)
    }
}()
```

### Pipeline Buffering

```go
// Buffer at different stages of pipeline
orders := make(chan Order)

// Small buffer after input
inputBuffer := streamz.NewBuffer[Order](100)

// Validation (fast)
validator := streamz.NewFilter(isValidOrder).WithName("valid")

// Large buffer before expensive processing
processingBuffer := streamz.NewBuffer[Order](5000)

// Expensive processing (slow)
processor := streamz.NewAsyncMapper(expensiveProcessing).WithWorkers(10)

// Small buffer before output
outputBuffer := streamz.NewBuffer[ProcessedOrder](100)

// Chain with strategic buffering
buffered1 := inputBuffer.Process(ctx, orders)
validated := validator.Process(ctx, buffered1)
buffered2 := processingBuffer.Process(ctx, validated)
processed := processor.Process(ctx, buffered2)
final := outputBuffer.Process(ctx, processed)
```

### Database Batch Processing

```go
type DatabaseWriter struct {
    db     *sql.DB
    buffer *streamz.Buffer[Record]
    batcher *streamz.Batcher[Record]
}

func NewDatabaseWriter(db *sql.DB) *DatabaseWriter {
    return &DatabaseWriter{
        db: db,
        // Large buffer to handle bursts
        buffer: streamz.NewBuffer[Record](10000),
        // Batch for efficient database writes
        batcher: streamz.NewBatcher[Record](streamz.BatchConfig{
            MaxSize:    500,
            MaxLatency: 5 * time.Second,
        }),
    }
}

func (dw *DatabaseWriter) Process(ctx context.Context, records <-chan Record) <-chan BatchResult {
    // Buffer incoming records
    buffered := dw.buffer.Process(ctx, records)
    
    // Batch for efficient database operations
    batched := dw.batcher.Process(ctx, buffered)
    
    // Write batches to database
    writer := streamz.NewAsyncMapper(func(ctx context.Context, batch []Record) (BatchResult, error) {
        return dw.writeBatch(ctx, batch)
    }).WithWorkers(2)
    
    return writer.Process(ctx, batched)
}

func (dw *DatabaseWriter) writeBatch(ctx context.Context, records []Record) (BatchResult, error) {
    tx, err := dw.db.BeginTx(ctx, nil)
    if err != nil {
        return BatchResult{}, err
    }
    defer tx.Rollback()
    
    // Bulk insert logic here
    for _, record := range records {
        if err := insertRecord(tx, record); err != nil {
            return BatchResult{}, err
        }
    }
    
    if err := tx.Commit(); err != nil {
        return BatchResult{}, err
    }
    
    return BatchResult{Count: len(records), Success: true}, nil
}
```

### Network Request Buffering

```go
type APIClient struct {
    httpClient *http.Client
    buffer     *streamz.Buffer[APIRequest]
    rateLimiter *streamz.Throttle[APIRequest]
}

func NewAPIClient(requestsPerSecond int) *APIClient {
    return &APIClient{
        httpClient: &http.Client{Timeout: 30 * time.Second},
        // Buffer requests to handle bursts
        buffer: streamz.NewBuffer[APIRequest](1000),
        // Rate limit to respect API limits
        rateLimiter: streamz.NewThrottle[APIRequest](requestsPerSecond),
    }
}

func (ac *APIClient) Process(ctx context.Context, requests <-chan APIRequest) <-chan APIResponse {
    // Buffer incoming requests
    buffered := ac.buffer.Process(ctx, requests)
    
    // Apply rate limiting
    limited := ac.rateLimiter.Process(ctx, buffered)
    
    // Process requests with retries
    processor := streamz.NewAsyncMapper(func(ctx context.Context, req APIRequest) (APIResponse, error) {
        return ac.makeRequest(ctx, req)
    }).WithWorkers(10)
    
    return processor.Process(ctx, limited)
}
```

## Buffer Sizing Guidelines

### Memory-Based Sizing

```go
// Calculate buffer size based on memory constraints
func calculateBufferSize[T any](maxMemoryMB int) int {
    var sample T
    itemSize := int(unsafe.Sizeof(sample))
    
    // Add overhead for pointers and channel mechanics
    itemSizeWithOverhead := itemSize + 64
    
    maxBytes := maxMemoryMB * 1024 * 1024
    bufferSize := maxBytes / itemSizeWithOverhead
    
    // Ensure minimum and maximum bounds
    if bufferSize < 10 {
        bufferSize = 10
    }
    if bufferSize > 100000 {
        bufferSize = 100000
    }
    
    return bufferSize
}

// Usage
bufferSize := calculateBufferSize[Order](50) // 50MB limit
buffer := streamz.NewBuffer[Order](bufferSize)
```

### Throughput-Based Sizing

```go
// Size buffer based on throughput requirements
func calculateThroughputBuffer(
    inputRate float64,      // items/sec
    outputRate float64,     // items/sec
    burstDuration time.Duration, // how long burst lasts
) int {
    if outputRate >= inputRate {
        // No buffering needed for sustained rates
        return int(inputRate * burstDuration.Seconds())
    }
    
    // Buffer for the difference in rates
    rateDifference := inputRate - outputRate
    bufferSize := int(rateDifference * burstDuration.Seconds())
    
    // Add safety margin
    safetyMargin := bufferSize / 4
    return bufferSize + safetyMargin
}

// Example: Input 1000/sec, output 800/sec, 30 second bursts
bufferSize := calculateThroughputBuffer(1000, 800, 30*time.Second) // 7500 items
buffer := streamz.NewBuffer[Event](bufferSize)
```

### Latency-Based Sizing

```go
// Size buffer based on acceptable latency
func calculateLatencyBuffer(
    throughput float64,        // items/sec
    maxLatency time.Duration,  // acceptable delay
) int {
    // Maximum items that can accumulate within latency window
    maxItems := throughput * maxLatency.Seconds()
    
    // Conservative sizing
    return int(maxItems / 2)
}

// Example: 500 items/sec, max 10ms latency
bufferSize := calculateLatencyBuffer(500, 10*time.Millisecond) // 2-3 items
buffer := streamz.NewBuffer[Request](bufferSize)
```

## Performance Characteristics

### Memory Usage

```go
// Monitor buffer memory usage
type BufferMonitor[T any] struct {
    buffer   *streamz.Buffer[T]
    capacity int
    itemSize int
}

func NewBufferMonitor[T any](capacity int) *BufferMonitor[T] {
    var sample T
    return &BufferMonitor[T]{
        buffer:   streamz.NewBuffer[T](capacity),
        capacity: capacity,
        itemSize: int(unsafe.Sizeof(sample)),
    }
}

func (bm *BufferMonitor[T]) MaxMemoryUsage() int {
    return bm.capacity * bm.itemSize
}

func (bm *BufferMonitor[T]) Process(ctx context.Context, in <-chan T) <-chan T {
    // Start monitoring
    go bm.monitorUsage(ctx)
    
    return bm.buffer.Process(ctx, in)
}

func (bm *BufferMonitor[T]) monitorUsage(ctx context.Context) {
    ticker := time.NewTicker(time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            // Buffer utilization would need to be tracked internally
            // This is a simplified example
            memoryMB := bm.MaxMemoryUsage() / (1024 * 1024)
            log.Info("Buffer memory usage", "max_memory_mb", memoryMB)
            
        case <-ctx.Done():
            return
        }
    }
}
```

### Throughput Impact

```go
func BenchmarkBufferThroughput(b *testing.B) {
    ctx := context.Background()
    
    bufferSizes := []int{0, 10, 100, 1000, 10000}
    
    for _, size := range bufferSizes {
        b.Run(fmt.Sprintf("buffer-%d", size), func(b *testing.B) {
            var buffer streamz.Processor[int, int]
            
            if size == 0 {
                // Direct channel (no buffer)
                buffer = &directChannel[int]{}
            } else {
                buffer = streamz.NewBuffer[int](size)
            }
            
            b.ResetTimer()
            
            for i := 0; i < b.N; i++ {
                input := make(chan int, 1000)
                output := buffer.Process(ctx, input)
                
                // Send test data
                go func() {
                    defer close(input)
                    for j := 0; j < 1000; j++ {
                        input <- j
                    }
                }()
                
                // Consume output
                count := 0
                for range output {
                    count++
                }
                
                if count != 1000 {
                    b.Fatalf("Expected 1000 items, got %d", count)
                }
            }
        })
    }
}
```

## Advanced Usage

### Dynamic Buffer Sizing

```go
type AdaptiveBuffer[T any] struct {
    currentCapacity int64
    buffer          atomic.Value // Stores current *streamz.Buffer[T]
    mutex           sync.RWMutex
    metrics         *BufferMetrics
}

type BufferMetrics struct {
    DroppedItems   int64
    FullCount      int64
    UtilizationSum int64
    Measurements   int64
}

func NewAdaptiveBuffer[T any](initialCapacity int) *AdaptiveBuffer[T] {
    ab := &AdaptiveBuffer[T]{
        currentCapacity: int64(initialCapacity),
        metrics:         &BufferMetrics{},
    }
    
    ab.buffer.Store(streamz.NewBuffer[T](initialCapacity))
    
    // Start adaptive sizing
    go ab.adaptiveResize()
    
    return ab
}

func (ab *AdaptiveBuffer[T]) Process(ctx context.Context, in <-chan T) <-chan T {
    buffer := ab.buffer.Load().(*streamz.Buffer[T])
    return buffer.Process(ctx, in)
}

func (ab *AdaptiveBuffer[T]) adaptiveResize() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        measurements := atomic.LoadInt64(&ab.metrics.Measurements)
        if measurements == 0 {
            continue
        }
        
        // Calculate average utilization
        utilizationSum := atomic.LoadInt64(&ab.metrics.UtilizationSum)
        avgUtilization := float64(utilizationSum) / float64(measurements)
        
        currentCapacity := atomic.LoadInt64(&ab.currentCapacity)
        newCapacity := currentCapacity
        
        // Resize based on utilization
        if avgUtilization > 0.8 {
            // High utilization - increase buffer
            newCapacity = int64(float64(currentCapacity) * 1.5)
        } else if avgUtilization < 0.3 {
            // Low utilization - decrease buffer
            newCapacity = int64(float64(currentCapacity) * 0.7)
        }
        
        // Apply bounds
        if newCapacity < 10 {
            newCapacity = 10
        }
        if newCapacity > 100000 {
            newCapacity = 100000
        }
        
        if newCapacity != currentCapacity {
            log.Info("Resizing buffer", 
                "old_capacity", currentCapacity,
                "new_capacity", newCapacity,
                "avg_utilization", avgUtilization)
            
            atomic.StoreInt64(&ab.currentCapacity, newCapacity)
            ab.buffer.Store(streamz.NewBuffer[T](int(newCapacity)))
            
            // Reset metrics
            atomic.StoreInt64(&ab.metrics.UtilizationSum, 0)
            atomic.StoreInt64(&ab.metrics.Measurements, 0)
        }
    }
}
```

### Health Monitoring

```go
type HealthMonitoredBuffer[T any] struct {
    buffer      *streamz.Buffer[T]
    capacity    int
    health      atomic.Value // Stores HealthStatus
    utilization float64
    mutex       sync.RWMutex
}

type HealthStatus struct {
    Healthy         bool    `json:"healthy"`
    Utilization     float64 `json:"utilization"`
    Capacity        int     `json:"capacity"`
    QueuedItems     int     `json:"queued_items"`
    LastUpdated     time.Time `json:"last_updated"`
    WarningMessages []string `json:"warnings,omitempty"`
}

func NewHealthMonitoredBuffer[T any](capacity int) *HealthMonitoredBuffer[T] {
    hmb := &HealthMonitoredBuffer[T]{
        buffer:   streamz.NewBuffer[T](capacity),
        capacity: capacity,
    }
    
    hmb.health.Store(HealthStatus{
        Healthy:     true,
        Utilization: 0.0,
        Capacity:    capacity,
        LastUpdated: time.Now(),
    })
    
    return hmb
}

func (hmb *HealthMonitoredBuffer[T]) Process(ctx context.Context, in <-chan T) <-chan T {
    // Start health monitoring
    go hmb.monitorHealth(ctx)
    
    return hmb.buffer.Process(ctx, in)
}

func (hmb *HealthMonitoredBuffer[T]) GetHealth() HealthStatus {
    return hmb.health.Load().(HealthStatus)
}

func (hmb *HealthMonitoredBuffer[T]) monitorHealth(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            hmb.updateHealthStatus()
        case <-ctx.Done():
            return
        }
    }
}

func (hmb *HealthMonitoredBuffer[T]) updateHealthStatus() {
    hmb.mutex.RLock()
    utilization := hmb.utilization
    hmb.mutex.RUnlock()
    
    status := HealthStatus{
        Utilization: utilization,
        Capacity:    hmb.capacity,
        QueuedItems: int(utilization * float64(hmb.capacity)),
        LastUpdated: time.Now(),
    }
    
    // Determine health status
    var warnings []string
    
    if utilization > 0.9 {
        status.Healthy = false
        warnings = append(warnings, "Buffer utilization > 90%")
    } else if utilization > 0.8 {
        warnings = append(warnings, "Buffer utilization > 80%")
    }
    
    if len(warnings) == 0 {
        status.Healthy = true
    }
    
    status.WarningMessages = warnings
    hmb.health.Store(status)
}
```

## Testing

```go
func TestBufferBasicOperation(t *testing.T) {
    ctx := context.Background()
    
    buffer := streamz.NewBuffer[int](10)
    
    input := sendData(ctx, []int{1, 2, 3, 4, 5})
    output := buffer.Process(ctx, input)
    results := collectAll(output)
    
    expected := []int{1, 2, 3, 4, 5}
    assert.Equal(t, expected, results)
}

func TestBufferBackpressure(t *testing.T) {
    ctx := context.Background()
    
    buffer := streamz.NewBuffer[int](2) // Small buffer
    
    input := make(chan int)
    output := buffer.Process(ctx, input)
    
    // Send more items than buffer capacity
    go func() {
        defer close(input)
        for i := 1; i <= 5; i++ {
            input <- i
            time.Sleep(10 * time.Millisecond)
        }
    }()
    
    // Slow consumer
    var results []int
    for item := range output {
        time.Sleep(50 * time.Millisecond) // Slower than producer
        results = append(results, item)
    }
    
    // All items should eventually be processed
    expected := []int{1, 2, 3, 4, 5}
    assert.Equal(t, expected, results)
}

func TestBufferContextCancellation(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())
    
    buffer := streamz.NewBuffer[int](100)
    
    input := make(chan int)
    output := buffer.Process(ctx, input)
    
    // Send some data
    go func() {
        defer close(input)
        for i := 1; i <= 1000; i++ {
            select {
            case input <- i:
            case <-ctx.Done():
                return
            }
        }
    }()
    
    // Cancel after short time
    go func() {
        time.Sleep(100 * time.Millisecond)
        cancel()
    }()
    
    var results []int
    for item := range output {
        results = append(results, item)
    }
    
    // Should have processed some items before cancellation
    assert.Greater(t, len(results), 0)
    assert.Less(t, len(results), 1000)
}
```

## Best Practices

1. **Size appropriately** - Consider memory, latency, and throughput requirements
2. **Monitor utilization** - Track buffer usage to optimize sizing
3. **Place strategically** - Buffer before expensive operations and after burst sources
4. **Set reasonable limits** - Avoid unbounded growth that can exhaust memory
5. **Test backpressure** - Verify system behavior when buffers fill up
6. **Consider alternatives** - Use dropping or sliding buffers for real-time systems
7. **Plan for failure** - Handle cases where buffers fill completely

## Related Processors

- **[DroppingBuffer](./dropping-buffer.md)**: Drop items when buffer is full
- **[SlidingBuffer](./sliding-buffer.md)**: Keep only recent items
- **[Batcher](./batcher.md)**: Group buffered items for processing
- **[Monitor](./monitor.md)**: Observe buffer performance

## See Also

- **[Concepts: Backpressure](../concepts/backpressure.md)**: Understanding flow control
- **[Concepts: Channels](../concepts/channels.md)**: Channel management patterns
- **[Guides: Performance](../guides/performance.md)**: Optimizing buffer performance
- **[Guides: Patterns](../guides/patterns.md)**: Real-world buffering patterns