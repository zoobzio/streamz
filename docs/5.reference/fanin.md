---
title: FanIn
description: Merge multiple input streams into one
author: zoobzio
published: 2025-01-09
updated: 2025-01-09
tags:
  - reference
  - processors
  - routing
---

# FanIn

The FanIn processor merges multiple input channels into a single output channel, collecting items from all sources concurrently.

## Overview

FanIn combines multiple streams into one, useful for aggregating data from multiple sources, merging parallel processing results, or collecting events from distributed systems. Items are emitted in the order they arrive from any input channel.

## Basic Usage

```go
import (
    "context"
    "github.com/zoobzio/streamz"
)

// Merge three channels into one
merged := streamz.NewFanIn(channel1, channel2, channel3).
    Process(ctx)
```

## Configuration Options

### Constructor Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `inputs...` | `...<-chan T` | Yes | Variable number of input channels |

### Methods

| Method | Description |
|--------|-------------|
| `WithName(string)` | Sets a custom name for monitoring |
| `Process(context)` | Starts merging and returns output channel |

## Usage Examples

### Merging Worker Results

```go
// Parallel workers processing data
worker1 := processPartition(ctx, partition1)
worker2 := processPartition(ctx, partition2)
worker3 := processPartition(ctx, partition3)

// Merge results
fanIn := streamz.NewFanIn(worker1, worker2, worker3).
    WithName("worker-merger")

results := fanIn.Process(ctx)

// Process merged results
for result := range results {
    saveResult(result)
}
```

### Event Aggregation

```go
// Collect events from multiple sources
httpEvents := collectHTTPEvents(ctx)
grpcEvents := collectGRPCEvents(ctx)
wsEvents := collectWebSocketEvents(ctx)

// Merge all event streams
eventFanIn := streamz.NewFanIn(httpEvents, grpcEvents, wsEvents)
allEvents := eventFanIn.Process(ctx)

// Process all events uniformly
for event := range allEvents {
    metrics.RecordEvent(event)
    
    if event.IsError() {
        handleError(event)
    }
}
```

### Distributed Processing

```go
// Fan out for parallel processing
fanOut := streamz.NewFanOut[Task](3)
outputs := fanOut.Process(ctx, tasks)

// Process in parallel with different strategies
processed1 := strategyA.Process(ctx, outputs[0])
processed2 := strategyB.Process(ctx, outputs[1])
processed3 := strategyC.Process(ctx, outputs[2])

// Merge results back
merger := streamz.NewFanIn(processed1, processed2, processed3)
merged := merger.Process(ctx)

// Aggregate results
for result := range merged {
    updateAggregates(result)
}
```

### Multi-Source Monitoring

```go
// Monitor multiple data sources
func MonitorSources(ctx context.Context, sources ...<-chan Metric) <-chan Alert {
    // Merge all metric streams
    fanIn := streamz.NewFanIn(sources...).
        WithName("metric-aggregator")
    
    allMetrics := fanIn.Process(ctx)
    
    // Check thresholds on merged stream
    alerter := streamz.NewMapper(func(m Metric) *Alert {
        if m.Value > m.Threshold {
            return &Alert{
                Source:    m.Source,
                Message:   fmt.Sprintf("%s exceeded threshold", m.Name),
                Severity:  "high",
                Value:     m.Value,
                Threshold: m.Threshold,
            }
        }
        return nil
    })
    
    alerts := alerter.Process(ctx, allMetrics)
    
    // Filter out nil alerts
    filtered := streamz.NewFilter(func(a *Alert) bool {
        return a != nil
    })
    
    return filtered.Process(ctx, alerts)
}
```

### Dynamic Source Addition

```go
// Merge dynamic number of sources
func MergeDynamicSources[T any](ctx context.Context) (<-chan T, func(<-chan T)) {
    sources := make([]<-chan T, 0)
    var mu sync.Mutex
    
    // Channel to signal updates
    updateCh := make(chan struct{}, 1)
    output := make(chan T)
    
    // Add source function
    addSource := func(ch <-chan T) {
        mu.Lock()
        sources = append(sources, ch)
        mu.Unlock()
        
        // Signal update
        select {
        case updateCh <- struct{}{}:
        default:
        }
    }
    
    go func() {
        defer close(output)
        
        for {
            select {
            case <-updateCh:
                // Recreate fan-in with current sources
                mu.Lock()
                if len(sources) > 0 {
                    fanIn := streamz.NewFanIn(sources...)
                    merged := fanIn.Process(ctx)
                    
                    // Forward merged items
                    for item := range merged {
                        select {
                        case output <- item:
                        case <-ctx.Done():
                            mu.Unlock()
                            return
                        }
                    }
                }
                mu.Unlock()
                
            case <-ctx.Done():
                return
            }
        }
    }()
    
    return output, addSource
}
```

## Performance Notes

- **Time Complexity**: O(1) per item
- **Space Complexity**: O(n) where n is number of input channels
- **Characteristics**:
  - Concurrent reading from all inputs
  - Fair scheduling between sources
  - Closes output when all inputs close
  - No buffering beyond channel buffers