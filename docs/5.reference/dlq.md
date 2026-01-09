---
title: Dead Letter Queue
description: Separate successful items from failures
author: zoobzio
published: 2025-01-09
updated: 2025-01-09
tags:
  - reference
  - processors
  - error-handling
---

# Dead Letter Queue Processor

The Dead Letter Queue (DLQ) processor captures failed items for later analysis or retry, preventing data loss due to processing failures.

## Overview

The DLQ processor wraps another processor and monitors its output. When items fail to process successfully, they are captured in a dead letter queue where they can be inspected, logged, stored, or retried manually. This pattern is essential for building resilient data pipelines that gracefully handle failures without losing data.

## Key Features

- **Automatic Retry**: Configure retry attempts before sending to DLQ
- **Error Classification**: Custom logic to determine which errors should be retried
- **Failure Callbacks**: React to failures with logging, alerting, or storage
- **Statistics Tracking**: Monitor success/failure rates
- **Flexible Error Handling**: Continue processing or halt on failures
- **Failed Item Access**: Access failed items through a dedicated channel

## Constructor

```go
func NewDeadLetterQueue[T any](processor Processor[T, T]) *DeadLetterQueue[T]
```

Creates a DLQ wrapper around any processor.

**Parameters:**
- `processor`: The processor to wrap with DLQ functionality

**Returns:** A new DeadLetterQueue with fluent configuration methods.

**Default Configuration:**
- MaxRetries: 0 (no retries)
- ContinueOnError: true
- FailedBufferSize: 100
- RetryDelay: 1 second
- Name: "dlq"

## Fluent API Methods

### MaxRetries(attempts int) *DeadLetterQueue[T]

Sets the maximum number of retry attempts before sending to DLQ.

```go
dlq := streamz.NewDeadLetterQueue(processor).MaxRetries(3)
```

**Parameters:**
- `attempts`: Number of retry attempts (0 = no retries)

### RetryDelay(delay time.Duration) *DeadLetterQueue[T]

Sets the delay between retry attempts.

```go
dlq := streamz.NewDeadLetterQueue(processor).
    MaxRetries(3).
    RetryDelay(500 * time.Millisecond)
```

### ContinueOnError(cont bool) *DeadLetterQueue[T]

Determines whether processing continues after failures.

```go
dlq := streamz.NewDeadLetterQueue(processor).
    ContinueOnError(false) // Stop on first failure
```

**Parameters:**
- `cont`: If false, pipeline stops on first failure

### WithFailedBufferSize(size int) *DeadLetterQueue[T]

Sets the buffer size for the failed items channel.

```go
dlq := streamz.NewDeadLetterQueue(processor).
    WithFailedBufferSize(1000) // Buffer up to 1000 failed items
```

### ShouldRetry(fn func(error) bool) *DeadLetterQueue[T]

Sets a function to determine if an error should be retried.

```go
dlq := streamz.NewDeadLetterQueue(processor).
    MaxRetries(5).
    ShouldRetry(func(err error) bool {
        // Only retry network errors
        return strings.Contains(err.Error(), "network") ||
               strings.Contains(err.Error(), "timeout")
    })
```

### OnFailure(handler DLQHandler[T]) *DeadLetterQueue[T]

Sets a callback for items sent to the DLQ.

```go
dlq := streamz.NewDeadLetterQueue(processor).
    OnFailure(func(ctx context.Context, item DLQItem[Order]) {
        log.Error("Order failed", 
            "id", item.Item.ID,
            "error", item.Error,
            "attempts", item.Attempts)
        // Store in database for manual review
        db.SaveFailedOrder(item)
    })
```

### OnRetry(fn func(item T, attempt int, err error)) *DeadLetterQueue[T]

Sets a callback invoked before each retry attempt.

```go
dlq := streamz.NewDeadLetterQueue(processor).
    MaxRetries(3).
    OnRetry(func(item Order, attempt int, err error) {
        log.Warn("Retrying order",
            "id", item.ID,
            "attempt", attempt,
            "error", err)
    })
```

