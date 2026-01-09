---
title: AsyncMapper
description: Transform items concurrently with order preservation
author: zoobzio
published: 2025-01-09
updated: 2025-01-09
tags:
  - reference
  - processors
  - transformation
  - concurrency
---

# AsyncMapper

Transforms items concurrently while preserving order, ideal for I/O-bound operations that benefit from parallel processing.

## Overview

The AsyncMapper processor applies transformation functions concurrently across multiple goroutines while maintaining the original order of items. This is essential for I/O-bound operations like API calls, database queries, or file operations where parallelism can significantly improve throughput.

**Type Signature:** `chan In → chan Out`  
**Concurrency:** Configurable worker pool  
**Order Preservation:** Yes

## When to Use

- **External API calls** - Enrich data from REST APIs, microservices
- **Database queries** - Fetch related data from databases
- **File operations** - Read/write files, process images
- **Network operations** - HTTP requests, DNS lookups
- **CPU-intensive operations** - Complex calculations that can be parallelized
- **I/O-bound transformations** - Any operation that benefits from concurrency

## Constructor

```go
func NewAsyncMapper[In, Out any](mapper func(context.Context, In) (Out, error)) *AsyncMapper[In, Out]
```

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `mapper` | `func(context.Context, In) (Out, error)` | Yes | Transformation function that may return errors |

## Fluent Methods

| Method | Description |
|--------|-------------|
| `WithWorkers(count int)` | Sets the number of concurrent workers (default: runtime.NumCPU()) |

## Examples

### Basic Concurrent Processing

```go
// Simulate expensive operation
expensiveOperation := func(ctx context.Context, n int) (int, error) {
    // Simulate network call or computation
    time.Sleep(100 * time.Millisecond)
    return n * n, nil
}

// Process with 10 concurrent workers
asyncMapper := streamz.NewAsyncMapper(expensiveOperation).WithWorkers(10)

numbers := sendData(ctx, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
results := asyncMapper.Process(ctx, numbers)

// Results will be in original order: [1, 4, 9, 16, 25, 36, 49, 64, 81, 100]
// But processing happened concurrently
output := collectAll(results)
```

### External API Enrichment

```go
type Order struct {
    ID         string
    CustomerID string
    Amount     float64
}

type EnrichedOrder struct {
    Order
    CustomerName  string
    CustomerTier  string
    IsVIP         bool
}

// Mock customer service
type CustomerService struct {
    client *http.Client
}

func (cs *CustomerService) GetCustomer(ctx context.Context, id string) (Customer, error) {
    // Simulate API call
    req, err := http.NewRequestWithContext(ctx, "GET", 
        fmt.Sprintf("/api/customers/%s", id), nil)
    if err != nil {
        return Customer{}, err
    }
    
    resp, err := cs.client.Do(req)
    if err != nil {
        return Customer{}, fmt.Errorf("customer service call failed: %w", err)
    }
    defer resp.Body.Close()
    
    var customer Customer
    if err := json.NewDecoder(resp.Body).Decode(&customer); err != nil {
        return Customer{}, fmt.Errorf("failed to decode customer: %w", err)
    }
    
    return customer, nil
}

// Create concurrent enricher
customerService := &CustomerService{client: &http.Client{Timeout: 5 * time.Second}}

enricher := streamz.NewAsyncMapper(func(ctx context.Context, order Order) (EnrichedOrder, error) {
    customer, err := customerService.GetCustomer(ctx, order.CustomerID)
    if err != nil {
        return EnrichedOrder{}, fmt.Errorf("failed to enrich order %s: %w", order.ID, err)
    }
    
    return EnrichedOrder{
        Order:        order,
        CustomerName: customer.Name,
        CustomerTier: customer.Tier,
        IsVIP:        customer.Tier == "VIP",
    }, nil
}).WithWorkers(20)

orders := []Order{
    {ID: "1", CustomerID: "cust1", Amount: 100},
    {ID: "2", CustomerID: "cust2", Amount: 200},
    {ID: "3", CustomerID: "cust3", Amount: 300},
}

input := sendData(ctx, orders)
enriched := enricher.Process(ctx, input)
results := collectAll(enriched)
```

