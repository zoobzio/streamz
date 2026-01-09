---
title: Switch
description: Route items by predicate to different processors
author: zoobzio
published: 2025-01-09
updated: 2025-01-09
tags:
  - reference
  - processors
  - routing
---

# Switch Processor

The Switch processor routes items to different processors based on conditions, similar to a switch statement in programming languages.

## Overview

The Switch processor evaluates conditions in order and processes items with the first matching case. It provides:

- Multiple conditional branches
- Default case for unmatched items
- Early termination on first match
- Named cases for monitoring
- Type-safe condition evaluation

## Key Features

- **Sequential Evaluation**: Cases are evaluated in the order they are added
- **First Match Wins**: Items are processed by the first matching case only
- **Default Handling**: Optional default processor for unmatched items
- **Named Cases**: Each case can have a name for monitoring and debugging
- **Statistics Tracking**: Track match counts for each case
- **Parallel Processing**: Each case processor runs independently

## Constructor

```go
func NewSwitch[T any]() *Switch[T]
```

Creates a new switch processor for routing items based on conditions.

**Returns:** A new Switch with fluent configuration methods.

**Default Configuration:**
- No cases (must add at least one case or default)
- Buffer size: 1
- Name: "switch"

## Fluent API Methods

### Case(name string, condition func(T) bool, processor Processor[T, T]) *Switch[T]

Adds a new case to the switch. Cases are evaluated in the order they are added.

```go
sw := streamz.NewSwitch[Order]().
    Case("urgent", func(o Order) bool { 
        return o.Priority == "urgent" 
    }, urgentProcessor)
```

**Parameters:**
- `name`: Name for the case (used in statistics)
- `condition`: Function that returns true if item matches this case
- `processor`: Processor to handle matching items

### Default(processor Processor[T, T]) *Switch[T]

Sets the processor for items that don't match any case.

```go
sw := streamz.NewSwitch[int]().
    Case("even", isEven, evenProcessor).
    Default(defaultProcessor) // Handle odd numbers
```

**Parameters:**
- `processor`: Processor to handle unmatched items

**Note:** If no default is set, unmatched items are dropped.

### WithBufferSize(size int) *Switch[T]

Sets the buffer size for internal channels.

```go
sw := streamz.NewSwitch[Order]().
    WithBufferSize(100) // Buffer for speed differences
```

**Parameters:**
- `size`: Buffer size (0 for unbuffered)

### WithName(name string) *Switch[T]

Sets a custom name for the processor.

```go
sw := streamz.NewSwitch[Order]().WithName("order-router")
```

### GetStats() SwitchStats

Returns statistics about case matches.

```go
stats := sw.GetStats()
fmt.Printf("Urgent orders: %d (%.1f%%)\n", 
    stats.CaseMatches["urgent"],
    stats.MatchRate("urgent"))
```

## Usage Examples

### Basic Routing

```go
// Route numbers by size
sw := streamz.NewSwitch[int]().
    Case("small", func(n int) bool { return n < 10 }, smallProcessor).
    Case("medium", func(n int) bool { return n < 100 }, mediumProcessor).
    Case("large", func(n int) bool { return n < 1000 }, largeProcessor).
    Default(veryLargeProcessor)

output := sw.Process(ctx, numbers)
```

### Order Processing Pipeline

```go
type Order struct {
    ID       string
    Amount   float64
    Priority string
    Country  string
}

// Different processors for order types
urgentProc := streamz.NewAsyncMapper(processUrgentOrder).WithWorkers(20)
highValueProc := streamz.NewAsyncMapper(processHighValueOrder).WithWorkers(10)
intlProc := streamz.NewAsyncMapper(processInternationalOrder).WithWorkers(5)
standardProc := streamz.NewAsyncMapper(processStandardOrder).WithWorkers(3)

// Route orders based on characteristics
orderRouter := streamz.NewSwitch[Order]().
    Case("urgent", func(o Order) bool {
        return o.Priority == "urgent" || o.Amount > 10000
    }, urgentProc).
    Case("high-value", func(o Order) bool {
        return o.Amount > 5000
    }, highValueProc).
    Case("international", func(o Order) bool {
        return o.Country != "US"
    }, intlProc).
    Default(standardProc).
    WithName("order-router")

// Process orders
routed := orderRouter.Process(ctx, orders)
```