### WithName(name string) *DeadLetterQueue[T]

Sets a custom name for the processor.

```go
dlq := streamz.NewDeadLetterQueue(processor).WithName("order-dlq")
```

## DLQItem Structure

Failed items are wrapped in a DLQItem structure:

```go
type DLQItem[T any] struct {
    Item      T                      // The original item that failed
    Error     error                  // The error that caused the failure
    Timestamp time.Time              // When the failure occurred
    Attempts  int                    // Number of processing attempts
    Metadata  map[string]interface{} // Additional context
}
```

## Usage Examples

### Basic DLQ with Retry

```go
// Processor that might fail
apiProcessor := streamz.NewAsyncMapper(func(ctx context.Context, order Order) (Order, error) {
    return callExternalAPI(ctx, order)
}).WithWorkers(10)

// Wrap with DLQ
dlq := streamz.NewDeadLetterQueue(apiProcessor).
    MaxRetries(3).
    RetryDelay(time.Second).
    OnFailure(func(ctx context.Context, item DLQItem[Order]) {
        log.Error("Order processing failed", 
            "order_id", item.Item.ID,
            "error", item.Error,
            "attempts", item.Attempts)
    })

// Process orders
protected := dlq.Process(ctx, orders)
```

### Smart Retry Logic

```go
// Custom retry logic based on error type
dlq := streamz.NewDeadLetterQueue(processor).
    MaxRetries(5).
    RetryDelay(100 * time.Millisecond).
    ShouldRetry(func(err error) bool {
        if err == nil {
            return false
        }
        
        errStr := strings.ToLower(err.Error())
        
        // Don't retry validation errors
        if strings.Contains(errStr, "invalid") ||
           strings.Contains(errStr, "validation") {
            return false
        }
        
        // Don't retry auth errors
        if strings.Contains(errStr, "unauthorized") ||
           strings.Contains(errStr, "forbidden") {
            return false
        }
        
        // Retry everything else (network, timeout, etc)
        return true
    })
```

### Failed Item Storage

```go
// Store failed items for manual processing
dlq := streamz.NewDeadLetterQueue(processor).
    MaxRetries(2).
    OnFailure(func(ctx context.Context, item DLQItem[Transaction]) {
        // Log failure
        log.Error("Transaction failed",
            "tx_id", item.Item.ID,
            "amount", item.Item.Amount,
            "error", item.Error)
        
        // Store in database
        err := db.Exec(`
            INSERT INTO failed_transactions 
            (tx_id, data, error, timestamp, attempts)
            VALUES (?, ?, ?, ?, ?)`,
            item.Item.ID,
            json.Marshal(item.Item),
            item.Error.Error(),
            item.Timestamp,
            item.Attempts,
        )
        
        if err != nil {
            log.Error("Failed to store DLQ item", "error", err)
        }
        
        // Send alert for high-value failures
        if item.Item.Amount > 10000 {
            alerting.SendHighPriority("High-value transaction failed", item)
        }
    })
```

### Accessing Failed Items Channel

```go
dlq := streamz.NewDeadLetterQueue(processor).
    MaxRetries(1).
    WithFailedBufferSize(500)

// Process in main pipeline
output := dlq.Process(ctx, input)

// Handle failed items separately
go func() {
    for failed := range dlq.FailedItems() {
        // Batch failed items for bulk processing
        failedBatch = append(failedBatch, failed)
        
        if len(failedBatch) >= 100 {
            processFai ledBatch(failedBatch)
            failedBatch = nil
        }
    }
}()
```

### Stop on Critical Failures

```go
// Stop processing on critical failures
dlq := streamz.NewDeadLetterQueue(processor).
    ContinueOnError(false). // Stop on any failure
    OnFailure(func(ctx context.Context, item DLQItem[CriticalData]) {
        log.Fatal("Critical processing failure",
            "item", item.Item,
            "error", item.Error)
    })
```