### Database Batch Processing

```go
type UserID string
type UserProfile struct {
    ID       UserID
    Name     string
    Settings map[string]interface{}
    LastSeen time.Time
}

// Database service
type UserDB interface {
    GetUserProfile(ctx context.Context, id UserID) (UserProfile, error)
}

func NewUserProfileEnricher(db UserDB, concurrency int) *streamz.AsyncMapper[UserID, UserProfile] {
    return streamz.NewAsyncMapper(func(ctx context.Context, userID UserID) (UserProfile, error) {
        // Add database query timeout
        ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
        defer cancel()
        
        profile, err := db.GetUserProfile(ctx, userID)
        if err != nil {
            return UserProfile{}, fmt.Errorf("failed to fetch profile for user %s: %w", userID, err)
        }
        
        return profile, nil
    }).WithWorkers(concurrency)
}

// Usage
profileEnricher := NewUserProfileEnricher(database, 15)
userIDs := sendData(ctx, []UserID{"user1", "user2", "user3", "user4"})
profiles := profileEnricher.Process(ctx, userIDs)
```

### File Processing

```go
type FileInfo struct {
    Path string
    Size int64
}

type ProcessedFile struct {
    Path    string
    Size    int64
    Hash    string
    Content []byte
}

// Process files concurrently
fileProcessor := streamz.NewAsyncMapper(func(ctx context.Context, file FileInfo) (ProcessedFile, error) {
    // Read file
    content, err := os.ReadFile(file.Path)
    if err != nil {
        return ProcessedFile{}, fmt.Errorf("failed to read file %s: %w", file.Path, err)
    }
    
    // Compute hash
    hash := sha256.Sum256(content)
    
    return ProcessedFile{
        Path:    file.Path,
        Size:    file.Size,
        Hash:    hex.EncodeToString(hash[:]),
        Content: content,
    }, nil
}).WithWorkers(5)

files := []FileInfo{
    {Path: "/tmp/file1.txt", Size: 1024},
    {Path: "/tmp/file2.txt", Size: 2048},
    {Path: "/tmp/file3.txt", Size: 512},
}

input := sendData(ctx, files)
processed := fileProcessor.Process(ctx, input)
results := collectAll(processed)
```

## Performance Characteristics

### Concurrency Guidelines

```go
// CPU-bound operations: Match CPU cores
cpuCores := runtime.NumCPU()
cpuMapper := streamz.NewAsyncMapper(cpuIntensiveFunction).WithWorkers(cpuCores)

// I/O-bound operations: Higher concurrency
ioMapper := streamz.NewAsyncMapper(networkOperation).WithWorkers(50)

// Memory-intensive operations: Lower concurrency
memoryMapper := streamz.NewAsyncMapper(memoryIntensiveOperation).WithWorkers(5)

// Database operations: Match connection pool size
dbMapper := streamz.NewAsyncMapper(databaseQuery).WithWorkers(10) // If pool size is 10
```

### Throughput Optimization

```go
// Benchmark different concurrency levels
func findOptimalConcurrency(operation func(context.Context, Item) (Item, error)) int {
    concurrencyLevels := []int{1, 2, 5, 10, 20, 50, 100}
    bestThroughput := 0.0
    bestConcurrency := 1
    
    for _, concurrency := range concurrencyLevels {
        throughput := benchmarkThroughput(operation, concurrency)
        if throughput > bestThroughput {
            bestThroughput = throughput
            bestConcurrency = concurrency
        }
    }
    
    return bestConcurrency
}

func benchmarkThroughput(operation func(context.Context, Item) (Item, error), concurrency int) float64 {
    ctx := context.Background()
    mapper := streamz.NewAsyncMapper(operation).WithWorkers(concurrency)
    
    itemCount := 1000
    start := time.Now()
    
    input := generateTestData(itemCount)
    output := mapper.Process(ctx, input)
    
    count := 0
    for range output {
        count++
    }
    
    duration := time.Since(start)
    return float64(count) / duration.Seconds()
}
```