### Event Type Routing

```go
type Event struct {
    Type    string
    Level   string
    Source  string
    Message string
}

// Route events to specialized handlers
eventSwitch := streamz.NewSwitch[Event]().
    Case("critical", func(e Event) bool {
        return e.Level == "CRITICAL" || e.Level == "FATAL"
    }, criticalHandler).
    Case("error", func(e Event) bool {
        return e.Level == "ERROR"
    }, errorHandler).
    Case("security", func(e Event) bool {
        return strings.Contains(e.Type, "security") ||
               strings.Contains(e.Message, "unauthorized")
    }, securityHandler).
    Case("performance", func(e Event) bool {
        return e.Type == "performance" || e.Type == "metric"
    }, performanceHandler).
    Default(generalHandler)

// Process event stream
processed := eventSwitch.Process(ctx, events)
```

### Complex Routing Logic

```go
// Multi-criteria routing
dataRouter := streamz.NewSwitch[DataPacket]().
    Case("realtime", func(p DataPacket) bool {
        // Real-time data needs immediate processing
        return p.Priority == "realtime" || 
               (p.Timestamp.Sub(time.Now()) < time.Second)
    }, realtimeProcessor).
    Case("batch-ready", func(p DataPacket) bool {
        // Batch when we have enough data or deadline approaching
        return p.Size > 1000 || p.Deadline.Before(time.Now().Add(time.Minute))
    }, batchProcessor).
    Case("archive", func(p DataPacket) bool {
        // Old data goes to archive
        return p.Timestamp.Before(time.Now().Add(-24 * time.Hour))
    }, archiveProcessor).
    Default(standardProcessor)
```

### With Monitoring

```go
// Monitor routing decisions
sw := streamz.NewSwitch[Transaction]().
    Case("fraud-check", isSuspicious, fraudProcessor).
    Case("high-risk", isHighRisk, riskProcessor).
    Case("standard", func(t Transaction) bool { return true }, standardProcessor)

// Process with monitoring
go func() {
    ticker := time.NewTicker(time.Minute)
    defer ticker.Stop()
    
    for range ticker.C {
        stats := sw.GetStats()
        log.Info("Routing stats",
            "total", stats.TotalItems,
            "fraud", stats.CaseMatches["fraud-check"],
            "high-risk", stats.CaseMatches["high-risk"],
            "standard", stats.CaseMatches["standard"])
        
        // Alert on high fraud rate
        if stats.MatchRate("fraud-check") > 5.0 {
            alerting.Send("High fraud rate detected", stats)
        }
    }
}()

output := sw.Process(ctx, transactions)
```

## Important Considerations

### 1. Order Matters

The first matching case wins:

```go
// ❌ Wrong: Specific case after general case
sw := streamz.NewSwitch[int]().
    Case("positive", func(n int) bool { return n > 0 }, processor1).
    Case("large", func(n int) bool { return n > 100 }, processor2) // Never reached!

// ✅ Correct: Specific case first
sw := streamz.NewSwitch[int]().
    Case("large", func(n int) bool { return n > 100 }, processor2).
    Case("positive", func(n int) bool { return n > 0 }, processor1)
```

### 2. Default Behavior

Without a default, unmatched items are silently dropped:

```go
// Items not matching any case are dropped
sw := streamz.NewSwitch[int]().
    Case("even", func(n int) bool { return n%2 == 0 }, evenProcessor)
// Odd numbers will be dropped!

// Better: Add default or catch-all case
sw := streamz.NewSwitch[int]().
    Case("even", func(n int) bool { return n%2 == 0 }, evenProcessor).
    Default(oddProcessor) // Handle all other cases
```

### 3. Condition Performance

Conditions are evaluated for every item, so keep them efficient:

```go
// ❌ Expensive condition
sw.Case("match", func(item Item) bool {
    result := expensiveDatabaseLookup(item.ID)
    return result.IsSpecial
}, processor)

// ✅ Efficient condition
cache := loadSpecialItemsCache()
sw.Case("match", func(item Item) bool {
    return cache.Contains(item.ID)
}, processor)
```

