---
title: Retry
description: Automatic retry with configurable backoff
author: zoobzio
published: 2025-01-09
updated: 2025-01-09
tags:
  - reference
  - processors
  - resilience
  - error-handling
---

# Retry Processor

The Retry processor wraps any processor with automatic retry logic using exponential backoff with jitter. It provides resilient stream processing by handling transient failures gracefully while maintaining stream flow.

## Overview

The Retry processor is essential for building robust streaming pipelines that can handle temporary failures from external services, network issues, or resource constraints. It automatically retries failed operations while implementing best practices for backoff timing and error classification.

## Key Features

- **Exponential Backoff**: Configurable base delay with exponential growth
- **Jitter Support**: Optional randomization to prevent thundering herd problems  
- **Custom Error Classification**: Flexible error handling via callback functions
- **Context Awareness**: Respects cancellation during retry delays
- **Stream Preservation**: Failed items are skipped to maintain pipeline flow
- **Type Safety**: Full generic type support for any processor type

## Constructor

```go
func NewRetry[T any](processor Processor[T, T]) *Retry[T]
```

Creates a retry wrapper around any processor that transforms type T to T.

**Parameters:**
- `processor`: The processor to wrap with retry logic

**Returns:** A new Retry processor with fluent configuration methods.

**Default Configuration:**
- MaxAttempts: 3
- BaseDelay: 100ms  
- MaxDelay: 30s
- WithJitter: true
- Name: "retry"

## Fluent API Methods

### MaxAttempts(attempts int) *Retry[T]

Sets the maximum number of retry attempts including the initial attempt.

```go
retry := streamz.NewRetry(processor).MaxAttempts(5)
```

**Parameters:**
- `attempts`: Maximum attempts (minimum 1)

### BaseDelay(delay time.Duration) *Retry[T]

Sets the base delay for exponential backoff calculation.

```go
retry := streamz.NewRetry(processor).BaseDelay(200 * time.Millisecond)
```

The actual delay for attempt N is: `baseDelay * 2^(N-1)`

**Example delays with 100ms base:**
- Attempt 1: 0ms (no delay)
- Attempt 2: 100ms  
- Attempt 3: 200ms
- Attempt 4: 400ms

### MaxDelay(delay time.Duration) *Retry[T]

Sets the maximum delay between retry attempts to cap exponential growth.

```go
retry := streamz.NewRetry(processor).MaxDelay(10 * time.Second)
```

### WithJitter(enabled bool) *Retry[T]

Enables or disables jitter in retry delays.

```go
retry := streamz.NewRetry(processor).WithJitter(true)
```

When enabled, delays are randomized between 50% and 100% of the calculated delay to prevent synchronized retries across multiple instances.

### WithName(name string) *Retry[T]

Sets a custom name for the processor.

```go
retry := streamz.NewRetry(processor).WithName("api-retry")
```

### OnError(fn func(error, int) bool) *Retry[T]

Sets a custom error classification function.

```go
retry := streamz.NewRetry(processor).OnError(func(err error, attempt int) bool {
    // Only retry timeout errors
    return strings.Contains(err.Error(), "timeout")
})
```

**Parameters:**
- `fn`: Function that receives (error, attempt) and returns true to retry

## Usage Examples

### Basic Retry

```go
// Wrap an API processor with retry logic
apiProcessor := streamz.NewAsyncMapper(func(ctx context.Context, id string) (User, error) {
    return fetchUserFromAPI(ctx, id)
}).WithWorkers(5)

resilient := streamz.NewRetry(apiProcessor)

users := resilient.Process(ctx, userIDs)
for user := range users {
    fmt.Printf("User: %+v\n", user)
}
```

### Custom Configuration

```go
retry := streamz.NewRetry(processor).
    MaxAttempts(5).
    BaseDelay(200 * time.Millisecond).
    MaxDelay(10 * time.Second).
    WithJitter(true).
    WithName("database-retry")
```

### Error Classification

```go
retry := streamz.NewRetry(processor).OnError(func(err error, attempt int) bool {
    errStr := strings.ToLower(err.Error())
    
    // Don't retry authentication errors
    if strings.Contains(errStr, "unauthorized") {
        return false
    }
    
    // Stop after 2 attempts for rate limit errors
    if strings.Contains(errStr, "rate limit") && attempt >= 2 {
        return false
    }
    
    // Retry all other errors
    return true
})
```

### Database Operations

```go
dbProcessor := streamz.NewAsyncMapper(func(ctx context.Context, order Order) (Order, error) {
    err := saveOrderToDatabase(ctx, order)
    if err != nil {
        return Order{}, err
    }
    return order, nil
}).WithWorkers(10)

resilientDB := streamz.NewRetry(dbProcessor).
    MaxAttempts(3).
    BaseDelay(100 * time.Millisecond).
    WithName("db-save")

saved := resilientDB.Process(ctx, orders)
```

### API Rate Limiting

```go
apiProcessor := streamz.NewAsyncMapper(func(ctx context.Context, req APIRequest) (APIResponse, error) {
    return callExternalAPI(ctx, req)
}).WithWorkers(5)

rateLimitRetry := streamz.NewRetry(apiProcessor).
    MaxAttempts(5).
    BaseDelay(1 * time.Second).
    MaxDelay(30 * time.Second).
    OnError(func(err error, attempt int) bool {
        // Exponential backoff for rate limits
        return strings.Contains(err.Error(), "rate limit")
    })

responses := rateLimitRetry.Process(ctx, requests)
```

## Pipeline Integration

### With Error Monitoring

