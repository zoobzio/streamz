# Error Handling in Streaming Systems

Learn how to build resilient streaming pipelines that gracefully handle failures, network issues, and unexpected conditions.

## streamz Error Philosophy

streamz follows a **graceful degradation** approach to error handling:

1. **Skip problematic items** rather than failing the entire stream
2. **Log errors** for debugging but don't break the pipeline
3. **Continue processing** remaining items
4. **Preserve pipeline stability** over individual item success

This approach keeps streaming systems running even when individual operations fail.

## Default Error Behavior

### Skip-on-Error Pattern

Most streamz processors use the "skip-on-error" pattern:

```go
// AsyncMapper skips items that fail processing
asyncMapper := streamz.NewAsyncMapper(func(ctx context.Context, order Order) (Order, error) {
    processed, err := expensiveOperation(order)
    if err != nil {
        // Error is logged internally, item is skipped
        log.Error("Processing failed", "order", order.ID, "error", err)
        return order, err // Item will be skipped
    }
    return processed, nil
}).WithWorkers(10)

// Input: [order1, order2, order3]
// If order2 fails: Output: [processed_order1, processed_order3]
// order2 is skipped, pipeline continues
```

### Filter Behavior

Filters naturally handle "errors" by excluding items:

```go
validator := streamz.NewFilter(func(order Order) bool {
    // Invalid orders are naturally filtered out
    return order.Amount > 0 && order.CustomerID != ""
}).WithName("valid-orders")

// Input: [validOrder, invalidOrder, validOrder]
// Output: [validOrder, validOrder]
// Invalid items are filtered out, no error handling needed
```

## Custom Error Handling Strategies

### 1. Error Enrichment

**Add error context** to items for downstream handling:

```go
type ProcessingResult[T any] struct {
    Item   T
    Error  error
    Retry  bool
}

func enrichWithErrors[T any](processor func(context.Context, T) (T, error)) streamz.Processor[T, ProcessingResult[T]] {
    return streamz.NewMapper(func(item T) ProcessingResult[T] {
        result, err := processor(context.Background(), item)
        return ProcessingResult[T]{
            Item:  result,
            Error: err,
            Retry: isRetryableError(err),
        }
    }).WithName("error-enricher")
}

// Usage
enricher := enrichWithErrors(riskyOperation)
results := enricher.Process(ctx, orders)

for result := range results {
    if result.Error != nil {
        if result.Retry {
            // Send to retry queue
            retryQueue <- result.Item
        } else {
            // Send to dead letter queue
            dlq <- FailedItem{Item: result.Item, Error: result.Error}
        }
    } else {
        // Process successful result
        processSuccessful(result.Item)
    }
}
```

### 2. Dead Letter Queue Pattern

**Capture failed items** for later analysis:

```go
type DLQProcessor[T any] struct {
    name      string
    processor func(context.Context, T) (T, error)
    dlq       chan<- FailedItem[T]
}

type FailedItem[T any] struct {
    Item      T
    Error     error
    Timestamp time.Time
    Attempts  int
}

func (d *DLQProcessor[T]) Process(ctx context.Context, in <-chan T) <-chan T {
    out := make(chan T)
    
    go func() {
        defer close(out)
        
        for {
            select {
            case item, ok := <-in:
                if !ok {
                    return
                }
                
                result, err := d.processor(ctx, item)
                if err != nil {
                    // Send to DLQ
                    select {
                    case d.dlq <- FailedItem[T]{
                        Item:      item,
                        Error:     err,
                        Timestamp: time.Now(),
                        Attempts:  1,
                    }:
                    case <-ctx.Done():
                        return
                    }
                } else {
                    // Send successful result
                    select {
                    case out <- result:
                    case <-ctx.Done():
                        return
                    }
                }
                
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return out
}

// Usage
dlq := make(chan FailedItem[Order], 1000)
processor := &DLQProcessor[Order]{
    name:      "order-processor",
    processor: processOrder,
    dlq:       dlq,
}

// Process DLQ items separately
go func() {
    for failed := range dlq {
        log.Error("Item failed processing", 
            "item", failed.Item,
            "error", failed.Error,
            "timestamp", failed.Timestamp)
        
        // Optionally retry or store for manual inspection
        if isRetryableError(failed.Error) {
            retryLater(failed.Item)
        }
    }
}()

processed := processor.Process(ctx, orders)
```

### 3. Retry with Exponential Backoff

**Automatically retry** failed operations:

```go
type RetryProcessor[T any] struct {
    name       string
    processor  func(context.Context, T) (T, error)
    maxRetries int
    baseDelay  time.Duration
    maxDelay   time.Duration
}

func (r *RetryProcessor[T]) Process(ctx context.Context, in <-chan T) <-chan T {
    out := make(chan T)
    
    go func() {
        defer close(out)
        
        for {
            select {
            case item, ok := <-in:
                if !ok {
                    return
                }
                
                result, err := r.processWithRetry(ctx, item)
                if err == nil {
                    select {
                    case out <- result:
                    case <-ctx.Done():
                        return
                    }
                }
                // If all retries failed, item is dropped
                
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return out
}

func (r *RetryProcessor[T]) processWithRetry(ctx context.Context, item T) (T, error) {
    var lastErr error
    
    for attempt := 0; attempt <= r.maxRetries; attempt++ {
        if attempt > 0 {
            // Exponential backoff with jitter
            delay := time.Duration(math.Pow(2, float64(attempt-1))) * r.baseDelay
            if delay > r.maxDelay {
                delay = r.maxDelay
            }
            
            // Add jitter to prevent thundering herd
            jitter := time.Duration(rand.Float64() * float64(delay) * 0.1)
            delay += jitter
            
            select {
            case <-time.After(delay):
            case <-ctx.Done():
                return item, ctx.Err()
            }
        }
        
        result, err := r.processor(ctx, item)
        if err == nil {
            if attempt > 0 {
                log.Info("Retry succeeded", "attempt", attempt, "item", item)
            }
            return result, nil
        }
        
        lastErr = err
        
        // Don't retry non-retryable errors
        if !isRetryableError(err) {
            log.Error("Non-retryable error", "error", err, "item", item)
            break
        }
        
        log.Warn("Retry attempt failed", "attempt", attempt, "error", err)
    }
    
    log.Error("All retry attempts failed", "max_retries", r.maxRetries, "error", lastErr)
    return item, fmt.Errorf("failed after %d retries: %w", r.maxRetries, lastErr)
}

func isRetryableError(err error) bool {
    // Network errors, timeouts, rate limits are typically retryable
    if err == nil {
        return false
    }
    
    errStr := err.Error()
    retryablePatterns := []string{
        "timeout",
        "connection refused",
        "rate limit",
        "service unavailable",
        "internal server error",
    }
    
    for _, pattern := range retryablePatterns {
        if strings.Contains(strings.ToLower(errStr), pattern) {
            return true
        }
    }
    
    return false
}
```

### 4. Circuit Breaker Pattern

**Fail fast** when downstream services are unavailable:

```go
type CircuitBreaker[T any] struct {
    name         string
    maxFailures  int
    resetTimeout time.Duration
    
    // State
    failures     int64
    requests     int64
    lastFailure  time.Time
    state        int32 // 0=closed, 1=open, 2=half-open
    mutex        sync.RWMutex
}

const (
    CircuitClosed = iota
    CircuitOpen
    CircuitHalfOpen
)

func NewCircuitBreaker[T any](name string, maxFailures int, resetTimeout time.Duration) *CircuitBreaker[T] {
    return &CircuitBreaker[T]{
        name:         name,
        maxFailures:  maxFailures,
        resetTimeout: resetTimeout,
    }
}

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
                    log.Warn("Circuit breaker open, rejecting item", "circuit", cb.name)
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
    cb.mutex.Lock()
    defer cb.mutex.Unlock()
    
    state := atomic.LoadInt32(&cb.state)
    
    switch state {
    case CircuitClosed:
        return false
    case CircuitOpen:
        if time.Since(cb.lastFailure) > cb.resetTimeout {
            // Try half-open
            atomic.StoreInt32(&cb.state, CircuitHalfOpen)
            log.Info("Circuit breaker half-open", "circuit", cb.name)
            return false
        }
        return true
    case CircuitHalfOpen:
        // Allow one request through
        return false
    default:
        return false
    }
}

func (cb *CircuitBreaker[T]) recordSuccess() {
    atomic.AddInt64(&cb.requests, 1)
    
    state := atomic.LoadInt32(&cb.state)
    if state == CircuitHalfOpen {
        // Success in half-open state, close circuit
        atomic.StoreInt32(&cb.state, CircuitClosed)
        atomic.StoreInt64(&cb.failures, 0)
        log.Info("Circuit breaker closed", "circuit", cb.name)
    }
}

func (cb *CircuitBreaker[T]) recordFailure() {
    atomic.AddInt64(&cb.requests, 1)
    failures := atomic.AddInt64(&cb.failures, 1)
    
    cb.mutex.Lock()
    cb.lastFailure = time.Now()
    cb.mutex.Unlock()
    
    if failures >= int64(cb.maxFailures) {
        // Open circuit
        atomic.StoreInt32(&cb.state, CircuitOpen)
        log.Warn("Circuit breaker opened", "circuit", cb.name, "failures", failures)
    }
}
```

## Error Aggregation and Reporting

### Batch Error Handling

**Handle errors in batch operations**:

```go
type BatchResult[T any] struct {
    Successful []T
    Failed     []FailedItem[T]
}

func batchProcessorWithErrors[T any](
    batchProcessor func(context.Context, []T) ([]T, []error),
) streamz.Processor[[]T, BatchResult[T]] {
    return streamz.NewMapper(func(batch []T) BatchResult[T] {
        results, errors := batchProcessor(context.Background(), batch)
        
        var successful []T
        var failed []FailedItem[T]
        
        for i, item := range batch {
            if i < len(errors) && errors[i] != nil {
                failed = append(failed, FailedItem[T]{
                    Item:      item,
                    Error:     errors[i],
                    Timestamp: time.Now(),
                })
            } else if i < len(results) {
                successful = append(successful, results[i])
            }
        }
        
        return BatchResult[T]{
            Successful: successful,
            Failed:     failed,
        }
    }).WithName("batch-with-errors")
}

// Usage
batcher := streamz.NewBatcher[Order](batchConfig)
processor := batchProcessorWithErrors(processBatchOfOrders)

batched := batcher.Process(ctx, orders)
results := processor.Process(ctx, batched)

for result := range results {
    // Process successful items
    for _, order := range result.Successful {
        handleSuccessfulOrder(order)
    }
    
    // Handle failed items
    for _, failed := range result.Failed {
        log.Error("Order failed", "order", failed.Item.ID, "error", failed.Error)
        dlq <- failed
    }
}
```

### Error Rate Monitoring

**Monitor and alert on error rates**:

```go
type ErrorRateMonitor[T any] struct {
    name           string
    windowSize     time.Duration
    errorThreshold float64
    
    totalCount  int64
    errorCount  int64
    windowStart time.Time
    mutex       sync.RWMutex
}

func NewErrorRateMonitor[T any](name string, windowSize time.Duration, errorThreshold float64) *ErrorRateMonitor[T] {
    return &ErrorRateMonitor[T]{
        name:           name,
        windowSize:     windowSize,
        errorThreshold: errorThreshold,
        windowStart:    time.Now(),
    }
}

func (e *ErrorRateMonitor[T]) Process(ctx context.Context, in <-chan ProcessingResult[T]) <-chan T {
    out := make(chan T)
    
    go func() {
        defer close(out)
        
        for {
            select {
            case result, ok := <-in:
                if !ok {
                    return
                }
                
                e.recordResult(result.Error != nil)
                
                if result.Error == nil {
                    select {
                    case out <- result.Item:
                    case <-ctx.Done():
                        return
                    }
                }
                
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return out
}

func (e *ErrorRateMonitor[T]) recordResult(isError bool) {
    e.mutex.Lock()
    defer e.mutex.Unlock()
    
    now := time.Now()
    
    // Reset window if needed
    if now.Sub(e.windowStart) > e.windowSize {
        e.totalCount = 0
        e.errorCount = 0
        e.windowStart = now
    }
    
    e.totalCount++
    if isError {
        e.errorCount++
    }
    
    // Check threshold
    if e.totalCount > 0 {
        errorRate := float64(e.errorCount) / float64(e.totalCount)
        if errorRate > e.errorThreshold {
            log.Warn("High error rate detected",
                "monitor", e.name,
                "error_rate", errorRate,
                "threshold", e.errorThreshold,
                "total_count", e.totalCount,
                "error_count", e.errorCount)
        }
    }
}
```

## Composite Error Handling

**Combine multiple error handling strategies**:

```go
func buildResilientPipeline(ctx context.Context, orders <-chan Order) (<-chan ProcessedOrder, <-chan FailedItem[Order]) {
    dlq := make(chan FailedItem[Order], 1000)
    
    // Step 1: Circuit breaker for external service protection
    circuitBreaker := NewCircuitBreaker[Order]("payment-service", 10, time.Minute)
    
    // Step 2: Retry for transient failures
    retryProcessor := &RetryProcessor[Order]{
        name:       "order-processor",
        processor:  processOrderWithExternalCalls,
        maxRetries: 3,
        baseDelay:  100 * time.Millisecond,
        maxDelay:   5 * time.Second,
    }
    
    // Step 3: DLQ for permanent failures
    dlqProcessor := &DLQProcessor[Order]{
        name:      "final-processor",
        processor: finalProcessing,
        dlq:       dlq,
    }
    
    // Step 4: Error rate monitoring
    errorMonitor := NewErrorRateMonitor[Order]("order-pipeline", time.Minute, 0.1) // 10% threshold
    
    // Compose the resilient pipeline
    protected := circuitBreaker.Process(ctx, orders)
    retried := retryProcessor.Process(ctx, protected)
    processed := dlqProcessor.Process(ctx, retried)
    
    return processed, dlq
}

// Usage
processed, failedItems := buildResilientPipeline(ctx, orders)

// Handle successful processing
go func() {
    for order := range processed {
        log.Info("Order processed successfully", "order", order.ID)
    }
}()

// Handle failed items
go func() {
    for failed := range failedItems {
        log.Error("Order permanently failed", 
            "order", failed.Item.ID,
            "error", failed.Error,
            "timestamp", failed.Timestamp)
        
        // Store in database for manual review
        storeFailedOrder(failed)
    }
}()
```