## Performance Characteristics

### Overhead

- **Condition Evaluation**: O(n) where n = number of cases until match
- **Routing Overhead**: ~200ns per item
- **Memory Usage**: Minimal, only statistics tracking

### Benchmarks

```
BenchmarkSwitchTwoCases-12      547053      2165 ns/op      376 B/op      12 allocs/op
BenchmarkSwitchManyCases-12     352894      3389 ns/op      376 B/op      12 allocs/op
```

### Optimization Tips

1. **Order cases by frequency**: Put most common cases first
2. **Use buffering**: Set appropriate buffer sizes for speed differences
3. **Avoid complex conditions**: Pre-compute when possible
4. **Consider alternatives**: For simple two-way splits, use Filter

## Common Patterns

### Priority Processing

```go
// Process items by priority
prioritySwitch := streamz.NewSwitch[Task]().
    Case("immediate", func(t Task) bool {
        return t.Priority >= 9 || t.Deadline.Before(time.Now().Add(time.Hour))
    }, immediateProcessor).
    Case("high", func(t Task) bool {
        return t.Priority >= 7
    }, highPriorityProcessor).
    Case("normal", func(t Task) bool {
        return t.Priority >= 4
    }, normalProcessor).
    Default(lowPriorityProcessor)
```

### Type-Based Routing

```go
// Route different event types
eventRouter := streamz.NewSwitch[interface{}]().
    Case("user", func(v interface{}) bool {
        _, ok := v.(UserEvent)
        return ok
    }, userEventProcessor).
    Case("system", func(v interface{}) bool {
        _, ok := v.(SystemEvent)
        return ok
    }, systemEventProcessor).
    Case("metric", func(v interface{}) bool {
        _, ok := v.(MetricEvent)
        return ok
    }, metricEventProcessor).
    Default(unknownEventProcessor)
```

### Conditional Fanout

```go
// Route to multiple processors based on characteristics
// Note: For true fanout (item goes to multiple processors), use Router
multiRouter := streamz.NewSwitch[Message]().
    Case("notification", func(m Message) bool {
        return m.Type == "notification" && len(m.Recipients) > 0
    }, notificationProcessor).
    Case("broadcast", func(m Message) bool {
        return m.Type == "broadcast" || len(m.Recipients) > 100
    }, broadcastProcessor).
    Case("direct", func(m Message) bool {
        return len(m.Recipients) == 1
    }, directMessageProcessor).
    Default(generalMessageProcessor)
```

## Integration with Other Processors

### With Circuit Breaker

```go
// Protect external services with circuit breaker
protectedUrgent := streamz.NewCircuitBreaker(urgentProcessor).
    FailureThreshold(0.5).
    RecoveryTimeout(30 * time.Second)

orderSwitch := streamz.NewSwitch[Order]().
    Case("urgent", isUrgent, protectedUrgent).
    Default(standardProcessor)
```

### With Retry

```go
// Add retry logic to specific cases
retriableIntl := streamz.NewRetry(intlProcessor).
    MaxAttempts(3).
    OnError(isRetryableError)

orderSwitch := streamz.NewSwitch[Order]().
    Case("international", isInternational, retriableIntl).
    Default(standardProcessor)
```

### With Rate Limiting

```go
// Apply different rate limits
limitedFree := streamz.NewRateLimiter(100, time.Second).
    Wrap(freeUserProcessor)

limitedPaid := streamz.NewRateLimiter(1000, time.Second).
    Wrap(paidUserProcessor)

userSwitch := streamz.NewSwitch[Request]().
    Case("paid", isPaidUser, limitedPaid).
    Default(limitedFree)
```

## See Also

- **[Router Processor](./router.md)**: For routing to multiple processors (fanout)
- **[Filter Processor](./filter.md)**: For simple include/exclude decisions
- **[Pattern Matching Guide](../guides/pattern-matching.md)**: Advanced routing patterns
- **[Best Practices](../guides/best-practices.md)**: Production recommendations