---
title: Dropping Buffer
description: Drop oldest items when buffer is full
author: zoobzio
published: 2025-01-09
updated: 2025-01-09
tags:
  - reference
  - processors
  - flow-control
  - backpressure
---

# Dropping Buffer

The Dropping Buffer provides buffering with overflow protection by dropping the oldest items when the buffer is full.

## Overview

Dropping Buffer maintains a fixed-size buffer that automatically drops the oldest items when new items arrive and the buffer is at capacity. This ensures the pipeline never blocks due to a full buffer, making it ideal for real-time systems where recent data is more valuable than old data.

## Basic Usage

```go
import (
    "context"
    "github.com/zoobzio/streamz"
)

// Create a dropping buffer that keeps the latest 100 items
buffer := streamz.NewDroppingBuffer[Event](100)

// Process events - old events dropped if buffer fills
buffered := buffer.Process(ctx, events)
```

## Configuration Options

### Constructor Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `size` | `int` | Yes | Maximum buffer size before dropping begins |

### Methods

| Method | Description |
|--------|-------------|
| `WithName(string)` | Sets a custom name for monitoring |
| `DroppedCount()` | Returns the number of dropped items |

## Usage Examples

### Real-Time Monitoring

```go
// Monitor system metrics, keeping only recent data
metricsBuffer := streamz.NewDroppingBuffer[Metric](1000).
    WithName("metrics-buffer")

buffered := metricsBuffer.Process(ctx, metrics)

// Periodically check dropped metrics
go func() {
    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()
    
    for range ticker.C {
        dropped := metricsBuffer.DroppedCount()
        if dropped > 0 {
            log.Printf("Dropped %d old metrics", dropped)
        }
    }
}()
```

### Live Event Stream

```go
// Handle live events where only recent events matter
liveBuffer := streamz.NewDroppingBuffer[LiveEvent](50)

// Fast producer
go func() {
    for event := range eventSource {
        select {
        case events <- event:
        case <-ctx.Done():
            return
        }
    }
}()

// Slower consumer processes only the most recent events
buffered := liveBuffer.Process(ctx, events)
for event := range buffered {
    // Process recent events, old ones automatically dropped
    processLiveEvent(event)
}
```

### Sensor Data Processing

```go
// IoT sensors sending data faster than we can process
sensorBuffer := streamz.NewDroppingBuffer[SensorReading](500)

readings := sensorBuffer.Process(ctx, sensorInput)

// Chain with other processors
throttled := streamz.NewThrottle[SensorReading](10). // 10 per second
    Process(ctx, readings)

aggregated := streamz.NewAggregate(
    SensorStats{},
    aggregateSensorData,
).WithTimeWindow(1 * time.Minute).
    Process(ctx, throttled)
```

## Performance Notes

- **Time Complexity**: O(1) for both push and drop operations
- **Space Complexity**: O(n) where n is the buffer size
- **Characteristics**: 
  - Non-blocking writes
  - Maintains insertion order
  - Thread-safe operations
  - Zero allocations after buffer is full