## Error Handling

### Error Strategies

AsyncMapper follows the "skip on error" pattern by default:

```go
// Items that error are skipped, pipeline continues
enricher := streamz.NewAsyncMapper(func(ctx context.Context, order Order) (EnrichedOrder, error) {
    if order.CustomerID == "" {
        return EnrichedOrder{}, errors.New("missing customer ID")
    }
    
    // This order will be skipped if enrichment fails
    return enrichOrder(ctx, order)
}).WithWorkers(10)

// Input: [order1, order2(error), order3, order4(error), order5]
// Output: [enrichedOrder1, enrichedOrder3, enrichedOrder5]
```

### Custom Error Handling

For more sophisticated error handling, wrap the result:

```go
type Result[T any] struct {
    Value T
    Error error
    Index int // For tracking original position
}

func NewSafeAsyncMapper[In, Out any](concurrency int, mapper func(context.Context, In) (Out, error)) *streamz.AsyncMapper[In, Result[Out]] {
    return streamz.NewAsyncMapper(func(ctx context.Context, item In) (Result[Out], error) {
        result, err := mapper(ctx, item)
        return Result[Out]{
            Value: result,
            Error: err,
        }, nil // Never return error - wrap it instead
    }).WithWorkers(concurrency)
}

// Usage
safeMapper := NewSafeAsyncMapper(10, riskyOperation)
results := safeMapper.Process(ctx, input)

for result := range results {
    if result.Error != nil {
        log.Error("Processing failed", "error", result.Error)
        // Handle error (retry, DLQ, etc.)
    } else {
        // Process successful result
        processSuccess(result.Value)
    }
}
```

### Timeout Management

```go
// Individual operation timeouts
timeoutMapper := streamz.NewAsyncMapper(func(ctx context.Context, item Item) (ProcessedItem, error) {
    // Set timeout for each operation
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    
    return expensiveOperation(ctx, item)
}).WithWorkers(10)

// Context timeout for entire pipeline
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

output := timeoutMapper.Process(ctx, input)
```

### Circuit Breaker Integration

```go
type CircuitBreakerMapper[In, Out any] struct {
    mapper  func(context.Context, In) (Out, error)
    breaker *CircuitBreaker
}

func (cb *CircuitBreakerMapper[In, Out]) Process(ctx context.Context, in <-chan In) <-chan Out {
    asyncMapper := streamz.NewAsyncMapper(func(ctx context.Context, item In) (Out, error) {
        if cb.breaker.IsOpen() {
            var zero Out
            return zero, errors.New("circuit breaker is open")
        }
        
        result, err := cb.mapper(ctx, item)
        if err != nil {
            cb.breaker.RecordFailure()
            return result, err
        }
        
        cb.breaker.RecordSuccess()
        return result, nil
    }).WithWorkers(10)
    
    return asyncMapper.Process(ctx, in)
}
```

## Advanced Usage

### Dynamic Concurrency

```go
type DynamicAsyncMapper[In, Out any] struct {
    mapper      func(context.Context, In) (Out, error)
    concurrency int64
    processor   atomic.Value // Stores current AsyncMapper
}

func NewDynamicAsyncMapper[In, Out any](initialConcurrency int, mapper func(context.Context, In) (Out, error)) *DynamicAsyncMapper[In, Out] {
    dam := &DynamicAsyncMapper[In, Out]{
        mapper: mapper,
    }
    dam.SetConcurrency(initialConcurrency)
    return dam
}

func (dam *DynamicAsyncMapper[In, Out]) SetConcurrency(concurrency int) {
    atomic.StoreInt64(&dam.concurrency, int64(concurrency))
    newProcessor := streamz.NewAsyncMapper(dam.mapper).WithWorkers(concurrency)
    dam.processor.Store(newProcessor)
}

func (dam *DynamicAsyncMapper[In, Out]) Process(ctx context.Context, in <-chan In) <-chan Out {
    processor := dam.processor.Load().(*streamz.AsyncMapper[In, Out])
    return processor.Process(ctx, in)
}

// Auto-adjust concurrency based on performance
go func() {
    ticker := time.NewTicker(time.Minute)
    defer ticker.Stop()
    
    for range ticker.C {
        currentConcurrency := int(atomic.LoadInt64(&dam.concurrency))
        
        // Measure current performance
        throughput := measureThroughput()
        
        // Adjust concurrency based on performance
        if throughput < targetThroughput && currentConcurrency < maxConcurrency {
            dam.SetConcurrency(currentConcurrency + 5)
        } else if throughput > targetThroughput*1.2 && currentConcurrency > 1 {
            dam.SetConcurrency(max(1, currentConcurrency-5))
        }
    }
}()
```

