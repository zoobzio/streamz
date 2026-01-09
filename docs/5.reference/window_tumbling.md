---
title: Tumbling Window
description: Fixed, non-overlapping time windows
author: zoobzio
published: 2025-01-09
updated: 2025-01-09
tags:
  - reference
  - processors
  - windowing
---

# Tumbling Window

The Tumbling Window processor groups items into fixed-size, non-overlapping time windows.

## Overview

Tumbling windows divide a stream into consecutive, non-overlapping time periods. Each item belongs to exactly one window based on its arrival time. Windows are emitted when they close, containing all items that arrived during that time period.

## Basic Usage

```go
import (
    "context"
    "time"
    "github.com/zoobzio/streamz"
)

// Create 1-minute tumbling windows
windower := streamz.NewTumblingWindow[Event](time.Minute)

windows := windower.Process(ctx, events)
for window := range windows {
    fmt.Printf("Window [%s - %s]: %d events\n", 
        window.Start, window.End, len(window.Items))
}
```

## Configuration Options

### Constructor Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `size` | `time.Duration` | Yes | Duration of each window |

### Methods

| Method | Description |
|--------|-------------|
| `WithName(string)` | Sets a custom name for monitoring |
| `WithTimestamp(func(T) time.Time)` | Use custom timestamp instead of arrival time |

## Usage Examples

### Time-Based Aggregation

```go
// Aggregate metrics by minute
windower := streamz.NewTumblingWindow[Metric](time.Minute).
    WithName("metric-windower")

windows := windower.Process(ctx, metrics)

for window := range windows {
    // Calculate statistics for this minute
    var sum, count float64
    for _, metric := range window.Items {
        sum += metric.Value
        count++
    }
    
    avg := sum / count
    fmt.Printf("[%s] Average: %.2f, Count: %.0f\n", 
        window.Start.Format("15:04"), avg, count)
    
    // Store aggregated result
    storeWindowedMetric(window.Start, avg, count)
}
```

### Batch Processing by Time

```go
// Process orders in 5-minute batches
windower := streamz.NewTumblingWindow[Order](5 * time.Minute)

windows := windower.Process(ctx, orders)

for window := range windows {
    log.Printf("Processing batch of %d orders from %s", 
        len(window.Items), window.Start.Format("15:04:05"))
    
    // Bulk process all orders in window
    err := processOrderBatch(window.Items)
    if err != nil {
        log.Printf("Batch processing failed: %v", err)
        // Handle individual orders
        for _, order := range window.Items {
            processOrderIndividually(order)
        }
    }
}
```

### Event Rate Calculation

```go
// Calculate events per second using 10-second windows
windower := streamz.NewTumblingWindow[Event](10 * time.Second)

windows := windower.Process(ctx, events)

for window := range windows {
    duration := window.End.Sub(window.Start).Seconds()
    rate := float64(len(window.Items)) / duration
    
    log.Printf("Event rate: %.2f events/sec at %s", 
        rate, window.Start.Format("15:04:05"))
    
    // Alert on high rates
    if rate > 1000 {
        alert("High event rate detected: %.2f/sec", rate)
    }
}
```

### Session Analysis

```go
// Analyze user sessions in hourly windows
windower := streamz.NewTumblingWindow[SessionEvent](time.Hour).
    WithTimestamp(func(e SessionEvent) time.Time {
        return e.Timestamp
    })

windows := windower.Process(ctx, sessionEvents)

for window := range windows {
    // Group events by session
    sessions := make(map[string][]SessionEvent)
    for _, event := range window.Items {
        sessions[event.SessionID] = append(sessions[event.SessionID], event)
    }
    
    // Calculate session metrics
    var totalDuration time.Duration
    var completedSessions int
    
    for _, events := range sessions {
        if len(events) >= 2 {
            duration := events[len(events)-1].Timestamp.Sub(events[0].Timestamp)
            totalDuration += duration
            completedSessions++
        }
    }
    
    avgDuration := totalDuration / time.Duration(completedSessions)
    log.Printf("Hour %s: %d sessions, avg duration: %v", 
        window.Start.Format("15:04"), completedSessions, avgDuration)
}
```

### Combined with Other Processors

```go
// Window then aggregate
windower := streamz.NewTumblingWindow[Sale](15 * time.Minute)
windows := windower.Process(ctx, sales)

// Calculate window statistics
statsMapper := streamz.NewMapper(func(w streamz.Window[Sale]) SaleStats {
    var total float64
    categories := make(map[string]int)
    
    for _, sale := range w.Items {
        total += sale.Amount
        categories[sale.Category]++
    }
    
    return SaleStats{
        Window:     w.Start,
        Total:      total,
        Count:      len(w.Items),
        Categories: categories,
    }
})

stats := statsMapper.Process(ctx, windows)

// Further process statistics
for stat := range stats {
    publishStats(stat)
    updateDashboard(stat)
}
```

## Performance Notes

- **Time Complexity**: O(1) per item insertion
- **Space Complexity**: O(n) where n is items in current window
- **Characteristics**:
  - Fixed memory per window duration
  - Windows emit only when complete
  - No overlap between windows
  - Timer-based window closing