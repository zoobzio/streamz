---
title: Sliding Window
description: Overlapping time-based windows
author: zoobzio
published: 2025-01-09
updated: 2025-01-09
tags:
  - reference
  - processors
  - windowing
---

# Sliding Window

The Sliding Window processor groups items into fixed-size, overlapping time windows that slide forward at regular intervals.

## Overview

Sliding windows create overlapping views of a stream, with new windows starting at regular intervals (slide) and each window covering a fixed duration (size). This allows for smooth, continuous analysis of streaming data with overlap between consecutive windows.

## Basic Usage

```go
import (
    "context"
    "time"
    "github.com/zoobzio/streamz"
)

// 5-minute windows sliding every minute
windower := streamz.NewSlidingWindow[Event](
    5*time.Minute,  // Window size
    1*time.Minute,  // Slide interval
)

windows := windower.Process(ctx, events)
```

## Configuration Options

### Constructor Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `size` | `time.Duration` | Yes | Duration covered by each window |
| `slide` | `time.Duration` | Yes | How often new windows start |

### Methods

| Method | Description |
|--------|-------------|
| `WithName(string)` | Sets a custom name for monitoring |
| `WithTimestamp(func(T) time.Time)` | Use custom timestamp instead of arrival time |

## Usage Examples

### Moving Average

```go
// 5-minute moving average updated every minute
windower := streamz.NewSlidingWindow[Metric](
    5*time.Minute,
    1*time.Minute,
).WithName("moving-average")

windows := windower.Process(ctx, metrics)

for window := range windows {
    var sum float64
    for _, metric := range window.Items {
        sum += metric.Value
    }
    
    avg := sum / float64(len(window.Items))
    
    fmt.Printf("[%s] 5-min average: %.2f\n", 
        window.End.Format("15:04:05"), avg)
    
    // Publish moving average
    publishMovingAverage(window.End, avg)
}
```

### Trend Detection

```go
// Detect trends using overlapping windows
windower := streamz.NewSlidingWindow[Price](
    10*time.Minute,  // Look at 10 minutes of data
    1*time.Minute,   // Update every minute
)

windows := windower.Process(ctx, prices)

var previousAvg float64
for window := range windows {
    // Calculate current average
    var sum float64
    for _, price := range window.Items {
        sum += price.Value
    }
    currentAvg := sum / float64(len(window.Items))
    
    // Detect trend
    if previousAvg > 0 {
        change := (currentAvg - previousAvg) / previousAvg * 100
        
        if change > 5 {
            alert("Upward trend: +%.2f%%", change)
        } else if change < -5 {
            alert("Downward trend: %.2f%%", change)
        }
    }
    
    previousAvg = currentAvg
}
```

### Anomaly Detection

```go
// Detect anomalies using sliding statistics
windower := streamz.NewSlidingWindow[SensorReading](
    30*time.Minute,  // 30-minute baseline
    5*time.Minute,   // Check every 5 minutes
)

windows := windower.Process(ctx, readings)

for window := range windows {
    // Calculate statistics
    values := make([]float64, len(window.Items))
    for i, reading := range window.Items {
        values[i] = reading.Value
    }
    
    mean, stdDev := calculateStats(values)
    
    // Check recent values for anomalies
    recentStart := window.End.Add(-5 * time.Minute)
    for _, reading := range window.Items {
        if reading.Timestamp.After(recentStart) {
            deviation := math.Abs(reading.Value - mean) / stdDev
            if deviation > 3 { // 3 sigma rule
                alertAnomaly(reading, deviation)
            }
        }
    }
}
```

### Rate Monitoring

```go
// Monitor request rate with smooth updates
windower := streamz.NewSlidingWindow[Request](
    1*time.Minute,    // 1-minute rate
    10*time.Second,   // Update every 10 seconds
)

windows := windower.Process(ctx, requests)

for window := range windows {
    rate := float64(len(window.Items)) / 60.0 // requests per second
    
    metrics.RecordRate(window.End, rate)
    
    // Check SLA
    if rate > maxRequestRate {
        throttle.Enable()
        log.Printf("Rate limit exceeded: %.2f req/s", rate)
    } else if rate < maxRequestRate * 0.8 {
        throttle.Disable()
    }
}
```

### Pattern Matching

```go
// Look for patterns in overlapping windows
type LogPattern struct {
    Pattern string
    Count   int
    Window  time.Time
}

windower := streamz.NewSlidingWindow[LogEntry](
    15*time.Minute,  // Pattern window
    1*time.Minute,   // Slide interval
)

windows := windower.Process(ctx, logs)

for window := range windows {
    // Count error patterns
    patterns := make(map[string]int)
    
    for _, log := range window.Items {
        if log.Level == "ERROR" {
            pattern := extractErrorPattern(log.Message)
            patterns[pattern]++
        }
    }
    
    // Report significant patterns
    for pattern, count := range patterns {
        if count > 10 {
            reportPattern(LogPattern{
                Pattern: pattern,
                Count:   count,
                Window:  window.End,
            })
        }
    }
}
```

## Performance Notes

- **Time Complexity**: O(n) where n is items in window
- **Space Complexity**: O(w*n) where w is size/slide ratio
- **Characteristics**:
  - Maintains multiple active windows
  - Higher memory usage than tumbling windows
  - Smooth metric transitions
  - Configurable overlap via size/slide ratio