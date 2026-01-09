---
title: Router
description: Route items to named destinations
author: zoobzio
published: 2025-01-09
updated: 2025-01-09
tags:
  - reference
  - processors
  - routing
---

# Router Processor

The Router processor implements content-based routing, sending items to different processors based on predicates. It evaluates each item against route predicates and sends it to matching routes.

## Overview

The Router is a powerful stream processing primitive that enables conditional routing of items to multiple downstream processors. It supports both first-match and all-matches routing strategies, allowing you to build complex processing pipelines where different types of items are handled by specialized processors.

## Routing Strategies

### First-Match (Default)
- Items are sent to the first route whose predicate returns true
- Once a match is found, no further routes are evaluated
- Most efficient for mutually exclusive routes

### All-Matches
- Items are sent to all routes whose predicates return true
- Every route is evaluated for every item
- Useful for broadcasting or multi-stage processing

## Key Features

- **Content-Based Routing**: Route items based on their properties
- **Multiple Strategies**: First-match or all-matches routing
- **Optional Default Route**: Fallback for unmatched items
- **Named Routes**: Easy identification and monitoring
- **Concurrent Processing**: All routes process independently
- **Type-Safe**: Full generic type support
- **Zero-Copy**: Items are passed by reference

## Constructor

```go
func NewRouter[T any]() *Router[T]
```

Creates a new content-based router with default configuration.

**Default Configuration:**
- Routing Strategy: First-match
- Buffer Size: 0 (unbuffered)
- Name: "router"

## Fluent API Methods

### AddRoute(name string, predicate func(T) bool, processor Processor[T, T]) *Router[T]

Adds a named route with a predicate and processor.

```go
router := streamz.NewRouter[Order]().
    AddRoute("high-value", func(o Order) bool {
        return o.Total > 1000
    }, highValueProcessor)
```

**Parameters:**
- `name`: Unique identifier for the route
- `predicate`: Function that returns true if item should be routed here
- `processor`: Processor to handle matching items

### WithDefault(processor Processor[T, T]) *Router[T]

Sets a default processor for items that don't match any route.

```go
router := streamz.NewRouter[Event]().
    AddRoute("critical", isCritical, criticalProcessor).
    WithDefault(normalProcessor)
```

Without a default route, unmatched items are dropped.

### AllMatches() *Router[T]

Enables routing to all matching routes instead of just the first.

```go
router := streamz.NewRouter[Message]().
    AllMatches().
    AddRoute("audit", needsAudit, auditProcessor).
    AddRoute("alerts", isAlert, alertProcessor)
```

### FirstMatch() *Router[T]

Explicitly sets first-match routing strategy (default).

```go
router := streamz.NewRouter[Request]().
    FirstMatch().
    AddRoute("api", isAPI, apiProcessor).
    AddRoute("web", isWeb, webProcessor)
```

### WithBufferSize(size int) *Router[T]

Sets the buffer size for route input channels.

```go
router := streamz.NewRouter[Task]().
    WithBufferSize(100).
    AddRoute("urgent", isUrgent, urgentProcessor)
```

This helps prevent blocking when routes process at different speeds.

### WithName(name string) *Router[T]

Sets a custom name for the processor.

```go
router := streamz.NewRouter[Event]().
    WithName("event-router")
```

## Process Method

### Process(ctx context.Context, in <-chan T) RouterOutput[T]

Routes items to appropriate processors based on predicates.

```go
outputs := router.Process(ctx, input)

// Access individual route outputs
highValueOrders := outputs.Routes["high-value"]
normalOrders := outputs.Routes["default"]
```

**Returns:** `RouterOutput[T]` containing a map of route names to output channels.

### ProcessToSingle(ctx context.Context, in <-chan T) <-chan T

Collects all route outputs into a single channel.

```go
output := router.ProcessToSingle(ctx, input)
```

Useful when you need a single output channel for compatibility.

## Usage Examples

### Basic Content-Based Routing

```go
// Route orders by value
orderRouter := streamz.NewRouter[Order]().
    AddRoute("high-value", func(o Order) bool {
        return o.Total > 1000
    }, processHighValue).
    AddRoute("medium-value", func(o Order) bool {
        return o.Total > 100
    }, processMedium).
    WithDefault(processStandard)

outputs := orderRouter.Process(ctx, orders)

// Process each route's output
go handleHighValue(outputs.Routes["high-value"])
go handleMedium(outputs.Routes["medium-value"])
go handleStandard(outputs.Routes["default"])
```

### Multi-Category Routing