## Testing Error Scenarios

### Simulating Failures

```go
func TestErrorHandling(t *testing.T) {
    ctx := context.Background()
    
    // Create a processor that fails predictably
    failingProcessor := streamz.NewAsyncMapper(func(ctx context.Context, order Order) (Order, error) {
        if order.ID == "fail-me" {
            return order, errors.New("simulated failure")
        }
        return order, nil
    }).WithWorkers(1)
    
    input := make(chan Order)
    output := failingProcessor.Process(ctx, input)
    
    // Send test data including items that will fail
    go func() {
        defer close(input)
        input <- Order{ID: "success-1"}
        input <- Order{ID: "fail-me"}
        input <- Order{ID: "success-2"}
    }()
    
    // Collect results
    var results []Order
    for result := range output {
        results = append(results, result)
    }
    
    // Verify: Only successful items should be output
    assert.Len(t, results, 2)
    assert.Equal(t, "success-1", results[0].ID)
    assert.Equal(t, "success-2", results[1].ID)
}
```

### Load Testing with Errors

```go
func TestErrorRateUnderLoad(t *testing.T) {
    ctx := context.Background()
    
    errorRate := 0.1 // 10% error rate
    totalItems := 10000
    
    processor := streamz.NewAsyncMapper(func(ctx context.Context, item int) (int, error) {
        if rand.Float64() < errorRate {
            return item, fmt.Errorf("random failure for item %d", item)
        }
        return item * 2, nil
    }).WithWorkers(10)
    
    input := make(chan int)
    output := processor.Process(ctx, input)
    
    // Generate high-volume input with expected error rate
    go func() {
        defer close(input)
        for i := 0; i < totalItems; i++ {
            input <- i
        }
    }()
    
    // Count successful outputs
    successCount := 0
    for range output {
        successCount++
    }
    
    // Verify approximately 90% success rate
    expectedSuccess := int(float64(totalItems) * (1 - errorRate))
    tolerance := int(float64(totalItems) * 0.05) // 5% tolerance
    
    assert.InDelta(t, expectedSuccess, successCount, float64(tolerance))
}
```

## Best Practices

### 1. **Choose Appropriate Error Strategy**
```go
// ✅ Transient failures: Use retry
retryProcessor := &RetryProcessor[Order]{maxRetries: 3}

// ✅ External service issues: Use circuit breaker  
circuitBreaker := NewCircuitBreaker[Order]("external-api", 10, time.Minute)

// ✅ Data quality issues: Use filtering
validator := streamz.NewFilter(isValid).WithName("valid")

// ✅ Critical failures: Use DLQ
dlqProcessor := &DLQProcessor[Order]{dlq: deadLetterQueue}
```

### 2. **Monitor Error Rates**
```go
monitor := streamz.NewMonitor[ProcessingResult[Order]](time.Minute).OnStats(func(stats streamz.StreamStats) {
    errorRate := calculateErrorRate(stats)
    if errorRate > 0.05 { // 5% threshold
        alerting.SendAlert("High error rate", errorRate)
    }
})
```

### 3. **Log Contextual Information**
```go
log.Error("Processing failed",
    "item_id", item.ID,
    "error", err,
    "processor", "order-enricher",
    "attempt", attempt,
    "timestamp", time.Now(),
    "context", additionalContext)
```

### 4. **Test Error Scenarios**
- Simulate network failures
- Test with malformed data
- Verify DLQ functionality
- Test circuit breaker behavior
- Load test with error injection

### 5. **Design for Observability**
- Track error rates by type
- Monitor retry success rates
- Alert on circuit breaker state changes
- Dashboard DLQ growth

## Common Anti-Patterns

### ❌ **Ignoring Errors**
```go
// DON'T: Silent failures make debugging impossible
result, _ := processor(item) // Ignoring error
```

### ❌ **Failing Fast on Recoverable Errors**
```go
// DON'T: Stopping pipeline for transient issues
if err != nil {
    panic(err) // Too aggressive
}
```

### ❌ **No Error Classification**
```go
// DON'T: Treating all errors the same
if err != nil {
    return err // Should classify: retryable vs permanent
}
```

## What's Next?

- **[Performance Guide](../guides/performance.md)**: Optimize error handling performance
- **[Testing Guide](../guides/testing.md)**: Test error scenarios thoroughly
- **[Best Practices](../guides/best-practices.md)**: Production error handling patterns
- **[API Reference: AsyncMapper](../api/async-mapper.md)**: Error handling in concurrent processing