### Resource Pool Integration

```go
type PooledAsyncMapper[In, Out any] struct {
    pool    sync.Pool
    mapper  func(context.Context, In, Resource) (Out, error)
    timeout time.Duration
}

type Resource struct {
    ID   string
    Conn *sql.DB
    // Other expensive resources
}

func NewPooledAsyncMapper[In, Out any](
    concurrency int,
    resourceFactory func() Resource,
    mapper func(context.Context, In, Resource) (Out, error),
) *PooledAsyncMapper[In, Out] {
    pam := &PooledAsyncMapper[In, Out]{
        mapper:  mapper,
        timeout: 30 * time.Second,
        pool: sync.Pool{
            New: func() interface{} {
                return resourceFactory()
            },
        },
    }
    
    return pam
}

func (pam *PooledAsyncMapper[In, Out]) Process(ctx context.Context, in <-chan In) <-chan Out {
    asyncMapper := streamz.NewAsyncMapper(func(ctx context.Context, item In) (Out, error) {
        // Get resource from pool
        resource := pam.pool.Get().(Resource)
        defer pam.pool.Put(resource)
        
        // Add timeout
        ctx, cancel := context.WithTimeout(ctx, pam.timeout)
        defer cancel()
        
        return pam.mapper(ctx, item, resource)
    }).WithWorkers(10)
    
    return asyncMapper.Process(ctx, in)
}
```

## Monitoring and Observability

```go
type MonitoredAsyncMapper[In, Out any] struct {
    mapper      func(context.Context, In) (Out, error)
    concurrency int
    metrics     *AsyncMapperMetrics
}

type AsyncMapperMetrics struct {
    ProcessingTime prometheus.Histogram
    ErrorRate      prometheus.Counter
    ActiveWorkers  prometheus.Gauge
    QueueDepth     prometheus.Gauge
}

func NewMonitoredAsyncMapper[In, Out any](
    name string,
    concurrency int,
    mapper func(context.Context, In) (Out, error),
) *MonitoredAsyncMapper[In, Out] {
    
    metrics := &AsyncMapperMetrics{
        ProcessingTime: prometheus.NewHistogram(prometheus.HistogramOpts{
            Name: "async_mapper_processing_duration_seconds",
            Help: "Time spent processing items",
            ConstLabels: prometheus.Labels{"mapper": name},
        }),
        ErrorRate: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "async_mapper_errors_total",
            Help: "Total number of processing errors",
            ConstLabels: prometheus.Labels{"mapper": name},
        }),
        ActiveWorkers: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "async_mapper_active_workers",
            Help: "Number of active worker goroutines",
            ConstLabels: prometheus.Labels{"mapper": name},
        }),
    }
    
    return &MonitoredAsyncMapper[In, Out]{
        mapper:      mapper,
        concurrency: concurrency,
        metrics:     metrics,
    }
}

func (mam *MonitoredAsyncMapper[In, Out]) Process(ctx context.Context, in <-chan In) <-chan Out {
    instrumentedMapper := func(ctx context.Context, item In) (Out, error) {
        start := time.Now()
        mam.metrics.ActiveWorkers.Inc()
        defer mam.metrics.ActiveWorkers.Dec()
        defer mam.metrics.ProcessingTime.Observe(time.Since(start).Seconds())
        
        result, err := mam.mapper(ctx, item)
        if err != nil {
            mam.metrics.ErrorRate.Inc()
        }
        
        return result, err
    }
    
    asyncMapper := streamz.NewAsyncMapper(instrumentedMapper).WithWorkers(mam.concurrency)
    return asyncMapper.Process(ctx, in)
}
```