```go
// Route events to multiple handlers
eventRouter := streamz.NewRouter[Event]().
    AllMatches(). // Enable all-matches mode
    AddRoute("audit", func(e Event) bool {
        return e.RequiresAudit
    }, auditLogger).
    AddRoute("alerts", func(e Event) bool {
        return e.Severity >= SeverityCritical
    }, alertManager).
    AddRoute("metrics", func(e Event) bool {
        return e.Type == "metric"
    }, metricsCollector)

// Events can go to multiple routes
outputs := eventRouter.Process(ctx, events)
```

### Load Distribution

```go
// Route requests to different servers based on type
loadBalancer := streamz.NewRouter[Request]().
    AddRoute("api-v1", func(r Request) bool {
        return strings.HasPrefix(r.Path, "/api/v1")
    }, apiV1Server).
    AddRoute("api-v2", func(r Request) bool {
        return strings.HasPrefix(r.Path, "/api/v2")
    }, apiV2Server).
    AddRoute("static", func(r Request) bool {
        return strings.HasPrefix(r.Path, "/static")
    }, staticServer).
    WithDefault(webServer)
```

### A/B Testing

```go
// Split traffic for A/B testing
abRouter := streamz.NewRouter[User]().
    AddRoute("test-group", func(u User) bool {
        // 20% of users go to test
        return hash(u.ID) % 100 < 20
    }, testFeatureProcessor).
    WithDefault(normalFeatureProcessor)
```

### Error Routing

```go
// Route based on error types
errorRouter := streamz.NewRouter[Result]().
    AddRoute("retryable", func(r Result) bool {
        return r.Error != nil && isRetryable(r.Error)
    }, retryProcessor).
    AddRoute("dead-letter", func(r Result) bool {
        return r.Error != nil && !isRetryable(r.Error)
    }, deadLetterProcessor).
    AddRoute("success", func(r Result) bool {
        return r.Error == nil
    }, successProcessor)
```

## Advanced Patterns

### Router Builder Pattern

```go
// Use the builder for complex configurations
router := streamz.NewRouterBuilder[Transaction]().
    Route("fraud-check").
        When(func(t Transaction) bool {
            return t.Amount > 10000 || t.RiskScore > 0.8
        }).
        To(fraudDetector).
    Route("international").
        When(func(t Transaction) bool {
            return t.Currency != "USD"
        }).
        To(currencyConverter).
    Route("standard").
        When(func(t Transaction) bool {
            return true // Catch-all
        }).
        To(standardProcessor).
    Build()
```

### Hierarchical Routing

```go
// Create nested routers for complex routing logic
mainRouter := streamz.NewRouter[Message]().
    AddRoute("commands", isCommand, commandRouter).
    AddRoute("queries", isQuery, queryRouter).
    AddRoute("events", isEvent, eventRouter)

// Each sub-router can have its own routing logic
commandRouter := streamz.NewRouter[Message]().
    AddRoute("create", isCreateCommand, createHandler).
    AddRoute("update", isUpdateCommand, updateHandler).
    AddRoute("delete", isDeleteCommand, deleteHandler)
```

### Dynamic Route Selection

```go
// Route based on configuration
type RouteConfig struct {
    Name      string
    Predicate func(Item) bool
    Processor Processor[Item, Item]
}

func buildRouter(configs []RouteConfig) *Router[Item] {
    router := streamz.NewRouter[Item]()
    for _, cfg := range configs {
        router.AddRoute(cfg.Name, cfg.Predicate, cfg.Processor)
    }
    return router
}
```

## Performance Characteristics

### Overhead

- **Predicate Evaluation**: ~50ns per predicate per item
- **First-Match Mode**: Stops at first match (best case: 1 predicate)
- **All-Matches Mode**: Evaluates all predicates (N predicates)
- **Routing Overhead**: ~200ns per routed item

### Benchmarks

```
BenchmarkRouterFirstMatch-12    421526    3776 ns/op    1008 B/op    13 allocs/op
BenchmarkRouterAllMatches-12    222172    5811 ns/op    1160 B/op    16 allocs/op
```

### Memory Usage

- Fixed overhead per router: ~500 bytes
- Per route overhead: ~200 bytes
- No per-item allocations for routing
- Channel buffers as configured

## Best Practices

### 1. Order Routes by Frequency

```go
// Put most common routes first in first-match mode
router := streamz.NewRouter[Request]().
    AddRoute("api", isAPI, apiHandler).      // 70% of traffic
    AddRoute("static", isStatic, cdnHandler). // 20% of traffic
    AddRoute("admin", isAdmin, adminHandler)  // 10% of traffic
```

### 2. Use Specific Predicates