```go
pipeline := func(ctx context.Context, input <-chan Order) <-chan ProcessedOrder {
    validator := streamz.NewFilter(isValidOrder).WithName("validator")
    
    processor := streamz.NewAsyncMapper(func(ctx context.Context, order Order) (ProcessedOrder, error) {
        return processOrder(ctx, order)
    }).WithWorkers(10)
    
    retry := streamz.NewRetry(processor).
        MaxAttempts(3).
        BaseDelay(100 * time.Millisecond).
        WithName("order-processor")
    
    monitor := streamz.NewMonitor[ProcessedOrder](time.Minute).OnStats(func(stats streamz.StreamStats) {
        log.Info("Processing rate", "orders_per_min", stats.Rate*60)
    })
    
    validated := validator.Process(ctx, input)
    processed := retry.Process(ctx, validated)
    return monitor.Process(ctx, processed)
}
```

### Circuit Breaker Pattern

```go
var circuitOpen bool

retry := streamz.NewRetry(processor).
    MaxAttempts(3).
    OnError(func(err error, attempt int) bool {
        if circuitOpen {
            return false // Don't retry if circuit is open
        }
        
        // Open circuit after too many failures
        if strings.Contains(err.Error(), "service unavailable") && attempt >= 2 {
            circuitOpen = true
            return false
        }
        
        return true
    })
```

## Error Handling Behavior

### Built-in Error Classification

The retry processor includes intelligent error classification:

**Retryable Errors:**
- timeout
- connection refused/reset
- network errors
- temporary failures
- rate limit exceeded
- service unavailable

**Non-retryable Errors:**
- authentication failed
- unauthorized
- forbidden
- not found
- invalid/malformed requests

### Custom Error Handling

```go
retry := streamz.NewRetry(processor).OnError(func(err error, attempt int) bool {
    switch {
    case strings.Contains(err.Error(), "auth"):
        return false // Never retry auth errors
    case strings.Contains(err.Error(), "timeout"):
        return attempt <= 5 // Retry timeouts up to 5 times
    case strings.Contains(err.Error(), "rate"):
        return attempt <= 2 // Limited retries for rate limits
    default:
        return true // Retry other errors
    }
})
```

## Performance Characteristics

### Overhead

- **Successful Operations**: Minimal overhead (~3ms per item)
- **Failed Operations**: Exponential increase based on retry count
- **Memory Usage**: Small per-item overhead for tracking

### Benchmarks

```
BenchmarkRetrySuccessfulProcessing-12    450042    3080 ns/op    808 B/op    9 allocs/op
BenchmarkRetryWithFailures-12            1030      1142351 ns/op 2553 B/op   33 allocs/op
```

### Tuning Guidelines

**High Throughput Systems:**
```go
retry := streamz.NewRetry(processor).
    MaxAttempts(2).          // Fewer attempts
    BaseDelay(50*time.Millisecond). // Faster retry
    WithJitter(true)         // Prevent coordination
```

**Critical Systems:**
```go
retry := streamz.NewRetry(processor).
    MaxAttempts(5).          // More attempts
    BaseDelay(1*time.Second). // Longer backoff
    MaxDelay(60*time.Second) // Higher cap
```

## Best Practices

### 1. Match Retry Strategy to Failure Type

```go
// For transient network issues
networkRetry := streamz.NewRetry(processor).
    MaxAttempts(3).
    BaseDelay(100*time.Millisecond)

// For rate-limited APIs
rateLimitRetry := streamz.NewRetry(processor).
    MaxAttempts(5).
    BaseDelay(1*time.Second).
    MaxDelay(30*time.Second)
```

### 2. Use Jitter in Distributed Systems

```go
retry := streamz.NewRetry(processor).
    WithJitter(true) // Prevents thundering herd
```

### 3. Classify Errors Appropriately

```go
retry := streamz.NewRetry(processor).OnError(func(err error, attempt int) bool {
    // Be conservative: don't retry errors that won't succeed
    return isTransientError(err)
})
```

### 4. Monitor Retry Metrics

```go
var retryCount atomic.Int64

retry := streamz.NewRetry(processor).OnError(func(err error, attempt int) bool {
    if attempt > 1 {
        retryCount.Add(1)
        metrics.RetryAttempts.Inc()
    }
    return isRetryableError(err)
})
```

### 5. Combine with Circuit Breakers

```go
// Use with external circuit breaker libraries or custom logic
retry := streamz.NewRetry(processor).OnError(func(err error, attempt int) bool {
    if circuitBreaker.State() == "open" {
        return false
    }
    return shouldRetry(err, attempt)
})
```

## Common Patterns

### Database Resilience

```go
dbRetry := streamz.NewRetry(dbProcessor).
    MaxAttempts(3).
    BaseDelay(100*time.Millisecond).
    OnError(func(err error, attempt int) bool {
        return strings.Contains(err.Error(), "connection") ||
               strings.Contains(err.Error(), "timeout")
    })
```

### API Gateway

```go
apiRetry := streamz.NewRetry(apiProcessor).
    MaxAttempts(5).
    BaseDelay(200*time.Millisecond).
    MaxDelay(10*time.Second).
    WithJitter(true).
    OnError(isAPIRetryableError)
```

### Batch Processing

```go
batchRetry := streamz.NewRetry(batchProcessor).
    MaxAttempts(2).
    BaseDelay(1*time.Second).
    OnError(func(err error, attempt int) bool {
        // Don't retry data validation errors
        return !strings.Contains(err.Error(), "invalid")
    })
```

## See Also

- **[AsyncMapper](./async-mapper.md)**: For processors that benefit from retry
- **[Error Handling Guide](../concepts/error-handling.md)**: Comprehensive error strategies
- **[Best Practices](../guides/best-practices.md)**: Production retry patterns
- **[Performance Guide](../guides/performance.md)**: Optimizing retry overhead