## Pipeline Integration

### With Multiple Processors

```go
func buildResilientPipeline(ctx context.Context, input <-chan Order) <-chan ProcessedOrder {
    // Validation with DLQ
    validator := streamz.NewFilter(isValidOrder)
    validationDLQ := streamz.NewDeadLetterQueue(validator).
        ContinueOnError(true).
        OnFailure(logValidationFailure)
    
    // Enrichment with retry
    enricher := streamz.NewAsyncMapper(enrichOrder).WithWorkers(10)
    enrichmentDLQ := streamz.NewDeadLetterQueue(enricher).
        MaxRetries(3).
        RetryDelay(time.Second).
        OnFailure(logEnrichmentFailure)
    
    // Processing with smart retry
    processor := streamz.NewAsyncMapper(processOrder).WithWorkers(20)
    processingDLQ := streamz.NewDeadLetterQueue(processor).
        MaxRetries(5).
        ShouldRetry(isRetryableError).
        OnFailure(storeFailedOrder)
    
    // Chain processors
    validated := validationDLQ.Process(ctx, input)
    enriched := enrichmentDLQ.Process(ctx, validated)
    return processingDLQ.Process(ctx, enriched)
}
```

### Combined with Circuit Breaker

```go
// Protect external service with circuit breaker and DLQ
apiProcessor := streamz.NewAsyncMapper(callPaymentAPI).WithWorkers(5)

// Add circuit breaker
protected := streamz.NewCircuitBreaker(apiProcessor).
    FailureThreshold(0.5).
    RecoveryTimeout(30 * time.Second)

// Add DLQ for permanent failures
resilient := streamz.NewDeadLetterQueue(protected).
    MaxRetries(3).
    ShouldRetry(func(err error) bool {
        // Don't retry if circuit is open
        if protected.GetState() == streamz.StateOpen {
            return false
        }
        return isRetryableError(err)
    }).
    OnFailure(func(ctx context.Context, item DLQItem[Payment]) {
        // Store for manual retry when service recovers
        storeForManualRetry(item)
    })
```

## Monitoring and Statistics

### Getting Statistics

```go
stats := dlq.GetStats()

fmt.Printf("DLQ Statistics:\n")
fmt.Printf("  Total Processed: %d\n", stats.Processed)
fmt.Printf("  Succeeded: %d (%.2f%%)\n", 
    stats.Succeeded, stats.SuccessRate())
fmt.Printf("  Failed: %d (%.2f%%)\n", 
    stats.Failed, stats.FailureRate())
fmt.Printf("  Total Retries: %d\n", stats.Retried)

// Alert on high failure rate
if stats.FailureRate() > 10.0 {
    alert.Send("High DLQ failure rate", stats)
}
```

### Metrics Integration

```go
dlq := streamz.NewDeadLetterQueue(processor).
    OnFailure(func(ctx context.Context, item DLQItem[T]) {
        // Update metrics
        metrics.DLQFailures.Inc()
        metrics.DLQFailuresByError.WithLabelValues(
            classifyError(item.Error)).Inc()
    }).
    OnRetry(func(item T, attempt int, err error) {
        metrics.DLQRetries.Inc()
        metrics.DLQRetryAttempts.Observe(float64(attempt))
    })

// Periodic stats export
go func() {
    ticker := time.NewTicker(time.Minute)
    defer ticker.Stop()
    
    for range ticker.C {
        stats := dlq.GetStats()
        metrics.DLQSuccessRate.Set(stats.SuccessRate())
        metrics.DLQFailureRate.Set(stats.FailureRate())
    }
}()
```

## Performance Characteristics

### Overhead

- **No Failures**: ~100ns additional overhead per item
- **With Retries**: Depends on retry delay and number of attempts
- **Failed Item Channel**: Minimal overhead, buffered channel operations

### Benchmarks