```go
// Good: Specific, fast predicates
router.AddRoute("orders", func(m Message) bool {
    return m.Type == "order" && m.Version == "v2"
}, orderProcessor)

// Avoid: Complex predicates that do heavy computation
router.AddRoute("complex", func(m Message) bool {
    return expensiveValidation(m) && slowCheck(m)
}, processor)
```

### 3. Buffer for Different Processing Speeds

```go
// Use buffers when routes have different processing speeds
router := streamz.NewRouter[Task]().
    WithBufferSize(100).
    AddRoute("fast", isFast, fastProcessor).    // Processes quickly
    AddRoute("slow", isSlow, slowProcessor)     // Takes longer
```

### 4. Handle All Route Outputs

```go
outputs := router.Process(ctx, input)

// Always consume all route outputs to prevent blocking
var wg sync.WaitGroup
for name, ch := range outputs.Routes {
    wg.Add(1)
    go func(routeName string, output <-chan Item) {
        defer wg.Done()
        processRoute(routeName, output)
    }(name, ch)
}
wg.Wait()
```

### 5. Use Default Routes for Completeness

```go
// Always include a default route for unmatched items
router := streamz.NewRouter[Event]().
    AddRoute("known-type-1", isType1, processor1).
    AddRoute("known-type-2", isType2, processor2).
    WithDefault(unknownTypeProcessor) // Don't lose items
```

## Common Patterns

### Pattern Matching Router

```go
// Route based on multiple patterns
patternRouter := streamz.NewRouter[LogEntry]().
    AddRoute("errors", func(e LogEntry) bool {
        return e.Level == "ERROR" || e.Level == "FATAL"
    }, errorHandler).
    AddRoute("warnings", func(e LogEntry) bool {
        return e.Level == "WARN"
    }, warningHandler).
    AddRoute("debug", func(e LogEntry) bool {
        return e.Level == "DEBUG" || e.Level == "TRACE"
    }, debugHandler).
    WithDefault(infoHandler)
```

### Percentage-Based Routing

```go
// Route based on percentages for canary deployments
canaryRouter := streamz.NewRouter[Request]().
    AddRoute("canary", func(r Request) bool {
        // 5% to canary
        return hash(r.SessionID) % 100 < 5
    }, canaryVersion).
    WithDefault(stableVersion)
```

### Time-Based Routing

```go
// Route based on time windows
timeRouter := streamz.NewRouter[Event]().
    AddRoute("business-hours", func(e Event) bool {
        hour := e.Timestamp.Hour()
        return hour >= 9 && hour < 17
    }, businessProcessor).
    AddRoute("after-hours", func(e Event) bool {
        hour := e.Timestamp.Hour()
        return hour < 9 || hour >= 17
    }, batchProcessor)
```

## Integration Examples

### With Retry and Circuit Breaker

```go
// Combine router with resilience patterns
resilientRouter := streamz.NewRouter[Request]().
    AddRoute("external-api", isExternalAPI,
        streamz.NewCircuitBreaker(
            streamz.NewRetry(apiProcessor).
                MaxAttempts(3),
        ).FailureThreshold(0.5),
    ).
    AddRoute("internal", isInternal, internalProcessor)
```

### With Transformation Pipeline

```go
// Route to different transformation pipelines
pipeline := func(ctx context.Context, input <-chan Data) <-chan Result {
    router := streamz.NewRouter[Data]().
        AddRoute("json", func(d Data) bool {
            return d.Format == "json"
        }, jsonPipeline).
        AddRoute("xml", func(d Data) bool {
            return d.Format == "xml"  
        }, xmlPipeline).
        AddRoute("csv", func(d Data) bool {
            return d.Format == "csv"
        }, csvPipeline)
    
    // Merge all outputs back together
    return router.ProcessToSingle(ctx, input)
}
```

## Monitoring

```go
// Add monitoring to routes
func monitoredRoute(name string, processor Processor[T,T]) Processor[T,T] {
    return streamz.NewMonitor(processor).
        OnItem(func(item T, d time.Duration) {
            routeMetrics.RecordLatency(name, d)
            routeMetrics.IncrementCount(name)
        })
}

router := streamz.NewRouter[Event]().
    AddRoute("critical", isCritical, 
        monitoredRoute("critical", criticalProcessor)).
    AddRoute("normal", isNormal,
        monitoredRoute("normal", normalProcessor))
```

## See Also

- **[Fanout Processor](./fanout.md)**: Broadcasts to multiple outputs
- **[Filter Processor](./filter.md)**: Simple conditional processing
- **[Switch Processor](./switch.md)**: Binary routing (when available)
- **[Best Practices](../guides/best-practices.md)**: General patterns
- **[Pipeline Patterns](../guides/patterns.md)**: Complex routing examples