## Testing

```go
func TestAsyncMapperOrdering(t *testing.T) {
    ctx := context.Background()
    
    // Function with variable delay to test ordering
    variableDelay := func(ctx context.Context, n int) (int, error) {
        // Higher numbers take longer (reverse order processing)
        delay := time.Duration(10-n) * time.Millisecond
        time.Sleep(delay)
        return n * 2, nil
    }
    
    mapper := streamz.NewAsyncMapper(variableDelay).WithWorkers(5)
    
    input := sendData(ctx, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
    output := mapper.Process(ctx, input)
    results := collectAll(output)
    
    // Verify order is preserved despite variable processing times
    expected := []int{2, 4, 6, 8, 10, 12, 14, 16, 18, 20}
    assert.Equal(t, expected, results)
}

func TestAsyncMapperErrorHandling(t *testing.T) {
    ctx := context.Background()
    
    // Function that errors on even numbers
    errorOnEven := func(ctx context.Context, n int) (int, error) {
        if n%2 == 0 {
            return 0, fmt.Errorf("error on even number: %d", n)
        }
        return n * 2, nil
    }
    
    mapper := streamz.NewAsyncMapper(errorOnEven).WithWorkers(3)
    
    input := sendData(ctx, []int{1, 2, 3, 4, 5, 6, 7, 8})
    output := mapper.Process(ctx, input)
    results := collectAll(output)
    
    // Only odd numbers should pass through (doubled)
    expected := []int{2, 6, 10, 14} // 1*2, 3*2, 5*2, 7*2
    assert.Equal(t, expected, results)
}

func TestAsyncMapperContextCancellation(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())
    
    slowFunction := func(ctx context.Context, n int) (int, error) {
        select {
        case <-time.After(100 * time.Millisecond):
            return n * 2, nil
        case <-ctx.Done():
            return 0, ctx.Err()
        }
    }
    
    mapper := streamz.NewAsyncMapper(slowFunction).WithWorkers(2)
    
    input := make(chan int)
    output := mapper.Process(ctx, input)
    
    // Send some data
    go func() {
        defer close(input)
        for i := 1; i <= 10; i++ {
            input <- i
        }
    }()
    
    // Cancel after short time
    go func() {
        time.Sleep(50 * time.Millisecond)
        cancel()
    }()
    
    var results []int
    for result := range output {
        results = append(results, result)
    }
    
    // Should have processed some but not all items
    assert.Greater(t, len(results), 0)
    assert.Less(t, len(results), 10)
}
```

## Best Practices

1. **Choose appropriate concurrency** - Start with 2x CPU cores for CPU-bound, higher for I/O-bound
2. **Set operation timeouts** - Always use context.WithTimeout for external calls
3. **Handle errors gracefully** - Decide whether to skip, retry, or wrap errors
4. **Monitor performance** - Track processing time, error rates, and queue depth
5. **Test ordering** - Verify order preservation under various conditions
6. **Avoid shared state** - Keep operations independent to prevent race conditions
7. **Use resource pools** - For expensive resources like database connections
8. **Implement circuit breakers** - For external service resilience

## Related Processors

- **[Mapper](./mapper.md)**: Synchronous transformation alternative
- **[Batcher](./batcher.md)**: Group items before async processing
- **[Buffer](./buffer.md)**: Add buffering before async processing
- **[Monitor](./monitor.md)**: Observe async processing performance

## See Also

- **[Concepts: Processors](../concepts/processors.md)**: Understanding processor fundamentals
- **[Concepts: Error Handling](../concepts/error-handling.md)**: Advanced error strategies
- **[Guides: Performance](../guides/performance.md)**: Optimizing concurrent processing
- **[Guides: Patterns](../guides/patterns.md)**: Real-world async processing patterns