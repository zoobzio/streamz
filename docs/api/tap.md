# Tap

The Tap processor allows you to observe items passing through a stream without modifying them, useful for logging, debugging, and monitoring.

## Overview

Tap executes a side effect function for each item while passing the items through unchanged. This is ideal for instrumentation, debugging, and analytics without affecting the main data flow.

## Basic Usage

```go
import (
    "context"
    "log"
    "github.com/zoobzio/streamz"
)

// Log items as they pass through
tapper := streamz.NewTap(func(event Event) {
    log.Printf("Event: %+v", event)
})

// Items pass through unchanged but are logged
tapped := tapper.Process(ctx, events)
```

## Configuration Options

### Constructor Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `fn` | `func(T)` | Yes | Side effect function to execute |

### Methods

| Method | Description |
|--------|-------------|
| `WithName(string)` | Sets a custom name for monitoring |

## Usage Examples

### Debug Logging

```go
// Debug log at specific pipeline stage
debugTap := streamz.NewTap(func(order Order) {
    log.Printf("[DEBUG] Order %s: status=%s, amount=%.2f", 
        order.ID, order.Status, order.Amount)
}).WithName("order-debugger")

// Insert tap between processors
validated := validator.Process(ctx, orders)
tapped := debugTap.Process(ctx, validated)
processed := processor.Process(ctx, tapped)
```

### Metrics Collection

```go
// Collect metrics without affecting flow
var (
    messageCount atomic.Int64
    totalBytes   atomic.Int64
)

metricsTap := streamz.NewTap(func(msg Message) {
    messageCount.Add(1)
    totalBytes.Add(int64(len(msg.Body)))
})

// Add metrics collection
messages := metricsTap.Process(ctx, incomingMessages)

// Periodic reporting
go func() {
    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()
    
    for range ticker.C {
        count := messageCount.Load()
        bytes := totalBytes.Load()
        rate := float64(count) / 60.0
        
        log.Printf("Messages: %d, Bytes: %d, Rate: %.2f/sec", 
            count, bytes, rate)
        
        // Reset counters
        messageCount.Store(0)
        totalBytes.Store(0)
    }
}()
```

### Audit Trail

```go
// Create audit trail
auditTap := streamz.NewTap(func(transaction Transaction) {
    audit := AuditEntry{
        Timestamp:   time.Now(),
        UserID:      transaction.UserID,
        Action:      "transaction_processed",
        Amount:      transaction.Amount,
        Details:     fmt.Sprintf("Transaction %s", transaction.ID),
    }
    
    // Write to audit log
    if err := auditLogger.Log(audit); err != nil {
        log.Printf("Audit log failed: %v", err)
    }
}).WithName("audit-tap")

transactions := auditTap.Process(ctx, incomingTransactions)
```

### Performance Monitoring

```go
// Monitor processing latency
type TimedItem[T any] struct {
    Item      T
    StartTime time.Time
}

// Add timing
startTap := streamz.NewTap(func(item *TimedItem[Order]) {
    item.StartTime = time.Now()
})

// Measure latency
endTap := streamz.NewTap(func(item *TimedItem[Order]) {
    latency := time.Since(item.StartTime)
    metrics.RecordLatency("order_processing", latency)
    
    if latency > 100*time.Millisecond {
        log.Printf("Slow processing: Order %s took %v", 
            item.Item.ID, latency)
    }
})

// Use in pipeline
timed := wrapWithTiming(orders)
started := startTap.Process(ctx, timed)
processed := processor.Process(ctx, started)
measured := endTap.Process(ctx, processed)
```

### Multi-Stage Debugging

```go
// Tap at multiple stages for debugging
tap1 := streamz.NewTap(func(raw RawData) {
    log.Printf("Stage 1 (raw): %s", raw.ID)
})

tap2 := streamz.NewTap(func(parsed ParsedData) {
    log.Printf("Stage 2 (parsed): %s, valid=%v", 
        parsed.ID, parsed.IsValid)
})

tap3 := streamz.NewTap(func(enriched EnrichedData) {
    log.Printf("Stage 3 (enriched): %s, score=%.2f", 
        enriched.ID, enriched.Score)
})

// Debug pipeline
raw := tap1.Process(ctx, input)
parsed := parser.Process(ctx, raw)
debugged := tap2.Process(ctx, parsed)
enriched := enricher.Process(ctx, debugged)
final := tap3.Process(ctx, enriched)
```

## Performance Notes

- **Time Complexity**: O(1) per item plus tap function time
- **Space Complexity**: O(1)
- **Characteristics**:
  - Zero-copy pass through
  - Side effects should be fast
  - Non-blocking tap functions recommended
  - No modification of items