```
BenchmarkDLQNoFailures-12     349968    5024 ns/op     808 B/op     9 allocs/op
BenchmarkDLQWithRetries-12    140886    7597 ns/op    1393 B/op    17 allocs/op
```

### Memory Usage

- Fixed overhead per DLQ instance: ~500 bytes
- Failed items buffer: `BufferSize * sizeof(DLQItem[T])`
- No additional allocations for successful items

## Best Practices

### 1. Choose Appropriate Retry Strategies

```go
// For transient network issues
dlq.MaxRetries(5).
    RetryDelay(100 * time.Millisecond).
    ShouldRetry(isNetworkError)

// For rate-limited APIs
dlq.MaxRetries(3).
    RetryDelay(time.Second * 5). // Longer delay
    ShouldRetry(isRateLimitError)

// For data validation errors
dlq.MaxRetries(0). // No retries for permanent errors
    OnFailure(logAndStore)
```

### 2. Monitor Failed Items

```go
// Don't let failed items accumulate unnoticed
go func() {
    threshold := 100
    count := 0
    
    for failed := range dlq.FailedItems() {
        count++
        handleFailedItem(failed)
        
        if count >= threshold {
            alert.Send("High DLQ volume", count)
            count = 0
        }
    }
}()
```

### 3. Use Structured Logging

```go
dlq.OnFailure(func(ctx context.Context, item DLQItem[T]) {
    log.WithFields(log.Fields{
        "item_id": item.Item.ID,
        "error": item.Error.Error(),
        "error_type": classifyError(item.Error),
        "attempts": item.Attempts,
        "timestamp": item.Timestamp,
        "processor": dlq.Name(),
    }).Error("Item sent to DLQ")
})
```

### 4. Implement Manual Retry Mechanisms

```go
// Endpoint to retry failed items
func retryDLQItems(w http.ResponseWriter, r *http.Request) {
    items := loadFailedItemsFromDB()
    
    for _, item := range items {
        select {
        case retryChannel <- item:
            markAsRetrying(item.ID)
        default:
            // Retry queue full
            http.Error(w, "Retry queue full", 503)
            return
        }
    }
    
    fmt.Fprintf(w, "Queued %d items for retry", len(items))
}
```

### 5. Set Appropriate Buffer Sizes

```go
// High-volume pipeline
dlq.WithFailedBufferSize(10000)

// Low-volume, critical pipeline
dlq.WithFailedBufferSize(100).
    ContinueOnError(false) // Stop on failures
```

## Common Patterns

### Dead Letter Queue Chain

```go
// Primary processor → Fallback processor → DLQ
primary := streamz.NewAsyncMapper(primaryAPI)
fallback := streamz.NewAsyncMapper(fallbackAPI)

// Try primary, then fallback, then DLQ
dlqWithFallback := streamz.NewDeadLetterQueue(primary).
    MaxRetries(2).
    OnFailure(func(ctx context.Context, item DLQItem[T]) {
        // Try fallback processor
        result, err := fallback.ProcessSingle(ctx, item.Item)
        if err != nil {
            // Both failed - store in permanent DLQ
            storePermanentFailure(item)
        } else {
            // Fallback succeeded
            sendToOutput(result)
        }
    })
```

### Scheduled Retry

```go
// Retry failed items on a schedule
func scheduleDLQRetries(dlq *DeadLetterQueue[Order]) {
    ticker := time.NewTicker(time.Hour)
    defer ticker.Stop()
    
    for range ticker.C {
        failedOrders := loadRecentFailures()
        
        for _, order := range failedOrders {
            if shouldRetry(order) {
                retryQueue <- order.Item
                markAsRetried(order.ID)
            }
        }
    }
}
```

## See Also

- **[Retry Processor](./retry.md)**: Built-in retry functionality
- **[Circuit Breaker](./circuit-breaker.md)**: Prevent cascading failures
- **[Error Handling Guide](../concepts/error-handling.md)**: Comprehensive error strategies
- **[Best Practices](../guides/best-practices.md)**: Production patterns